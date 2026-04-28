package networkactions

import (
	"context"
	"testing"
)

// TestDefaultRoleResolver_KnownRoles checks the canonical role →
// capability mapping the platform ships.
func TestDefaultRoleResolver_KnownRoles(t *testing.T) {
	r := NewDefaultRoleResolver()

	cases := []struct {
		roles []string
		want  []Capability
	}{
		{[]string{"net_admin"}, []Capability{
			CapabilityToggleFlip, CapabilityDestructiveExecute, CapabilityDestructiveDryRun,
		}},
		{[]string{"net_ops"}, []Capability{
			CapabilityDestructiveExecute, CapabilityDestructiveDryRun,
		}},
		{[]string{"net_viewer"}, []Capability{}},
		{[]string{"unknown_role"}, []Capability{}},
		{[]string{}, []Capability{}},
	}
	for _, c := range cases {
		got, err := r.Capabilities(context.Background(), Principal{Roles: c.roles})
		if err != nil {
			t.Errorf("roles=%v err=%v", c.roles, err)
			continue
		}
		if !sameCaps(got, c.want) {
			t.Errorf("roles=%v got=%v want=%v", c.roles, got, c.want)
		}
	}
}

// TestDefaultRoleResolver_RoleUnion — multiple roles union their
// capabilities, no duplicates.
func TestDefaultRoleResolver_RoleUnion(t *testing.T) {
	r := NewDefaultRoleResolver()
	got, err := r.Capabilities(context.Background(), Principal{
		Roles: []string{"net_ops", "net_viewer", "net_admin"},
	})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	want := map[Capability]bool{
		CapabilityToggleFlip:         true,
		CapabilityDestructiveExecute: true,
		CapabilityDestructiveDryRun:  true,
	}
	for _, c := range got {
		if !want[c] {
			t.Errorf("unexpected capability %q", c)
		}
		delete(want, c)
	}
	if len(want) != 0 {
		t.Errorf("missing capabilities: %v", want)
	}
}

// TestStaticRoleResolver_NilStoreIsUnavailable — nil receiver MUST
// return ErrRBACResolverUnavailable so callers can fail-closed.
func TestStaticRoleResolver_NilStoreIsUnavailable(t *testing.T) {
	var r *StaticRoleResolver
	_, err := r.Capabilities(context.Background(), Principal{Roles: []string{"net_admin"}})
	if err != ErrRBACResolverUnavailable {
		t.Errorf("nil resolver must return ErrRBACResolverUnavailable, got %v", err)
	}
}

// TestHasCapability_FailClosed — a resolver error MUST cause
// HasCapability to return false (deny).
func TestHasCapability_FailClosed(t *testing.T) {
	var r *StaticRoleResolver
	have := HasCapability(context.Background(), r, Principal{Roles: []string{"net_admin"}}, CapabilityDestructiveExecute)
	if have {
		t.Errorf("nil resolver MUST fail-closed (deny)")
	}
}

// TestHasCapability_NilResolverDenies — nil resolver argument is
// treated as fail-closed too.
func TestHasCapability_NilResolverDenies(t *testing.T) {
	if HasCapability(context.Background(), nil, Principal{Roles: []string{"net_admin"}}, CapabilityDestructiveExecute) {
		t.Errorf("nil resolver MUST deny")
	}
}

// TestStaticRoleResolver_CaseInsensitive — role lookup is case +
// whitespace tolerant so a session with "Net_Admin " still resolves.
func TestStaticRoleResolver_CaseInsensitive(t *testing.T) {
	r := NewDefaultRoleResolver()
	got, err := r.Capabilities(context.Background(), Principal{Roles: []string{"  Net_Admin "}})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if len(got) == 0 {
		t.Errorf("expected admin caps, got none")
	}
}

func sameCaps(a, b []Capability) bool {
	if len(a) != len(b) {
		return false
	}
	m := map[Capability]int{}
	for _, c := range a {
		m[c]++
	}
	for _, c := range b {
		if m[c] == 0 {
			return false
		}
		m[c]--
	}
	return true
}
