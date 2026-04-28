package networkinv

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wisp-ops-center/wisp-ops-center/internal/dude"
)

// ErrNotFound is returned when a device or run lookup misses.
var ErrNotFound = errors.New("networkinv: not_found")

// Repository wraps the pgx pool with concrete CRUD methods.
type Repository struct {
	P *pgxpool.Pool
}

func NewRepository(p *pgxpool.Pool) *Repository { return &Repository{P: p} }

// ----------------------------------------------------------------------------
// discovery_runs
// ----------------------------------------------------------------------------

// CreateRun inserts a new discovery_runs row in 'running' state.
func (r *Repository) CreateRun(ctx context.Context, correlationID, triggeredBy string) (*Run, error) {
	row := r.P.QueryRow(ctx, `
INSERT INTO discovery_runs (correlation_id, triggered_by)
VALUES ($1, $2)
RETURNING id, source, correlation_id, started_at, finished_at, status,
          device_count, ap_count, cpe_count, bridge_count, link_count,
          router_count, switch_count, unknown_count, low_conf_count,
          COALESCE(error_code,''), COALESCE(error_message,''),
          commands_run, triggered_by, created_at`,
		correlationID, triggeredBy)
	return scanRun(row)
}

// ComputeRunStatus is the single source of truth for the run status
// invariant (Phase 8 hotfix v8.4.0):
//
//   - success=true AND errorCode=="" -> "succeeded"
//   - errorCode=="panic_recovered"   -> "failed" (never partial)
//   - some devices observed          -> "partial"
//   - nothing observed               -> "failed"
//
// A non-empty errorCode MUST NEVER produce "succeeded".
func ComputeRunStatus(success bool, errorCode string, deviceCount int) string {
	if success && errorCode == "" {
		return "succeeded"
	}
	if errorCode == "panic_recovered" {
		return "failed"
	}
	if deviceCount > 0 {
		return "partial"
	}
	return "failed"
}

// FinalizeRun updates the run with terminal state + counters using
// the ComputeRunStatus invariant.
func (r *Repository) FinalizeRun(ctx context.Context, runID string, res dude.RunResult) error {
	status := ComputeRunStatus(res.Success, res.ErrorCode, len(res.Devices))
	cmds := res.CommandsRun
	if cmds == nil {
		cmds = []string{}
	}
	_, err := r.P.Exec(ctx, `
UPDATE discovery_runs
SET finished_at = now(),
    status = $2,
    device_count = $3, ap_count = $4, cpe_count = $5, bridge_count = $6,
    link_count = $7, router_count = $8, switch_count = $9, unknown_count = $10,
    low_conf_count = $11,
    error_code = NULLIF($12,''),
    error_message = NULLIF($13,''),
    commands_run = $14
WHERE id = $1`,
		runID, status,
		res.Stats.Total, res.Stats.APs, res.Stats.CPEs, res.Stats.Bridges,
		res.Stats.BackhaulLinks, res.Stats.Routers, res.Stats.Switches,
		res.Stats.Unknown, res.Stats.LowConfidence,
		res.ErrorCode, res.Error, cmds)
	return err
}

// ListRuns returns the most recent runs (newest first).
func (r *Repository) ListRuns(ctx context.Context, limit int) ([]Run, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.P.Query(ctx, `
SELECT id, source, correlation_id, started_at, finished_at, status,
       device_count, ap_count, cpe_count, bridge_count, link_count,
       router_count, switch_count, unknown_count, low_conf_count,
       COALESCE(error_code,''), COALESCE(error_message,''),
       commands_run, triggered_by, created_at
FROM discovery_runs
ORDER BY started_at DESC
LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Run, 0)
	for rows.Next() {
		v, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

// LatestRun returns the most recently started run (any status).
func (r *Repository) LatestRun(ctx context.Context) (*Run, error) {
	row := r.P.QueryRow(ctx, `
SELECT id, source, correlation_id, started_at, finished_at, status,
       device_count, ap_count, cpe_count, bridge_count, link_count,
       router_count, switch_count, unknown_count, low_conf_count,
       COALESCE(error_code,''), COALESCE(error_message,''),
       commands_run, triggered_by, created_at
FROM discovery_runs
ORDER BY started_at DESC
LIMIT 1`)
	v, err := scanRun(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return v, err
}

// ----------------------------------------------------------------------------
// network_devices upsert
// ----------------------------------------------------------------------------

// UpsertStats reports per-run insert/update/skip counters from
// UpsertDevices. Useful for run finalize + audit metadata.
type UpsertStats struct {
	Inserted int
	Updated  int
	Skipped  int
}

// UpsertDevices applies a discovery result to network_devices using
// per-device dispatch on the strongest stable identity:
//
//  1. (source, mac)              when MAC present
//  2. (source, host, name)       when MAC empty and host+name present
//  3. (source, name) name-only   when MAC and host empty
//  4. otherwise                  skipped (no stable identity)
//
// Each branch uses an ON CONFLICT clause whose target+predicate match
// the corresponding partial unique index in migration 000008 so
// re-running the same dataset is idempotent and never raises 23505.
//
// Returns:
//   - ids[i]: persisted device id; "" when device i was skipped
//   - stats:  insert/update/skip counters
func (r *Repository) UpsertDevices(ctx context.Context, runID string, devs []dude.DiscoveredDevice) ([]string, UpsertStats, error) {
	ids := make([]string, len(devs))
	var stats UpsertStats
	if len(devs) == 0 {
		return ids, stats, nil
	}

	tx, err := r.P.Begin(ctx)
	if err != nil {
		return nil, stats, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for i, d := range devs {
		host := strings.TrimSpace(d.IP)
		name := strings.TrimSpace(d.Name)
		mac := strings.ToUpper(strings.TrimSpace(d.MAC))

		if mac == "" && host == "" && name == "" {
			stats.Skipped++
			continue
		}
		// host without name is not an identifying combination for the
		// (source,host,name) index; only name-only branch covers that
		// case via name. If even name is empty, we skip too.
		if mac == "" && name == "" {
			stats.Skipped++
			continue
		}

		id, inserted, err := upsertOneDevice(ctx, tx, d, host, name, mac)
		if err != nil {
			return nil, stats, fmt.Errorf("upsert device %d: %w", i, err)
		}
		ids[i] = id
		if inserted {
			stats.Inserted++
		} else {
			stats.Updated++
		}

		if runID != "" {
			if err := refreshEvidence(ctx, tx, id, runID, d); err != nil {
				return nil, stats, fmt.Errorf("evidence device %d: %w", i, err)
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, stats, err
	}
	return ids, stats, nil
}

func upsertOneDevice(ctx context.Context, tx pgx.Tx, d dude.DiscoveredDevice, host, name, mac string) (string, bool, error) {
	raw, _ := json.Marshal(d.Raw)
	category := string(d.Classification.Category)
	if category == "" {
		category = string(dude.CategoryUnknown)
	}
	status := strings.ToLower(strings.TrimSpace(d.Status))
	switch status {
	case "up", "down", "partial", "unknown":
	default:
		status = "unknown"
	}

	var sqlText string
	switch {
	case mac != "":
		sqlText = sqlUpsertDeviceByMAC
	case host != "" && name != "":
		sqlText = sqlUpsertDeviceByHostName
	case name != "":
		sqlText = sqlUpsertDeviceByName
	default:
		// Caller already filters this case; defensive only.
		return "", false, errors.New("no stable identity")
	}

	var id string
	var inserted bool
	err := tx.QueryRow(ctx, sqlText,
		nullIfEmpty(host),           // $1 host
		name,                        // $2 name
		nullIfEmpty(mac),            // $3 mac
		nullIfEmpty(d.Model),        // $4 model
		nullIfEmpty(d.OSVersion),    // $5 os_version
		nullIfEmpty(d.Identity),     // $6 identity
		nullIfEmpty(d.Type),         // $7 device_type
		category,                    // $8 category
		d.Classification.Confidence, // $9 confidence
		status,                      // $10 status
		d.LastSeen,                  // $11 last_seen_at
		string(raw),                 // $12 raw_metadata
	).Scan(&id, &inserted)
	return id, inserted, err
}

// All three INSERTs share the same column list and parameter order;
// they differ only in the ON CONFLICT target+predicate to match the
// three partial unique indexes from migration 000008.

const sqlUpsertDeviceCommonValues = `
INSERT INTO network_devices
(source, host, name, mac, model, os_version, identity, device_type,
 category, confidence, status, last_seen_at, raw_metadata)
VALUES ('mikrotik_dude', $1::inet, $2, $3, $4, $5, $6, $7,
        $8, $9, $10, $11, $12::jsonb)
`

const sqlUpsertDeviceCommonUpdate = `
DO UPDATE SET
  host = COALESCE(EXCLUDED.host, network_devices.host),
  name = CASE WHEN EXCLUDED.name <> '' THEN EXCLUDED.name ELSE network_devices.name END,
  mac = COALESCE(EXCLUDED.mac, network_devices.mac),
  model = COALESCE(EXCLUDED.model, network_devices.model),
  os_version = COALESCE(EXCLUDED.os_version, network_devices.os_version),
  identity = COALESCE(EXCLUDED.identity, network_devices.identity),
  device_type = COALESCE(EXCLUDED.device_type, network_devices.device_type),
  category = EXCLUDED.category,
  confidence = EXCLUDED.confidence,
  status = EXCLUDED.status,
  raw_metadata = EXCLUDED.raw_metadata,
  last_seen_at = EXCLUDED.last_seen_at,
  updated_at = now()
RETURNING id, (xmax = 0) AS inserted
`

var (
	sqlUpsertDeviceByMAC      = sqlUpsertDeviceCommonValues + `ON CONFLICT (source, mac) WHERE mac IS NOT NULL ` + sqlUpsertDeviceCommonUpdate
	sqlUpsertDeviceByHostName = sqlUpsertDeviceCommonValues + `ON CONFLICT (source, host, name) WHERE host IS NOT NULL ` + sqlUpsertDeviceCommonUpdate
	sqlUpsertDeviceByName     = sqlUpsertDeviceCommonValues + `ON CONFLICT (source, name) WHERE host IS NULL AND mac IS NULL AND name <> '' ` + sqlUpsertDeviceCommonUpdate
)

func refreshEvidence(ctx context.Context, tx pgx.Tx, deviceID, runID string, d dude.DiscoveredDevice) error {
	if _, err := tx.Exec(ctx, `
DELETE FROM device_category_evidence WHERE device_id = $1 AND run_id = $2`,
		deviceID, runID); err != nil {
		return err
	}
	category := string(d.Classification.Category)
	if category == "" {
		category = string(dude.CategoryUnknown)
	}
	for _, ev := range d.Classification.Evidences {
		if _, err := tx.Exec(ctx, `
INSERT INTO device_category_evidence
(device_id, run_id, heuristic, category, weight, reason)
VALUES ($1, $2, $3, $4, $5, $6)`,
			deviceID, runID, ev.Heuristic, category, ev.Weight, ev.Reason); err != nil {
			return err
		}
	}
	return nil
}

// ----------------------------------------------------------------------------
// network_devices read
// ----------------------------------------------------------------------------

// ListDevices returns a filtered slice.
func (r *Repository) ListDevices(ctx context.Context, f Filter) ([]Device, error) {
	limit := f.Limit
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	args := []any{limit, f.Offset}
	conds := []string{"1=1"}

	if f.Category != "" {
		args = append(args, f.Category)
		conds = append(conds, fmt.Sprintf("category = $%d", len(args)))
	}
	if f.Status != "" {
		args = append(args, f.Status)
		conds = append(conds, fmt.Sprintf("status = $%d", len(args)))
	}
	if f.OnlyLowConf {
		conds = append(conds, "confidence < 50")
	}
	if f.OnlyUnknown {
		conds = append(conds, "category = 'Unknown'")
	}

	q := fmt.Sprintf(`
SELECT id, source, COALESCE(host(host),''), name, COALESCE(mac,''),
       COALESCE(model,''), COALESCE(os_version,''), COALESCE(identity,''),
       COALESCE(device_type,''), category, confidence, status,
       last_seen_at, first_seen_at, raw_metadata, created_at, updated_at
FROM network_devices
WHERE %s
ORDER BY last_seen_at DESC
LIMIT $1 OFFSET $2`, strings.Join(conds, " AND "))

	rows, err := r.P.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Device, 0)
	for rows.Next() {
		var d Device
		var raw []byte
		var category string
		if err := rows.Scan(&d.ID, &d.Source, &d.Host, &d.Name, &d.MAC,
			&d.Model, &d.OSVersion, &d.Identity, &d.DeviceType,
			&category, &d.Confidence, &d.Status,
			&d.LastSeenAt, &d.FirstSeenAt, &raw, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		d.Category = dude.Category(category)
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &d.RawMetadata)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// GetDevice returns one device by id.
func (r *Repository) GetDevice(ctx context.Context, id string) (*Device, error) {
	row := r.P.QueryRow(ctx, `
SELECT id, source, COALESCE(host(host),''), name, COALESCE(mac,''),
       COALESCE(model,''), COALESCE(os_version,''), COALESCE(identity,''),
       COALESCE(device_type,''), category, confidence, status,
       last_seen_at, first_seen_at, raw_metadata, created_at, updated_at
FROM network_devices WHERE id = $1`, id)
	var d Device
	var raw []byte
	var category string
	if err := row.Scan(&d.ID, &d.Source, &d.Host, &d.Name, &d.MAC,
		&d.Model, &d.OSVersion, &d.Identity, &d.DeviceType,
		&category, &d.Confidence, &d.Status,
		&d.LastSeenAt, &d.FirstSeenAt, &raw, &d.CreatedAt, &d.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	d.Category = dude.Category(category)
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &d.RawMetadata)
	}
	return &d, nil
}

// ----------------------------------------------------------------------------
// helpers
// ----------------------------------------------------------------------------

func scanRun(row pgx.Row) (*Run, error) {
	var v Run
	var finished *time.Time
	var commands []string
	if err := row.Scan(&v.ID, &v.Source, &v.CorrelationID, &v.StartedAt,
		&finished, &v.Status, &v.DeviceCount, &v.APCount, &v.CPECount,
		&v.BridgeCount, &v.LinkCount, &v.RouterCount, &v.SwitchCount,
		&v.UnknownCount, &v.LowConfCount,
		&v.ErrorCode, &v.ErrorMessage, &commands, &v.TriggeredBy, &v.CreatedAt); err != nil {
		return nil, err
	}
	v.FinishedAt = finished
	v.CommandsRun = commands
	if v.CommandsRun == nil {
		v.CommandsRun = []string{}
	}
	return &v, nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
