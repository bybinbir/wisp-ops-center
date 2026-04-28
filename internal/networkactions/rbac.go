package networkactions

import (
	"context"
	"errors"
	"strings"
)

// Capability is a fine-grained permission name the action layer
// checks before allowing a destructive operation. Phase 10A does
// NOT plumb capabilities through the API yet; the values exist so
// future Phase 10 work has stable strings to attach to roles.
type Capability string

const (
	// CapabilityDestructiveExecute is the master capability required
	// to invoke any IsDestructive action (live, not dry-run).
	CapabilityDestructiveExecute Capability = "network_action.destructive.execute"
	// CapabilityDestructiveDryRun is required to even submit a
	// dry-run that targets a destructive Kind. Operators with read-
	// only roles MUST NOT see destructive Kinds enumerated.
	CapabilityDestructiveDryRun Capability = "network_action.destructive.dryrun"
	// CapabilityToggleFlip is required to flip the master toggle.
	CapabilityToggleFlip Capability = "network_action.toggle.flip"
	// CapabilityMaintenanceManage is required to create/disable
	// maintenance windows. Read-only listing only requires
	// CapabilityPreflightRead.
	CapabilityMaintenanceManage Capability = "network_action.maintenance.manage"
	// CapabilityPreflightRead is required to view the destructive
	// preflight status (current toggle + active windows + checklist).
	// Phase 10B exposes this through GET /api/v1/network/actions/preflight.
	CapabilityPreflightRead Capability = "network_action.preflight.read"
)

// RBACResolver looks up the capabilities a principal holds. Phase
// 10A ships a static, in-memory implementation
// (`StaticRoleResolver`) that maps role names to capability sets.
// A Postgres-backed resolver can replace it later without touching
// the gate.
type RBACResolver interface {
	// Capabilities returns the capabilities the principal currently
	// holds. Implementations MUST return a non-nil (possibly empty)
	// slice; the action layer treats nil as "deny".
	Capabilities(ctx context.Context, p Principal) ([]Capability, error)
}

// Principal is the security subject the RBAC resolver evaluates.
// The API layer fills it from the authenticated session.
type Principal struct {
	// Actor is a stable identifier for audit (username, service id).
	Actor string
	// Roles is the role list claimed by the session. The resolver
	// uses these to look up capabilities; the action layer NEVER
	// reads roles directly so authorization decisions are centralized.
	Roles []string
}

// ErrRBACResolverUnavailable indicates a resolver lookup failed.
// Phase 10A's contract is fail-closed: a resolver error MUST be
// treated as deny. Tests can construct this sentinel directly.
var ErrRBACResolverUnavailable = errors.New("networkactions: rbac_resolver_unavailable")

// StaticRoleResolver maps a role name to a capability set. The map
// is read-only after construction; concurrent reads are safe.
type StaticRoleResolver struct {
	// RoleCapabilities maps lowercased role -> capability list.
	RoleCapabilities map[string][]Capability
}

// NewDefaultRoleResolver returns a resolver with the conservative
// default mapping the platform ships:
//
//   - net_admin  → toggle flip + destructive execute + dry-run +
//     maintenance manage + preflight read
//   - net_ops    → destructive execute + dry-run + maintenance
//     manage + preflight read (no toggle flip)
//   - net_viewer → preflight read only
//
// Phase 10B consumers should use this resolver until a real RBAC
// store lands. Tests typically construct StaticRoleResolver inline
// to exercise edge cases. The Postgres-backed PgRBACResolver wraps
// a real role store and falls back to a static base for unmapped
// roles.
func NewDefaultRoleResolver() *StaticRoleResolver {
	return &StaticRoleResolver{
		RoleCapabilities: map[string][]Capability{
			"net_admin": {
				CapabilityToggleFlip,
				CapabilityDestructiveExecute,
				CapabilityDestructiveDryRun,
				CapabilityMaintenanceManage,
				CapabilityPreflightRead,
			},
			"net_ops": {
				CapabilityDestructiveExecute,
				CapabilityDestructiveDryRun,
				CapabilityMaintenanceManage,
				CapabilityPreflightRead,
			},
			"net_viewer": {
				CapabilityPreflightRead,
			},
		},
	}
}

// Capabilities implements RBACResolver. Empty role list →
// empty capability list (deny). Unknown roles are ignored
// (not matched against any capability) so a typo does not
// accidentally grant access.
func (s *StaticRoleResolver) Capabilities(_ context.Context, p Principal) ([]Capability, error) {
	if s == nil || s.RoleCapabilities == nil {
		return nil, ErrRBACResolverUnavailable
	}
	seen := map[Capability]struct{}{}
	out := make([]Capability, 0)
	for _, role := range p.Roles {
		caps, ok := s.RoleCapabilities[strings.ToLower(strings.TrimSpace(role))]
		if !ok {
			continue
		}
		for _, c := range caps {
			if _, dup := seen[c]; dup {
				continue
			}
			seen[c] = struct{}{}
			out = append(out, c)
		}
	}
	return out, nil
}

// HasCapability is a small helper: fail-closed wrapper around the
// resolver. Returns false on any resolver error so callers cannot
// accidentally fail-open under a transient store outage.
func HasCapability(ctx context.Context, r RBACResolver, p Principal, want Capability) bool {
	if r == nil {
		return false
	}
	caps, err := r.Capabilities(ctx, p)
	if err != nil || len(caps) == 0 {
		return false
	}
	for _, c := range caps {
		if c == want {
			return true
		}
	}
	return false
}
