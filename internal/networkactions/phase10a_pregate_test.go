package networkactions

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestEnsureDestructiveAllowedWithProviders_NilProvidersFailClosed —
// nil providers MUST refuse, never panic.
func TestEnsureDestructiveAllowedWithProviders_NilProvidersFailClosed(t *testing.T) {
	err := EnsureDestructiveAllowedWithProviders(context.Background(), nil, DestructiveRequest{
		Kind: KindFrequencyCorrection,
	})
	if !errors.Is(err, ErrToggleProviderRequired) {
		t.Errorf("nil providers: got %v want ErrToggleProviderRequired", err)
	}
}

// TestEnsureDestructiveAllowedWithProviders_DefaultClosedToggleBlocks —
// even a perfectly-shaped request fails when the toggle is closed.
// This is Phase 10A's master safety invariant.
func TestEnsureDestructiveAllowedWithProviders_DefaultClosedToggleBlocks(t *testing.T) {
	now := time.Now().UTC()
	store := NewMemoryMaintenanceStore()
	_, _ = store.Create(context.Background(), MaintenanceRecord{
		Title: "open", Start: now.Add(-time.Minute), End: now.Add(time.Hour),
	})
	providers := &DestructiveProviders{
		Toggle:      NewMemoryToggle(), // default closed
		RBAC:        NewDefaultRoleResolver(),
		Maintenance: store,
	}
	req := DestructiveRequest{
		Kind:           KindFrequencyCorrection,
		Actor:          "alice",
		ActorRoles:     []string{"net_admin"},
		Confirm:        true,
		RollbackNote:   "rollback in 60s",
		IdempotencyKey: "intent-A",
		Now:            now,
	}
	err := EnsureDestructiveAllowedWithProviders(context.Background(), providers, req)
	if !errors.Is(err, ErrDestructiveDisabled) {
		t.Fatalf("default-closed toggle MUST block, got %v", err)
	}
}

// TestEnsureDestructiveAllowedWithProviders_NonDestructiveBypass — a
// Kind whose IsDestructive()=false bypasses the gate even when the
// toggle is open. (Phase 9 v2 read-only Kinds keep working.)
func TestEnsureDestructiveAllowedWithProviders_NonDestructiveBypass(t *testing.T) {
	tg := NewMemoryToggle()
	_, _ = tg.Flip(context.Background(), true, "alice", "test")
	providers := &DestructiveProviders{Toggle: tg}
	if err := EnsureDestructiveAllowedWithProviders(context.Background(), providers, DestructiveRequest{
		Kind: KindFrequencyCheck,
	}); err != nil {
		t.Errorf("non-destructive must bypass, got %v", err)
	}
}

// TestEnsureDestructiveAllowedWithProviders_FullGuardrailMatrix —
// when toggle is open AND Kind is destructive, every other
// guardrail must enforce in order.
func TestEnsureDestructiveAllowedWithProviders_FullGuardrailMatrix(t *testing.T) {
	now := time.Now().UTC()
	makeProviders := func() *DestructiveProviders {
		tg := NewMemoryToggle()
		_, _ = tg.Flip(context.Background(), true, "alice", "open for test")
		store := NewMemoryMaintenanceStore()
		_, _ = store.Create(context.Background(), MaintenanceRecord{
			Title: "open", Start: now.Add(-time.Minute), End: now.Add(time.Hour),
		})
		return &DestructiveProviders{
			Toggle:      tg,
			RBAC:        NewDefaultRoleResolver(),
			Maintenance: store,
		}
	}
	good := DestructiveRequest{
		Kind:           KindFrequencyCorrection,
		Actor:          "alice",
		ActorRoles:     []string{"net_admin"},
		Confirm:        true,
		RollbackNote:   "rollback in 60s",
		IdempotencyKey: "intent-A",
		Now:            now,
	}
	if err := EnsureDestructiveAllowedWithProviders(context.Background(), makeProviders(), good); err != nil {
		t.Fatalf("good request rejected: %v", err)
	}

	checks := []struct {
		name string
		mut  func(*DestructiveRequest, *DestructiveProviders)
		want error
	}{
		{"rbac no caps", func(r *DestructiveRequest, _ *DestructiveProviders) {
			r.ActorRoles = []string{"net_viewer"}
		}, ErrRBACDenied},
		{"rbac unknown role", func(r *DestructiveRequest, _ *DestructiveProviders) {
			r.ActorRoles = []string{"random"}
		}, ErrRBACDenied},
		{"intent missing", func(r *DestructiveRequest, _ *DestructiveProviders) {
			r.Confirm = false
		}, ErrIntentNotConfirmed},
		{"window provider nil", func(_ *DestructiveRequest, p *DestructiveProviders) {
			p.Maintenance = nil
		}, ErrWindowProviderRequired},
		{"window closed", func(_ *DestructiveRequest, p *DestructiveProviders) {
			p.Maintenance = NewMemoryMaintenanceStore() // empty store
		}, ErrMaintenanceWindowClosed},
		{"rollback empty", func(r *DestructiveRequest, _ *DestructiveProviders) {
			r.RollbackNote = "  "
		}, ErrRollbackNoteMissing},
		{"idempotency empty", func(r *DestructiveRequest, _ *DestructiveProviders) {
			r.IdempotencyKey = ""
		}, ErrIdempotencyKeyMissing},
	}
	for _, c := range checks {
		req := good
		req.ActorRoles = append([]string{}, good.ActorRoles...)
		providers := makeProviders()
		c.mut(&req, providers)
		err := EnsureDestructiveAllowedWithProviders(context.Background(), providers, req)
		if !errors.Is(err, c.want) {
			t.Errorf("%s: got %v want %v", c.name, err, c.want)
		}
	}
}

// TestAuditCatalog_Stable — every audit action name MUST stay stable
// because downstream consumers grep by literal.
func TestAuditCatalog_Stable(t *testing.T) {
	want := []DestructiveAuditAction{
		"network_action.confirmed",
		"network_action.gate_fail",
		"network_action.dry_run",
		"network_action.live_start_blocked",
		"network_action.toggle_flipped",
		"network_action.rbac_denied",
		"network_action.maintenance_window_denied",
	}
	got := DestructiveAuditCatalog()
	if len(got) != len(want) {
		t.Fatalf("catalog size changed: got %d want %d (%+v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("catalog[%d]: got %q want %q", i, got[i], want[i])
		}
	}
}

// TestAuditActionForGateError_Mapping — RBAC + window errors get
// targeted action names; others fall through to gate_fail.
func TestAuditActionForGateError_Mapping(t *testing.T) {
	cases := map[error]DestructiveAuditAction{
		ErrRBACDenied:               AuditActionRBACDenied,
		ErrMaintenanceWindowMissing: AuditActionMaintenanceWindowDenied,
		ErrMaintenanceWindowClosed:  AuditActionMaintenanceWindowDenied,
		ErrWindowProviderRequired:   AuditActionMaintenanceWindowDenied,
		ErrDestructiveDisabled:      AuditActionGateFail,
		ErrIntentNotConfirmed:       AuditActionGateFail,
	}
	for err, want := range cases {
		if got := AuditActionForGateError(err); got != want {
			t.Errorf("err=%v: got %q want %q", err, got, want)
		}
	}
}

// TestProviderGate_FailClosedOnToggleStoreError — when the toggle
// store returns an error (simulated by a custom impl), the gate
// MUST fail-closed (ErrDestructiveDisabled), not panic.
func TestProviderGate_FailClosedOnToggleStoreError(t *testing.T) {
	providers := &DestructiveProviders{
		Toggle: errToggle{},
	}
	err := EnsureDestructiveAllowedWithProviders(context.Background(), providers, DestructiveRequest{
		Kind: KindFrequencyCorrection,
	})
	if !errors.Is(err, ErrDestructiveDisabled) {
		t.Errorf("toggle error MUST fail-closed, got %v", err)
	}
}

type errToggle struct{}

func (errToggle) Enabled(_ context.Context) (bool, error) { return false, errors.New("simulated") }
func (errToggle) Flip(_ context.Context, _ bool, _, _ string) (FlipReceipt, error) {
	return FlipReceipt{}, errors.New("simulated")
}
