package networkactions

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestEnsureDestructiveAllowed_MasterSwitchBlocksAll — the gate
// must refuse every destructive request as long as
// DestructiveActionEnabled=false. Phase 9 v3 ships with this
// false.
func TestEnsureDestructiveAllowed_MasterSwitchBlocksAll(t *testing.T) {
	if DestructiveActionEnabled {
		t.Fatalf("DestructiveActionEnabled MUST be false in Phase 9 v3")
	}
	now := time.Now().UTC()
	req := DestructiveRequest{
		Kind:           KindFrequencyCorrection,
		Actor:          "alice",
		ActorRoles:     []string{"net_ops"},
		Confirm:        true,
		RollbackNote:   "rollback to previous channel within 60s",
		IdempotencyKey: "intent-123",
		Window:         &MaintenanceWindow{Start: now.Add(-5 * time.Minute), End: now.Add(5 * time.Minute)},
		Now:            now,
	}
	err := EnsureDestructiveAllowed(context.Background(), req)
	if !errors.Is(err, ErrDestructiveDisabled) {
		t.Fatalf("master switch must block, got %v", err)
	}
	if DestructiveErrorCode(err) != "destructive_disabled" {
		t.Errorf("error_code mapping wrong: %q", DestructiveErrorCode(err))
	}
}

// TestEnsureDestructiveAllowed_NonDestructiveBypassesGate — calling
// the gate with a Kind whose IsDestructive()=false must return nil
// (the pre-gate is only meant to fence destructive Kinds).
func TestEnsureDestructiveAllowed_NonDestructiveBypassesGate(t *testing.T) {
	prev := DestructiveActionEnabled
	DestructiveActionEnabled = true
	defer func() { DestructiveActionEnabled = prev }()
	req := DestructiveRequest{Kind: KindFrequencyCheck}
	if err := EnsureDestructiveAllowed(context.Background(), req); err != nil {
		t.Errorf("non-destructive must bypass gate, got %v", err)
	}
}

// TestEnsureDestructiveAllowed_RequiresAllGuardrails — when the
// master switch is flipped on (Phase 10), the rest of the gate must
// reject in order: RBAC, intent, window, rollback, idempotency.
func TestEnsureDestructiveAllowed_RequiresAllGuardrails(t *testing.T) {
	prev := DestructiveActionEnabled
	DestructiveActionEnabled = true
	defer func() { DestructiveActionEnabled = prev }()
	now := time.Now().UTC()
	good := DestructiveRequest{
		Kind:           KindFrequencyCorrection,
		Actor:          "alice",
		ActorRoles:     []string{"net_ops"},
		Confirm:        true,
		RollbackNote:   "rollback within 60s",
		IdempotencyKey: "intent-1",
		Window:         &MaintenanceWindow{Start: now.Add(-5 * time.Minute), End: now.Add(5 * time.Minute)},
		Now:            now,
	}
	if err := EnsureDestructiveAllowed(context.Background(), good); err != nil {
		t.Fatalf("good request rejected: %v", err)
	}

	checks := []struct {
		name string
		mut  func(*DestructiveRequest)
		want error
	}{
		{"rbac", func(r *DestructiveRequest) { r.ActorRoles = []string{"viewer"} }, ErrRBACDenied},
		{"intent", func(r *DestructiveRequest) { r.Confirm = false }, ErrIntentNotConfirmed},
		{"window_missing", func(r *DestructiveRequest) { r.Window = nil }, ErrMaintenanceWindowMissing},
		{"window_closed", func(r *DestructiveRequest) {
			r.Window = &MaintenanceWindow{
				Start: r.Now.Add(time.Hour), End: r.Now.Add(2 * time.Hour),
			}
		}, ErrMaintenanceWindowClosed},
		{"rollback", func(r *DestructiveRequest) { r.RollbackNote = "" }, ErrRollbackNoteMissing},
		{"idempotency", func(r *DestructiveRequest) { r.IdempotencyKey = "" }, ErrIdempotencyKeyMissing},
	}
	for _, c := range checks {
		req := good
		req.ActorRoles = append([]string{}, good.ActorRoles...)
		if good.Window != nil {
			w := *good.Window
			req.Window = &w
		}
		c.mut(&req)
		err := EnsureDestructiveAllowed(context.Background(), req)
		if !errors.Is(err, c.want) {
			t.Errorf("%s: got %v want %v", c.name, err, c.want)
		}
	}
}

// TestPreGateChecklist_LeastFourteenItems — the checklist exists
// and covers the canonical Phase 10 guardrails. Numerical guard
// against accidental shrinkage.
func TestPreGateChecklist_LeastFourteenItems(t *testing.T) {
	got := PreGateChecklist()
	if len(got) < 14 {
		t.Errorf("checklist shrunk: %d items, want >=14: %+v", len(got), got)
	}
	required := []string{
		"destructive_master_switch", "rbac_role",
		"explicit_intent_confirmation", "maintenance_window_open",
		"rollback_note_required", "idempotency_key_required",
		"per_device_lock", "rate_limit", "panic_recovery",
		"mutation_deny_list", "secret_redaction", "mac_masking",
	}
	have := map[string]struct{}{}
	for _, s := range got {
		have[s] = struct{}{}
	}
	for _, r := range required {
		if _, ok := have[r]; !ok {
			t.Errorf("checklist missing required item %q", r)
		}
	}
}

// TestDestructiveErrorCode_StableLabels — every typed sentinel maps
// to a stable, short label.
func TestDestructiveErrorCode_StableLabels(t *testing.T) {
	cases := map[error]string{
		nil:                             "",
		ErrDestructiveDisabled:          "destructive_disabled",
		ErrIntentNotConfirmed:           "intent_not_confirmed",
		ErrMaintenanceWindowMissing:     "maintenance_window_missing",
		ErrMaintenanceWindowClosed:      "maintenance_window_closed",
		ErrRollbackNoteMissing:          "rollback_note_missing",
		ErrRBACDenied:                   "rbac_denied",
		ErrIdempotencyKeyMissing:        "idempotency_key_missing",
		errors.New("some random error"): "unknown",
	}
	for err, want := range cases {
		if got := DestructiveErrorCode(err); got != want {
			t.Errorf("DestructiveErrorCode(%v)=%q want %q", err, got, want)
		}
	}
}
