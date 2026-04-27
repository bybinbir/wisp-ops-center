package scheduler

import "time"

// MaintenanceWindow describes a time-bounded operational window.
// Recurrence strings are limited to the subset documented in
// docs/MAINTENANCE_WINDOWS.md.
type MaintenanceWindow struct {
	ID         string
	Name       string
	ScopeType  ScopeType
	ScopeID    string
	StartsAt   time.Time
	EndsAt     time.Time
	Timezone   string
	Recurrence string // "" | "daily" | "weekly" | "monthly"
	Enabled    bool
}

// IsActive returns true if `at` falls inside the window. Recurrence is
// applied by sliding the (start,end) interval to the current cycle.
func (w MaintenanceWindow) IsActive(at time.Time) bool {
	if !w.Enabled {
		return false
	}
	loc, _ := time.LoadLocation(w.Timezone)
	if loc == nil {
		loc = time.UTC
	}
	at = at.In(loc)
	start := w.StartsAt.In(loc)
	end := w.EndsAt.In(loc)
	if !end.After(start) {
		return false
	}
	switch w.Recurrence {
	case "":
		return !at.Before(start) && at.Before(end)
	case "daily":
		// Project onto today.
		s := time.Date(at.Year(), at.Month(), at.Day(), start.Hour(), start.Minute(), start.Second(), 0, loc)
		dur := end.Sub(start)
		e := s.Add(dur)
		return !at.Before(s) && at.Before(e)
	case "weekly":
		// Project onto current week.
		days := int(at.Weekday()) - int(start.Weekday())
		s := time.Date(at.Year(), at.Month(), at.Day(), start.Hour(), start.Minute(), 0, 0, loc).AddDate(0, 0, -days)
		dur := end.Sub(start)
		e := s.Add(dur)
		return !at.Before(s) && at.Before(e)
	case "monthly":
		s := time.Date(at.Year(), at.Month(), start.Day(), start.Hour(), start.Minute(), 0, 0, loc)
		dur := end.Sub(start)
		e := s.Add(dur)
		return !at.Before(s) && at.Before(e)
	}
	return false
}

// GuardWindow returns the appropriate sentinel error when the given
// risk level is incompatible with the windows for `at`. Low-risk passes
// regardless; medium-risk passes (caller may emit a warning); high-risk
// requires an active window.
func GuardWindow(risk RiskLevel, windows []MaintenanceWindow, at time.Time) error {
	switch risk {
	case RiskLow, RiskMedium:
		return nil
	case RiskHigh:
		for _, w := range windows {
			if w.IsActive(at) {
				return nil
			}
		}
		return ErrOutsideMaintenanceWindow
	}
	return nil
}

// WarnMediumRiskOutsideWindow returns true if the caller should log a
// warning because a medium-risk job is being scheduled outside any
// configured window.
func WarnMediumRiskOutsideWindow(risk RiskLevel, windows []MaintenanceWindow, at time.Time) bool {
	if risk != RiskMedium {
		return false
	}
	for _, w := range windows {
		if w.IsActive(at) {
			return false
		}
	}
	return true
}
