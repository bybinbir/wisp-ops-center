package networkactions

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PgRBACResolver is the Postgres-backed RBAC boundary. Phase 10B
// ships the type + interface seam so Phase 10C can drop in a real
// role/capability store without touching the gate or the API
// handlers. Today it ALWAYS delegates to the static fallback so
// production deployments behave identically to dev/test until the
// real role store lands.
//
// IMPORTANT contract:
//   - When the supplied pgxpool.Pool is nil, the resolver delegates
//     to Fallback (typically NewDefaultRoleResolver). This means
//     environments without a wired RBAC schema still get a safe,
//     enumerated capability set.
//   - When the pool is non-nil, the current implementation STILL
//     falls back to the static resolver (Phase 10B contract: no
//     real role store yet). The hook exists; Phase 10C swaps in
//     the SQL lookup.
//   - Any future SQL error MUST surface as fail-closed: a deny.
type PgRBACResolver struct {
	P        *pgxpool.Pool
	Fallback RBACResolver
}

// NewPgRBACResolver wires the resolver. If fallback is nil, a
// default static resolver is created internally so the seam is
// never silently fail-open.
func NewPgRBACResolver(p *pgxpool.Pool, fallback RBACResolver) *PgRBACResolver {
	if fallback == nil {
		fallback = NewDefaultRoleResolver()
	}
	return &PgRBACResolver{P: p, Fallback: fallback}
}

// Capabilities implements RBACResolver. Phase 10B contract: ALWAYS
// delegate to the fallback; the SQL hook is reserved for Phase 10C.
// This keeps behavior identical to the merged Phase 10A surface
// while introducing the type the API server will hold long-term.
func (r *PgRBACResolver) Capabilities(ctx context.Context, p Principal) ([]Capability, error) {
	if r == nil || r.Fallback == nil {
		return nil, ErrRBACResolverUnavailable
	}
	// Phase 10C will replace this body with a SELECT against the
	// roles + role_capabilities tables, layered on top of the
	// fallback.
	return r.Fallback.Capabilities(ctx, p)
}
