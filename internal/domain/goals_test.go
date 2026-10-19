package domain

import "testing"

func TestEvaluateGoalProgressAndMonthlyNeeded(t *testing.T) {
	goal := Goal{TargetCents: 1200000, Deadline: "2027-07-02", StartDate: "2026-10-01"}
	p := EvaluateGoal(goal, 300000, "2026-10-02")
	if p.ProgressPct != 25 || p.RemainingCents != 900000 || p.MonthsLeft != 9 || p.MonthlyNeededCents != 100000 {
		t.Fatalf("progress = %+v", p)
	}
}

func TestEvaluateGoalMonthlyNeededRoundsUp(t *testing.T) {
	goal := Goal{TargetCents: 1000, Deadline: "2026-12-31", StartDate: "2026-10-01"}
	p := EvaluateGoal(goal, 900, "2026-10-02")
	if p.MonthsLeft != 2 || p.MonthlyNeededCents != 50 {
		t.Fatalf("progress = %+v", p)
	}
	goal.TargetCents = 1001
	if p := EvaluateGoal(goal, 900, "2026-10-02"); p.MonthlyNeededCents != 51 {
		t.Fatalf("ceil expected 51, got %+v", p)
	}
}

func TestEvaluateGoalOverdueAndDone(t *testing.T) {
	goal := Goal{TargetCents: 10000, Deadline: "2026-09-01", StartDate: "2026-01-01"}
	if p := EvaluateGoal(goal, 5000, "2026-10-02"); p.Status != GoalOverdue || p.MonthlyNeededCents != 5000 {
		t.Fatalf("overdue = %+v", p)
	}
	if p := EvaluateGoal(goal, 10000, "2026-10-02"); p.Status != GoalDone || p.ProgressPct != 100 || p.RemainingCents != 0 || p.MonthlyNeededCents != 0 {
		t.Fatalf("done = %+v", p)
	}
	if p := EvaluateGoal(goal, 15000, "2026-10-02"); p.ProgressPct != 100 || p.RemainingCents != 0 {
		t.Fatalf("over-saved must cap at 100: %+v", p)
	}
}

func TestEvaluateGoalOnTrackVersusBehind(t *testing.T) {
	goal := Goal{TargetCents: 10000, Deadline: "2026-12-31", StartDate: "2026-01-01"}
	today := Date("2026-07-02")
	if p := EvaluateGoal(goal, 9000, today); p.Status != GoalOnTrack {
		t.Errorf("ahead of pace should be on_track: %+v", p)
	}
	if p := EvaluateGoal(goal, 100, today); p.Status != GoalBehind {
		t.Errorf("below pace should be behind: %+v", p)
	}
}

func TestEvaluateGoalDeadlineInCurrentMonthNeedsOneMonth(t *testing.T) {
	goal := Goal{TargetCents: 1000, Deadline: "2026-10-30", StartDate: "2026-09-01"}
	if p := EvaluateGoal(goal, 0, "2026-10-02"); p.MonthsLeft != 1 || p.MonthlyNeededCents != 1000 {
		t.Fatalf("progress = %+v", p)
	}
}

func TestIdealPace(t *testing.T) {
	goal := Goal{TargetCents: 10000, Deadline: "2026-12-31", StartDate: "2026-01-01"}
	if got := IdealPace(goal, "2025-12-31"); got != 0 {
		t.Errorf("before start = %d", got)
	}
	if got := IdealPace(goal, "2027-03-01"); got != 10000 {
		t.Errorf("after deadline = %d", got)
	}
	mid := IdealPace(goal, "2026-07-02")
	if mid < 4900 || mid > 5200 {
		t.Errorf("mid = %d", mid)
	}
	same := Goal{TargetCents: 500, Deadline: "2026-01-01", StartDate: "2026-01-01"}
	if got := IdealPace(same, "2026-01-01"); got != 500 {
		t.Errorf("zero-length goal = %d", got)
	}
}
