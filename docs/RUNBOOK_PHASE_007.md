# Runbook — Phase 7

Operasyon ekibi için iş emirleri + raporlama günlük rehberi.

## 1. Migration uygulanması

```bash
# Yeni schema versiyonu 7 — idempotent + transactional
WISP_DATABASE_URL=postgres://... ./apps/api/cmd/api/api migrate
```

Beklenen çıktı: `migration_apply_done version=7 name=000007_work_orders_reports.sql`.

`schema_migrations` satırı kontrol edilir:

```sql
SELECT version, name, applied_at FROM schema_migrations ORDER BY version;
```

## 2. Operatör akışları

### 2.1 Aday → İş emri

1. `/musteriler/[id]` → "İş Emri Adayı Oluştur" tıklanır.
2. Aynı sayfa altındaki "İş Emri Adayları" tablosunda satır görünür.
3. "İş Emrine Çevir" butonu ile aday gerçek iş emrine dönüştürülür ve `/is-emirleri/[id]`'ye yönlendirilir.
4. Aynı butona tekrar basılırsa duplicate=true ile mevcut iş emri açılır (yeni satır oluşmaz).

### 2.2 İş emri yönetimi

`/is-emirleri/[id]`:

- **Atanan kaydet**: `assigned_to` alanı doldurulur. `auto_start: true` open → assigned otomatik geçişi sağlar.
- **Priority / ETA Kaydet**: priority dropdown ve datetime-local. Boş bırakılırsa eski değer korunur.
- **İşleme Al**: in_progress'e geçer.
- **Çözüldü Olarak İşaretle**: resolved + opsiyonel resolution_note.
- **İptal Et**: confirmation prompt + cancelled.
- **Not Ekle**: yalnız `note_added` event'i (status değiştirmez).

Geçersiz status geçişleri `422 invalid_status_transition` döner; UI `alert` ile gösterir.

### 2.3 Raporlar

`/raporlar`:

- CSV indirmeleri (problem-customers, ap-health, tower-risk, work-orders) — UTF-8, Türkçe başlık.
- Yazdırılabilir HTML (PDF: tarayıcı "Yazdır → PDF olarak kaydet").
- `/raporlar/yonetici-ozeti` — severity dağılımı + en riskli 10 AP/kule + son 7 gün trendi.

### 2.4 Dashboard

Ana sayfa kartları artık Phase 7 metriklerini gösterir:
- Açık İş Emri
- Urgent / High Priority
- ETA Geçenler
- Bugün Oluşturulan

## 3. Cooldown davranışı

| Sebep | HTTP cevabı | Audit metadata |
|---|---|---|
| `duplicate_open_candidate` | `200 OK` `{duplicate:true, id: <existing>}` | `duplicate=true` |
| `already_promoted`         | `200 OK` aynı şekilde — UI promoted iş emrine link verir | iş emri ID promoted_work_order_id'den |
| `recently_dismissed`       | `200 OK` aynı şekilde; cooldown gün sayısı `work_order_duplicate_cooldown_days` (varsayılan 7) | sebep eklenir |
| `recently_cancelled`       | `200 OK` aynı şekilde | sebep eklenir |

Cooldown'ı değiştirmek için:

```
PATCH /api/v1/scoring-thresholds
{ "updates": { "work_order_duplicate_cooldown_days": 14 } }
```

## 4. Scheduler — daily_executive_summary

Job tipi: `daily_executive_summary` (risk=low, enabled).

Çalıştırma:

1. `WISP_SCHEDULER_ENABLED=true` ile worker boot.
2. `scheduled_checks` tablosuna kayıt:
   ```
   POST /api/v1/scheduled-checks
   { "job_type": "daily_executive_summary", "schedule_type": "daily", "cron": "0 6", ... }
   ```
3. Worker tetiklediğinde `report_snapshots` tablosuna `executive_summary` satırı eklenir.

Snapshot listesi:

```
GET /api/v1/reports?type=executive_summary
```

Hata durumunda `job_runs.error_text` net mesaj içerir.

## 5. TLS hardening (RouterOS API-SSL)

Yeni credential profile alanları:

- `verify_tls`: TLS doğrulaması açık mı.
- `ca_certificate_pem`: özel CA PEM (tek veya zincir).
- `server_name_override`: SNI/peer doğrulamada kullanılacak hostname.

Davranış:

- `verify_tls=false` → InsecureSkipVerify (Faz 3 default'u). `server_name_override` SNI'a yine de yazılır.
- `verify_tls=true` + boş CA → sistem CA havuzu.
- `verify_tls=true` + geçersiz CA → **fail-closed** (`ErrInvalidCA`); bağlantı kurulmaz.

API:

```
PATCH /api/v1/credential-profiles/{id}
{ "verify_tls": true,
  "ca_certificate_pem": "-----BEGIN CERTIFICATE-----...",
  "server_name_override": "ap-north.example.com" }
```

Ham PEM API yanıtında **dönmez** — sadece `ca_certificate_set: true` flag'i.

## 6. Audit export

```
GET /api/v1/audit/export.json?action=work_order.created&date_from=2026-04-01T00:00:00Z
GET /api/v1/audit/export.ndjson?limit=20000
```

90 gün retention politikası. Otomatik temizlik için manuel SQL veya Phase 8'de scheduler job:

```sql
DELETE FROM audit_logs WHERE at < now() - interval '90 days';
```

## 7. Smoke test (Postgres mevcut ise)

```bash
# Migration uygula
WISP_DATABASE_URL=postgres://... ./apps/api/cmd/api/api migrate

# API'yi başlat
WISP_DATABASE_URL=... WISP_VAULT_KEY=$(openssl rand -base64 32) ./apps/api/cmd/api/api

# Aday üret (Phase 6 endpoint'i)
curl -X POST http://localhost:8080/api/v1/customers/<uuid>/calculate-score
curl -X POST http://localhost:8080/api/v1/customers/<uuid>/create-work-order-from-score
# → { "data": { "id": "<candidate_uuid>", ... } }

# Promote
curl -X POST http://localhost:8080/api/v1/work-order-candidates/<candidate_uuid>/promote \
  -H "Content-Type: application/json" \
  -d '{ "title": "Saha kontrol — Müşteri X", "priority": "high" }'
# → { "data": { "id": "<work_order_uuid>", "status": "open", ... } }

# Raporlar
curl http://localhost:8080/api/v1/reports/executive-summary | jq
curl -OJ "http://localhost:8080/api/v1/reports/work-orders.csv?status=open"
```

## 8. Rollback

`docs/PHASE_007_WORK_ORDERS_REPORTS.md` §8'e bakın. Hızlı özet:

```sql
BEGIN;
DROP TABLE work_order_events CASCADE;
DROP TABLE work_orders CASCADE;
DROP TABLE report_snapshots CASCADE;
ALTER TABLE work_order_candidates DROP CONSTRAINT work_order_candidates_status_check;
ALTER TABLE work_order_candidates ADD CONSTRAINT work_order_candidates_status_check
  CHECK (status IN ('open','dismissed','promoted'));
DELETE FROM scoring_thresholds WHERE key IN
  ('work_order_duplicate_cooldown_days','work_order_default_eta_hours');
DELETE FROM schema_migrations WHERE version = 7;
COMMIT;
```

Sonrasında `git checkout phase/006-customer-signal-scoring` ile kod tarafı geri alınır.

## 9. Yaygın sorunlar

| Belirti | Sebep | Çözüm |
|---|---|---|
| `503 database_not_configured` | Postgres bağlı değil | `WISP_DATABASE_URL` set + migration |
| `422 invalid_status_transition` | resolved/cancelled üzerinde aksiyon | Geçişleri docs/PHASE_007 §1.6'dan kontrol et |
| `422 candidate_not_promotable` | Aday dismissed/cancelled | Yeni aday üret veya cooldown bekle |
| Promote sonrası boş work order ID | Migration eksik | `schema_migrations` versiyonunu kontrol et |
| ETA UI'da yanlış zaman dilimi | datetime-local local TZ kullanır | API tarafında UTC saklanır; UI `toLocaleString` ile gösterir |
| TLS handshake hata | invalid CA | `ca_certificate_pem` PEM formatında mı, ServerName override eşleşiyor mu |

## 10. İletişim

- Aktif Faz: 7 (work orders + reports + executive summaries)
- Branch: `phase/007-work-orders-reports-exec-summaries`
- İlgili dokümanlar: `docs/PHASE_007_WORK_ORDERS_REPORTS.md`, `docs/SAFETY_MODEL.md`.
