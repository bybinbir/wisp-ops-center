package http

// Phase R1 — "Why is this device classified this way?" drill-down.
//
// GET /api/v1/network/devices/{id}/evidence
//
// Operator clicks a row on /ag-envanteri and asks why it is
// `Bilinmeyen`. This endpoint returns the persisted classification
// trail (`device_category_evidence`), the missing-signal hints, and
// a per-action applicability advisory. It is read-only and runs no
// SSH; everything comes from already-ingested rows.

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/wisp-ops-center/wisp-ops-center/internal/dude"
	"github.com/wisp-ops-center/wisp-ops-center/internal/networkinv"
)

type deviceEvidenceResponse struct {
	Device            *networkinv.Device    `json:"device"`
	Evidence          []evidenceRow         `json:"evidence"`
	EvidenceSummary   evidenceSummary       `json:"evidence_summary"`
	MissingSignals    []missingSignal       `json:"missing_signals"`
	ApplicableActions []actionApplicability `json:"applicable_actions"`
	RecentActions     []recentAction        `json:"recent_actions"`
	GeneratedAt       time.Time             `json:"generated_at"`
}

type evidenceRow struct {
	ID        int64     `json:"id"`
	Heuristic string    `json:"heuristic"`
	Category  string    `json:"category"`
	Weight    int       `json:"weight"`
	Reason    string    `json:"reason"`
	RunID     string    `json:"run_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type evidenceSummary struct {
	TotalRows        int            `json:"total_rows"`
	UniqueHeuristics []string       `json:"unique_heuristics"`
	WeightByCategory map[string]int `json:"weight_by_category"`
	Winner           string         `json:"winner"`
	WinnerWeight     int            `json:"winner_weight"`
	RunnerUp         string         `json:"runner_up,omitempty"`
	RunnerUpWeight   int            `json:"runner_up_weight,omitempty"`
}

type missingSignal struct {
	Signal      string `json:"signal"`
	Explanation string `json:"explanation"`
	WouldHelp   string `json:"would_help"`
}

type actionApplicability struct {
	Kind         string `json:"kind"`
	Suffix       string `json:"suffix"`
	Label        string `json:"label"`
	Applicable   string `json:"applicable"` // "likely_yes" | "likely_no" | "unknown"
	Reason       string `json:"reason"`
	SafetyStatus string `json:"safety_status"` // always "read_only_dry_run" for the four R1 kinds
}

type recentAction struct {
	ID         string    `json:"id"`
	ActionType string    `json:"action_type"`
	Status     string    `json:"status"`
	StartedAt  time.Time `json:"started_at"`
	DurationMS int       `json:"duration_ms"`
	Confidence int       `json:"confidence"`
	DryRun     bool      `json:"dry_run"`
	Skipped    bool      `json:"skipped"`
	SkipReason string    `json:"skip_reason,omitempty"`
}

// handleNetworkDeviceEvidence — GET /api/v1/network/devices/{id}/evidence
func (s *Server) handleNetworkDeviceEvidence(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	if !s.requireDB(w) {
		return
	}
	id := extractDeviceIDForEvidence(r.URL.Path)
	if id == "" || !looksLikeUUID(id) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_id"})
		return
	}

	ctx := r.Context()
	dev, err := s.netInv.GetDevice(ctx, id)
	if err != nil {
		if err == networkinv.ErrNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		s.log.Warn("device_evidence_get_failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}

	rows, err := s.deviceEvidenceRows(ctx, id)
	if err != nil {
		s.log.Warn("device_evidence_rows_failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}

	resp := deviceEvidenceResponse{
		Device:            dev,
		Evidence:          rows,
		EvidenceSummary:   summarizeEvidence(rows),
		MissingSignals:    deriveMissingSignals(dev),
		ApplicableActions: deriveActionApplicability(dev),
		GeneratedAt:       time.Now().UTC(),
	}

	if recent, rErr := s.deviceRecentActions(ctx, id, 10); rErr == nil {
		resp.RecentActions = recent
	}

	writeJSON(w, http.StatusOK, resp)
}

// extractDeviceIDForEvidence pulls {id} out of
// /api/v1/network/devices/{id}/evidence. Defensive against odd
// trailing slashes.
func extractDeviceIDForEvidence(p string) string {
	rest := strings.TrimPrefix(p, "/api/v1/network/devices/")
	// Trim trailing slash before evidence suffix so both
	// /evidence and /evidence/ resolve to the same id.
	rest = strings.TrimSuffix(rest, "/")
	rest = strings.TrimSuffix(rest, "/evidence")
	if strings.Contains(rest, "/") {
		return ""
	}
	return rest
}

// deviceEvidenceRows returns the device_category_evidence trail for
// one device, newest first. We intentionally do NOT join run rows —
// the run_id alone is enough for the operator to cross-reference if
// they want; otherwise the discovery_runs page covers that.
func (s *Server) deviceEvidenceRows(ctx context.Context, deviceID string) ([]evidenceRow, error) {
	const q = `
SELECT id, heuristic, category, weight, COALESCE(reason,''),
       COALESCE(run_id::text,''), created_at
FROM device_category_evidence
WHERE device_id = $1
ORDER BY created_at DESC, id DESC
LIMIT 100`
	out := []evidenceRow{}
	rows, err := s.db.P.Query(ctx, q, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var e evidenceRow
		if err := rows.Scan(&e.ID, &e.Heuristic, &e.Category, &e.Weight,
			&e.Reason, &e.RunID, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// summarizeEvidence collapses the per-row weights into per-category
// totals and picks the winner / runner-up. This is the same logic
// the classifier uses internally; surfacing it here gives the
// operator a chance to see "AP got 30 weight, Router got 25, Unknown
// commits because nothing crossed the threshold".
func summarizeEvidence(rows []evidenceRow) evidenceSummary {
	out := evidenceSummary{
		WeightByCategory: map[string]int{},
		UniqueHeuristics: []string{},
	}
	out.TotalRows = len(rows)
	seen := map[string]struct{}{}
	for _, e := range rows {
		out.WeightByCategory[e.Category] += e.Weight
		if _, ok := seen[e.Heuristic]; !ok {
			seen[e.Heuristic] = struct{}{}
			out.UniqueHeuristics = append(out.UniqueHeuristics, e.Heuristic)
		}
	}
	// Pick winner + runner-up.
	for cat, w := range out.WeightByCategory {
		switch {
		case w > out.WinnerWeight:
			out.RunnerUp = out.Winner
			out.RunnerUpWeight = out.WinnerWeight
			out.Winner = cat
			out.WinnerWeight = w
		case w > out.RunnerUpWeight:
			out.RunnerUp = cat
			out.RunnerUpWeight = w
		}
	}
	if out.Winner == "" {
		out.Winner = string(dude.CategoryUnknown)
	}
	return out
}

// deriveMissingSignals returns operator-friendly explanations of what
// data we did NOT see for this device. The lab evidence (892/893
// Unknown post-Phase 8.1) suggests the most common gap is missing
// MAC + missing neighbor platform, so those are listed first.
func deriveMissingSignals(d *networkinv.Device) []missingSignal {
	out := []missingSignal{}
	if d.MAC == "" {
		out = append(out, missingSignal{
			Signal:      "mac",
			Explanation: "Bu cihaz için MAC adresi enrich edilemedi (ip/neighbor + dude/probe + dude/service'ten gelmedi).",
			WouldHelp:   "MAC, sınıflandırma kararını çoğu zaman tek başına AP / Bridge'e taşır.",
		})
	}
	if d.Platform == "" {
		out = append(out, missingSignal{
			Signal:      "neighbor_platform",
			Explanation: "/ip/neighbor bu cihaz için platform alanı döndürmedi (RouterOS / SwOS / Mimosa-OS gibi).",
			WouldHelp:   "Platform alanı CPE vs Router ayrımında %20 ağırlığa sahip.",
		})
	}
	if d.Board == "" {
		out = append(out, missingSignal{
			Signal:      "board",
			Explanation: "Cihazın board / model alanı yok (RB951, hAP, Audience vs.).",
			WouldHelp:   "Board adı 'AP' / 'CPE' / 'Switch' family eşlemesi için kullanılır.",
		})
	}
	if d.InterfaceName == "" {
		out = append(out, missingSignal{
			Signal:      "interface_name",
			Explanation: "Cihazın hangi interface üzerinden gözüktüğü kayıt edilmedi.",
			WouldHelp:   "Bridge port adı (ör. 'br-customers') Bridge sınıflandırması için belirleyici.",
		})
	}
	if d.EvidenceSummary == "" {
		out = append(out, missingSignal{
			Signal:      "evidence_summary",
			Explanation: "evidence_summary alanı boş — son discovery enrichment bu cihaz için ek kanıt biriktirmedi.",
			WouldHelp:   "Discovery'yi yeniden çalıştırmak yeni neighbor / probe / service kayıtlarını yakalayabilir.",
		})
	}
	return out
}

// deriveActionApplicability returns a per-kind hint about whether
// running each of the four R1 read-only actions is likely to produce
// useful output. We are deliberately conservative — when we are not
// sure, we say "unknown" rather than "yes". The frontend uses this
// to show the action button colour + tooltip; the action will still
// run if the operator presses it (and will return `skipped` in that
// case, which is honest).
func deriveActionApplicability(d *networkinv.Device) []actionApplicability {
	cat := string(d.Category)
	if d.Confidence < 50 {
		// low-confidence rows leave applicability ambiguous for
		// every kind — operator can still try.
		cat = string(dude.CategoryUnknown)
	}

	out := []actionApplicability{
		applicabilityFor("frequency_check", "frequency-check", "Frekans Kontrol", cat, "AP", "Bridge,Switch,Router,CPE"),
		applicabilityFor("ap_client_test", "ap-client-test", "AP Client Test", cat, "AP", "Bridge,Switch,Router,BackhaulLink"),
		applicabilityFor("link_signal_test", "link-signal-test", "Link Signal Test", cat, "BackhaulLink", "Bridge,Switch,Router,CPE"),
		applicabilityFor("bridge_health_check", "bridge-health-check", "Bridge Health", cat, "Bridge", "AP,Router,CPE,BackhaulLink"),
	}
	return out
}

// applicabilityFor classifies the relationship between a device's
// category and an action's preferred / disqualifying categories.
//
//	expectedCat  : best-fit category (likely_yes if matches)
//	disqualifying: comma-separated list of categories that disqualify
//	               the action (likely_no if matches)
func applicabilityFor(kind, suffix, label, deviceCat, expectedCat, disqualifying string) actionApplicability {
	r := actionApplicability{
		Kind:         kind,
		Suffix:       suffix,
		Label:        label,
		SafetyStatus: "read_only_dry_run",
	}
	if deviceCat == expectedCat {
		r.Applicable = "likely_yes"
		r.Reason = "Cihaz kategorisi (" + deviceCat + ") bu aksiyon için uygun."
		return r
	}
	for _, dq := range strings.Split(disqualifying, ",") {
		if dq == deviceCat {
			r.Applicable = "likely_no"
			r.Reason = "Cihaz kategorisi (" + deviceCat + ") için bu aksiyon büyük olasılıkla `skipped` döner — ilgili menü yok."
			return r
		}
	}
	r.Applicable = "unknown"
	if deviceCat == string(dude.CategoryUnknown) {
		r.Reason = "Cihaz Bilinmeyen — aksiyon denenebilir; sonuç skipped olabilir."
	} else {
		r.Reason = "Bu kategoride aksiyon davranışı belirsiz."
	}
	return r
}

// deviceRecentActions returns up to `limit` most recent action runs
// targeting this device, parsing skip reason out of the result jsonb
// when present. This drives the per-device drill-down "Aksiyon
// Geçmişi" tab in the UI.
func (s *Server) deviceRecentActions(ctx context.Context, deviceID string, limit int) ([]recentAction, error) {
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	const q = `
SELECT id, action_type, status, started_at,
       COALESCE(duration_ms, 0), COALESCE(confidence, 0), dry_run,
       COALESCE(result->>'frequency_check->skipped','false'),
       COALESCE(error_message,'')
FROM network_action_runs
WHERE target_device_id = $1
ORDER BY started_at DESC
LIMIT $2`
	rows, err := s.db.P.Query(ctx, q, deviceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []recentAction{}
	for rows.Next() {
		var a recentAction
		var skipFlag, errMsg string
		if err := rows.Scan(&a.ID, &a.ActionType, &a.Status, &a.StartedAt,
			&a.DurationMS, &a.Confidence, &a.DryRun, &skipFlag, &errMsg); err != nil {
			return nil, err
		}
		a.Skipped = a.Status == "skipped"
		if a.Skipped && errMsg != "" {
			a.SkipReason = errMsg
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
