package http

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/wisp-ops-center/wisp-ops-center/internal/audit"
	"github.com/wisp-ops-center/wisp-ops-center/internal/networkactions"
	"github.com/wisp-ops-center/wisp-ops-center/internal/networkinv"
)

// Phase 10C — Destructive runtime lifecycle.
//
// This file is the destructive counterpart to handlers_actions.go.
// Critical invariants enforced by the runner below:
//
//   1. Destructive Kinds NEVER reach action.Execute. Phase 10C is
//      lifecycle-only; the registry's destructive entries stay as
//      stubs and the handler short-circuits before any SSH dial.
//   2. Every destructive request MUST carry an idempotency_key,
//      intent, rollback_note. Missing fields → 400.
//   3. A duplicate (action_type, idempotency_key) returns the
//      original run + emits `network_action.idempotency_reused`.
//   4. The pre-gate runs through EnsureDestructiveAllowedWithProviders.
//      ANY failure emits the specific subtype event +
//      `network_action.gate_fail` + `network_action.destructive_denied`.
//   5. If the gate would pass with Confirm=true (live request),
//      the runner emits `network_action.live_start_blocked` +
//      `network_action.destructive_denied` and persists the run as
//      failed. Phase 10C invariant: live execution NEVER starts.
//   6. If the gate would pass with DryRun=true (read-mode preview),
//      the runner emits `network_action.dry_run` and persists the
//      run as succeeded — without calling Execute.

// destructiveCreateRequest is the inbound JSON body shared by
// /:kind/dry-run and /:kind/confirm.
type destructiveCreateRequest struct {
	TargetDeviceID string `json:"target_device_id,omitempty"`
	TargetHost     string `json:"target_host,omitempty"`
	IdempotencyKey string `json:"idempotency_key"`
	Intent         string `json:"intent"`
	RollbackNote   string `json:"rollback_note"`
	Reason         string `json:"reason,omitempty"`
}

// destructiveEndpoints binds URL suffixes to destructive Kinds. Phase
// 10C only ships frequency_correction; future destructive Kinds add
// rows here.
type destructiveEndpoint struct {
	Suffix string
	Kind   networkactions.Kind
}

var destructiveEndpoints = []destructiveEndpoint{
	{"frequency-correction", networkactions.KindFrequencyCorrection},
}

// handleDestructiveDispatch routes
//
//	POST /api/v1/network/actions/destructive/{kind}/dry-run
//	POST /api/v1/network/actions/destructive/{kind}/confirm
//	GET  /api/v1/network/actions/{run_id}/lifecycle
//
// Lifecycle dispatch is HTTP-prefixed under /api/v1/network/actions
// because the URL ends in /lifecycle; handlers_actions dispatcher
// already handles per-uuid GETs. To keep that surface intact, we add
// a separate dispatcher routed by routes.go for the lifecycle path.
func (s *Server) handleDestructiveDispatch(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/network/actions/destructive/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) < 2 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	kindSuffix := parts[0]
	mode := parts[1]
	var kind networkactions.Kind
	for _, e := range destructiveEndpoints {
		if e.Suffix == kindSuffix {
			kind = e.Kind
			break
		}
	}
	if kind == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown_destructive_kind"})
		return
	}
	switch mode {
	case "dry-run":
		s.handleDestructiveCreate(w, r, kind, false /* confirm */)
	case "confirm":
		s.handleDestructiveCreate(w, r, kind, true /* confirm */)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown_mode"})
	}
}

// handleDestructiveCreate is the single entry point for both modes.
// `confirm` is true for the live-attempt endpoint and false for the
// dry-run endpoint.
func (s *Server) handleDestructiveCreate(
	w http.ResponseWriter, r *http.Request,
	kind networkactions.Kind, confirm bool,
) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	if !s.requireDB(w) {
		return
	}
	principal := principalFromRequest(r)
	// Capability gate — single choke-point. Confirm path requires
	// Execute capability; dry-run path requires DryRun capability so
	// a viewer cannot probe destructive intents.
	want := networkactions.CapabilityDestructiveDryRun
	if confirm {
		want = networkactions.CapabilityDestructiveExecute
	}
	if !s.requireCapability(w, r, principal, want) {
		return
	}
	var req destructiveCreateRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body", "hint": err.Error()})
		return
	}
	host := strings.TrimSpace(req.TargetHost)
	deviceID := strings.TrimSpace(req.TargetDeviceID)
	idemKey := strings.TrimSpace(req.IdempotencyKey)
	intent := strings.TrimSpace(req.Intent)
	rollbackNote := strings.TrimSpace(req.RollbackNote)
	if idemKey == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "idempotency_key_required"})
		return
	}
	if intent == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "intent_required"})
		return
	}
	if rollbackNote == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "rollback_note_required"})
		return
	}
	// Resolve target. Same shape as handlers_actions.handleActionCreateGeneric.
	label := ""
	if deviceID != "" {
		if !looksLikeUUID(deviceID) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_device_id"})
			return
		}
		dev, err := s.netInv.GetDevice(r.Context(), deviceID)
		if err != nil {
			if errors.Is(err, networkinv.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "device_not_found"})
				return
			}
			s.log.Warn("destructive_device_lookup_failed", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		host = dev.Host
		label = dev.Name
	}
	if host == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "missing_target",
			"hint":  "provide target_device_id (with non-empty host) or target_host",
		})
		return
	}
	if err := networkactions.ValidateTargetHost(host); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid_target_host",
			"hint":  "target_host must be a valid IPv4, IPv6 or hostname",
		})
		return
	}

	// Idempotency lookup BEFORE creating a new run.
	if existing, err := s.actionRepo.FindByIdempotencyKey(r.Context(), kind, idemKey); err == nil && existing != nil {
		s.audit(r.Context(), audit.Entry{
			Actor:   principal.Actor,
			Action:  audit.Action(networkactions.AuditActionIdempotencyReused),
			Outcome: audit.OutcomeSuccess,
			Subject: existing.ID,
			Metadata: map[string]any{
				"run_id":          existing.ID,
				"action_type":     string(kind),
				"idempotency_key": idemKey,
				"intent":          intent,
				"correlation_id":  existing.CorrelationID,
				"original_status": string(existing.Status),
			},
		})
		writeJSON(w, http.StatusOK, map[string]any{
			"run_id":         existing.ID,
			"correlation_id": existing.CorrelationID,
			"status":         string(existing.Status),
			"reused":         true,
		})
		return
	}

	correlationID := networkactions.NewCorrelationID()
	run, err := s.actionRepo.CreateDestructiveRun(r.Context(), networkactions.DestructiveCreateRunInput{
		ActionType:     kind,
		TargetDeviceID: deviceID,
		TargetHost:     host,
		TargetLabel:    label,
		Actor:          principal.Actor,
		CorrelationID:  correlationID,
		DryRun:         !confirm,
		IdempotencyKey: idemKey,
		Intent:         intent,
		RollbackNote:   rollbackNote,
	})
	if err != nil {
		// Race: another request inserted the same key between our
		// FindByIdempotencyKey and the INSERT. Treat as reused.
		if errors.Is(err, networkactions.ErrIdempotencyConflict) {
			existing, fetchErr := s.actionRepo.FindByIdempotencyKey(r.Context(), kind, idemKey)
			if fetchErr == nil && existing != nil {
				s.audit(r.Context(), audit.Entry{
					Actor:   principal.Actor,
					Action:  audit.Action(networkactions.AuditActionIdempotencyReused),
					Outcome: audit.OutcomeSuccess,
					Subject: existing.ID,
					Metadata: map[string]any{
						"run_id":          existing.ID,
						"action_type":     string(kind),
						"idempotency_key": idemKey,
						"intent":          intent,
						"correlation_id":  existing.CorrelationID,
						"race":            true,
					},
				})
				writeJSON(w, http.StatusOK, map[string]any{
					"run_id":         existing.ID,
					"correlation_id": existing.CorrelationID,
					"status":         string(existing.Status),
					"reused":         true,
				})
				return
			}
		}
		s.log.Warn("destructive_create_failed", "err", err, "kind", string(kind))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}

	// Audit: rollback metadata recorded (run row carries it now).
	s.audit(r.Context(), audit.Entry{
		Actor:   principal.Actor,
		Action:  audit.Action(networkactions.AuditActionRollbackMetadataRecorded),
		Outcome: audit.OutcomeSuccess,
		Subject: run.ID,
		Metadata: map[string]any{
			"run_id":              run.ID,
			"action_type":         string(kind),
			"correlation_id":      correlationID,
			"intent":              intent,
			"rollback_note_bytes": len(rollbackNote),
			"idempotency_key":     idemKey,
		},
	})
	// Audit: operator confirmed destructive intent (Phase 10C
	// emits this as soon as the request shape is acceptable, even
	// before the gate runs, so a downstream auditor can find every
	// destructive intent — including denied ones).
	s.audit(r.Context(), audit.Entry{
		Actor:   principal.Actor,
		Action:  audit.Action(networkactions.AuditActionConfirmed),
		Outcome: audit.OutcomeSuccess,
		Subject: run.ID,
		Metadata: map[string]any{
			"run_id":         run.ID,
			"action_type":    string(kind),
			"correlation_id": correlationID,
			"dry_run":        !confirm,
			"confirm":        confirm,
			"reason":         networkactions.SanitizeMessage(req.Reason),
		},
	})

	// Hand off to the runner. The runner runs the pre-gate, emits
	// every lifecycle event, persists the terminal status, and
	// NEVER calls action.Execute for a destructive Kind.
	go s.runDestructiveActionAsync(kind, run.ID, correlationID, host, deviceID, label,
		principal, confirm, idemKey, intent, rollbackNote)

	writeJSON(w, http.StatusAccepted, map[string]any{
		"run_id":         run.ID,
		"correlation_id": correlationID,
		"status":         "queued",
		"dry_run":        !confirm,
	})
}

// runDestructiveActionAsync is the destructive equivalent of
// runActionAsync. CRITICAL Phase 10C invariant: this function MUST
// NOT call action.Execute. It runs the pre-gate, emits lifecycle
// audit events, finalizes the run row, and exits.
func (s *Server) runDestructiveActionAsync(
	kind networkactions.Kind,
	runID, correlationID, host, deviceID, label string,
	principal networkactions.Principal,
	confirm bool,
	idempotencyKey, intent, rollbackNote string,
) {
	startedAt := time.Now()
	if err := s.actionRepo.MarkRunning(context.Background(), runID); err != nil {
		s.log.Warn("destructive_mark_running_failed", "err", err, "run_id", runID)
	}

	gateCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	providers := s.destructiveProviders()
	now := time.Now().UTC()
	gateReq := networkactions.DestructiveRequest{
		Kind:           kind,
		DeviceID:       deviceID,
		Actor:          principal.Actor,
		ActorRoles:     principal.Roles,
		DryRun:         !confirm,
		Confirm:        confirm,
		RollbackNote:   rollbackNote,
		IdempotencyKey: idempotencyKey,
		Now:            now,
	}

	gateErr := networkactions.EnsureDestructiveAllowedWithProviders(gateCtx, providers, gateReq)

	// On gate failure: emit the specific subtype event + gate_fail +
	// destructive_denied. Persist the run as failed with
	// error_code=DestructiveErrorCode(err). Do NOT call Execute.
	if gateErr != nil {
		errCode := networkactions.DestructiveErrorCode(gateErr)
		specific := networkactions.AuditActionForGateError(gateErr)

		s.audit(gateCtx, audit.Entry{
			Actor:   principal.Actor,
			Action:  audit.Action(specific),
			Outcome: audit.OutcomeFailure,
			Subject: runID,
			Metadata: map[string]any{
				"run_id":         runID,
				"action_type":    string(kind),
				"correlation_id": correlationID,
				"error_code":     errCode,
				"target_host":    host,
				"intent":         intent,
			},
		})
		if specific != networkactions.AuditActionGateFail {
			s.audit(gateCtx, audit.Entry{
				Actor:   principal.Actor,
				Action:  audit.Action(networkactions.AuditActionGateFail),
				Outcome: audit.OutcomeFailure,
				Subject: runID,
				Metadata: map[string]any{
					"run_id":         runID,
					"action_type":    string(kind),
					"correlation_id": correlationID,
					"error_code":     errCode,
				},
			})
		}
		s.audit(gateCtx, audit.Entry{
			Actor:   principal.Actor,
			Action:  audit.Action(networkactions.AuditActionDestructiveDenied),
			Outcome: audit.OutcomeFailure,
			Subject: runID,
			Metadata: map[string]any{
				"run_id":         runID,
				"action_type":    string(kind),
				"correlation_id": correlationID,
				"error_code":     errCode,
				"phase":          "gate_fail",
				"confirm":        confirm,
			},
		})

		_ = s.actionRepo.FinalizeRun(gateCtx, runID, networkactions.FinalizeInput{
			Status:       networkactions.StatusFailed,
			DurationMS:   time.Since(startedAt).Milliseconds(),
			Result:       map[string]any{"phase": "gate_fail"},
			ErrorCode:    errCode,
			ErrorMessage: networkactions.SanitizeMessage(gateErr.Error()),
		})
		return
	}

	// Gate passed. Phase 10C invariant: live execution NEVER starts.
	// Two paths:
	//   * dry-run (Confirm=false): emit dry_run, persist as succeeded.
	//   * confirm (Confirm=true):  emit live_start_blocked +
	//     destructive_denied, persist as failed.
	if !confirm {
		s.audit(gateCtx, audit.Entry{
			Actor:   principal.Actor,
			Action:  audit.Action(networkactions.AuditActionDryRunCompleted),
			Outcome: audit.OutcomeSuccess,
			Subject: runID,
			Metadata: map[string]any{
				"run_id":         runID,
				"action_type":    string(kind),
				"correlation_id": correlationID,
				"target_host":    host,
				"target_label":   label,
				"intent":         intent,
				// Phase 10C does NOT call Execute; the dry-run is
				// purely a "gate passed in dry-run mode" signal.
				"phase": "dry_run_completed_no_execute",
			},
		})
		_ = s.actionRepo.FinalizeRun(gateCtx, runID, networkactions.FinalizeInput{
			Status:     networkactions.StatusSucceeded,
			DurationMS: time.Since(startedAt).Milliseconds(),
			Result:     map[string]any{"phase": "dry_run_completed_no_execute"},
			Confidence: 30, // dry-run preview confidence
		})
		return
	}

	// Confirm=true path. Phase 10C MUST block live execution.
	s.audit(gateCtx, audit.Entry{
		Actor:   principal.Actor,
		Action:  audit.Action(networkactions.AuditActionLiveStartBlocked),
		Outcome: audit.OutcomeFailure,
		Subject: runID,
		Metadata: map[string]any{
			"run_id":         runID,
			"action_type":    string(kind),
			"correlation_id": correlationID,
			"target_host":    host,
			"intent":         intent,
			"phase":          "live_start_blocked",
			"reason":         "phase_10c_destructive_runtime_disabled",
		},
	})
	s.audit(gateCtx, audit.Entry{
		Actor:   principal.Actor,
		Action:  audit.Action(networkactions.AuditActionDestructiveDenied),
		Outcome: audit.OutcomeFailure,
		Subject: runID,
		Metadata: map[string]any{
			"run_id":         runID,
			"action_type":    string(kind),
			"correlation_id": correlationID,
			"error_code":     "live_start_blocked",
			"phase":          "live_start_blocked",
		},
	})
	_ = s.actionRepo.FinalizeRun(gateCtx, runID, networkactions.FinalizeInput{
		Status:       networkactions.StatusFailed,
		DurationMS:   time.Since(startedAt).Milliseconds(),
		Result:       map[string]any{"phase": "live_start_blocked"},
		ErrorCode:    "live_start_blocked",
		ErrorMessage: "Phase 10C blocks live destructive execution by design.",
	})
}

// destructiveProviders returns the bundled providers consumed by
// EnsureDestructiveAllowedWithProviders. The handler builds it on
// demand because the toggle/window providers may be wired
// differently in dev vs production.
func (s *Server) destructiveProviders() *networkactions.DestructiveProviders {
	return &networkactions.DestructiveProviders{
		Toggle:      s.actionToggle,
		RBAC:        s.actionRBAC,
		Maintenance: s.actionWindowsProv,
	}
}

// handleActionLifecycle — GET /api/v1/network/actions/lifecycle/{run_id}
func (s *Server) handleActionLifecycle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	if !s.requireDB(w) {
		return
	}
	principal := principalFromRequest(r)
	if !s.requireCapability(w, r, principal, networkactions.CapabilityPreflightRead) {
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/network/actions/lifecycle/")
	id = strings.Trim(id, "/")
	if id == "" || strings.Contains(id, "/") || !looksLikeUUID(id) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_id"})
		return
	}
	run, events, err := s.actionRepo.GetLifecycle(r.Context(), id)
	if err != nil {
		if errors.Is(err, networkactions.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		s.log.Warn("destructive_lifecycle_failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"run":    run,
		"events": events,
	})
}
