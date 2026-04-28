package networkactions

import (
	"context"
	"errors"
	"testing"
)

// TestPgRBACResolver_NilDelegatesToFallback — when the resolver has
// no pool, it still consults the static fallback so dev environments
// keep working safely.
func TestPgRBACResolver_DelegatesToFallback(t *testing.T) {
	r := NewPgRBACResolver(nil, NewDefaultRoleResolver())
	caps, err := r.Capabilities(context.Background(), Principal{Roles: []string{"net_admin"}})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if len(caps) == 0 {
		t.Fatalf("expected admin caps from fallback, got empty")
	}
	hasExec := false
	for _, c := range caps {
		if c == CapabilityDestructiveExecute {
			hasExec = true
		}
	}
	if !hasExec {
		t.Errorf("admin should hold CapabilityDestructiveExecute")
	}
}

// TestPgRBACResolver_NilFallbackFailsClosed — nil fallback path
// surfaces ErrRBACResolverUnavailable.
func TestPgRBACResolver_NilFallbackFailsClosed(t *testing.T) {
	r := &PgRBACResolver{P: nil, Fallback: nil}
	_, err := r.Capabilities(context.Background(), Principal{Roles: []string{"net_admin"}})
	if !errors.Is(err, ErrRBACResolverUnavailable) {
		t.Errorf("nil fallback: err=%v want ErrRBACResolverUnavailable", err)
	}
}

// TestNewPgRBACResolver_NilFallbackAutowiresDefault — constructor
// MUST never produce a fail-open zero value. When the caller passes
// nil for the fallback, NewPgRBACResolver injects the default.
func TestNewPgRBACResolver_NilFallbackAutowiresDefault(t *testing.T) {
	r := NewPgRBACResolver(nil, nil)
	if r.Fallback == nil {
		t.Fatal("constructor must auto-wire a non-nil fallback")
	}
	caps, err := r.Capabilities(context.Background(), Principal{Roles: []string{"net_admin"}})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if len(caps) == 0 {
		t.Errorf("default fallback should provide admin caps")
	}
}

// TestRBAC_NewCapabilities — Phase 10B added Maintenance + Preflight
// caps to the default mapping. Pin the mapping so a future edit
// can't quietly remove them.
func TestRBAC_DefaultMappingIncludesPhase10BCaps(t *testing.T) {
	r := NewDefaultRoleResolver()
	cases := []struct {
		role string
		caps []Capability
	}{
		{"net_admin", []Capability{
			CapabilityToggleFlip, CapabilityDestructiveExecute, CapabilityDestructiveDryRun,
			CapabilityMaintenanceManage, CapabilityPreflightRead,
		}},
		{"net_ops", []Capability{
			CapabilityDestructiveExecute, CapabilityDestructiveDryRun,
			CapabilityMaintenanceManage, CapabilityPreflightRead,
		}},
		{"net_viewer", []Capability{
			CapabilityPreflightRead,
		}},
	}
	for _, c := range cases {
		caps, err := r.Capabilities(context.Background(), Principal{Roles: []string{c.role}})
		if err != nil {
			t.Errorf("%s: err=%v", c.role, err)
			continue
		}
		have := map[Capability]struct{}{}
		for _, cap := range caps {
			have[cap] = struct{}{}
		}
		for _, want := range c.caps {
			if _, ok := have[want]; !ok {
				t.Errorf("%s missing capability %q", c.role, want)
			}
		}
	}
}

// TestCapabilityNamesStable — Phase 10B promised these literal
// strings. Pin them so renames cannot drift unnoticed.
func TestCapabilityNamesStable(t *testing.T) {
	want := map[Capability]string{
		CapabilityDestructiveExecute: "network_action.destructive.execute",
		CapabilityDestructiveDryRun:  "network_action.destructive.dryrun",
		CapabilityToggleFlip:         "network_action.toggle.flip",
		CapabilityMaintenanceManage:  "network_action.maintenance.manage",
		CapabilityPreflightRead:      "network_action.preflight.read",
	}
	for cap, expected := range want {
		if string(cap) != expected {
			t.Errorf("capability %q drifted: got %q", expected, string(cap))
		}
	}
}
