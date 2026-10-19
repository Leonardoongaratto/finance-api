package domain

import (
	"math"
	"math/big"
	"testing"
)

func TestDebtBalanceHistoryAppliesInterestAndPayments(t *testing.T) {
	debt := Debt{OriginalCents: 10000, MonthlyRateBP: 100, StartDate: "2026-01-15"}
	payments := []Payment{
		{Date: "2026-01-20", AmountCents: 1000},
		{Date: "2026-02-05", AmountCents: 1000},
	}
	got := DebtBalanceHistory(debt, payments, "2026-03")
	want := []MonthBalance{{"2026-01", 9000}, {"2026-02", 8090}, {"2026-03", 8171}}
	assertBalances(t, got, want)
}

func TestDebtBalanceNeverNegative(t *testing.T) {
	debt := Debt{OriginalCents: 1000, MonthlyRateBP: 0, StartDate: "2026-01-01"}
	payments := []Payment{{Date: "2026-01-10", AmountCents: 5000}, {Date: "2026-02-10", AmountCents: 100}}
	got := DebtBalanceHistory(debt, payments, "2026-02")
	assertBalances(t, got, []MonthBalance{{"2026-01", 0}, {"2026-02", 0}})
}

func TestDebtBalanceHistoryBeforeStartIsEmpty(t *testing.T) {
	debt := Debt{OriginalCents: 1000, StartDate: "2026-05-01"}
	if got := DebtBalanceHistory(debt, nil, "2026-04"); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

func TestDebtBalanceAt(t *testing.T) {
	debt := Debt{OriginalCents: 10000, MonthlyRateBP: 100, StartDate: "2026-01-15"}
	if got := DebtBalanceAt(debt, nil, "2026-02"); got != 10100 {
		t.Errorf("with interest and no payments = %d", got)
	}
	if got := DebtBalanceAt(debt, nil, "2025-12"); got != 0 {
		t.Errorf("before start = %d", got)
	}
}

func TestProjectPayoffZeroInterest(t *testing.T) {
	p := ProjectPayoff(1000000, 0, 250000, "2026-10")
	if p.NeverPaysOff || p.PaidOff || p.Months != 4 || p.PayoffMonth != "2027-02" {
		t.Fatalf("projection = %+v", p)
	}
	if len(p.Points) != 4 || p.Points[3].BalanceCents != 0 || p.Points[0].BalanceCents != 750000 {
		t.Fatalf("points = %+v", p.Points)
	}
}

func TestProjectPayoffLastInstallmentIsSmaller(t *testing.T) {
	p := ProjectPayoff(1000000, 0, 300000, "2026-10")
	if p.Months != 4 || p.Points[2].BalanceCents != 100000 {
		t.Fatalf("projection = %+v", p)
	}
}

func TestProjectPayoffNeverWhenPaymentDoesNotCoverInterest(t *testing.T) {
	for _, payment := range []int64{20000, 19999, 0} {
		p := ProjectPayoff(1000000, 200, payment, "2026-10")
		if !p.NeverPaysOff || p.PayoffMonth != "" || p.Months != 0 {
			t.Errorf("payment %d: %+v", payment, p)
		}
	}
}

func TestProjectPayoffRespectsMonthLimit(t *testing.T) {
	p := ProjectPayoff(10000000, 200, 200001, "2026-10")
	if !p.NeverPaysOff {
		t.Fatalf("slow amortization must hit the %d month cap: %+v", MaxProjectionMonths, p)
	}
}

func TestProjectPayoffAlreadyPaid(t *testing.T) {
	p := ProjectPayoff(0, 100, 500, "2026-10")
	if !p.PaidOff || p.NeverPaysOff || p.Months != 0 {
		t.Fatalf("projection = %+v", p)
	}
}

func TestDebtBalanceWithoutPaymentsNeverShrinksOrOverflows(t *testing.T) {
	debt := Debt{OriginalCents: 100000, MonthlyRateBP: 10000, StartDate: "2023-01-01"}
	history := DebtBalanceHistory(debt, nil, "2026-10")
	previous := debt.OriginalCents
	for _, point := range history {
		if point.BalanceCents < previous {
			t.Fatalf("balance shrank without payment at %s: %d -> %d", point.Month, previous, point.BalanceCents)
		}
		previous = point.BalanceCents
	}
}

func TestDebtBalanceSaturatesAtTheCeiling(t *testing.T) {
	debt := Debt{OriginalCents: 100000, MonthlyRateBP: 10000, StartDate: "2023-01-01"}
	if got := DebtBalanceAt(debt, nil, "2026-10"); got != MaxBalanceCents {
		t.Fatalf("balance = %d, want ceiling %d", got, MaxBalanceCents)
	}
	if got := monthlyInterest(math.MaxInt64, 10000); got != MaxBalanceCents {
		t.Fatalf("interest on MaxInt64 = %d", got)
	}
	if got := monthlyInterest(MaxBalanceCents*9, 10000); got != MaxBalanceCents {
		t.Fatalf("interest above the ceiling = %d", got)
	}
	if got := accrue(math.MaxInt64, 1); got != MaxBalanceCents {
		t.Fatalf("accrue = %d", got)
	}
	if monthlyInterest(-1, 100) != 0 || monthlyInterest(100, -1) != 0 {
		t.Fatal("negative inputs accrue no interest")
	}
}

func TestProjectPayoffFromTheCeilingStaysWithinIt(t *testing.T) {
	p := ProjectPayoff(MaxBalanceCents, 100, MaxBalanceCents/10, "2026-10")
	if p.NeverPaysOff || p.PayoffMonth == "" {
		t.Fatalf("projection = %+v", p)
	}
	previous := MaxBalanceCents
	for _, point := range p.Points {
		if point.BalanceCents < 0 || point.BalanceCents >= previous {
			t.Fatalf("points = %+v", p.Points)
		}
		previous = point.BalanceCents
	}
}

func TestDebtOutlookBeforeStartUsesOriginalAndProjectsFromStart(t *testing.T) {
	debt := Debt{OriginalCents: 1000000, MonthlyPaymentCents: 250000, StartDate: "2027-03-01"}
	balance, p := DebtOutlook(debt, nil, "2026-10")
	if balance != 1000000 || p.PaidOff || p.NeverPaysOff || p.PayoffMonth != "2027-07" || p.Months != 9 {
		t.Fatalf("balance %d, projection %+v", balance, p)
	}
	if p.Points[0].Month != "2027-04" || p.Points[0].BalanceCents != 750000 {
		t.Fatalf("points = %+v", p.Points)
	}

	never := Debt{OriginalCents: 1000000, MonthlyRateBP: 500, MonthlyPaymentCents: 100, StartDate: "2027-03-01"}
	if balance, p := DebtOutlook(never, nil, "2026-10"); balance != 1000000 || !p.NeverPaysOff || p.Months != 0 {
		t.Fatalf("never pays off: balance %d, projection %+v", balance, p)
	}
}

func TestDebtOutlookBeforeStartDeductsPaymentsAlreadyMade(t *testing.T) {
	debt := Debt{OriginalCents: 5000000, MonthlyRateBP: 100, MonthlyPaymentCents: 100000, StartDate: "2027-03-01"}
	payments := []Payment{
		{Date: "2026-10-01", AmountCents: 1000000},
		{Date: "2027-01-10", AmountCents: 500000},
	}
	balance, p := DebtOutlook(debt, payments, "2026-10")
	if balance != 4000000 || p.PaidOff || p.NeverPaysOff {
		t.Fatalf("balance %d, projection %+v", balance, p)
	}
	if p.Points[0].Month != "2027-04" || p.Points[0].BalanceCents != 3940000 {
		t.Fatalf("points = %+v", p.Points[:1])
	}
	if atStart := DebtBalanceAt(debt, payments[:1], "2027-03"); atStart != balance {
		t.Fatalf("reference %d differs from the balance at start %d", balance, atStart)
	}

	covered := []Payment{{Date: "2026-09-15", AmountCents: 5000000}}
	if balance, p := DebtOutlook(debt, covered, "2026-10"); balance != 0 || !p.PaidOff || p.Months != 0 {
		t.Fatalf("covered before start: balance %d, projection %+v", balance, p)
	}
}

func TestDebtOutlookAfterStartUsesCurrentBalance(t *testing.T) {
	debt := Debt{OriginalCents: 10000, MonthlyRateBP: 100, MonthlyPaymentCents: 5000, StartDate: "2026-01-15"}
	balance, p := DebtOutlook(debt, []Payment{{Date: "2026-01-20", AmountCents: 1000}}, "2026-02")
	if balance != 9090 || p.Months != 2 || p.PayoffMonth != "2026-04" {
		t.Fatalf("balance %d, projection %+v", balance, p)
	}
}

func TestMonthlyInterestIsExactForLargeBalances(t *testing.T) {
	cases := []struct{ balance, rate int64 }{
		{900_000_000_000_000, 10000},
		{999_999_999_999_999, 9999},
		{123_456_789_012_345, 1234},
		{10001, 1},
	}
	for _, c := range cases {
		want := new(big.Int).Mul(big.NewInt(c.balance), big.NewInt(c.rate))
		want.Add(want, big.NewInt(basisPointsPerUnit/2)).Quo(want, big.NewInt(basisPointsPerUnit))
		if got := monthlyInterest(c.balance, c.rate); got != want.Int64() {
			t.Errorf("monthlyInterest(%d, %d) = %d, want %s", c.balance, c.rate, got, want)
		}
	}
}

func assertBalances(t *testing.T, got, want []MonthBalance) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
