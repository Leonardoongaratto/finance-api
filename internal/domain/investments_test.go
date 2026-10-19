package domain

import "testing"

func TestSummarizeInvestmentGain(t *testing.T) {
	contribs := []Contribution{{Date: "2026-01-10", AmountCents: 100000}, {Date: "2026-02-10", AmountCents: 50000}}
	vals := []Valuation{{Date: "2026-03-01", ValueCents: 180000}}
	s := SummarizeInvestment(contribs, vals, "2026-12-31")
	if s.InvestedCents != 150000 || s.CurrentCents != 180000 || s.GainCents != 30000 || s.ReturnPct != 20 {
		t.Fatalf("summary = %+v", s)
	}
}

func TestSummarizeInvestmentWithoutValuationUsesInvested(t *testing.T) {
	s := SummarizeInvestment([]Contribution{{Date: "2026-01-10", AmountCents: 1000}}, nil, "2026-12-31")
	if s.CurrentCents != 1000 || s.GainCents != 0 || s.ReturnPct != 0 {
		t.Fatalf("summary = %+v", s)
	}
}

func TestSummarizeInvestmentIsAsOfDate(t *testing.T) {
	contribs := []Contribution{{Date: "2026-01-10", AmountCents: 1000}, {Date: "2026-06-10", AmountCents: 500}}
	vals := []Valuation{{Date: "2026-02-01", ValueCents: 1200}, {Date: "2026-05-01", ValueCents: 1400}}

	atMarch := SummarizeInvestment(contribs, vals, "2026-03-31")
	if atMarch.InvestedCents != 1000 || atMarch.CurrentCents != 1200 {
		t.Errorf("march = %+v", atMarch)
	}
	atJune := SummarizeInvestment(contribs, vals, "2026-06-30")
	if atJune.InvestedCents != 1500 || atJune.CurrentCents != 1900 {
		t.Errorf("june = %+v (latest valuation plus later contributions)", atJune)
	}
	beforeAny := SummarizeInvestment(contribs, vals, "2025-12-31")
	if beforeAny.InvestedCents != 0 || beforeAny.CurrentCents != 0 || beforeAny.ReturnPct != 0 {
		t.Errorf("before = %+v", beforeAny)
	}
}

func TestSummarizeInvestmentNegativeReturn(t *testing.T) {
	s := SummarizeInvestment([]Contribution{{Date: "2026-01-01", AmountCents: 10000}}, []Valuation{{Date: "2026-02-01", ValueCents: 9000}}, "2026-12-31")
	if s.GainCents != -1000 || s.ReturnPct != -10 {
		t.Fatalf("summary = %+v", s)
	}
}
