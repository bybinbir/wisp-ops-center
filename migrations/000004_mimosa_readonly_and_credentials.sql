-- =================================================================
-- wisp-ops-center  ::  Faz 4
--   * Mimosa salt-okuma kalıcılığı
--   * Credential profile SNMPv3 alanları
--   * RouterOS API TLS verify ve SSH host key sertleştirme
--   * device_credentials priority/purpose alanları
-- =================================================================

-- --- credential_profiles: SNMPv3 + transport hardening alanları ---
ALTER TABLE credential_profiles
    ADD COLUMN IF NOT EXISTS snmpv3_username                  TEXT,
    ADD COLUMN IF NOT EXISTS snmpv3_security_level            TEXT,
    ADD COLUMN IF NOT EXISTS snmpv3_auth_protocol             TEXT,
    ADD COLUMN IF NOT EXISTS snmpv3_auth_secret_ciphertext    BYTEA,
    ADD COLUMN IF NOT EXISTS snmpv3_priv_protocol             TEXT,
    ADD COLUMN IF NOT EXISTS snmpv3_priv_secret_ciphertext    BYTEA,
    ADD COLUMN IF NOT EXISTS snmpv3_secret_key_id             TEXT,
    -- RouterOS API-SSL hardening
    ADD COLUMN IF NOT EXISTS verify_tls                       BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS ca_certificate_pem               TEXT,
    ADD COLUMN IF NOT EXISTS server_name_override             TEXT,
    -- SSH host key
    ADD COLUMN IF NOT EXISTS ssh_host_key_policy              TEXT
        DEFAULT 'insecure_ignore',
    ADD COLUMN IF NOT EXISTS ssh_host_key_fingerprint         TEXT;

ALTER TABLE credential_profiles
    DROP CONSTRAINT IF EXISTS credential_profiles_snmpv3_security_level_check;
ALTER TABLE credential_profiles
    ADD CONSTRAINT credential_profiles_snmpv3_security_level_check
    CHECK (snmpv3_security_level IS NULL
           OR snmpv3_security_level IN ('noAuthNoPriv','authNoPriv','authPriv'));

ALTER TABLE credential_profiles
    DROP CONSTRAINT IF EXISTS credential_profiles_snmpv3_auth_protocol_check;
ALTER TABLE credential_profiles
    ADD CONSTRAINT credential_profiles_snmpv3_auth_protocol_check
    CHECK (snmpv3_auth_protocol IS NULL
           OR snmpv3_auth_protocol IN ('MD5','SHA','SHA256'));

ALTER TABLE credential_profiles
    DROP CONSTRAINT IF EXISTS credential_profiles_snmpv3_priv_protocol_check;
ALTER TABLE credential_profiles
    ADD CONSTRAINT credential_profiles_snmpv3_priv_protocol_check
    CHECK (snmpv3_priv_protocol IS NULL
           OR snmpv3_priv_protocol IN ('DES','AES','AES192','AES256'));

ALTER TABLE credential_profiles
    DROP CONSTRAINT IF EXISTS credential_profiles_ssh_host_key_policy_check;
ALTER TABLE credential_profiles
    ADD CONSTRAINT credential_profiles_ssh_host_key_policy_check
    CHECK (ssh_host_key_policy IN ('insecure_ignore','trust_on_first_use','pinned'));

-- --- device_credentials: priority + purpose + enabled --------------
ALTER TABLE device_credentials
    ADD COLUMN IF NOT EXISTS purpose   TEXT NOT NULL DEFAULT 'primary',
    ADD COLUMN IF NOT EXISTS priority  INTEGER NOT NULL DEFAULT 100,
    ADD COLUMN IF NOT EXISTS enabled   BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now();

ALTER TABLE device_credentials
    DROP CONSTRAINT IF EXISTS device_credentials_purpose_check;
ALTER TABLE device_credentials
    ADD CONSTRAINT device_credentials_purpose_check
    CHECK (purpose IN ('primary','api','ssh','snmp','fallback'));

CREATE INDEX IF NOT EXISTS idx_dev_creds_device_priority
    ON device_credentials(device_id, priority);

-- --- mimosa_wireless_interfaces / clients / links ------------------
CREATE TABLE IF NOT EXISTS mimosa_wireless_interfaces (
    id                BIGSERIAL PRIMARY KEY,
    device_id         UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    poll_result_id    BIGINT REFERENCES device_poll_results(id) ON DELETE CASCADE,
    name              TEXT NOT NULL,
    frequency_mhz     INTEGER,
    channel_width_mhz INTEGER,
    tx_power_dbm      NUMERIC(5,2),
    noise_floor_dbm   NUMERIC(5,2),
    collected_at      TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_mmi_device_time ON mimosa_wireless_interfaces(device_id, collected_at DESC);

CREATE TABLE IF NOT EXISTS mimosa_wireless_clients (
    id              BIGSERIAL PRIMARY KEY,
    device_id       UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    poll_result_id  BIGINT REFERENCES device_poll_results(id) ON DELETE CASCADE,
    mac             MACADDR,
    ip              INET,
    hostname        TEXT,
    signal_dbm      NUMERIC(5,2),
    snr_db          NUMERIC(5,2),
    tx_rate_mbps    NUMERIC(7,2),
    rx_rate_mbps    NUMERIC(7,2),
    uptime_sec      BIGINT,
    collected_at    TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_mmc_device_time ON mimosa_wireless_clients(device_id, collected_at DESC);

CREATE TABLE IF NOT EXISTS mimosa_links (
    id                BIGSERIAL PRIMARY KEY,
    device_id         UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    poll_result_id    BIGINT REFERENCES device_poll_results(id) ON DELETE CASCADE,
    name              TEXT NOT NULL,
    peer_ip           INET,
    signal_dbm        NUMERIC(5,2),
    snr_db            NUMERIC(5,2),
    capacity_mbps     NUMERIC(7,2),
    uptime_sec        BIGINT,
    station_count     INTEGER,
    collected_at      TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_mml_device_time ON mimosa_links(device_id, collected_at DESC);

-- --- device_poll_results: vendor_mib_status ek alanı --------------
ALTER TABLE device_poll_results
    ADD COLUMN IF NOT EXISTS vendor_mib_status TEXT;
