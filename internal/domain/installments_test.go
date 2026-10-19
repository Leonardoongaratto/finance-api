package domain

import "testing"

func TestSplitInstallmentsPutsRemainderOnFirst(t *testing.T) {
	got, err := SplitInstallments(10000, 3)
	if err != nil {
		t.Fatal(err)
	}
	want := []int64{3334, 3333, 3333}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("SplitInstallments = %v, want %v", got, want)
		}
	}
}

func TestSplitInstallmentsAlwaysSumsToTotal(t *testing.T) {
	for _, total := range []int64{49, 100, 9999, 123457, 1000000007} {
		for n := 2; n <= 48; n++ {
			if total < int64(n) {
				continue
			}
			parts, err := SplitInstallments(total, n)
			if err != nil {
				t.Fatalf("total=%d n=%d: %v", total, n, err)
			}
			var sum int64
			for _, p := range parts {
				sum += p
			}
			if sum != total || len(parts) != n {
				t.Fatalf("total=%d n=%d sum=%d len=%d", total, n, sum, len(parts))
			}
		}
	}
}

func TestSplitInstallmentsValidation(t *testing.T) {
	for _, c := range []struct {
		total int64
		n     int
	}{{10000, 1}, {10000, 49}, {0, 3}, {-5, 3}, {2, 3}} {
		if _, err := SplitInstallments(c.total, c.n); err == nil {
			t.Errorf("SplitInstallments(%d,%d) expected error", c.total, c.n)
		}
	}
}

func TestBuildInstallmentsCreatesLinkedMonthlyExpenses(t *testing.T) {
	plan := InstallmentPlan{
		TotalCents: 10000, Installments: 3, FirstDate: "2026-01-31",
		AccountID: 7, CategoryID: 9, Description: "Notebook",
	}
	txs, err := BuildInstallments(plan, "grp1")
	if err != nil {
		t.Fatal(err)
	}
	wantAmounts := []int64{3334, 3333, 3333}
	wantDates := []Date{"2026-01-31", "2026-02-28", "2026-03-31"}
	wantDesc := []string{"Notebook 1/3", "Notebook 2/3", "Notebook 3/3"}
	for i, tx := range txs {
		if tx.AmountCents != wantAmounts[i] || tx.Date != wantDates[i] || tx.Description != wantDesc[i] {
			t.Errorf("tx[%d] = %+v", i, tx)
		}
		if tx.Type != Expense || tx.InstallmentGroup != "grp1" || tx.AccountID != 7 || tx.CategoryID != 9 {
			t.Errorf("tx[%d] metadata = %+v", i, tx)
		}
	}
}

func TestBuildInstallmentsEmptyDescriptionStillNumbered(t *testing.T) {
	txs, err := BuildInstallments(InstallmentPlan{TotalCents: 200, Installments: 2, FirstDate: "2026-01-01"}, "g")
	if err != nil || txs[0].Description != "1/2" {
		t.Fatalf("got %+v err=%v", txs, err)
	}
}

func TestBuildInstallmentsPropagatesSplitError(t *testing.T) {
	if _, err := BuildInstallments(InstallmentPlan{TotalCents: 100, Installments: 1, FirstDate: "2026-01-01"}, "g"); err == nil {
		t.Fatal("expected error")
	}
}
