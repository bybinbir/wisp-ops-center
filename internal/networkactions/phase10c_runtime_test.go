package networkactions

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestPhase10C_GateBlocksWithClosedToggle — the destructive runner's
// MOST IMPORTANT invariant: with the master switch closed (default),
// the gate refuses every destructive request regardless of what
// other guardrails would say. Phase 10C MUST emit
// `destructive_denied` and never reach Execute.
func TestPhase10C_GateBlocksWithClosedToggle(t *testing.T) {
	providers := &DestructiveProviders{
		Toggle:      NewMemoryToggle(),        // default-closed
		RBAC:        NewDefaultRoleResolver(), // would grant net_admin
		Maintenance: NewMemoryMaintenanceStore(),
	}
	req := DestructiveRequest{
		Kind:           KindFrequencyCorrection,
		DeviceID:       "device-1",
		Actor:          "alice",
		ActorRoles:     []string{"net_admin"},
		DryRun:         false,
		Confirm:        true,
		RollbackNote:   "revert via /interface/wireless/set",
		IdempotencyKey: "k-1",
		Now:            time.Now().UTC(),
	}
	err := EnsureDestructiveAllowedWithProviders(context.Background(), providers, req)
	if !errors.Is(err, ErrDestructiveDisabled) {
		t.Errorf("closed toggle: err=%v want ErrDestructiveDisabled", err)
	}
}

// TestPhase10C_GateBlocksWithMissingWindow — even with master
// switch flipped (hermetic test), missing maintenance window MUST
// deny. Phase 10C records this as `maintenance_window_denied`.
func TestPhase10C_GateBlocksWithMissingWindow(t *testing.T) {
	tog := NewMemoryToggle()
	if _, err := tog.Flip(context.Background(), true, "test", "open for invariant test"); err != nil {
		t.Fatalf("toggle flip: %v", err)
	}
	providers := &DestructiveProviders{
		Toggle:      tog,
		RBAC:        NewDefaultRoleResolver(),
		Maintenance: NewMemoryMaintenanceStore(), // empty
	}
	req := DestructiveRequest{
		Kind:           KindFrequencyCorrection,
		DeviceID:       "device-1",
		Actor:          "alice",
		ActorRoles:     []string{"net_admin"},
		Confirm:        true,
		RollbackNote:   "revert plan",
		IdempotencyKey: "k-2",
		Now:            time.Now().UTC(),
	}
	err := EnsureDestructiveAllowedWithProviders(context.Background(), providers, req)
	if !errors.Is(err, ErrMaintenanceWindowClosed) {
		t.Errorf("missing window: err=%v want ErrMaintenanceWindowClosed", err)
	}
}

// TestPhase10C_GateBlocksWithoutConfirmation — Confirm=false MUST
// fail at intent_not_confirmed when master switch is open.
func TestPhase10C_GateBlocksWithoutConfirmation(t *testing.T) {
	tog := NewMemoryToggle()
	_, _ = tog.Flip(context.Background(), true, "test", "open for invariant test")
	providers := &DestructiveProviders{
		Toggle:      tog,
		RBAC:        NewDefaultRoleResolver(),
		Maintenance: NewMemoryMaintenanceStore(),
	}
	req := DestructiveRequest{
		Kind:       KindFrequencyCorrection,
		Actor:      "alice",
		ActorRoles: []string{"net_admin"},
		Confirm:    false,
		Now:        time.Now().UTC(),
	}
	err := EnsureDestructiveAllowedWithProviders(context.Background(), providers, req)
	if !errors.Is(err, ErrIntentNotConfirmed) {
		t.Errorf("no confirm: err=%v want ErrIntentNotConfirmed", err)
	}
}

// TestPhase10C_GateBlocksWithoutRollbackNote — even with all other
// guardrails passing, missing rollback_note MUST deny. Phase 10C
// records the run with rollback_note column populated only when
// the gate accepts the request.
func TestPhase10C_GateBlocksWithoutRollbackNote(t *testing.T) {
	tog := NewMemoryToggle()
	_, _ = tog.Flip(context.Background(), true, "test", "open")
	store := NewMemoryMaintenanceStore()
	now := time.Now().UTC()
	if _, err := store.Create(context.Background(), MaintenanceRecord{
		Title:     "test",
		Start:     now.Add(-time.Hour),
		End:       now.Add(time.Hour),
		CreatedBy: "test",
	}); err != nil {
		t.Fatalf("create window: %v", err)
	}
	providers := &DestructiveProviders{Toggle: tog, RBAC: NewDefaultRoleResolver(), Maintenance: store}
	req := DestructiveRequest{
		Kind:           KindFrequencyCorrection,
		Actor:          "alice",
		ActorRoles:     []string{"net_admin"},
		Confirm:        true,
		IdempotencyKey: "k-3",
		Now:            now,
		// RollbackNote intentionally empty
	}
	err := EnsureDestructiveAllowedWithProviders(context.Background(), providers, req)
	if !errors.Is(err, ErrRollbackNoteMissing) {
		t.Errorf("no rollback: err=%v want ErrRollbackNoteMissing", err)
	}
}

// TestPhase10C_GateBlocksWithoutIdempotencyKey — every destructive
// request MUST carry an idempotency_key so a duplicate POST cannot
// re-execute the same intent. Phase 10C ALSO enforces uniqueness
// at the DB layer via uniq_nar_action_idem partial index.
func TestPhase10C_GateBlocksWithoutIdempotencyKey(t *testing.T) {
	tog := NewMemoryToggle()
	_, _ = tog.Flip(context.Background(), true, "test", "open")
	store := NewMemoryMaintenanceStore()
	now := time.Now().UTC()
	_, _ = store.Create(context.Background(), MaintenanceRecord{
		Title: "w", Start: now.Add(-time.Hour), End: now.Add(time.Hour), CreatedBy: "t",
	})
	providers := &DestructiveProviders{Toggle: tog, RBAC: NewDefaultRoleResolver(), Maintenance: store}
	req := DestructiveRequest{
		Kind:         KindFrequencyCorrection,
		Actor:        "alice",
		ActorRoles:   []string{"net_admin"},
		Confirm:      true,
		RollbackNote: "rollback plan",
		Now:          now,
		// IdempotencyKey intentionally empty
	}
	err := EnsureDestructiveAllowedWithProviders(context.Background(), providers, req)
	if !errors.Is(err, ErrIdempotencyKeyMissing) {
		t.Errorf("no idem: err=%v want ErrIdempotencyKeyMissing", err)
	}
}

// TestPhase10C_GatePassesUnderHermeticHappyPath — the only way to
// exercise the gate's "pass" branch is in a hermetic test with a
// flipped MemoryToggle, a non-empty MemoryMaintenanceStore, full
// guardrail set. The Phase 10C runner consumes this branch by
// emitting `dry_run` (DryRun=true) or `live_start_blocked`
// (Confirm=true) — Execute is NEVER called either way.
func TestPhase10C_GatePassesUnderHermeticHappyPath(t *testing.T) {
	tog := NewMemoryToggle()
	_, _ = tog.Flip(context.Background(), true, "test", "open")
	store := NewMemoryMaintenanceStore()
	now := time.Now().UTC()
	_, _ = store.Create(context.Background(), MaintenanceRecord{
		Title: "w", Start: now.Add(-time.Hour), End: now.Add(time.Hour), CreatedBy: "t",
	})
	providers := &DestructiveProviders{Toggle: tog, RBAC: NewDefaultRoleResolver(), Maintenance: store}
	req := DestructiveRequest{
		Kind:           KindFrequencyCorrection,
		DeviceID:       "device-x",
		Actor:          "alice",
		ActorRoles:     []string{"net_admin"},
		Confirm:        true,
		RollbackNote:   "revert via /interface/wireless/set",
		IdempotencyKey: "k-pass",
		Now:            now,
	}
	if err := EnsureDestructiveAllowedWithProviders(context.Background(), providers, req); err != nil {
		t.Errorf("hermetic happy path: gate denied (%v); Phase 10C contract requires the gate to pass here so the runner can emit live_start_blocked", err)
	}
}

// TestPhase10C_NonDestructiveBypassesGate — read-only Kinds MUST
// bypass the gate's RBAC + maintenance + rollback + idempotency
// checks once the master switch is open. The destructive runner is
// never invoked for read-only Kinds in production (handlers_actions
// keeps that path separate); this test pins the gate's bypass
// shape so that contract is documented in code.
//
// Note: with the master switch CLOSED, every request — including
// non-destructive — returns ErrDestructiveDisabled at step 2.
// That is fine because the destructive runner is the only caller
// of this gate. Phase 9 read-only handlers (handlers_actions.go)
// never invoke EnsureDestructiveAllowedWithProviders.
func TestPhase10C_NonDestructiveBypassesGate(t *testing.T) {
	tog := NewMemoryToggle()
	_, _ = tog.Flip(context.Background(), true, "test", "open for bypass test")
	providers := &DestructiveProviders{
		Toggle:      tog,
		RBAC:        NewDefaultRoleResolver(),
		Maintenance: NewMemoryMaintenanceStore(),
	}
	for _, k := range []Kind{KindFrequencyCheck, KindAPClientTest, KindLinkSignalTest, KindBridgeHealthCheck} {
		req := DestructiveRequest{Kind: k, Actor: "alice", Now: time.Now().UTC()}
		if err := EnsureDestructiveAllowedWithProviders(context.Background(), providers, req); err != nil {
			t.Errorf("non-destructive %q: err=%v want nil (bypass)", k, err)
		}
	}
}
