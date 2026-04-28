package networkactions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository is the persistence layer for network_action_runs.
type Repository struct {
	P *pgxpool.Pool
}

func NewRepository(p *pgxpool.Pool) *Repository { return &Repository{P: p} }

// ErrNotFound is returned when a run lookup misses.
var ErrNotFound = errors.New("networkactions: not_found")

// CreateRun inserts a new network_action_runs row in 'queued' state
// and returns the populated ActionRun. correlation_id MUST come from
// the caller so the API + audit + run all share one identifier.
func (r *Repository) CreateRun(ctx context.Context, in CreateRunInput) (*ActionRun, error) {
	row := r.P.QueryRow(ctx, `
INSERT INTO network_action_runs
  (action_type, target_device_id, target_host, target_label,
   actor, correlation_id, dry_run, status)
VALUES ($1, $2, $3::inet, $4, $5, $6, $7, 'queued')
RETURNING id, action_type, COALESCE(target_device_id::text,''), COALESCE(host(target_host),''),
          target_label, status, started_at, finished_at, duration_ms,
          actor, correlation_id, dry_run, result,
          command_count, warning_count, confidence,
          COALESCE(error_code,''), COALESCE(error_message,''),
          created_at, updated_at`,
		string(in.ActionType),
		nullIfEmpty(in.TargetDeviceID),
		nullIfEmpty(in.TargetHost),
		in.TargetLabel,
		emptyIfBlank(in.Actor, "system"),
		in.CorrelationID,
		in.DryRun,
	)
	return scanActionRun(row)
}

// CreateRunInput is the constructor payload.
type CreateRunInput struct {
	ActionType     Kind
	TargetDeviceID string
	TargetHost     string
	TargetLabel    string
	Actor          string
	CorrelationID  string
	DryRun         bool
}

// MarkRunning transitions a run from queued to running and stamps
// started_at = now().
func (r *Repository) MarkRunning(ctx context.Context, id string) error {
	_, err := r.P.Exec(ctx, `
UPDATE network_action_runs
SET status='running', started_at=now(), updated_at=now()
WHERE id=$1 AND status='queued'`, id)
	return err
}

// FinalizeRun applies the terminal status + result fields. Idempotent
// at the DB level (uses UPDATE ... RETURNING * shape via UPDATE).
func (r *Repository) FinalizeRun(ctx context.Context, id string, fin FinalizeInput) error {
	if !IsValidStatus(string(fin.Status)) {
		return fmt.Errorf("networkactions: invalid status %q", fin.Status)
	}
	resultJSON, err := json.Marshal(fin.Result)
	if err != nil {
		return err
	}
	dur := fin.DurationMS
	if dur < 0 {
		dur = 0
	}
	_, err = r.P.Exec(ctx, `
UPDATE network_action_runs
SET status        = $2,
    finished_at   = COALESCE(finished_at, now()),
    duration_ms   = $3,
    result        = $4::jsonb,
    command_count = $5,
    warning_count = $6,
    confidence    = $7,
    error_code    = NULLIF($8,''),
    error_message = NULLIF($9,''),
    updated_at    = now()
WHERE id=$1`,
		id, string(fin.Status), dur, string(resultJSON),
		fin.CommandCount, fin.WarningCount, fin.Confidence,
		fin.ErrorCode, fin.ErrorMessage)
	return err
}

// FinalizeInput is the FinalizeRun payload.
type FinalizeInput struct {
	Status       RunStatus
	DurationMS   int64
	Result       map[string]any
	CommandCount int
	WarningCount int
	Confidence   int
	ErrorCode    string
	ErrorMessage string
}

// GetRun returns one run by id.
func (r *Repository) GetRun(ctx context.Context, id string) (*ActionRun, error) {
	row := r.P.QueryRow(ctx, sqlSelectActionRunByID, id)
	v, err := scanActionRun(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return v, err
}

// ListRuns returns the most recent runs (newest first), filtered.
func (r *Repository) ListRuns(ctx context.Context, f ListFilter) ([]ActionRun, error) {
	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	args := []any{limit}
	conds := []string{"1=1"}
	if f.ActionType != "" {
		args = append(args, f.ActionType)
		conds = append(conds, fmt.Sprintf("action_type = $%d", len(args)))
	}
	if f.Status != "" {
		args = append(args, f.Status)
		conds = append(conds, fmt.Sprintf("status = $%d", len(args)))
	}
	if f.DeviceID != "" {
		args = append(args, f.DeviceID)
		conds = append(conds, fmt.Sprintf("target_device_id = $%d", len(args)))
	}
	q := fmt.Sprintf(`
SELECT id, action_type, COALESCE(target_device_id::text,''), COALESCE(host(target_host),''),
       target_label, status, started_at, finished_at, duration_ms,
       actor, correlation_id, dry_run, result,
       command_count, warning_count, confidence,
       COALESCE(error_code,''), COALESCE(error_message,''),
       created_at, updated_at
FROM network_action_runs
WHERE %s
ORDER BY created_at DESC
LIMIT $1`, strings.Join(conds, " AND "))
	rows, err := r.P.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ActionRun, 0)
	for rows.Next() {
		v, err := scanActionRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

// ListFilter narrows a ListRuns call.
type ListFilter struct {
	ActionType string
	Status     string
	DeviceID   string
	Limit      int
}

// ----------------------------------------------------------------------------
// helpers
// ----------------------------------------------------------------------------

const sqlSelectActionRunByID = `
SELECT id, action_type, COALESCE(target_device_id::text,''), COALESCE(host(target_host),''),
       target_label, status, started_at, finished_at, duration_ms,
       actor, correlation_id, dry_run, result,
       command_count, warning_count, confidence,
       COALESCE(error_code,''), COALESCE(error_message,''),
       created_at, updated_at
FROM network_action_runs
WHERE id=$1`

func scanActionRun(row pgx.Row) (*ActionRun, error) {
	var v ActionRun
	var actionType, status string
	var startedAt, finishedAt *time.Time
	var resultRaw []byte
	if err := row.Scan(&v.ID, &actionType, &v.TargetDeviceID, &v.TargetHost,
		&v.TargetLabel, &status, &startedAt, &finishedAt, &v.DurationMS,
		&v.Actor, &v.CorrelationID, &v.DryRun, &resultRaw,
		&v.CommandCount, &v.WarningCount, &v.Confidence,
		&v.ErrorCode, &v.ErrorMessage,
		&v.CreatedAt, &v.UpdatedAt); err != nil {
		return nil, err
	}
	v.ActionType = Kind(actionType)
	v.Status = RunStatus(status)
	v.StartedAt = startedAt
	v.FinishedAt = finishedAt
	if len(resultRaw) > 0 {
		_ = json.Unmarshal(resultRaw, &v.Result)
	}
	if v.Result == nil {
		v.Result = map[string]any{}
	}
	return &v, nil
}

func nullIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func emptyIfBlank(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}
