package networkactions

import (
	"context"
	"errors"
	"testing"
)

// Phase 10F-A — RBAC strict-mode (RequireSQL=true) test surface.
//
// Phase 10C wired the RequireSQL flag but only exercised it on the
// "SQL resolver responds with ErrPrincipalUnknown" branch. Phase
// 10F-A hardens every code path that COULD silently fall through
// to the header-based static fallback when the operator has
// explicitly required SQL authorisation.
//
// Every test in this file constructs a PgRBACResolver WITHOUT a
// real pgxpool.Pool — the SQL seam is mocked via a stub that
// implements the RBACResolver interface so we can drive each error
// shape deterministically.

// stubRBAC is a programmable RBACResolver. Each test sets caps + err
// to drive PgRBACResolver.Capabilities into the branch under test.
type stubRBAC struct {
	caps []Capability
	err  error
	// recordedActor is set on every call so tests can assert the
	// resolver did/did not hit the SQL seam.
	recordedActor string
	calls         int
}

func (s *stubRBAC) Capabilities(_ context.Context, p Principal) ([]Capability, error) {
	s.recordedActor = p.Actor
	s.calls++
	return s.caps, s.err
}

// TestPhase10F_RequireSQL_AllowWhenSQLGrants pins the happy path:
// SQL resolver returns a non-empty cap list → those caps are
// returned even with RequireSQL=true, header roles ignored.
func TestPhase10F_RequireSQL_AllowWhenSQLGrants(t *testing.T) {
	stub := &stubRBAC{caps: []Capability{
		CapabilityDestructiveExecute,
		CapabilityDestructiveDryRun,
	}}
	r := &PgRBACResolver{
		Fallback:   NewDefaultRoleResolver(),
		SQL:        stub,
		RequireSQL: true,
	}
	caps, err := r.Capabilities(context.Background(), Principal{
		Actor: "alice",
		// header roles deliberately point at viewer-only; SQL must win.
		Roles: []string{"net_viewer"},
	})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if len(caps) != 2 {
		t.Errorf("caps len = %d, want 2 (SQL list)", len(caps))
	}
	if stub.calls != 1 {
		t.Errorf("SQL stub called %d times, want exactly 1", stub.calls)
	}
}

// TestPhase10F_RequireSQL_DenyOnPrincipalUnknown pins the strict-
// mode core: SQL says ErrPrincipalUnknown → ErrPrincipalUnknown is
// surfaced (header roles are NOT consulted).
func TestPhase10F_RequireSQL_DenyOnPrincipalUnknown(t *testing.T) {
	stub := &stubRBAC{err: ErrPrincipalUnknown}
	r := &PgRBACResolver{
		Fallback:   NewDefaultRoleResolver(),
		SQL:        stub,
		RequireSQL: true,
	}
	_, err := r.Capabilities(context.Background(), Principal{
		Actor: "unknown",
		Roles: []string{"net_admin"}, // would have authorised in Phase 10B
	})
	if !errors.Is(err, ErrPrincipalUnknown) {
		t.Errorf("err = %v, want ErrPrincipalUnknown", err)
	}
	// HasCapability turns this error into false; the canonical
	// helper test exercises that translation.
	if HasCapability(context.Background(), r, Principal{
		Actor: "unknown",
		Roles: []string{"net_admin"},
	}, CapabilityDestructiveExecute) {
		t.Error("HasCapability returned true for unknown actor under RequireSQL=true")
	}
}

// TestPhase10F_RequireSQL_DenyOnDBError pins fail-closed for any
// non-ErrPrincipalUnknown error returned by the SQL resolver
// (DB outage, malformed actor, mapping miss, etc.). Header roles
// MUST NOT authorise the call.
func TestPhase10F_RequireSQL_DenyOnDBError(t *testing.T) {
	stub := &stubRBAC{err: ErrRBACResolverUnavailable}
	r := &PgRBACResolver{
		Fallback:   NewDefaultRoleResolver(),
		SQL:        stub,
		RequireSQL: true,
	}
	_, err := r.Capabilities(context.Background(), Principal{
		Actor: "alice",
		Roles: []string{"net_admin"},
	})
	if !errors.Is(err, ErrRBACResolverUnavailable) {
		t.Errorf("err = %v, want ErrRBACResolverUnavailable", err)
	}
}

// TestPhase10F_RequireSQL_DenyWhenSQLNotWired locks in the Phase
// 10F-A hardening: even if the SQL seam is unwired (pool=nil →
// SQL=nil) the resolver MUST refuse rather than silently degrade
// to header-based authorisation. This is the branch the operator
// promises will never fail open after they flip RequireSQL.
func TestPhase10F_RequireSQL_DenyWhenSQLNotWired(t *testing.T) {
	r := &PgRBACResolver{
		Fallback:   NewDefaultRoleResolver(),
		SQL:        nil, // operator forgot to seed the SQL seam
		RequireSQL: true,
	}
	_, err := r.Capabilities(context.Background(), Principal{
		Actor: "alice",
		Roles: []string{"net_admin"},
	})
	if !errors.Is(err, ErrRBACResolverUnavailable) {
		t.Errorf("err = %v, want ErrRBACResolverUnavailable", err)
	}
}

// TestPhase10F_RequireSQL_DenyOnNilFallback covers the defensive
// nil-Fallback branch under strict mode: the resolver itself is
// misconfigured. We refuse the request rather than panic.
func TestPhase10F_RequireSQL_DenyOnNilFallback(t *testing.T) {
	r := &PgRBACResolver{
		Fallback:   nil,
		SQL:        &stubRBAC{caps: []Capability{CapabilityDestructiveExecute}},
		RequireSQL: true,
	}
	_, err := r.Capabilities(context.Background(), Principal{Actor: "alice"})
	if !errors.Is(err, ErrRBACResolverUnavailable) {
		t.Errorf("err = %v, want ErrRBACResolverUnavailable", err)
	}
}

// TestPhase10F_RequireSQL_DenyOnEmptyActor confirms the SQL seam's
// empty-actor refusal flows through under RequireSQL=true.
// PgRoleResolver.LookupRoles returns ErrPrincipalUnknown for an
// empty actor; the test stubs that response so we don't need a
// real DB to exercise the path.
func TestPhase10F_RequireSQL_DenyOnEmptyActor(t *testing.T) {
	stub := &stubRBAC{err: ErrPrincipalUnknown}
	r := &PgRBACResolver{
		Fallback:   NewDefaultRoleResolver(),
		SQL:        stub,
		RequireSQL: true,
	}
	_, err := r.Capabilities(context.Background(), Principal{
		Actor: "",
		Roles: []string{"net_admin"},
	})
	if !errors.Is(err, ErrPrincipalUnknown) {
		t.Errorf("err = %v, want ErrPrincipalUnknown", err)
	}
}

// TestPhase10F_RequireSQLFalse_StillFallsBack confirms backward
// compatibility: when the operator has NOT yet flipped RequireSQL,
// the resolver continues to honour header roles for actors that
// SQL says are unknown. This is the Phase 10B / 10C parity behaviour
// and MUST keep working until every deployment seeds grants.
func TestPhase10F_RequireSQLFalse_StillFallsBack(t *testing.T) {
	stub := &stubRBAC{err: ErrPrincipalUnknown}
	r := &PgRBACResolver{
		Fallback:   NewDefaultRoleResolver(),
		SQL:        stub,
		RequireSQL: false, // Phase 10B/10C parity
	}
	caps, err := r.Capabilities(context.Background(), Principal{
		Actor: "header-alice",
		Roles: []string{"net_admin"},
	})
	if err != nil {
		t.Fatalf("err = %v, want nil (fallback to header roles)", err)
	}
	hasExec := false
	for _, c := range caps {
		if c == CapabilityDestructiveExecute {
			hasExec = true
		}
	}
	if !hasExec {
		t.Error("RequireSQL=false should let header net_admin role grant DestructiveExecute via fallback")
	}
}

// TestPhase10F_HasCapability_FailsClosedOnRequireSQLMisconfig
// exercises the canonical helper under each strict-mode failure
// shape so callers (handler.requireCapability) keep returning 403.
func TestPhase10F_HasCapability_FailsClosedOnRequireSQLMisconfig(t *testing.T) {
	cases := []struct {
		name     string
		resolver *PgRBACResolver
	}{
		{"sql-not-wired", &PgRBACResolver{
			Fallback:   NewDefaultRoleResolver(),
			SQL:        nil,
			RequireSQL: true,
		}},
		{"sql-db-error", &PgRBACResolver{
			Fallback:   NewDefaultRoleResolver(),
			SQL:        &stubRBAC{err: ErrRBACResolverUnavailable},
			RequireSQL: true,
		}},
		{"sql-principal-unknown", &PgRBACResolver{
			Fallback:   NewDefaultRoleResolver(),
			SQL:        &stubRBAC{err: ErrPrincipalUnknown},
			RequireSQL: true,
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ok := HasCapability(context.Background(), c.resolver, Principal{
				Actor: "alice",
				Roles: []string{"net_admin"},
			}, CapabilityDestructiveExecute)
			if ok {
				t.Errorf("HasCapability returned true under %s; expected fail-closed", c.name)
			}
		})
	}
}
