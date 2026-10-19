package store

import (
	"context"
	"errors"

	"github.com/Leonardoongaratto/finance-api/internal/domain"
)

// DuplicateSourceRefs reports which of the given source refs are already
// recorded for the account, so an OFX preview can flag repeated imports. It
// loads the account's existing refs once and compares in memory, instead of
// an IN (...) with one placeholder per ref, which SQLite caps at 32766
// bound parameters: a large statement would otherwise fail with a 500.
func (s *Store) DuplicateSourceRefs(ctx context.Context, accountID int64, refs []string) (map[string]bool, error) {
	found := map[string]bool{}
	if len(refs) == 0 {
		return found, nil
	}

	wanted := make(map[string]bool, len(refs))
	for _, ref := range refs {
		wanted[ref] = true
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT source_ref FROM transactions WHERE account_id = ? AND source_ref <> ''`, accountID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var ref string
		if err := rows.Scan(&ref); err != nil {
			return nil, err
		}
		if wanted[ref] {
			found[ref] = true
		}
	}
	return found, rows.Err()
}

// LastCategoryByDescription maps each normalized description already used
// in this account to the category of its most recent transaction.
func (s *Store) LastCategoryByDescription(ctx context.Context, accountID int64) (map[string]int64, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT description, category_id FROM transactions WHERE account_id = ? ORDER BY date ASC, id ASC`,
		accountID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	history := map[string]int64{}
	for rows.Next() {
		var description string
		var categoryID int64
		if err := rows.Scan(&description, &categoryID); err != nil {
			return nil, err
		}
		history[domain.NormalizeDescription(description)] = categoryID
	}
	return history, rows.Err()
}

// ImportTransactions writes an OFX import's confirmed rows inside a single
// transaction: a row whose (account_id, source_ref) already exists is
// skipped instead of failing, but any other error rolls back everything, so
// a confirm either lands in full or not at all.
func (s *Store) ImportTransactions(ctx context.Context, rows []domain.Transaction) ([]domain.Transaction, int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = tx.Rollback() }()

	created := make([]domain.Transaction, 0, len(rows))
	skipped := 0
	for _, t := range rows {
		result, err := tx.ExecContext(ctx,
			insertTransactionSQL,
			t.Type, t.AmountCents, t.Date, t.AccountID, t.CategoryID, t.Description, t.InstallmentGroup, t.SourceRef)
		if err != nil {
			mapped := mapError(err, ErrInvalidReference)
			if errors.Is(mapped, ErrConflict) {
				skipped++
				continue
			}
			return nil, 0, mapped
		}
		if t.ID, err = result.LastInsertId(); err != nil {
			return nil, 0, err
		}
		created = append(created, t)
	}
	return created, skipped, tx.Commit()
}
