package networkactions

import "errors"

// DestructiveAuditAction enumerates the audit event names Phase 10A
// reserves for the destructive-action lifecycle. The action layer
// emits events under these names so a security reviewer can grep
// audit_logs for the exact phase that fired.
//
// IMPORTANT: every constant here MUST be a stable string literal —
// renaming one is a breaking change for downstream log consumers.
type DestructiveAuditAction string

const (
	// network_action.confirmed — operator passed the explicit-intent
	// guard. Emitted before any execution attempt.
	AuditActionConfirmed DestructiveAuditAction = "network_action.confirmed"
	// network_action.gate_fail — pre-gate refused the request. The
	// metadata MUST include error_code from DestructiveErrorCode.
	AuditActionGateFail DestructiveAuditAction = "network_action.gate_fail"
	// network_action.dry_run — dry-run flow completed successfully.
	AuditActionDryRunCompleted DestructiveAuditAction = "network_action.dry_run"
	// network_action.live_start_blocked — a request that ASKED for
	// live execution (DryRun=false) was refused by the master
	// switch or a pre-gate guardrail. Phase 10A guarantees this
	// fires and is the ONLY place the API can refuse a live
	// destructive request.
	AuditActionLiveStartBlocked DestructiveAuditAction = "network_action.live_start_blocked"
	// network_action.toggle_flipped — the master switch was turned
	// on/off. Metadata: enabled, actor, reason, flipped_at.
	AuditActionToggleFlipped DestructiveAuditAction = "network_action.toggle_flipped"
	// network_action.rbac_denied — the pre-gate denied because the
	// principal lacks the required capability.
	AuditActionRBACDenied DestructiveAuditAction = "network_action.rbac_denied"
	// network_action.maintenance_window_denied — the pre-gate denied
	// because no maintenance window is open at Now or none applies
	// to the target device.
	AuditActionMaintenanceWindowDenied DestructiveAuditAction = "network_action.maintenance_window_denied"
)

// AuditActionForGateError maps a Phase 10A pre-gate sentinel to the
// most specific DestructiveAuditAction. Callers that emit a
// network_action.gate_fail event SHOULD use this helper to choose
// a more specific action where one exists; the API can then surface
// targeted alerts (e.g. RBAC denials) without parsing error_code.
func AuditActionForGateError(err error) DestructiveAuditAction {
	switch {
	case errors.Is(err, ErrRBACDenied):
		return AuditActionRBACDenied
	case errors.Is(err, ErrMaintenanceWindowMissing),
		errors.Is(err, ErrMaintenanceWindowClosed),
		errors.Is(err, ErrWindowProviderRequired):
		return AuditActionMaintenanceWindowDenied
	default:
		return AuditActionGateFail
	}
}

// DestructiveAuditCatalog returns every audit action name Phase 10A
// reserves. Tests assert this set matches what TASK_BOARD documents
// so the catalog cannot drift unnoticed.
func DestructiveAuditCatalog() []DestructiveAuditAction {
	return []DestructiveAuditAction{
		AuditActionConfirmed,
		AuditActionGateFail,
		AuditActionDryRunCompleted,
		AuditActionLiveStartBlocked,
		AuditActionToggleFlipped,
		AuditActionRBACDenied,
		AuditActionMaintenanceWindowDenied,
	}
}
