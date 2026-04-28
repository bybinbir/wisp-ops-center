package networkactions

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgMaintenanceStore is the Postgres-backed MaintenanceStore. Phase
// 10B persists windows in `network_action_maintenance_windows` so
// they survive restarts and so multiple API replicas observe the
// same state.
//
// IMPORTANT contract:
//   - Validation goes through ValidateMaintenanceRecord BEFORE INSERT.
//   - ActiveAt SKIPS rows where disabled_at IS NOT NULL (idx_namw_active_only).
//   - Any DB error surfaces to the destructive gate as a deny.
type PgMaintenanceStore struct {
	P *pgxpool.Pool
}

// NewPgMaintenanceStore wires the store.
func NewPgMaintenanceStore(p *pgxpool.Pool) *PgMaintenanceStore {
	return &PgMaintenanceStore{P: p}
}

// CreateInput is the API-level shape a handler fills before calling
// Create. It carries the operator-supplied notes; createdBy is set
// from the authenticated session, never trusted from the request body.
type CreateInput struct {
	Title     string
	Start     time.Time
	End       time.Time
	Scope     []string // optional list of network_devices.id (empty = all)
	CreatedBy string
	Notes     string
}

// Create implements MaintenanceStore (extended). Validates, INSERTs,
// and returns the persisted record (with DB-stamped id + created_at).
func (s *PgMaintenanceStore) Create(ctx context.Context, in CreateInput) (MaintenanceRecord, error) {
	if s == nil || s.P == nil {
		return MaintenanceRecord{}, errors.New("networkactions: maintenance store unavailable")
	}
	rec := MaintenanceRecord{
		Title:     strings.TrimSpace(in.Title),
		Start:     in.Start.UTC(),
		End:       in.End.UTC(),
		Scope:     in.Scope,
		CreatedBy: strings.TrimSpace(in.CreatedBy),
	}
	if err := ValidateMaintenanceRecord(rec); err != nil {
		return MaintenanceRecord{}, err
	}
	if rec.CreatedBy == "" {
		rec.CreatedBy = "system"
	}
	notes := strings.TrimSpace(in.Notes)
	scope := rec.Scope
	if scope == nil {
		scope = []string{}
	}
	row := s.P.QueryRow(ctx, `
INSERT INTO network_action_maintenance_windows
  (title, start_at, end_at, scope, created_by, notes)
VALUES ($1, $2, $3, $4::uuid[], $5, $6)
RETURNING id, created_at`,
		rec.Title, rec.Start, rec.End, scope, rec.CreatedBy, notes)
	var (
		id        string
		createdAt time.Time
	)
	if err := row.Scan(&id, &createdAt); err != nil {
		return MaintenanceRecord{}, err
	}
	rec.ID = id
	rec.CreatedAt = createdAt
	return rec, nil
}

// Get implements MaintenanceStore.
func (s *PgMaintenanceStore) Get(ctx context.Context, id MaintenanceWindowID) (MaintenanceRecord, error) {
	if s == nil || s.P == nil {
		return MaintenanceRecord{}, errors.New("networkactions: maintenance store unavailable")
	}
	row := s.P.QueryRow(ctx, `
SELECT id, title, start_at, end_at, scope, created_by, created_at
FROM network_action_maintenance_windows
WHERE id = $1::uuid`, id)
	var rec MaintenanceRecord
	var scope []string
	if err := row.Scan(&rec.ID, &rec.Title, &rec.Start, &rec.End, &scope, &rec.CreatedBy, &rec.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return MaintenanceRecord{}, ErrMaintenanceWindowNotFound
		}
		return MaintenanceRecord{}, err
	}
	rec.Scope = scope
	return rec, nil
}

// List implements MaintenanceStore. Returns ALL records (active +
// disabled) ordered by Start ascending. The API layer filters as
// needed.
func (s *PgMaintenanceStore) List(ctx context.Context) ([]MaintenanceRecord, error) {
	if s == nil || s.P == nil {
		return nil, errors.New("networkactions: maintenance store unavailable")
	}
	rows, err := s.P.Query(ctx, `
SELECT id, title, start_at, end_at, scope, created_by, created_at
FROM network_action_maintenance_windows
ORDER BY start_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]MaintenanceRecord, 0)
	for rows.Next() {
		var rec MaintenanceRecord
		var scope []string
		if err := rows.Scan(&rec.ID, &rec.Title, &rec.Start, &rec.End, &scope, &rec.CreatedBy, &rec.CreatedAt); err != nil {
			return nil, err
		}
		rec.Scope = scope
		out = append(out, rec)
	}
	return out, rows.Err()
}

// ActiveAt implements MaintenanceProvider. Skips disabled rows and
// rows whose [start_at, end_at) does not contain `at`.
func (s *PgMaintenanceStore) ActiveAt(ctx context.Context, deviceID string, at time.Time) ([]MaintenanceRecord, error) {
	if s == nil || s.P == nil {
		return nil, errors.New("networkactions: maintenance store unavailable")
	}
	at = at.UTC()
	rows, err := s.P.Query(ctx, `
SELECT id, title, start_at, end_at, scope, created_by, created_at
FROM network_action_maintenance_windows
WHERE disabled_at IS NULL
  AND start_at <= $1
  AND end_at   >  $1
ORDER BY start_at ASC`, at)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]MaintenanceRecord, 0)
	for rows.Next() {
		var rec MaintenanceRecord
		var scope []string
		if err := rows.Scan(&rec.ID, &rec.Title, &rec.Start, &rec.End, &scope, &rec.CreatedBy, &rec.CreatedAt); err != nil {
			return nil, err
		}
		rec.Scope = scope
		if !rec.AppliesToDevice(deviceID) {
			continue
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// DisableInput is the API-level shape a handler fills before calling
// Disable.
type DisableInput struct {
	ID            MaintenanceWindowID
	DisabledBy    string
	DisableReason string
}

// Disable marks a window as disabled. Returns ErrMaintenanceWindowNotFound
// if no row exists or already disabled.
func (s *PgMaintenanceStore) Disable(ctx context.Context, in DisableInput) (MaintenanceRecord, error) {
	if s == nil || s.P == nil {
		return MaintenanceRecord{}, errors.New("networkactions: maintenance store unavailable")
	}
	by := strings.TrimSpace(in.DisabledBy)
	reason := strings.TrimSpace(in.DisableReason)
	if by == "" {
		return MaintenanceRecord{}, errors.New("networkactions: disable requires non-empty actor")
	}
	if reason == "" {
		return MaintenanceRecord{}, errors.New("networkactions: disable requires non-empty reason")
	}
	row := s.P.QueryRow(ctx, `
UPDATE network_action_maintenance_windows
SET disabled_at = now(),
    disabled_by = $2,
    disable_reason = $3
WHERE id = $1::uuid AND disabled_at IS NULL
RETURNING id, title, start_at, end_at, scope, created_by, created_at`,
		in.ID, by, reason)
	var rec MaintenanceRecord
	var scope []string
	if err := row.Scan(&rec.ID, &rec.Title, &rec.Start, &rec.End, &scope, &rec.CreatedBy, &rec.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return MaintenanceRecord{}, ErrMaintenanceWindowNotFound
		}
		return MaintenanceRecord{}, err
	}
	rec.Scope = scope
	return rec, nil
}
