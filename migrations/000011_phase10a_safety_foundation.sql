-- 000011_phase10a_safety_foundation.sql
-- Faz 10A: Destructive-action safety foundation.
--
-- Phase 10A keeps DestructiveActionEnabled=false and ships only the
-- supporting tables a future destructive action would need:
--
--   1. network_action_toggle_flips  — append-only audit of every
--      operator toggle flip (who turned destructive on/off, when,
--      why). Phase 10A's MemoryToggle does NOT yet write here, but
--      Phase 10's Postgres-backed toggle will. The table exists now
--      so the schema is in place and idempotency/replay is proven.
--
--   2. network_action_maintenance_windows — operator-declared
--      maintenance windows for destructive network actions. Phase
--      10A ships an in-memory store; this table is the canonical
--      persistence layer Phase 10 will adopt. NOTE: this table is
--      INTENTIONALLY separate from the existing `maintenance_windows`
--      table (Phase 5 scheduled-checks domain) to avoid conflating
--      two different semantics under one name.
--
-- The migration is idempotent + transactional. ALTER/CREATE only,
-- no DROP. Nothing here changes the default-closed behavior of the
-- destructive toggle: a freshly-applied schema means "no flip ever
-- happened, no maintenance window is open, gate stays fail-closed".

BEGIN;

-- ============================================================================
-- network_action_toggle_flips: append-only audit of master switch
-- ============================================================================
CREATE TABLE IF NOT EXISTS network_action_toggle_flips (
  id          bigserial PRIMARY KEY,
  enabled     boolean NOT NULL,
  actor       text NOT NULL,
  reason      text NOT NULL,
  flipped_at  timestamptz NOT NULL DEFAULT now(),
  CHECK (length(actor) > 0),
  CHECK (length(reason) > 0)
);

CREATE INDEX IF NOT EXISTS idx_natoggle_flipped
  ON network_action_toggle_flips (flipped_at DESC);

CREATE INDEX IF NOT EXISTS idx_natoggle_enabled
  ON network_action_toggle_flips (enabled, flipped_at DESC);

-- ============================================================================
-- network_action_maintenance_windows: operator-declared change
-- windows for destructive network actions (Phase 10).
-- ============================================================================
CREATE TABLE IF NOT EXISTS network_action_maintenance_windows (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  title        text NOT NULL CHECK (length(title) > 0),
  -- start_at/end_at are timestamptz; end_at must be strictly > start_at.
  start_at     timestamptz NOT NULL,
  end_at       timestamptz NOT NULL CHECK (end_at > start_at),
  -- scope is the optional list of network_devices.id values this
  -- window covers; empty means "all devices in this tenant".
  scope        uuid[] NOT NULL DEFAULT '{}',
  created_by   text NOT NULL DEFAULT 'system',
  created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_namw_active
  ON network_action_maintenance_windows (start_at, end_at);

CREATE INDEX IF NOT EXISTS idx_namw_created
  ON network_action_maintenance_windows (created_at DESC);

COMMIT;
