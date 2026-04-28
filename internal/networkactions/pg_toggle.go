package networkactions

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgToggleStore is the production DestructiveToggle backed by the
// `network_action_toggle_flips` table. Phase 10B introduces this so
// the master switch survives a restart and every flip is captured
// in an append-only audit table.
//
// IMPORTANT contract:
//   - Default state is closed: when the table holds no rows,
//     Enabled() returns false. The newest row decides the current
//     state.
//   - Every Flip is INSERTed; we never UPDATE a previous row.
//     Auditors can replay the full history later.
//   - Any DB error MUST surface as fail-closed: Enabled() returns
//     false + error so the gate never accidentally opens.
type PgToggleStore struct {
	P *pgxpool.Pool
}

// NewPgToggleStore wires the store. The pool MUST be non-nil; the
// API layer enforces this at boot.
func NewPgToggleStore(p *pgxpool.Pool) *PgToggleStore { return &PgToggleStore{P: p} }

// Enabled implements DestructiveToggle.
//
// Reads the most recent row from network_action_toggle_flips. If
// the table is empty, returns (false, nil) — the default-closed
// invariant. Any DB error returns (false, err) so callers can
// fail-closed.
func (s *PgToggleStore) Enabled(ctx context.Context) (bool, error) {
	if s == nil || s.P == nil {
		return false, ErrToggleStoreUnavailable
	}
	row := s.P.QueryRow(ctx, `
SELECT enabled
FROM network_action_toggle_flips
ORDER BY flipped_at DESC, id DESC
LIMIT 1`)
	var enabled bool
	if err := row.Scan(&enabled); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return enabled, nil
}

// Flip implements DestructiveToggle. Refuses empty actor/reason so
// the audit trail can always answer "who/why". Returns the persisted
// receipt (with DB-stamped flipped_at).
func (s *PgToggleStore) Flip(ctx context.Context, enabled bool, actor, reason string) (FlipReceipt, error) {
	actor = strings.TrimSpace(actor)
	reason = strings.TrimSpace(reason)
	if actor == "" {
		return FlipReceipt{}, errors.New("networkactions: toggle flip requires non-empty actor")
	}
	if reason == "" {
		return FlipReceipt{}, errors.New("networkactions: toggle flip requires non-empty reason")
	}
	if s == nil || s.P == nil {
		return FlipReceipt{}, ErrToggleStoreUnavailable
	}
	var flippedAt time.Time
	row := s.P.QueryRow(ctx, `
INSERT INTO network_action_toggle_flips (enabled, actor, reason)
VALUES ($1, $2, $3)
RETURNING flipped_at`, enabled, actor, reason)
	if err := row.Scan(&flippedAt); err != nil {
		return FlipReceipt{}, err
	}
	return FlipReceipt{
		Enabled:   enabled,
		Actor:     actor,
		Reason:    reason,
		FlippedAt: flippedAt,
	}, nil
}

// LastFlip returns the most recent receipt (or nil if none). Used
// by the API surface that exposes "current destructive state" to
// the operator. DB errors return (nil, err); the caller surfaces
// the error.
func (s *PgToggleStore) LastFlip(ctx context.Context) (*FlipReceipt, error) {
	if s == nil || s.P == nil {
		return nil, ErrToggleStoreUnavailable
	}
	row := s.P.QueryRow(ctx, `
SELECT enabled, actor, reason, flipped_at
FROM network_action_toggle_flips
ORDER BY flipped_at DESC, id DESC
LIMIT 1`)
	var r FlipReceipt
	if err := row.Scan(&r.Enabled, &r.Actor, &r.Reason, &r.FlippedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &r, nil
}
