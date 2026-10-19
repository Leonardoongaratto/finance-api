package domain

import "math/bits"

const (
	MaxProjectionMonths = 600
	basisPointsPerUnit  = 10000
	// MaxBalanceCents caps a debt balance (R$ 10 trilhões). Interest saturates here
	// instead of overflowing int64, so a balance never shrinks without a payment.
	MaxBalanceCents int64 = 1_000_000_000_000_000
)

type MonthBalance struct {
	Month        Month `json:"month"`
	BalanceCents int64 `json:"balance_cents"`
}

type PayoffProjection struct {
	PaidOff      bool           `json:"paid_off"`
	NeverPaysOff bool           `json:"never_pays_off"`
	PayoffMonth  Month          `json:"payoff_month,omitempty"`
	Months       int            `json:"payoff_months"`
	Points       []MonthBalance `json:"points"`
}

// monthlyInterest rounds balance*rateBP/10000 using a 128-bit product, saturating
// at MaxBalanceCents.
func monthlyInterest(balance, rateBP int64) int64 {
	if balance <= 0 || rateBP <= 0 {
		return 0
	}
	hi, lo := bits.Mul64(uint64(balance), uint64(rateBP))
	lo, carry := bits.Add64(lo, basisPointsPerUnit/2, 0)
	hi += carry
	if hi >= basisPointsPerUnit {
		return MaxBalanceCents
	}
	quotient, _ := bits.Div64(hi, lo, basisPointsPerUnit)
	if quotient > uint64(MaxBalanceCents) {
		return MaxBalanceCents
	}
	return int64(quotient)
}

func accrue(balance, rateBP int64) int64 {
	return min(min(balance, MaxBalanceCents)+monthlyInterest(balance, rateBP), MaxBalanceCents)
}

// DebtBalanceHistory returns the balance at the end of every month from the
// debt start through the given month: previous balance plus interest minus the
// month's payments, never below zero. The start month accrues no interest.
func DebtBalanceHistory(debt Debt, payments []Payment, through Month) []MonthBalance {
	start := debt.StartDate.Month()
	paidByMonth := make(map[Month]int64)
	for _, p := range payments {
		paidByMonth[max(p.Date.Month(), start)] += p.AmountCents
	}

	var history []MonthBalance
	balance := min(debt.OriginalCents, MaxBalanceCents)
	for m := start; m <= through; m = m.Add(1) {
		if m != start {
			balance = accrue(balance, debt.MonthlyRateBP)
		}
		balance = max(balance-paidByMonth[m], 0)
		history = append(history, MonthBalance{Month: m, BalanceCents: balance})
	}
	return history
}

// DebtBalanceAt is the balance owed at the end of the month, or 0 before the
// debt starts: a debt that has not started yet is not part of the net worth.
func DebtBalanceAt(debt Debt, payments []Payment, month Month) int64 {
	history := DebtBalanceHistory(debt, payments, month)
	if len(history) == 0 {
		return 0
	}
	return history[len(history)-1].BalanceCents
}

// DebtOutlook is the reference balance and payoff projection as seen from
// `month`. Before the debt starts the reference is the balance it will have in
// its start month counting only the payments made until `month`, the same
// figure DebtBalanceHistory produces there; the projection runs from the start
// month (first payment in the month after it), with Months counted from `month`.
func DebtOutlook(debt Debt, payments []Payment, month Month) (int64, PayoffProjection) {
	start := debt.StartDate.Month()
	if !debt.StartsAfter(month) {
		balance := DebtBalanceAt(debt, payments, month)
		return balance, ProjectPayoff(balance, debt.MonthlyRateBP, debt.MonthlyPaymentCents, month)
	}
	balance := DebtBalanceAt(debt, paymentsUntil(payments, month.EndDate()), start)
	projection := ProjectPayoff(balance, debt.MonthlyRateBP, debt.MonthlyPaymentCents, start)
	if projection.PayoffMonth != "" {
		projection.Months = MonthsBetween(month, projection.PayoffMonth)
	}
	return balance, projection
}

func (d Debt) StartsAfter(month Month) bool {
	return month < d.StartDate.Month()
}

func paymentsUntil(payments []Payment, end Date) []Payment {
	var made []Payment
	for _, p := range payments {
		if p.Date <= end {
			made = append(made, p)
		}
	}
	return made
}

// ProjectPayoff simulates monthly interest and the fixed payment from the month
// after `from` until the balance reaches zero, within MaxProjectionMonths.
func ProjectPayoff(balance, rateBP, monthlyPayment int64, from Month) PayoffProjection {
	if balance <= 0 {
		return PayoffProjection{PaidOff: true}
	}
	if monthlyPayment <= monthlyInterest(balance, rateBP) {
		return PayoffProjection{NeverPaysOff: true}
	}

	var points []MonthBalance
	for k := 1; k <= MaxProjectionMonths; k++ {
		balance = max(accrue(balance, rateBP)-monthlyPayment, 0)
		month := from.Add(k)
		points = append(points, MonthBalance{Month: month, BalanceCents: balance})
		if balance == 0 {
			return PayoffProjection{PayoffMonth: month, Months: k, Points: points}
		}
	}
	return PayoffProjection{NeverPaysOff: true}
}
