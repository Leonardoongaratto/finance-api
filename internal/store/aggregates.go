package store

import (
	"context"

	"github.com/Leonardoongaratto/finance-api/internal/domain"
)

func (s *Store) ListInvestments(ctx context.Context) ([]domain.Investment, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, type FROM investments ORDER BY name, id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	investments := []domain.Investment{}
	for rows.Next() {
		var i domain.Investment
		if err := rows.Scan(&i.ID, &i.Name, &i.Type); err != nil {
			return nil, err
		}
		investments = append(investments, i)
	}
	return investments, rows.Err()
}

func (s *Store) CreateInvestment(ctx context.Context, i domain.Investment) (domain.Investment, error) {
	id, err := s.insert(ctx, `INSERT INTO investments (name, type) VALUES (?, ?)`, i.Name, i.Type)
	if err != nil {
		return domain.Investment{}, err
	}
	i.ID = id
	return i, nil
}

func (s *Store) UpdateInvestment(ctx context.Context, i domain.Investment) (domain.Investment, error) {
	err := s.updateOne(ctx, `UPDATE investments SET name = ?, type = ? WHERE id = ?`, i.Name, i.Type, i.ID)
	return i, err
}

func (s *Store) DeleteInvestment(ctx context.Context, id int64) error {
	return s.deleteOne(ctx, `DELETE FROM investments WHERE id = ?`, id)
}

func (s *Store) ListGoals(ctx context.Context) ([]domain.Goal, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, target_cents, deadline, start_date FROM goals ORDER BY deadline, id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	goals := []domain.Goal{}
	for rows.Next() {
		var g domain.Goal
		if err := rows.Scan(&g.ID, &g.Name, &g.TargetCents, &g.Deadline, &g.StartDate); err != nil {
			return nil, err
		}
		goals = append(goals, g)
	}
	return goals, rows.Err()
}

func (s *Store) CreateGoal(ctx context.Context, g domain.Goal) (domain.Goal, error) {
	id, err := s.insert(ctx, `INSERT INTO goals (name, target_cents, deadline, start_date) VALUES (?, ?, ?, ?)`,
		g.Name, g.TargetCents, g.Deadline, g.StartDate)
	if err != nil {
		return domain.Goal{}, err
	}
	g.ID = id
	return g, nil
}

func (s *Store) UpdateGoal(ctx context.Context, g domain.Goal) (domain.Goal, error) {
	err := s.updateOne(ctx, `UPDATE goals SET name = ?, target_cents = ?, deadline = ?, start_date = ? WHERE id = ?`,
		g.Name, g.TargetCents, g.Deadline, g.StartDate, g.ID)
	return g, err
}

func (s *Store) DeleteGoal(ctx context.Context, id int64) error {
	return s.deleteOne(ctx, `DELETE FROM goals WHERE id = ?`, id)
}

func (s *Store) ListDebts(ctx context.Context) ([]domain.Debt, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, creditor, original_cents, monthly_rate_bp, monthly_payment_cents, start_date FROM debts ORDER BY start_date, id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	debts := []domain.Debt{}
	for rows.Next() {
		var d domain.Debt
		if err := rows.Scan(&d.ID, &d.Name, &d.Creditor, &d.OriginalCents, &d.MonthlyRateBP, &d.MonthlyPaymentCents, &d.StartDate); err != nil {
			return nil, err
		}
		debts = append(debts, d)
	}
	return debts, rows.Err()
}

func (s *Store) CreateDebt(ctx context.Context, d domain.Debt) (domain.Debt, error) {
	id, err := s.insert(ctx,
		`INSERT INTO debts (name, creditor, original_cents, monthly_rate_bp, monthly_payment_cents, start_date) VALUES (?, ?, ?, ?, ?, ?)`,
		d.Name, d.Creditor, d.OriginalCents, d.MonthlyRateBP, d.MonthlyPaymentCents, d.StartDate)
	if err != nil {
		return domain.Debt{}, err
	}
	d.ID = id
	return d, nil
}

func (s *Store) UpdateDebt(ctx context.Context, d domain.Debt) (domain.Debt, error) {
	err := s.updateOne(ctx,
		`UPDATE debts SET name = ?, creditor = ?, original_cents = ?, monthly_rate_bp = ?, monthly_payment_cents = ?, start_date = ? WHERE id = ?`,
		d.Name, d.Creditor, d.OriginalCents, d.MonthlyRateBP, d.MonthlyPaymentCents, d.StartDate, d.ID)
	return d, err
}

func (s *Store) DeleteDebt(ctx context.Context, id int64) error {
	return s.deleteOne(ctx, `DELETE FROM debts WHERE id = ?`, id)
}
