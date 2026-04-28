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

// freqCheckCreateRequest is the inbound JSON for
// POST /api/v1/network/actions/frequency-check.
//
// Either target_device_id (a network_devices.id from Phase 8
// inventory) OR target_host (raw IP/host) is required. When both are
// provided, target_device_id wins and the host is taken from the
// inventory row.
type freqCheckCreateRequest struct {
	TargetDeviceID string `json:"target_device_id,omitempty"`
	TargetHost     string `json:"target_host,omitempty"`
	Reason         string `json:"reason,omitempty"`
}

// handleFrequencyCheckCreate — POST /api/v1/network/actions/frequency-check
//
// Lifecycle:
//  1. Resolve target → host + label (inventory lookup if device_id).
//  2. Persist a queued network_action_runs row.
//  3. Emit network_action.start audit event.
//  4. Execute the action asynchronously; finalize the row +
//     emit network_action.finish/failed when done.
//  5. Respond 202 with { run_id, correlation_id, status: "running" }.
//
// The async pattern matches Phase 8 dude discovery so the UI can
// poll /actions/{id} for the result.
func (s *Server) handleFrequencyCheckCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	if !s.requireDB(w) {
		return
	}
	var req freqCheckCreateRequest
	if err := readJSON(r, &req); err != nil && err.Error() != "EOF" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body", "hint": err.Error()})
		return
	}

	// Resolve target.
	host := strings.TrimSpace(req.TargetHost)
	deviceID := strings.TrimSpace(req.TargetDeviceID)
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
			s.log.Warn("nwaction_device_lookup_failed", "err", err)
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

	cfg, ok := s.dudeConfigFromRuntime()
	if !ok {
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{
			"error": "not_configured",
			"hint":  "MIKROTIK_DUDE_USERNAME/PASSWORD must be set so frequency_check can authenticate to target devices",
		})
		return
	}

	correlationID := networkactions.NewCorrelationID()
	run, err := s.actionRepo.CreateRun(r.Context(), networkactions.CreateRunInput{
		ActionType:     networkactions.KindFrequencyCheck,
		TargetDeviceID: deviceID,
		TargetHost:     host,
		TargetLabel:    label,
		Actor:          actor(r),
		CorrelationID:  correlationID,
		DryRun:         true, // Phase 9 actions are read-only by definition
	})
	if err != nil {
		s.log.Warn("nwaction_create_failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}

	s.audit(r.Context(), audit.Entry{
		Actor:   actor(r),
		Action:  audit.Action("network_action.start"),
		Outcome: audit.OutcomeSuccess,
		Subject: host,
		Metadata: map[string]any{
			"run_id":           run.ID,
			"action_type":      string(networkactions.KindFrequencyCheck),
			"correlation_id":   correlationID,
			"target_device_id": deviceID,
			"target_host":      host,
			"target_label":     label,
			"dry_run":          true,
			"reason":           networkactions.SanitizeMessage(req.Reason),
		},
	})

	go s.runFrequencyCheckAsync(run.ID, correlationID, host, deviceID, label, cfg.Username, cfg.Password, cfg.Port, cfg.HostKeyPolicy, cfg.HostKeyFingerprint, cfg.Timeout)

	writeJSON(w, http.StatusAccepted, map[string]any{
		"run_id":         run.ID,
		"correlation_id": correlationID,
		"status":         "running",
	})
}

// runFrequencyCheckAsync runs the action in a goroutine, finalizes
// the row, and emits the terminal audit event. Panics are recovered;
// the run is marked failed with error_code=panic_recovered.
func (s *Server) runFrequencyCheckAsync(
	runID, correlationID, host, deviceID, label string,
	username, password string, port int,
	hostKeyPolicy, hostKeyFingerprint string,
	timeout time.Duration,
) {
	startedAt := time.Now()
	defer func() {
		if rec := recover(); rec != nil {
			s.log.Warn("nwaction_panic_recovered",
				"correlation_id", correlationID, "run_id", runID,
				"panic", networkactions.SanitizeMessage("recovered"),
			)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.actionRepo.FinalizeRun(ctx, runID, networkactions.FinalizeInput{
				Status:       networkactions.StatusFailed,
				DurationMS:   time.Since(startedAt).Milliseconds(),
				Result:       map[string]any{},
				ErrorCode:    "panic_recovered",
				ErrorMessage: "frequency_check worker panicked; see server logs",
			})
		}
	}()

	if err := s.actionRepo.MarkRunning(context.Background(), runID); err != nil {
		s.log.Warn("nwaction_mark_running_failed", "err", err, "run_id", runID)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout+45*time.Second)
	defer cancel()

	action := &networkactions.FrequencyCheckAction{
		Log:        s.log,
		KnownHosts: s.dudeKnownHostsStore(),
		Target: networkactions.SSHTarget{
			Host:               host,
			Port:               port,
			Username:           username,
			Password:           password,
			Timeout:            timeout,
			HostKeyPolicy:      hostKeyPolicy,
			HostKeyFingerprint: hostKeyFingerprint,
		},
	}

	res, err := action.Execute(ctx, networkactions.Request{
		Kind:          networkactions.KindFrequencyCheck,
		DeviceID:      deviceID,
		CorrelationID: correlationID,
		DryRun:        true,
		Actor:         "system",
	})

	// Compute the persisted shape.
	status := networkactions.StatusFailed
	switch {
	case err == nil && res.Success:
		// Distinguish "succeeded with data" vs "succeeded but skipped".
		if fc, ok := getFrequencyCheckResult(res.Result); ok && fc.Skipped {
			status = networkactions.StatusSkipped
		} else {
			status = networkactions.StatusSucceeded
		}
	}

	commandCount, warningCount, confidence := summarizeFrequencyCheckResult(res)
	sanitized := networkactions.SanitizeResultMap(res.Result)

	finishOutcome := audit.OutcomeFailure
	if status == networkactions.StatusSucceeded || status == networkactions.StatusSkipped {
		finishOutcome = audit.OutcomeSuccess
	}

	finalErrCode := res.ErrorCode
	finalErrMsg := networkactions.SanitizeMessage(res.Message)
	if status == networkactions.StatusSucceeded || status == networkactions.StatusSkipped {
		// Don't persist a non-empty error string for terminal-success
		// outcomes; keep the row clean.
		finalErrCode = ""
		finalErrMsg = ""
	}

	persistErr := s.actionRepo.FinalizeRun(ctx, runID, networkactions.FinalizeInput{
		Status:       status,
		DurationMS:   time.Since(startedAt).Milliseconds(),
		Result:       sanitized,
		CommandCount: commandCount,
		WarningCount: warningCount,
		Confidence:   confidence,
		ErrorCode:    finalErrCode,
		ErrorMessage: finalErrMsg,
	})
	if persistErr != nil {
		s.log.Warn("nwaction_finalize_failed", "err", persistErr, "run_id", runID)
	}

	finalAction := "network_action.finish"
	if status == networkactions.StatusFailed {
		finalAction = "network_action.failed"
	}
	if errors.Is(err, networkactions.ErrDisallowedCommand) {
		finalAction = "network_action.blocked_command"
	}
	s.audit(ctx, audit.Entry{
		Actor:   "system",
		Action:  audit.Action(finalAction),
		Outcome: finishOutcome,
		Subject: host,
		Metadata: map[string]any{
			"run_id":           runID,
			"action_type":      string(networkactions.KindFrequencyCheck),
			"correlation_id":   correlationID,
			"target_device_id": deviceID,
			"target_host":      host,
			"target_label":     label,
			"status":           string(status),
			"duration_ms":      time.Since(startedAt).Milliseconds(),
			"command_count":    commandCount,
			"warning_count":    warningCount,
			"confidence":       confidence,
			"error_code":       finalErrCode,
		},
	})
}

// handleNetworkActionsList — GET /api/v1/network/actions?action_type=&status=&device_id=&limit=
func (s *Server) handleNetworkActionsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	if !s.requireDB(w) {
		return
	}
	q := r.URL.Query()
	f := networkactions.ListFilter{
		ActionType: q.Get("action_type"),
		Status:     q.Get("status"),
		DeviceID:   q.Get("device_id"),
	}
	if f.ActionType != "" && !networkactions.IsValidKind(f.ActionType) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_action_type"})
		return
	}
	if f.Status != "" && !networkactions.IsValidStatus(f.Status) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_status"})
		return
	}
	if f.DeviceID != "" && !looksLikeUUID(f.DeviceID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_device_id"})
		return
	}
	rows, err := s.actionRepo.ListRuns(r.Context(), f)
	if err != nil {
		s.log.Warn("nwaction_list_failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rows})
}

// handleNetworkActionItem — GET /api/v1/network/actions/{id}
func (s *Server) handleNetworkActionItem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	if !s.requireDB(w) {
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/network/actions/")
	if id == "" || strings.Contains(id, "/") || !looksLikeUUID(id) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_id"})
		return
	}
	row, err := s.actionRepo.GetRun(r.Context(), id)
	if err != nil {
		if errors.Is(err, networkactions.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		s.log.Warn("nwaction_get_failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": row})
}

// handleNetworkActionsDispatch demultiplexes /network/actions and
// /network/actions/{id|frequency-check}.
func (s *Server) handleNetworkActionsDispatch(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/network/actions")
	switch {
	case rest == "" || rest == "/":
		s.handleNetworkActionsList(w, r)
	case rest == "/frequency-check":
		s.handleFrequencyCheckCreate(w, r)
	default:
		s.handleNetworkActionItem(w, r)
	}
}

// ----------------------------------------------------------------------------
// helpers
// ----------------------------------------------------------------------------

// getFrequencyCheckResult extracts the typed FrequencyCheckResult
// from the dynamic Result map. Tolerates both struct and map shapes
// (the struct shape is what the action returns; tests sometimes pass
// raw maps).
func getFrequencyCheckResult(m map[string]any) (networkactions.FrequencyCheckResult, bool) {
	if m == nil {
		return networkactions.FrequencyCheckResult{}, false
	}
	v, ok := m["frequency_check"]
	if !ok {
		return networkactions.FrequencyCheckResult{}, false
	}
	if fc, ok := v.(networkactions.FrequencyCheckResult); ok {
		return fc, true
	}
	return networkactions.FrequencyCheckResult{}, false
}

// summarizeFrequencyCheckResult tallies command count, warning count
// and confidence from the action result. Confidence:
//
//   - 0   on failure
//   - 30  on skipped (no wireless menu)
//   - 60  on success with at least one running interface
//   - 80  on success with running interface AND populated registration
//   - +10 if frequency / band / width all present
//
// Tests can drive this without touching the SSH layer.
func summarizeFrequencyCheckResult(res networkactions.Result) (cmds, warns, conf int) {
	if res.Result == nil {
		return 0, 0, 0
	}
	if v, ok := res.Result["commands"]; ok {
		switch t := v.(type) {
		case []networkactions.SourceCommand:
			cmds = len(t)
		case []any:
			cmds = len(t)
		}
	}
	fc, _ := getFrequencyCheckResult(res.Result)
	warns = len(fc.Warnings)

	if !res.Success {
		return cmds, warns, 0
	}
	if fc.Skipped || len(fc.Interfaces) == 0 {
		return cmds, warns, 30
	}
	conf = 60
	hasRegistered := false
	hasFullSpec := false
	for _, iface := range fc.Interfaces {
		if iface.RegistrationOK {
			hasRegistered = true
		}
		if iface.Frequency != "" && iface.Band != "" && iface.ChannelWidth != "" {
			hasFullSpec = true
		}
	}
	if hasRegistered {
		conf = 80
	}
	if hasFullSpec {
		conf += 10
	}
	if conf > 100 {
		conf = 100
	}
	return cmds, warns, conf
}
