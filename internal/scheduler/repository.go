package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound surfaces from the repository when a row is missing.
var ErrNotFound = errors.New("scheduler: not found")

// Repository wraps the pgx pool. Phase 5 only persists scheduled_checks +
// job_runs + maintenance_windows; locking is in-memory until Faz 5
// Redis adoption.
type Repository struct {
	P *pgxpool.Pool
}

// NewRepository wires up the repo.
func NewRepository(p *pgxpool.Pool) *Repository { return &Repository{P: p} }

// ScheduledCheckRow is the API-safe view of scheduled_checks.
type ScheduledCheckRow struct {
	ID                  string     `json:"id"`
	Name                string     `json:"name"`
	JobType             string     `json:"job_type"`
	ScheduleType        string     `json:"schedule_type"`
	CronExpression      string     `json:"cron_expression,omitempty"`
	Timezone            string     `json:"timezone"`
	IntervalSec         *int       `json:"interval_sec,omitempty"`
	ScopeType           string     `json:"scope_type"`
	ScopeID             string     `json:"scope_id,omitempty"`
	Mode                string     `json:"action_mode"`
	RiskLevel           string     `json:"risk_level"`
	MaintenanceWindowID *string    `json:"maintenance_window_id,omitempty"`
	MaxDurationSec      int        `json:"max_duration_seconds"`
	MaxParallel         int        `json:"max_parallel"`
	Enabled             bool       `json:"enabled"`
	NextRunAt           *time.Time `json:"next_run_at,omitempty"`
	LastRunAt           *time.Time `json:"last_run_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

const checkCols = `id, name, job_type, schedule_type, COALESCE(cron_expression,''),
COALESCE(timezone,'UTC'), interval_sec, COALESCE(scope_type,'all_network'),
COALESCE(scope_id,''), COALESCE(mode,'report_only'),
COALESCE(risk_level,'low'), maintenance_window_id::text,
COALESCE(max_duration_seconds,60), COALESCE(max_parallel,4),
enabled, next_run_at, last_run_at, created_at, updated_at`

func scanCheck(row pgx.Row) (*ScheduledCheckRow, error) {
	var r ScheduledCheckRow
	var mwid *string
	var interval *int
	if err := row.Scan(
		&r.ID, &r.Name, &r.JobType, &r.ScheduleType, &r.CronExpression,
		&r.Timezone, &interval, &r.ScopeType, &r.ScopeID, &r.Mode,
		&r.RiskLevel, &mwid, &r.MaxDurationSec, &r.MaxParallel,
		&r.Enabled, &r.NextRunAt, &r.LastRunAt, &r.CreatedAt, &r.UpdatedAt,
	); err != nil {
		return nil, err
	}
	r.MaintenanceWindowID = mwid
	r.IntervalSec = interval
	return &r, nil
}

// ListChecks returns every scheduled check, ordered by name.
func (r *Repository) ListChecks(ctx context.Context) ([]ScheduledCheckRow, error) {
	rows, err := r.P.Query(ctx, `SELECT `+checkCols+` FROM scheduled_checks ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ScheduledCheckRow, 0)
	for rows.Next() {
		c, err := scanCheck(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// GetCheck returns one row by id.
func (r *Repository) GetCheck(ctx context.Context, id string) (*ScheduledCheckRow, error) {
	row := r.P.QueryRow(ctx, `SELECT `+checkCols+` FROM scheduled_checks WHERE id = $1`, id)
	c, err := scanCheck(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return c, err
}

// CreateCheck persists a validated input.
func (r *Repository) CreateCheck(ctx context.Context, in ScheduledCheckInput) (*ScheduledCheckRow, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	next, _ := in.NextRun(time.Now())
	var mwid any
	if in.MaintenanceWinID != "" {
		mwid = in.MaintenanceWinID
	}
	row := r.P.QueryRow(ctx, `
INSERT INTO scheduled_checks
  (name, job_type, schedule_type, cron_expression, timezone, interval_sec,
   scope_type, scope_id, mode, risk_level, maintenance_window_id,
   max_duration_seconds, max_parallel, enabled, next_run_at, cadence)
VALUES ($1,$2,$3,NULLIF($4,''),$5,$6,$7,NULLIF($8,''),$9,$10,$11::uuid,$12,$13,$14,$15, COALESCE($3,'manual'))
RETURNING `+checkCols,
		strings.TrimSpace(in.Name), string(in.JobType), string(in.ScheduleType),
		in.CronExpression, in.Timezone, nilIfZero(in.IntervalSec),
		string(in.ScopeType), in.ScopeID, string(in.ActionMode), string(in.RiskLevel),
		mwid, in.MaxDurationSec, in.MaxParallel, in.Enabled, nilIfZeroTime(next),
	)
	return scanCheck(row)
}

// UpdateCheck patches an existing row.
func (r *Repository) UpdateCheck(ctx context.Context, id string, in ScheduledCheckInput) (*ScheduledCheckRow, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	next, _ := in.NextRun(time.Now())
	var mwid any
	if in.MaintenanceWinID != "" {
		mwid = in.MaintenanceWinID
	}
	row := r.P.QueryRow(ctx, `
UPDATE scheduled_checks SET
  name=$2, job_type=$3, schedule_type=$4, cron_expression=NULLIF($5,''),
  timezone=$6, interval_sec=$7, scope_type=$8, scope_id=NULLIF($9,''),
  mode=$10, risk_level=$11, maintenance_window_id=$12::uuid,
  max_duration_seconds=$13, max_parallel=$14, enabled=$15,
  next_run_at=$16, updated_at=now()
 WHERE id=$1
 RETURNING `+checkCols,
		id, in.Name, string(in.JobType), string(in.ScheduleType),
		in.CronExpression, in.Timezone, nilIfZero(in.IntervalSec),
		string(in.ScopeType), in.ScopeID, string(in.ActionMode), string(in.RiskLevel),
		mwid, in.MaxDurationSec, in.MaxParallel, in.Enabled, nilIfZeroTime(next),
	)
	c, err := scanCheck(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return c, err
}

// DeleteCheck removes a row.
func (r *Repository) DeleteCheck(ctx context.Context, id string) error {
	cmd, err := r.P.Exec(ctx, `DELETE FROM scheduled_checks WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// JobRunRow is the API-safe view of job_runs.
type JobRunRow struct {
	ID         string         `json:"id"`
	CheckID    *string        `json:"scheduled_check_id,omitempty"`
	JobType    string         `json:"job_type"`
	ScopeType  string         `json:"scope_type,omitempty"`
	ScopeID    string         `json:"scope_id,omitempty"`
	Status     string         `json:"status"`
	StartedAt  time.Time      `json:"started_at"`
	FinishedAt *time.Time     `json:"finished_at,omitempty"`
	DurationMS *int           `json:"duration_ms,omitempty"`
	ErrorCode  string         `json:"error_code,omitempty"`
	ErrorText  string         `json:"error_message,omitempty"`
	Summary    map[string]any `json:"summary"`
	CreatedAt  *time.Time     `json:"created_at,omitempty"`
}

// RecordJobRun inserts a row and returns id.
func (r *Repository) RecordJobRun(ctx context.Context, in JobRunRow) (string, error) {
	summary, _ := json.Marshal(in.Summary)
	var id string
	err := r.P.QueryRow(ctx, `
INSERT INTO job_runs
  (check_id, job_type, scope_type, scope_id, status, started_at, finished_at,
   duration_ms, error_code, error_text, summary)
VALUES (NULLIF($1,'')::uuid, $2, NULLIF($3,''), NULLIF($4,''), $5, $6, $7, $8,
        NULLIF($9,''), NULLIF($10,''), $11)
RETURNING id::text`,
		strDeref(in.CheckID), in.JobType, in.ScopeType, in.ScopeID,
		in.Status, in.StartedAt, in.FinishedAt, in.DurationMS,
		in.ErrorCode, in.ErrorText, summary,
	).Scan(&id)
	return id, err
}

// ListJobRuns returns the recent N rows.
func (r *Repository) ListJobRuns(ctx context.Context, limit int) ([]JobRunRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := r.P.Query(ctx, `
SELECT id::text, check_id::text, job_type, COALESCE(scope_type,''), COALESCE(scope_id,''),
       status, started_at, finished_at, duration_ms,
       COALESCE(error_code,''), COALESCE(error_text,''), summary
  FROM job_runs ORDER BY started_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]JobRunRow, 0)
	for rows.Next() {
		var r1 JobRunRow
		var checkID *string
		var summary []byte
		var dur *int
		if err := rows.Scan(&r1.ID, &checkID, &r1.JobType, &r1.ScopeType, &r1.ScopeID,
			&r1.Status, &r1.StartedAt, &r1.FinishedAt, &dur,
			&r1.ErrorCode, &r1.ErrorText, &summary); err != nil {
			return nil, err
		}
		r1.CheckID = checkID
		r1.DurationMS = dur
		if len(summary) > 0 {
			_ = json.Unmarshal(summary, &r1.Summary)
		} else {
			r1.Summary = map[string]any{}
		}
		out = append(out, r1)
	}
	return out, rows.Err()
}

// MaintenanceRow is the API-safe maintenance_window view.
type MaintenanceRow struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	ScopeType  string    `json:"scope_type"`
	ScopeID    string    `json:"scope_id,omitempty"`
	StartsAt   time.Time `json:"starts_at"`
	EndsAt     time.Time `json:"ends_at"`
	Timezone   string    `json:"timezone"`
	Recurrence string    `json:"recurrence,omitempty"`
	Enabled    bool      `json:"enabled"`
	Notes      string    `json:"notes,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

const mwCols = `id::text, name, scope_type, COALESCE(scope_id,''), starts_at, ends_at,
COALESCE(timezone,'UTC'), COALESCE(recurrence,''), enabled, COALESCE(notes,''),
created_at, updated_at`

func scanMW(row pgx.Row) (*MaintenanceRow, error) {
	var m MaintenanceRow
	if err := row.Scan(&m.ID, &m.Name, &m.ScopeType, &m.ScopeID, &m.StartsAt,
		&m.EndsAt, &m.Timezone, &m.Recurrence, &m.Enabled, &m.Notes,
		&m.CreatedAt, &m.UpdatedAt); err != nil {
		return nil, err
	}
	return &m, nil
}

// ListWindows returns every maintenance window.
func (r *Repository) ListWindows(ctx context.Context) ([]MaintenanceRow, error) {
	rows, err := r.P.Query(ctx, `SELECT `+mwCols+` FROM maintenance_windows ORDER BY starts_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]MaintenanceRow, 0)
	for rows.Next() {
		m, err := scanMW(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *m)
	}
	return out, rows.Err()
}

// CreateWindow inserts a row.
func (r *Repository) CreateWindow(ctx context.Context, m MaintenanceRow) (*MaintenanceRow, error) {
	if m.Timezone == "" {
		m.Timezone = "UTC"
	}
	row := r.P.QueryRow(ctx, `
INSERT INTO maintenance_windows
  (name, scope_type, scope_id, starts_at, ends_at, timezone, recurrence, enabled, notes)
VALUES ($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,$9)
RETURNING `+mwCols,
		m.Name, m.ScopeType, m.ScopeID, m.StartsAt, m.EndsAt,
		m.Timezone, m.Recurrence, m.Enabled, m.Notes,
	)
	return scanMW(row)
}

// DeleteWindow removes a row by id.
func (r *Repository) DeleteWindow(ctx context.Context, id string) error {
	cmd, err := r.P.Exec(ctx, `DELETE FROM maintenance_windows WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SSHKnownHostsStore is the Postgres implementation of the host key
// store consumed by the SSH adapter.
type SSHKnownHostsStore struct{ P *pgxpool.Pool }

// Get returns the stored fingerprint for the host.
func (s *SSHKnownHostsStore) Get(host string) (string, bool, error) {
	var fp string
	err := s.P.QueryRow(context.Background(),
		`SELECT fingerprint FROM ssh_known_hosts WHERE host = $1`, host,
	).Scan(&fp)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return fp, true, nil
}

// Put stores or updates the fingerprint.
func (s *SSHKnownHostsStore) Put(host, fp string) error {
	_, err := s.P.Exec(context.Background(),
		`INSERT INTO ssh_known_hosts (host, fingerprint) VALUES ($1,$2)
		 ON CONFLICT (host) DO UPDATE SET fingerprint = EXCLUDED.fingerprint, seen_last = now()`,
		host, fp,
	)
	return err
}

// helpers

func nilIfZero(v int) any {
	if v == 0 {
		return nil
	}
	return v
}

func nilIfZeroTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

func strDeref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
