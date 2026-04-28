package networkactions

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgRoleResolver looks up actor → role bindings in
// `network_action_role_grants`. Phase 10C ships this so destructive
// authorization no longer leans on header-supplied roles for any
// production deployment that has seeded the table.
//
// IMPORTANT contract:
//   - nil pool → ErrRBACResolverUnavailable (fail-closed).
//   - DB error → ErrRBACResolverUnavailable (fail-closed; the
//     calling HasCapability helper turns this into a deny).
//   - Empty actor → ErrPrincipalUnknown (deny).
//   - Actor not in table:
//   - RequireSQL=false → ErrPrincipalUnknown (caller chooses
//     whether to fall back to header roles).
//   - RequireSQL=true  → ErrPrincipalUnknown (deny, no fallback).
//   - Actor in table → roles from DB drive the static capability
//     mapping. Header roles are IGNORED to prevent privilege
//     escalation via spoofed headers.
type PgRoleResolver struct {
	P *pgxpool.Pool
	// CapabilityMap is the role → capability map applied AFTER the
	// SQL lookup returns the role list. Defaults to the static
	// production mapping (NewDefaultRoleResolver) when nil.
	CapabilityMap *StaticRoleResolver
}

// ErrPrincipalUnknown is returned when the SQL grants table has no
// row for the principal. Callers (PgRBACResolver) decide whether to
// translate this into a deny or to fall back to a static resolver
// based on the RequireSQL flag.
var ErrPrincipalUnknown = errors.New("networkactions: principal_unknown")

// NewPgRoleResolver wires the resolver. When `caps` is nil, the
// production default mapping is used so empty wiring still produces
// a safe, enumerated capability set.
func NewPgRoleResolver(p *pgxpool.Pool, caps *StaticRoleResolver) *PgRoleResolver {
	if caps == nil {
		caps = NewDefaultRoleResolver()
	}
	return &PgRoleResolver{P: p, CapabilityMap: caps}
}

// LookupRoles returns the role list bound to an actor in the SQL
// grants table. It is the single point of contact with the DB for
// authorization, so every error path is fail-closed.
func (r *PgRoleResolver) LookupRoles(ctx context.Context, actor string) ([]string, time.Time, error) {
	actor = strings.TrimSpace(actor)
	if r == nil || r.P == nil {
		return nil, time.Time{}, ErrRBACResolverUnavailable
	}
	if actor == "" {
		return nil, time.Time{}, ErrPrincipalUnknown
	}
	row := r.P.QueryRow(ctx, `
SELECT roles, updated_at
FROM network_action_role_grants
WHERE actor = $1`, actor)
	var roles []string
	var updatedAt time.Time
	if err := row.Scan(&roles, &updatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, time.Time{}, ErrPrincipalUnknown
		}
		return nil, time.Time{}, ErrRBACResolverUnavailable
	}
	return roles, updatedAt, nil
}

// Capabilities implements RBACResolver. Always queries the SQL
// grants table for the principal's actor; header roles are not
// trusted. When the actor is missing from the table the resolver
// returns ErrPrincipalUnknown so the wrapper PgRBACResolver can
// honor the RequireSQL flag.
func (r *PgRoleResolver) Capabilities(ctx context.Context, p Principal) ([]Capability, error) {
	if r == nil || r.CapabilityMap == nil {
		return nil, ErrRBACResolverUnavailable
	}
	roles, _, err := r.LookupRoles(ctx, p.Actor)
	if err != nil {
		return nil, err
	}
	if len(roles) == 0 {
		return []Capability{}, nil
	}
	dbPrincipal := Principal{Actor: p.Actor, Roles: roles}
	caps, mapErr := r.CapabilityMap.Capabilities(ctx, dbPrincipal)
	if mapErr != nil {
		return nil, mapErr
	}
	return caps, nil
}

// SeedActor writes (or updates) a single grant row. Phase 10C uses
// this from tests + an internal admin endpoint (out-of-scope for
// this PR) to bootstrap a deployment. NEVER persists a secret.
func (r *PgRoleResolver) SeedActor(ctx context.Context, actor string, roles []string, grantedBy string, notes string) error {
	if r == nil || r.P == nil {
		return ErrRBACResolverUnavailable
	}
	actor = strings.TrimSpace(actor)
	grantedBy = strings.TrimSpace(grantedBy)
	if actor == "" {
		return errors.New("networkactions: seed_actor_empty")
	}
	if grantedBy == "" {
		return errors.New("networkactions: seed_actor_granted_by_required")
	}
	cleanRoles := make([]string, 0, len(roles))
	for _, role := range roles {
		role = strings.ToLower(strings.TrimSpace(role))
		if role == "" {
			continue
		}
		cleanRoles = append(cleanRoles, role)
	}
	_, err := r.P.Exec(ctx, `
INSERT INTO network_action_role_grants (actor, roles, granted_by, notes)
VALUES ($1, $2, $3, $4)
ON CONFLICT (actor) DO UPDATE
SET roles = EXCLUDED.roles,
    granted_by = EXCLUDED.granted_by,
    notes = EXCLUDED.notes,
    updated_at = now()`, actor, cleanRoles, grantedBy, notes)
	return err
}
