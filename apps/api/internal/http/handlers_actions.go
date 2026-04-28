package http

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	wispssh "github.com/wisp-ops-center/wisp-ops-center/internal/adapters/ssh"
	"github.com/wisp-ops-center/wisp-ops-center/internal/audit"
	"github.com/wisp-ops-center/wisp-ops-center/internal/networkactions"
	"github.com/wisp-ops-center/wisp-ops-center/internal/networkinv"
)

// actionCreateRequest is the inbound JSON for any of the read-only
// action endpoints. Either target_device_id (a network_devices.id
// from Phase 8 inventory) OR target_host (raw IP/host) is required.
// When both are provided, target_device_id wins.
type actionCreateRequest struct {
	TargetDeviceID string `json:"target_device_id,omitempty"`
	TargetHost     string `json:"target_host,omitempty"`
	Reason         string `json:"reason,omitempty"`
}

// actionEndpoint binds a URL suffix to one of the registered Kinds.
// Phase 9 v2 adds three more after frequency_check.
type actionEndpoint struct {
	Suffix string // e.g. "frequency-check", "ap-client-test"
	Kind   networkactions.Kind
}

var actionEndpoints = []actionEndpoint{
	{"frequency-check", networkactions.KindFrequencyCheck},
	{"ap-client-test", networkactions.KindAPClientTest},
	{"link-signal-test", networkactions.KindLinkSignalTest},
	{"bridge-health-check", networkactions.KindBridgeHealthCheck},
}

// handleActionCreateGeneric is the single entry point that all four
// "create + run" endpoints share. The kind is bound via the route
// table; everything else (target resolution, run row, audit, async
// runner) is identical.
//
// Lifecycle:
//  1. Resolve target → host + label (inventory lookup if device_id).
//  2. Persist a queued network_action_runs row.
//  3. Emit network_action.start audit event.
//  4. Execute the action asynchronously; finalize the row +
//     emit network_action.finish/failed when done.
//  5. Respond 202 with { run_id, correlation_id, status: "running" }.
func (s *Server) handleActionCreateGeneric(w http.ResponseWriter, r *http.Request, kind networkactions.Kind) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	if !s.requireDB(w) {
		return
	}
	var req actionCreateRequest
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
	// Phase 9 v3 — structural target_host validation BEFORE the DB
	// inet cast. Any smuggled write payload (e.g. "frequency=5180")
	// gets a clean 400 here and never reaches the database, audit,
	// or SSH layer.
	if err := networkactions.ValidateTargetHost(host); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid_target_host",
			"hint":  "target_host must be a valid IPv4, IPv6 or hostname",
		})
		return
	}

	if !s.dudeConfigured() {
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{
			"error": "not_configured",
			"hint":  "MIKROTIK_DUDE_USERNAME/PASSWORD must be set so the action can authenticate to target devices",
		})
		return
	}

	correlationID := networkactions.NewCorrelationID()
	run, err := s.actionRepo.CreateRun(r.Context(), networkactions.CreateRunInput{
		ActionType:     kind,
		TargetDeviceID: deviceID,
		TargetHost:     host,
		TargetLabel:    label,
		Actor:          actor(r),
		CorrelationID:  correlationID,
		DryRun:         true, // Phase 9 v2: dry_run unconditional for read-only
	})
	if err != nil {
		s.log.Warn("nwaction_create_failed", "err", err, "kind", string(kind))
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
			"action_type":      string(kind),
			"correlation_id":   correlationID,
			"target_device_id": deviceID,
			"target_host":      host,
			"target_label":     label,
			"dry_run":          true,
			"reason":           networkactions.SanitizeMessage(req.Reason),
		},
	})

	go s.runActionAsync(kind, run.ID, correlationID, host, deviceID, label)

	writeJSON(w, http.StatusAccepted, map[string]any{
		"run_id":         run.ID,
		"correlation_id": correlationID,
		"status":         "running",
	})
}

// runActionAsync runs the chosen action in a goroutine, finalizes
// the row, and emits the terminal audit event. Panics are recovered;
// the run is marked failed with error_code=panic_recovered.
//
// Phase 9 v3: credentials are resolved through s.actionCreds
// (CredentialResolver) per-call, so per-device profiles can replace
// the Dude fallback later without touching this body. A typed
// ErrCredentialNotFound is surfaced as a stable error_code
// ("credential_not_found") and NEVER triggers an SSH dial.
func (s *Server) runActionAsync(
	kind networkactions.Kind,
	runID, correlationID, host, deviceID, label string,
) {
	startedAt := time.Now()
	defer func() {
		if rec := recover(); rec != nil {
			s.log.Warn("nwaction_panic_recovered",
				"correlation_id", correlationID, "run_id", runID,
				"kind", string(kind),
				"panic", networkactions.SanitizeMessage(fmt.Sprintf("%v", rec)),
			)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.actionRepo.FinalizeRun(ctx, runID, networkactions.FinalizeInput{
				Status:       networkactions.StatusFailed,
				DurationMS:   time.Since(startedAt).Milliseconds(),
				Result:       map[string]any{},
				ErrorCode:    "panic_recovered",
				ErrorMessage: "action worker panicked; see server logs",
			})
		}
	}()

	if err := s.actionRepo.MarkRunning(context.Background(), runID); err != nil {
		s.log.Warn("nwaction_mark_running_failed", "err", err, "run_id", runID)
	}

	resolveCtx, resolveCancel := context.WithTimeout(context.Background(), 5*time.Second)
	target, credErr := s.actionCreds.Resolve(resolveCtx, deviceID, host)
	resolveCancel()
	if credErr != nil {
		s.handleActionCredentialFailure(runID, correlationID, host, deviceID, label, kind,
			startedAt, credErr)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), target.Timeout+45*time.Second)
	defer cancel()

	action := s.buildAction(kind, target)

	res, err := action.Execute(ctx, networkactions.Request{
		Kind:          kind,
		DeviceID:      deviceID,
		CorrelationID: correlationID,
		DryRun:        true,
		Actor:         "system",
	})

	// Compute persisted shape per action kind. Skipped vs succeeded
	// is decided by the action's own result body.
	status := networkactions.StatusFailed
	if err == nil && res.Success {
		if isSkipped(kind, res.Result) {
			status = networkactions.StatusSkipped
		} else {
			status = networkactions.StatusSucceeded
		}
	}

	commandCount, warningCount, confidence := summarizeActionResult(kind, res)
	sanitized := networkactions.SanitizeResultMap(res.Result)

	finishOutcome := audit.OutcomeFailure
	if status == networkactions.StatusSucceeded || status == networkactions.StatusSkipped {
		finishOutcome = audit.OutcomeSuccess
	}

	finalErrCode := res.ErrorCode
	finalErrMsg := networkactions.SanitizeMessage(res.Message)
	if status == networkactions.StatusSucceeded || status == networkactions.StatusSkipped {
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
			"action_type":      string(kind),
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

// handleActionCredentialFailure finalizes a run row and emits the
// correct audit event when CredentialResolver.Resolve fails. The
// SSH layer is NEVER reached; error_code reflects the typed
// resolver error.
func (s *Server) handleActionCredentialFailure(
	runID, correlationID, host, deviceID, label string,
	kind networkactions.Kind,
	startedAt time.Time,
	credErr error,
) {
	errCode := networkactions.ErrorCode(credErr)
	if errCode == "" || errCode == "unknown" {
		errCode = "credential_resolve_failed"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = s.actionRepo.FinalizeRun(ctx, runID, networkactions.FinalizeInput{
		Status:       networkactions.StatusFailed,
		DurationMS:   time.Since(startedAt).Milliseconds(),
		Result:       map[string]any{},
		ErrorCode:    errCode,
		ErrorMessage: networkactions.SanitizeMessage(credErr.Error()),
	})
	s.audit(ctx, audit.Entry{
		Actor:   "system",
		Action:  audit.Action("network_action.failed"),
		Outcome: audit.OutcomeFailure,
		Subject: host,
		Metadata: map[string]any{
			"run_id":           runID,
			"action_type":      string(kind),
			"correlation_id":   correlationID,
			"target_device_id": deviceID,
			"target_host":      host,
			"target_label":     label,
			"status":           string(networkactions.StatusFailed),
			"error_code":       errCode,
			"duration_ms":      time.Since(startedAt).Milliseconds(),
			// Phase 9 v3: name the credential bucket consulted, NEVER
			// the secret value.
			"credential_profile": networkactions.DudeStaticProfile,
		},
	})
}

// buildAction constructs the right action implementation for a kind.
// Each is independently configured from the same SSHTarget +
// known-hosts store so credential reuse is explicit.
func (s *Server) buildAction(kind networkactions.Kind, target networkactions.SSHTarget) networkactions.Action {
	store := s.dudeKnownHostsStore()
	switch kind {
	case networkactions.KindFrequencyCheck:
		return &networkactions.FrequencyCheckAction{Log: s.log, KnownHosts: store, Target: target}
	case networkactions.KindAPClientTest:
		return &networkactions.APClientTestAction{Log: s.log, KnownHosts: store, Target: target}
	case networkactions.KindLinkSignalTest:
		return &networkactions.LinkSignalTestAction{Log: s.log, KnownHosts: store, Target: target}
	case networkactions.KindBridgeHealthCheck:
		return &networkactions.BridgeHealthCheckAction{Log: s.log, KnownHosts: store, Target: target}
	}
	// Unknown kind → return a stub that always errors; the registry
	// already returns ErrActionNotImplemented for stubs.
	return s.netActions.Get(kind)
}

// _ keep symbol referenced — we still rely on slog/wispssh in the
// per-action constructors; keep imports honest if nothing inlines.
var _ = func(*slog.Logger, wispssh.KnownHostsStore) {}

// isSkipped looks at the typed result map and returns true when the
// action explicitly set Skipped=true.
func isSkipped(kind networkactions.Kind, m map[string]any) bool {
	if m == nil {
		return false
	}
	switch kind {
	case networkactions.KindFrequencyCheck:
		if v, ok := m["frequency_check"]; ok {
			if fc, ok := v.(networkactions.FrequencyCheckResult); ok {
				return fc.Skipped
			}
		}
	case networkactions.KindAPClientTest:
		if v, ok := m["ap_client_test"]; ok {
			if ap, ok := v.(networkactions.APClientTestResult); ok {
				return ap.Skipped
			}
		}
	case networkactions.KindLinkSignalTest:
		if v, ok := m["link_signal_test"]; ok {
			if lr, ok := v.(networkactions.LinkSignalTestResult); ok {
				return lr.Skipped
			}
		}
	case networkactions.KindBridgeHealthCheck:
		if v, ok := m["bridge_health_check"]; ok {
			if br, ok := v.(networkactions.BridgeHealthResult); ok {
				return br.Skipped
			}
		}
	}
	return false
}

// summarizeActionResult tallies command_count, warning_count and
// confidence per action kind. Confidence:
//
//   - 0   on failure
//   - 30  on skipped (no relevant data observed)
//   - 60  on success with at least one observed entity
//   - 70-90 with richer evidence (see per-kind helpers)
func summarizeActionResult(kind networkactions.Kind, res networkactions.Result) (cmds, warns, conf int) {
	if res.Result == nil {
		if res.Success {
			return 0, 0, 30
		}
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
	switch kind {
	case networkactions.KindFrequencyCheck:
		c, w, n := summarizeFrequencyCheckResult(res)
		// summarizeFrequencyCheckResult already counts commands; prefer
		// our local count if non-zero (skips edge cases of nil slices).
		if cmds == 0 {
			cmds = c
		}
		return cmds, w, n
	case networkactions.KindAPClientTest:
		ap, _ := res.Result["ap_client_test"].(networkactions.APClientTestResult)
		warns = len(ap.Warnings)
		switch {
		case !res.Success:
			conf = 0
		case ap.Skipped, ap.ClientCount == 0:
			conf = 30
		case ap.ClientCount >= 5:
			conf = 80
		default:
			conf = 60
		}
		if ap.AvgSignal != nil {
			conf += 5
		}
		if conf > 100 {
			conf = 100
		}
	case networkactions.KindLinkSignalTest:
		lr, _ := res.Result["link_signal_test"].(networkactions.LinkSignalTestResult)
		warns = len(lr.Warnings)
		switch {
		case !res.Success:
			conf = 0
		case lr.Skipped, !lr.LinkDetected:
			conf = 30
		case lr.HealthStatus == "healthy":
			conf = 85
		case lr.HealthStatus == "warning":
			conf = 70
		case lr.HealthStatus == "critical":
			conf = 60
		default:
			conf = 40
		}
	case networkactions.KindBridgeHealthCheck:
		br, _ := res.Result["bridge_health_check"].(networkactions.BridgeHealthResult)
		warns = len(br.Warnings)
		switch {
		case !res.Success:
			conf = 0
		case br.Skipped, br.BridgeCount == 0:
			conf = 30
		case len(br.DownPorts) > 0 || len(br.DisabledPorts) > 0:
			conf = 70
		default:
			conf = 80
		}
	}
	return cmds, warns, conf
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

// handleNetworkActionsDispatch demultiplexes
//
//	/api/v1/network/actions
//	/api/v1/network/actions/{kind-suffix}     (POST)
//	/api/v1/network/actions/{run-uuid}        (GET)
func (s *Server) handleNetworkActionsDispatch(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/network/actions")
	switch {
	case rest == "" || rest == "/":
		s.handleNetworkActionsList(w, r)
		return
	}
	leaf := strings.TrimPrefix(rest, "/")
	for _, ep := range actionEndpoints {
		if leaf == ep.Suffix {
			s.handleActionCreateGeneric(w, r, ep.Kind)
			return
		}
	}
	s.handleNetworkActionItem(w, r)
}

// ----------------------------------------------------------------------------
// frequency_check legacy summarizer (kept for backwards compatibility
// with summarizeActionResult above; calls the original Phase 9
// confidence ladder).
// ----------------------------------------------------------------------------

// summarizeFrequencyCheckResult tallies command count, warning count
// and confidence from a frequency_check result.
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
	fc, _ := res.Result["frequency_check"].(networkactions.FrequencyCheckResult)
	warns = len(fc.Warnings)
	if !res.Success {
		return cmds, warns, 0
	}
	if fc.Skipped || len(fc.Interfaces) == 0 {
		return cmds, warns, 30
	}
	conf = 60
	hasReg, hasFull := false, false
	for _, iface := range fc.Interfaces {
		if iface.RegistrationOK {
			hasReg = true
		}
		if iface.Frequency != "" && iface.Band != "" && iface.ChannelWidth != "" {
			hasFull = true
		}
	}
	if hasReg {
		conf = 80
	}
	if hasFull {
		conf += 10
	}
	if conf > 100 {
		conf = 100
	}
	return cmds, warns, conf
}
