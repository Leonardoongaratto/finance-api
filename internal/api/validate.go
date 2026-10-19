package api

import (
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/Leonardoongaratto/finance-api/internal/domain"
)

const (
	maxCents       = 10_000_000_000_000
	maxRateBP      = 10_000
	maxNameLength  = 120
	maxTextLength  = 200
	errRequiredFmt = " is required"
)

func trimmed(field string, value *string, maxLength int, required bool) error {
	*value = strings.TrimSpace(*value)
	if required && *value == "" {
		return errors.New(field + errRequiredFmt)
	}
	if utf8.RuneCountInString(*value) > maxLength {
		return errors.New(field + " is too long")
	}
	return nil
}

func positiveCents(field string, cents int64) error {
	if cents <= 0 || cents > maxCents {
		return errors.New(field + " must be greater than zero and within the supported range")
	}
	return nil
}

func requireDate(field string, date domain.Date) error {
	if date == "" {
		return errors.New(field + errRequiredFmt)
	}
	return nil
}

func requireID(field string, id int64) error {
	if id <= 0 {
		return errors.New(field + errRequiredFmt)
	}
	return nil
}

func firstError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func validateAccount(a *domain.Account) error {
	if !a.Type.Valid() {
		return errors.New("type must be checking, savings, cash or credit_card")
	}
	return trimmed("name", &a.Name, maxNameLength, true)
}

func validateCategory(c *domain.Category) error {
	return trimmed("name", &c.Name, maxNameLength, true)
}

func validateTransaction(t *domain.Transaction) error {
	if !t.Type.Valid() {
		return errors.New("type must be income or expense")
	}
	return firstError(
		positiveCents("amount_cents", t.AmountCents),
		requireDate("date", t.Date),
		requireID("account_id", t.AccountID),
		requireID("category_id", t.CategoryID),
		trimmed("description", &t.Description, maxTextLength, false),
	)
}

func validateNewTransaction(t *domain.Transaction) error {
	t.InstallmentGroup = ""
	t.SourceRef = ""
	return validateTransaction(t)
}

func validateInstallmentPlan(p *domain.InstallmentPlan) error {
	return firstError(
		positiveCents("total_cents", p.TotalCents),
		requireDate("first_date", p.FirstDate),
		requireID("account_id", p.AccountID),
		requireID("category_id", p.CategoryID),
		trimmed("description", &p.Description, maxTextLength-10, false),
	)
}

func validateInvestment(i *domain.Investment) error {
	if !i.Type.Valid() {
		return errors.New("type must be fixed_income, stocks, reits, treasury, crypto or other")
	}
	return trimmed("name", &i.Name, maxNameLength, true)
}

func validateContribution(c *domain.Contribution) error {
	return firstError(requireDate("date", c.Date), positiveCents("amount_cents", c.AmountCents))
}

func validateValuation(v *domain.Valuation) error {
	if v.ValueCents < 0 || v.ValueCents > maxCents {
		return errors.New("value_cents must not be negative and must be within the supported range")
	}
	return requireDate("date", v.Date)
}

func (s *Server) prepareNewGoal(g *domain.Goal) error {
	if g.StartDate == "" {
		g.StartDate = s.today()
	}
	return validateGoal(g)
}

func validateGoal(g *domain.Goal) error {
	if err := firstError(
		trimmed("name", &g.Name, maxNameLength, true),
		positiveCents("target_cents", g.TargetCents),
		requireDate("deadline", g.Deadline),
		requireDate("start_date", g.StartDate),
	); err != nil {
		return err
	}
	if g.Deadline < g.StartDate {
		return errors.New("deadline must not be before start_date")
	}
	return nil
}

func validateDebt(d *domain.Debt) error {
	if d.MonthlyRateBP < 0 || d.MonthlyRateBP > maxRateBP {
		return errors.New("monthly_rate_bp must be between 0 and 10000")
	}
	if d.MonthlyPaymentCents < 0 || d.MonthlyPaymentCents > maxCents {
		return errors.New("monthly_payment_cents must not be negative and must be within the supported range")
	}
	return firstError(
		trimmed("name", &d.Name, maxNameLength, true),
		trimmed("creditor", &d.Creditor, maxNameLength, false),
		positiveCents("original_cents", d.OriginalCents),
		requireDate("start_date", d.StartDate),
	)
}

func validatePayment(p *domain.Payment) error {
	return firstError(requireDate("date", p.Date), positiveCents("amount_cents", p.AmountCents))
}
