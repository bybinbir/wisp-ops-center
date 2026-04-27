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

// FinalizeRun updates the run with terminal state + counters.
func (r *Repository) FinalizeRun(ctx context.Context, runID string, res dude.RunResult) error {
	status := "succeeded"
	if !res.Success {
		if len(res.Devices) > 0 {
			status = "partial"
		} else {
			status = "failed"
		}
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
		res.ErrorCode, res.Error, res.CommandsRun)
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
// network_devices
// ----------------------------------------------------------------------------

// UpsertDevices applies a discovery result to network_devices using
// (source,mac), (source,host,name) and (source,name) match keys in
// that priority. Returns the per-device persisted IDs in the same
// order as the input slice.
//
// Evidence rows for run_id are also written if non-empty.
func (r *Repository) UpsertDevices(ctx context.Context, runID string, devs []dude.DiscoveredDevice) ([]string, error) {
	ids := make([]string, len(devs))
	if len(devs) == 0 {
		return ids, nil
	}

	tx, err := r.P.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for i, d := range devs {
		raw, _ := json.Marshal(d.Raw)
		host := strings.TrimSpace(d.IP)
		name := strings.TrimSpace(d.Name)
		mac := strings.ToUpper(strings.TrimSpace(d.MAC))
		category := string(d.Classification.Category)
		if category == "" {
			category = string(dude.CategoryUnknown)
		}
		status := strings.ToLower(strings.TrimSpace(d.Status))
		if status == "" {
			status = "unknown"
		}
		if status != "up" && status != "down" && status != "partial" && status != "unknown" {
			status = "unknown"
		}

		// Find existing row (mac > host+name > name)
		var existingID string
		findErr := tx.QueryRow(ctx, `
SELECT id FROM network_devices
WHERE source = 'mikrotik_dude'
  AND ( ($1::text <> '' AND mac = $1)
     OR ($2::text <> '' AND host = $2::inet AND name = $3)
     OR ($1::text = '' AND $2::text = '' AND name = $3)
      )
ORDER BY updated_at DESC
LIMIT 1`, mac, nullIfEmpty(host), name).Scan(&existingID)

		var id string
		if findErr == nil && existingID != "" {
			// Update.
			err := tx.QueryRow(ctx, `
UPDATE network_devices
SET host = COALESCE(NULLIF($1,'')::inet, host),
    name = CASE WHEN $2 <> '' THEN $2 ELSE name END,
    mac = CASE WHEN $3 <> '' THEN $3 ELSE mac END,
    model = CASE WHEN $4 <> '' THEN $4 ELSE model END,
    os_version = CASE WHEN $5 <> '' THEN $5 ELSE os_version END,
    identity = CASE WHEN $6 <> '' THEN $6 ELSE identity END,
    device_type = CASE WHEN $7 <> '' THEN $7 ELSE device_type END,
    category = $8,
    confidence = $9,
    status = $10,
    raw_metadata = $11::jsonb,
    last_seen_at = $12,
    updated_at = now()
WHERE id = $13
RETURNING id`,
				host, name, mac, d.Model, d.OSVersion, d.Identity, d.Type,
				category, d.Classification.Confidence, status, string(raw),
				d.LastSeen, existingID).Scan(&id)
			if err != nil {
				return nil, fmt.Errorf("update device %d: %w", i, err)
			}
		} else if errors.Is(findErr, pgx.ErrNoRows) || existingID == "" {
			// Insert.
			err := tx.QueryRow(ctx, `
INSERT INTO network_devices
(source, host, name, mac, model, os_version, identity, device_type,
 category, confidence, status, last_seen_at, raw_metadata)
VALUES ('mikrotik_dude', NULLIF($1,'')::inet, $2, NULLIF($3,''), NULLIF($4,''),
        NULLIF($5,''), NULLIF($6,''), NULLIF($7,''),
        $8, $9, $10, $11, $12::jsonb)
RETURNING id`,
				host, name, mac, d.Model, d.OSVersion, d.Identity, d.Type,
				category, d.Classification.Confidence, status, d.LastSeen, string(raw)).Scan(&id)
			if err != nil {
				return nil, fmt.Errorf("insert device %d: %w", i, err)
			}
		} else {
			return nil, fmt.Errorf("lookup device %d: %w", i, findErr)
		}

		ids[i] = id

		if runID != "" {
			// Refresh evidence for this device + run.
			_, err := tx.Exec(ctx, `
DELETE FROM device_category_evidence WHERE device_id = $1 AND run_id = $2`,
				id, runID)
			if err != nil {
				return nil, fmt.Errorf("clear evidence %d: %w", i, err)
			}
			for _, ev := range d.Classification.Evidences {
				_, err := tx.Exec(ctx, `
INSERT INTO device_category_evidence
(device_id, run_id, heuristic, category, weight, reason)
VALUES ($1, $2, $3, $4, $5, $6)`,
					id, runID, ev.Heuristic, category, ev.Weight, ev.Reason)
				if err != nil {
					return nil, fmt.Errorf("insert evidence %d: %w", i, err)
				}
			}
		}
	}

	return ids, tx.Commit(ctx)
}

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
