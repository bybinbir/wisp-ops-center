// Package networkactions Phase 10 pre-gate.
//
// This file is the single source of truth for what a destructive
// action MUST satisfy BEFORE it is allowed to run. Phase 9 v3 ships
// the gate without enabling any destructive action — the gate is
// pure scaffolding so Phase 10 can wire its frequency_correction
// implementation through `EnsureDestructiveAllowed` without
// inventing the contract on the spot.
//
// Every guardrail Phase 10 needs is enumerated below as a typed
// requirement. Calling EnsureDestructiveAllowed with a request that
// fails ANY requirement returns the typed error so the action layer
// can surface a stable error_code without leaking detail.

package networkactions

import (
	"context"
	"errors"
	"strings"
	"time"
)

// Destructive pre-gate sentinels. Each maps to a stable error_code
// the API + audit layer can surface without leaking input detail.
var (
	ErrDestructiveDisabled      = errors.New("networkactions: destructive_disabled")
	ErrDryRunRequired           = errors.New("networkactions: dry_run_required")
	ErrIntentNotConfirmed       = errors.New("networkactions: intent_not_confirmed")
	ErrMaintenanceWindowMissing = errors.New("networkactions: maintenance_window_missing")
	ErrMaintenanceWindowClosed  = errors.New("networkactions: maintenance_window_closed")
	ErrRollbackNoteMissing      = errors.New("networkactions: rollback_note_missing")
	ErrRBACDenied               = errors.New("networkactions: rbac_denied")
	ErrIdempotencyKeyMissing    = errors.New("networkactions: idempotency_key_missing")
	// Phase 10A — provider-level failures.
	ErrToggleProviderRequired = errors.New("networkactions: toggle_provider_required")
	ErrRBACProviderRequired   = errors.New("networkactions: rbac_provider_required")
	ErrWindowProviderRequired = errors.New("networkactions: window_provider_required")
)

// DestructiveRequest is the input shape Phase 10 callers will use.
// The API layer fills it from the request body + session context.
type DestructiveRequest struct {
	Kind           Kind
	DeviceID       string
	Actor          string
	ActorRoles     []string
	DryRun         bool
	Confirm        bool               // explicit operator intent
	RollbackNote   string             // operator-written rollback plan
	IdempotencyKey string             // dedupe key per device + intent
	Window         *MaintenanceWindow // [start, end) UTC
	Now            time.Time          // injectable clock for tests
}

// AllowedRoles is the RBAC role list authorized to invoke any
// destructive action in production. Phase 10 will read this from
// config/CRD; Phase 9 v3 ships the constant set so the pre-gate has
// something to assert against.
var AllowedRolesForDestructive = []string{"net_ops", "net_admin"}

// DestructiveActionEnabled is the master switch. Phase 9 v3 hard-
// codes it to false so frequency_correction (and any future
// destructive Kind) cannot run, no matter what the request claims.
// Phase 10 will flip this via build flag or operator-controlled
// runtime toggle gated by the rest of the pre-gate.
var DestructiveActionEnabled = false

// EnsureDestructiveAllowed runs every Phase 10 guardrail in order
// and returns the FIRST failing typed error. A nil return means the
// caller may proceed. Phase 9 v3: even a perfectly-shaped request
// will fail at the master switch (DestructiveActionEnabled=false).
func EnsureDestructiveAllowed(ctx context.Context, req DestructiveRequest) error {
	// 1. Master switch.
	if !DestructiveActionEnabled {
		return ErrDestructiveDisabled
	}
	// 2. Kind must actually be a destructive surface.
	if !req.Kind.IsDestructive() {
		return nil // non-destructive paths bypass the gate
	}
	// 3. RBAC.
	if !hasAnyRole(req.ActorRoles, AllowedRolesForDestructive) {
		return ErrRBACDenied
	}
	// 4. dry_run default true unless caller explicitly opted out via
	//    Confirm=true. The gate refuses live execution without
	//    explicit confirmation.
	if !req.Confirm {
		return ErrIntentNotConfirmed
	}
	// 5. Maintenance window must exist + be open at Now.
	if req.Window == nil {
		return ErrMaintenanceWindowMissing
	}
	now := req.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if now.Before(req.Window.Start) || !now.Before(req.Window.End) {
		return ErrMaintenanceWindowClosed
	}
	// 6. Rollback plan present (any non-empty trimmed string).
	if strings.TrimSpace(req.RollbackNote) == "" {
		return ErrRollbackNoteMissing
	}
	// 7. Idempotency key required so a duplicate POST cannot
	//    re-execute the same destructive intent.
	if strings.TrimSpace(req.IdempotencyKey) == "" {
		return ErrIdempotencyKeyMissing
	}
	return nil
}

// DestructiveProviders bundles the operator-controlled stores the
// Phase 10A pre-gate consults. Pass concrete implementations
// (MemoryToggle, StaticRoleResolver, MemoryMaintenanceStore) for
// hermetic tests; production wires Postgres-backed equivalents.
//
// Every field is required for live execution. The gate
// fail-closes when any field is nil.
type DestructiveProviders struct {
	Toggle      DestructiveToggle
	RBAC        RBACResolver
	Maintenance MaintenanceProvider
}

// EnsureDestructiveAllowedWithProviders is the Phase 10A successor
// to EnsureDestructiveAllowed. It performs the same guardrails but
// reads each invariant from an explicit provider, never from
// process-global state. The legacy helper is preserved for
// backward-compat with Phase 9 tests.
//
// Order (each step returns the FIRST failing typed error):
//
//  1. providers != nil
//  2. Toggle.Enabled == true                                (master switch)
//  3. Kind.IsDestructive()                                  (non-destructive bypass)
//  4. RBAC: principal holds CapabilityDestructiveExecute    (RBAC)
//  5. req.Confirm == true                                   (explicit intent)
//  6. Maintenance.ActiveAt returns >=1 window               (maintenance window)
//  7. RollbackNote non-empty                                (rollback)
//  8. IdempotencyKey non-empty                              (idempotency)
//
// dry_run requests still flow through this gate; callers MUST
// continue to set DryRun=true unless every guardrail returns nil.
func EnsureDestructiveAllowedWithProviders(
	ctx context.Context,
	providers *DestructiveProviders,
	req DestructiveRequest,
) error {
	if providers == nil {
		return ErrToggleProviderRequired
	}
	if providers.Toggle == nil {
		return ErrToggleProviderRequired
	}
	enabled, err := providers.Toggle.Enabled(ctx)
	if err != nil || !enabled {
		return ErrDestructiveDisabled
	}
	if !req.Kind.IsDestructive() {
		return nil
	}
	if providers.RBAC == nil {
		return ErrRBACProviderRequired
	}
	principal := Principal{Actor: req.Actor, Roles: req.ActorRoles}
	if !HasCapability(ctx, providers.RBAC, principal, CapabilityDestructiveExecute) {
		return ErrRBACDenied
	}
	if !req.Confirm {
		return ErrIntentNotConfirmed
	}
	if providers.Maintenance == nil {
		return ErrWindowProviderRequired
	}
	now := req.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	wins, werr := providers.Maintenance.ActiveAt(ctx, req.DeviceID, now)
	if werr != nil {
		return ErrMaintenanceWindowMissing
	}
	if len(wins) == 0 {
		return ErrMaintenanceWindowClosed
	}
	if strings.TrimSpace(req.RollbackNote) == "" {
		return ErrRollbackNoteMissing
	}
	if strings.TrimSpace(req.IdempotencyKey) == "" {
		return ErrIdempotencyKeyMissing
	}
	return nil
}

// PreGateChecklist returns the list of guardrails the gate enforces
// today. Used by docs/TASK_BOARD and the API pre-flight endpoint so
// operators can see exactly what Phase 10 must satisfy.
func PreGateChecklist() []string {
	return []string{
		"destructive_master_switch",
		"rbac_role",
		"explicit_intent_confirmation",
		"maintenance_window_open",
		"rollback_note_required",
		"idempotency_key_required",
		"dry_run_default_true",
		"per_device_lock",    // Registry.AcquireLock
		"rate_limit",         // Registry.CheckRate
		"panic_recovery",     // handler runActionAsync defer
		"mutation_deny_list", // EnsureCommandAllowed
		"secret_redaction",   // SanitizeAttrs/Message/ResultMap
		"mac_masking",        // maskMAC in extractClients
		"audit_event_for_every_phase",
	}
}

func hasAnyRole(have, allowed []string) bool {
	for _, h := range have {
		for _, a := range allowed {
			if strings.EqualFold(strings.TrimSpace(h), a) {
				return true
			}
		}
	}
	return false
}

// DestructiveErrorCode maps the pre-gate sentinels to short, stable
// labels suitable for the API and audit log. Useful for the eventual
// Phase 10 endpoint that will surface guardrail failures.
func DestructiveErrorCode(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrDestructiveDisabled):
		return "destructive_disabled"
	case errors.Is(err, ErrDryRunRequired):
		return "dry_run_required"
	case errors.Is(err, ErrIntentNotConfirmed):
		return "intent_not_confirmed"
	case errors.Is(err, ErrMaintenanceWindowMissing):
		return "maintenance_window_missing"
	case errors.Is(err, ErrMaintenanceWindowClosed):
		return "maintenance_window_closed"
	case errors.Is(err, ErrRollbackNoteMissing):
		return "rollback_note_missing"
	case errors.Is(err, ErrRBACDenied):
		return "rbac_denied"
	case errors.Is(err, ErrIdempotencyKeyMissing):
		return "idempotency_key_missing"
	default:
		return "unknown"
	}
}
