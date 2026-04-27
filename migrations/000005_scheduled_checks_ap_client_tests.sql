-- =================================================================
-- wisp-ops-center  ::  Faz 5
--   * Scheduler engine (scheduled_checks alanları, scheduler_locks)
--   * Maintenance windows
--   * AP-to-client safe test runtime
--   * SSH known_hosts (TOFU)
-- =================================================================

-- --- scheduled_checks: Phase 5 yeni alanları ---------------------
ALTER TABLE scheduled_checks
    ADD COLUMN IF NOT EXISTS schedule_type TEXT NOT NULL DEFAULT 'manual',
    ADD COLUMN IF NOT EXISTS cron_expression TEXT,
    ADD COLUMN IF NOT EXISTS timezone TEXT NOT NULL DEFAULT 'UTC',
    ADD COLUMN IF NOT EXISTS interval_sec INTEGER,
    ADD COLUMN IF NOT EXISTS scope_type TEXT NOT NULL DEFAULT 'all_network',
    ADD COLUMN IF NOT EXISTS scope_id   TEXT,
    ADD COLUMN IF NOT EXISTS risk_level TEXT NOT NULL DEFAULT 'low',
    ADD COLUMN IF NOT EXISTS maintenance_window_id UUID,
    ADD COLUMN IF NOT EXISTS max_duration_seconds INTEGER NOT NULL DEFAULT 60,
    ADD COLUMN IF NOT EXISTS max_parallel INTEGER NOT NULL DEFAULT 4,
    ADD COLUMN IF NOT EXISTS approved_by TEXT,
    ADD COLUMN IF NOT EXISTS approved_at TIMESTAMPTZ;

ALTER TABLE scheduled_checks
    DROP CONSTRAINT IF EXISTS scheduled_checks_schedule_type_check;
ALTER TABLE scheduled_checks
    ADD CONSTRAINT scheduled_checks_schedule_type_check
    CHECK (schedule_type IN ('manual','daily','weekly','monthly','one_time','interval'));

ALTER TABLE scheduled_checks
    DROP CONSTRAINT IF EXISTS scheduled_checks_scope_type_check;
ALTER TABLE scheduled_checks
    ADD CONSTRAINT scheduled_checks_scope_type_check
    CHECK (scope_type IN ('all_network','site','tower','device','customer_group','customer','link'));

ALTER TABLE scheduled_checks
    DROP CONSTRAINT IF EXISTS scheduled_checks_risk_level_check;
ALTER TABLE scheduled_checks
    ADD CONSTRAINT scheduled_checks_risk_level_check
    CHECK (risk_level IN ('low','medium','high'));

CREATE INDEX IF NOT EXISTS idx_sched_checks_next_run
    ON scheduled_checks(next_run_at) WHERE enabled = TRUE AND mode <> 'controlled_apply';
CREATE INDEX IF NOT EXISTS idx_sched_checks_job_type ON scheduled_checks(job_type);

-- --- scheduler_locks: distributed lock to avoid double execution -
CREATE TABLE IF NOT EXISTS scheduler_locks (
    name        TEXT PRIMARY KEY,
    holder      TEXT NOT NULL,
    acquired_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ NOT NULL
);

-- --- job_runs: Phase 5 yeni alanları -----------------------------
-- (job_runs tablosu Phase 1'de yaratıldı; eksik alanları doldur.)
ALTER TABLE job_runs
    ADD COLUMN IF NOT EXISTS scope_type TEXT,
    ADD COLUMN IF NOT EXISTS scope_id   TEXT,
    ADD COLUMN IF NOT EXISTS duration_ms INTEGER,
    ADD COLUMN IF NOT EXISTS error_code  TEXT;

CREATE INDEX IF NOT EXISTS idx_job_runs_started ON job_runs(started_at DESC);

-- --- maintenance_windows -----------------------------------------
CREATE TABLE IF NOT EXISTS maintenance_windows (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT NOT NULL,
    scope_type   TEXT NOT NULL DEFAULT 'all_network',
    scope_id     TEXT,
    starts_at    TIMESTAMPTZ NOT NULL,
    ends_at      TIMESTAMPTZ NOT NULL,
    timezone     TEXT NOT NULL DEFAULT 'UTC',
    recurrence   TEXT NOT NULL DEFAULT '',
    enabled      BOOLEAN NOT NULL DEFAULT TRUE,
    notes        TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE maintenance_windows
    DROP CONSTRAINT IF EXISTS maintenance_windows_recurrence_check;
ALTER TABLE maintenance_windows
    ADD CONSTRAINT maintenance_windows_recurrence_check
    CHECK (recurrence IN ('','daily','weekly','monthly'));

CREATE INDEX IF NOT EXISTS idx_mw_starts_ends ON maintenance_windows(starts_at, ends_at);

-- --- ap_client_test_results: Faz 5 ek alanları --------------------
ALTER TABLE ap_client_test_results
    ADD COLUMN IF NOT EXISTS test_type      TEXT,
    ADD COLUMN IF NOT EXISTS target_ip      INET,
    ADD COLUMN IF NOT EXISTS latency_min_ms NUMERIC(8,2),
    ADD COLUMN IF NOT EXISTS latency_avg_ms NUMERIC(8,2),
    ADD COLUMN IF NOT EXISTS latency_max_ms NUMERIC(8,2),
    ADD COLUMN IF NOT EXISTS hop_count      INTEGER,
    ADD COLUMN IF NOT EXISTS status         TEXT NOT NULL DEFAULT 'success',
    ADD COLUMN IF NOT EXISTS error_code     TEXT,
    ADD COLUMN IF NOT EXISTS error_message  TEXT;

ALTER TABLE ap_client_test_results
    DROP CONSTRAINT IF EXISTS ap_client_test_results_test_type_check;
ALTER TABLE ap_client_test_results
    ADD CONSTRAINT ap_client_test_results_test_type_check
    CHECK (test_type IS NULL OR test_type IN
      ('ping_latency','packet_loss','jitter','traceroute',
       'limited_throughput','mikrotik_bandwidth_test'));

ALTER TABLE ap_client_test_results
    DROP CONSTRAINT IF EXISTS ap_client_test_results_status_check;
ALTER TABLE ap_client_test_results
    ADD CONSTRAINT ap_client_test_results_status_check
    CHECK (status IN ('success','partial','failed','blocked'));

CREATE INDEX IF NOT EXISTS idx_ap_results_status ON ap_client_test_results(status);

-- --- ssh_known_hosts (TOFU) --------------------------------------
CREATE TABLE IF NOT EXISTS ssh_known_hosts (
    host        TEXT PRIMARY KEY,
    fingerprint TEXT NOT NULL,
    seen_first  TIMESTAMPTZ NOT NULL DEFAULT now(),
    seen_last   TIMESTAMPTZ NOT NULL DEFAULT now(),
    notes       TEXT
);

-- =================================================================
-- Faz 5 sonu — 000006 Faz 6 (skor motoru) ile gelecek.
-- =================================================================
