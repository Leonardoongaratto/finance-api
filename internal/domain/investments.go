package domain

import "math"

type InvestmentSummary struct {
	InvestedCents int64   `json:"invested_cents"`
	CurrentCents  int64   `json:"current_cents"`
	GainCents     int64   `json:"gain_cents"`
	ReturnPct     float64 `json:"return_pct"`
}

// SummarizeInvestment values an investment as of a date: the latest valuation
// on or before it plus contributions made after that valuation, or the total
// invested when no valuation exists yet.
func SummarizeInvestment(contributions []Contribution, valuations []Valuation, asOf Date) InvestmentSummary {
	var invested int64
	for _, c := range contributions {
		if c.Date <= asOf {
			invested += c.AmountCents
		}
	}

	current := invested
	if latest, ok := latestValuation(valuations, asOf); ok {
		current = latest.ValueCents
		for _, c := range contributions {
			if c.Date > latest.Date && c.Date <= asOf {
				current += c.AmountCents
			}
		}
	}

	summary := InvestmentSummary{InvestedCents: invested, CurrentCents: current, GainCents: current - invested}
	if invested > 0 {
		summary.ReturnPct = math.Round(float64(summary.GainCents)*10000/float64(invested)) / 100
	}
	return summary
}

func latestValuation(valuations []Valuation, asOf Date) (Valuation, bool) {
	var best Valuation
	found := false
	for _, v := range valuations {
		if v.Date > asOf {
			continue
		}
		if !found || v.Date > best.Date || (v.Date == best.Date && v.ID > best.ID) {
			best, found = v, true
		}
	}
	return best, found
}
