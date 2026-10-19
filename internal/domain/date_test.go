package domain

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseDate(t *testing.T) {
	if d, err := ParseDate("2026-02-28"); err != nil || d != "2026-02-28" {
		t.Fatalf("ParseDate valid = %q, %v", d, err)
	}
	for _, in := range []string{"", "2026-02-30", "28/02/2026", "2026-2-1", "abc"} {
		if _, err := ParseDate(in); err == nil {
			t.Errorf("ParseDate(%q) expected error", in)
		}
	}
}

func TestParseDateAndMonthRejectYearsOutsideSupportedRange(t *testing.T) {
	for _, in := range []string{"1900-01-01", "2199-12-31"} {
		if _, err := ParseDate(in); err != nil {
			t.Errorf("ParseDate(%q) = %v, want valid", in, err)
		}
	}
	for _, in := range []string{"0206-01-01", "0026-05-10", "1899-12-31", "2200-01-01", "9999-12-31"} {
		if _, err := ParseDate(in); err == nil {
			t.Errorf("ParseDate(%q) expected error", in)
		}
	}
	for _, in := range []string{"0206-01", "1899-12", "2200-01"} {
		if _, err := ParseMonth(in); err == nil {
			t.Errorf("ParseMonth(%q) expected error", in)
		}
	}
}

func TestDateUnmarshalJSONValidates(t *testing.T) {
	var ok struct{ D Date }
	if err := json.Unmarshal([]byte(`{"D":"2026-10-02"}`), &ok); err != nil || ok.D != "2026-10-02" {
		t.Fatalf("valid date: %v %q", err, ok.D)
	}
	var bad struct{ D Date }
	for _, body := range []string{`{"D":"2026-13-01"}`, `{"D":5}`, `{"D":null}`} {
		if err := json.Unmarshal([]byte(body), &bad); err == nil {
			t.Errorf("body %s expected error", body)
		}
	}
}

func TestDateMonthAndAddMonthsClampsDay(t *testing.T) {
	d := Date("2026-01-31")
	if d.Month() != "2026-01" {
		t.Fatalf("Month = %s", d.Month())
	}
	cases := map[int]Date{1: "2026-02-28", 2: "2026-03-31", 13: "2027-02-28", 0: "2026-01-31"}
	for n, want := range cases {
		if got := d.AddMonths(n); got != want {
			t.Errorf("AddMonths(%d) = %s, want %s", n, got, want)
		}
	}
	if got := Date("2024-01-31").AddMonths(1); got != "2024-02-29" {
		t.Errorf("leap year = %s", got)
	}
}

func TestMonthHelpers(t *testing.T) {
	m, err := ParseMonth("2026-12")
	if err != nil {
		t.Fatal(err)
	}
	if m.Add(1) != "2027-01" || m.Add(-12) != "2025-12" {
		t.Errorf("Add wrong: %s %s", m.Add(1), m.Add(-12))
	}
	if m.EndDate() != "2026-12-31" || Month("2026-02").EndDate() != "2026-02-28" {
		t.Errorf("EndDate wrong")
	}
	if MonthsBetween("2026-10", "2027-07") != 9 || MonthsBetween("2027-07", "2026-10") != -9 {
		t.Errorf("MonthsBetween wrong")
	}
	for _, in := range []string{"", "2026-13", "2026-1", "2026-12-01"} {
		if _, err := ParseMonth(in); err == nil {
			t.Errorf("ParseMonth(%q) expected error", in)
		}
	}
}

func TestMonthRange(t *testing.T) {
	got := MonthRange("2026-11", "2027-01")
	want := []Month{"2026-11", "2026-12", "2027-01"}
	if len(got) != 3 || got[0] != want[0] || got[2] != want[2] {
		t.Fatalf("MonthRange = %v", got)
	}
	if len(MonthRange("2027-01", "2026-11")) != 0 {
		t.Errorf("reversed range must be empty")
	}
}

func TestDateOf(t *testing.T) {
	if got := DateOf(time.Date(2026, 10, 2, 23, 59, 0, 0, time.UTC)); got != "2026-10-02" {
		t.Errorf("DateOf = %s", got)
	}
}
