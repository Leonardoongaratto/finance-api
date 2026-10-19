package store

import (
	"context"
	"strconv"
	"testing"

	"github.com/Leonardoongaratto/finance-api/internal/domain"
)

func TestDuplicateSourceRefsFindsOnlyExistingOnes(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	acc := mustAccount(t, s)
	cat := firstCategory(t, s)

	if _, err := s.CreateTransaction(ctx, domain.Transaction{
		Type: domain.Expense, AmountCents: 100, Date: "2026-09-01",
		AccountID: acc.ID, CategoryID: cat.ID, SourceRef: "FITID-1",
	}); err != nil {
		t.Fatal(err)
	}

	found, err := s.DuplicateSourceRefs(ctx, acc.ID, []string{"FITID-1", "FITID-2"})
	if err != nil {
		t.Fatal(err)
	}
	if !found["FITID-1"] || found["FITID-2"] {
		t.Fatalf("found = %+v", found)
	}
}

func TestDuplicateSourceRefsHandlesMoreRefsThanSQLiteParameterLimit(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	acc := mustAccount(t, s)
	cat := firstCategory(t, s)

	if _, err := s.CreateTransaction(ctx, domain.Transaction{
		Type: domain.Expense, AmountCents: 100, Date: "2026-09-01",
		AccountID: acc.ID, CategoryID: cat.ID, SourceRef: "FITID-1",
	}); err != nil {
		t.Fatal(err)
	}

	refs := make([]string, 40000)
	for i := range refs {
		refs[i] = "REF-" + strconv.Itoa(i)
	}
	refs[12345] = "FITID-1"

	found, err := s.DuplicateSourceRefs(ctx, acc.ID, refs)
	if err != nil {
		t.Fatalf("DuplicateSourceRefs with %d refs: %v", len(refs), err)
	}
	if !found["FITID-1"] || len(found) != 1 {
		t.Fatalf("found = %d entries, want only FITID-1", len(found))
	}
}

func TestDuplicateSourceRefsScopedPerAccount(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	acc1 := mustAccount(t, s)
	acc2, err := s.CreateAccount(ctx, domain.Account{Name: "Outra", Type: domain.AccountSavings})
	if err != nil {
		t.Fatal(err)
	}
	cat := firstCategory(t, s)

	if _, err := s.CreateTransaction(ctx, domain.Transaction{
		Type: domain.Expense, AmountCents: 100, Date: "2026-09-01",
		AccountID: acc1.ID, CategoryID: cat.ID, SourceRef: "FITID-1",
	}); err != nil {
		t.Fatal(err)
	}

	found, err := s.DuplicateSourceRefs(ctx, acc2.ID, []string{"FITID-1"})
	if err != nil {
		t.Fatal(err)
	}
	if found["FITID-1"] {
		t.Fatalf("cross-account false positive: %+v", found)
	}
}

func TestLastCategoryByDescriptionUsesMostRecentMatch(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	acc := mustAccount(t, s)
	cats, _ := s.ListCategories(ctx)
	first, second := cats[0], cats[1]

	if _, err := s.CreateTransaction(ctx, domain.Transaction{
		Type: domain.Expense, AmountCents: 100, Date: "2026-08-01",
		AccountID: acc.ID, CategoryID: first.ID, Description: "Mercado",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateTransaction(ctx, domain.Transaction{
		Type: domain.Expense, AmountCents: 200, Date: "2026-09-01",
		AccountID: acc.ID, CategoryID: second.ID, Description: "Mercado",
	}); err != nil {
		t.Fatal(err)
	}

	history, err := s.LastCategoryByDescription(ctx, acc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if history["MERCADO"] != second.ID {
		t.Fatalf("history = %+v, want most recent category %d", history, second.ID)
	}
}

func TestImportTransactionsSkipsExistingSourceRef(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	acc := mustAccount(t, s)
	cat := firstCategory(t, s)

	if _, err := s.CreateTransaction(ctx, domain.Transaction{
		Type: domain.Expense, AmountCents: 100, Date: "2026-09-01",
		AccountID: acc.ID, CategoryID: cat.ID, SourceRef: "DUP-1",
	}); err != nil {
		t.Fatal(err)
	}

	rows := []domain.Transaction{
		{Type: domain.Expense, AmountCents: 100, Date: "2026-09-01", AccountID: acc.ID, CategoryID: cat.ID, SourceRef: "DUP-1"},
		{Type: domain.Income, AmountCents: 500, Date: "2026-09-02", AccountID: acc.ID, CategoryID: cat.ID, SourceRef: "NEW-1"},
	}
	created, skipped, err := s.ImportTransactions(ctx, rows)
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 1 || skipped != 1 {
		t.Fatalf("created=%d skipped=%d, want 1 and 1", len(created), skipped)
	}
	if created[0].SourceRef != "NEW-1" {
		t.Fatalf("created = %+v", created)
	}
}

func TestImportTransactionsRollsBackOnInvalidReference(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	acc := mustAccount(t, s)
	cat := firstCategory(t, s)

	rows := []domain.Transaction{
		{Type: domain.Expense, AmountCents: 100, Date: "2026-09-01", AccountID: acc.ID, CategoryID: cat.ID, SourceRef: "OK-1"},
		{Type: domain.Expense, AmountCents: 100, Date: "2026-09-01", AccountID: acc.ID, CategoryID: 999999, SourceRef: "BAD-1"},
	}
	if _, _, err := s.ImportTransactions(ctx, rows); err == nil {
		t.Fatal("expected error for invalid category_id")
	}

	all, err := s.ListTransactions(ctx, TransactionFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 0 {
		t.Fatalf("transaction leaked outside rolled-back import: %+v", all)
	}
}

func TestImportTransactionsReimportingSameFileSkipsAll(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	acc := mustAccount(t, s)
	cat := firstCategory(t, s)

	rows := []domain.Transaction{
		{Type: domain.Expense, AmountCents: 100, Date: "2026-09-01", AccountID: acc.ID, CategoryID: cat.ID, SourceRef: "A"},
		{Type: domain.Expense, AmountCents: 200, Date: "2026-09-02", AccountID: acc.ID, CategoryID: cat.ID, SourceRef: "B"},
	}
	if _, skipped, err := s.ImportTransactions(ctx, rows); err != nil || skipped != 0 {
		t.Fatalf("first import: skipped=%d err=%v", skipped, err)
	}

	created, skipped, err := s.ImportTransactions(ctx, rows)
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 0 || skipped != 2 {
		t.Fatalf("second import created=%d skipped=%d, want 0 and 2", len(created), skipped)
	}
}
