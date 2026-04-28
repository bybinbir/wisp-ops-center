-- 000013_phase10c_destructive_runtime_lifecycle.sql
-- Faz 10C: Destructive runtime lifecycle infrastructure (no execution).
--
-- Phase 10C wires the runtime scaffolding a destructive action would
-- need to survive in production: SQL-backed RBAC grants, idempotency
-- DB-level uniqueness, rollback metadata persistence. NO destructive
-- execution path is opened by this migration. The master switch
-- (DestructiveActionEnabled=false + latest network_action_toggle_flips
-- enabled=false) is preserved.
--
-- This migration is idempotent + transactional. ALTER/CREATE only,
-- no DROP. Existing Phase 5 `maintenance_windows` table is NOT
-- touched. Phase 10A (000011) and Phase 10B (000012) tables are
-- consumed read-only here; only `network_action_runs` (000010) gets
-- additive columns.
--
-- Rollback plan:
--   * Schema: every change is `IF NOT EXISTS`; reverting Phase 10C
--     code makes the new columns and table unused but harmless.
--   * Data: `network_action_role_grants` is operator-managed; rolling
--     back means clearing it (TRUNCATE) and falling back to the
--     header-based static resolver.

BEGIN;

-- ============================================================================
-- network_action_role_grants — actor → roles binding (RBAC SQL store).
--
-- Phase 10B's `PgRBACResolver` was a typed seam that always delegated
-- to the static fallback. Phase 10C lets operators bind real actor
-- identities to role lists in this table. The resolver looks up the
-- principal here first; rows take precedence over header-supplied
-- roles. When `WISP_RBAC_REQUIRE_SQL=true`, missing actor → deny.
-- ============================================================================
CREATE TABLE IF NOT EXISTS network_action_role_grants (
  actor       text PRIMARY KEY CHECK (length(actor) > 0),
  roles       text[] NOT NULL DEFAULT '{}',
  granted_by  text NOT NULL CHECK (length(granted_by) > 0),
  granted_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  notes       text NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_narg_updated
  ON network_action_role_grants (updated_at DESC);

-- ============================================================================
-- network_action_runs additive columns — idempotency + rollback metadata.
--
-- A destructive request MUST carry an idempotency_key + rollback_note
-- + intent. The unique partial index makes a duplicate POST observable
-- at the DB layer (Phase 10C handler returns 409 idempotency_reused).
-- The `intent` column is a stable string the operator types so the
-- same idempotency_key cannot accidentally cover two different intents.
-- ============================================================================
ALTER TABLE network_action_runs
  ADD COLUMN IF NOT EXISTS idempotency_key text,
  ADD COLUMN IF NOT EXISTS intent          text,
  ADD COLUMN IF NOT EXISTS rollback_note   text;

-- Unique on (action_type, idempotency_key) for any non-NULL key. We
-- intentionally do NOT include target_device_id in the key so that an
-- operator cannot replay the same intent against a different device
-- by reusing the key. The partial predicate keeps Phase 9 read-only
-- runs (idempotency_key IS NULL) free of any new constraint.
CREATE UNIQUE INDEX IF NOT EXISTS uniq_nar_action_idem
  ON network_action_runs (action_type, idempotency_key)
  WHERE idempotency_key IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_nar_intent
  ON network_action_runs (intent)
  WHERE intent IS NOT NULL;

COMMIT;
