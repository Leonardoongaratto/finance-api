package store

import (
	"context"

	"github.com/Leonardoongaratto/finance-api/internal/domain"
)

const (
	investmentContributions = "investment_contributions"
	investmentValuations    = "investment_valuations"
	goalContributions       = "goal_contributions"
	debtPayments            = "debt_payments"
)

type child struct {
	ID         int64
	ParentID   int64
	Date       domain.Date
	ValueCents int64
}

func (s *Store) addChild(ctx context.Context, table string, c child) (child, error) {
	id, err := s.insert(ctx, `INSERT INTO `+table+` (parent_id, date, value_cents) VALUES (?, ?, ?)`, c.ParentID, c.Date, c.ValueCents)
	if err != nil {
		return child{}, err
	}
	c.ID = id
	return c, nil
}

func (s *Store) listChildren(ctx context.Context, table string) ([]child, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, parent_id, date, value_cents FROM `+table+` ORDER BY date, id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	children := []child{}
	for rows.Next() {
		var c child
		if err := rows.Scan(&c.ID, &c.ParentID, &c.Date, &c.ValueCents); err != nil {
			return nil, err
		}
		children = append(children, c)
	}
	return children, rows.Err()
}

func (s *Store) deleteChild(ctx context.Context, table string, parentID, id int64) error {
	return s.deleteOne(ctx, `DELETE FROM `+table+` WHERE id = ? AND parent_id = ?`, id, parentID)
}

func toContributions(children []child) []domain.Contribution {
	out := make([]domain.Contribution, len(children))
	for i, c := range children {
		out[i] = domain.Contribution{ID: c.ID, ParentID: c.ParentID, Date: c.Date, AmountCents: c.ValueCents}
	}
	return out
}

func (s *Store) AddInvestmentContribution(ctx context.Context, c domain.Contribution) (domain.Contribution, error) {
	created, err := s.addChild(ctx, investmentContributions, child{ParentID: c.ParentID, Date: c.Date, ValueCents: c.AmountCents})
	if err != nil {
		return domain.Contribution{}, err
	}
	c.ID = created.ID
	return c, nil
}

func (s *Store) ListInvestmentContributions(ctx context.Context) ([]domain.Contribution, error) {
	children, err := s.listChildren(ctx, investmentContributions)
	return toContributions(children), err
}

func (s *Store) DeleteInvestmentContribution(ctx context.Context, investmentID, id int64) error {
	return s.deleteChild(ctx, investmentContributions, investmentID, id)
}

func (s *Store) AddInvestmentValuation(ctx context.Context, v domain.Valuation) (domain.Valuation, error) {
	created, err := s.addChild(ctx, investmentValuations, child{ParentID: v.ParentID, Date: v.Date, ValueCents: v.ValueCents})
	if err != nil {
		return domain.Valuation{}, err
	}
	v.ID = created.ID
	return v, nil
}

func (s *Store) ListInvestmentValuations(ctx context.Context) ([]domain.Valuation, error) {
	children, err := s.listChildren(ctx, investmentValuations)
	out := make([]domain.Valuation, len(children))
	for i, c := range children {
		out[i] = domain.Valuation{ID: c.ID, ParentID: c.ParentID, Date: c.Date, ValueCents: c.ValueCents}
	}
	return out, err
}

func (s *Store) DeleteInvestmentValuation(ctx context.Context, investmentID, id int64) error {
	return s.deleteChild(ctx, investmentValuations, investmentID, id)
}

func (s *Store) AddGoalContribution(ctx context.Context, c domain.Contribution) (domain.Contribution, error) {
	created, err := s.addChild(ctx, goalContributions, child{ParentID: c.ParentID, Date: c.Date, ValueCents: c.AmountCents})
	if err != nil {
		return domain.Contribution{}, err
	}
	c.ID = created.ID
	return c, nil
}

func (s *Store) ListGoalContributions(ctx context.Context) ([]domain.Contribution, error) {
	children, err := s.listChildren(ctx, goalContributions)
	return toContributions(children), err
}

func (s *Store) DeleteGoalContribution(ctx context.Context, goalID, id int64) error {
	return s.deleteChild(ctx, goalContributions, goalID, id)
}

func (s *Store) AddDebtPayment(ctx context.Context, p domain.Payment) (domain.Payment, error) {
	created, err := s.addChild(ctx, debtPayments, child{ParentID: p.ParentID, Date: p.Date, ValueCents: p.AmountCents})
	if err != nil {
		return domain.Payment{}, err
	}
	p.ID = created.ID
	return p, nil
}

func (s *Store) ListDebtPayments(ctx context.Context) ([]domain.Payment, error) {
	children, err := s.listChildren(ctx, debtPayments)
	out := make([]domain.Payment, len(children))
	for i, c := range children {
		out[i] = domain.Payment{ID: c.ID, ParentID: c.ParentID, Date: c.Date, AmountCents: c.ValueCents}
	}
	return out, err
}

func (s *Store) DeleteDebtPayment(ctx context.Context, debtID, id int64) error {
	return s.deleteChild(ctx, debtPayments, debtID, id)
}
