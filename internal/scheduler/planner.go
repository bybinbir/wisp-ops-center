package scheduler

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

// ScheduleType identifies a top-level cadence.
type ScheduleType string

const (
	SchedManual   ScheduleType = "manual"
	SchedDaily    ScheduleType = "daily"
	SchedWeekly   ScheduleType = "weekly"
	SchedMonthly  ScheduleType = "monthly"
	SchedOneTime  ScheduleType = "one_time"
	SchedInterval ScheduleType = "interval"
)

// ScopeType for scheduled checks.
type ScopeType string

const (
	ScopeAll       ScopeType = "all_network"
	ScopeSite      ScopeType = "site"
	ScopeTower     ScopeType = "tower"
	ScopeDevice    ScopeType = "device"
	ScopeCustGroup ScopeType = "customer_group"
	ScopeCustomer  ScopeType = "customer"
	ScopeLink      ScopeType = "link"
)

// PlanInput describes the data the planner uses to compute next_run_at.
type PlanInput struct {
	ScheduleType   ScheduleType
	CronExpression string // "M H D Mo W" simple form (Phase 5 limited subset)
	Timezone       string // IANA name; UTC if empty
	IntervalSec    int    // for interval schedules
	From           time.Time
}

// ErrInvalidSchedule reports a malformed PlanInput.
var ErrInvalidSchedule = errors.New("scheduler: invalid schedule")

// CalculateNextRunAt computes the next execution time. Faz 5 yalnız
// daily/weekly/monthly/one_time/interval/manual destekler. Cron
// "M H * * *" / "M H * * D" formatlarını anlar; daha karmaşık ifadeler
// kabul edilmez (kasıtlı: misconfig riski azaltır).
func CalculateNextRunAt(in PlanInput) (time.Time, error) {
	loc, _ := time.LoadLocation(in.Timezone)
	if loc == nil {
		loc = time.UTC
	}
	now := in.From
	if now.IsZero() {
		now = time.Now()
	}
	now = now.In(loc)

	switch in.ScheduleType {
	case SchedManual:
		// Manual schedules never auto-fire.
		return time.Time{}, nil
	case SchedOneTime:
		// Cron is repurposed as "YYYY-MM-DDTHH:MM:SS" if provided.
		if in.CronExpression == "" {
			return time.Time{}, ErrInvalidSchedule
		}
		t, err := time.ParseInLocation("2006-01-02T15:04:05", in.CronExpression, loc)
		if err != nil {
			return time.Time{}, ErrInvalidSchedule
		}
		if t.Before(now) {
			return time.Time{}, nil
		}
		return t, nil
	case SchedInterval:
		if in.IntervalSec < 30 {
			return time.Time{}, ErrInvalidSchedule
		}
		return now.Add(time.Duration(in.IntervalSec) * time.Second), nil
	case SchedDaily, SchedWeekly, SchedMonthly:
		return calcCronLike(in.CronExpression, in.ScheduleType, now, loc)
	}
	return time.Time{}, ErrInvalidSchedule
}

// calcCronLike resolves a tiny subset of cron: "M H" baseline, optional
// weekday for weekly, day-of-month for monthly.
func calcCronLike(expr string, st ScheduleType, from time.Time, loc *time.Location) (time.Time, error) {
	parts := strings.Fields(expr)
	if len(parts) < 2 {
		return time.Time{}, ErrInvalidSchedule
	}
	min, err := strconv.Atoi(parts[0])
	if err != nil || min < 0 || min > 59 {
		return time.Time{}, ErrInvalidSchedule
	}
	hour, err := strconv.Atoi(parts[1])
	if err != nil || hour < 0 || hour > 23 {
		return time.Time{}, ErrInvalidSchedule
	}

	candidate := time.Date(from.Year(), from.Month(), from.Day(), hour, min, 0, 0, loc)
	if !candidate.After(from) {
		candidate = candidate.Add(24 * time.Hour)
	}

	switch st {
	case SchedDaily:
		return candidate, nil
	case SchedWeekly:
		// Optional weekday in third position (0=Sunday).
		if len(parts) < 3 {
			return candidate, nil
		}
		wd, err := strconv.Atoi(parts[2])
		if err != nil || wd < 0 || wd > 6 {
			return time.Time{}, ErrInvalidSchedule
		}
		for candidate.Weekday() != time.Weekday(wd) {
			candidate = candidate.Add(24 * time.Hour)
		}
		return candidate, nil
	case SchedMonthly:
		// Optional day-of-month in third position.
		if len(parts) < 3 {
			return candidate, nil
		}
		dom, err := strconv.Atoi(parts[2])
		if err != nil || dom < 1 || dom > 31 {
			return time.Time{}, ErrInvalidSchedule
		}
		c := time.Date(candidate.Year(), candidate.Month(), dom, hour, min, 0, 0, loc)
		if !c.After(from) {
			c = c.AddDate(0, 1, 0)
		}
		return c, nil
	}
	return time.Time{}, ErrInvalidSchedule
}
