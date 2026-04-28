-- 000010_network_action_runs.sql
-- Faz 9: Read-only network action framework + frequency_check.
--
-- Table holds one row per action invocation. Phase 9 ships only
-- read-only actions (frequency_check). Future destructive actions
-- (frequency_correction) MUST honor the dry_run flag and the
-- maintenance window contract; the schema is shaped so those phases
-- can land without any further DDL.
--
-- Idempotent + transactional. ALTER/CREATE only, no DROP.

BEGIN;

-- ============================================================================
-- network_action_runs: One execution of one network action.
-- ============================================================================
CREATE TABLE IF NOT EXISTS network_action_runs (
  id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  action_type      text NOT NULL
                   CHECK (action_type IN (
                     'frequency_check',
                     'frequency_correction',
                     'ap_client_test',
                     'link_signal_test',
                     'bridge_health_check',
                     'maintenance_window'
                   )),
  -- Phase 9 binds to network_devices (Faz 8 inventory). target_device_id
  -- may be NULL when the action targets a raw host (operator-provided
  -- IP) without an inventory record.
  target_device_id uuid REFERENCES network_devices(id) ON DELETE SET NULL,
  target_host      inet,
  target_label     text NOT NULL DEFAULT '',

  status           text NOT NULL DEFAULT 'queued'
                   CHECK (status IN ('queued','running','succeeded','failed','skipped')),

  started_at       timestamptz,
  finished_at      timestamptz,
  duration_ms      bigint NOT NULL DEFAULT 0,

  actor            text NOT NULL DEFAULT 'system',
  correlation_id   text NOT NULL DEFAULT '',
  dry_run          boolean NOT NULL DEFAULT true,

  -- Sanitized result payload. NEVER contains raw secrets — the action
  -- runner redacts known secret-bearing keys before persisting.
  result           jsonb NOT NULL DEFAULT '{}'::jsonb,

  command_count    int  NOT NULL DEFAULT 0,
  warning_count    int  NOT NULL DEFAULT 0,
  confidence       int  NOT NULL DEFAULT 0
                   CHECK (confidence BETWEEN 0 AND 100),

  error_code       text,
  error_message    text,

  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_nar_started     ON network_action_runs (started_at DESC NULLS LAST);
CREATE INDEX IF NOT EXISTS idx_nar_status      ON network_action_runs (status, started_at DESC NULLS LAST);
CREATE INDEX IF NOT EXISTS idx_nar_action_type ON network_action_runs (action_type, started_at DESC NULLS LAST);
CREATE INDEX IF NOT EXISTS idx_nar_target      ON network_action_runs (target_device_id);

COMMIT;
