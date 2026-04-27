-- 000008_mikrotik_dude_discovery.sql
-- Faz 8: MikroTik Dude SSH discovery + Network Inventory.
--
-- Bu migration idempotent + transactional. DROP yapılmaz.
-- Faz 8 sadece read-only discovery uygular; bu şemada tutulan
-- network_devices/network_links tabloları discovery sonuçlarını
-- saklamak içindir.
--
-- Action framework iskeleti (network_automation_jobs) Faz 8'de
-- yalnızca discovery zamanlamasını destekler; aktif/destructive
-- aksiyonlar sonraki fazlara bırakılmıştır.

BEGIN;

-- ============================================================================
-- 1. discovery_runs: Bir discovery koşusunun başı/sonu, sonuç özeti.
-- ============================================================================
CREATE TABLE IF NOT EXISTS discovery_runs (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  source          text NOT NULL DEFAULT 'mikrotik_dude'
                  CHECK (source IN ('mikrotik_dude')),
  correlation_id  text NOT NULL,
  started_at      timestamptz NOT NULL DEFAULT now(),
  finished_at     timestamptz,
  status          text NOT NULL DEFAULT 'running'
                  CHECK (status IN ('running','succeeded','failed','partial')),
  device_count    int  NOT NULL DEFAULT 0,
  ap_count        int  NOT NULL DEFAULT 0,
  cpe_count       int  NOT NULL DEFAULT 0,
  bridge_count    int  NOT NULL DEFAULT 0,
  link_count      int  NOT NULL DEFAULT 0,
  router_count    int  NOT NULL DEFAULT 0,
  switch_count    int  NOT NULL DEFAULT 0,
  unknown_count   int  NOT NULL DEFAULT 0,
  low_conf_count  int  NOT NULL DEFAULT 0,
  error_code      text,
  error_message   text,
  commands_run    text[] NOT NULL DEFAULT '{}',
  triggered_by    text NOT NULL DEFAULT 'system',
  created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_discovery_runs_started
  ON discovery_runs (started_at DESC);

CREATE INDEX IF NOT EXISTS idx_discovery_runs_status
  ON discovery_runs (status, started_at DESC);

-- ============================================================================
-- 2. network_devices: Dude'tan çıkarılan envanter.
-- ============================================================================
CREATE TABLE IF NOT EXISTS network_devices (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  source        text NOT NULL DEFAULT 'mikrotik_dude'
                CHECK (source IN ('mikrotik_dude')),
  host          inet,
  name          text NOT NULL DEFAULT '',
  mac           text,
  model         text,
  os_version    text,
  identity      text,
  device_type   text,
  category      text NOT NULL DEFAULT 'Unknown'
                CHECK (category IN ('AP','BackhaulLink','Bridge','CPE','Router','Switch','Unknown')),
  confidence    int  NOT NULL DEFAULT 0
                CHECK (confidence BETWEEN 0 AND 100),
  status        text NOT NULL DEFAULT 'unknown'
                CHECK (status IN ('up','down','partial','unknown')),
  last_seen_at  timestamptz NOT NULL DEFAULT now(),
  raw_metadata  jsonb NOT NULL DEFAULT '{}'::jsonb,
  first_seen_at timestamptz NOT NULL DEFAULT now(),
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_netdev_source_mac
  ON network_devices (source, mac)
  WHERE mac IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_netdev_source_host_name
  ON network_devices (source, host, name)
  WHERE host IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_netdev_source_name_when_no_id
  ON network_devices (source, name)
  WHERE host IS NULL AND mac IS NULL AND name <> '';

CREATE INDEX IF NOT EXISTS idx_netdev_category ON network_devices (category);
CREATE INDEX IF NOT EXISTS idx_netdev_status   ON network_devices (status);
CREATE INDEX IF NOT EXISTS idx_netdev_last_seen ON network_devices (last_seen_at DESC);
CREATE INDEX IF NOT EXISTS idx_netdev_low_conf  ON network_devices (confidence)
  WHERE confidence < 50;

-- ============================================================================
-- 3. network_links: Backhaul / link tipi cihazlar için iki uçlu kayıt.
-- ============================================================================
CREATE TABLE IF NOT EXISTS network_links (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  source          text NOT NULL DEFAULT 'mikrotik_dude',
  from_device_id  uuid REFERENCES network_devices(id) ON DELETE CASCADE,
  to_device_id    uuid REFERENCES network_devices(id) ON DELETE CASCADE,
  link_type       text NOT NULL DEFAULT 'unknown',
  status          text NOT NULL DEFAULT 'unknown',
  raw_metadata    jsonb NOT NULL DEFAULT '{}'::jsonb,
  last_seen_at    timestamptz NOT NULL DEFAULT now(),
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_netlink_pair
  ON network_links (source, from_device_id, to_device_id)
  WHERE from_device_id IS NOT NULL AND to_device_id IS NOT NULL;

-- ============================================================================
-- 4. device_category_evidence: Sınıflandırma için biriken ipuçları.
-- ============================================================================
CREATE TABLE IF NOT EXISTS device_category_evidence (
  id          bigserial PRIMARY KEY,
  device_id   uuid NOT NULL REFERENCES network_devices(id) ON DELETE CASCADE,
  run_id      uuid REFERENCES discovery_runs(id) ON DELETE SET NULL,
  heuristic   text NOT NULL,
  category    text NOT NULL
              CHECK (category IN ('AP','BackhaulLink','Bridge','CPE','Router','Switch','Unknown')),
  weight      int  NOT NULL DEFAULT 0,
  reason      text NOT NULL DEFAULT '',
  created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_dce_device   ON device_category_evidence (device_id);
CREATE INDEX IF NOT EXISTS idx_dce_run      ON device_category_evidence (run_id);

-- ============================================================================
-- 5. network_automation_jobs: Action framework için zamanlama tablosu.
--    Faz 8 yalnızca 'discovery' job_type'ını destekler. Destructive
--    aksiyonlar (frequency_correction, reboot vs.) sonraki fazlarda.
-- ============================================================================
CREATE TABLE IF NOT EXISTS network_automation_jobs (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  job_type        text NOT NULL CHECK (job_type IN ('discovery')),
  source          text NOT NULL DEFAULT 'mikrotik_dude',
  cron_expr       text,
  enabled         boolean NOT NULL DEFAULT true,
  last_run_at     timestamptz,
  last_run_id     uuid REFERENCES discovery_runs(id) ON DELETE SET NULL,
  last_status     text,
  config          jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_naj_enabled ON network_automation_jobs (enabled, job_type);

COMMIT;
