package store

import (
	"context"
	"strings"

	"github.com/Leonardoongaratto/finance-api/internal/domain"
)

type TransactionFilter struct {
	From       domain.Date
	To         domain.Date
	AccountID  int64
	CategoryID int64
	Type       domain.TransactionType
}

const (
	transactionColumns   = `id, type, amount_cents, date, account_id, category_id, description, installment_group, source_ref`
	insertTransactionSQL = `INSERT INTO transactions (type, amount_cents, date, account_id, category_id, description, installment_group, source_ref) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
)

func (s *Store) ListTransactions(ctx context.Context, f TransactionFilter) ([]domain.Transaction, error) {
	var conditions []string
	var args []any
	add := func(condition string, arg any) {
		conditions = append(conditions, condition)
		args = append(args, arg)
	}
	if f.From != "" {
		add("date >= ?", f.From)
	}
	if f.To != "" {
		add("date <= ?", f.To)
	}
	if f.AccountID != 0 {
		add("account_id = ?", f.AccountID)
	}
	if f.CategoryID != 0 {
		add("category_id = ?", f.CategoryID)
	}
	if f.Type != "" {
		add("type = ?", f.Type)
	}

	query := `SELECT ` + transactionColumns + ` FROM transactions`
	if len(conditions) > 0 {
		query += ` WHERE ` + strings.Join(conditions, " AND ")
	}
	query += ` ORDER BY date DESC, id DESC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	transactions := []domain.Transaction{}
	for rows.Next() {
		var t domain.Transaction
		if err := rows.Scan(&t.ID, &t.Type, &t.AmountCents, &t.Date, &t.AccountID, &t.CategoryID, &t.Description, &t.InstallmentGroup, &t.SourceRef); err != nil {
			return nil, err
		}
		transactions = append(transactions, t)
	}
	return transactions, rows.Err()
}

func (s *Store) CreateTransaction(ctx context.Context, t domain.Transaction) (domain.Transaction, error) {
	id, err := s.insert(ctx,
		insertTransactionSQL,
		t.Type, t.AmountCents, t.Date, t.AccountID, t.CategoryID, t.Description, t.InstallmentGroup, t.SourceRef)
	if err != nil {
		return domain.Transaction{}, err
	}
	t.ID = id
	return t, nil
}

func (s *Store) CreateTransactions(ctx context.Context, transactions []domain.Transaction) ([]domain.Transaction, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	created := make([]domain.Transaction, len(transactions))
	for i, t := range transactions {
		result, err := tx.ExecContext(ctx,
			insertTransactionSQL,
			t.Type, t.AmountCents, t.Date, t.AccountID, t.CategoryID, t.Description, t.InstallmentGroup, t.SourceRef)
		if err != nil {
			return nil, mapError(err, ErrInvalidReference)
		}
		if t.ID, err = result.LastInsertId(); err != nil {
			return nil, err
		}
		created[i] = t
	}
	return created, tx.Commit()
}

func (s *Store) UpdateTransaction(ctx context.Context, t domain.Transaction) (domain.Transaction, error) {
	err := s.updateOne(ctx,
		`UPDATE transactions SET type = ?, amount_cents = ?, date = ?, account_id = ?, category_id = ?, description = ? WHERE id = ?`,
		t.Type, t.AmountCents, t.Date, t.AccountID, t.CategoryID, t.Description, t.ID)
	if err != nil {
		return domain.Transaction{}, err
	}
	return s.transactionByID(ctx, t.ID)
}

func (s *Store) transactionByID(ctx context.Context, id int64) (domain.Transaction, error) {
	var t domain.Transaction
	err := s.db.QueryRowContext(ctx, `SELECT `+transactionColumns+` FROM transactions WHERE id = ?`, id).
		Scan(&t.ID, &t.Type, &t.AmountCents, &t.Date, &t.AccountID, &t.CategoryID, &t.Description, &t.InstallmentGroup, &t.SourceRef)
	return t, err
}

func (s *Store) DeleteTransaction(ctx context.Context, id int64) error {
	return s.deleteOne(ctx, `DELETE FROM transactions WHERE id = ?`, id)
}

func (s *Store) DeleteInstallmentGroup(ctx context.Context, group string) error {
	if group == "" {
		return ErrNotFound
	}
	return s.deleteOne(ctx, `DELETE FROM transactions WHERE installment_group = ?`, group)
}
