package http

// Phase R1 — Operator-usable dashboard endpoint.
//
// The pre-R1 dashboard (/) sourced its KPI cards from
// /api/v1/reports/executive-summary, which only has signal once the
// scoring engine has run against ingested customer telemetry. In the
// real lab against 194.15.45.62 there is no such telemetry, so the
// dashboard rendered "—" everywhere. This endpoint replaces that
// data source with **what we actually have today**: discovery state,
// network-action lifecycle counters, safety-chassis state, and
// reachability health.
//
// Strict invariants (audit, R1):
//   - never emits fake data;
//   - returns explicit `data_insufficient` reasons instead of zeroed
//     fields when a section has nothing to show;
//   - never reaches Execute() of any action;
//   - never flips the destructive master switch;
//   - is read-only (HTTP GET) and does NOT require RBAC capabilities
//     because the operator dashboard must remain accessible even when
//     the role store is empty (Phase R1 ↔ Phase R5 split).

import (
	"context"
	"net/http"
	"time"

	"github.com/wisp-ops-center/wisp-ops-center/internal/networkactions"
)

// opsPanelResponse is the JSON shape returned by GET
// /api/v1/dashboard/operations-panel. Every field is documented so a
// frontend or a curl-using operator can read the schema directly.
type opsPanelResponse struct {
	GeneratedAt time.Time `json:"generated_at"`

	Discovery opsDiscovery `json:"discovery"`
	Actions   opsActions   `json:"actions"`
	Safety    opsSafety    `json:"safety"`
	Health    opsHealth    `json:"health"`

	// DataInsufficient calls out, by area, why a card might still
	// look empty. Each entry has a stable area_code so the frontend
	// can render a localized panel without parsing free text.
	DataInsufficient []opsDataInsufficient `json:"data_insufficient"`
}

type opsDiscovery struct {
	Configured              bool         `json:"configured"`
	LastRun                 *opsRunBrief `json:"last_run,omitempty"`
	LastRunFinishedSecsAgo  *int64       `json:"last_run_finished_seconds_ago,omitempty"`
	Totals                  opsInvTotals `json:"totals"`
	UnknownPercentage       float64      `json:"unknown_percentage"`
	LowConfidencePercentage float64      `json:"low_confidence_percentage"`
}

type opsRunBrief struct {
	ID           string     `json:"id"`
	Status       string     `json:"status"`
	StartedAt    time.Time  `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
	DeviceCount  int        `json:"device_count"`
	ErrorCode    string     `json:"error_code,omitempty"`
	ErrorMessage string     `json:"error_message,omitempty"`
	TriggeredBy  string     `json:"triggered_by"`
}

type opsInvTotals struct {
	Total         int `json:"total"`
	AP            int `json:"ap"`
	Link          int `json:"link"`
	Bridge        int `json:"bridge"`
	CPE           int `json:"cpe"`
	Router        int `json:"router"`
	Switch        int `json:"switch"`
	Unknown       int `json:"unknown"`
	LowConfidence int `json:"low_confidence"`
	WithMAC       int `json:"with_mac"`
	WithHost      int `json:"with_host"`
	Enriched      int `json:"enriched"`
}

type opsActions struct {
	Last24h   opsActionWindow `json:"last_24h"`
	LatestRun *opsActionBrief `json:"latest_run,omitempty"`
}

type opsActionWindow struct {
	Total     int            `json:"total"`
	Succeeded int            `json:"succeeded"`
	Skipped   int            `json:"skipped"`
	Failed    int            `json:"failed"`
	Running   int            `json:"running"`
	ByKind    map[string]int `json:"by_kind"`
}

type opsActionBrief struct {
	ID            string    `json:"id"`
	ActionType    string    `json:"action_type"`
	Status        string    `json:"status"`
	StartedAt     time.Time `json:"started_at"`
	DurationMS    int       `json:"duration_ms"`
	Confidence    int       `json:"confidence"`
	DryRun        bool      `json:"dry_run"`
	TargetLabel   string    `json:"target_label,omitempty"`
	TargetHost    string    `json:"target_host,omitempty"`
	CorrelationID string    `json:"correlation_id"`
}

type opsSafety struct {
	DestructiveEnabled        bool                               `json:"destructive_enabled"`
	ToggleSource              string                             `json:"toggle_source"`
	ActiveMaintenanceWindows  []networkactions.MaintenanceRecord `json:"active_maintenance_windows"`
	BlockingReasons           []string                           `json:"blocking_reasons"`
	DryRunOnly                bool                               `json:"dry_run_only"`
	LegacyMasterSwitchEnabled bool                               `json:"legacy_master_switch_enabled"`
	ProviderToggleEnabled     bool                               `json:"provider_toggle_enabled"`
	LastFlip                  *networkactions.FlipReceipt        `json:"last_flip,omitempty"`
	Checklist                 []string                           `json:"checklist"`
}

type opsHealth struct {
	DBOK                  bool       `json:"db_ok"`
	DudeConfigured        bool       `json:"dude_configured"`
	DudeHost              string     `json:"dude_host,omitempty"`
	LastDudeTestAt        *time.Time `json:"last_dude_test_at,omitempty"`
	LastDudeTestReachable *bool      `json:"last_dude_test_reachable,omitempty"`
	LastDudeTestErrorCode string     `json:"last_dude_test_error_code,omitempty"`
}

type opsDataInsufficient struct {
	AreaCode string `json:"area_code"`
	Title    string `json:"title"`
	Reason   string `json:"reason"`
	Hint     string `json:"hint,omitempty"`
}

// handleOperationsPanel — GET /api/v1/dashboard/operations-panel.
//
// Returns the operator-usable dashboard payload. Read-only, no
// audit-event emission (this is a hot-path call that the dashboard
// can poll), no Execute() reach, no toggle flip.
func (s *Server) handleOperationsPanel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	if !s.requireDB(w) {
		return
	}

	ctx := r.Context()
	now := time.Now().UTC()

	resp := opsPanelResponse{
		GeneratedAt:      now,
		DataInsufficient: []opsDataInsufficient{},
	}

	// ----- Discovery section ------------------------------------------
	resp.Discovery.Configured = s.cfg.Dude.Configured()
	if !resp.Discovery.Configured {
		resp.DataInsufficient = append(resp.DataInsufficient, opsDataInsufficient{
			AreaCode: "discovery_not_configured",
			Title:    "Dude bağlantısı tanımlı değil",
			Reason:   "MIKROTIK_DUDE_HOST/USERNAME/PASSWORD env değerleri eksik. Discovery koşturulamaz.",
			Hint:     ".env dosyasını doldurup API servisini yeniden başlatın.",
		})
	}

	if s.netInv != nil {
		latest, err := s.netInv.LatestRun(ctx)
		switch {
		case err == nil && latest != nil:
			brief := opsRunBrief{
				ID:           latest.ID,
				Status:       latest.Status,
				StartedAt:    latest.StartedAt,
				DeviceCount:  latest.DeviceCount,
				ErrorCode:    latest.ErrorCode,
				ErrorMessage: latest.ErrorMessage,
				TriggeredBy:  latest.TriggeredBy,
			}
			if latest.FinishedAt != nil && !latest.FinishedAt.IsZero() {
				ft := latest.FinishedAt.UTC()
				brief.FinishedAt = &ft
				secs := int64(now.Sub(ft).Seconds())
				resp.Discovery.LastRunFinishedSecsAgo = &secs
			}
			resp.Discovery.LastRun = &brief
		default:
			// no rows yet, or error — we will surface as
			// data_insufficient below.
			s.maybeWarn("ops_panel_latest_run", err)
		}

		// Inventory totals: prefer fast SQL aggregation over
		// ListDevices()+computeSummary() because the live lab has
		// 893+ rows and we do not need the row data here.
		totals, terr := s.opsPanelInventoryTotals(ctx)
		if terr == nil {
			resp.Discovery.Totals = totals
			if totals.Total > 0 {
				resp.Discovery.UnknownPercentage = round1(float64(totals.Unknown) * 100 / float64(totals.Total))
				resp.Discovery.LowConfidencePercentage = round1(float64(totals.LowConfidence) * 100 / float64(totals.Total))
			}
		} else {
			s.maybeWarn("ops_panel_inv_totals", terr)
		}

		if resp.Discovery.LastRun == nil {
			resp.DataInsufficient = append(resp.DataInsufficient, opsDataInsufficient{
				AreaCode: "discovery_no_run",
				Title:    "Henüz discovery koşturulmadı",
				Reason:   "discovery_runs tablosu boş.",
				Hint:     "Ağ Envanteri sayfasından 'Discovery Çalıştır' butonuna basın.",
			})
		}
	}

	// ----- Actions section --------------------------------------------
	if s.actionRepo != nil {
		win, latestAct, err := s.opsPanelActionWindow(ctx, now.Add(-24*time.Hour))
		if err == nil {
			resp.Actions.Last24h = win
			resp.Actions.LatestRun = latestAct
			if win.Total == 0 {
				resp.DataInsufficient = append(resp.DataInsufficient, opsDataInsufficient{
					AreaCode: "actions_no_runs_24h",
					Title:    "Son 24 saatte aksiyon koşturulmadı",
					Reason:   "network_action_runs tablosunda son 24 saate ait satır yok.",
					Hint:     "Ağ Envanteri sayfasında bir cihaz satırının read-only aksiyon butonlarına basın.",
				})
			}
		} else {
			s.maybeWarn("ops_panel_actions", err)
		}
	} else {
		resp.Actions.Last24h.ByKind = map[string]int{}
	}
	if resp.Actions.Last24h.ByKind == nil {
		resp.Actions.Last24h.ByKind = map[string]int{}
	}

	// ----- Safety section ---------------------------------------------
	resp.Safety = s.opsPanelSafetyState(ctx, now)

	// ----- Health section ---------------------------------------------
	resp.Health.DBOK = s.db != nil && s.db.P != nil
	resp.Health.DudeConfigured = s.cfg.Dude.Configured()
	if resp.Health.DudeConfigured {
		resp.Health.DudeHost = s.cfg.Dude.Host
	}
	if t, ok := s.opsPanelLastDudeTest(ctx); ok {
		resp.Health.LastDudeTestAt = &t.At
		reachable := t.Reachable
		resp.Health.LastDudeTestReachable = &reachable
		resp.Health.LastDudeTestErrorCode = t.ErrorCode
	} else {
		resp.DataInsufficient = append(resp.DataInsufficient, opsDataInsufficient{
			AreaCode: "dude_no_test_yet",
			Title:    "Dude bağlantı testi yapılmadı",
			Reason:   "audit_logs içinde network.dude.test_connection kaydı yok.",
			Hint:     "Ağ Envanteri sayfasından 'Bağlantıyı Test Et' butonuna basın.",
		})
	}

	// Always-on insufficiency banners that apply until later phases land.
	if !resp.Safety.LegacyMasterSwitchEnabled || !resp.Safety.ProviderToggleEnabled {
		resp.Safety.DryRunOnly = true
	}
	resp.DataInsufficient = append(resp.DataInsufficient, opsDataInsufficient{
		AreaCode: "scoring_no_telemetry",
		Title:    "Müşteri sinyal skorları henüz hesaplanmadı",
		Reason:   "Telemetry kaynağı (RouterOS poll, SNMP, Mimosa) bu lab DB'sine henüz bağlı değil.",
		Hint:     "Phase R3 ile gerçek wireless RouterOS lab target sağlandığında scoring engine besleyecek.",
	})

	writeJSON(w, http.StatusOK, resp)
}

// opsPanelInventoryTotals runs one aggregation query against
// network_devices and returns the per-category counts. The SQL
// matches the categories defined in migration 000008
// (AP/BackhaulLink/Bridge/CPE/Router/Switch/Unknown) and the
// low_confidence flag computed elsewhere as confidence < 50.
func (s *Server) opsPanelInventoryTotals(ctx context.Context) (opsInvTotals, error) {
	const q = `
SELECT
  COUNT(*)                                                AS total,
  COUNT(*) FILTER (WHERE category = 'AP')                 AS ap,
  COUNT(*) FILTER (WHERE category = 'BackhaulLink')       AS link,
  COUNT(*) FILTER (WHERE category = 'Bridge')             AS bridge,
  COUNT(*) FILTER (WHERE category = 'CPE')                AS cpe,
  COUNT(*) FILTER (WHERE category = 'Router')             AS router,
  COUNT(*) FILTER (WHERE category = 'Switch')             AS sw,
  COUNT(*) FILTER (WHERE category = 'Unknown')            AS unknown,
  COUNT(*) FILTER (WHERE confidence < 50)                 AS low_conf,
  COUNT(*) FILTER (WHERE mac IS NOT NULL AND mac <> '')   AS with_mac,
  COUNT(*) FILTER (WHERE host IS NOT NULL)                AS with_host,
  COUNT(*) FILTER (WHERE last_enriched_at IS NOT NULL)    AS enriched
FROM network_devices`
	var t opsInvTotals
	row := s.db.P.QueryRow(ctx, q)
	if err := row.Scan(&t.Total, &t.AP, &t.Link, &t.Bridge, &t.CPE,
		&t.Router, &t.Switch, &t.Unknown, &t.LowConfidence,
		&t.WithMAC, &t.WithHost, &t.Enriched); err != nil {
		return opsInvTotals{}, err
	}
	return t, nil
}

// opsPanelActionWindow returns the last 24h status histogram and the
// latest (newest started_at) action row. byKind groups frequency_check
// + ap_client_test + link_signal_test + bridge_health_check +
// frequency_correction (shown but never expected to succeed today).
func (s *Server) opsPanelActionWindow(ctx context.Context, since time.Time) (opsActionWindow, *opsActionBrief, error) {
	const qWin = `
SELECT
  COUNT(*) AS total,
  COUNT(*) FILTER (WHERE status = 'succeeded') AS succeeded,
  COUNT(*) FILTER (WHERE status = 'skipped')   AS skipped,
  COUNT(*) FILTER (WHERE status = 'failed')    AS failed,
  COUNT(*) FILTER (WHERE status IN ('queued','running')) AS running
FROM network_action_runs
WHERE started_at >= $1`
	out := opsActionWindow{ByKind: map[string]int{}}
	row := s.db.P.QueryRow(ctx, qWin, since)
	if err := row.Scan(&out.Total, &out.Succeeded, &out.Skipped, &out.Failed, &out.Running); err != nil {
		return out, nil, err
	}

	const qByKind = `
SELECT action_type, COUNT(*)
FROM network_action_runs
WHERE started_at >= $1
GROUP BY action_type`
	rows, err := s.db.P.Query(ctx, qByKind, since)
	if err != nil {
		return out, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		var n int
		if err := rows.Scan(&k, &n); err != nil {
			return out, nil, err
		}
		out.ByKind[k] = n
	}

	// Latest action (any time, any status): newest started_at wins.
	const qLatest = `
SELECT id, action_type, status, started_at,
       COALESCE(duration_ms, 0), COALESCE(confidence, 0), dry_run,
       COALESCE(target_label,''), COALESCE(host(target_host),''), correlation_id
FROM network_action_runs
ORDER BY started_at DESC
LIMIT 1`
	var brief opsActionBrief
	if err := s.db.P.QueryRow(ctx, qLatest).Scan(
		&brief.ID, &brief.ActionType, &brief.Status, &brief.StartedAt,
		&brief.DurationMS, &brief.Confidence, &brief.DryRun,
		&brief.TargetLabel, &brief.TargetHost, &brief.CorrelationID,
	); err != nil {
		// No rows is not an error here — the dashboard will
		// surface "no actions yet".
		return out, nil, nil
	}
	return out, &brief, nil
}

// opsPanelSafetyState returns the same data the /preflight endpoint
// returns minus the audit emission. It is used by the dashboard to
// show the master-switch state and active maintenance windows.
//
// The legacy const DestructiveActionEnabled is read by the gate code
// in `internal/networkactions/phase10_pregate.go`. We surface its
// value here without flipping it. The provider toggle is the
// runtime store (Pg or memory) and is read-only here too.
func (s *Server) opsPanelSafetyState(ctx context.Context, now time.Time) opsSafety {
	out := opsSafety{
		ActiveMaintenanceWindows: []networkactions.MaintenanceRecord{},
		BlockingReasons:          []string{},
		Checklist:                networkactions.PreGateChecklist(),
	}
	out.LegacyMasterSwitchEnabled = networkactions.DestructiveActionEnabled
	if s.actionToggle != nil {
		enabled, terr := s.actionToggle.Enabled(ctx)
		out.ToggleSource = "store"
		out.ProviderToggleEnabled = enabled && terr == nil
		if pg, ok := s.actionToggle.(*networkactions.PgToggleStore); ok && pg != nil {
			out.LastFlip, _ = pg.LastFlip(ctx)
		} else if mem, ok := s.actionToggle.(*networkactions.MemoryToggle); ok && mem != nil {
			out.LastFlip = mem.LastFlip()
		}
	} else {
		out.ToggleSource = "none"
	}
	out.DestructiveEnabled = out.LegacyMasterSwitchEnabled && out.ProviderToggleEnabled

	if s.actionWindowsProv != nil {
		w, werr := s.actionWindowsProv.ActiveAt(ctx, "", now)
		if werr == nil && w != nil {
			out.ActiveMaintenanceWindows = w
		}
	}

	if !out.LegacyMasterSwitchEnabled {
		out.BlockingReasons = append(out.BlockingReasons, "legacy_master_switch_disabled")
	}
	if !out.ProviderToggleEnabled {
		out.BlockingReasons = append(out.BlockingReasons, "provider_toggle_disabled")
	}
	if len(out.ActiveMaintenanceWindows) == 0 {
		out.BlockingReasons = append(out.BlockingReasons, "no_active_maintenance_window")
	}
	out.DryRunOnly = !out.DestructiveEnabled
	return out
}

type opsDudeTestPing struct {
	At        time.Time
	Reachable bool
	ErrorCode string
}

// opsPanelLastDudeTest pulls the most recent
// `network.dude.test_connection` row from audit_logs. We use this as
// a cheap "was Dude reachable when last asked?" indicator without
// dialing SSH from the dashboard hot-path.
func (s *Server) opsPanelLastDudeTest(ctx context.Context) (opsDudeTestPing, bool) {
	const q = `
SELECT at, outcome, COALESCE(metadata->>'error_code','')
FROM audit_logs
WHERE action = 'network.dude.test_connection'
ORDER BY at DESC
LIMIT 1`
	var p opsDudeTestPing
	var outcome string
	if err := s.db.P.QueryRow(ctx, q).Scan(&p.At, &outcome, &p.ErrorCode); err != nil {
		return opsDudeTestPing{}, false
	}
	p.Reachable = outcome == "success"
	return p, true
}

// maybeWarn logs at warn level only when err is non-nil. Centralised
// so the operations-panel handler stays terse.
func (s *Server) maybeWarn(component string, err error) {
	if err == nil {
		return
	}
	s.log.Warn(component, "err", err)
}

// round1 truncates a float to one decimal place. Used for the
// percentage cards on the dashboard.
func round1(v float64) float64 {
	return float64(int(v*10+0.5)) / 10
}
