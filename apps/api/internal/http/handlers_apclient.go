package http

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wisp-ops-center/wisp-ops-center/internal/apclienttest"
	"github.com/wisp-ops-center/wisp-ops-center/internal/audit"
)

// /api/v1/ap-client-test-runs/run-now
func (s *Server) handleAPClientRunNow(w http.ResponseWriter, r *http.Request) {
	if !s.requireDB(w) {
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	var in struct {
		APDeviceID       string `json:"ap_device_id"`
		CustomerID       string `json:"customer_id,omitempty"`
		CustomerDeviceID string `json:"customer_device_id,omitempty"`
		TargetIP         string `json:"target_ip"`
		Type             string `json:"test_type"`
		Count            int    `json:"count,omitempty"`
		TimeoutMS        int    `json:"timeout_ms,omitempty"`
		MaxDurationSec   int    `json:"max_duration_seconds,omitempty"`
		RiskLevel        string `json:"risk_level,omitempty"`
	}
	if err := readJSON(r, &in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	req := apclienttest.TestRequest{
		APDeviceID:       in.APDeviceID,
		CustomerID:       in.CustomerID,
		CustomerDeviceID: in.CustomerDeviceID,
		TargetIP:         in.TargetIP,
		Type:             apclienttest.TestType(in.Type),
		Count:            in.Count,
		Timeout:          time.Duration(in.TimeoutMS) * time.Millisecond,
		MaxDuration:      time.Duration(in.MaxDurationSec) * time.Second,
		RiskLevel:        in.RiskLevel,
	}
	runner := &apclienttest.Runner{}
	ctx, cancel := context.WithTimeout(r.Context(), 70*time.Second)
	defer cancel()
	res := runner.Run(ctx, req)

	if s.db != nil && s.db.P != nil {
		_ = persistAPClientResult(ctx, s.db.P, res)
	}

	s.audit(r.Context(), audit.Entry{
		Actor: actor(r), Action: audit.ActionScheduledCheckRan,
		Subject: "ap_client_test:" + string(res.Type),
		Metadata: map[string]any{
			"event":        "ap_client_run_now",
			"target_ip":    res.TargetIP,
			"status":       res.Status,
			"diagnosis":    string(res.Diagnosis),
			"ap_device_id": res.APDeviceID,
		},
	})
	writeJSON(w, http.StatusOK, map[string]any{"data": res})
}

// /api/v1/ap-client-test-results
func (s *Server) handleAPClientResults(w http.ResponseWriter, r *http.Request) {
	if !s.requireDB(w) {
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	rows, err := s.db.P.Query(r.Context(), `
SELECT id, COALESCE(test_type,''), COALESCE(host(target_ip),''),
       latency_min_ms, latency_avg_ms, latency_max_ms,
       packet_loss_percent, jitter_ms, hop_count,
       COALESCE(diagnosis,''), COALESCE(risk_level,''),
       COALESCE(status,''), COALESCE(error_code,''), COALESCE(error_message,''),
       created_at
  FROM ap_client_test_results ORDER BY created_at DESC LIMIT 200`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var (
			id                                           int64
			testType, target, diag, risk, status, ec, em string
			lmin, lavg, lmax, loss, jit                  *float64
			hops                                         *int
			at                                           time.Time
		)
		if err := rows.Scan(&id, &testType, &target, &lmin, &lavg, &lmax, &loss, &jit, &hops,
			&diag, &risk, &status, &ec, &em, &at); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		out = append(out, map[string]any{
			"id": id, "test_type": testType, "target_ip": target,
			"latency_min_ms": lmin, "latency_avg_ms": lavg, "latency_max_ms": lmax,
			"packet_loss_percent": loss, "jitter_ms": jit, "hop_count": hops,
			"diagnosis": diag, "risk_level": risk, "status": status,
			"error_code": ec, "error_message": em, "created_at": at,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}

func persistAPClientResult(ctx context.Context, p *pgxpool.Pool, res apclienttest.TestResult) error {
	if p == nil {
		return nil
	}
	_, err := p.Exec(ctx, `
INSERT INTO ap_client_test_results
  (run_id, target_ip, test_type, latency_min_ms, latency_avg_ms, latency_max_ms,
   packet_loss_percent, jitter_ms, hop_count, diagnosis, risk_level,
   status, error_code, error_message)
VALUES ($1, NULLIF($2,'')::inet, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NULLIF($13,''), NULLIF($14,''))`,
		nil, // run_id is set when scheduled-check tied test arrives in Phase 6+
		res.TargetIP, string(res.Type),
		floatPtrAny(res.LatencyMinMs), floatPtrAny(res.LatencyAvgMs), floatPtrAny(res.LatencyMaxMs),
		floatPtrAny(res.PacketLossPercent), floatPtrAny(res.JitterMs), intPtrAny(res.HopCount),
		string(res.Diagnosis), res.RiskLevel,
		res.Status, res.ErrorCode, res.ErrorMessage,
	)
	return err
}

func floatPtrAny(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}
func intPtrAny(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}
