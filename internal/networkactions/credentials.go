package networkactions

import (
	"context"
	"errors"
	"strings"
	"time"
)

// CredentialResolver resolves the SSH credentials that an action
// must use to reach a target device. Phase 9 v3 lifted this from
// the implicit "everyone reuses Dude credentials" pattern into an
// explicit interface so the action runner can:
//
//   - swap in a per-device SecretProvider in the future without
//     touching action code,
//   - return a typed ErrCredentialNotFound BEFORE any SSH dial when
//     a device truly has no usable credential profile,
//   - keep the Dude-reuse path as an explicit, named fallback that
//     is easy to audit.
//
// Implementations MUST NOT log, audit, or marshal the resolved
// password. The plaintext stays in the SSHTarget struct only for
// the lifetime of one action and never crosses the API/DB
// boundary.
type CredentialResolver interface {
	// Resolve returns SSH credentials suitable for `host`. deviceID
	// is the network_devices.id when known (empty for raw-host
	// invocations). Implementations may consult the SecretProvider
	// for the password material.
	Resolve(ctx context.Context, deviceID, host string) (SSHTarget, error)
}

// SecretProvider is the abstraction that hides where a credential
// secret comes from (vault, keyring, env). Phase 9 v3 ships a
// memory-backed implementation (used by tests) and a DudeStaticProvider
// that reuses the existing Dude SSH credentials for backward
// compatibility.
type SecretProvider interface {
	// Lookup returns the password (or empty + ErrCredentialNotFound
	// when no credential exists for this profile). Implementations
	// MUST NOT log the value.
	Lookup(ctx context.Context, profileID string) (string, error)
}

// ErrCredentialNotFound is returned when no credential profile is
// available for the requested target. Callers MUST surface this as
// a stable error_code and MUST NOT attempt an SSH dial.
var ErrCredentialNotFound = errors.New("networkactions: credential_not_found")

// ErrInvalidTargetHost is returned by ValidateTargetHost when the
// caller-provided host is not a syntactically valid IPv4/IPv6/
// hostname. Callers MUST translate this to HTTP 400 BEFORE any
// DB inet cast or SSH attempt.
var ErrInvalidTargetHost = errors.New("networkactions: invalid_target_host")

// MemorySecretProvider is a hermetic in-memory SecretProvider for
// tests. NOT for production: provides plaintext-by-key lookup with
// no encryption. Callers should use a vault-backed provider in a
// real deployment.
type MemorySecretProvider struct {
	Secrets map[string]string
}

func NewMemorySecretProvider(initial map[string]string) *MemorySecretProvider {
	out := &MemorySecretProvider{Secrets: map[string]string{}}
	for k, v := range initial {
		out.Secrets[k] = v
	}
	return out
}

// Lookup implements SecretProvider.
func (m *MemorySecretProvider) Lookup(_ context.Context, profileID string) (string, error) {
	if v, ok := m.Secrets[profileID]; ok && v != "" {
		return v, nil
	}
	return "", ErrCredentialNotFound
}

// DudeStaticProfile is the fixed credential profile id reserved for
// the Dude SSH reuse fallback path. Audit/log entries name the
// profile by id, NEVER by secret value.
const DudeStaticProfile = "dude_static_admin"

// DudeFallbackResolver wraps a SecretProvider that is preloaded with
// the Dude admin credentials. It is the Phase 9 v3 backward-compat
// path: if the operator has not yet attached a per-device
// credential profile, the resolver hands back the Dude admin
// credentials (the same ones Phase 9 / 9 v2 used implicitly), but
// records that fact via Profile=DudeStaticProfile so the audit log
// can show which credential bucket was actually used.
type DudeFallbackResolver struct {
	Provider           SecretProvider
	Username           string
	Port               int
	HostKeyPolicy      string
	HostKeyFingerprint string
	Timeout            time.Duration
}

// Resolve implements CredentialResolver.
//
// Phase 9 v3 contract: every Resolve call goes through SecretProvider
// so per-device profiles can be slotted in later without touching
// action code. When the provider returns ErrCredentialNotFound the
// resolver bubbles up the typed error — the caller MUST NOT dial.
func (r *DudeFallbackResolver) Resolve(ctx context.Context, deviceID, host string) (SSHTarget, error) {
	username := strings.TrimSpace(r.Username)
	if username == "" {
		return SSHTarget{}, ErrNotConfigured
	}
	pwd, err := r.Provider.Lookup(ctx, DudeStaticProfile)
	if err != nil {
		return SSHTarget{}, err
	}
	if pwd == "" {
		return SSHTarget{}, ErrCredentialNotFound
	}
	return SSHTarget{
		Host:               strings.TrimSpace(host),
		Port:               r.Port,
		Username:           username,
		Password:           pwd,
		Timeout:            r.Timeout,
		HostKeyPolicy:      r.HostKeyPolicy,
		HostKeyFingerprint: r.HostKeyFingerprint,
	}, nil
}
