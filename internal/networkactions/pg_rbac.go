package networkactions

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PgRBACResolver is the Postgres-backed RBAC boundary. Phase 10B
// shipped the typed seam; Phase 10C wires the SQL grants lookup so
// production authorization no longer trusts header-supplied roles.
//
// IMPORTANT contract:
//   - When the pool is nil, the resolver delegates to Fallback
//     (typically NewDefaultRoleResolver). Dev/test environments
//     without a wired RBAC schema still get a safe enumerated
//     capability set.
//   - When the pool is non-nil, the resolver consults
//     PgRoleResolver first. If the SQL lookup returns roles those
//     drive the capability map. Header roles are IGNORED for that
//     actor.
//   - When the SQL lookup reports the actor is unknown (no row in
//     network_action_role_grants):
//   - RequireSQL=false → fall back to the header-roles static
//     resolver. This keeps Phase 10B and earlier deployments
//     working until grants are seeded.
//   - RequireSQL=true → return ErrPrincipalUnknown. The
//     HasCapability helper translates this into a deny. This is
//     the production posture once an operator has finished
//     seeding the table.
//   - Any DB error → ErrRBACResolverUnavailable (fail-closed).
type PgRBACResolver struct {
	P        *pgxpool.Pool
	Fallback RBACResolver
	// SQL is the SQL-backed resolver consulted before Fallback. Held
	// as RBACResolver (rather than *PgRoleResolver) so tests can
	// swap in mocks without reaching into private state. Production
	// wiring uses *PgRoleResolver against the live grants table.
	SQL RBACResolver
	// RequireSQL flips the resolver into strict mode: an unknown
	// actor returns ErrPrincipalUnknown instead of falling back to
	// the static resolver. Operators flip this to true after seeding
	// network_action_role_grants. Default: false (Phase 10B parity).
	RequireSQL bool
}

// NewPgRBACResolver wires the resolver. If fallback is nil, a
// default static resolver is created internally so the seam is
// never silently fail-open.
func NewPgRBACResolver(p *pgxpool.Pool, fallback RBACResolver) *PgRBACResolver {
	if fallback == nil {
		fallback = NewDefaultRoleResolver()
	}
	r := &PgRBACResolver{P: p, Fallback: fallback}
	if p != nil {
		// Auto-wire the SQL resolver. The capability map is the same
		// static mapping the fallback uses so a role list coming
		// from the DB resolves to the same capability set the static
		// resolver would have produced for the same roles.
		caps := defaultCapabilityMap(fallback)
		r.SQL = NewPgRoleResolver(p, caps)
	}
	return r
}

// Capabilities implements RBACResolver. The lookup order is:
//
//  1. RequireSQL=true and SQL not wired → ErrRBACResolverUnavailable
//     (Phase 10F-A hardening: header roles MUST NOT authorise a
//     destructive call when the operator has explicitly demanded
//     SQL-backed RBAC).
//  2. SQL resolver wired. Found → those roles drive caps.
//  3. SQL says ErrPrincipalUnknown:
//     - RequireSQL=true  → return ErrPrincipalUnknown (deny).
//     - RequireSQL=false → fall back to Fallback (header roles).
//  4. SQL says any other error (DB outage, malformed actor) →
//     return that (deny). Never falls through to header roles.
//  5. SQL not wired and RequireSQL=false → Fallback.
//
// Either branch returns a non-nil error or a possibly-empty slice.
func (r *PgRBACResolver) Capabilities(ctx context.Context, p Principal) ([]Capability, error) {
	if r == nil || r.Fallback == nil {
		return nil, ErrRBACResolverUnavailable
	}
	// Phase 10F-A: RequireSQL=true is the operator's promise that
	// header roles are not authoritative for this deployment. If
	// the SQL seam is missing for any reason we MUST fail closed
	// rather than degrade silently to the static fallback.
	if r.RequireSQL && r.SQL == nil {
		return nil, ErrRBACResolverUnavailable
	}
	if r.SQL != nil {
		caps, err := r.SQL.Capabilities(ctx, p)
		if err == nil {
			return caps, nil
		}
		if errors.Is(err, ErrPrincipalUnknown) {
			if r.RequireSQL {
				return nil, ErrPrincipalUnknown
			}
			// fall through to header-based fallback
		} else {
			// Any other error (DB outage, resolver unavailable) is
			// a hard deny. We do NOT fall through to the static
			// resolver because that would let an attacker degrade
			// the DB connection to escalate via headers.
			return nil, err
		}
	}
	return r.Fallback.Capabilities(ctx, p)
}

// defaultCapabilityMap extracts a *StaticRoleResolver from an
// RBACResolver if possible; otherwise returns NewDefaultRoleResolver.
// This is best-effort: if the caller provides a custom fallback, the
// SQL resolver still needs a known role→capability mapping, so we
// fall back to the canonical default.
func defaultCapabilityMap(fallback RBACResolver) *StaticRoleResolver {
	if s, ok := fallback.(*StaticRoleResolver); ok && s != nil {
		return s
	}
	return NewDefaultRoleResolver()
}
