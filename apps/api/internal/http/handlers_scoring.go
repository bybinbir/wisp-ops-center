package http

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/wisp-ops-center/wisp-ops-center/internal/scoring"
)

// scoringAvailable, dependency'ler hazır mı?
func (s *Server) scoringAvailable(w http.ResponseWriter) bool {
	if s.scoring == nil || s.hydrate == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "database_not_configured",
			"hint":  "WISP_DATABASE_URL ayarlayıp migration uygulayın",
		})
		return false
	}
	return true
}

// =====================================================================
// /customers/{id}/calculate-score : POST → tek müşteri skor hesapla
// /customers/{id}/signal-score    : GET  → en son skor satırı
// /customers/{id}/signal-history  : GET  → geçmiş
// /customers/{id}/create-work-order-from-score : POST
// =====================================================================

// handleCustomerScoreSubpath, /api/v1/customers/{id}/{action} dispatch.
func (s *Server) handleCustomerScoreSubpath(w http.ResponseWriter, r *http.Request) {
	// path: /api/v1/customers/{id}/{action}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/customers/"), "/")
	if len(parts) < 2 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	id, action := parts[0], parts[1]
	if !s.scoringAvailable(w) {
		return
	}
	switch action {
	case "calculate-score":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
			return
		}
		s.handleCalculateScore(w, r, id)
	case "signal-score":
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
			return
		}
		row, err := s.scoring.LatestCustomerScore(r.Context(), id)
		if err != nil {
			if errors.Is(err, scoring.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
				return
			}
			s.log.Warn("score_latest_failed", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": row})
	case "signal-history":
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
			return
		}
		limit := 50
		if v := r.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				limit = n
			}
		}
		rows, err := s.scoring.CustomerScoreHistory(r.Context(), id, limit)
		if err != nil {
			s.log.Warn("score_history_failed", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": rows})
	case "create-work-order-from-score":
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
			return
		}
		s.handleCreateWorkOrderFromScore(w, r, id)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
	}
}

// handleCalculateScore, hydrate → engine → save.
func (s *Server) handleCalculateScore(w http.ResponseWriter, r *http.Request, customerID string) {
	thr, err := s.scoring.LoadThresholds(r.Context())
	if err != nil {
		s.log.Warn("score_thresholds_load_failed", "err", err)
		thr = scoring.DefaultThresholds()
	}
	hyd, err := s.hydrate.HydrateCustomer(r.Context(), customerID)
	if err != nil {
		if errors.Is(err, scoring.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		s.log.Warn("score_hydrate_failed", "err", err, "customer_id", customerID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	res := scoring.NewEngine(thr).ScoreCustomer(hyd.Inputs)
	id, err := s.scoring.SaveCustomerScore(r.Context(), customerID,
		hyd.APDeviceID, hyd.TowerID, hyd.Inputs, res)
	if err != nil {
		s.log.Warn("score_save_failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"id":                 id,
			"customer_id":        customerID,
			"score":              res.Score,
			"severity":           res.Severity,
			"diagnosis":          res.Diagnosis,
			"recommended_action": res.RecommendedAction,
			"reasons":            res.Reasons,
			"is_stale":           res.IsStale,
			"calculated_at":      res.CalculatedAt,
		},
	})
}

// handleCreateWorkOrderFromScore, son skor → work_order_candidates satırı.
func (s *Server) handleCreateWorkOrderFromScore(w http.ResponseWriter, r *http.Request, customerID string) {
	row, err := s.scoring.LatestCustomerScore(r.Context(), customerID)
	if err != nil {
		if errors.Is(err, scoring.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "no_score_yet"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	cust := customerID
	in := scoring.CreateWorkOrderCandidateInput{
		CustomerID:        &cust,
		APDeviceID:        row.APDeviceID,
		TowerID:           row.TowerID,
		SourceScoreID:     &row.ID,
		Diagnosis:         row.Diagnosis,
		RecommendedAction: row.RecommendedAction,
		Severity:          row.Severity,
		Reasons:           row.Reasons,
	}
	id, err := s.scoring.CreateWorkOrderCandidate(r.Context(), in)
	if err != nil {
		s.log.Warn("woc_create_failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{
		"id":                 id,
		"customer_id":        customerID,
		"diagnosis":          row.Diagnosis,
		"recommended_action": row.RecommendedAction,
		"severity":           row.Severity,
		"status":             "open",
	}})
}

// =====================================================================
// /customers/with-issues : GET
// =====================================================================

func (s *Server) handleCustomersWithIssues(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	if !s.scoringAvailable(w) {
		return
	}
	q := r.URL.Query()
	f := scoring.CustomerWithIssuesFilter{
		Severity:   q.Get("severity"),
		Diagnosis:  q.Get("diagnosis"),
		TowerID:    q.Get("tower_id"),
		APDeviceID: q.Get("ap_device_id"),
		OnlyStale:  q.Get("stale") == "true",
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			f.Limit = n
		}
	}
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			f.Offset = n
		}
	}
	rows, err := s.scoring.CustomersWithIssues(r.Context(), f)
	if err != nil {
		s.log.Warn("with_issues_failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rows})
}

// =====================================================================
// /scoring/run : POST { customer_ids?: [...], all_customers?: bool, all_aps?: bool }
//                Toplu skor üretir; sandbox güvenliği için max 200 müşteri.
// =====================================================================

type scoringRunRequest struct {
	CustomerIDs   []string `json:"customer_ids"`
	AllCustomers  bool     `json:"all_customers"`
	AllAPs        bool     `json:"all_aps"`
	AllTowers     bool     `json:"all_towers"`
	MaxCustomers  int      `json:"max_customers"`
}

type scoringRunResponse struct {
	Processed int      `json:"processed"`
	Errors    []string `json:"errors,omitempty"`
}

func (s *Server) handleScoringRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	if !s.scoringAvailable(w) {
		return
	}
	var req scoringRunRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	if req.MaxCustomers <= 0 || req.MaxCustomers > 200 {
		req.MaxCustomers = 200
	}
	ids := req.CustomerIDs
	if len(ids) == 0 && req.AllCustomers && s.db != nil {
		rows, err := s.db.P.Query(r.Context(),
			`SELECT id::text FROM customers WHERE status = 'active' LIMIT $1`, req.MaxCustomers)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err == nil {
				ids = append(ids, id)
			}
		}
		rows.Close()
	}
	thr, err := s.scoring.LoadThresholds(r.Context())
	if err != nil {
		thr = scoring.DefaultThresholds()
	}
	eng := scoring.NewEngine(thr)
	resp := scoringRunResponse{}
	for _, cid := range ids {
		hyd, err := s.hydrate.HydrateCustomer(r.Context(), cid)
		if err != nil {
			resp.Errors = append(resp.Errors, cid+": "+err.Error())
			continue
		}
		res := eng.ScoreCustomer(hyd.Inputs)
		_, err = s.scoring.SaveCustomerScore(r.Context(), cid,
			hyd.APDeviceID, hyd.TowerID, hyd.Inputs, res)
		if err != nil {
			resp.Errors = append(resp.Errors, cid+": "+err.Error())
			continue
		}
		resp.Processed++
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": resp})
}

// =====================================================================
// /scoring-thresholds : GET / PATCH
// =====================================================================

type thresholdPatchRequest struct {
	Updates map[string]float64 `json:"updates"`
	By      string             `json:"by"`
}

func (s *Server) handleScoringThresholds(w http.ResponseWriter, r *http.Request) {
	if !s.scoringAvailable(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		rows, err := s.scoring.ListThresholds(r.Context())
		if err != nil {
			s.log.Warn("threshold_list_failed", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"data":     rows,
			"defaults": scoring.SeedDefaults(),
		})
	case http.MethodPatch:
		var req thresholdPatchRequest
		if err := readJSON(r, &req); err != nil || len(req.Updates) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
			return
		}
		if req.By == "" {
			req.By = "api"
		}
		updated := 0
		for k, v := range req.Updates {
			if err := s.scoring.UpsertThreshold(r.Context(), k, v, req.By); err != nil {
				s.log.Warn("threshold_upsert_failed", "key", k, "err", err)
				continue
			}
			updated++
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{"updated": updated},
		})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
	}
}

// =====================================================================
// /devices/{id}/ap-health : GET
// /towers/{id}/risk-score : GET
// =====================================================================

func (s *Server) handleDeviceAPHealth(w http.ResponseWriter, r *http.Request, deviceID string) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	if !s.scoringAvailable(w) {
		return
	}
	row, err := s.scoring.LatestAPScore(r.Context(), deviceID)
	if err != nil {
		if errors.Is(err, scoring.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": row})
}

func (s *Server) handleTowerRiskScore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	if !s.scoringAvailable(w) {
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/towers/"), "/")
	if len(parts) < 2 || parts[1] != "risk-score" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	row, err := s.scoring.LatestTowerScore(r.Context(), parts[0])
	if err != nil {
		if errors.Is(err, scoring.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": row})
}

// =====================================================================
// /work-order-candidates : GET / POST(status update)
// =====================================================================

func (s *Server) handleWorkOrderCandidates(w http.ResponseWriter, r *http.Request) {
	if !s.scoringAvailable(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		rows, err := s.scoring.ListWorkOrderCandidates(r.Context(),
			r.URL.Query().Get("status"), 100)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": rows})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
	}
}

func (s *Server) handleWorkOrderCandidateItem(w http.ResponseWriter, r *http.Request) {
	if !s.scoringAvailable(w) {
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/work-order-candidates/"), "/")
	if len(parts) < 1 || parts[0] == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	id := parts[0]
	switch r.Method {
	case http.MethodPatch:
		var body struct {
			Status string  `json:"status"`
			Notes  *string `json:"notes,omitempty"`
		}
		if err := readJSON(r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
			return
		}
		if err := s.scoring.UpdateCandidateStatus(r.Context(), id, body.Status, body.Notes); err != nil {
			if errors.Is(err, scoring.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
				return
			}
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": id, "status": body.Status}})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
	}
}
