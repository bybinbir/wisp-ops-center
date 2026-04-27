-- =================================================================
-- wisp-ops-center  ::  Faz 3 — MikroTik salt-okuma kalıcılığı
--
-- Bu migration MikroTik read-only entegrasyonunun ürettiği veriyi
-- saklamak için yeni tablolar ve indeksler ekler. Mevcut
-- telemetry_snapshots/wireless_metrics tablolarını da bozmadan
-- kullanır; ek alanlar device_poll_results üzerinde yaşar.
-- =================================================================

-- --- device_poll_results: her probe/poll çalıştırmasının özeti ----
CREATE TABLE IF NOT EXISTS device_poll_results (
    id                BIGSERIAL PRIMARY KEY,
    device_id         UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    vendor            TEXT NOT NULL,
    operation         TEXT NOT NULL CHECK (operation IN ('probe','poll')),
    transport         TEXT NOT NULL CHECK (transport IN ('api-ssl','ssh','snmp','vendor-api')),
    status            TEXT NOT NULL CHECK (status IN ('success','failed','blocked','partial')),
    started_at        TIMESTAMPTZ NOT NULL,
    finished_at       TIMESTAMPTZ NOT NULL,
    duration_ms       INTEGER NOT NULL,
    error_code        TEXT,
    error_message     TEXT,
    payload_hash      TEXT,
    summary           JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_dpr_device_started ON device_poll_results(device_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_dpr_status         ON device_poll_results(status);

-- --- mikrotik_wireless_clients: registration-table snapshot ------
CREATE TABLE IF NOT EXISTS mikrotik_wireless_clients (
    id              BIGSERIAL PRIMARY KEY,
    device_id       UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    poll_result_id  BIGINT REFERENCES device_poll_results(id) ON DELETE CASCADE,
    interface_name  TEXT NOT NULL,
    mac             MACADDR,
    ip              INET,
    ssid            TEXT,
    uptime_sec      BIGINT,
    signal_dbm      NUMERIC(5,2),
    snr_db          NUMERIC(5,2),
    tx_rate_mbps    NUMERIC(7,2),
    rx_rate_mbps    NUMERIC(7,2),
    ccq             NUMERIC(5,2),
    collected_at    TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_mwc_device_time ON mikrotik_wireless_clients(device_id, collected_at DESC);
CREATE INDEX IF NOT EXISTS idx_mwc_mac         ON mikrotik_wireless_clients(mac);

-- --- mikrotik_wireless_interfaces: per-radio snapshot ------------
CREATE TABLE IF NOT EXISTS mikrotik_wireless_interfaces (
    id                BIGSERIAL PRIMARY KEY,
    device_id         UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    poll_result_id    BIGINT REFERENCES device_poll_results(id) ON DELETE CASCADE,
    name              TEXT NOT NULL,
    ssid              TEXT,
    mode              TEXT,
    band              TEXT,
    frequency_mhz     INTEGER,
    channel_width_mhz INTEGER,
    tx_power_dbm      NUMERIC(5,2),
    noise_floor_dbm   NUMERIC(5,2),
    disabled          BOOLEAN NOT NULL DEFAULT FALSE,
    running           BOOLEAN NOT NULL DEFAULT FALSE,
    collected_at      TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_mwi_device_time ON mikrotik_wireless_interfaces(device_id, collected_at DESC);

-- --- interface_metrics: vendor-bağımsız arayüz sayaçları ---------
CREATE TABLE IF NOT EXISTS interface_metrics (
    id              BIGSERIAL PRIMARY KEY,
    device_id       UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    poll_result_id  BIGINT REFERENCES device_poll_results(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    type            TEXT,
    running         BOOLEAN NOT NULL DEFAULT FALSE,
    disabled        BOOLEAN NOT NULL DEFAULT FALSE,
    mtu             INTEGER,
    mac             MACADDR,
    rx_bytes        BIGINT,
    tx_bytes        BIGINT,
    rx_packets      BIGINT,
    tx_packets      BIGINT,
    rx_errors       BIGINT,
    tx_errors       BIGINT,
    link_downs      BIGINT,
    collected_at    TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_ifm_device_time ON interface_metrics(device_id, collected_at DESC);

-- --- devices: last_poll bilgi kolonları (UI için) ----------------
ALTER TABLE devices
    ADD COLUMN IF NOT EXISTS last_poll_at      TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_poll_status  TEXT,
    ADD COLUMN IF NOT EXISTS last_poll_error   TEXT;

CREATE INDEX IF NOT EXISTS idx_devices_last_poll_status ON devices(last_poll_status) WHERE deleted_at IS NULL;

-- =================================================================
-- Faz 3 sonu — Faz 4'te Mimosa için 000004_*.sql gelecek.
-- =================================================================
