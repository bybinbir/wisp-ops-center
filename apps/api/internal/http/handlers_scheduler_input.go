package http

import (
	"errors"
	"net/http"

	"github.com/wisp-ops-center/wisp-ops-center/internal/scheduler"
)

// readScheduleInput decodes the JSON body into a ScheduledCheckInput.
func readScheduleInput(r *http.Request) (scheduler.ScheduledCheckInput, error) {
	var in struct {
		Name             string `json:"name"`
		JobType          string `json:"job_type"`
		ScopeType        string `json:"scope_type,omitempty"`
		ScopeID          string `json:"scope_id,omitempty"`
		ScheduleType     string `json:"schedule_type,omitempty"`
		CronExpression   string `json:"cron_expression,omitempty"`
		Timezone         string `json:"timezone,omitempty"`
		IntervalSec      int    `json:"interval_sec,omitempty"`
		Enabled          *bool  `json:"enabled,omitempty"`
		ActionMode       string `json:"action_mode,omitempty"`
		RiskLevel        string `json:"risk_level,omitempty"`
		MaintenanceWinID string `json:"maintenance_window_id,omitempty"`
		MaxDurationSec   int    `json:"max_duration_seconds,omitempty"`
		MaxParallel      int    `json:"max_parallel,omitempty"`
	}
	if err := readJSON(r, &in); err != nil {
		return scheduler.ScheduledCheckInput{}, err
	}
	if in.Name == "" || in.JobType == "" {
		return scheduler.ScheduledCheckInput{}, errors.New("name and job_type required")
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	if in.ScopeType == "" {
		in.ScopeType = "all_network"
	}
	if in.ScheduleType == "" {
		in.ScheduleType = "manual"
	}
	if in.RiskLevel == "" {
		in.RiskLevel = "low"
	}
	if in.ActionMode == "" {
		in.ActionMode = "report_only"
	}
	if in.Timezone == "" {
		in.Timezone = "UTC"
	}
	if in.MaxDurationSec == 0 {
		in.MaxDurationSec = 60
	}
	if in.MaxParallel == 0 {
		in.MaxParallel = 4
	}
	return scheduler.ScheduledCheckInput{
		Name:             in.Name,
		JobType:          scheduler.JobType(in.JobType),
		ScheduleType:     scheduler.ScheduleType(in.ScheduleType),
		CronExpression:   in.CronExpression,
		Timezone:         in.Timezone,
		IntervalSec:      in.IntervalSec,
		ScopeType:        scheduler.ScopeType(in.ScopeType),
		ScopeID:          in.ScopeID,
		Enabled:          enabled,
		ActionMode:       scheduler.ActionMode(in.ActionMode),
		RiskLevel:        scheduler.RiskLevel(in.RiskLevel),
		MaintenanceWinID: in.MaintenanceWinID,
		MaxDurationSec:   in.MaxDurationSec,
		MaxParallel:      in.MaxParallel,
	}, nil
}
