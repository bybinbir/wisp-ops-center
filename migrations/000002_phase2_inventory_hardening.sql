-- =================================================================
-- wisp-ops-center  ::  Faz 2 — envanter sertleştirmesi
--
-- İçerik:
--   * audit_logs için en azından "uygulama rolünden silme/güncelleme
--     yetkisinin alınması" kuralı yorum olarak eklendi (rol
--     deployment'ta yaratılır).
--   * Yeni alan ve indeksler: devices.tags / status / os_version,
--     credential_profiles.auth_type, port, secret_ciphertext.
--   * AP-to-Client test mühendisliği için 3 yeni tablo (yalnızca
--     veri modeli + güvenlik kısıtları; Faz 5'e kadar yürütme yok).
-- =================================================================

-- --- devices: envanter alanları ---------------------------------
ALTER TABLE devices
    ADD COLUMN IF NOT EXISTS os_version       TEXT,
    ADD COLUMN IF NOT EXISTS firmware_version TEXT,
    ADD COLUMN IF NOT EXISTS status           TEXT NOT NULL DEFAULT 'active',
    ADD COLUMN IF NOT EXISTS tags             TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS notes            TEXT,
    ADD COLUMN IF NOT EXISTS deleted_at       TIMESTAMPTZ;

ALTER TABLE devices
    DROP CONSTRAINT IF EXISTS devices_status_check;
ALTER TABLE devices
    ADD CONSTRAINT devices_status_check
    CHECK (status IN ('active','retired','maintenance','spare'));

CREATE INDEX IF NOT EXISTS idx_devices_status      ON devices(status) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_devices_tags_gin    ON devices USING GIN(tags);
CREATE INDEX IF NOT EXISTS idx_devices_name        ON devices(name);
CREATE INDEX IF NOT EXISTS idx_devices_deleted_at  ON devices(deleted_at);

-- --- towers/customers/links yardımcı indeksleri -----------------
CREATE INDEX IF NOT EXISTS idx_towers_name        ON towers(name);
CREATE INDEX IF NOT EXISTS idx_customers_name     ON customers(full_name);
CREATE INDEX IF NOT EXISTS idx_customers_status   ON customers(status);
CREATE INDEX IF NOT EXISTS idx_links_master       ON links(master_device_id);
CREATE INDEX IF NOT EXISTS idx_links_risk         ON links(risk);

-- --- credential_profiles: AES-GCM + tip alanları ----------------
ALTER TABLE credential_profiles
    ADD COLUMN IF NOT EXISTS auth_type TEXT,
    ADD COLUMN IF NOT EXISTS port      INTEGER;

UPDATE credential_profiles
    SET auth_type = 'routeros_api_ssl'
    WHERE auth_type IS NULL;

ALTER TABLE credential_profiles
    ALTER COLUMN auth_type SET NOT NULL;

ALTER TABLE credential_profiles
    DROP CONSTRAINT IF EXISTS credential_profiles_auth_type_check;
ALTER TABLE credential_profiles
    ADD CONSTRAINT credential_profiles_auth_type_check
    CHECK (auth_type IN ('routeros_api_ssl','ssh','snmp_v2','snmp_v3','mimosa_snmp','vendor_api'));

CREATE INDEX IF NOT EXISTS idx_cred_profiles_name      ON credential_profiles(name);
CREATE INDEX IF NOT EXISTS idx_cred_profiles_auth_type ON credential_profiles(auth_type);

-- --- audit_logs sertleştirmesi ----------------------------------
-- audit zincirini koru: app rolünün UPDATE/DELETE yetkisi olmamalı.
-- Rol "wispops_app" deployment'ta yaratılır; bu migration kayıt
-- altına alır. Eksikse no-op gibi davranır.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'wispops_app') THEN
    EXECUTE 'REVOKE UPDATE, DELETE ON audit_logs FROM wispops_app';
  END IF;
END$$;

-- audit_logs için ek indeksler
CREATE INDEX IF NOT EXISTS idx_audit_logs_actor_at ON audit_logs(actor, at DESC);

-- =================================================================
-- AP-to-Client Test Engine  (Faz 2: yalnızca veri modeli)
-- =================================================================

CREATE TABLE IF NOT EXISTS ap_client_test_profiles (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name                     TEXT NOT NULL UNIQUE,
    test_type                TEXT NOT NULL
                             CHECK (test_type IN ('ping_latency','packet_loss','jitter',
                                                  'traceroute','limited_throughput','mikrotik_bandwidth_test')),
    risk_level               TEXT NOT NULL
                             CHECK (risk_level IN ('low','medium','high')),
    max_duration_seconds     INTEGER NOT NULL CHECK (max_duration_seconds > 0 AND max_duration_seconds <= 600),
    max_rate_mbps            NUMERIC(7,2),
    requires_manual_approval BOOLEAN NOT NULL DEFAULT TRUE,
    enabled                  BOOLEAN NOT NULL DEFAULT FALSE,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Faz 2 kuralı: yüksek riskli testler manuel onaysız aktif olamaz.
ALTER TABLE ap_client_test_profiles
    DROP CONSTRAINT IF EXISTS ap_client_high_risk_requires_approval;
ALTER TABLE ap_client_test_profiles
    ADD CONSTRAINT ap_client_high_risk_requires_approval
    CHECK (risk_level <> 'high' OR requires_manual_approval = TRUE);

-- Faz 2 sınırı: testler bu fazda çalıştırılmaz; profil enabled=true
-- olsa bile worker veri çekimini reddedecek (kod tarafında).

CREATE TABLE IF NOT EXISTS ap_client_test_runs (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    profile_id          UUID NOT NULL REFERENCES ap_client_test_profiles(id) ON DELETE RESTRICT,
    ap_device_id        UUID NOT NULL REFERENCES devices(id) ON DELETE RESTRICT,
    scheduled_check_id  UUID REFERENCES scheduled_checks(id) ON DELETE SET NULL,
    status              TEXT NOT NULL DEFAULT 'planned'
                        CHECK (status IN ('planned','running','done','failed','blocked','cancelled')),
    started_at          TIMESTAMPTZ,
    finished_at         TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_ap_runs_profile ON ap_client_test_runs(profile_id);
CREATE INDEX IF NOT EXISTS idx_ap_runs_ap      ON ap_client_test_runs(ap_device_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_ap_runs_status  ON ap_client_test_runs(status);

CREATE TABLE IF NOT EXISTS ap_client_test_results (
    id                  BIGSERIAL PRIMARY KEY,
    run_id              UUID NOT NULL REFERENCES ap_client_test_runs(id) ON DELETE CASCADE,
    customer_id         UUID REFERENCES customers(id),
    customer_device_id  UUID REFERENCES customer_devices(id),
    target_device_id    UUID REFERENCES devices(id),
    latency_ms          NUMERIC(7,2),
    packet_loss_percent NUMERIC(5,2),
    jitter_ms           NUMERIC(7,2),
    throughput_mbps     NUMERIC(7,2),
    diagnosis           TEXT,
    risk_level          TEXT CHECK (risk_level IN ('low','medium','high')),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_ap_results_run      ON ap_client_test_results(run_id);
CREATE INDEX IF NOT EXISTS idx_ap_results_customer ON ap_client_test_results(customer_id, created_at DESC);

-- =================================================================
-- Faz 2 sonu — sonraki migration 000003_*.sql olarak gelecek.
-- =================================================================
