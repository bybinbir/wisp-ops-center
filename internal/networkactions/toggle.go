package networkactions

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
)

// DestructiveToggle is the operator-controlled, audit-logged switch
// that gates every destructive (`IsDestructive`) action. Phase 10A
// replaces the constant `DestructiveActionEnabled` global with this
// abstraction so:
//
//   - Production stays fail-closed by default (a freshly-booted
//     server returns Enabled=false from the canonical store until
//     an explicit, audit-logged Flip records "yes, I want
//     destructive actions live").
//   - A Flip CAN be reverted at any time without code changes;
//     state lives in the store, not the binary.
//   - Tests can swap in MemoryToggle to simulate an open switch
//     without touching a global var.
//
// IMPORTANT: implementations MUST persist the flip + actor + reason
// in a way that the audit layer can read; an in-memory store is
// acceptable for tests but NOT production.
type DestructiveToggle interface {
	// Enabled reports whether destructive actions are currently
	// allowed. Implementations MUST default to false on first
	// call when no operator has flipped the switch.
	Enabled(ctx context.Context) (bool, error)

	// Flip records an explicit operator decision. `enabled=true`
	// turns the master switch on; `enabled=false` turns it off.
	// Both transitions MUST emit an audit event so a security
	// reviewer can answer "who turned this on, when, why".
	Flip(ctx context.Context, enabled bool, actor, reason string) (FlipReceipt, error)
}

// FlipReceipt is the persisted record of one toggle flip. The audit
// layer copies these fields into an `network_action_toggle.flipped`
// event.
type FlipReceipt struct {
	Enabled   bool      `json:"enabled"`
	Actor     string    `json:"actor"`
	Reason    string    `json:"reason"`
	FlippedAt time.Time `json:"flipped_at"`
}

// ErrToggleStoreUnavailable indicates the operator-controlled store
// could not be reached. Phase 10A's contract is **fail-closed**:
// a store error MUST be treated by callers as Enabled=false.
var ErrToggleStoreUnavailable = errors.New("networkactions: toggle_store_unavailable")

// MemoryToggle is the canonical hermetic implementation. Tests use
// it; the API server can use it as a temporary backing store until a
// Postgres-backed implementation lands. Production deployments
// SHOULD swap in a persistent store (config + DB row) so the flip
// survives a restart. MemoryToggle is concurrent-safe.
type MemoryToggle struct {
	mu      sync.RWMutex
	enabled bool
	last    *FlipReceipt
}

// NewMemoryToggle returns a toggle in the **default-closed** state.
// No flip has happened yet; Enabled returns false.
func NewMemoryToggle() *MemoryToggle { return &MemoryToggle{} }

// Enabled implements DestructiveToggle.
func (t *MemoryToggle) Enabled(_ context.Context) (bool, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.enabled, nil
}

// Flip implements DestructiveToggle.
func (t *MemoryToggle) Flip(_ context.Context, enabled bool, actor, reason string) (FlipReceipt, error) {
	if strings.TrimSpace(actor) == "" {
		return FlipReceipt{}, errors.New("networkactions: toggle flip requires non-empty actor")
	}
	if strings.TrimSpace(reason) == "" {
		return FlipReceipt{}, errors.New("networkactions: toggle flip requires non-empty reason")
	}
	r := FlipReceipt{
		Enabled:   enabled,
		Actor:     actor,
		Reason:    reason,
		FlippedAt: time.Now().UTC(),
	}
	t.mu.Lock()
	t.enabled = enabled
	t.last = &r
	t.mu.Unlock()
	return r, nil
}

// LastFlip returns the most recent Flip receipt or nil if none has
// happened. Useful for the API surface that exposes "current
// destructive state" to the operator.
func (t *MemoryToggle) LastFlip() *FlipReceipt {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.last == nil {
		return nil
	}
	cp := *t.last
	return &cp
}

// IsDestructiveEnabled is a convenience helper: it consults the
// supplied DestructiveToggle and returns false on any error so the
// caller's contract stays fail-closed.
//
// IMPORTANT: this helper does NOT consult the legacy global
// `DestructiveActionEnabled`. Phase 10A explicitly migrates callers
// to the toggle interface; the global is kept only for backward
// compatibility with existing tests.
func IsDestructiveEnabled(ctx context.Context, t DestructiveToggle) bool {
	if t == nil {
		return false
	}
	enabled, err := t.Enabled(ctx)
	if err != nil || !enabled {
		return false
	}
	return true
}
