package report

import (
	"errors"
	"testing"

	"github.com/Leonardoongaratto/finance-api/internal/domain"
)

const today = domain.Date("2026-10-02")

func sampleData() Data {
	return Data{
		Categories: []domain.Category{{ID: 2, Name: "Alimentação"}, {ID: 3, Name: "Transporte"}, {ID: 8, Name: "Salário"}},
		Transactions: []domain.Transaction{
			{Type: domain.Income, AmountCents: 500000, Date: "2026-09-05", AccountID: 1, CategoryID: 8},
			{Type: domain.Expense, AmountCents: 20000, Date: "2026-09-10", AccountID: 1, CategoryID: 2},
			{Type: domain.Expense, AmountCents: 5000, Date: "2026-09-12", AccountID: 1, CategoryID: 3},
			{Type: domain.Expense, AmountCents: 10000, Date: "2026-10-01", AccountID: 1, CategoryID: 2},
		},
		Investments:   []domain.Investment{{ID: 1, Name: "CDB", Type: domain.InvestmentFixedIncome}},
		Contributions: []domain.Contribution{{ID: 1, ParentID: 1, Date: "2026-09-15", AmountCents: 100000}},
		Valuations:    []domain.Valuation{{ID: 1, ParentID: 1, Date: "2026-10-01", ValueCents: 120000}},
		Goals:         []domain.Goal{{ID: 1, Name: "Viagem", TargetCents: 100000, Deadline: "2026-12-31", StartDate: "2026-09-01"}},
		GoalContributions: []domain.Contribution{
			{ID: 1, ParentID: 1, Date: "2026-09-15", AmountCents: 30000},
		},
		Debts:    []domain.Debt{{ID: 1, Name: "Emprestimo", OriginalCents: 100000, MonthlyPaymentCents: 25000, StartDate: "2026-09-01"}},
		Payments: []domain.Payment{{ID: 1, ParentID: 1, Date: "2026-09-20", AmountCents: 25000}},
	}
}

func build(t *testing.T, from, to domain.Month) Dashboard {
	t.Helper()
	d, err := Build(sampleData(), Query{From: from, To: to, Today: today})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestCashflowPerMonth(t *testing.T) {
	d := build(t, "2026-09", "2026-10")
	want := []CashflowPoint{
		{Month: "2026-09", IncomeCents: 500000, ExpenseCents: 25000, BalanceCents: 475000},
		{Month: "2026-10", IncomeCents: 0, ExpenseCents: 10000, BalanceCents: -10000},
	}
	if len(d.Cashflow) != 2 || d.Cashflow[0] != want[0] || d.Cashflow[1] != want[1] {
		t.Fatalf("cashflow = %+v", d.Cashflow)
	}
}

func TestSpendingByCategoryGroupsByMonthAndCategory(t *testing.T) {
	d := build(t, "2026-09", "2026-10")
	want := []SpendingPoint{
		{Month: "2026-09", CategoryID: 2, Category: "Alimentação", AmountCents: 20000},
		{Month: "2026-09", CategoryID: 3, Category: "Transporte", AmountCents: 5000},
		{Month: "2026-10", CategoryID: 2, Category: "Alimentação", AmountCents: 10000},
	}
	if len(d.SpendingByCategory) != len(want) {
		t.Fatalf("spending = %+v", d.SpendingByCategory)
	}
	for i := range want {
		if d.SpendingByCategory[i] != want[i] {
			t.Errorf("spending[%d] = %+v, want %+v", i, d.SpendingByCategory[i], want[i])
		}
	}
}

func TestNetWorthIsAccountsPlusInvestmentsMinusDebts(t *testing.T) {
	d := build(t, "2026-09", "2026-10")
	sept, oct := d.NetWorth[0], d.NetWorth[1]
	if sept.AccountsCents != 475000 || sept.InvestmentsCents != 100000 || sept.DebtsCents != 75000 || sept.NetWorthCents != 500000 {
		t.Errorf("september = %+v", sept)
	}
	if oct.AccountsCents != 465000 || oct.InvestmentsCents != 120000 || oct.DebtsCents != 75000 || oct.NetWorthCents != 510000 {
		t.Errorf("october = %+v", oct)
	}
}

func TestRangeLimitsSeriesToRequestedMonths(t *testing.T) {
	d := build(t, "2026-10", "2026-10")
	if len(d.NetWorth) != 1 || len(d.Cashflow) != 1 || len(d.Investments) != 1 || d.Cashflow[0].Month != "2026-10" {
		t.Fatalf("series not limited: %+v", d)
	}
	if len(d.SpendingByCategory) != 1 || d.SpendingByCategory[0].Month != "2026-10" {
		t.Fatalf("spending not limited: %+v", d.SpendingByCategory)
	}
	if d.Goals[0].Series[0].Month != "2026-10" || d.Debts[0].History[0].Month != "2026-10" {
		t.Fatalf("goal/debt series not limited")
	}
}

func TestDefaultRangeStartsAtEarliestEventAndEndsToday(t *testing.T) {
	d, err := Build(sampleData(), Query{Today: today})
	if err != nil {
		t.Fatal(err)
	}
	if d.From != "2026-09" || d.To != "2026-10" || len(d.Cashflow) != 2 {
		t.Fatalf("range = %s..%s (%d)", d.From, d.To, len(d.Cashflow))
	}
}

func TestEmptyDataStillReturnsCurrentMonthWithEmptySlices(t *testing.T) {
	d, err := Build(Data{}, Query{Today: today})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.NetWorth) != 1 || d.NetWorth[0].NetWorthCents != 0 {
		t.Fatalf("net worth = %+v", d.NetWorth)
	}
	if d.SpendingByCategory == nil || d.Debts == nil || d.Goals == nil || d.Investments == nil {
		t.Fatal("slices must be non-nil for JSON")
	}
}

func TestInvalidRange(t *testing.T) {
	if _, err := Build(sampleData(), Query{From: "2026-10", To: "2026-09", Today: today}); !errors.Is(err, ErrInvalidRange) {
		t.Errorf("reversed range: %v", err)
	}
	if _, err := Build(sampleData(), Query{From: "1900-01", To: "2026-09", Today: today}); !errors.Is(err, ErrInvalidRange) {
		t.Errorf("too long range: %v", err)
	}
}

func TestKPIsUseLastMonthOfRange(t *testing.T) {
	k := build(t, "2026-09", "2026-10").KPIs
	if k.NetWorthCents != 510000 || k.MonthExpensesCents != 10000 || k.InvestedCents != 100000 || k.InvestmentsValueCents != 120000 || k.TotalDebtCents != 75000 {
		t.Fatalf("kpis = %+v", k)
	}
}

func TestInvestmentSeriesAggregatesInvestedAndCurrent(t *testing.T) {
	d := build(t, "2026-09", "2026-10")
	if d.Investments[0].InvestedCents != 100000 || d.Investments[0].CurrentCents != 100000 {
		t.Errorf("sept = %+v", d.Investments[0])
	}
	if d.Investments[1].InvestedCents != 100000 || d.Investments[1].CurrentCents != 120000 {
		t.Errorf("oct = %+v", d.Investments[1])
	}
}

func TestDebtSeriesHasHistoryAndProjection(t *testing.T) {
	debt := build(t, "2026-09", "2026-10").Debts[0]
	if len(debt.History) != 2 || debt.History[0].BalanceCents != 75000 || debt.History[1].BalanceCents != 75000 {
		t.Fatalf("history = %+v", debt.History)
	}
	if debt.BalanceCents != 75000 || debt.NeverPaysOff || debt.PayoffMonth != "2027-01" || len(debt.Projection) != 3 {
		t.Fatalf("debt = %+v", debt)
	}
}

func TestGoalSeriesTracksSavedAndIdealPace(t *testing.T) {
	goal := build(t, "2026-09", "2026-10").Goals[0]
	if goal.TargetCents != 100000 || len(goal.Series) != 2 {
		t.Fatalf("goal = %+v", goal)
	}
	sept := goal.Series[0]
	if sept.SavedCents != 30000 || sept.IdealCents <= 0 || sept.IdealCents >= 100000 {
		t.Errorf("sept = %+v", sept)
	}
	if goal.Series[1].IdealCents <= sept.IdealCents {
		t.Errorf("ideal pace must grow: %+v", goal.Series)
	}
}

func TestProjectionIsCappedForDisplay(t *testing.T) {
	data := sampleData()
	data.Debts = []domain.Debt{{ID: 1, Name: "Longa", OriginalCents: 10000000, MonthlyPaymentCents: 20000, StartDate: "2026-10-01"}}
	data.Payments = nil
	d, err := Build(data, Query{Today: today})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(d.Debts[0].Projection); got != MaxProjectionPoints {
		t.Fatalf("projection points = %d, want %d", got, MaxProjectionPoints)
	}
}

func TestFutureDebtProjectsFromStartAndIsNotInNetWorthBeforeIt(t *testing.T) {
	data := sampleData()
	data.Debts = append(data.Debts, domain.Debt{ID: 2, Name: "Financiamento", OriginalCents: 1000000, MonthlyPaymentCents: 250000, StartDate: "2027-03-01"})
	d, err := Build(data, Query{From: "2026-09", To: "2026-10", Today: today})
	if err != nil {
		t.Fatal(err)
	}
	if d.Debts[0].NotStarted || !d.Debts[1].NotStarted {
		t.Fatalf("not_started flags = %v, %v", d.Debts[0].NotStarted, d.Debts[1].NotStarted)
	}
	future := d.Debts[1]
	if future.BalanceCents != 1000000 || future.NeverPaysOff || future.PayoffMonth != "2027-07" || len(future.History) != 0 {
		t.Fatalf("future debt = %+v", future)
	}
	if len(future.Projection) != 4 || future.Projection[0].Month != "2027-04" || future.Projection[0].BalanceCents != 750000 {
		t.Fatalf("projection = %+v", future.Projection)
	}
	if d.NetWorth[1].DebtsCents != 75000 || d.KPIs.TotalDebtCents != 75000 {
		t.Fatalf("debt not started yet must stay out of net worth: %+v", d.NetWorth[1])
	}
}

func TestRangeEndingMoreThanAMonthAheadIsInvalid(t *testing.T) {
	for _, q := range []Query{{From: "2026-09", To: "2026-12", Today: today}, {To: "2027-05", Today: today}, {From: "2026-11", To: "2026-11", Today: today}} {
		if _, err := Build(sampleData(), q); !errors.Is(err, ErrInvalidRange) {
			t.Errorf("%+v: %v", q, err)
		}
	}
}

func TestRangeEndingNextMonthIsClampedToTheCurrentMonth(t *testing.T) {
	d, err := Build(sampleData(), Query{From: "2026-09", To: "2026-11", Today: today})
	if err != nil {
		t.Fatal(err)
	}
	if d.To != "2026-10" || len(d.NetWorth) != 2 || d.NetWorth[1].Month != "2026-10" {
		t.Fatalf("range = %s..%s (%d)", d.From, d.To, len(d.NetWorth))
	}
}

func TestFutureDebtWithPaymentsUsesTheSameReferenceInSeriesAndNetWorth(t *testing.T) {
	data := sampleData()
	data.Debts = []domain.Debt{{ID: 2, Name: "Financiamento", OriginalCents: 5000000, MonthlyRateBP: 100, MonthlyPaymentCents: 100000, StartDate: "2026-10-01"}}
	data.Payments = []domain.Payment{{ID: 1, ParentID: 2, Date: "2026-09-10", AmountCents: 1000000}}
	d, err := Build(data, Query{From: "2026-09", To: "2026-09", Today: "2026-09-20"})
	if err != nil {
		t.Fatal(err)
	}
	if d.Debts[0].BalanceCents != 4000000 || d.NetWorth[0].DebtsCents != 0 || d.Debts[0].Projection[0].BalanceCents != 3940000 {
		t.Fatalf("series = %+v, net worth = %+v", d.Debts[0], d.NetWorth)
	}
}

func TestDefaultRangeIsClampedToTheMaximumWhenDataIsVeryOld(t *testing.T) {
	data := sampleData()
	data.Transactions = append(data.Transactions, domain.Transaction{Type: domain.Expense, AmountCents: 100, Date: "1950-01-01", AccountID: 1, CategoryID: 2})
	d, err := Build(data, Query{Today: today})
	if err != nil {
		t.Fatalf("default range with very old data: %v", err)
	}
	if d.To != "2026-10" || len(d.Cashflow) != MaxRangeMonths || d.From != domain.Month("2026-10").Add(-(MaxRangeMonths-1)) {
		t.Fatalf("range = %s..%s (%d)", d.From, d.To, len(d.Cashflow))
	}
}

func TestNeverPaysOffDebtHasNoProjection(t *testing.T) {
	data := sampleData()
	data.Debts = []domain.Debt{{ID: 1, Name: "Rotativo", OriginalCents: 100000, MonthlyRateBP: 1000, MonthlyPaymentCents: 5000, StartDate: "2026-09-01"}}
	data.Payments = nil
	d, _ := Build(data, Query{Today: today})
	if !d.Debts[0].NeverPaysOff || len(d.Debts[0].Projection) != 0 {
		t.Fatalf("debt = %+v", d.Debts[0])
	}
}
