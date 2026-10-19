package store

import (
	"context"

	"github.com/Leonardoongaratto/finance-api/internal/domain"
)

func (s *Store) ListAccounts(ctx context.Context) ([]domain.Account, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, type FROM accounts ORDER BY name, id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	accounts := []domain.Account{}
	for rows.Next() {
		var a domain.Account
		if err := rows.Scan(&a.ID, &a.Name, &a.Type); err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
}

func (s *Store) CreateAccount(ctx context.Context, a domain.Account) (domain.Account, error) {
	id, err := s.insert(ctx, `INSERT INTO accounts (name, type) VALUES (?, ?)`, a.Name, a.Type)
	if err != nil {
		return domain.Account{}, err
	}
	a.ID = id
	return a, nil
}

func (s *Store) UpdateAccount(ctx context.Context, a domain.Account) (domain.Account, error) {
	err := s.updateOne(ctx, `UPDATE accounts SET name = ?, type = ? WHERE id = ?`, a.Name, a.Type, a.ID)
	return a, err
}

func (s *Store) DeleteAccount(ctx context.Context, id int64) error {
	return s.deleteOne(ctx, `DELETE FROM accounts WHERE id = ?`, id)
}

func (s *Store) ListCategories(ctx context.Context) ([]domain.Category, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name FROM categories ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	categories := []domain.Category{}
	for rows.Next() {
		var c domain.Category
		if err := rows.Scan(&c.ID, &c.Name); err != nil {
			return nil, err
		}
		categories = append(categories, c)
	}
	return categories, rows.Err()
}

func (s *Store) CreateCategory(ctx context.Context, c domain.Category) (domain.Category, error) {
	id, err := s.insert(ctx, `INSERT INTO categories (name) VALUES (?)`, c.Name)
	if err != nil {
		return domain.Category{}, err
	}
	c.ID = id
	return c, nil
}

func (s *Store) UpdateCategory(ctx context.Context, c domain.Category) (domain.Category, error) {
	err := s.updateOne(ctx, `UPDATE categories SET name = ? WHERE id = ?`, c.Name, c.ID)
	return c, err
}

func (s *Store) DeleteCategory(ctx context.Context, id int64) error {
	return s.deleteOne(ctx, `DELETE FROM categories WHERE id = ?`, id)
}
