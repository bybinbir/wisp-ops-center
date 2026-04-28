package networkactions

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrIdempotencyConflict is returned by CreateDestructiveRun when an
// existing row already binds the same (action_type, idempotency_key)
// pair. The handler turns this into an idempotency_reused audit
// event and returns the original run.
var ErrIdempotencyConflict = errors.New("networkactions: idempotency_conflict")

// DestructiveCreateRunInput is the constructor payload for a
// destructive-Kind run. Phase 10C requires every destructive request
// to carry an idempotency key + intent + rollback note. The handler
// validates non-empty values BEFORE touching this method.
type DestructiveCreateRunInput struct {
	ActionType     Kind
	TargetDeviceID string
	TargetHost     string
	TargetLabel    string
	Actor          string
	CorrelationID  string
	DryRun         bool
	IdempotencyKey string
	Intent         string
	RollbackNote   string
}

// CreateDestructiveRun inserts a network_action_runs row for a
// destructive request. The schema's `uniq_nar_action_idem` partial
// unique index makes a duplicate POST visible as a 23505. We surface
// it as ErrIdempotencyConflict so the handler can fetch the original
// row instead of producing a new one.
func (r *Repository) CreateDestructiveRun(ctx context.Context, in DestructiveCreateRunInput) (*ActionRun, error) {
	if strings.TrimSpace(in.IdempotencyKey) == "" {
		return nil, errors.New("networkactions: idempotency_key required for destructive run")
	}
	if strings.TrimSpace(in.Intent) == "" {
		return nil, errors.New("networkactions: intent required for destructive run")
	}
	if strings.TrimSpace(in.RollbackNote) == "" {
		return nil, errors.New("networkactions: rollback_note required for destructive run")
	}
	row := r.P.QueryRow(ctx, `
INSERT INTO network_action_runs
  (action_type, target_device_id, target_host, target_label,
   actor, correlation_id, dry_run, status,
   idempotency_key, intent, rollback_note)
VALUES ($1, $2, $3::inet, $4, $5, $6, $7, 'queued',
        $8, $9, $10)
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
		in.IdempotencyKey,
		in.Intent,
		in.RollbackNote,
	)
	v, err := scanActionRun(row)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrIdempotencyConflict
		}
		return nil, err
	}
	return v, nil
}

// FindByIdempotencyKey returns the existing run for the given
// (action_type, idempotency_key) pair or ErrNotFound.
func (r *Repository) FindByIdempotencyKey(ctx context.Context, kind Kind, key string) (*ActionRun, error) {
	if strings.TrimSpace(key) == "" {
		return nil, ErrNotFound
	}
	row := r.P.QueryRow(ctx, `
SELECT id, action_type, COALESCE(target_device_id::text,''), COALESCE(host(target_host),''),
       target_label, status, started_at, finished_at, duration_ms,
       actor, correlation_id, dry_run, result,
       command_count, warning_count, confidence,
       COALESCE(error_code,''), COALESCE(error_message,''),
       created_at, updated_at
FROM network_action_runs
WHERE action_type = $1 AND idempotency_key = $2
ORDER BY created_at DESC
LIMIT 1`, string(kind), key)
	v, err := scanActionRun(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return v, err
}

// LifecycleEvent is one audit row tagged with this run's
// correlation_id. Returned by GetLifecycle for the
// /:run_id/lifecycle endpoint.
type LifecycleEvent struct {
	OccurredAt time.Time      `json:"occurred_at"`
	Action     string         `json:"action"`
	Outcome    string         `json:"outcome"`
	Actor      string         `json:"actor"`
	Subject    string         `json:"subject,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// GetLifecycle returns the run row + its audit_logs filtered by
// correlation_id. The audit_logs table is shared with every Phase 10
// audit producer; the correlation_id ties them together.
func (r *Repository) GetLifecycle(ctx context.Context, runID string) (*ActionRun, []LifecycleEvent, error) {
	run, err := r.GetRun(ctx, runID)
	if err != nil {
		return nil, nil, err
	}
	rows, err := r.P.Query(ctx, `
SELECT at, action, outcome, actor, COALESCE(subject,''), COALESCE(metadata, '{}'::jsonb)
FROM audit_logs
WHERE metadata->>'correlation_id' = $1
   OR metadata->>'run_id' = $2
ORDER BY at ASC`, run.CorrelationID, run.ID)
	if err != nil {
		return run, nil, err
	}
	defer rows.Close()
	out := make([]LifecycleEvent, 0)
	for rows.Next() {
		var ev LifecycleEvent
		var metaRaw []byte
		if err := rows.Scan(&ev.OccurredAt, &ev.Action, &ev.Outcome, &ev.Actor, &ev.Subject, &metaRaw); err != nil {
			return run, out, err
		}
		if len(metaRaw) > 0 {
			_ = json.Unmarshal(metaRaw, &ev.Metadata)
		}
		out = append(out, ev)
	}
	return run, out, rows.Err()
}

// isUniqueViolation reports whether err looks like a Postgres
// duplicate-key 23505 violation. We compare on the SQLSTATE prefix
// to avoid pulling in pgconn just for this check.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "23505") ||
		strings.Contains(strings.ToLower(msg), "duplicate key value")
}
