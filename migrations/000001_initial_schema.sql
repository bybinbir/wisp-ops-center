-- ============================================================
-- wisp-ops-center  ::  ilk şema
-- Faz 1 — operasyon merkezli WISP sahipliği için temel tablolar
--
-- Önemli:
--   * Hiçbir tablo, ham parolayı düz metin olarak saklamaz.
--     credential_profiles.secret_ciphertext alanı dış
--     anahtar yönetim sistemiyle çözülmek üzere tasarlanmıştır.
--   * Telemetri tabloları zaman serisine uygun indekslenir.
--   * Tüm operasyonel aksiyonlar audit_logs tablosuna düşer.
-- ============================================================

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ----------------------------------------------------------------
-- Saha / kule / cihaz hiyerarşisi
-- ----------------------------------------------------------------

CREATE TABLE sites (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT NOT NULL,
    code         TEXT UNIQUE,
    region       TEXT,
    address      TEXT,
    latitude     DOUBLE PRECISION,
    longitude    DOUBLE PRECISION,
    notes        TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE towers (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    site_id      UUID REFERENCES sites(id) ON DELETE SET NULL,
    name         TEXT NOT NULL,
    code         TEXT,
    height_m     NUMERIC(5,1),
    latitude     DOUBLE PRECISION,
    longitude    DOUBLE PRECISION,
    notes        TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (site_id, name)
);

CREATE TABLE devices (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    site_id         UUID REFERENCES sites(id)   ON DELETE SET NULL,
    tower_id        UUID REFERENCES towers(id)  ON DELETE SET NULL,
    name            TEXT NOT NULL,
    vendor          TEXT NOT NULL CHECK (vendor IN ('mikrotik','mimosa','unknown')),
    role            TEXT NOT NULL CHECK (role IN ('ap','cpe','ptp_master','ptp_slave','router','switch')),
    model           TEXT,
    firmware        TEXT,
    routeros        TEXT,
    ip              INET,
    mac             MACADDR,
    serial_number   TEXT,
    last_seen_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (vendor, ip)
);

CREATE INDEX idx_devices_tower      ON devices(tower_id);
CREATE INDEX idx_devices_site       ON devices(site_id);
CREATE INDEX idx_devices_vendor_role ON devices(vendor, role);

CREATE TABLE device_capabilities (
    device_id                  UUID PRIMARY KEY REFERENCES devices(id) ON DELETE CASCADE,
    can_read_health            BOOLEAN NOT NULL DEFAULT FALSE,
    can_read_wireless_metrics  BOOLEAN NOT NULL DEFAULT FALSE,
    can_read_clients           BOOLEAN NOT NULL DEFAULT FALSE,
    can_read_frequency         BOOLEAN NOT NULL DEFAULT FALSE,
    can_run_scan               BOOLEAN NOT NULL DEFAULT FALSE,
    can_recommend_frequency    BOOLEAN NOT NULL DEFAULT FALSE,
    can_backup_config          BOOLEAN NOT NULL DEFAULT FALSE,
    can_apply_frequency        BOOLEAN NOT NULL DEFAULT FALSE,
    can_rollback               BOOLEAN NOT NULL DEFAULT FALSE,
    requires_manual_apply      BOOLEAN NOT NULL DEFAULT TRUE,
    supports_snmp              BOOLEAN NOT NULL DEFAULT FALSE,
    supports_routeros_api      BOOLEAN NOT NULL DEFAULT FALSE,
    supports_ssh               BOOLEAN NOT NULL DEFAULT FALSE,
    supports_vendor_api        BOOLEAN NOT NULL DEFAULT FALSE,
    last_verified_at           TIMESTAMPTZ,
    notes                      TEXT
);

-- ----------------------------------------------------------------
-- PTP / PTMP hatlar
-- ----------------------------------------------------------------

CREATE TABLE links (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name               TEXT NOT NULL,
    topology           TEXT NOT NULL CHECK (topology IN ('ptp','ptmp')),
    master_device_id   UUID NOT NULL REFERENCES devices(id) ON DELETE RESTRICT,
    frequency_mhz      INTEGER,
    channel_width_mhz  INTEGER,
    last_signal_dbm    NUMERIC(5,1),
    last_snr_db        NUMERIC(5,1),
    last_capacity_mbps NUMERIC(7,1),
    risk               TEXT NOT NULL DEFAULT 'healthy' CHECK (risk IN ('healthy','watch','warning','critical')),
    last_checked_at    TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE link_slaves (
    link_id    UUID NOT NULL REFERENCES links(id)   ON DELETE CASCADE,
    device_id  UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    PRIMARY KEY (link_id, device_id)
);

-- ----------------------------------------------------------------
-- Müşteriler
-- ----------------------------------------------------------------

CREATE TABLE customers (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    external_code   TEXT UNIQUE,
    full_name       TEXT NOT NULL,
    phone           TEXT,
    address         TEXT,
    site_id         UUID REFERENCES sites(id),
    tower_id        UUID REFERENCES towers(id),
    contracted_mbps INTEGER,
    status          TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','suspended','cancelled')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE customer_devices (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id     UUID NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    device_id       UUID REFERENCES devices(id),
    role            TEXT NOT NULL CHECK (role IN ('ap','cpe')),
    UNIQUE (customer_id, device_id, role)
);

-- ----------------------------------------------------------------
-- Kimlik bilgisi profilleri (vault dışında saklanır)
-- ----------------------------------------------------------------

CREATE TABLE credential_profiles (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name                TEXT NOT NULL UNIQUE,
    vendor              TEXT,
    username            TEXT,
    secret_ciphertext   BYTEA,             -- KMS/AES ile şifreli
    secret_key_id       TEXT,              -- KMS anahtar referansı
    snmp_community_ciphertext BYTEA,
    notes               TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    rotated_at          TIMESTAMPTZ,
    last_used_at        TIMESTAMPTZ
);

-- Profil ↔ cihaz eşlemesi (bir profil birden fazla cihaza atanabilir)
CREATE TABLE device_credentials (
    device_id      UUID NOT NULL REFERENCES devices(id)             ON DELETE CASCADE,
    profile_id     UUID NOT NULL REFERENCES credential_profiles(id) ON DELETE RESTRICT,
    transport      TEXT NOT NULL CHECK (transport IN ('api-ssl','ssh','snmp','vendor-api')),
    PRIMARY KEY (device_id, transport)
);

-- ----------------------------------------------------------------
-- Telemetri (zaman serisi)
-- ----------------------------------------------------------------

CREATE TABLE telemetry_snapshots (
    id              BIGSERIAL PRIMARY KEY,
    device_id       UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    collected_at    TIMESTAMPTZ NOT NULL,
    online          BOOLEAN NOT NULL,
    uptime_sec      BIGINT,
    cpu_percent     NUMERIC(5,2),
    mem_percent     NUMERIC(5,2),
    temp_c          NUMERIC(5,2)
);
CREATE INDEX idx_telemetry_device_time ON telemetry_snapshots(device_id, collected_at DESC);

CREATE TABLE wireless_metrics (
    id                 BIGSERIAL PRIMARY KEY,
    device_id          UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    interface          TEXT NOT NULL,
    collected_at       TIMESTAMPTZ NOT NULL,
    frequency_mhz      INTEGER,
    channel_width_mhz  INTEGER,
    tx_power_dbm       NUMERIC(5,2),
    noise_floor_dbm    NUMERIC(5,2),
    rssi_dbm           NUMERIC(5,2),
    snr_db             NUMERIC(5,2),
    ccq                NUMERIC(5,2),
    tx_rate_mbps       NUMERIC(7,2),
    rx_rate_mbps       NUMERIC(7,2),
    tx_bytes           BIGINT,
    rx_bytes           BIGINT
);
CREATE INDEX idx_wireless_device_time ON wireless_metrics(device_id, collected_at DESC);

CREATE TABLE health_scores (
    id              BIGSERIAL PRIMARY KEY,
    customer_id     UUID REFERENCES customers(id) ON DELETE CASCADE,
    device_id       UUID REFERENCES devices(id)   ON DELETE CASCADE,
    link_id         UUID REFERENCES links(id)     ON DELETE CASCADE,
    score           SMALLINT NOT NULL CHECK (score BETWEEN 0 AND 100),
    diagnosis       TEXT NOT NULL,
    recommended_action TEXT,
    reasons         JSONB NOT NULL DEFAULT '[]'::jsonb,
    computed_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_health_customer_time ON health_scores(customer_id, computed_at DESC);
CREATE INDEX idx_health_device_time   ON health_scores(device_id,   computed_at DESC);
CREATE INDEX idx_health_link_time     ON health_scores(link_id,     computed_at DESC);

-- ----------------------------------------------------------------
-- Zamanlanmış kontroller ve iş yürütme geçmişi
-- ----------------------------------------------------------------

CREATE TABLE scheduled_checks (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT NOT NULL,
    job_type     TEXT NOT NULL,
    cadence      TEXT NOT NULL CHECK (cadence IN ('once','daily','weekly','monthly','maintenance_window')),
    mode         TEXT NOT NULL CHECK (mode IN ('report_only','recommend_only','manual_approval','controlled_apply')),
    scope        JSONB NOT NULL DEFAULT '{}'::jsonb,
    next_run_at  TIMESTAMPTZ,
    last_run_at  TIMESTAMPTZ,
    enabled      BOOLEAN NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Faz 1 emniyeti: controlled_apply mod kullanılırsa veritabanı
-- seviyesinde reddedilir.
ALTER TABLE scheduled_checks
    ADD CONSTRAINT phase1_no_apply
    CHECK (mode <> 'controlled_apply');

CREATE TABLE job_runs (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    check_id     UUID REFERENCES scheduled_checks(id) ON DELETE SET NULL,
    job_type     TEXT NOT NULL,
    started_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at  TIMESTAMPTZ,
    status       TEXT NOT NULL CHECK (status IN ('pending','running','success','failed','blocked')),
    error_text   TEXT,
    summary      JSONB NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX idx_job_runs_check_started ON job_runs(check_id, started_at DESC);
CREATE INDEX idx_job_runs_status        ON job_runs(status);

-- ----------------------------------------------------------------
-- Frekans önerileri
-- ----------------------------------------------------------------

CREATE TABLE frequency_recommendations (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    device_id                UUID REFERENCES devices(id),
    link_id                  UUID REFERENCES links(id),
    current_frequency_mhz    INTEGER,
    recommended_frequency_mhz INTEGER,
    channel_width_mhz        INTEGER,
    risk                     TEXT NOT NULL CHECK (risk IN ('low','medium','high')),
    affected_customer_count  INTEGER NOT NULL DEFAULT 0,
    expected_improvement     TEXT,
    reasons                  JSONB NOT NULL DEFAULT '[]'::jsonb,
    status                   TEXT NOT NULL DEFAULT 'draft'
                             CHECK (status IN ('draft','reviewed','approved','applied','rolled_back','dismissed')),
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ----------------------------------------------------------------
-- Raporlar ve iş emirleri
-- ----------------------------------------------------------------

CREATE TABLE reports (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kind         TEXT NOT NULL CHECK (kind IN ('daily','weekly','ad_hoc')),
    period_start TIMESTAMPTZ NOT NULL,
    period_end   TIMESTAMPTZ NOT NULL,
    format       TEXT NOT NULL CHECK (format IN ('json','html','pdf')),
    summary      JSONB NOT NULL DEFAULT '{}'::jsonb,
    storage_url  TEXT,
    generated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE work_orders (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title        TEXT NOT NULL,
    description  TEXT,
    customer_id  UUID REFERENCES customers(id),
    device_id    UUID REFERENCES devices(id),
    link_id      UUID REFERENCES links(id),
    priority     TEXT NOT NULL DEFAULT 'normal' CHECK (priority IN ('low','normal','high','critical')),
    status       TEXT NOT NULL DEFAULT 'open'
                 CHECK (status IN ('open','assigned','in_progress','done','cancelled')),
    assignee     TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ----------------------------------------------------------------
-- Audit
-- ----------------------------------------------------------------

CREATE TABLE audit_logs (
    id         BIGSERIAL PRIMARY KEY,
    at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    actor      TEXT NOT NULL,
    action     TEXT NOT NULL,
    subject    TEXT,
    outcome    TEXT NOT NULL CHECK (outcome IN ('success','failure','blocked')),
    reason     TEXT,
    metadata   JSONB NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX idx_audit_logs_at      ON audit_logs(at DESC);
CREATE INDEX idx_audit_logs_action  ON audit_logs(action);
CREATE INDEX idx_audit_logs_subject ON audit_logs(subject);

-- ============================================================
-- Faz 1 sonu — sonraki migration'lar 000002_*.sql olarak gelecek.
-- ============================================================
