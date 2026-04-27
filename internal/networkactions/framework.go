// Package networkactions is the Phase 8 *scaffolding* for controlled
// network actions against discovered devices. This package
// intentionally contains NO destructive operations: it defines the
// types, interfaces and policy hooks that future phases will use to
// implement frequency correction, AP client tests, link signal
// tests, bridge health checks and scheduled maintenance windows.
//
// Calling Execute on any registered action returns
// ErrActionNotImplemented in Phase 8. The infrastructure exists so
// the dashboard, scheduler and audit log can reason about actions
// uniformly when later phases turn the feature on.
package networkactions

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ErrActionNotImplemented is returned by any Action.Execute call in
// Phase 8. It is a sentinel so tests + UI can detect "scaffolding-
// only" mode and surface a friendly message.
var ErrActionNotImplemented = errors.New("networkactions: action not implemented in phase 8")

// ErrActionDisabled is returned when policy refuses execution
// (maintenance window closed, dry-run only, etc.).
var ErrActionDisabled = errors.New("networkactions: action disabled by policy")

// ErrActionLocked is returned when the per-device lock is held.
var ErrActionLocked = errors.New("networkactions: device locked by another action")

// Kind enumerates the action surface area planned for later phases.
// Phase 8 only declares the names; nothing is wired to live device
// I/O.
type Kind string

const (
	KindFrequencyCheck      Kind = "frequency_check"
	KindFrequencyCorrection Kind = "frequency_correction"
	KindAPClientTest        Kind = "ap_client_test"
	KindLinkSignalTest      Kind = "link_signal_test"
	KindBridgeHealthCheck   Kind = "bridge_health_check"
	KindMaintenanceWindow   Kind = "maintenance_window"
)

// IsDestructive reports whether a Kind COULD modify device state if
// implemented. Used by the audit + UI layers to demand explicit
// confirmation in later phases.
func (k Kind) IsDestructive() bool {
	switch k {
	case KindFrequencyCorrection:
		return true
	default:
		return false
	}
}

// Request is the inbound action invocation, surfaced over the API
// or a scheduled job.
type Request struct {
	Kind          Kind
	DeviceID      string
	CorrelationID string
	DryRun        bool
	Confirm       bool
	Reason        string
	Actor         string
	Window        *MaintenanceWindow
}

// Result is the action's terminal state, recorded into the audit
// log. The shape is stable so later phases can reuse it.
type Result struct {
	Kind          Kind
	DeviceID      string
	CorrelationID string
	StartedAt     time.Time
	FinishedAt    time.Time
	Success       bool
	DryRun        bool
	ErrorCode     string
	Message       string
}

// MaintenanceWindow is a [start, end) interval during which an
// action *may* run. Phase 8 stores nothing yet; the type is here so
// the API contract is stable.
type MaintenanceWindow struct {
	Start time.Time
	End   time.Time
}

// Action is the interface every concrete action will implement in
// later phases. Phase 8 ships a no-op stub for each Kind via the
// registry; calling Execute returns ErrActionNotImplemented.
type Action interface {
	Kind() Kind
	Execute(ctx context.Context, req Request) (Result, error)
}

// Registry holds the available action implementations + cross-
// cutting policy state (per-device locks, rate limiter).
type Registry struct {
	mu       sync.Mutex
	actions  map[Kind]Action
	locks    map[string]time.Time // deviceID -> lock acquired at
	rateGate *RateLimiter
}

// NewRegistry returns a registry pre-populated with stub actions
// for every Kind. Replace stubs with real implementations in later
// phases via Register.
func NewRegistry() *Registry {
	r := &Registry{
		actions:  map[Kind]Action{},
		locks:    map[string]time.Time{},
		rateGate: NewRateLimiter(10, time.Minute),
	}
	for _, k := range []Kind{
		KindFrequencyCheck, KindFrequencyCorrection,
		KindAPClientTest, KindLinkSignalTest,
		KindBridgeHealthCheck, KindMaintenanceWindow,
	} {
		r.actions[k] = stubAction{kind: k}
	}
	return r
}

// Register replaces the action for kind. Used by later phases when
// real implementations land.
func (r *Registry) Register(a Action) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.actions[a.Kind()] = a
}

// Get returns the action for kind (or nil).
func (r *Registry) Get(k Kind) Action {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.actions[k]
}

// AcquireLock attempts a per-device exclusive lock. Returns
// ErrActionLocked if held. Caller must Release on completion.
func (r *Registry) AcquireLock(deviceID string) error {
	if deviceID == "" {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, held := r.locks[deviceID]; held {
		return ErrActionLocked
	}
	r.locks[deviceID] = time.Now().UTC()
	return nil
}

// ReleaseLock frees a lock acquired by AcquireLock. Idempotent.
func (r *Registry) ReleaseLock(deviceID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.locks, deviceID)
}

// CheckRate returns nil when the call may proceed. Phase 8 uses a
// simple in-memory token bucket; later phases can swap in Redis.
func (r *Registry) CheckRate() error {
	if r.rateGate.Allow() {
		return nil
	}
	return ErrActionDisabled
}

// stubAction is the Phase 8 placeholder. Calling Execute returns
// ErrActionNotImplemented and writes nothing to the device.
type stubAction struct{ kind Kind }

func (s stubAction) Kind() Kind { return s.kind }

func (s stubAction) Execute(ctx context.Context, req Request) (Result, error) {
	return Result{
		Kind:          s.kind,
		DeviceID:      req.DeviceID,
		CorrelationID: req.CorrelationID,
		StartedAt:     time.Now().UTC(),
		FinishedAt:    time.Now().UTC(),
		Success:       false,
		DryRun:        req.DryRun,
		ErrorCode:     "not_implemented",
		Message:       "Phase 8 ships scaffolding only; real action in a later phase.",
	}, ErrActionNotImplemented
}
