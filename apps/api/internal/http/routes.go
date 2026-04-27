package http

import (
	"net/http"
	"runtime"
	"strings"
	"time"
)

func (s *Server) routes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/health", s.handleHealth)
	mux.HandleFunc("/", s.handleRoot)

	mux.HandleFunc("/api/v1/sites", s.handleSites)
	mux.HandleFunc("/api/v1/towers", s.handleTowers)
	mux.HandleFunc("/api/v1/towers/", s.handleTowerRiskScore)
	mux.HandleFunc("/api/v1/links", s.handleLinks)
	mux.HandleFunc("/api/v1/customers", s.handleCustomers)
	mux.HandleFunc("/api/v1/customers/", s.handleCustomersItemDispatch)
	mux.HandleFunc("/api/v1/customers-with-issues", s.handleCustomersWithIssues)

	mux.HandleFunc("/api/v1/devices", s.handleDevicesCollection)
	mux.HandleFunc("/api/v1/devices/", s.handleDevicesItemDispatch)

	mux.HandleFunc("/api/v1/credential-profiles", s.handleCredCollection)
	mux.HandleFunc("/api/v1/credential-profiles/", s.handleCredItem)

	mux.HandleFunc("/api/v1/scheduled-checks", s.handleScheduledChecks)
	mux.HandleFunc("/api/v1/scheduled-checks/", s.handleScheduledCheckItem)
	mux.HandleFunc("/api/v1/job-runs", s.handleJobRuns)
	mux.HandleFunc("/api/v1/maintenance-windows", s.handleMaintenanceWindows)
	mux.HandleFunc("/api/v1/maintenance-windows/", s.handleMaintenanceWindowItem)
	mux.HandleFunc("/api/v1/ap-client-test-runs/run-now", s.handleAPClientRunNow)
	mux.HandleFunc("/api/v1/ap-client-test-results", s.handleAPClientResults)
	mux.HandleFunc("/api/v1/reports", stub("reports.list"))
	mux.HandleFunc("/api/v1/frequency-recommendations", stub("frequency_recommendations.list"))
	mux.HandleFunc("/api/v1/audit-logs", s.handleAuditLogs)

	mux.HandleFunc("/api/v1/mikrotik/poll-results", s.handleMikrotikPollResults)
	mux.HandleFunc("/api/v1/mimosa/poll-results", s.handleMimosaPollResults)

	// Faz 6 — Skorlama
	mux.HandleFunc("/api/v1/scoring/run", s.handleScoringRun)
	mux.HandleFunc("/api/v1/scoring-thresholds", s.handleScoringThresholds)
	mux.HandleFunc("/api/v1/work-order-candidates", s.handleWorkOrderCandidates)
	mux.HandleFunc("/api/v1/work-order-candidates/", s.handleWorkOrderCandidateItem)
}

// handleCustomersItemDispatch, /api/v1/customers/{id}/{action} dispatch.
// {action} olmazsa müşteri kalemini handle etmek henüz yok; basit not_found döner.
func (s *Server) handleCustomersItemDispatch(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/customers/")
	if rest == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	if strings.Contains(rest, "/") {
		s.handleCustomerScoreSubpath(w, r)
		return
	}
	// /customers/{id} → şimdilik 404 (Faz 7 detay endpoint'i)
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
}

// handleDevicesItemDispatch demultiplexes the /devices/{id} sub-tree.
func (s *Server) handleDevicesItemDispatch(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasSuffix(r.URL.Path, "/probe"):
		s.handleDeviceProbe(w, r)
	case strings.HasSuffix(r.URL.Path, "/poll"):
		s.handleDevicePoll(w, r)
	case strings.HasSuffix(r.URL.Path, "/telemetry/latest"):
		s.handleDeviceTelemetryLatest(w, r)
	case strings.HasSuffix(r.URL.Path, "/wireless-clients/latest"):
		s.handleDeviceWirelessClientsLatest(w, r)
	case strings.HasSuffix(r.URL.Path, "/interfaces/latest"):
		s.handleDeviceInterfacesLatest(w, r)
	case strings.HasSuffix(r.URL.Path, "/mimosa/latest"):
		s.handleDeviceMimosaLatest(w, r)
	case strings.HasSuffix(r.URL.Path, "/credentials"):
		s.handleDeviceCredentials(w, r)
	case strings.Contains(r.URL.Path, "/credentials/"):
		s.handleDeviceCredentialDelete(w, r)
	case strings.HasSuffix(r.URL.Path, "/ap-health"):
		id := pathID(r.URL.Path, "/api/v1/devices/")
		s.handleDeviceAPHealth(w, r, id)
	default:
		s.handleDevicesItem(w, r)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	dbStatus := "disabled"
	if s.db != nil && s.db.P != nil {
		if err := s.db.Ping(r.Context()); err != nil {
			dbStatus = "degraded"
		} else {
			dbStatus = "ok"
		}
	}
	vaultStatus := "noop"
	if s.cfg.Vault.Key != "" {
		vaultStatus = "ready"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":     "ok",
		"service":    "wisp-ops-api",
		"phase":      5,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"go_version": runtime.Version(),
		"db":         dbStatus,
		"vault":      vaultStatus,
		"safety": map[string]any{
			"write_disabled":           true,
			"frequency_apply_blocked":  true,
			"mimosa_readonly_only":     true,
			"mikrotik_readonly_only":   true,
			"high_risk_tests_blocked":  true,
			"controlled_apply_blocked": true,
		},
	})
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"service": "wisp-ops-center API",
		"phase":   5,
		"routes": []string{
			"/api/v1/health",
			"/api/v1/sites",
			"/api/v1/towers",
			"/api/v1/links",
			"/api/v1/customers",
			"/api/v1/devices",
			"/api/v1/devices/{id}",
			"/api/v1/devices/{id}/probe",
			"/api/v1/devices/{id}/poll",
			"/api/v1/devices/{id}/telemetry/latest",
			"/api/v1/devices/{id}/wireless-clients/latest",
			"/api/v1/devices/{id}/interfaces/latest",
			"/api/v1/devices/{id}/mimosa/latest",
			"/api/v1/devices/{id}/credentials",
			"/api/v1/devices/{id}/credentials/{credential_profile_id}",
			"/api/v1/credential-profiles",
			"/api/v1/credential-profiles/{id}",
			"/api/v1/scheduled-checks",
			"/api/v1/scheduled-checks/{id}",
			"/api/v1/scheduled-checks/{id}/run-now",
			"/api/v1/job-runs",
			"/api/v1/maintenance-windows",
			"/api/v1/maintenance-windows/{id}",
			"/api/v1/ap-client-test-runs/run-now",
			"/api/v1/ap-client-test-results",
			"/api/v1/reports",
			"/api/v1/frequency-recommendations",
			"/api/v1/audit-logs",
			"/api/v1/mikrotik/poll-results",
			"/api/v1/mimosa/poll-results",
		},
	})
}

func pathID(p, prefix string) string {
	rest := strings.TrimPrefix(p, prefix)
	if i := strings.Index(rest, "/"); i >= 0 {
		rest = rest[:i]
	}
	return rest
}
