// Package telemetry persists MikroTik (and later Mimosa) read-only poll
// results into PostgreSQL. The repository never accepts raw secrets and
// stores only sanitized error messages.
package telemetry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wisp-ops-center/wisp-ops-center/internal/adapters/mikrotik"
)

// ErrNotFound surfaces "no telemetry recorded yet" cleanly to handlers.
var ErrNotFound = errors.New("telemetry: not found")

// Repository wraps the pgx pool with persistence helpers.
type Repository struct {
	P *pgxpool.Pool
}

func NewRepository(p *pgxpool.Pool) *Repository { return &Repository{P: p} }

// PollOperation is the op stored on device_poll_results.
type PollOperation string

const (
	OpProbe PollOperation = "probe"
	OpPoll  PollOperation = "poll"
)

// PollStatus is the status enum for device_poll_results.
type PollStatus string

const (
	StatusSuccess PollStatus = "success"
	StatusFailed  PollStatus = "failed"
	StatusBlocked PollStatus = "blocked"
	StatusPartial PollStatus = "partial"
)

// PollResultInput captures the data we want to persist for a single
// probe/poll execution.
type PollResultInput struct {
	DeviceID     string
	Vendor       string
	Operation    PollOperation
	Transport    string
	Status       PollStatus
	StartedAt    time.Time
	FinishedAt   time.Time
	ErrorCode    string
	ErrorMessage string // already sanitized by adapter
	Summary      map[string]any
}

// PollResultRow is a row from device_poll_results.
type PollResultRow struct {
	ID           int64          `json:"id"`
	DeviceID     string         `json:"device_id"`
	Vendor       string         `json:"vendor"`
	Operation    string         `json:"operation"`
	Transport    string         `json:"transport"`
	Status       string         `json:"status"`
	StartedAt    time.Time      `json:"started_at"`
	FinishedAt   time.Time      `json:"finished_at"`
	DurationMS   int            `json:"duration_ms"`
	ErrorCode    string         `json:"error_code,omitempty"`
	ErrorMessage string         `json:"error_message,omitempty"`
	Summary      map[string]any `json:"summary"`
	CreatedAt    time.Time      `json:"created_at"`
}

// RecordPollResult inserts a row and returns its id.
func (r *Repository) RecordPollResult(ctx context.Context, in PollResultInput) (int64, error) {
	if in.Vendor == "" {
		in.Vendor = "mikrotik"
	}
	if in.Summary == nil {
		in.Summary = map[string]any{}
	}
	dur := in.FinishedAt.Sub(in.StartedAt).Milliseconds()
	if dur < 0 {
		dur = 0
	}
	summaryJSON, _ := json.Marshal(in.Summary)
	var id int64
	err := r.P.QueryRow(ctx, `
INSERT INTO device_poll_results
  (device_id, vendor, operation, transport, status, started_at, finished_at, duration_ms, error_code, error_message, summary)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9,''),NULLIF($10,''),$11)
RETURNING id`,
		in.DeviceID, in.Vendor, string(in.Operation), in.Transport, string(in.Status),
		in.StartedAt, in.FinishedAt, dur, in.ErrorCode, in.ErrorMessage, summaryJSON,
	).Scan(&id)
	if err != nil {
		return 0, err
	}
	// Mirror the latest poll metadata onto devices for fast UI lookups.
	_, _ = r.P.Exec(ctx,
		`UPDATE devices
		    SET last_poll_at = $2,
		        last_poll_status = $3,
		        last_poll_error = NULLIF($4,'')
		  WHERE id = $1`,
		in.DeviceID, in.FinishedAt, string(in.Status), in.ErrorMessage,
	)
	return id, nil
}

// PersistMikroTikSnapshot writes the full snapshot fanout (interfaces +
// wireless interfaces + wireless clients) tied to a poll_result_id.
func (r *Repository) PersistMikroTikSnapshot(ctx context.Context, pollID int64, snap *mikrotik.MikroTikReadOnlySnapshot) error {
	collected := snap.FinishedAt
	if collected.IsZero() {
		collected = time.Now().UTC()
	}

	if _, err := r.P.Exec(ctx, `INSERT INTO telemetry_snapshots
		(device_id, collected_at, online, uptime_sec, cpu_percent, mem_percent, temp_c)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		snap.DeviceID, collected, true,
		nilOrInt64(snap.System, func(s *mikrotik.MikroTikSystemInfo) int64 { return s.UptimeSec }),
		nilOrFloat(snap.System, func(s *mikrotik.MikroTikSystemInfo) *float64 { return s.CPULoadPct }),
		memPercent(snap.System),
		nilOrFloat(snap.System, func(s *mikrotik.MikroTikSystemInfo) *float64 { return s.TempCelsius }),
	); err != nil {
		return fmt.Errorf("telemetry_snapshots: %w", err)
	}

	for _, m := range snap.Interfaces {
		if _, err := r.P.Exec(ctx, `INSERT INTO interface_metrics
			(device_id, poll_result_id, name, type, running, disabled, mtu, mac, rx_bytes, tx_bytes, rx_packets, tx_packets, rx_errors, tx_errors, link_downs, collected_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,NULLIF($8,'')::macaddr,$9,$10,$11,$12,$13,$14,$15,$16)`,
			snap.DeviceID, pollID, m.Name, m.Type, m.Running, m.Disabled, m.MTU, m.MAC,
			m.RxByte, m.TxByte, m.RxPacket, m.TxPacket, m.RxError, m.TxError, m.LinkDownCnt, collected,
		); err != nil {
			return fmt.Errorf("interface_metrics: %w", err)
		}
	}

	for _, w := range snap.WirelessInterfaces {
		if _, err := r.P.Exec(ctx, `INSERT INTO mikrotik_wireless_interfaces
			(device_id, poll_result_id, name, ssid, mode, band, frequency_mhz, channel_width_mhz, tx_power_dbm, noise_floor_dbm, disabled, running, collected_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
			snap.DeviceID, pollID, w.Name, w.SSID, w.Mode, w.Band,
			intPtrToAny(w.FrequencyMHz), intPtrToAny(w.ChannelWidthMHz),
			floatPtrToAny(w.TxPowerDBm), floatPtrToAny(w.NoiseFloor),
			w.Disabled, w.Running, collected,
		); err != nil {
			return fmt.Errorf("mikrotik_wireless_interfaces: %w", err)
		}
	}

	for _, c := range snap.WirelessClients {
		if _, err := r.P.Exec(ctx, `INSERT INTO mikrotik_wireless_clients
			(device_id, poll_result_id, interface_name, mac, ip, ssid, uptime_sec, signal_dbm, snr_db, tx_rate_mbps, rx_rate_mbps, ccq, collected_at)
			VALUES ($1,$2,$3,NULLIF($4,'')::macaddr,NULLIF($5,'')::inet,$6,$7,$8,$9,$10,$11,$12,$13)`,
			snap.DeviceID, pollID, c.Interface, c.MAC, c.IP, c.SSID, c.UptimeSec,
			floatPtrToAny(c.SignalDBm), floatPtrToAny(c.SignalToNoise),
			floatPtrToAny(c.TxRateMbps), floatPtrToAny(c.RxRateMbps), floatPtrToAny(c.CCQ),
			collected,
		); err != nil {
			return fmt.Errorf("mikrotik_wireless_clients: %w", err)
		}
	}

	// Also append a wireless_metrics row per registered client for the
	// scoring engine in Phase 6.
	for _, c := range snap.WirelessClients {
		if _, err := r.P.Exec(ctx, `INSERT INTO wireless_metrics
			(device_id, interface, collected_at, rssi_dbm, snr_db, tx_rate_mbps, rx_rate_mbps, ccq)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			snap.DeviceID, c.Interface, collected,
			floatPtrToAny(c.SignalDBm), floatPtrToAny(c.SignalToNoise),
			floatPtrToAny(c.TxRateMbps), floatPtrToAny(c.RxRateMbps),
			floatPtrToAny(c.CCQ),
		); err != nil {
			return fmt.Errorf("wireless_metrics: %w", err)
		}
	}

	return nil
}

// LatestSummary returns the most recent device_poll_results row.
func (r *Repository) LatestSummary(ctx context.Context, deviceID string) (*PollResultRow, error) {
	row := r.P.QueryRow(ctx, `
SELECT id, device_id, vendor, operation, transport, status, started_at, finished_at,
       duration_ms, COALESCE(error_code,''), COALESCE(error_message,''), summary, created_at
  FROM device_poll_results
 WHERE device_id = $1
 ORDER BY started_at DESC
 LIMIT 1`, deviceID)
	out, err := scanPollRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return out, err
}

// RecentResults lists the most recent poll results across all devices.
func (r *Repository) RecentResults(ctx context.Context, limit int) ([]PollResultRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := r.P.Query(ctx, `
SELECT id, device_id, vendor, operation, transport, status, started_at, finished_at,
       duration_ms, COALESCE(error_code,''), COALESCE(error_message,''), summary, created_at
  FROM device_poll_results
 ORDER BY started_at DESC
 LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]PollResultRow, 0)
	for rows.Next() {
		r1, err := scanPollRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r1)
	}
	return out, rows.Err()
}

// LatestWirelessClients returns the most recent registration-table snapshot
// rows for the given device.
func (r *Repository) LatestWirelessClients(ctx context.Context, deviceID string) ([]map[string]any, error) {
	rows, err := r.P.Query(ctx, `
SELECT interface_name, COALESCE(mac::text,''), COALESCE(host(ip),''), COALESCE(ssid,''),
       uptime_sec, signal_dbm, snr_db, tx_rate_mbps, rx_rate_mbps, ccq, collected_at
  FROM mikrotik_wireless_clients
 WHERE device_id = $1
   AND collected_at = (SELECT MAX(collected_at) FROM mikrotik_wireless_clients WHERE device_id = $1)
 ORDER BY interface_name, mac`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var iface, mac, ip, ssid string
		var uptime *int64
		var sig, snr, tx, rx, ccq *float64
		var at time.Time
		if err := rows.Scan(&iface, &mac, &ip, &ssid, &uptime, &sig, &snr, &tx, &rx, &ccq, &at); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"interface":    iface,
			"mac":          mac,
			"ip":           ip,
			"ssid":         ssid,
			"uptime_sec":   uptime,
			"signal_dbm":   sig,
			"snr_db":       snr,
			"tx_rate_mbps": tx,
			"rx_rate_mbps": rx,
			"ccq":          ccq,
			"collected_at": at,
		})
	}
	return out, rows.Err()
}

// LatestInterfaces returns the most recent interface_metrics rows.
func (r *Repository) LatestInterfaces(ctx context.Context, deviceID string) ([]map[string]any, error) {
	rows, err := r.P.Query(ctx, `
SELECT name, COALESCE(type,''), running, disabled, mtu, COALESCE(mac::text,''),
       rx_bytes, tx_bytes, rx_packets, tx_packets, rx_errors, tx_errors, link_downs, collected_at
  FROM interface_metrics
 WHERE device_id = $1
   AND collected_at = (SELECT MAX(collected_at) FROM interface_metrics WHERE device_id = $1)
 ORDER BY name`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var name, typ, mac string
		var running, disabled bool
		var mtu *int
		var rx, tx, rp, tp, re, te, ld *int64
		var at time.Time
		if err := rows.Scan(&name, &typ, &running, &disabled, &mtu, &mac, &rx, &tx, &rp, &tp, &re, &te, &ld, &at); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"name":         name,
			"type":         typ,
			"running":      running,
			"disabled":     disabled,
			"mtu":          mtu,
			"mac":          mac,
			"rx_bytes":     rx,
			"tx_bytes":     tx,
			"rx_packets":   rp,
			"tx_packets":   tp,
			"rx_errors":    re,
			"tx_errors":    te,
			"link_downs":   ld,
			"collected_at": at,
		})
	}
	return out, rows.Err()
}

// CapabilitiesUpsert persists the post-probe capability flags. Write
// capabilities (canApply/canBackup/canRollback) are NEVER set here; they
// are managed only by manual safety review (see SAFETY_MODEL.md).
func (r *Repository) CapabilitiesUpsert(ctx context.Context, deviceID string, caps mikrotik.CapabilityFlags) error {
	_, err := r.P.Exec(ctx, `
INSERT INTO device_capabilities (
    device_id, can_read_health, can_read_wireless_metrics, can_read_clients,
    can_read_frequency, can_recommend_frequency, supports_snmp,
    supports_routeros_api, supports_ssh, last_verified_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9, now())
ON CONFLICT (device_id) DO UPDATE SET
    can_read_health           = EXCLUDED.can_read_health OR device_capabilities.can_read_health,
    can_read_wireless_metrics = EXCLUDED.can_read_wireless_metrics OR device_capabilities.can_read_wireless_metrics,
    can_read_clients          = EXCLUDED.can_read_clients OR device_capabilities.can_read_clients,
    can_read_frequency        = EXCLUDED.can_read_frequency OR device_capabilities.can_read_frequency,
    can_recommend_frequency   = EXCLUDED.can_recommend_frequency OR device_capabilities.can_recommend_frequency,
    supports_snmp             = EXCLUDED.supports_snmp OR device_capabilities.supports_snmp,
    supports_routeros_api     = EXCLUDED.supports_routeros_api OR device_capabilities.supports_routeros_api,
    supports_ssh              = EXCLUDED.supports_ssh OR device_capabilities.supports_ssh,
    last_verified_at          = now()
`,
		deviceID,
		caps.CanReadHealth, caps.CanReadWirelessMetrics, caps.CanReadClients,
		caps.CanReadFrequency, caps.CanRecommendFrequency,
		caps.SupportsSNMP, caps.SupportsRouterOSAPI, caps.SupportsSSH,
	)
	return err
}

// helpers ---------------------------------------------------------

func scanPollRow(row interface{ Scan(...any) error }) (*PollResultRow, error) {
	var p PollResultRow
	var summary []byte
	if err := row.Scan(&p.ID, &p.DeviceID, &p.Vendor, &p.Operation, &p.Transport, &p.Status,
		&p.StartedAt, &p.FinishedAt, &p.DurationMS, &p.ErrorCode, &p.ErrorMessage, &summary, &p.CreatedAt); err != nil {
		return nil, err
	}
	if len(summary) > 0 {
		_ = json.Unmarshal(summary, &p.Summary)
	} else {
		p.Summary = map[string]any{}
	}
	return &p, nil
}

func nilOrInt64(s *mikrotik.MikroTikSystemInfo, get func(*mikrotik.MikroTikSystemInfo) int64) any {
	if s == nil {
		return nil
	}
	v := get(s)
	if v == 0 {
		return nil
	}
	return v
}

func nilOrFloat(s *mikrotik.MikroTikSystemInfo, get func(*mikrotik.MikroTikSystemInfo) *float64) any {
	if s == nil {
		return nil
	}
	v := get(s)
	if v == nil {
		return nil
	}
	return *v
}

func memPercent(s *mikrotik.MikroTikSystemInfo) any {
	if s == nil || s.FreeMemoryB == nil || s.TotalMemoryB == nil || *s.TotalMemoryB <= 0 {
		return nil
	}
	used := float64(*s.TotalMemoryB-*s.FreeMemoryB) * 100 / float64(*s.TotalMemoryB)
	return used
}

func intPtrToAny(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}

func floatPtrToAny(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}
