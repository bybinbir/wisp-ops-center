package networkactions

import (
	"context"
	"errors"
	"testing"
)

// TestPgRoleResolver_NilPoolFailsClosed — nil pool MUST return
// ErrRBACResolverUnavailable. The HasCapability helper turns this
// into a deny.
func TestPgRoleResolver_NilPoolFailsClosed(t *testing.T) {
	var r *PgRoleResolver
	_, err := r.Capabilities(context.Background(), Principal{Actor: "alice"})
	if !errors.Is(err, ErrRBACResolverUnavailable) {
		t.Errorf("nil receiver: err=%v want ErrRBACResolverUnavailable", err)
	}
	r2 := &PgRoleResolver{P: nil, CapabilityMap: NewDefaultRoleResolver()}
	_, _, err = r2.LookupRoles(context.Background(), "alice")
	if !errors.Is(err, ErrRBACResolverUnavailable) {
		t.Errorf("nil P: err=%v want ErrRBACResolverUnavailable", err)
	}
}

// TestPgRBACResolver_FallbackWhenSQLUnavailable — when no SQL
// resolver is wired, the wrapper falls through to the header-based
// static resolver. This preserves Phase 10B parity for deployments
// without a wired grants table.
func TestPgRBACResolver_FallbackWhenSQLUnavailable(t *testing.T) {
	r := NewPgRBACResolver(nil, NewDefaultRoleResolver())
	caps, err := r.Capabilities(context.Background(), Principal{Actor: "alice", Roles: []string{"net_admin"}})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if len(caps) == 0 {
		t.Errorf("expected admin caps from header fallback")
	}
}

// TestPgRBACResolver_StrictMode_DeniesUnknownPrincipal — when
// RequireSQL=true and the SQL resolver reports the principal is
// unknown, the wrapper MUST return ErrPrincipalUnknown without
// touching the static fallback. This is the production posture.
func TestPgRBACResolver_StrictMode_DeniesUnknownPrincipal(t *testing.T) {
	w := &PgRBACResolver{
		Fallback:   NewDefaultRoleResolver(),
		SQL:        &fakeSQLResolver{err: ErrPrincipalUnknown},
		RequireSQL: true,
	}
	_, err := w.Capabilities(context.Background(), Principal{Actor: "alice", Roles: []string{"net_admin"}})
	if !errors.Is(err, ErrPrincipalUnknown) {
		t.Errorf("strict mode: err=%v want ErrPrincipalUnknown", err)
	}
}

// TestPgRBACResolver_PermissiveModeFallsBackOnUnknown — when
// RequireSQL=false and the SQL resolver reports unknown, the
// wrapper consults the header-based fallback so a not-yet-seeded
// deployment keeps working. Header roles drive caps in this mode.
func TestPgRBACResolver_PermissiveModeFallsBackOnUnknown(t *testing.T) {
	w := &PgRBACResolver{
		Fallback:   NewDefaultRoleResolver(),
		SQL:        &fakeSQLResolver{err: ErrPrincipalUnknown},
		RequireSQL: false,
	}
	caps, err := w.Capabilities(context.Background(), Principal{Actor: "alice", Roles: []string{"net_admin"}})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if len(caps) == 0 {
		t.Errorf("expected fallback to grant admin caps via headers")
	}
}

// TestPgRBACResolver_DBErrorIsHardDeny — even when RequireSQL=false,
// a DB outage MUST NOT silently fall through to header roles. An
// attacker who can degrade the DB connection should not gain caps.
func TestPgRBACResolver_DBErrorIsHardDeny(t *testing.T) {
	w := &PgRBACResolver{
		Fallback:   NewDefaultRoleResolver(),
		SQL:        &fakeSQLResolver{err: ErrRBACResolverUnavailable},
		RequireSQL: false,
	}
	_, err := w.Capabilities(context.Background(), Principal{Actor: "alice", Roles: []string{"net_admin"}})
	if !errors.Is(err, ErrRBACResolverUnavailable) {
		t.Errorf("db error: err=%v want ErrRBACResolverUnavailable", err)
	}
}

// TestPgRBACResolver_SQLOverridesHeaderRoles — when SQL returns
// caps for an actor, those drive authorization. Header roles are
// IGNORED for that actor (privilege-escalation defense).
func TestPgRBACResolver_SQLOverridesHeaderRoles(t *testing.T) {
	w := &PgRBACResolver{
		Fallback: NewDefaultRoleResolver(),
		SQL: &fakeSQLResolver{caps: []Capability{
			CapabilityPreflightRead,
		}},
	}
	caps, err := w.Capabilities(context.Background(), Principal{Actor: "alice", Roles: []string{"net_admin"}})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if len(caps) != 1 || caps[0] != CapabilityPreflightRead {
		t.Errorf("SQL must override headers: caps=%v want [%s]", caps, CapabilityPreflightRead)
	}
}

// fakeSQLResolver is a stand-in for PgRoleResolver whose Capabilities
// returns a fixed result. Lets us drive the wrapper logic without a
// real DB.
type fakeSQLResolver struct {
	caps []Capability
	err  error
}

func (f *fakeSQLResolver) Capabilities(_ context.Context, _ Principal) ([]Capability, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.caps, nil
}
