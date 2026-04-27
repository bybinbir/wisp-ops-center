package http

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wisp-ops-center/wisp-ops-center/internal/reports"
)

// reportsAvailable, dependency hazır mı?
func (s *Server) reportsAvailable(w http.ResponseWriter) bool {
	if s.reports == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "database_not_configured",
			"hint":  "WISP_DATABASE_URL ayarlayıp migration uygulayın",
		})
		return false
	}
	return true
}

// parseFilter, yaygın query stringleri filtreye dönüştürür.
func parseReportsFilter(q map[string][]string) reports.ReportsFilter {
	get := func(k string) string {
		if v, ok := q[k]; ok && len(v) > 0 {
			return v[0]
		}
		return ""
	}
	f := reports.ReportsFilter{
		Severity:   get("severity"),
		Diagnosis:  get("diagnosis"),
		TowerID:    get("tower_id"),
		APDeviceID: get("ap_device_id"),
		CustomerID: get("customer_id"),
		Status:     get("status"),
		Priority:   get("priority"),
		AssignedTo: get("assigned_to"),
		OnlyStale:  get("stale") == "true",
	}
	if v := get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			f.Limit = n
		}
	}
	if v := get("date_from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.DateFrom = &t
		}
	}
	if v := get("date_to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.DateTo = &t
		}
	}
	return f
}

// handleReportsExecutiveSummary, GET /api/v1/reports/executive-summary
func (s *Server) handleReportsExecutiveSummary(w http.ResponseWriter, r *http.Request) {
	if !s.reportsAvailable(w) {
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	es, err := s.reports.BuildExecutiveSummary(r.Context())
	if err != nil {
		s.log.Warn("exec_summary_failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": es})
}

// handleReportsExecutiveSummaryHTML, GET /api/v1/reports/executive-summary.pdf
// (HTML-printable; tarayıcıdan PDF kaydedilir).
func (s *Server) handleReportsExecutiveSummaryHTML(w http.ResponseWriter, r *http.Request) {
	if !s.reportsAvailable(w) {
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	es, err := s.reports.BuildExecutiveSummary(r.Context())
	if err != nil {
		s.log.Warn("exec_summary_html_failed", "err", err)
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Disposition",
		"inline; filename=\"yonetici-ozeti-"+time.Now().UTC().Format("2006-01-02")+".html\"")
	if err := reports.RenderExecutiveSummaryHTML(w, es); err != nil {
		s.log.Warn("exec_summary_html_render_failed", "err", err)
	}
}

// handleReportsProblemCustomers — GET /api/v1/reports/problem-customers
// + .csv variant.
func (s *Server) handleReportsProblemCustomers(w http.ResponseWriter, r *http.Request) {
	if !s.reportsAvailable(w) {
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	csvMode := strings.HasSuffix(r.URL.Path, ".csv")
	f := parseReportsFilter(r.URL.Query())
	rows, err := s.reports.ProblemCustomers(r.Context(), f)
	if err != nil {
		s.log.Warn("problem_customers_failed", "err", err)
		if csvMode {
			http.Error(w, "internal", http.StatusInternalServerError)
		} else {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		}
		return
	}
	if csvMode {
		writeCSVResponse(w, "sorunlu-musteriler", func(out http.ResponseWriter) error {
			return reports.ProblemCustomersCSV(out, rows)
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rows})
}

// handleReportsAPHealth — GET /api/v1/reports/ap-health + .csv
func (s *Server) handleReportsAPHealth(w http.ResponseWriter, r *http.Request) {
	if !s.reportsAvailable(w) {
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	csvMode := strings.HasSuffix(r.URL.Path, ".csv")
	f := parseReportsFilter(r.URL.Query())
	rows, err := s.reports.APHealth(r.Context(), f)
	if err != nil {
		s.log.Warn("ap_health_report_failed", "err", err)
		if csvMode {
			http.Error(w, "internal", http.StatusInternalServerError)
		} else {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		}
		return
	}
	if csvMode {
		writeCSVResponse(w, "ap-health", func(out http.ResponseWriter) error {
			return reports.APHealthCSV(out, rows)
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rows})
}

// handleReportsTowerRisk — GET /api/v1/reports/tower-risk + .csv
func (s *Server) handleReportsTowerRisk(w http.ResponseWriter, r *http.Request) {
	if !s.reportsAvailable(w) {
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	csvMode := strings.HasSuffix(r.URL.Path, ".csv")
	f := parseReportsFilter(r.URL.Query())
	rows, err := s.reports.TowerRisk(r.Context(), f)
	if err != nil {
		s.log.Warn("tower_risk_report_failed", "err", err)
		if csvMode {
			http.Error(w, "internal", http.StatusInternalServerError)
		} else {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		}
		return
	}
	if csvMode {
		writeCSVResponse(w, "kule-risk", func(out http.ResponseWriter) error {
			return reports.TowerRiskCSV(out, rows)
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rows})
}

// handleReportsWorkOrders — GET /api/v1/reports/work-orders + .csv + .pdf
func (s *Server) handleReportsWorkOrders(w http.ResponseWriter, r *http.Request) {
	if !s.reportsAvailable(w) {
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	path := r.URL.Path
	csvMode := strings.HasSuffix(path, ".csv")
	htmlMode := strings.HasSuffix(path, ".pdf")
	f := parseReportsFilter(r.URL.Query())
	rows, err := s.reports.WorkOrdersReport(r.Context(), f)
	if err != nil {
		s.log.Warn("wo_report_failed", "err", err)
		if csvMode || htmlMode {
			http.Error(w, "internal", http.StatusInternalServerError)
		} else {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		}
		return
	}
	if csvMode {
		writeCSVResponse(w, "is-emirleri", func(out http.ResponseWriter) error {
			return reports.WorkOrdersCSV(out, rows)
		})
		return
	}
	if htmlMode {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Disposition",
			"inline; filename=\"is-emirleri-"+time.Now().UTC().Format("2006-01-02")+".html\"")
		ctx := reports.WorkOrdersHTMLContext{
			GeneratedAt: time.Now().UTC(),
			Rows:        rows,
			Filter:      f,
		}
		if err := reports.RenderWorkOrdersHTML(w, ctx); err != nil {
			s.log.Warn("wo_report_html_render_failed", "err", err)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rows})
}

// writeCSVResponse, ortak CSV header ve filename üretici.
func writeCSVResponse(w http.ResponseWriter, basename string, write func(http.ResponseWriter) error) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition",
		"attachment; filename=\""+basename+"-"+time.Now().UTC().Format("2006-01-02")+".csv\"")
	if err := write(w); err != nil {
		// Header zaten gönderildiği için content yazmıyoruz; istemci yarım dosya görür.
		return
	}
}
