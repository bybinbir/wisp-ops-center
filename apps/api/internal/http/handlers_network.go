package http

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	wispssh "github.com/wisp-ops-center/wisp-ops-center/internal/adapters/ssh"
	"github.com/wisp-ops-center/wisp-ops-center/internal/audit"
	"github.com/wisp-ops-center/wisp-ops-center/internal/dude"
	"github.com/wisp-ops-center/wisp-ops-center/internal/networkinv"
	"github.com/wisp-ops-center/wisp-ops-center/internal/scheduler"
)

// validNetworkStatuses mirrors the CHECK constraint on
// network_devices.status. Filter values outside this set produce
// a 400 instead of an empty result so the dashboard can surface
// the typo immediately.
var validNetworkStatuses = map[string]struct{}{
	"up": {}, "down": {}, "partial": {}, "unknown": {},
}

// uuidLikeRe is a cheap shape check for the UUIDs we hand to PG.
// Real validation is the database's job, but we want to answer 400
// for obviously malformed ids instead of letting them reach the
// query layer.
func looksLikeUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			isHex := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
			if !isHex {
				return false
			}
		}
	}
	return true
}

// dudeConfigFromRuntime materializes a dude.Config from server config.
// Returns ok=false when the password is missing — the caller should
// answer 412 not_configured rather than attempt to dial.
func (s *Server) dudeConfigFromRuntime() (dude.Config, bool) {
	c := s.cfg.Dude
	if !c.Configured() {
		return dude.Config{}, false
	}
	return dude.Config{
		Host:               c.Host,
		Port:               c.Port,
		Username:           c.Username,
		Password:           c.Password,
		Timeout:            c.Timeout,
		HostKeyPolicy:      c.HostKeyPolicy,
		HostKeyFingerprint: c.HostKeyFingerprint,
	}, true
}

// dudeKnownHostsStore returns the host-key store backed by Postgres
// when available, and an in-memory fallback otherwise.
func (s *Server) dudeKnownHostsStore() wispssh.KnownHostsStore {
	if s.db != nil && s.db.P != nil {
		return &scheduler.SSHKnownHostsStore{P: s.db.P}
	}
	return wispssh.NewMemoryStore()
}

// handleDudeTestConnection — POST /api/v1/network/discovery/mikrotik-dude/test-connection
func (s *Server) handleDudeTestConnection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	cfg, ok := s.dudeConfigFromRuntime()
	if !ok {
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{
			"error": "not_configured",
			"hint":  "MIKROTIK_DUDE_HOST/USERNAME/PASSWORD eksik; .env değerlerini doldurup servisi yeniden başlatın.",
		})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), cfg.Timeout+5*time.Second)
	defer cancel()
	res := dude.TestConnection(ctx, cfg, s.log, s.dudeKnownHostsStore())
	outcome := audit.OutcomeFailure
	if res.Reachable {
		outcome = audit.OutcomeSuccess
	}
	s.audit(r.Context(), audit.Entry{
		Actor:    actor(r),
		Action:   audit.Action("network.dude.test_connection"),
		Outcome:  outcome,
		Subject:  cfg.Host,
		Metadata: map[string]any{"error_code": res.ErrorCode, "duration_ms": res.DurationMS},
	})
	writeJSON(w, http.StatusOK, res)
}

// handleDudeRunDiscovery — POST /api/v1/network/discovery/mikrotik-dude/run
//
// The actual discovery is launched on a goroutine so the request
// returns immediately with the run id; the dashboard polls
// /runs to observe progress. Only one run runs at a time per
// process; concurrent attempts get 409.
func (s *Server) handleDudeRunDiscovery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	if !s.requireDB(w) {
		return
	}
	cfg, ok := s.dudeConfigFromRuntime()
	if !ok {
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "not_configured"})
		return
	}

	s.dudeRunMu.Lock()
	if s.dudeRunActive {
		s.dudeRunMu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]string{"error": "discovery_already_running"})
		return
	}
	s.dudeRunActive = true
	s.dudeRunMu.Unlock()

	correlationID := dude.NewCorrelationID()
	run, err := s.netInv.CreateRun(r.Context(), correlationID, actor(r))
	if err != nil {
		s.dudeRunMu.Lock()
		s.dudeRunActive = false
		s.dudeRunMu.Unlock()
		s.log.Warn("dude_run_create_failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}

	s.audit(r.Context(), audit.Entry{
		Actor:    actor(r),
		Action:   audit.Action("network.dude.run.start"),
		Outcome:  audit.OutcomeSuccess,
		Subject:  cfg.Host,
		Metadata: map[string]any{"run_id": run.ID, "correlation_id": correlationID},
	})

	go s.runDudeDiscoveryAsync(run.ID, correlationID, cfg)

	writeJSON(w, http.StatusAccepted, map[string]any{
		"run_id":         run.ID,
		"correlation_id": correlationID,
		"status":         "running",
	})
}

// runDudeDiscoveryAsync is the goroutine body. It owns the SSH
// connection lifecycle, persists results, refreshes evidence and
// finalizes the run row. Panics inside Run/UpsertDevices/FinalizeRun
// are recovered so the dudeRunActive flag is always cleared and the
// API process stays up; the run row is marked failed on recovery.
func (s *Server) runDudeDiscoveryAsync(runID, correlationID string, cfg dude.Config) {
	defer func() {
		if rec := recover(); rec != nil {
			s.log.Warn("dude_run_panic_recovered",
				"correlation_id", correlationID,
				"run_id", runID,
				"panic", dude.SanitizeMessage(fmt.Sprintf("%v", rec)),
			)
			// Best-effort terminal state for the run row.
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.netInv.FinalizeRun(ctx, runID, dude.RunResult{
				CorrelationID: correlationID,
				Success:       false,
				ErrorCode:     "panic_recovered",
				Error:         "discovery worker panicked; see server logs",
			})
		}
		s.dudeRunMu.Lock()
		s.dudeRunActive = false
		s.dudeRunMu.Unlock()
	}()
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout+60*time.Second)
	defer cancel()

	res := dude.Run(ctx, cfg, s.log, s.dudeKnownHostsStore())
	res.CorrelationID = correlationID

	_, persistStats, err := s.netInv.UpsertDevices(ctx, runID, res.Devices)
	if err != nil {
		s.log.Warn("dude_upsert_failed", "err", err, "correlation_id", correlationID)
		// HOTFIX v8.4.0 — run state consistency: persist failure must
		// flip success=false so FinalizeRun marks the row failed/partial
		// and the audit event reports failure. success+error_code can
		// never both be true.
		res.Success = false
		if res.Error == "" {
			res.Error = "persist_failed: " + dude.SanitizeMessage(err.Error())
		}
		if res.ErrorCode == "" {
			res.ErrorCode = "persist_failed"
		}
	}

	if err := s.netInv.FinalizeRun(ctx, runID, res); err != nil {
		s.log.Warn("dude_finalize_failed", "err", err, "correlation_id", correlationID)
	}

	finishOutcome := audit.OutcomeFailure
	if res.Success && res.ErrorCode == "" {
		finishOutcome = audit.OutcomeSuccess
	}
	s.audit(ctx, audit.Entry{
		Actor:   "system",
		Action:  audit.Action("network.dude.run.finish"),
		Outcome: finishOutcome,
		Subject: cfg.Host,
		Metadata: map[string]any{
			"run_id":         runID,
			"correlation_id": correlationID,
			"device_count":   res.Stats.Total,
			"inserted_count": persistStats.Inserted,
			"updated_count":  persistStats.Updated,
			"skipped_count":  persistStats.Skipped,
			"error_code":     res.ErrorCode,
		},
	})
}

// handleDiscoveryRuns — GET /api/v1/network/discovery/runs
func (s *Server) handleDiscoveryRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	if !s.requireDB(w) {
		return
	}
	rows, err := s.netInv.ListRuns(r.Context(), 50)
	if err != nil {
		s.log.Warn("dude_list_runs_failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rows})
}

// handleNetworkDevices — GET /api/v1/network/devices?category=&status=&low_confidence=&unknown=
func (s *Server) handleNetworkDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	if !s.requireDB(w) {
		return
	}
	q := r.URL.Query()
	f := networkinv.Filter{
		Category:    q.Get("category"),
		Status:      q.Get("status"),
		OnlyLowConf: q.Get("low_confidence") == "1" || q.Get("low_confidence") == "true",
		OnlyUnknown: q.Get("unknown") == "1" || q.Get("unknown") == "true",
	}
	if f.Category != "" && !dude.IsValidCategory(f.Category) {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid_category",
			"hint":  "AP|BackhaulLink|Bridge|CPE|Router|Switch|Unknown",
		})
		return
	}
	if f.Status != "" {
		if _, ok := validNetworkStatuses[f.Status]; !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "invalid_status",
				"hint":  "up|down|partial|unknown",
			})
			return
		}
	}
	rows, err := s.netInv.ListDevices(r.Context(), f)
	if err != nil {
		s.log.Warn("netdev_list_failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}

	// Summary block — useful for dashboard cards.
	summary := computeSummary(rows)
	writeJSON(w, http.StatusOK, map[string]any{"data": rows, "summary": summary})
}

// handleNetworkDeviceItem — GET /api/v1/network/devices/{id}
func (s *Server) handleNetworkDeviceItem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	if !s.requireDB(w) {
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/network/devices/")
	if id == "" || strings.Contains(id, "/") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_id"})
		return
	}
	if !looksLikeUUID(id) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_id"})
		return
	}
	row, err := s.netInv.GetDevice(r.Context(), id)
	if err != nil {
		if err == networkinv.ErrNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		s.log.Warn("netdev_get_failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": row})
}

// computeSummary tallies the dashboard cards from a device slice.
func computeSummary(rows []networkinv.Device) map[string]int {
	summary := map[string]int{
		"total": len(rows), "ap": 0, "cpe": 0, "bridge": 0, "link": 0,
		"router": 0, "switch": 0, "unknown": 0, "low_confidence": 0,
	}
	for _, d := range rows {
		switch d.Category {
		case dude.CategoryAP:
			summary["ap"]++
		case dude.CategoryCPE:
			summary["cpe"]++
		case dude.CategoryBridge:
			summary["bridge"]++
		case dude.CategoryBackhaul:
			summary["link"]++
		case dude.CategoryRouter:
			summary["router"]++
		case dude.CategorySwitch:
			summary["switch"]++
		default:
			summary["unknown"]++
		}
		if d.Confidence < 50 {
			summary["low_confidence"]++
		}
	}
	return summary
}

// handleNetworkDevicesDispatch routes /api/v1/network/devices and
// /api/v1/network/devices/{id} to the right handler.
func (s *Server) handleNetworkDevicesDispatch(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/network/devices")
	if rest == "" || rest == "/" {
		s.handleNetworkDevices(w, r)
		return
	}
	s.handleNetworkDeviceItem(w, r)
}
