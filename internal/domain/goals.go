package domain

type GoalStatus string

const (
	GoalOnTrack GoalStatus = "on_track"
	GoalBehind  GoalStatus = "behind"
	GoalDone    GoalStatus = "done"
	GoalOverdue GoalStatus = "overdue"
)

type GoalProgress struct {
	SavedCents         int64      `json:"saved_cents"`
	ProgressPct        int64      `json:"progress_pct"`
	RemainingCents     int64      `json:"remaining_cents"`
	MonthsLeft         int        `json:"months_left"`
	MonthlyNeededCents int64      `json:"monthly_needed_cents"`
	Status             GoalStatus `json:"status"`
}

func EvaluateGoal(goal Goal, savedCents int64, today Date) GoalProgress {
	remaining := max(goal.TargetCents-savedCents, 0)
	progress := GoalProgress{
		SavedCents:     savedCents,
		ProgressPct:    min(savedCents*100/goal.TargetCents, 100),
		RemainingCents: remaining,
		MonthsLeft:     max(MonthsBetween(today.Month(), goal.Deadline.Month()), 0),
	}

	switch {
	case remaining == 0:
		progress.Status = GoalDone
	case goal.Deadline < today:
		progress.Status = GoalOverdue
		progress.MonthsLeft = 0
		progress.MonthlyNeededCents = remaining
	default:
		progress.MonthsLeft = max(progress.MonthsLeft, 1)
		progress.MonthlyNeededCents = ceilDiv(remaining, int64(progress.MonthsLeft))
		progress.Status = GoalBehind
		if onPace(goal, savedCents, today) {
			progress.Status = GoalOnTrack
		}
	}
	return progress
}

// IdealPace is the amount a goal should have accumulated by a date if saved at a constant rate from start to deadline.
func IdealPace(goal Goal, date Date) int64 {
	switch {
	case date >= goal.Deadline:
		return goal.TargetCents
	case date <= goal.StartDate:
		return 0
	}
	total := daysBetween(goal.StartDate, goal.Deadline)
	elapsed := daysBetween(goal.StartDate, date)
	return goal.TargetCents * elapsed / total
}

func onPace(goal Goal, savedCents int64, today Date) bool {
	if today <= goal.StartDate || goal.Deadline <= goal.StartDate {
		return true
	}
	total := daysBetween(goal.StartDate, goal.Deadline)
	elapsed := daysBetween(goal.StartDate, today)
	return savedCents*total >= goal.TargetCents*elapsed
}

func daysBetween(from, to Date) int64 {
	return int64(to.Time().Sub(from.Time()).Hours() / 24)
}

func ceilDiv(a, b int64) int64 {
	return (a + b - 1) / b
}
