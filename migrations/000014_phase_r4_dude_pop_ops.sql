-- 000014_phase_r4_dude_pop_ops.sql
-- Faz R4: Operator-provided POP/IP inventory + Read-Only Telemetry.
--
-- Bu migration WISP-R4-DUDE-TO-POP-OPS-FINISH prompt'unun veri
-- katmanı ihtiyacını karşılar. Karar (operatör onayı):
--   • Ana seed kaynağı operatörün YAML POP/IP envanteri.
--   • Dude binary DB parser kritik path DEĞİL; Dude sadece
--     secondary enrichment.
--   • Probe sonucu manuel mapping'le çelişirse `mapping_conflict`
--     state'i üretilir, sessiz overwrite YAPILMAZ.
--   • Bu fazda mutation 0, secret leak 0, fake success 0.
--
-- Mevcut tablolar (`network_devices` = device_inventory,
-- `device_category_evidence` = device_classification_evidence)
-- yeniden oluşturulmaz; R4'e özgü 13 yeni tablo eklenir.
--
-- Kapsam:
--   • Operator topology: pop_topology_imports (YAML import audit)
--   • Probe lifecycle: device_probe_runs, device_raw_snapshots
--   • Normalized telemetry: device_interfaces,
--     device_wireless_interfaces, device_wireless_clients,
--     device_bridge_ports, device_neighbors, device_link_metrics
--   • POP grouping: pop_groups, pop_device_membership
--   • Reporting: weak_client_findings, frequency_plan_runs
--
-- Idempotent + transactional. CREATE/ALTER only, no DROP.
-- Tüm raw payload kolonları 'redacted' marker'ı taşır; redaction
-- enforcement Go layer'da `internal/devicectl/redact.go` ile
-- yapılır. SQL constraint sadece "non-null kontrol" sağlar.
--
-- Hiçbir destructive runtime path açılmaz; bu migration sadece
-- okunmuş veriyi saklar. Master switch (Phase 10A-C) etkilenmez.
--
-- Rollback plan: kolonlar/tablolar kullanılmazsa zararsızdır;
-- veri silmek isteyen operatör TRUNCATE ile başlayıp DROP TABLE
-- yapar.

BEGIN;

-- ============================================================================
-- 1. device_probe_runs — her read-only probe koşusunun lifecycle kaydı.
--
-- Operatör bir cihazın deep telemetry'sine bakmak istediğinde,
-- veya scheduler periyodik olarak çalıştırdığında bu tablo bir
-- satır kazanır. `transport` enum hangi protokolün kullanıldığını
-- saklar (routeros_api, ssh, snmp, routeros_rest, mimosa_http,
-- mimosa_ssh, mimosa_snmp, dude_seed, manual_winbox).
-- `probe_status` Go layer'ın honest sonucu; sahte success yok.
-- ============================================================================
CREATE TABLE IF NOT EXISTS device_probe_runs (
  id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  device_id         uuid REFERENCES network_devices(id) ON DELETE CASCADE,
  ip                inet,
  transport         text NOT NULL
                    CHECK (transport IN (
                      'routeros_api','ssh','snmp','routeros_rest',
                      'mimosa_http','mimosa_ssh','mimosa_snmp',
                      'dude_seed','manual_winbox'
                    )),
  credential_profile text,
  started_at        timestamptz NOT NULL DEFAULT now(),
  finished_at       timestamptz,
  duration_ms       int,
  probe_status      text NOT NULL DEFAULT 'pending'
                    CHECK (probe_status IN (
                      'pending','succeeded','partial','timeout',
                      'unreachable','credential_failed','protocol_error',
                      'parser_error','blocked_by_allowlist','unknown'
                    )),
  parser_version    text NOT NULL DEFAULT '',
  error_state       text,
  error_message     text,
  confidence_score  int  NOT NULL DEFAULT 0
                    CHECK (confidence_score BETWEEN 0 AND 100),
  triggered_by      text NOT NULL DEFAULT 'system',
  correlation_id    text NOT NULL DEFAULT '',
  created_at        timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_dpr_device     ON device_probe_runs (device_id);
CREATE INDEX IF NOT EXISTS idx_dpr_started    ON device_probe_runs (started_at DESC);
CREATE INDEX IF NOT EXISTS idx_dpr_status     ON device_probe_runs (probe_status, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_dpr_transport  ON device_probe_runs (transport, started_at DESC);

-- ============================================================================
-- 2. device_raw_snapshots — redacted ham probe yanıtı.
--
-- Her probe sonucu burada saklanır. payload_redacted JSONB
-- olarak tutulur; payload SADECE Go redactor'dan geçtikten sonra
-- yazılır (password/secret/key/token/ppp-secret alanları
-- maskelenmiş halde). `redaction_version` redactor sürümünü
-- işaretler — gelecekte redaction kuralları sıkılaşırsa eski
-- snapshot'ları yeniden işleyebiliriz.
-- ============================================================================
CREATE TABLE IF NOT EXISTS device_raw_snapshots (
  id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  probe_run_id        uuid NOT NULL REFERENCES device_probe_runs(id) ON DELETE CASCADE,
  device_id           uuid REFERENCES network_devices(id) ON DELETE CASCADE,
  command_or_endpoint text NOT NULL,
  payload_redacted    jsonb NOT NULL DEFAULT '{}'::jsonb,
  payload_text        text,
  byte_size           int  NOT NULL DEFAULT 0,
  redaction_version   text NOT NULL DEFAULT 'v1',
  collected_at        timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_drs_run     ON device_raw_snapshots (probe_run_id);
CREATE INDEX IF NOT EXISTS idx_drs_device  ON device_raw_snapshots (device_id, collected_at DESC);
CREATE INDEX IF NOT EXISTS idx_drs_command ON device_raw_snapshots (command_or_endpoint);

-- ============================================================================
-- 3. device_interfaces — `/interface print detail` normalize edilmiş hali.
-- ============================================================================
CREATE TABLE IF NOT EXISTS device_interfaces (
  id              bigserial PRIMARY KEY,
  device_id       uuid NOT NULL REFERENCES network_devices(id) ON DELETE CASCADE,
  probe_run_id    uuid REFERENCES device_probe_runs(id) ON DELETE SET NULL,
  name            text NOT NULL,
  type            text,
  mac             text,
  mtu             int,
  running         boolean,
  disabled        boolean,
  comment         text,
  rx_bytes        bigint,
  tx_bytes        bigint,
  last_link_up_at timestamptz,
  collected_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_di_device ON device_interfaces (device_id, collected_at DESC);
CREATE INDEX IF NOT EXISTS idx_di_name   ON device_interfaces (device_id, name);

-- ============================================================================
-- 4. device_wireless_interfaces — `/interface wireless print detail`.
-- ============================================================================
CREATE TABLE IF NOT EXISTS device_wireless_interfaces (
  id                  bigserial PRIMARY KEY,
  device_id           uuid NOT NULL REFERENCES network_devices(id) ON DELETE CASCADE,
  probe_run_id        uuid REFERENCES device_probe_runs(id) ON DELETE SET NULL,
  name                text NOT NULL,
  ssid                text,
  band                text,
  frequency_mhz       int,
  channel_width       text,
  wireless_mode       text,
  tx_power_dbm        int,
  noise_floor_dbm     int,
  registered_clients  int,
  ccq_pct             int,
  comment             text,
  collected_at        timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_dwi_device      ON device_wireless_interfaces (device_id, collected_at DESC);
CREATE INDEX IF NOT EXISTS idx_dwi_freq        ON device_wireless_interfaces (frequency_mhz)
  WHERE frequency_mhz IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_dwi_mode        ON device_wireless_interfaces (wireless_mode);

-- ============================================================================
-- 5. device_wireless_clients — `/interface wireless registration-table`.
-- ============================================================================
CREATE TABLE IF NOT EXISTS device_wireless_clients (
  id                bigserial PRIMARY KEY,
  device_id         uuid NOT NULL REFERENCES network_devices(id) ON DELETE CASCADE,
  probe_run_id      uuid REFERENCES device_probe_runs(id) ON DELETE SET NULL,
  interface_name    text NOT NULL,
  client_mac        text NOT NULL,
  signal_dbm        int,
  signal_to_noise   int,
  ccq_tx_pct        int,
  ccq_rx_pct        int,
  tx_rate_mbps      int,
  rx_rate_mbps      int,
  uptime_seconds    bigint,
  last_activity_ms  bigint,
  comment           text,
  collected_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_dwc_device     ON device_wireless_clients (device_id, collected_at DESC);
CREATE INDEX IF NOT EXISTS idx_dwc_iface      ON device_wireless_clients (device_id, interface_name);
CREATE INDEX IF NOT EXISTS idx_dwc_mac        ON device_wireless_clients (client_mac);
CREATE INDEX IF NOT EXISTS idx_dwc_weak       ON device_wireless_clients (device_id, signal_dbm)
  WHERE signal_dbm IS NOT NULL AND signal_dbm < -75;

-- ============================================================================
-- 6. device_bridge_ports — `/interface bridge port print detail`.
-- ============================================================================
CREATE TABLE IF NOT EXISTS device_bridge_ports (
  id              bigserial PRIMARY KEY,
  device_id       uuid NOT NULL REFERENCES network_devices(id) ON DELETE CASCADE,
  probe_run_id    uuid REFERENCES device_probe_runs(id) ON DELETE SET NULL,
  bridge_name     text NOT NULL,
  port_name       text NOT NULL,
  enabled         boolean,
  pvid            int,
  edge            text,
  point_to_point  text,
  collected_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_dbp_device  ON device_bridge_ports (device_id, collected_at DESC);

-- ============================================================================
-- 7. device_neighbors — `/ip neighbor print detail`.
-- ============================================================================
CREATE TABLE IF NOT EXISTS device_neighbors (
  id              bigserial PRIMARY KEY,
  device_id       uuid NOT NULL REFERENCES network_devices(id) ON DELETE CASCADE,
  probe_run_id    uuid REFERENCES device_probe_runs(id) ON DELETE SET NULL,
  interface_name  text,
  neighbor_ip     inet,
  neighbor_mac    text,
  neighbor_id     text,
  platform        text,
  identity        text,
  version         text,
  board           text,
  age_seconds     int,
  collected_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_dn_device  ON device_neighbors (device_id, collected_at DESC);
CREATE INDEX IF NOT EXISTS idx_dn_mac     ON device_neighbors (neighbor_mac);

-- ============================================================================
-- 8. device_link_metrics — Mimosa/PtP link telemetrisi.
-- ============================================================================
CREATE TABLE IF NOT EXISTS device_link_metrics (
  id                  bigserial PRIMARY KEY,
  device_id           uuid NOT NULL REFERENCES network_devices(id) ON DELETE CASCADE,
  probe_run_id        uuid REFERENCES device_probe_runs(id) ON DELETE SET NULL,
  link_status         text,
  rssi_dbm            int,
  snr_db              int,
  frequency_mhz       int,
  channel_width       text,
  capacity_mbps       int,
  throughput_mbps     int,
  uptime_seconds      bigint,
  remote_peer_id      text,
  remote_peer_ip      inet,
  collected_at        timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_dlm_device ON device_link_metrics (device_id, collected_at DESC);
CREATE INDEX IF NOT EXISTS idx_dlm_status ON device_link_metrics (link_status);

-- ============================================================================
-- 9. pop_topology_imports — operator YAML envanter import audit log.
--
-- Operatör R4-2 sonrası YAML inventory dosyasını (POP > router/AP/
-- mimosa/cpe ranges) import ettiğinde her import bir satır kazanır.
-- `payload_redacted` import edilen YAML'ın redacted kopyası
-- (credential profile referansları kalır, secret değer asla yer
-- almaz çünkü YAML hiç secret taşımaz). `applied_changes` özet
-- (kaç POP, kaç cihaz, kaç conflict). Her import re-runnable
-- ama idempotent değil — operatör yeni satır ister, önceki kayıt
-- audit için saklı kalır.
-- ============================================================================
CREATE TABLE IF NOT EXISTS pop_topology_imports (
  id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  imported_by         text NOT NULL DEFAULT 'system',
  source_filename     text NOT NULL DEFAULT '',
  source_checksum     text NOT NULL DEFAULT '',
  yaml_redacted       text NOT NULL DEFAULT '',
  pop_count           int  NOT NULL DEFAULT 0,
  router_count        int  NOT NULL DEFAULT 0,
  ap_count            int  NOT NULL DEFAULT 0,
  mimosa_link_count   int  NOT NULL DEFAULT 0,
  cpe_range_count     int  NOT NULL DEFAULT 0,
  conflict_count      int  NOT NULL DEFAULT 0,
  validation_errors   jsonb NOT NULL DEFAULT '[]'::jsonb,
  applied_at          timestamptz NOT NULL DEFAULT now(),
  status              text NOT NULL DEFAULT 'applied'
                      CHECK (status IN ('applied','rejected','partial'))
);

CREATE INDEX IF NOT EXISTS idx_pti_applied ON pop_topology_imports (applied_at DESC);

-- ============================================================================
-- 10. pop_groups — operator-defined or auto-resolved POP gruplaması.
-- ============================================================================
CREATE TABLE IF NOT EXISTS pop_groups (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  code            text NOT NULL UNIQUE CHECK (length(code) > 0),
  display_name    text NOT NULL DEFAULT '',
  source          text NOT NULL DEFAULT 'auto'
                  CHECK (source IN ('manual','dude_metadata','name_pattern','subnet','auto')),
  notes           text NOT NULL DEFAULT '',
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_pg_source ON pop_groups (source);

-- ============================================================================
-- 11. pop_device_membership — cihazın hangi POP'a üye olduğu + neden.
-- ============================================================================
CREATE TABLE IF NOT EXISTS pop_device_membership (
  pop_id          uuid NOT NULL REFERENCES pop_groups(id) ON DELETE CASCADE,
  device_id       uuid NOT NULL REFERENCES network_devices(id) ON DELETE CASCADE,
  resolution_rule text NOT NULL
                  CHECK (resolution_rule IN (
                    'manual','dude_metadata','name_pattern',
                    'subnet','unknown_pop'
                  )),
  reason          text NOT NULL DEFAULT '',
  resolved_at     timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (pop_id, device_id)
);

CREATE INDEX IF NOT EXISTS idx_pdm_device ON pop_device_membership (device_id);

-- ============================================================================
-- 12. weak_client_findings — bridge/customer CPE verimsizlik raporu.
-- ============================================================================
CREATE TABLE IF NOT EXISTS weak_client_findings (
  id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  device_id           uuid REFERENCES network_devices(id) ON DELETE CASCADE,
  ap_device_id        uuid REFERENCES network_devices(id) ON DELETE SET NULL,
  pop_id              uuid REFERENCES pop_groups(id) ON DELETE SET NULL,
  client_mac          text,
  signal_dbm          int,
  ccq_avg_pct         int,
  drop_count_24h      int,
  risk_score          int  NOT NULL DEFAULT 0
                      CHECK (risk_score BETWEEN 0 AND 100),
  reasons             text[] NOT NULL DEFAULT '{}',
  observed_at         timestamptz NOT NULL DEFAULT now(),
  resolved_at         timestamptz,
  notes               text NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_wcf_pop      ON weak_client_findings (pop_id);
CREATE INDEX IF NOT EXISTS idx_wcf_ap       ON weak_client_findings (ap_device_id);
CREATE INDEX IF NOT EXISTS idx_wcf_open     ON weak_client_findings (observed_at DESC)
  WHERE resolved_at IS NULL;

-- ============================================================================
-- 13. frequency_plan_runs — dry-run frekans planı çıktıları.
--
-- Mutation=false invariant'ı SQL constraint ile sertleştirilir.
-- ============================================================================
CREATE TABLE IF NOT EXISTS frequency_plan_runs (
  id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  pop_id              uuid REFERENCES pop_groups(id) ON DELETE SET NULL,
  triggered_by        text NOT NULL DEFAULT 'system',
  started_at          timestamptz NOT NULL DEFAULT now(),
  finished_at         timestamptz,
  status              text NOT NULL DEFAULT 'pending'
                      CHECK (status IN ('pending','succeeded','failed')),
  mutation            boolean NOT NULL DEFAULT false
                      CHECK (mutation = false),
  current_frequency_map jsonb NOT NULL DEFAULT '{}'::jsonb,
  recommended_changes   jsonb NOT NULL DEFAULT '[]'::jsonb,
  affected_devices      jsonb NOT NULL DEFAULT '[]'::jsonb,
  expected_benefit      text NOT NULL DEFAULT '',
  confidence_score      int  NOT NULL DEFAULT 0
                        CHECK (confidence_score BETWEEN 0 AND 100),
  reasons               text[] NOT NULL DEFAULT '{}',
  error_message         text,
  created_at            timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_fpr_pop      ON frequency_plan_runs (pop_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_fpr_status   ON frequency_plan_runs (status, started_at DESC);

-- ============================================================================
-- 14. network_devices.r4_classification — R4 sınıflandırma enum'u.
--
-- Phase 8 'category' kolonu (AP/BackhaulLink/Bridge/CPE/Router/
-- Switch/Unknown) korunur; R4 daha zengin bir enum getiriyor:
--
--   mikrotik_router      — operatör mapping veya probe vendor
--   mikrotik_ap          — wireless mode ap-bridge / probe doğruladı
--   mimosa_link          — Mimosa peer + RSSI/SNR + remote peer
--   bridge_customer_cpe  — wireless mode station + bridge port
--   switch               — wired only, multiple bridge ports, no wireless
--   unknown              — yetersiz kanıt
--   unreachable          — ICMP/TCP/SNMP hiçbiri cevap vermedi
--   credential_failed    — bağlandı, auth reddedildi
--   mapping_conflict     — operatör YAML'ı bir tip diyor, probe
--                          farklı tip kanıtlıyor → operatör çözer
--
-- Eski category kolonu Phase 8/R2/R3 kodu için olduğu gibi kalır;
-- R4 yeni kolonu okur/yazar. Dashboard her ikisini de gösterip
-- gerekirse "legacy vs r4" karşılaştırması yapabilir.
-- ============================================================================
ALTER TABLE network_devices
  ADD COLUMN IF NOT EXISTS r4_classification text,
  ADD COLUMN IF NOT EXISTS r4_confidence     int  NOT NULL DEFAULT 0
                                              CHECK (r4_confidence BETWEEN 0 AND 100),
  ADD COLUMN IF NOT EXISTS r4_classified_at  timestamptz,
  ADD COLUMN IF NOT EXISTS r4_reasons        text[] NOT NULL DEFAULT '{}',
  ADD COLUMN IF NOT EXISTS r4_evidence_sources text[] NOT NULL DEFAULT '{}',
  ADD COLUMN IF NOT EXISTS r4_last_probe_at  timestamptz,
  ADD COLUMN IF NOT EXISTS r4_probe_status   text;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.check_constraints
    WHERE constraint_name = 'chk_netdev_r4_classification'
  ) THEN
    ALTER TABLE network_devices
      ADD CONSTRAINT chk_netdev_r4_classification
      CHECK (r4_classification IS NULL OR r4_classification IN (
        'mikrotik_router','mikrotik_ap','mimosa_link',
        'bridge_customer_cpe','switch','unknown',
        'unreachable','credential_failed','mapping_conflict'
      ));
  END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_netdev_r4_class
  ON network_devices (r4_classification)
  WHERE r4_classification IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_netdev_r4_conflict
  ON network_devices (r4_last_probe_at DESC)
  WHERE r4_classification = 'mapping_conflict';

COMMIT;
