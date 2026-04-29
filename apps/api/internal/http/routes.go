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
	mux.HandleFunc("/api/v1/reports", s.handleReportsRoot)
	mux.HandleFunc("/api/v1/reports/", s.handleReportsDispatch)
	mux.HandleFunc("/api/v1/frequency-recommendations", stub("frequency_recommendations.list"))
	mux.HandleFunc("/api/v1/audit-logs", s.handleAuditLogs)
	mux.HandleFunc("/api/v1/audit/export", s.handleAuditExport)
	mux.HandleFunc("/api/v1/audit/export.json", s.handleAuditExport)
	mux.HandleFunc("/api/v1/audit/export.ndjson", s.handleAuditExport)

	mux.HandleFunc("/api/v1/mikrotik/poll-results", s.handleMikrotikPollResults)
	mux.HandleFunc("/api/v1/mimosa/poll-results", s.handleMimosaPollResults)

	// Faz 6 — Skorlama
	mux.HandleFunc("/api/v1/scoring/run", s.handleScoringRun)
	mux.HandleFunc("/api/v1/scoring-thresholds", s.handleScoringThresholds)
	mux.HandleFunc("/api/v1/work-order-candidates", s.handleWorkOrderCandidates)
	mux.HandleFunc("/api/v1/work-order-candidates/", s.handleWorkOrderCandidatesItemDispatch)

	// Faz 7 — Gerçek iş emirleri + raporlar
	mux.HandleFunc("/api/v1/work-orders", s.handleWorkOrdersCollection)
	mux.HandleFunc("/api/v1/work-orders/", s.handleWorkOrderItem)

	// Faz 8 — MikroTik Dude SSH discovery + Network Inventory
	mux.HandleFunc("/api/v1/network/discovery/mikrotik-dude/test-connection", s.handleDudeTestConnection)
	mux.HandleFunc("/api/v1/network/discovery/mikrotik-dude/run", s.handleDudeRunDiscovery)
	mux.HandleFunc("/api/v1/network/discovery/runs", s.handleDiscoveryRuns)
	mux.HandleFunc("/api/v1/network/devices", s.handleNetworkDevicesDispatch)
	mux.HandleFunc("/api/v1/network/devices/", s.handleNetworkDevicesDispatch)

	// Faz 9 — Read-only network actions (frequency_check)
	mux.HandleFunc("/api/v1/network/actions", s.handleNetworkActionsDispatch)
	mux.HandleFunc("/api/v1/network/actions/", s.handleNetworkActionsDispatch)

	// Faz 10B — Postgres-backed safety stores + API surface.
	// Preflight + toggle + maintenance windows. NEVER runs an SSH
	// command; toggle flip records intent only.
	mux.HandleFunc("/api/v1/network/actions/preflight", s.handleSafetyPreflight)
	mux.HandleFunc("/api/v1/network/actions/toggle", s.handleSafetyToggle)
	mux.HandleFunc("/api/v1/network/actions/maintenance-windows", s.handleSafetyMaintenanceWindowsDispatch)
	mux.HandleFunc("/api/v1/network/actions/maintenance-windows/", s.handleSafetyMaintenanceWindowsDispatch)

	// Faz 10C — Destructive runtime lifecycle. NEVER reaches
	// action.Execute. Every endpoint runs through the pre-gate +
	// emits the lifecycle audit catalog. Live execution is blocked
	// by design while DestructiveActionEnabled stays false.
	mux.HandleFunc("/api/v1/network/actions/destructive/", s.handleDestructiveDispatch)
	mux.HandleFunc("/api/v1/network/actions/lifecycle/", s.handleActionLifecycle)

	// Phase R1 — Operator-usable dashboard. Read-only.
	//   /operations-panel               — discovery + actions + safety + health summary
	// Per-device evidence drill-down is dispatched from the existing
	// /api/v1/network/devices/{id}/evidence path-tail (see
	// handleNetworkDevicesDispatch); no new mux entry needed.
	mux.HandleFunc("/api/v1/dashboard/operations-panel", s.handleOperationsPanel)
}

// handleReportsRoot, /api/v1/reports kökü — snapshot listesi.
func (s *Server) handleReportsRoot(w http.ResponseWriter, r *http.Request) {
	if !s.reportsAvailable(w) {
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	rt := r.URL.Query().Get("type")
	limit := 50
	rows, err := s.reports.ListSnapshots(r.Context(), rt, limit)
	if err != nil {
		s.log.Warn("reports_list_failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rows})
}

// handleReportsDispatch, /api/v1/reports/<name>[.csv|.pdf] dispatch.
func (s *Server) handleReportsDispatch(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/reports/")
	// extension'ı çıkar.
	name := rest
	if i := strings.LastIndexByte(rest, '.'); i > 0 {
		// .csv / .pdf / .json suffixleri name'e dahil; handler'lar path'den okur.
		name = rest[:i]
	}
	switch name {
	case "executive-summary":
		if strings.HasSuffix(rest, ".pdf") || strings.HasSuffix(rest, ".html") {
			s.handleReportsExecutiveSummaryHTML(w, r)
			return
		}
		s.handleReportsExecutiveSummary(w, r)
	case "problem-customers":
		s.handleReportsProblemCustomers(w, r)
	case "ap-health":
		s.handleReportsAPHealth(w, r)
	case "tower-risk":
		s.handleReportsTowerRisk(w, r)
	case "work-orders":
		s.handleReportsWorkOrders(w, r)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
	}
}

// handleWorkOrderCandidatesItemDispatch, /api/v1/work-order-candidates/{id}[/promote]
// Phase 7: promote alt-yolu eklendi; default davranış Phase 6'daki PATCH'tir.
func (s *Server) handleWorkOrderCandidatesItemDispatch(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/work-order-candidates/")
	if rest == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	parts := strings.SplitN(rest, "/", 2)
	id := parts[0]
	if len(parts) == 2 && parts[1] == "promote" {
		s.handleWorkOrderCandidatePromote(w, r, id)
		return
	}
	s.handleWorkOrderCandidateItem(w, r)
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
		"phase":      7,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"go_version": runtime.Version(),
		"db":         dbStatus,
		"vault":      vaultStatus,
		"safety": map[string]any{
			"write_disabled":              true,
			"frequency_apply_blocked":     true,
			"mimosa_readonly_only":        true,
			"mikrotik_readonly_only":      true,
			"high_risk_tests_blocked":     true,
			"controlled_apply_blocked":    true,
			"scoring_is_rule_based_no_ml": true,
			"work_orders_candidate_only":  true,
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
		"phase":   7,
		"routes": []string{
			"/api/v1/health",
			"/api/v1/sites",
			"/api/v1/towers",
			"/api/v1/towers/{id}/risk-score",
			"/api/v1/links",
			"/api/v1/customers",
			"/api/v1/customers/{id}/calculate-score",
			"/api/v1/customers/{id}/signal-score",
			"/api/v1/customers/{id}/signal-history",
			"/api/v1/customers/{id}/create-work-order-from-score",
			"/api/v1/customers-with-issues",
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
			"/api/v1/devices/{id}/ap-health",
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
			"/api/v1/scoring/run",
			"/api/v1/scoring-thresholds",
			"/api/v1/work-order-candidates",
			"/api/v1/work-order-candidates/{id}",
			"/api/v1/work-order-candidates/{id}/promote",
			"/api/v1/work-orders",
			"/api/v1/work-orders/{id}",
			"/api/v1/work-orders/{id}/events",
			"/api/v1/work-orders/{id}/assign",
			"/api/v1/work-orders/{id}/resolve",
			"/api/v1/work-orders/{id}/cancel",
			"/api/v1/reports",
			"/api/v1/reports/executive-summary",
			"/api/v1/reports/executive-summary.pdf",
			"/api/v1/reports/problem-customers",
			"/api/v1/reports/problem-customers.csv",
			"/api/v1/reports/ap-health",
			"/api/v1/reports/ap-health.csv",
			"/api/v1/reports/tower-risk",
			"/api/v1/reports/tower-risk.csv",
			"/api/v1/reports/work-orders",
			"/api/v1/reports/work-orders.csv",
			"/api/v1/reports/work-orders.pdf",
			"/api/v1/audit/export",
			"/api/v1/audit/export.json",
			"/api/v1/audit/export.ndjson",
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
