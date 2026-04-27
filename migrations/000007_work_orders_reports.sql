-- 000007_work_orders_reports.sql
-- Faz 7: Gerçek iş emirleri, iş emri olayları, rapor snapshot'ları,
-- aday cooldown alanları ve scheduler için executive summary job genişletmesi.
--
-- Bu migration idempotent + transactional çalıştırılır
-- (internal/database/migrations.go). Hiçbir DROP yapılmaz.

BEGIN;

-- ============================================================================
-- 1. work_orders: Gerçek iş emri tablosu. Faz 6'daki work_order_candidates
--    "promoted" duruma geçtiğinde buraya bir satır yazılır.
-- ============================================================================
CREATE TABLE IF NOT EXISTS work_orders (
  id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  customer_id           uuid REFERENCES customers(id) ON DELETE SET NULL,
  ap_device_id          uuid REFERENCES devices(id)   ON DELETE SET NULL,
  tower_id              uuid REFERENCES towers(id)    ON DELETE SET NULL,
  source_candidate_id   uuid REFERENCES work_order_candidates(id) ON DELETE SET NULL,
  source_score_id       uuid REFERENCES customer_signal_scores(id) ON DELETE SET NULL,
  diagnosis             text NOT NULL,
  recommended_action    text NOT NULL,
  severity              text NOT NULL CHECK (severity IN ('healthy','warning','critical','unknown')),
  title                 text NOT NULL,
  description           text NOT NULL DEFAULT '',
  status                text NOT NULL DEFAULT 'open'
                        CHECK (status IN ('open','assigned','in_progress','resolved','cancelled')),
  priority              text NOT NULL DEFAULT 'medium'
                        CHECK (priority IN ('low','medium','high','urgent')),
  assigned_to           text,
  eta_at                timestamptz,
  resolved_at           timestamptz,
  resolution_note       text,
  created_at            timestamptz NOT NULL DEFAULT now(),
  updated_at            timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_wo_status_created
  ON work_orders (status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_wo_priority_created
  ON work_orders (priority, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_wo_customer_created
  ON work_orders (customer_id, created_at DESC)
  WHERE customer_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_wo_tower_created
  ON work_orders (tower_id, created_at DESC)
  WHERE tower_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_wo_ap_created
  ON work_orders (ap_device_id, created_at DESC)
  WHERE ap_device_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_wo_assigned_status
  ON work_orders (assigned_to, status)
  WHERE assigned_to IS NOT NULL;

-- ============================================================================
-- 2. work_order_events: append-only iş emri timeline'ı (status değişimleri,
--    atama, ETA, not, çözüm vb.)
-- ============================================================================
CREATE TABLE IF NOT EXISTS work_order_events (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  work_order_id uuid NOT NULL REFERENCES work_orders(id) ON DELETE CASCADE,
  event_type    text NOT NULL,
  old_value     text,
  new_value     text,
  note          text,
  actor         text NOT NULL DEFAULT 'system',
  created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_woe_wo_created
  ON work_order_events (work_order_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_woe_type_created
  ON work_order_events (event_type, created_at DESC);

-- ============================================================================
-- 3. work_order_candidates: cancelled status'unu da destekleyecek şekilde
--    constraint'i genişlet ve cooldown analizi için index/aktarım kolayı.
-- ============================================================================
ALTER TABLE work_order_candidates
  DROP CONSTRAINT IF EXISTS work_order_candidates_status_check;

ALTER TABLE work_order_candidates
  ADD CONSTRAINT work_order_candidates_status_check
  CHECK (status IN ('open','dismissed','promoted','cancelled'));

CREATE INDEX IF NOT EXISTS idx_woc_customer_diag_status
  ON work_order_candidates (customer_id, diagnosis, status, updated_at DESC)
  WHERE customer_id IS NOT NULL;

-- ============================================================================
-- 4. report_snapshots: scheduler veya API tarafından üretilen rapor
--    çıktılarının (executive summary, problem-customers vs.) JSON olarak
--    saklandığı tablo.
-- ============================================================================
CREATE TABLE IF NOT EXISTS report_snapshots (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  report_type   text NOT NULL,
  period_start  timestamptz NOT NULL,
  period_end    timestamptz NOT NULL,
  payload       jsonb NOT NULL DEFAULT '{}'::jsonb,
  generated_at  timestamptz NOT NULL DEFAULT now(),
  generated_by  text NOT NULL DEFAULT 'system'
);

CREATE INDEX IF NOT EXISTS idx_rs_type_generated
  ON report_snapshots (report_type, generated_at DESC);

CREATE INDEX IF NOT EXISTS idx_rs_period
  ON report_snapshots (period_start, period_end);

-- ============================================================================
-- 5. scoring_thresholds: Faz 7 yeni eşik anahtarları.
--    work_order_duplicate_cooldown_days: aynı (customer_id, diagnosis) için
--    kapatılmış (dismissed/cancelled) bir aday varsa N gün boyunca yenisi
--    üretilmez. SLA eta default'u operatör panel'inde bilgilendirme amaçlı.
-- ============================================================================
INSERT INTO scoring_thresholds (key, value, description) VALUES
  ('work_order_duplicate_cooldown_days', 7,
   'İş emri adayı yeniden açma cooldown süresi (gün); dismissed/cancelled için.'),
  ('work_order_default_eta_hours', 24,
   'İş emri için varsayılan ETA penceresi (saat) — UI önerisidir, dayatmaz.')
ON CONFLICT (key) DO NOTHING;

-- ============================================================================
-- 6. credential_profiles: ca_certificate_pem ve server_name_override alanları
--    Faz 4'te eklenmişti, repository View'ında okunmuyordu. Faz 7 runtime
--    tüketimi için ek bir kolon değişikliği yok; yalnızca View'a eklenecek.
--    Yorum amaçlıdır.
-- ============================================================================

COMMIT;
