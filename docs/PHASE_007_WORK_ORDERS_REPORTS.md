# Phase 7 — Work Orders, Reports & Executive Summaries

**Branch:** `phase/007-work-orders-reports-exec-summaries`
**Bağımlılık:** Phase 6 (`customer_signal_scores`, `ap_health_scores`, `tower_risk_scores`, `work_order_candidates`).

Phase 7 üç şeyi getirir:

1. Gerçek **work order** modülü (state machine + olay timeline + atama/ETA/çözüm).
2. Operatöre net **rapor**lar: yönetici özeti, problem müşteriler, AP sağlığı, kule riski, iş emri raporu (JSON, CSV, yazdırılabilir HTML/PDF).
3. Operasyon karar destek: **scheduler daily_executive_summary**, audit export, work order duplicate **cooldown**.

Ek olarak Phase 6'dan kalan **TLS hardening borcu** (ca_certificate_pem, server_name_override) kapatıldı.

---

## 1. Veri Modeli

### 1.1 `work_orders`

```
id                  uuid pk
customer_id         uuid → customers (SET NULL on delete)
ap_device_id        uuid → devices   (SET NULL)
tower_id            uuid → towers    (SET NULL)
source_candidate_id uuid → work_order_candidates (SET NULL)
source_score_id     uuid → customer_signal_scores (SET NULL)
diagnosis           text
recommended_action  text
severity            text  CHECK ∈ {healthy, warning, critical, unknown}
title               text
description         text
status              text  CHECK ∈ {open, assigned, in_progress, resolved, cancelled}
priority            text  CHECK ∈ {low, medium, high, urgent}
assigned_to         text
eta_at              timestamptz
resolved_at         timestamptz
resolution_note     text
created_at          timestamptz default now()
updated_at          timestamptz default now()
```

Indeksler:

```
idx_wo_status_created       (status, created_at DESC)
idx_wo_priority_created     (priority, created_at DESC)
idx_wo_customer_created     (customer_id, created_at DESC) WHERE not null
idx_wo_tower_created        (tower_id, created_at DESC) WHERE not null
idx_wo_ap_created           (ap_device_id, created_at DESC) WHERE not null
idx_wo_assigned_status      (assigned_to, status) WHERE not null
```

### 1.2 `work_order_events`

Append-only timeline. Statü değişikliği, atama, ETA güncellemesi, not, resolve, cancel olayları yazılır.

```
id, work_order_id, event_type, old_value, new_value, note, actor, created_at
```

Bilinen `event_type`'lar:
`created · status_changed · assigned · unassigned · priority_changed · eta_updated · note_added · resolved · cancelled · reopened`

### 1.3 `work_order_candidates` genişletmesi

- `status` constraint genişletildi: `cancelled` da kabul edilir.
- Yeni partial index: `(customer_id, diagnosis, status, updated_at DESC)` — cooldown sorgusu O(log n).

### 1.4 `report_snapshots`

`scheduler/daily_executive_summary` ve manuel rapor sürümlerinin JSON snapshot'ı. Geriye dönük raporlama ve compliance için.

```
id, report_type, period_start, period_end, payload jsonb, generated_at, generated_by
```

### 1.5 `scoring_thresholds` ek anahtarları

| Anahtar | Varsayılan | Anlam |
|---|---|---|
| `work_order_duplicate_cooldown_days` | 7 | Aynı (customer_id, diagnosis) için kapatılmış (dismissed/cancelled) bir aday varsa N gün boyunca yenisi üretilmez. |
| `work_order_default_eta_hours`        | 24 | UI öneri amaçlıdır; dayatma yoktur. |

### 1.6 State machine

```
open ──→ assigned ──→ in_progress ──→ resolved
  │          │             │          (terminal)
  │          ↓             ↓
  ├──→ cancelled ←──── cancelled
  │
  └──→ in_progress (kısayol)
```

Reopen yalnız `assigned` veya `in_progress` üzerinden `open`'a izin verilir; `resolved/cancelled` terminaldir.

---

## 2. API

### 2.1 Work order CRUD

| Method | Path | Açıklama |
|---|---|---|
| GET    | `/api/v1/work-orders` | Filtre: `status, priority, severity, tower_id, ap_device_id, customer_id, assigned_to, date_from, date_to, limit, offset`. Yanıt: `{data, total, limit, offset}`. |
| GET    | `/api/v1/work-orders/{id}` | Detay + olay timeline. |
| PATCH  | `/api/v1/work-orders/{id}` | Status, priority, atanan, başlık, açıklama, ETA. Geçersiz transition → `422 invalid_status_transition`. |
| POST   | `/api/v1/work-orders/{id}/events` | Manuel event ekle (note_added vb.). |
| POST   | `/api/v1/work-orders/{id}/assign` | Atama; opsiyonel `auto_start: true` open→assigned otomatik geçişi yapar. |
| POST   | `/api/v1/work-orders/{id}/resolve` | open/assigned/in_progress → resolved. |
| POST   | `/api/v1/work-orders/{id}/cancel`  | Aktif → cancelled. |

### 2.2 Aday → İş Emri promosyonu

```
POST /api/v1/work-order-candidates/{id}/promote
{ "title?", "description?", "priority?", "assigned_to?", "eta_at?" }
```

Davranış:

- Aday status='open' → yeni `work_orders` satırı, candidate status=`promoted`, `promoted_work_order_id` set edilir.
- Aday zaten `promoted` ise `200 OK` + `duplicate=true` ve mevcut iş emri döner.
- Aday `dismissed/cancelled` ise `422 candidate_not_promotable`.
- Audit: `work_order.created` (yeni promosyon) ve `work_order.promoted` (her durumda).

### 2.3 Reports

JSON gövdeli endpointler:

- `GET /api/v1/reports` — son `report_snapshots` listesi.
- `GET /api/v1/reports/executive-summary` — yönetici özeti.
- `GET /api/v1/reports/problem-customers`
- `GET /api/v1/reports/ap-health`
- `GET /api/v1/reports/tower-risk`
- `GET /api/v1/reports/work-orders`

CSV (filtreyle uyumlu, UTF-8, Türkçe başlık, attachment):

- `/api/v1/reports/problem-customers.csv`
- `/api/v1/reports/ap-health.csv`
- `/api/v1/reports/tower-risk.csv`
- `/api/v1/reports/work-orders.csv`

PDF (yazdırılabilir HTML — açık teknik borç bkz. §6):

- `/api/v1/reports/executive-summary.pdf`
- `/api/v1/reports/work-orders.pdf`

### 2.4 Audit export

```
GET /api/v1/audit/export[.json|.ndjson]?action=&actor=&date_from=&date_to=&limit=
```

JSON (default `5000`, max `50000`) veya NDJSON streaming. Retention politikası §7.

### 2.5 Duplicate guard cevabı (Phase 6 endpoint'i genişledi)

`POST /api/v1/customers/{id}/create-work-order-from-score` artık Outcome içinde `duplicate=true` döndürdüğünde sebep ayrıştırılabilir. Repository tarafında `CreateCandidateOutcome.DuplicateReason` üretilir; HTTP cevabında `data.duplicate` zaten vardı, ek alan eklemedik (UI mevcut akışı kırmasın diye), ancak audit metadata'da `duplicate_reason` operatörle paylaşılabilir.

Sebep değerleri:
`duplicate_open_candidate · already_promoted · recently_dismissed · recently_cancelled`

---

## 3. Web UI (Türkçe)

| Yol | İçerik |
|---|---|
| `/is-emirleri` | Filtreli iş emri listesi, CSV/PDF indirme. Toolbar: status, priority, severity, tower, AP, atanan. |
| `/is-emirleri/[id]` | Detay kartları (status / severity-priority / ETA / atanan), aksiyon paneli (atama, priority, ETA, çöz, iptal, not), olay timeline. |
| `/raporlar` | Rapor merkezi: CSV indirmeleri, yazdırılabilir HTML/PDF linkleri, yönetici özeti girişi. |
| `/raporlar/yonetici-ozeti` | Severity dağılımı, en riskli 10 AP/kule, en sık tanılar, son 7 gün trendi, açık iş emri özetleri. |
| `/musteriler/[id]` | Aday satırlarına "İş Emrine Çevir" + "Dismiss" butonları eklendi. Promote edilen aday iş emri detayına link verir. |
| `/` Dashboard | Phase 7 kartları: Açık İş Emri, Urgent/High, ETA Geçenler, Bugün Oluşturulanlar. |

Boş durumlar: tablolar gerçek veri yokken net mesaj gösterir; DB bağlı değilse banner uyarısı.

---

## 4. Scheduler

Yeni job tipi: **`daily_executive_summary`** (`risk=low`, enabled). Worker handler: `DailyExecutiveSummaryHandler`.

Davranış:
- `reports.BuildExecutiveSummary` → `report_snapshots` insert.
- 60 sn timeout (sandbox güvenliği).
- Cihaza yazma yok; frekans değişikliği yok; bandwidth-test yok.

`AllJobTypes()` ve `JobCatalog` listelerine eklendi; `EnsureJobAllowed` testi Phase 7 için bu job'ı doğrular.

---

## 5. Security

Kesin yasaklar (Faz 7 testleriyle de doğrulanmış sınırlar):

- Mikrotik / Mimosa cihazlarına **write yok**.
- Frekans / kanal **apply yok**.
- `bandwidth-test` çalıştırma **yok**.
- Müşteri bağlantısını kesecek işlem **yok**.
- Otomatik cihaz konfigürasyonu **yok**.

İzinli işlemler:

- Skor okuma, raporlama, aday üretme, iş emri üretme, CSV/PDF export, audit, scheduler summary.

### 5.1 RouterOS API-SSL TLS hardening

`internal/adapters/mikrotik/tls.go::BuildAPITLSConfig` davranışı:

| VerifyTLS | CACertificatePEM | ServerNameOverride | Sonuç |
|---|---|---|---|
| false | — | — | InsecureSkipVerify=true (Faz 3 davranışı korunur) |
| false | — | "router.lab" | InsecureSkipVerify=true; SNI ad olarak "router.lab" |
| true  | — | — | Sistem CA havuzu kullanılır |
| true  | valid PEM | — | RootCAs custom CA |
| true  | invalid PEM | — | **fail-closed** → `ErrInvalidCA`; bağlantı kurulmaz |
| true  | valid | "ap-north.example.com" | RootCAs + ServerName override |

Kapsanan testler (`internal/adapters/mikrotik/tls_test.go`):
- VerifyTLS=false default
- Server name override insecure modda da SNI'ya yazılır
- VerifyTLS=true CA olmadan (sistem havuzu)
- VerifyTLS=true + custom CA AppendCertsFromPEM
- VerifyTLS=true + invalid CA → ErrInvalidCA
- ServerName override secure modda peer doğrulama için kullanılır

### 5.2 devicectl entegrasyonu

`internal/devicectl/service.go::loadCredential` artık `verify_tls` kolonunu doğru adıyla okur (Phase 6'daki `tls_verify` typo'su düzeltildi) ve `ca_certificate_pem`, `server_name_override` alanlarını mikrotik.Config'e taşır.

### 5.3 Credential View

`credentials.View` artık `ca_certificate_set: bool` döner — ham PEM API'ye sızdırılmaz. Create/Update payload'ları `ca_certificate_pem` alanı kabul eder.

---

## 6. Açık Borçlar

### 6.1 Server-side PDF rendering

Phase 7'de PDF endpointleri **yazdırılabilir HTML** olarak servis edilir. Tarayıcının "Yazdır → PDF olarak kaydet" akışı kurumsal görünümde A4 / landscape çıktı verir.

Server-side gerçek PDF için seçenekler:

- `signintech/gopdf` veya `jung-kurt/gofpdf` — Türkçe/UTF-8 için harici font (DejaVuSans veya benzeri) gerekir; bağımlılık ağırlığı ve CGO yok.
- `chromedp + headless Chrome` — sürümleme/sandbox riski, prod için ekstra container.

Faz 8 / 9'da değerlendirilmek üzere bu doküman ve TASK_BOARD'a yazıldı.

### 6.2 Audit retention otomasyonu

`/api/v1/audit/export` ile dışa aktarılır. **90 gün retention** politikası §7'de yazılı; otomatik temizlik için Phase 8'de cron benzeri bir scheduler job önerilir (`audit_retention_purge`, risk=low) — Phase 7'de güvenli olmadığı için uygulanmadı. Manuel SQL:

```sql
DELETE FROM audit_logs WHERE at < now() - interval '90 days';
```

### 6.3 Dynamic API typings

UI'daki Phase 7 türleri `apps/web/src/lib/api.ts` içine eklendi. `WorkOrderEvent` ve `ExecutiveSummary` strict-typed. CredentialProfile.View tarafına `ca_certificate_set` eklemesi opsiyonel — UI Faz 7'de bu alanı henüz kullanmıyor.

---

## 7. Audit Retention Policy

- `audit_logs` tablosu append-only.
- Retention süresi: **90 gün**.
- Saklamanın amacı: olay araştırması (security + operational), KVKK kişisel veri ilkesine uyumlu (audit_logs içinde **ham secret yok**, yalnızca operasyon olayları).
- Export yöntemi: `GET /api/v1/audit/export[.json|.ndjson]` — JSON Array veya NDJSON streaming.
- Otomatik silme: Phase 7'de **uygulanmadı**. Phase 8 için scheduler job önerildi.

---

## 8. Rollback Planı

Phase 7 tamamen ek niteliktedir; mevcut Phase 6 işlevselliğini kırmaz.

1. **Migration geri alma:** Phase 7 yeni tablolar (`work_orders`, `work_order_events`, `report_snapshots`) ekler ve `work_order_candidates` constraint'ini genişletir. Hızlı rollback için:
   ```sql
   DROP TABLE IF EXISTS work_order_events CASCADE;
   DROP TABLE IF EXISTS work_orders        CASCADE;
   DROP TABLE IF EXISTS report_snapshots   CASCADE;

   ALTER TABLE work_order_candidates DROP CONSTRAINT IF EXISTS work_order_candidates_status_check;
   ALTER TABLE work_order_candidates ADD CONSTRAINT work_order_candidates_status_check
     CHECK (status IN ('open','dismissed','promoted'));

   DELETE FROM scoring_thresholds WHERE key IN
     ('work_order_duplicate_cooldown_days','work_order_default_eta_hours');

   DELETE FROM schema_migrations WHERE version = 7;
   ```

2. **Code rollback:** branch'i bir önceki SHA'ya `git reset --hard 86b7fee` veya `git checkout phase/006-customer-signal-scoring`.

3. **Worker:** scheduler job tipi unknown olursa `EnsureJobAllowed` kontrolü `daily_executive_summary` için hata döndürür. Mevcut planlanmış check'ler bu nedenle engellenmez.

4. **UI:** Phase 7 sayfaları olmadan da Phase 6 davranışı korunur; `/is-emirleri` ve `/raporlar` Phase 6 versiyonları skeleton banner gösteriyordu.

---

## 9. Test Sonuçları

### 9.1 Go

```
gofmt -l .                        # temiz
go vet -buildvcs=false ./...      # temiz
go test -buildvcs=false ./...     # tüm paketler PASS
go build -buildvcs=false ./...    # temiz
```

Yeni testler:

- `internal/workorders/workorders_test.go` — state machine geçişleri, priority varsayılanı, terminal kontrol.
- `internal/scoring/thresholds_test.go` — Phase 7 eşik anahtarları ve aralıkları.
- `internal/reports/csv_test.go` — Türkçe başlık, ETA overdue flag, AP-health header sırası.
- `internal/scheduler/engine_test.go` — `JobDailyExecutiveSummary` katalog kaydı.
- `internal/adapters/mikrotik/tls_test.go` — 6 TLS senaryosu (insecure, override, valid CA, invalid CA fail-closed, server name override).

### 9.2 Frontend

```
npm install
npm run build  # ✓ tüm rotalar derlendi (16 sayfa)
```

Phase 7 yeni rotalar: `/is-emirleri`, `/is-emirleri/[id]`, `/raporlar/yonetici-ozeti`. Sidebar etiketi `Faz 7 · iş emirleri + raporlar`.

### 9.3 Test edilemeyenler

- Gerçek lab Postgres: cooldown davranışı, promote idempotency, scheduler daily_executive_summary kayıt zinciri yalnızca canlı DB üzerinde end-to-end doğrulanır.
- TLS handshake: BuildAPITLSConfig saf TLS config üretimini test eder; gerçek RouterOS sertifikası ile handshake yine lab gerektirir.
- PDF render: HTML yazdırılabilir; tarayıcı PDF kaydı manuel doğrulama ister.

---

## 10. İlgili Dokümanlar

- `docs/RUNBOOK_PHASE_007.md` — operatör runbook.
- `docs/SAFETY_MODEL.md` — güvenlik sınırları.
- `docs/CUSTOMER_SIGNAL_SCORING.md` — Phase 6 skor motoru.
- `docs/WORK_ORDER_CANDIDATES.md` — Phase 6 aday üretimi.
