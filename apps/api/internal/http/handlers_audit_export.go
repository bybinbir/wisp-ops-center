package http

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// handleAuditExport, GET /api/v1/audit/export[.json|.ndjson]
//
// Filtreler: action, actor, date_from, date_to, limit (max 50000).
// Default 5000.
//
// Faz 7 — 90 gün retention politikası dokümante edildi
// (docs/PHASE_007_WORK_ORDERS_REPORTS.md → "Audit retention").
// Otomatik silme bu fazda çalıştırılmıyor; operatörün manuel SQL ile
// veya Faz 8'de scheduler job ile temizlenmesi planlanıyor.
func (s *Server) handleAuditExport(w http.ResponseWriter, r *http.Request) {
	if !s.requireDB(w) {
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	q := r.URL.Query()
	action := q.Get("action")
	actorVal := q.Get("actor")
	dateFromS := q.Get("date_from")
	dateToS := q.Get("date_to")
	limit := 5000
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			if n > 50000 {
				n = 50000
			}
			limit = n
		}
	}

	conds := []string{"1=1"}
	args := []any{}
	add := func(cond string, val any) {
		args = append(args, val)
		conds = append(conds, cond+"$"+strconv.Itoa(len(args)))
	}
	if action != "" {
		add("action = ", action)
	}
	if actorVal != "" {
		add("actor = ", actorVal)
	}
	if dateFromS != "" {
		if t, err := time.Parse(time.RFC3339, dateFromS); err == nil {
			add("at >= ", t)
		}
	}
	if dateToS != "" {
		if t, err := time.Parse(time.RFC3339, dateToS); err == nil {
			add("at <= ", t)
		}
	}
	args = append(args, limit)
	limitArg := strconv.Itoa(len(args))

	rows, err := s.db.P.Query(r.Context(), `
SELECT at, actor, action, COALESCE(subject,''),
       COALESCE(outcome,''), COALESCE(reason,''), metadata
  FROM audit_logs
 WHERE `+strings.Join(conds, " AND ")+`
 ORDER BY at DESC
 LIMIT $`+limitArg, args...)
	if err != nil {
		s.log.Warn("audit_export_failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	defer rows.Close()

	ndjson := strings.HasSuffix(r.URL.Path, ".ndjson")
	if ndjson {
		w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
		w.Header().Set("Content-Disposition",
			"attachment; filename=\"audit-"+time.Now().UTC().Format("2006-01-02")+".ndjson\"")
		enc := json.NewEncoder(w)
		for rows.Next() {
			rec, err := scanAuditRow(rows)
			if err != nil {
				s.log.Warn("audit_export_scan_failed", "err", err)
				return
			}
			_ = enc.Encode(rec)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition",
		"attachment; filename=\"audit-"+time.Now().UTC().Format("2006-01-02")+".json\"")
	out := []map[string]any{}
	for rows.Next() {
		rec, err := scanAuditRow(rows)
		if err != nil {
			s.log.Warn("audit_export_scan_failed", "err", err)
			break
		}
		out = append(out, rec)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(map[string]any{"data": out, "limit": limit})
}

// scanAuditRow, ortak audit satır taraması.
func scanAuditRow(scanner interface {
	Scan(...any) error
}) (map[string]any, error) {
	var (
		at                           time.Time
		actor, action, subj, oc, rsn string
		md                           []byte
	)
	if err := scanner.Scan(&at, &actor, &action, &subj, &oc, &rsn, &md); err != nil {
		return nil, err
	}
	var meta any
	_ = json.Unmarshal(md, &meta)
	return map[string]any{
		"at":       at.UTC().Format(time.RFC3339),
		"actor":    actor,
		"action":   action,
		"subject":  subj,
		"outcome":  oc,
		"reason":   rsn,
		"metadata": meta,
	}, nil
}
