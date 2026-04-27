// Package scheduler additions — Phase 5 policy & engine helpers.
//
// The engine is split across files so risk/jobs/planner/maintenance can
// be unit-tested without a Postgres or Redis dependency. See
// engine_test.go for the canonical invariants.
package scheduler

import (
	"errors"
	"strings"
	"time"
)

// ScheduledCheckInput is the shape of CRUD input received from the API
// or worker tests. The repository converts this into a row.
type ScheduledCheckInput struct {
	Name             string
	JobType          JobType
	ScopeType        ScopeType
	ScopeID          string
	ScheduleType     ScheduleType
	CronExpression   string
	Timezone         string
	IntervalSec      int
	Enabled          bool
	ActionMode       ActionMode
	RiskLevel        RiskLevel
	MaintenanceWinID string
	MaxDurationSec   int
	MaxParallel      int
}

// Validate enforces Phase 5 schedule + risk + action rules.
func (in ScheduledCheckInput) Validate() error {
	if strings.TrimSpace(in.Name) == "" {
		return errors.New("name required")
	}
	if err := EnsureJobAllowed(in.JobType); err != nil {
		return err
	}
	if !IsValidRiskLevel(in.RiskLevel) {
		return errors.New("invalid risk_level")
	}
	if in.ActionMode == ModeControlledApply {
		return ErrControlledApplyForbidden
	}
	if in.RiskLevel == RiskHigh && in.ActionMode != ModeManualApproval {
		return ErrHighRiskNeedsApproval
	}
	switch in.ScheduleType {
	case SchedManual, SchedDaily, SchedWeekly, SchedMonthly, SchedOneTime, SchedInterval:
	default:
		return errors.New("invalid schedule_type")
	}
	if in.MaxDurationSec < 0 {
		return errors.New("max_duration_seconds must be >= 0")
	}
	if in.MaxParallel < 0 || in.MaxParallel > 32 {
		return errors.New("max_parallel out of range")
	}
	return nil
}

// NextRun returns the next scheduled fire time for in (zero if manual).
func (in ScheduledCheckInput) NextRun(now time.Time) (time.Time, error) {
	return CalculateNextRunAt(PlanInput{
		ScheduleType:   in.ScheduleType,
		CronExpression: in.CronExpression,
		Timezone:       in.Timezone,
		IntervalSec:    in.IntervalSec,
		From:           now,
	})
}
