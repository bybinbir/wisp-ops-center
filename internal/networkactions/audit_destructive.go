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
	// network_action.idempotency_reused — Phase 10C. A destructive
	// request landed with an idempotency_key matching an existing
	// run. The handler returned the original run id without
	// reaching the gate or Execute path.
	AuditActionIdempotencyReused DestructiveAuditAction = "network_action.idempotency_reused"
	// network_action.rollback_metadata_recorded — Phase 10C. The
	// destructive request's rollback note + intent were persisted on
	// the run row before the gate ran. Emitted exactly once per
	// run so an auditor can match every destructive intent to a
	// rollback plan.
	AuditActionRollbackMetadataRecorded DestructiveAuditAction = "network_action.rollback_metadata_recorded"
	// network_action.destructive_denied — Phase 10C. Terminal event
	// emitted when the destructive runtime refused a request: the
	// gate failed, the master switch is closed, or live execution
	// was requested. Mirrors `gate_fail` / `live_start_blocked` at
	// a higher level so a single grep can find every refused
	// destructive intent.
	AuditActionDestructiveDenied DestructiveAuditAction = "network_action.destructive_denied"
	// network_action.execute_attempted — Phase 10D. Emitted in the
	// runner immediately BEFORE action.Execute is called on a
	// destructive Kind that passed the pre-gate (toggle ON +
	// window active + RBAC granted + confirm=true). The audit
	// row pins the moment execution authority transferred from
	// the gate to the registered action. Phase 10D registry stubs
	// always return ErrActionNotImplemented, so this event is
	// followed by ExecuteNotImplemented in every Phase 10D run.
	AuditActionExecuteAttempted DestructiveAuditAction = "network_action.execute_attempted"
	// network_action.execute_not_implemented — Phase 10D. Emitted
	// in the runner immediately AFTER action.Execute returned
	// ErrActionNotImplemented. Terminal status for the run is
	// failed with error_code=action_not_implemented. This is the
	// only success-shaped path through the gate that Phase 10D
	// permits; reaching a non-NotImplemented Execute return is a
	// Phase 10D invariant violation.
	AuditActionExecuteNotImplemented DestructiveAuditAction = "network_action.execute_not_implemented"
	// network_action.execute_started — Phase 10E. Emitted by the
	// concrete action AFTER it has resolved the device + read its
	// pre-write snapshot, immediately BEFORE the first write goes
	// out. Metadata MUST include target_host, interface, and the
	// snapshot value the rollback path will restore to.
	AuditActionExecuteStarted DestructiveAuditAction = "network_action.execute_started"
	// network_action.execute_write_succeeded — Phase 10E. Emitted
	// by the concrete action AFTER the device returned a success
	// reply for the destructive write. The change is on the wire
	// at this point but NOT yet verified by a read-back; do not
	// treat this as the terminal happy event.
	AuditActionExecuteWriteSucceeded DestructiveAuditAction = "network_action.execute_write_succeeded"
	// network_action.execute_write_failed — Phase 10E. Emitted by
	// the concrete action when the write itself failed (SSH error,
	// device-side parse rejection, or the destructive_write
	// allowlist refused the command). Terminal status: failed.
	// No rollback is attempted: there is no successful write to
	// reverse.
	AuditActionExecuteWriteFailed DestructiveAuditAction = "network_action.execute_write_failed"
	// network_action.execute_verified — Phase 10E. Terminal happy
	// event. Emitted AFTER the post-write read-back returns a
	// snapshot whose target field matches the requested value.
	AuditActionExecuteVerified DestructiveAuditAction = "network_action.execute_verified"
	// network_action.execute_verification_failed — Phase 10E.
	// Emitted when the post-write read-back returned a value that
	// does NOT match the request. Caller falls through to the
	// rollback path; this event is non-terminal — exactly one of
	// rollback_succeeded / rollback_failed must follow.
	AuditActionExecuteVerificationFailed DestructiveAuditAction = "network_action.execute_verification_failed"
	// network_action.execute_rollback_started — Phase 10E. Emitted
	// at the start of the rollback write (issuing the snapshot
	// value back to the device). Always preceded by
	// execute_verification_failed.
	AuditActionExecuteRollbackStarted DestructiveAuditAction = "network_action.execute_rollback_started"
	// network_action.execute_rollback_succeeded — Phase 10E.
	// Terminal event. The rollback write completed AND the
	// re-verify read returned the snapshot value. Run row status:
	// failed with error_code=verification_failed_rollback_recovered.
	AuditActionExecuteRollbackSucceeded DestructiveAuditAction = "network_action.execute_rollback_succeeded"
	// network_action.execute_rollback_failed — Phase 10E. Terminal
	// event AND incident anchor. The rollback write failed or the
	// re-verify did not match the snapshot. Operator MUST review
	// the device manually; the destructive runtime cannot reason
	// about the on-wire state from here.
	AuditActionExecuteRollbackFailed DestructiveAuditAction = "network_action.execute_rollback_failed"
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
// reserves plus the Phase 10C lifecycle events, the Phase 10D execute-
// path events, and the Phase 10E real-Execute events. Tests assert
// this set matches what TASK_BOARD documents so the catalog cannot
// drift unnoticed.
func DestructiveAuditCatalog() []DestructiveAuditAction {
	return []DestructiveAuditAction{
		AuditActionConfirmed,
		AuditActionGateFail,
		AuditActionDryRunCompleted,
		AuditActionLiveStartBlocked,
		AuditActionToggleFlipped,
		AuditActionRBACDenied,
		AuditActionMaintenanceWindowDenied,
		AuditActionIdempotencyReused,
		AuditActionRollbackMetadataRecorded,
		AuditActionDestructiveDenied,
		AuditActionExecuteAttempted,
		AuditActionExecuteNotImplemented,
		AuditActionExecuteStarted,
		AuditActionExecuteWriteSucceeded,
		AuditActionExecuteWriteFailed,
		AuditActionExecuteVerified,
		AuditActionExecuteVerificationFailed,
		AuditActionExecuteRollbackStarted,
		AuditActionExecuteRollbackSucceeded,
		AuditActionExecuteRollbackFailed,
	}
}
