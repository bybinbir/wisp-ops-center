-- 000012_phase10b_maintenance_window_disable.sql
-- Faz 10B: Postgres-backed safety stores + API surface.
--
-- Phase 10A added the network_action_maintenance_windows table.
-- Phase 10B adds the columns needed to support the
-- PATCH .../disable endpoint without losing audit history:
--
--   * disabled_at      — NULL while the window is active; set to
--                        a timestamp when the operator cancels it.
--                        ActiveAt queries skip rows where disabled_at
--                        IS NOT NULL.
--   * disabled_by      — actor that disabled the window.
--   * disable_reason   — operator-supplied reason (audit trail).
--   * notes            — optional operator note attached at create
--                        time (Phase 10B API requires non-empty
--                        operator + reason on create; notes is a
--                        free-form supplement).
--
-- The migration is idempotent + transactional. ALTER/CREATE only,
-- no DROP. Existing Phase 5 `maintenance_windows` table is NOT
-- touched (different domain).

BEGIN;

ALTER TABLE network_action_maintenance_windows
  ADD COLUMN IF NOT EXISTS disabled_at     timestamptz,
  ADD COLUMN IF NOT EXISTS disabled_by     text,
  ADD COLUMN IF NOT EXISTS disable_reason  text,
  ADD COLUMN IF NOT EXISTS notes           text NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_namw_active_only
  ON network_action_maintenance_windows (start_at, end_at)
  WHERE disabled_at IS NULL;

COMMIT;
