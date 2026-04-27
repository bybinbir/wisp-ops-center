-- 000006_customer_signal_scoring.sql
-- Faz 6: Müşteri sinyal skorlama, AP/Tower skorları, eşik tablosu
-- ve iş emri adayları.
--
-- Bu migration idempotent + transactional çalıştırılır
-- (apps/api/internal/database/migrate.go). Hiçbir DROP yapılmaz.

BEGIN;

-- ============================================================================
-- 1. scoring_thresholds: skor eşiklerinin runtime override tablosu.
-- ============================================================================
CREATE TABLE IF NOT EXISTS scoring_thresholds (
  key         text PRIMARY KEY,
  value       double precision NOT NULL,
  description text NOT NULL DEFAULT '',
  updated_at  timestamptz NOT NULL DEFAULT now(),
  updated_by  text
);

-- Seed varsayılanlar (override edilmezse Engine.DefaultThresholds() ile aynı)
INSERT INTO scoring_thresholds (key, value, description) VALUES
  ('rssi_critical_dbm',                      -80, 'RSSI kritik (dBm)'),
  ('rssi_warning_dbm',                       -70, 'RSSI uyarı (dBm)'),
  ('snr_critical_db',                         15, 'SNR kritik (dB)'),
  ('snr_warning_db',                          25, 'SNR uyarı (dB)'),
  ('ccq_critical_percent',                    50, 'CCQ kritik (%)'),
  ('ccq_warning_percent',                     75, 'CCQ uyarı (%)'),
  ('packet_loss_critical_percent',             5, 'Paket kaybı kritik (%)'),
  ('packet_loss_warning_percent',              2, 'Paket kaybı uyarı (%)'),
  ('latency_critical_ms',                    100, 'Gecikme kritik (ms)'),
  ('latency_warning_ms',                      50, 'Gecikme uyarı (ms)'),
  ('jitter_critical_ms',                      30, 'Jitter kritik (ms)'),
  ('jitter_warning_ms',                       15, 'Jitter uyarı (ms)'),
  ('stale_data_minutes',                      60, 'Veri tazelik eşiği (dk)'),
  ('ap_degradation_customer_ratio_warning',  0.25, 'AP peer-group degradation uyarı oranı'),
  ('ap_degradation_customer_ratio_critical', 0.40, 'AP peer-group degradation kritik oranı'),
  ('severity_healthy_at',                     80, 'score >= → healthy'),
  ('severity_warning_at',                     50, 'score >= → warning')
ON CONFLICT (key) DO NOTHING;

-- ============================================================================
-- 2. customer_signal_scores: hesaplanan skor + tanı + aksiyon.
--    Her hesaplama yeni bir satır üretir (geçmiş için). En güncel satır,
--    son calculated_at değerine sahip olandır.
-- ============================================================================
CREATE TABLE IF NOT EXISTS customer_signal_scores (
  id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  customer_id          uuid NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
  ap_device_id         uuid REFERENCES devices(id) ON DELETE SET NULL,
  tower_id             uuid REFERENCES towers(id) ON DELETE SET NULL,
  score                int  NOT NULL CHECK (score BETWEEN 0 AND 100),
  severity             text NOT NULL CHECK (severity IN ('healthy','warning','critical','unknown')),
  diagnosis            text NOT NULL,
  recommended_action   text NOT NULL,
  reasons              jsonb NOT NULL DEFAULT '[]',
  contributing_metrics jsonb NOT NULL DEFAULT '{}',
  rssi_dbm             double precision,
  snr_db               double precision,
  ccq                  double precision,
  packet_loss_pct      double precision,
  avg_latency_ms       double precision,
  jitter_ms            double precision,
  signal_trend_7d      double precision,
  is_stale             boolean NOT NULL DEFAULT false,
  calculated_at        timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_css_customer_calc
  ON customer_signal_scores (customer_id, calculated_at DESC);

CREATE INDEX IF NOT EXISTS idx_css_severity_calc
  ON customer_signal_scores (severity, calculated_at DESC);

CREATE INDEX IF NOT EXISTS idx_css_ap_calc
  ON customer_signal_scores (ap_device_id, calculated_at DESC)
  WHERE ap_device_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_css_tower_calc
  ON customer_signal_scores (tower_id, calculated_at DESC)
  WHERE tower_id IS NOT NULL;

-- ============================================================================
-- 3. ap_health_scores: AP cihaz seviyesinde özet skor.
-- ============================================================================
CREATE TABLE IF NOT EXISTS ap_health_scores (
  id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  ap_device_id            uuid NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
  ap_score                int  NOT NULL CHECK (ap_score BETWEEN 0 AND 100),
  severity                text NOT NULL CHECK (severity IN ('healthy','warning','critical','unknown')),
  total_customers         int  NOT NULL DEFAULT 0,
  critical_customers      int  NOT NULL DEFAULT 0,
  warning_customers       int  NOT NULL DEFAULT 0,
  healthy_customers       int  NOT NULL DEFAULT 0,
  degradation_ratio       double precision NOT NULL DEFAULT 0,
  is_ap_wide_interference boolean NOT NULL DEFAULT false,
  reasons                 jsonb NOT NULL DEFAULT '[]',
  calculated_at           timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_aps_device_calc
  ON ap_health_scores (ap_device_id, calculated_at DESC);

CREATE INDEX IF NOT EXISTS idx_aps_severity_calc
  ON ap_health_scores (severity, calculated_at DESC);

-- ============================================================================
-- 4. tower_risk_scores: kule operasyonel risk skoru.
-- ============================================================================
CREATE TABLE IF NOT EXISTS tower_risk_scores (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tower_id      uuid NOT NULL REFERENCES towers(id) ON DELETE CASCADE,
  risk_score    int  NOT NULL CHECK (risk_score BETWEEN 0 AND 100),
  severity      text NOT NULL CHECK (severity IN ('healthy','warning','critical','unknown')),
  reasons       jsonb NOT NULL DEFAULT '[]',
  calculated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_trs_tower_calc
  ON tower_risk_scores (tower_id, calculated_at DESC);

-- ============================================================================
-- 5. work_order_candidates: skor motorunun ürettiği iş emri adayları.
--    İş emri oluşturulduktan sonra promoted_work_order_id doldurulur.
-- ============================================================================
CREATE TABLE IF NOT EXISTS work_order_candidates (
  id                       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  customer_id              uuid REFERENCES customers(id) ON DELETE SET NULL,
  ap_device_id             uuid REFERENCES devices(id) ON DELETE SET NULL,
  tower_id                 uuid REFERENCES towers(id) ON DELETE SET NULL,
  source_score_id          uuid REFERENCES customer_signal_scores(id) ON DELETE SET NULL,
  diagnosis                text NOT NULL,
  recommended_action       text NOT NULL,
  severity                 text NOT NULL CHECK (severity IN ('healthy','warning','critical','unknown')),
  reasons                  jsonb NOT NULL DEFAULT '[]',
  status                   text NOT NULL DEFAULT 'open'
                           CHECK (status IN ('open','dismissed','promoted')),
  notes                    text,
  promoted_work_order_id   uuid,
  created_at               timestamptz NOT NULL DEFAULT now(),
  updated_at               timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_woc_status_created
  ON work_order_candidates (status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_woc_customer_created
  ON work_order_candidates (customer_id, created_at DESC)
  WHERE customer_id IS NOT NULL;

-- ============================================================================
-- 6. customers: skor erişimi için yardımcı sütunlar (opsiyonel cache).
-- ============================================================================
ALTER TABLE customers
  ADD COLUMN IF NOT EXISTS last_signal_score      int,
  ADD COLUMN IF NOT EXISTS last_signal_severity   text,
  ADD COLUMN IF NOT EXISTS last_signal_diagnosis  text,
  ADD COLUMN IF NOT EXISTS last_signal_at         timestamptz;

CREATE INDEX IF NOT EXISTS idx_customers_severity
  ON customers (last_signal_severity)
  WHERE last_signal_severity IS NOT NULL;

-- ============================================================================
-- 7. Audit / privilege: wispops_app rolü için skor tablolarına UPDATE/DELETE
--    izinlerini reddet — sadece scoring-engine süper-rolü güncelleyebilsin
--    yapılmadı çünkü uygulamamız tek roldur. Bu yorum bilgi amaçlıdır.
-- ============================================================================

COMMIT;
