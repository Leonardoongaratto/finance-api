package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Leonardoongaratto/finance-api/internal/domain"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func mustAccount(t *testing.T, s *Store) domain.Account {
	t.Helper()
	a, err := s.CreateAccount(context.Background(), domain.Account{Name: "Conta", Type: domain.AccountChecking})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func firstCategory(t *testing.T, s *Store) domain.Category {
	t.Helper()
	cats, err := s.ListCategories(context.Background())
	if err != nil || len(cats) == 0 {
		t.Fatalf("categories: %v %v", cats, err)
	}
	return cats[0]
}

func TestOpenSeedsDefaultCategoriesOnce(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "finance.db")

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	cats, _ := s.ListCategories(ctx)
	if len(cats) != 9 {
		t.Fatalf("seeded %d categories, want 9", len(cats))
	}
	for _, c := range cats {
		if err := s.DeleteCategory(ctx, c.ID); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	if cats, _ := reopened.ListCategories(ctx); len(cats) != 0 {
		t.Fatalf("migrations must not reseed after reopening, got %d", len(cats))
	}
}

func TestImportMigrationIsIdempotentAndEnforcesUniqueSourceRef(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "finance.db")

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	acc := mustAccount(t, s)
	cat := firstCategory(t, s)
	if _, err := s.CreateTransaction(ctx, domain.Transaction{
		Type: domain.Expense, AmountCents: 100, Date: "2026-09-01",
		AccountID: acc.ID, CategoryID: cat.ID, SourceRef: "DUP",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateTransaction(ctx, domain.Transaction{
		Type: domain.Expense, AmountCents: 200, Date: "2026-09-02",
		AccountID: acc.ID, CategoryID: cat.ID, SourceRef: "DUP",
	}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate source_ref in same account should conflict, got %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopening must not re-run migration 002: %v", err)
	}
	defer func() { _ = reopened.Close() }()
	all, err := reopened.ListTransactions(ctx, TransactionFilter{})
	if err != nil || len(all) != 1 {
		t.Fatalf("transactions after reopen: %+v %v", all, err)
	}
}

func TestOpenFailsForUnusablePath(t *testing.T) {
	if _, err := Open(filepath.Join(t.TempDir(), "missing-dir", "x.db")); err == nil {
		t.Fatal("expected error")
	}
}

func TestAccountsCRUD(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	created, err := s.CreateAccount(ctx, domain.Account{Name: "Nubank", Type: domain.AccountCreditCard})
	if err != nil || created.ID == 0 {
		t.Fatalf("create: %+v %v", created, err)
	}
	updated, err := s.UpdateAccount(ctx, domain.Account{ID: created.ID, Name: "Itau", Type: domain.AccountSavings})
	if err != nil || updated.Name != "Itau" {
		t.Fatalf("update: %+v %v", updated, err)
	}
	list, _ := s.ListAccounts(ctx)
	if len(list) != 1 || list[0].Type != domain.AccountSavings {
		t.Fatalf("list: %+v", list)
	}
	if _, err := s.UpdateAccount(ctx, domain.Account{ID: 999, Name: "x", Type: domain.AccountCash}); !errors.Is(err, ErrNotFound) {
		t.Errorf("update missing: %v", err)
	}
	if err := s.DeleteAccount(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteAccount(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete missing: %v", err)
	}
}

func TestCategoriesConflicts(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	if _, err := s.CreateCategory(ctx, domain.Category{Name: "Moradia"}); !errors.Is(err, ErrConflict) {
		t.Errorf("duplicate name: %v", err)
	}
	c, err := s.CreateCategory(ctx, domain.Category{Name: "Pets"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateCategory(ctx, domain.Category{ID: c.ID, Name: "Moradia"}); !errors.Is(err, ErrConflict) {
		t.Errorf("rename to duplicate: %v", err)
	}
	renamed, err := s.UpdateCategory(ctx, domain.Category{ID: c.ID, Name: "Animais"})
	if err != nil || renamed.Name != "Animais" {
		t.Errorf("rename: %+v %v", renamed, err)
	}
	if _, err := s.UpdateCategory(ctx, domain.Category{ID: 999, Name: "Zzz"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("update missing: %v", err)
	}
}

func TestTransactionsCRUDAndFilters(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	acc := mustAccount(t, s)
	cat := firstCategory(t, s)

	base := domain.Transaction{Type: domain.Expense, AmountCents: 1500, Date: "2026-09-10", AccountID: acc.ID, CategoryID: cat.ID, Description: "Almoco"}
	tx, err := s.CreateTransaction(ctx, base)
	if err != nil || tx.ID == 0 {
		t.Fatalf("create: %+v %v", tx, err)
	}
	income := base
	income.Type, income.Date, income.AmountCents = domain.Income, "2026-10-05", 90000
	if _, err := s.CreateTransaction(ctx, income); err != nil {
		t.Fatal(err)
	}

	all, _ := s.ListTransactions(ctx, TransactionFilter{})
	if len(all) != 2 || all[0].Date != "2026-10-05" {
		t.Fatalf("all (newest first): %+v", all)
	}
	assertCount := func(name string, f TransactionFilter, want int) {
		t.Helper()
		got, err := s.ListTransactions(ctx, f)
		if err != nil || len(got) != want {
			t.Errorf("%s: got %d (%v), want %d", name, len(got), err, want)
		}
	}
	assertCount("from", TransactionFilter{From: "2026-10-01"}, 1)
	assertCount("to", TransactionFilter{To: "2026-09-30"}, 1)
	assertCount("type", TransactionFilter{Type: domain.Income}, 1)
	assertCount("account", TransactionFilter{AccountID: acc.ID}, 2)
	assertCount("other account", TransactionFilter{AccountID: acc.ID + 1}, 0)
	assertCount("category", TransactionFilter{CategoryID: cat.ID}, 2)

	tx.AmountCents = 2000
	updated, err := s.UpdateTransaction(ctx, tx)
	if err != nil || updated.AmountCents != 2000 {
		t.Fatalf("update: %+v %v", updated, err)
	}
	if _, err := s.UpdateTransaction(ctx, domain.Transaction{ID: 999, Type: domain.Income, AmountCents: 1, Date: "2026-01-01", AccountID: acc.ID, CategoryID: cat.ID}); !errors.Is(err, ErrNotFound) {
		t.Errorf("update missing: %v", err)
	}
	if _, err := s.UpdateTransaction(ctx, domain.Transaction{ID: tx.ID, Type: domain.Income, AmountCents: 1, Date: "2026-01-01", AccountID: 999, CategoryID: cat.ID}); !errors.Is(err, ErrInvalidReference) {
		t.Errorf("update with unknown account: %v", err)
	}
	if err := s.DeleteTransaction(ctx, tx.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteTransaction(ctx, tx.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete missing: %v", err)
	}
}

func TestCreateTransactionRejectsUnknownReferences(t *testing.T) {
	s := newStore(t)
	_, err := s.CreateTransaction(context.Background(), domain.Transaction{Type: domain.Expense, AmountCents: 10, Date: "2026-01-01", AccountID: 42, CategoryID: 42})
	if !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("got %v", err)
	}
}

func TestInstallmentGroupLifecycle(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	acc, cat := mustAccount(t, s), firstCategory(t, s)

	txs, _ := domain.BuildInstallments(domain.InstallmentPlan{TotalCents: 10000, Installments: 3, FirstDate: "2026-10-01", AccountID: acc.ID, CategoryID: cat.ID, Description: "TV"}, "g1")
	created, err := s.CreateTransactions(ctx, txs)
	if err != nil || len(created) != 3 || created[0].ID == 0 {
		t.Fatalf("create: %+v %v", created, err)
	}
	if n, _ := s.ListTransactions(ctx, TransactionFilter{}); len(n) != 3 {
		t.Fatalf("listed %d", len(n))
	}

	if err := s.DeleteInstallmentGroup(ctx, "g1"); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.ListTransactions(ctx, TransactionFilter{}); len(n) != 0 {
		t.Fatalf("group not removed: %d", len(n))
	}
	if err := s.DeleteInstallmentGroup(ctx, "g1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("second delete: %v", err)
	}
}

func TestCreateTransactionsIsAtomic(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	acc, cat := mustAccount(t, s), firstCategory(t, s)
	good := domain.Transaction{Type: domain.Expense, AmountCents: 10, Date: "2026-01-01", AccountID: acc.ID, CategoryID: cat.ID}
	bad := good
	bad.AccountID = 999

	if _, err := s.CreateTransactions(ctx, []domain.Transaction{good, bad}); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("got %v", err)
	}
	if n, _ := s.ListTransactions(ctx, TransactionFilter{}); len(n) != 0 {
		t.Fatalf("partial insert leaked %d rows", len(n))
	}
}

func TestDeleteInUseAccountAndCategoryConflict(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	acc, cat := mustAccount(t, s), firstCategory(t, s)
	if _, err := s.CreateTransaction(ctx, domain.Transaction{Type: domain.Expense, AmountCents: 10, Date: "2026-01-01", AccountID: acc.ID, CategoryID: cat.ID}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteCategory(ctx, cat.ID); !errors.Is(err, ErrConflict) {
		t.Errorf("category in use: %v", err)
	}
	if err := s.DeleteAccount(ctx, acc.ID); !errors.Is(err, ErrConflict) {
		t.Errorf("account in use: %v", err)
	}
}

func TestInvestmentsWithChildren(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	inv, err := s.CreateInvestment(ctx, domain.Investment{Name: "CDB", Type: domain.InvestmentFixedIncome})
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.AddInvestmentContribution(ctx, domain.Contribution{ParentID: inv.ID, Date: "2026-01-10", AmountCents: 100000})
	if err != nil || c.ID == 0 {
		t.Fatalf("contribution: %+v %v", c, err)
	}
	v, err := s.AddInvestmentValuation(ctx, domain.Valuation{ParentID: inv.ID, Date: "2026-02-10", ValueCents: 110000})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddInvestmentContribution(ctx, domain.Contribution{ParentID: 999, Date: "2026-01-10", AmountCents: 1}); !errors.Is(err, ErrInvalidReference) {
		t.Errorf("unknown parent: %v", err)
	}

	contribs, _ := s.ListInvestmentContributions(ctx)
	vals, _ := s.ListInvestmentValuations(ctx)
	if len(contribs) != 1 || contribs[0].ParentID != inv.ID || len(vals) != 1 || vals[0].ValueCents != 110000 {
		t.Fatalf("children: %+v %+v", contribs, vals)
	}

	renamed, err := s.UpdateInvestment(ctx, domain.Investment{ID: inv.ID, Name: "CDB 2", Type: domain.InvestmentTreasury})
	if err != nil || renamed.Type != domain.InvestmentTreasury {
		t.Fatalf("update: %+v %v", renamed, err)
	}
	if _, err := s.UpdateInvestment(ctx, domain.Investment{ID: 999, Name: "x", Type: domain.InvestmentOther}); !errors.Is(err, ErrNotFound) {
		t.Errorf("update missing: %v", err)
	}

	if err := s.DeleteInvestmentContribution(ctx, 999, c.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("wrong parent must not delete: %v", err)
	}
	if err := s.DeleteInvestmentContribution(ctx, inv.ID, c.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteInvestmentValuation(ctx, inv.ID, v.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := s.AddInvestmentContribution(ctx, domain.Contribution{ParentID: inv.ID, Date: "2026-03-01", AmountCents: 5}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteInvestment(ctx, inv.ID); err != nil {
		t.Fatal(err)
	}
	if contribs, _ := s.ListInvestmentContributions(ctx); len(contribs) != 0 {
		t.Errorf("children must cascade, got %d", len(contribs))
	}
	if err := s.DeleteInvestment(ctx, inv.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete missing: %v", err)
	}
	if list, _ := s.ListInvestments(ctx); len(list) != 0 {
		t.Errorf("list: %+v", list)
	}
}

func TestGoalsWithContributions(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	goal, err := s.CreateGoal(ctx, domain.Goal{Name: "Viagem", TargetCents: 500000, Deadline: "2027-06-01", StartDate: "2026-10-01"})
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.AddGoalContribution(ctx, domain.Contribution{ParentID: goal.ID, Date: "2026-10-05", AmountCents: 20000})
	if err != nil {
		t.Fatal(err)
	}
	if list, _ := s.ListGoalContributions(ctx); len(list) != 1 || list[0].AmountCents != 20000 {
		t.Fatalf("contributions: %+v", list)
	}
	if _, err := s.AddGoalContribution(ctx, domain.Contribution{ParentID: 999, Date: "2026-10-05", AmountCents: 1}); !errors.Is(err, ErrInvalidReference) {
		t.Errorf("unknown parent: %v", err)
	}
	goal.TargetCents = 600000
	if updated, err := s.UpdateGoal(ctx, goal); err != nil || updated.TargetCents != 600000 {
		t.Fatalf("update: %+v %v", updated, err)
	}
	if _, err := s.UpdateGoal(ctx, domain.Goal{ID: 999, Name: "x", TargetCents: 1, Deadline: "2027-01-01", StartDate: "2026-01-01"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("update missing: %v", err)
	}
	if list, _ := s.ListGoals(ctx); len(list) != 1 {
		t.Fatalf("list: %+v", list)
	}
	if err := s.DeleteGoalContribution(ctx, goal.ID, c.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteGoal(ctx, goal.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteGoal(ctx, goal.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete missing: %v", err)
	}
}

func TestDebtsWithPayments(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)

	debt, err := s.CreateDebt(ctx, domain.Debt{Name: "Cartao", Creditor: "Banco", OriginalCents: 100000, MonthlyRateBP: 150, MonthlyPaymentCents: 20000, StartDate: "2026-01-05"})
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.AddDebtPayment(ctx, domain.Payment{ParentID: debt.ID, Date: "2026-02-05", AmountCents: 20000})
	if err != nil {
		t.Fatal(err)
	}
	if list, _ := s.ListDebtPayments(ctx); len(list) != 1 || list[0].ParentID != debt.ID {
		t.Fatalf("payments: %+v", list)
	}
	if _, err := s.AddDebtPayment(ctx, domain.Payment{ParentID: 999, Date: "2026-02-05", AmountCents: 1}); !errors.Is(err, ErrInvalidReference) {
		t.Errorf("unknown parent: %v", err)
	}
	debt.Creditor = "Outro"
	if updated, err := s.UpdateDebt(ctx, debt); err != nil || updated.Creditor != "Outro" {
		t.Fatalf("update: %+v %v", updated, err)
	}
	if _, err := s.UpdateDebt(ctx, domain.Debt{ID: 999, Name: "x", OriginalCents: 1, StartDate: "2026-01-01"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("update missing: %v", err)
	}
	if list, _ := s.ListDebts(ctx); len(list) != 1 || list[0].MonthlyRateBP != 150 {
		t.Fatalf("list: %+v", list)
	}
	if err := s.DeleteDebtPayment(ctx, debt.ID, p.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteDebtPayment(ctx, debt.ID, p.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete missing payment: %v", err)
	}
	if err := s.DeleteDebt(ctx, debt.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteDebt(ctx, debt.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete missing: %v", err)
	}
}

func TestStoreErrorsAfterClose(t *testing.T) {
	s := newStore(t)
	_ = s.Close()
	if _, err := s.ListAccounts(context.Background()); err == nil {
		t.Error("ListAccounts on closed store should fail")
	}
	if _, err := s.CreateAccount(context.Background(), domain.Account{Name: "x", Type: domain.AccountCash}); err == nil {
		t.Error("CreateAccount on closed store should fail")
	}
	if _, err := s.CreateTransactions(context.Background(), []domain.Transaction{{}}); err == nil {
		t.Error("CreateTransactions on closed store should fail")
	}
}
