package networkactions

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Phase 10E — real frequency_correction Execute (lab-only).
//
// This file replaces the Phase 8 stubAction for KindFrequencyCorrection
// with a concrete Execute that:
//
//   1. Reads a snapshot of the current frequency on the target wireless
//      interface (rollback anchor).
//   2. Issues the destructive write through the mikrotik adapter's
//      destructive write allowlist (single path: /interface/wireless/set).
//   3. Re-reads the frequency to verify the device accepted the value.
//   4. On verification mismatch, automatically issues the rollback write
//      (snapshot value back) and re-verifies.
//
// What this file does NOT do:
//   - It does NOT flip either master switch (legacy const +
//     provider toggle). Those stay closed; the runner's pre-gate
//     enforces them.
//   - It does NOT decide which device to target. The handler resolves
//     the device + target frequency from the request body BEFORE
//     calling Execute; this action only drives the device once it
//     has a typed FrequencyCorrectionRequest.
//   - It does NOT touch any non-frequency field on the wireless
//     interface (SSID, passphrase, mode). The destructive_write
//     allowlist in the mikrotik adapter rejects any extra arg.
//   - It does NOT recover from a rollback_failed terminal state.
//     That is an incident anchor for the operator to review the
//     device manually.

// FrequencyCorrectionRequest is the typed payload Execute consumes.
// The HTTP handler is responsible for shaping the request body into
// this struct; the action does not parse free-form operator text.
type FrequencyCorrectionRequest struct {
	// DeviceID is the canonical id of the target (UUID from the
	// network_devices table). Used only for audit metadata + the
	// per-device lock; the actual device handle is opaque to this
	// package and lives behind FrequencyCorrectionWriter.
	DeviceID string
	// Interface is the RouterOS interface name (e.g. "wlan1"). The
	// destructive write allowlist requires it as a "number=" arg.
	Interface string
	// TargetFrequencyMHz is the frequency the operator wants applied.
	// MUST be > 0; the runner does not validate the value against a
	// regulatory band — that is upstream policy.
	TargetFrequencyMHz int
	// CorrelationID flows from the gate's correlation_id and is
	// echoed into every audit event so an auditor can stitch the
	// pre-gate rows together with the in-flight Execute rows.
	CorrelationID string
	// RunID is the network_action_runs row id for this Execute.
	// Carried so the action's own audit events use the same
	// `subject` as the Phase 10C/10D pre-gate events.
	RunID string
	// Actor is the principal that authorized the destructive run
	// (from the gate's principal). Echoed into audit metadata.
	Actor string
	// Intent is the operator-supplied free-form rationale. The audit
	// layer redacts secret-like substrings before persistence.
	Intent string
}

// FrequencyCorrectionWriter is the device-side seam Execute uses.
// In production it is satisfied by the mikrotik adapter
// (SSHClient.ExecRead + ExecWrite); in tests it is a fake that
// drives the action's lifecycle deterministically.
//
// All three methods receive ctx so the action can honour the
// runner's timeout. The implementation MUST NOT swallow context
// cancellation; returning ctx.Err() is the expected behaviour.
type FrequencyCorrectionWriter interface {
	// SnapshotFrequency reads the current operational frequency on
	// the named interface. Returns ErrFrequencyUnreadable if the
	// device replied but the value could not be parsed; returns
	// ErrDeviceUnreachable on dial / IO failure.
	SnapshotFrequency(ctx context.Context, deviceID, ifaceName string) (mhz int, err error)
	// SetFrequency issues the destructive write. The implementation
	// MUST go through the mikrotik destructive write allowlist (or
	// equivalent: this is the byte-level safety boundary).
	SetFrequency(ctx context.Context, deviceID, ifaceName string, mhz int) error
}

// FrequencyCorrectionAuditEmitter is the seam the action uses to
// emit its 8 lifecycle events. The handler injects an emitter
// bound to the run's actor + correlation_id + run_id so the audit
// row for each event carries the full context.
type FrequencyCorrectionAuditEmitter interface {
	Emit(ctx context.Context, action DestructiveAuditAction, outcome AuditOutcome, metadata map[string]any)
}

// AuditOutcome mirrors the audit.Outcome enum at the package
// boundary so the networkactions package does not import the audit
// package directly. The handler maps these to the concrete audit
// outcome strings ("success" / "failure").
type AuditOutcome string

const (
	AuditOutcomeSuccess AuditOutcome = "success"
	AuditOutcomeFailure AuditOutcome = "failure"
)

// FrequencyCorrectionLogger is the structured-log seam the action
// uses for operator-visible diagnostics. The handler wires
// internal/logger.Logger into this interface.
type FrequencyCorrectionLogger interface {
	Info(msg string, attrs ...any)
	Warn(msg string, attrs ...any)
	Error(msg string, attrs ...any)
}

// frequencyCorrectionAction is the concrete Action that drives the
// device. Wire it into the registry via RegisterFrequencyCorrection.
type frequencyCorrectionAction struct {
	writer FrequencyCorrectionWriter
	audit  FrequencyCorrectionAuditEmitter
	log    FrequencyCorrectionLogger
}

// Sentinel errors the action returns + that the handler maps onto
// run row error_codes. Tests pin the strings.
var (
	// ErrFrequencyDeviceUnreachable means the snapshot read failed
	// (network IO / dial). No write was attempted.
	ErrFrequencyDeviceUnreachable = errors.New("frequency_correction: device unreachable")
	// ErrFrequencyUnreadable means the device replied but the
	// frequency value could not be parsed from the response.
	ErrFrequencyUnreadable = errors.New("frequency_correction: snapshot value unreadable")
	// ErrFrequencyWriteFailed means the destructive write itself
	// failed (allowlist rejection, IO error, device-side rejection).
	ErrFrequencyWriteFailed = errors.New("frequency_correction: write failed")
	// ErrFrequencyVerificationFailedRollbackRecovered means the
	// post-write read returned a value that did not match the
	// target, but the rollback write + re-verify succeeded. Run
	// row terminal status is failed/error_code=verification_failed_rollback_recovered.
	ErrFrequencyVerificationFailedRollbackRecovered = errors.New("frequency_correction: verification failed but rollback recovered")
	// ErrFrequencyRollbackFailed means the post-write verification
	// missed AND the rollback could not restore the snapshot. The
	// device is in an unknown state from the runtime's perspective.
	// Operator MUST review.
	ErrFrequencyRollbackFailed = errors.New("frequency_correction: rollback failed — operator review required")
)

// RegisterFrequencyCorrection wires the real Phase 10E action into
// the registry. Call this from the API server bootstrap once the
// mikrotik writer + audit emitter + logger are available. The
// other (read-only and not-yet-implemented destructive) Kinds keep
// their stubAction.
func RegisterFrequencyCorrection(
	r *Registry,
	w FrequencyCorrectionWriter,
	emitter FrequencyCorrectionAuditEmitter,
	log FrequencyCorrectionLogger,
) {
	r.Register(&frequencyCorrectionAction{
		writer: w,
		audit:  emitter,
		log:    log,
	})
}

// Kind implements Action.
func (a *frequencyCorrectionAction) Kind() Kind { return KindFrequencyCorrection }

// Execute runs the Phase 10E lifecycle. It assumes the pre-gate
// already passed; do NOT call this without the runner's gate +
// per-device lock acquired upstream.
//
// Branching:
//
//	snapshot fails           → execute_started? NO (snapshot precedes started)
//	                           execute_write_failed (device_unreachable)
//	                           return Result{Success:false, ErrorCode:"device_unreachable"}
//	write fails              → execute_started, execute_write_failed
//	                           return Result{Success:false, ErrorCode:"write_failed"}
//	verify matches           → execute_started, execute_write_succeeded,
//	                           execute_verified
//	                           return Result{Success:true}
//	verify mismatches +
//	rollback succeeds        → execute_started, execute_write_succeeded,
//	                           execute_verification_failed,
//	                           execute_rollback_started,
//	                           execute_rollback_succeeded
//	                           return Result{Success:false,
//	                             ErrorCode:"verification_failed_rollback_recovered"}
//	verify mismatches +
//	rollback fails           → execute_started, execute_write_succeeded,
//	                           execute_verification_failed,
//	                           execute_rollback_started,
//	                           execute_rollback_failed
//	                           return Result{Success:false,
//	                             ErrorCode:"rollback_failed"}
func (a *frequencyCorrectionAction) Execute(ctx context.Context, req Request) (Result, error) {
	startedAt := time.Now().UTC()
	res := Result{
		Kind:          KindFrequencyCorrection,
		DeviceID:      req.DeviceID,
		CorrelationID: req.CorrelationID,
		StartedAt:     startedAt,
		DryRun:        req.DryRun,
		Result:        map[string]any{},
	}
	finish := func(success bool, code, msg string, extra map[string]any) (Result, error) {
		res.FinishedAt = time.Now().UTC()
		res.Success = success
		res.ErrorCode = code
		res.Message = msg
		for k, v := range extra {
			res.Result[k] = v
		}
		var sentinel error
		switch code {
		case "":
			sentinel = nil
		case "device_unreachable":
			sentinel = ErrFrequencyDeviceUnreachable
		case "snapshot_unreadable":
			sentinel = ErrFrequencyUnreadable
		case "write_failed":
			sentinel = ErrFrequencyWriteFailed
		case "verification_failed_rollback_recovered":
			sentinel = ErrFrequencyVerificationFailedRollbackRecovered
		case "rollback_failed":
			sentinel = ErrFrequencyRollbackFailed
		default:
			sentinel = nil
		}
		return res, sentinel
	}

	// The handler MUST shape the typed payload into the Request
	// fields the action consumes. We pull what we need defensively.
	cfg, cfgErr := frequencyCorrectionRequestFromGeneric(req)
	if cfgErr != nil {
		// Mis-shaped request is a Phase 10D-style invariant violation
		// from the action's perspective: a destructive Kind reached
		// Execute without enough context. Surface it loudly; the
		// handler's invariant-violation branch persists this.
		return finish(false, "request_invalid", cfgErr.Error(), map[string]any{
			"phase": "request_invalid",
		})
	}

	// === Snapshot ===
	snapshot, err := a.writer.SnapshotFrequency(ctx, cfg.DeviceID, cfg.Interface)
	if err != nil {
		// No write attempted yet; emit the write_failed event with
		// device_unreachable so the lifecycle clearly distinguishes
		// "we never wrote" from "we wrote and lost track".
		a.emit(ctx, AuditActionExecuteWriteFailed, AuditOutcomeFailure, cfg, map[string]any{
			"phase":      "snapshot_failed",
			"error_code": "device_unreachable",
			"reason":     err.Error(),
		})
		a.log.Error("frequency_correction snapshot failed",
			"run_id", cfg.RunID,
			"device_id", cfg.DeviceID,
			"err", err,
		)
		return finish(false, "device_unreachable", "snapshot read failed before any write was issued", map[string]any{
			"phase":          "snapshot_failed",
			"snapshot_error": err.Error(),
		})
	}
	if snapshot <= 0 {
		a.emit(ctx, AuditActionExecuteWriteFailed, AuditOutcomeFailure, cfg, map[string]any{
			"phase":      "snapshot_unreadable",
			"error_code": "snapshot_unreadable",
			"snapshot":   snapshot,
		})
		return finish(false, "snapshot_unreadable", "snapshot returned a non-positive frequency", map[string]any{
			"phase":    "snapshot_unreadable",
			"snapshot": snapshot,
		})
	}
	res.Result["snapshot_freq_mhz"] = snapshot
	res.Result["target_freq_mhz"] = cfg.TargetFrequencyMHz
	res.Result["interface"] = cfg.Interface

	// === execute_started ===
	a.emit(ctx, AuditActionExecuteStarted, AuditOutcomeSuccess, cfg, map[string]any{
		"phase":             "execute_started",
		"snapshot_freq_mhz": snapshot,
		"target_freq_mhz":   cfg.TargetFrequencyMHz,
		"interface":         cfg.Interface,
	})

	// === Write ===
	if err := a.writer.SetFrequency(ctx, cfg.DeviceID, cfg.Interface, cfg.TargetFrequencyMHz); err != nil {
		a.emit(ctx, AuditActionExecuteWriteFailed, AuditOutcomeFailure, cfg, map[string]any{
			"phase":      "write_failed",
			"error_code": "write_failed",
			"reason":     err.Error(),
		})
		a.log.Error("frequency_correction write failed",
			"run_id", cfg.RunID,
			"device_id", cfg.DeviceID,
			"err", err,
		)
		return finish(false, "write_failed", "destructive write rejected", map[string]any{
			"phase":       "write_failed",
			"write_error": err.Error(),
		})
	}
	a.emit(ctx, AuditActionExecuteWriteSucceeded, AuditOutcomeSuccess, cfg, map[string]any{
		"phase":           "write_succeeded",
		"target_freq_mhz": cfg.TargetFrequencyMHz,
		"interface":       cfg.Interface,
	})

	// Increment the runtime's command_count signal so the run row's
	// command_count reflects the byte we put on the wire.
	res.Result["command_count"] = 1

	// === Verify ===
	verifyMhz, verifyErr := a.writer.SnapshotFrequency(ctx, cfg.DeviceID, cfg.Interface)
	if verifyErr == nil && verifyMhz == cfg.TargetFrequencyMHz {
		a.emit(ctx, AuditActionExecuteVerified, AuditOutcomeSuccess, cfg, map[string]any{
			"phase":         "verified",
			"verified_freq": verifyMhz,
			"target_freq":   cfg.TargetFrequencyMHz,
			"interface":     cfg.Interface,
		})
		res.Result["verified_freq_mhz"] = verifyMhz
		res.Result["command_count"] = 2 // write + verify-read
		return finish(true, "", "frequency_correction applied + verified", map[string]any{
			"phase": "verified",
		})
	}

	// Verification missed (read failed OR mismatched). Emit the
	// non-terminal failure event and fall into the rollback path.
	verifyMeta := map[string]any{
		"phase":         "verification_failed",
		"target_freq":   cfg.TargetFrequencyMHz,
		"interface":     cfg.Interface,
		"verified_freq": verifyMhz,
	}
	if verifyErr != nil {
		verifyMeta["verify_error"] = verifyErr.Error()
	}
	a.emit(ctx, AuditActionExecuteVerificationFailed, AuditOutcomeFailure, cfg, verifyMeta)

	// === Rollback ===
	a.emit(ctx, AuditActionExecuteRollbackStarted, AuditOutcomeSuccess, cfg, map[string]any{
		"phase":              "rollback_started",
		"target_rollback_to": snapshot,
		"interface":          cfg.Interface,
	})
	res.Result["command_count"] = 2 // write + verify-read so far
	if err := a.writer.SetFrequency(ctx, cfg.DeviceID, cfg.Interface, snapshot); err != nil {
		a.emit(ctx, AuditActionExecuteRollbackFailed, AuditOutcomeFailure, cfg, map[string]any{
			"phase":         "rollback_failed",
			"error_code":    "rollback_failed",
			"reason":        err.Error(),
			"snapshot_freq": snapshot,
			"interface":     cfg.Interface,
		})
		a.log.Error("frequency_correction rollback write failed",
			"run_id", cfg.RunID,
			"device_id", cfg.DeviceID,
			"err", err,
		)
		return finish(false, "rollback_failed", "rollback write failed; operator must review the device", map[string]any{
			"phase":          "rollback_failed",
			"rollback_error": err.Error(),
			"snapshot_freq":  snapshot,
		})
	}
	res.Result["command_count"] = 3 // write + verify-read + rollback-write
	rollbackVerify, rbVerifyErr := a.writer.SnapshotFrequency(ctx, cfg.DeviceID, cfg.Interface)
	if rbVerifyErr != nil || rollbackVerify != snapshot {
		meta := map[string]any{
			"phase":         "rollback_verification_failed",
			"error_code":    "rollback_failed",
			"snapshot_freq": snapshot,
			"rollback_freq": rollbackVerify,
		}
		if rbVerifyErr != nil {
			meta["verify_error"] = rbVerifyErr.Error()
		}
		a.emit(ctx, AuditActionExecuteRollbackFailed, AuditOutcomeFailure, cfg, meta)
		a.log.Error("frequency_correction rollback re-verify failed",
			"run_id", cfg.RunID,
			"device_id", cfg.DeviceID,
		)
		return finish(false, "rollback_failed", "rollback write succeeded but re-verify did not match snapshot; operator must review the device", map[string]any{
			"phase":         "rollback_verification_failed",
			"snapshot_freq": snapshot,
			"rollback_freq": rollbackVerify,
		})
	}
	res.Result["command_count"] = 4 // write + verify-read + rollback-write + rollback-verify-read
	a.emit(ctx, AuditActionExecuteRollbackSucceeded, AuditOutcomeSuccess, cfg, map[string]any{
		"phase":         "rollback_succeeded",
		"snapshot_freq": snapshot,
		"rollback_freq": rollbackVerify,
		"interface":     cfg.Interface,
	})
	return finish(false, "verification_failed_rollback_recovered", "verification did not match target; rollback restored snapshot value", map[string]any{
		"phase":         "rollback_succeeded",
		"snapshot_freq": snapshot,
		"rollback_freq": rollbackVerify,
		"target_freq":   cfg.TargetFrequencyMHz,
	})
}

// emit centralises the audit metadata Phase 10E events share. The
// caller-supplied extras override the defaults; the merged map is
// what reaches the audit emitter.
func (a *frequencyCorrectionAction) emit(
	ctx context.Context,
	action DestructiveAuditAction,
	outcome AuditOutcome,
	cfg FrequencyCorrectionRequest,
	extras map[string]any,
) {
	if a.audit == nil {
		return
	}
	meta := map[string]any{
		"action_type":    string(KindFrequencyCorrection),
		"run_id":         cfg.RunID,
		"correlation_id": cfg.CorrelationID,
		"device_id":      cfg.DeviceID,
		"interface":      cfg.Interface,
	}
	if cfg.Intent != "" {
		meta["intent"] = cfg.Intent
	}
	for k, v := range extras {
		meta[k] = v
	}
	a.audit.Emit(ctx, action, outcome, meta)
}

// frequencyCorrectionRequestFromGeneric extracts the typed Phase 10E
// payload from the generic Request. The runner's HTTP handler stuffs
// the payload into Request.Reason as a single sentinel field is not
// available; instead Phase 10E uses a side-channel: the handler
// MUST call SetFrequencyCorrectionPayload(req, &payload) before
// dispatching. If the side-channel is unset we treat the run as a
// request_invalid case so the caller cannot accidentally drive a
// destructive Kind without typed inputs.
func frequencyCorrectionRequestFromGeneric(req Request) (FrequencyCorrectionRequest, error) {
	payload := getFrequencyCorrectionPayload(req.CorrelationID)
	if payload == nil {
		return FrequencyCorrectionRequest{}, fmt.Errorf("frequency_correction: typed payload missing for correlation_id=%q", req.CorrelationID)
	}
	if payload.DeviceID == "" {
		return FrequencyCorrectionRequest{}, errors.New("frequency_correction: device_id missing")
	}
	if payload.Interface == "" {
		return FrequencyCorrectionRequest{}, errors.New("frequency_correction: interface missing")
	}
	if payload.TargetFrequencyMHz <= 0 {
		return FrequencyCorrectionRequest{}, errors.New("frequency_correction: target_freq_mhz must be > 0")
	}
	return *payload, nil
}
