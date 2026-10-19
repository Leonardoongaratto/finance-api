package domain

import (
	"errors"
	"fmt"
	"strings"
)

const (
	MinInstallments = 2
	MaxInstallments = 48
)

var ErrInvalidInstallments = errors.New("installments must be between 2 and 48 and total must cover at least one cent each")

type InstallmentPlan struct {
	TotalCents   int64  `json:"total_cents"`
	Installments int    `json:"installments"`
	FirstDate    Date   `json:"first_date"`
	AccountID    int64  `json:"account_id"`
	CategoryID   int64  `json:"category_id"`
	Description  string `json:"description"`
}

func SplitInstallments(total int64, n int) ([]int64, error) {
	if n < MinInstallments || n > MaxInstallments || total < int64(n) {
		return nil, ErrInvalidInstallments
	}
	base, remainder := total/int64(n), total%int64(n)
	parts := make([]int64, n)
	for i := range parts {
		parts[i] = base
	}
	parts[0] += remainder
	return parts, nil
}

func BuildInstallments(plan InstallmentPlan, group string) ([]Transaction, error) {
	amounts, err := SplitInstallments(plan.TotalCents, plan.Installments)
	if err != nil {
		return nil, err
	}
	txs := make([]Transaction, len(amounts))
	for i, amount := range amounts {
		label := fmt.Sprintf("%d/%d", i+1, len(amounts))
		txs[i] = Transaction{
			Type:             Expense,
			AmountCents:      amount,
			Date:             plan.FirstDate.AddMonths(i),
			AccountID:        plan.AccountID,
			CategoryID:       plan.CategoryID,
			Description:      strings.TrimSpace(plan.Description + " " + label),
			InstallmentGroup: group,
		}
	}
	return txs, nil
}
