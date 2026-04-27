package scheduler

import (
	"errors"
	"testing"
	"time"
)

func TestRiskLevelEnumIsClosed(t *testing.T) {
	if IsValidRiskLevel("nuclear") {
		t.Fatal("invalid risk should not be accepted")
	}
	for _, r := range AllRiskLevels() {
		if !IsValidRiskLevel(r) {
			t.Fatalf("expected valid: %s", r)
		}
	}
}

func TestJobCatalogControlsExecution(t *testing.T) {
	if err := EnsureJobAllowed("ap_client_ping_latency"); err != nil {
		t.Fatalf("ping_latency must be enabled: %v", err)
	}
	if err := EnsureJobAllowed("mikrotik_bandwidth_test"); !errors.Is(err, ErrJobTypeDisabled) {
		t.Fatalf("bandwidth-test must be disabled: %v", err)
	}
	if err := EnsureJobAllowed("ap_client_limited_throughput"); !errors.Is(err, ErrJobTypeDisabled) {
		t.Fatalf("limited_throughput must be disabled: %v", err)
	}
	if err := EnsureJobAllowed("nope"); !errors.Is(err, ErrJobTypeUnknown) {
		t.Fatalf("unknown should fail: %v", err)
	}
	// Phase 7 — daily_executive_summary kayıtlı ve aktif olmalı.
	if err := EnsureJobAllowed(JobDailyExecutiveSummary); err != nil {
		t.Fatalf("daily_executive_summary must be enabled: %v", err)
	}
}

func TestControlledApplyAlwaysRejected(t *testing.T) {
	in := ScheduledCheckInput{
		Name: "x", JobType: JobMikroTikReadOnlyPoll,
		ScopeType: ScopeAll, ScheduleType: SchedDaily,
		ActionMode: ModeControlledApply, RiskLevel: RiskLow,
		CronExpression: "0 3",
	}
	if err := in.Validate(); !errors.Is(err, ErrControlledApplyForbidden) {
		t.Fatalf("controlled_apply must be forbidden: %v", err)
	}
}

func TestHighRiskRequiresManualApproval(t *testing.T) {
	in := ScheduledCheckInput{
		Name: "x", JobType: JobMikroTikReadOnlyPoll,
		ScopeType: ScopeDevice, ScheduleType: SchedManual,
		ActionMode: ModeReportOnly, RiskLevel: RiskHigh,
	}
	if err := in.Validate(); !errors.Is(err, ErrHighRiskNeedsApproval) {
		t.Fatalf("high-risk must require manual approval: %v", err)
	}
}

func TestNextRunDailyAdvances(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")
	from := time.Date(2025, 1, 1, 12, 0, 0, 0, loc)
	t1, err := CalculateNextRunAt(PlanInput{
		ScheduleType: SchedDaily, CronExpression: "30 9", From: from,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2025, 1, 2, 9, 30, 0, 0, loc)
	if !t1.Equal(want) {
		t.Fatalf("daily next: got %v want %v", t1, want)
	}
}

func TestNextRunWeeklyHonorsWeekday(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")
	from := time.Date(2025, 1, 1, 12, 0, 0, 0, loc) // Wednesday
	t1, err := CalculateNextRunAt(PlanInput{
		ScheduleType: SchedWeekly, CronExpression: "0 6 1", From: from, // Monday 06:00
	})
	if err != nil {
		t.Fatal(err)
	}
	if t1.Weekday() != time.Monday {
		t.Fatalf("expected Monday, got %v", t1.Weekday())
	}
	if !t1.After(from) {
		t.Fatal("must be in the future")
	}
}

func TestIntervalRequiresMin30Seconds(t *testing.T) {
	if _, err := CalculateNextRunAt(PlanInput{ScheduleType: SchedInterval, IntervalSec: 5}); !errors.Is(err, ErrInvalidSchedule) {
		t.Fatal("interval too small must fail")
	}
}

func TestManualScheduleHasNoNextRun(t *testing.T) {
	t1, err := CalculateNextRunAt(PlanInput{ScheduleType: SchedManual})
	if err != nil {
		t.Fatal(err)
	}
	if !t1.IsZero() {
		t.Fatal("manual schedule must not auto-fire")
	}
}

func TestMaintenanceWindowGuardsHighRisk(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")
	now := time.Date(2025, 1, 1, 14, 0, 0, 0, loc)
	winInside := MaintenanceWindow{
		StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour),
		Enabled: true, Timezone: "UTC",
	}
	winOutside := MaintenanceWindow{
		StartsAt: now.Add(2 * time.Hour), EndsAt: now.Add(3 * time.Hour),
		Enabled: true, Timezone: "UTC",
	}
	// High risk + inside window: ok.
	if err := GuardWindow(RiskHigh, []MaintenanceWindow{winInside}, now); err != nil {
		t.Fatalf("inside window must pass: %v", err)
	}
	// High risk + outside windows: blocked.
	if err := GuardWindow(RiskHigh, []MaintenanceWindow{winOutside}, now); !errors.Is(err, ErrOutsideMaintenanceWindow) {
		t.Fatalf("expected outside-window block: %v", err)
	}
	// Low risk: always passes.
	if err := GuardWindow(RiskLow, nil, now); err != nil {
		t.Fatal("low-risk must always pass")
	}
}

func TestMaintenanceWindowDailyRecurrence(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")
	w := MaintenanceWindow{
		StartsAt:   time.Date(2024, 12, 1, 2, 0, 0, 0, loc),
		EndsAt:     time.Date(2024, 12, 1, 4, 0, 0, 0, loc),
		Recurrence: "daily", Enabled: true, Timezone: "UTC",
	}
	// 2025-01-15 03:00 UTC must be inside the window.
	at := time.Date(2025, 1, 15, 3, 0, 0, 0, loc)
	if !w.IsActive(at) {
		t.Fatal("daily window must include 03:00 UTC")
	}
	at = time.Date(2025, 1, 15, 5, 0, 0, 0, loc)
	if w.IsActive(at) {
		t.Fatal("daily window must exclude 05:00 UTC")
	}
}

func TestMediumRiskWarnOutsideWindow(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")
	now := time.Date(2025, 1, 1, 14, 0, 0, 0, loc)
	if !WarnMediumRiskOutsideWindow(RiskMedium, nil, now) {
		t.Fatal("medium risk outside any window must warn")
	}
	if WarnMediumRiskOutsideWindow(RiskLow, nil, now) {
		t.Fatal("low risk should not warn")
	}
}
