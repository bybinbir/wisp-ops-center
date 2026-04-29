package retention

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeStore is a deterministic Store for the retention service tests.
// Each method records its calls so tests can assert "we counted N
// rows" + "we issued M deletes" without an actual DB.
type fakeStore struct {
	counts  map[Table]int64
	deletes map[Table]int64
	// errOn maps table → error to return on the matching call.
	countErrOn  map[Table]error
	deleteErrOn map[Table]error
	// protected tables refuse DELETE with ErrTableProtected.
	protected map[Table]struct{}

	countCalls  []countCall
	deleteCalls []deleteCall
}

type countCall struct {
	table  Table
	cutoff time.Time
}

type deleteCall struct {
	table  Table
	cutoff time.Time
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		counts:      map[Table]int64{},
		deletes:     map[Table]int64{},
		countErrOn:  map[Table]error{},
		deleteErrOn: map[Table]error{},
		protected:   map[Table]struct{}{},
	}
}

func (f *fakeStore) CountCandidates(_ context.Context, t Table, cutoff time.Time) (int64, error) {
	f.countCalls = append(f.countCalls, countCall{table: t, cutoff: cutoff})
	if e, ok := f.countErrOn[t]; ok {
		return 0, e
	}
	return f.counts[t], nil
}

func (f *fakeStore) DeleteOlderThan(_ context.Context, t Table, cutoff time.Time) (int64, error) {
	f.deleteCalls = append(f.deleteCalls, deleteCall{table: t, cutoff: cutoff})
	if _, p := f.protected[t]; p {
		return 0, ErrTableProtected
	}
	if e, ok := f.deleteErrOn[t]; ok {
		return 0, e
	}
	return f.deletes[t], nil
}

// fakeAudit records every emit so tests can assert which audit
// events fired with what metadata. We deliberately do NOT assert
// the exact metadata bag content — only structural fields — so
// the test stays robust against benign metadata additions.
type fakeAudit struct {
	events []retEvent
}

type retEvent struct {
	action  string
	outcome string
	meta    map[string]any
}

func (f *fakeAudit) Emit(_ context.Context, action, outcome string, meta map[string]any) {
	cp := make(map[string]any, len(meta))
	for k, v := range meta {
		cp[k] = v
	}
	f.events = append(f.events, retEvent{action: action, outcome: outcome, meta: cp})
}

func (f *fakeAudit) actions() []string {
	out := make([]string, len(f.events))
	for i, e := range f.events {
		out[i] = e.action
	}
	return out
}

// silentLogger is the package's local Logger no-op.
type silentLogger struct{}

func (silentLogger) Info(string, ...any)  {}
func (silentLogger) Warn(string, ...any)  {}
func (silentLogger) Error(string, ...any) {}

// =============================================================================
// Disabled mode does nothing
// =============================================================================
func TestPhase10F_Retention_DisabledNoop(t *testing.T) {
	cfg := Config{Mode: ModeDisabled}
	store := newFakeStore()
	audit := &fakeAudit{}
	svc, err := New(cfg, store, audit, silentLogger{})
	if err != nil {
		t.Fatalf("New err = %v, want nil", err)
	}
	summaries, err := svc.Cleanup(context.Background())
	if err != nil {
		t.Errorf("Cleanup err = %v, want nil", err)
	}
	if len(summaries) != 0 {
		t.Errorf("summaries len = %d, want 0", len(summaries))
	}
	if len(store.countCalls) != 0 || len(store.deleteCalls) != 0 {
		t.Error("disabled mode must not touch the store")
	}
	want := []string{AuditActionRetentionDisabled}
	got := audit.actions()
	if !equalStrings(got, want) {
		t.Errorf("audit actions:\n got  %v\n want %v", got, want)
	}
}

// =============================================================================
// Empty / zero Mode also disables
// =============================================================================
func TestPhase10F_Retention_EmptyModeIsDisabled(t *testing.T) {
	store := newFakeStore()
	audit := &fakeAudit{}
	cfg := Config{Mode: ""} // explicit zero value
	svc, err := New(cfg, store, audit, silentLogger{})
	if err != nil {
		t.Fatalf("New err = %v, want nil for empty mode", err)
	}
	if _, err := svc.Cleanup(context.Background()); err != nil {
		t.Errorf("Cleanup err = %v, want nil", err)
	}
	if len(audit.events) != 1 || audit.events[0].action != AuditActionRetentionDisabled {
		t.Errorf("empty mode must emit retention.disabled exactly once, got %v", audit.actions())
	}
}

// =============================================================================
// Dry-run counts but does not delete
// =============================================================================
func TestPhase10F_Retention_DryRunCountsNoDelete(t *testing.T) {
	store := newFakeStore()
	store.counts[TableNetworkActionRuns] = 5
	store.counts[TableAuditLogs] = 12
	audit := &fakeAudit{}
	cfg := Config{
		Mode:  ModeDryRun,
		Actor: "alice",
		Tables: []TableConfig{
			{Table: TableAuditLogs, RetentionDays: 90},
			{Table: TableNetworkActionRuns, RetentionDays: 30},
		},
	}
	svc, err := New(cfg, store, audit, silentLogger{})
	if err != nil {
		t.Fatalf("New err = %v", err)
	}
	fixed := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
	svc.SetClock(func() time.Time { return fixed })
	summaries, err := svc.Cleanup(context.Background())
	if err != nil {
		t.Errorf("Cleanup err = %v, want nil", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("summaries len = %d, want 2", len(summaries))
	}
	if len(store.deleteCalls) != 0 {
		t.Error("dry-run must NOT issue any DELETE")
	}
	if len(store.countCalls) != 2 {
		t.Errorf("count calls = %d, want 2", len(store.countCalls))
	}
	auditWant := []string{
		AuditActionRetentionDryRunCounted, // audit_logs first per config order
		AuditActionRetentionDryRunCounted, // network_action_runs second
	}
	if !equalStrings(audit.actions(), auditWant) {
		t.Errorf("audit actions:\n got  %v\n want %v", audit.actions(), auditWant)
	}
}

// =============================================================================
// Boundary date is now() - days
// =============================================================================
func TestPhase10F_Retention_CutoffBoundaryCorrect(t *testing.T) {
	store := newFakeStore()
	audit := &fakeAudit{}
	cfg := Config{
		Mode:  ModeDryRun,
		Actor: "alice",
		Tables: []TableConfig{
			{Table: TableNetworkActionRuns, RetentionDays: 30},
		},
	}
	svc, err := New(cfg, store, audit, silentLogger{})
	if err != nil {
		t.Fatalf("New err = %v", err)
	}
	fixed := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
	svc.SetClock(func() time.Time { return fixed })
	if _, err := svc.Cleanup(context.Background()); err != nil {
		t.Fatalf("Cleanup err = %v", err)
	}
	if len(store.countCalls) != 1 {
		t.Fatalf("count calls = %d, want 1", len(store.countCalls))
	}
	wantCutoff := fixed.Add(-30 * 24 * time.Hour)
	if !store.countCalls[0].cutoff.Equal(wantCutoff) {
		t.Errorf("cutoff = %v, want %v", store.countCalls[0].cutoff, wantCutoff)
	}
	// Audit metadata MUST carry the formatted cutoff.
	if len(audit.events) != 1 {
		t.Fatalf("audit events = %d, want 1", len(audit.events))
	}
	cutoffStr, _ := audit.events[0].meta["cutoff"].(string)
	if !strings.HasPrefix(cutoffStr, "2026-03-30") {
		t.Errorf("audit cutoff = %q, want prefix 2026-03-30", cutoffStr)
	}
}

// =============================================================================
// Execute deletes only when permitted; protected tables count-only
// =============================================================================
func TestPhase10F_Retention_ExecuteRespectsProtection(t *testing.T) {
	store := newFakeStore()
	store.counts[TableAuditLogs] = 10
	store.counts[TableNetworkActionRuns] = 7
	store.deletes[TableNetworkActionRuns] = 7
	store.protected[TableAuditLogs] = struct{}{}
	audit := &fakeAudit{}
	cfg := Config{
		Mode:  ModeExecute,
		Actor: "alice",
		Tables: []TableConfig{
			{Table: TableAuditLogs, RetentionDays: 365},
			{Table: TableNetworkActionRuns, RetentionDays: 30},
		},
	}
	svc, err := New(cfg, store, audit, silentLogger{})
	if err != nil {
		t.Fatalf("New err = %v", err)
	}
	summaries, err := svc.Cleanup(context.Background())
	if err != nil {
		t.Errorf("Cleanup err = %v, want nil", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("summaries len = %d, want 2", len(summaries))
	}
	// audit_logs is protected → counted but not deleted
	if !summaries[0].Protected {
		t.Error("audit_logs should be marked Protected")
	}
	if summaries[0].Deleted != 0 {
		t.Errorf("audit_logs Deleted = %d, want 0", summaries[0].Deleted)
	}
	if summaries[0].Candidates != 10 {
		t.Errorf("audit_logs Candidates = %d, want 10", summaries[0].Candidates)
	}
	// network_action_runs deleted
	if summaries[1].Protected {
		t.Error("network_action_runs should not be marked Protected")
	}
	if summaries[1].Deleted != 7 {
		t.Errorf("network_action_runs Deleted = %d, want 7", summaries[1].Deleted)
	}
	// Audit fired retention.table_protected for audit_logs and
	// retention.deleted for network_action_runs.
	want := []string{
		AuditActionRetentionTableProtected,
		AuditActionRetentionDeleted,
	}
	if !equalStrings(audit.actions(), want) {
		t.Errorf("audit actions:\n got  %v\n want %v", audit.actions(), want)
	}
}

// =============================================================================
// Zero/negative retention days opt the table out
// =============================================================================
func TestPhase10F_Retention_ZeroDaysOptsOut(t *testing.T) {
	store := newFakeStore()
	audit := &fakeAudit{}
	cfg := Config{
		Mode:  ModeDryRun,
		Actor: "alice",
		Tables: []TableConfig{
			{Table: TableAuditLogs, RetentionDays: 0}, // opt-out
			{Table: TableNetworkActionRuns, RetentionDays: 30},
		},
	}
	svc, _ := New(cfg, store, audit, silentLogger{})
	summaries, _ := svc.Cleanup(context.Background())
	if len(summaries) != 1 {
		t.Errorf("summaries len = %d, want 1 (opt-out skipped silently)", len(summaries))
	}
	if len(store.countCalls) != 1 {
		t.Errorf("count calls = %d, want 1", len(store.countCalls))
	}
	if store.countCalls[0].table != TableNetworkActionRuns {
		t.Errorf("counted wrong table: %v", store.countCalls[0].table)
	}
}

// =============================================================================
// Invalid config fails closed at New()
// =============================================================================
func TestPhase10F_Retention_InvalidConfigRejected(t *testing.T) {
	cases := map[string]Config{
		"unknown-mode": {
			Mode:  Mode("yolo"),
			Actor: "alice",
		},
		"actor-missing-when-active": {
			Mode: ModeDryRun,
		},
		"unknown-table": {
			Mode:  ModeDryRun,
			Actor: "alice",
			Tables: []TableConfig{
				{Table: Table("/etc/passwd"), RetentionDays: 1},
			},
		},
		"duplicate-table": {
			Mode:  ModeDryRun,
			Actor: "alice",
			Tables: []TableConfig{
				{Table: TableAuditLogs, RetentionDays: 1},
				{Table: TableAuditLogs, RetentionDays: 2},
			},
		},
		"negative-days": {
			Mode:  ModeDryRun,
			Actor: "alice",
			Tables: []TableConfig{
				{Table: TableAuditLogs, RetentionDays: -7},
			},
		},
	}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := New(cfg, newFakeStore(), &fakeAudit{}, silentLogger{})
			if err == nil {
				t.Errorf("New(%s) accepted invalid config; want error", name)
			}
		})
	}
}

// =============================================================================
// Count error is sanitised + does not abort the next table
// =============================================================================
func TestPhase10F_Retention_CountErrorContinues(t *testing.T) {
	store := newFakeStore()
	store.countErrOn[TableAuditLogs] = errors.New("pq: connection refused (DSN=postgres://app:hunter2@host/db?sslmode=disable)\nQuery: SELECT count(*) FROM audit_logs WHERE at < $1\nDETAIL: redacted")
	store.counts[TableNetworkActionRuns] = 3
	audit := &fakeAudit{}
	cfg := Config{
		Mode:  ModeDryRun,
		Actor: "alice",
		Tables: []TableConfig{
			{Table: TableAuditLogs, RetentionDays: 90},
			{Table: TableNetworkActionRuns, RetentionDays: 30},
		},
	}
	svc, _ := New(cfg, store, audit, silentLogger{})
	summaries, err := svc.Cleanup(context.Background())
	if err == nil {
		t.Error("expected aggregate err for first failed table")
	}
	if len(summaries) != 2 {
		t.Errorf("summaries len = %d, want 2 (continued past first err)", len(summaries))
	}
	if summaries[0].Err == nil {
		t.Error("first summary must record per-table error")
	}
	if summaries[1].Candidates != 3 {
		t.Errorf("second summary Candidates = %d, want 3", summaries[1].Candidates)
	}
	// Audit row for the failure MUST NOT contain the DSN or the
	// query body — sanitiseStoreError trims after the first newline
	// and clamps length, so the second line ("Query: …") is dropped.
	failEvt := audit.events[0]
	errMsg, _ := failEvt.meta["error"].(string)
	if strings.Contains(errMsg, "Query:") || strings.Contains(errMsg, "DETAIL:") {
		t.Errorf("audit error leak: %q", errMsg)
	}
	if strings.Contains(errMsg, "hunter2") {
		t.Errorf("audit error leaked password: %q", errMsg)
	}
}

// =============================================================================
// Helpers
// =============================================================================

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
