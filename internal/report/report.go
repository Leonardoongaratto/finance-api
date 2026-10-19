// Package report derives the dashboard series from the recorded events.
// Every monthly figure is computed as of the last day of that month, so editing
// an old entry is always reflected and no snapshot can go stale.
package report

import (
	"errors"
	"sort"

	"github.com/Leonardoongaratto/finance-api/internal/domain"
)

const (
	MaxRangeMonths      = 600
	MaxProjectionPoints = 120
)

var ErrInvalidRange = errors.New("invalid period")

type Data struct {
	Categories        []domain.Category
	Transactions      []domain.Transaction
	Investments       []domain.Investment
	Contributions     []domain.Contribution
	Valuations        []domain.Valuation
	Goals             []domain.Goal
	GoalContributions []domain.Contribution
	Debts             []domain.Debt
	Payments          []domain.Payment
}

type Query struct {
	From  domain.Month
	To    domain.Month
	Today domain.Date
}

type Dashboard struct {
	From               domain.Month      `json:"from"`
	To                 domain.Month      `json:"to"`
	KPIs               KPIs              `json:"kpis"`
	NetWorth           []NetWorthPoint   `json:"net_worth"`
	Cashflow           []CashflowPoint   `json:"cashflow"`
	SpendingByCategory []SpendingPoint   `json:"spending_by_category"`
	Debts              []DebtSeries      `json:"debts"`
	Investments        []InvestmentPoint `json:"investments"`
	Goals              []GoalSeries      `json:"goals"`
}

type KPIs struct {
	NetWorthCents         int64 `json:"net_worth_cents"`
	MonthExpensesCents    int64 `json:"month_expenses_cents"`
	InvestedCents         int64 `json:"invested_cents"`
	InvestmentsValueCents int64 `json:"investments_value_cents"`
	TotalDebtCents        int64 `json:"total_debt_cents"`
}

type NetWorthPoint struct {
	Month            domain.Month `json:"month"`
	AccountsCents    int64        `json:"accounts_cents"`
	InvestmentsCents int64        `json:"investments_cents"`
	DebtsCents       int64        `json:"debts_cents"`
	NetWorthCents    int64        `json:"net_worth_cents"`
}

type CashflowPoint struct {
	Month        domain.Month `json:"month"`
	IncomeCents  int64        `json:"income_cents"`
	ExpenseCents int64        `json:"expense_cents"`
	BalanceCents int64        `json:"balance_cents"`
}

type SpendingPoint struct {
	Month       domain.Month `json:"month"`
	CategoryID  int64        `json:"category_id"`
	Category    string       `json:"category"`
	AmountCents int64        `json:"amount_cents"`
}

type InvestmentPoint struct {
	Month         domain.Month `json:"month"`
	InvestedCents int64        `json:"invested_cents"`
	CurrentCents  int64        `json:"current_cents"`
}

type DebtSeries struct {
	ID           int64                 `json:"id"`
	Name         string                `json:"name"`
	BalanceCents int64                 `json:"balance_cents"`
	NeverPaysOff bool                  `json:"never_pays_off"`
	NotStarted   bool                  `json:"not_started"`
	PayoffMonth  domain.Month          `json:"payoff_month,omitempty"`
	History      []domain.MonthBalance `json:"history"`
	Projection   []domain.MonthBalance `json:"projection"`
}

type GoalPoint struct {
	Month      domain.Month `json:"month"`
	SavedCents int64        `json:"saved_cents"`
	IdealCents int64        `json:"ideal_cents"`
}

type GoalSeries struct {
	ID          int64       `json:"id"`
	Name        string      `json:"name"`
	TargetCents int64       `json:"target_cents"`
	Series      []GoalPoint `json:"series"`
}

func Build(data Data, q Query) (Dashboard, error) {
	from, to, err := resolveRange(data, q)
	if err != nil {
		return Dashboard{}, err
	}
	months := domain.MonthRange(from, to)

	dashboard := Dashboard{
		From:               from,
		To:                 to,
		NetWorth:           make([]NetWorthPoint, 0, len(months)),
		Cashflow:           cashflow(data.Transactions, months),
		SpendingByCategory: spending(data, months),
		Investments:        make([]InvestmentPoint, 0, len(months)),
	}
	for _, m := range months {
		net, invested := netWorthAt(data, m)
		dashboard.NetWorth = append(dashboard.NetWorth, net)
		dashboard.Investments = append(dashboard.Investments, invested)
	}
	dashboard.Debts = debtSeries(data, months, q.Today.Month())
	dashboard.Goals = goalSeries(data, months)
	dashboard.KPIs = kpis(dashboard)
	return dashboard, nil
}

// resolveRange accepts `to` one month past the server's current month and
// clamps it, because a browser in another time zone may already be there.
func resolveRange(data Data, q Query) (domain.Month, domain.Month, error) {
	current := q.Today.Month()
	to := q.To
	if to == "" || to == current.Add(1) {
		to = current
	}
	from := q.From
	if from == "" {
		from = max(earliestMonth(data, to), to.Add(-(MaxRangeMonths - 1)))
	}
	if from > to || to > current || domain.MonthsBetween(from, to) >= MaxRangeMonths {
		return "", "", ErrInvalidRange
	}
	return from, to, nil
}

func earliestMonth(data Data, fallback domain.Month) domain.Month {
	earliest := fallback
	consider := func(d domain.Date) { earliest = min(earliest, d.Month()) }
	for _, t := range data.Transactions {
		consider(t.Date)
	}
	for _, c := range data.Contributions {
		consider(c.Date)
	}
	for _, v := range data.Valuations {
		consider(v.Date)
	}
	for _, g := range data.Goals {
		consider(g.StartDate)
	}
	for _, c := range data.GoalContributions {
		consider(c.Date)
	}
	for _, d := range data.Debts {
		consider(d.StartDate)
	}
	for _, p := range data.Payments {
		consider(p.Date)
	}
	return earliest
}

func cashflow(transactions []domain.Transaction, months []domain.Month) []CashflowPoint {
	byMonth := make(map[domain.Month]*CashflowPoint, len(months))
	points := make([]CashflowPoint, len(months))
	for i, m := range months {
		points[i].Month = m
		byMonth[m] = &points[i]
	}
	for _, t := range transactions {
		point, ok := byMonth[t.Date.Month()]
		if !ok {
			continue
		}
		if t.Type == domain.Income {
			point.IncomeCents += t.AmountCents
		} else {
			point.ExpenseCents += t.AmountCents
		}
		point.BalanceCents = point.IncomeCents - point.ExpenseCents
	}
	return points
}

func spending(data Data, months []domain.Month) []SpendingPoint {
	names := make(map[int64]string, len(data.Categories))
	for _, c := range data.Categories {
		names[c.ID] = c.Name
	}
	inRange := make(map[domain.Month]bool, len(months))
	for _, m := range months {
		inRange[m] = true
	}

	type key struct {
		month    domain.Month
		category int64
	}
	totals := make(map[key]int64)
	for _, t := range data.Transactions {
		if t.Type == domain.Expense && inRange[t.Date.Month()] {
			totals[key{t.Date.Month(), t.CategoryID}] += t.AmountCents
		}
	}

	points := make([]SpendingPoint, 0, len(totals))
	for k, amount := range totals {
		points = append(points, SpendingPoint{Month: k.month, CategoryID: k.category, Category: names[k.category], AmountCents: amount})
	}
	sort.Slice(points, func(i, j int) bool {
		if points[i].Month != points[j].Month {
			return points[i].Month < points[j].Month
		}
		return points[i].CategoryID < points[j].CategoryID
	})
	return points
}

func netWorthAt(data Data, month domain.Month) (NetWorthPoint, InvestmentPoint) {
	end := month.EndDate()

	var accounts int64
	for _, t := range data.Transactions {
		if t.Date <= end {
			accounts += t.SignedCents()
		}
	}

	invested := InvestmentPoint{Month: month}
	for _, inv := range data.Investments {
		summary := domain.SummarizeInvestment(
			filterContributions(data.Contributions, inv.ID),
			filterValuations(data.Valuations, inv.ID),
			end)
		invested.InvestedCents += summary.InvestedCents
		invested.CurrentCents += summary.CurrentCents
	}

	var debts int64
	for _, d := range data.Debts {
		debts += domain.DebtBalanceAt(d, filterPayments(data.Payments, d.ID), month)
	}

	return NetWorthPoint{
		Month:            month,
		AccountsCents:    accounts,
		InvestmentsCents: invested.CurrentCents,
		DebtsCents:       debts,
		NetWorthCents:    accounts + invested.CurrentCents - debts,
	}, invested
}

func debtSeries(data Data, months []domain.Month, currentMonth domain.Month) []DebtSeries {
	series := make([]DebtSeries, 0, len(data.Debts))
	last := months[len(months)-1]
	for _, d := range data.Debts {
		payments := filterPayments(data.Payments, d.ID)
		history := []domain.MonthBalance{}
		for _, point := range domain.DebtBalanceHistory(d, payments, last) {
			if point.Month >= months[0] {
				history = append(history, point)
			}
		}

		balance, projection := domain.DebtOutlook(d, payments, currentMonth)
		points := projection.Points[:min(len(projection.Points), MaxProjectionPoints)]
		if points == nil {
			points = []domain.MonthBalance{}
		}
		series = append(series, DebtSeries{
			ID: d.ID, Name: d.Name, BalanceCents: balance,
			NeverPaysOff: projection.NeverPaysOff, NotStarted: d.StartsAfter(currentMonth), PayoffMonth: projection.PayoffMonth,
			History: history, Projection: points,
		})
	}
	return series
}

func goalSeries(data Data, months []domain.Month) []GoalSeries {
	series := make([]GoalSeries, 0, len(data.Goals))
	for _, g := range data.Goals {
		contributions := filterContributions(data.GoalContributions, g.ID)
		points := make([]GoalPoint, len(months))
		for i, m := range months {
			end := m.EndDate()
			var saved int64
			for _, c := range contributions {
				if c.Date <= end {
					saved += c.AmountCents
				}
			}
			points[i] = GoalPoint{Month: m, SavedCents: saved, IdealCents: domain.IdealPace(g, end)}
		}
		series = append(series, GoalSeries{ID: g.ID, Name: g.Name, TargetCents: g.TargetCents, Series: points})
	}
	return series
}

func kpis(d Dashboard) KPIs {
	last := len(d.NetWorth) - 1
	return KPIs{
		NetWorthCents:         d.NetWorth[last].NetWorthCents,
		MonthExpensesCents:    d.Cashflow[last].ExpenseCents,
		InvestedCents:         d.Investments[last].InvestedCents,
		InvestmentsValueCents: d.Investments[last].CurrentCents,
		TotalDebtCents:        d.NetWorth[last].DebtsCents,
	}
}

func filterContributions(all []domain.Contribution, parent int64) []domain.Contribution {
	var out []domain.Contribution
	for _, c := range all {
		if c.ParentID == parent {
			out = append(out, c)
		}
	}
	return out
}

func filterValuations(all []domain.Valuation, parent int64) []domain.Valuation {
	var out []domain.Valuation
	for _, v := range all {
		if v.ParentID == parent {
			out = append(out, v)
		}
	}
	return out
}

func filterPayments(all []domain.Payment, parent int64) []domain.Payment {
	var out []domain.Payment
	for _, p := range all {
		if p.ParentID == parent {
			out = append(out, p)
		}
	}
	return out
}
