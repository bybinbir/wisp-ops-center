# TASK_BOARD — wisp-ops-center

Notasyon: ☐ yapılmadı · ▣ kısmen · ☑ tamamlandı.

---

## 1. Current Status

- ☑ Completed: **Phase 1, 2, 3, 4, 5, 6, 7**
- ▣ Active: **Phase 8 — MikroTik Dude SSH Discovery + Network Inventory** (kod + UI + docs hazır; gerçek MikroTik bağlantı testi şifre repoda olmadığı için manuel doğrulama bekliyor)
- ☐ Remaining: **Phase 9, 10**

---

## 2. Global Safety Rules

- ☑ No fake telemetry
- ☑ No fake scores
- ☑ No fake success
- ☑ No device config write
- ☑ No frequency apply
- ☑ No scan activation
- ☑ No bandwidth-test
- ☑ No Mimosa write
- ☑ No raw secrets in API/audit/logs/docs/commits
- ☑ `data_insufficient` when no telemetry exists
- ☑ Audit all mutating actions (`work_order_candidate.created`, `scoring_threshold.updated`, vb.)

---

## 3. Completed Phases

### Phase 1 — Foundation
- ☑ Go API + Worker iskeleti
- ☑ Next.js Türkçe UI iskeleti
- ☑ PostgreSQL şema taslağı (migration 000001)
- ☑ Adapter contract'ları + scheduler/scoring/reporting/safety dokümanları
- ☑ Hiçbir gerçek cihaz I/O'su yok

### Phase 2 — Inventory + Credentials
- ☑ pgx PostgreSQL entegrasyonu, migration runner
- ☑ Cihaz envanteri CRUD, credential profilleri
- ☑ AES-GCM vault temeli (`WISP_VAULT_KEY`)
- ☑ Capability matrix + audit logs
- ☑ Frontend cihaz/site/tower/link/credential formları

### Phase 3 — MikroTik Read-Only
- ☑ RouterOS API-SSL adapter (read-only, allowlist)
- ☑ SSH fallback (allowlisted komut + RouterOS CLI çevirisi)
- ☑ SNMP read-only collector
- ☑ Telemetri persistence + probe/poll endpoint'leri
- ☑ Device detail UI

### Phase 4 — Mimosa Read-Only
- ☑ Mimosa SNMP adapter (v2c + kısmi v3 USM)
- ☑ Mimosa telemetri persistence
- ☑ TLS verify alanı + SSH host key alanları (credential profile)
- ☑ Credential hardening UI

### Phase 5 — Scheduled Checks + Safe AP→Client Tests
- ☑ Scheduler engine + JobCatalog + risk policy
- ☑ Maintenance window model + API + risk enforcement
- ☑ Low-risk AP→Client testleri (ping_latency, packet_loss, jitter, traceroute)
- ☑ Yüksek riskli testler kapatıldı (`limited_throughput`, `mikrotik_bandwidth_test`)
- ☑ SSH TOFU temeli + SNMPv3 USM runtime
- ☑ Migration 000005 + frontend planlı kontroller, /job-runs, /ap-client-tests

---

## 4. Phase 6 Completed Tasks

### Context Restore
- ☑ Repo, docs, migrations, internal/, apps/api, apps/worker, apps/web okundu
- ☑ Bozuk Go atomic-write artıkları temizlendi (36 temp + 4 corrupted file)
- ☑ Git deposu yeniden kuruldu (no objects, broken config) → branch `phase/006-customer-signal-scoring`

### Migration & Schema
- ☑ `migrations/000006_customer_signal_scoring.sql`
- ☑ `scoring_thresholds` (key, value, description, updated_at, updated_by)
- ☑ `customer_signal_scores` (score, severity, diagnosis, action, reasons, contributing_metrics, raw metric snapshots, is_stale)
- ☑ `ap_health_scores` (degradation_ratio, is_ap_wide_interference, customer counts)
- ☑ `tower_risk_scores`
- ☑ `work_order_candidates` (status open/dismissed/promoted, source_score_id, reasons)
- ☑ `customers.last_signal_*` cache kolonları + index'ler

### Scoring Engine
- ☑ `internal/scoring/engine.go::ScoreCustomer` — deterministik penalty tablosu
- ☑ `internal/scoring/diagnosis.go::Classify` — 12 tanı sıralı karar ağacı
- ☑ `internal/scoring/actions.go::ActionFor` — 10 aksiyon kategorisi mapping
- ☑ AP-wide degradation (`AnalyzePeerSet` + `ScoreAP`)
- ☑ Link/Tower agregasyon (`ScoreLink`, `ScoreTower`)
- ☑ 7 günlük sinyal trend regresyonu (`SignalTrend7d`)
- ☑ Threshold key/range validation (`IsKnownThresholdKey`, `IsValidThresholdValue`)

### Diagnosis & Recommended Actions
- ☑ Kategoriler: healthy, weak_customer_signal, possible_cpe_alignment_issue, ap_wide_interference, ptp_link_degradation, frequency_channel_risk, high_latency, packet_loss, unstable_jitter, device_offline, stale_data, data_insufficient
- ☑ Aksiyonlar: no_action, monitor, schedule_field_visit, check_cpe_alignment, check_customer_cable, check_ap_interference, check_ptp_backhaul, review_frequency_plan, verify_power_or_ethernet, escalate_network_ops

### Problem Customer APIs
- ☑ `GET /api/v1/customers-with-issues` (filtreler: severity, diagnosis, tower_id, ap_device_id, stale, limit, offset)
- ☑ `GET /api/v1/customers/{id}/signal-score`
- ☑ `GET /api/v1/customers/{id}/signal-history`
- ☑ `POST /api/v1/customers/{id}/calculate-score`
- ☑ `POST /api/v1/scoring/run`
- ☑ `GET /api/v1/devices/{id}/ap-health`
- ☑ `GET /api/v1/towers/{id}/risk-score`
- ☑ `GET/PATCH /api/v1/scoring-thresholds`

### Work Order Candidate
- ☑ `POST /api/v1/customers/{id}/create-work-order-from-score`
- ☑ Duplicate guard: aynı `customer_id + diagnosis + status='open'` için yeni satır YOK; mevcut id `duplicate=true` ile döner
- ☑ Yalnız `warning`/`critical` skor aday üretebilir (422 score_severity_not_actionable)
- ☑ `GET /api/v1/work-order-candidates`, `PATCH /api/v1/work-order-candidates/{id}`
- ☑ Audit kaydı `work_order_candidate.created`

### Scheduler Handler
- ☑ `apps/worker/internal/customer_signal_handler.go::CustomerSignalCheckHandler`
- ☑ Worker boot içinde `JobCustomerSignalCheck` registry'ye Faz 6 handler'ı ile bağlandı
- ☑ Hydrate → Engine → SaveCustomerScore döngüsü, hata sayar
- ☐ Gerçek lab Postgres ile job_runs özet görseli (sandbox kısıtı)

### Frontend (Türkçe UI)
- ☑ `Sorunlu Müşteriler` real (`/api/v1/customers-with-issues`, filtreler, "Skoru Yenile", "İş Emri Adayı Oluştur")
- ☑ Customer detail `/musteriler/[id]` (skor, evidence, geçmiş, candidate listesi)
- ☑ Dashboard real kartlar (kritik/uyarı/data_insufficient/AP-wide/last run)
- ☑ Ayarlar — Scoring Thresholds bölümü
- ☑ Kuleler — risk score badge
- ☑ Cihaz detail — AP health badge

### Transport Hardening Closure
- ☑ SSH host key runtime enforcement: `internal/adapters/mikrotik/ssh_client.go::Dial` `EnforcePolicy` çağırıyor (insecure_ignore | trust_on_first_use | pinned)
- ☑ Postgres-backed `SSHKnownHostsStore` Service init'te global olarak set ediliyor
- ☑ RouterOS API TLS verify runtime: `APIClient.Dial` `cfg.VerifyTLS` tüketiyor (`InsecureSkipVerify: !VerifyTLS`)
- ☐ `ca_certificate_pem`, `server_name_override` runtime tüketimi — Faz 7'ye ertelendi (credential profile şemasında alanlar mevcut)

### Documentation
- ☑ `docs/CUSTOMER_SIGNAL_SCORING.md`
- ☑ `docs/PROBLEM_CUSTOMER_DETECTION.md`
- ☑ `docs/WORK_ORDER_CANDIDATES.md`
- ☑ TASK_BOARD.md + README.md güncel

### Tests / Build
- ☑ `internal/scoring/engine_test.go` (eşik penalty + diagnosis + AP/Tower)
- ☑ `internal/scoring/thresholds_test.go` (key/range validation)
- ☑ Duplicate guard testi (in-memory mock; gerçek pgx integration sandbox dışı)
- ☑ `gofmt -l .`, `go vet ./...`, `go test ./...`, `go build ./...` — yeşil
- ☐ `npm run build` — sandbox'ta Node toolchain'i kullanılabilirse çalıştırılır; aksi halde TS değişiklikleri tip uyumlu yazıldı

---

## 5. Phase 7 Active — Work Orders + Reports + Executive Summaries

### Migration & Schema
- ☑ `migrations/000007_work_orders_reports.sql`
- ☑ `work_orders` (status state machine, priority, ETA, atama, çözüm)
- ☑ `work_order_events` (append-only timeline)
- ☑ `report_snapshots` (executive_summary jsonb payload)
- ☑ `work_order_candidates.status` constraint genişletildi (`cancelled` eklendi)
- ☑ Cooldown index `(customer_id, diagnosis, status, updated_at)`
- ☑ `scoring_thresholds`: `work_order_duplicate_cooldown_days=7`, `work_order_default_eta_hours=24`

### Work Order Repository (`internal/workorders/`)
- ☑ State machine: open ↔ assigned ↔ in_progress → resolved | cancelled
- ☑ `PromoteCandidate` (idempotent, lock-based)
- ☑ `Patch` her değişiklik için event yazar (status_changed, priority_changed, eta_updated, assigned, unassigned)
- ☑ `Resolve`, `Cancel`, `Assign`, `AppendEvent`
- ☑ `List` filtre: status/priority/severity/tower/AP/customer/assigned_to/date_range
- ☑ `Counts` dashboard sayaçları

### API endpoints
- ☑ `GET/PATCH /api/v1/work-orders/{id}`
- ☑ `GET /api/v1/work-orders` (filtre + pagination)
- ☑ `POST /api/v1/work-orders/{id}/events`
- ☑ `POST /api/v1/work-orders/{id}/assign` (auto_start desteği)
- ☑ `POST /api/v1/work-orders/{id}/resolve`
- ☑ `POST /api/v1/work-orders/{id}/cancel`
- ☑ `POST /api/v1/work-order-candidates/{id}/promote`
- ☑ `GET /api/v1/reports` (snapshot listesi)
- ☑ `GET /api/v1/reports/executive-summary[.pdf]`
- ☑ `GET /api/v1/reports/problem-customers[.csv]`
- ☑ `GET /api/v1/reports/ap-health[.csv]`
- ☑ `GET /api/v1/reports/tower-risk[.csv]`
- ☑ `GET /api/v1/reports/work-orders[.csv|.pdf]`
- ☑ `GET /api/v1/audit/export[.json|.ndjson]`

### Duplicate Guard Cooldown
- ☑ `duplicate_open_candidate` — aynı open aday
- ☑ `already_promoted` — promoted ve aktif iş emri var
- ☑ `recently_dismissed` — cooldown içinde dismiss
- ☑ `recently_cancelled` — cooldown içinde cancel
- ☑ Threshold: `work_order_duplicate_cooldown_days` (default 7)

### Reports
- ☑ Executive summary: severity dağılımı, top10 risky AP/tower, top diagnoses, work order sayaçları, 7d/30d trend
- ☑ CSV (UTF-8, Türkçe başlık, streaming, `Content-Disposition: attachment`)
- ☑ HTML-printable PDF (server-side gerçek PDF Faz 8/9'a ertelendi — açık teknik borç)

### Scheduler
- ☑ `JobDailyExecutiveSummary` katalog kaydı (risk=low, enabled)
- ☑ `DailyExecutiveSummaryHandler` worker'da kayıtlı
- ☑ `report_snapshots` tablosuna yazım

### TLS Hardening
- ☑ `mikrotik.BuildAPITLSConfig` — CA pool + ServerName override
- ☑ `verify_tls=true + invalid CA` fail-closed (`ErrInvalidCA`)
- ☑ devicectl `verify_tls` typo düzeltildi (önceden `tls_verify` okuyordu)
- ☑ `credentials.View.CACertificateSet` flag (ham PEM API'ye sızmaz)
- ☑ Test: 6 senaryo (`internal/adapters/mikrotik/tls_test.go`)

### Audit Export
- ☑ `/api/v1/audit/export.json` ve `.ndjson`
- ☑ Filtre: action, actor, date_from, date_to, limit (max 50000)
- ☑ 90 gün retention politikası dokümante edildi
- ☐ Otomatik retention scheduler job → Phase 8

### Web UI (Türkçe)
- ☑ `/is-emirleri` — filtreli liste, CSV/PDF butonları
- ☑ `/is-emirleri/[id]` — detay kartları, aksiyon paneli, olay timeline
- ☑ `/raporlar` — rapor merkezi + CSV/PDF linkleri
- ☑ `/raporlar/yonetici-ozeti` — severity, top10 AP/tower, trend
- ☑ Dashboard Phase 7 kartları (Açık İş Emri, Urgent/High, ETA Geçenler, Bugün Oluşturulan)
- ☑ `/musteriler/[id]` aday satırına "İş Emrine Çevir" + "Dismiss"
- ☑ Sidebar etiketi `Faz 7 · iş emirleri + raporlar`

### Documentation
- ☑ `docs/PHASE_007_WORK_ORDERS_REPORTS.md` — kapsamlı dokümantasyon
- ☑ `docs/RUNBOOK_PHASE_007.md` — operatör runbook
- ☑ `README.md` Phase 7 bölümü
- ☑ TASK_BOARD.md güncel

### Tests / Build
- ☑ `internal/workorders/workorders_test.go` (state machine + priority)
- ☑ `internal/scoring/thresholds_test.go` Phase 7 anahtarları
- ☑ `internal/reports/csv_test.go` (Türkçe başlık + ETA overdue)
- ☑ `internal/scheduler/engine_test.go` daily_executive_summary kataloğu
- ☑ `internal/adapters/mikrotik/tls_test.go` (CA / ServerName / fail-closed)
- ☑ `gofmt -l .`, `go vet ./...`, `go test ./...`, `go build ./...` — yeşil
- ☑ `npm run build` — yeşil (16 sayfa)
- ☐ Gerçek Postgres ile promote/cooldown end-to-end (sandbox dışı)

### Açık Borçlar
- ☐ Server-side gerçek PDF rendering (Faz 8/9)
- ☐ Otomatik audit retention temizlik scheduler job (Faz 8)

## 6. Phase 8 Active — MikroTik Dude SSH Discovery + Network Inventory

Detay: `docs/PHASE_008_MIKROTIK_DUDE_DISCOVERY.md`. Branch: `phase/008-mikrotik-dude-discovery`. Migration: `000008_mikrotik_dude_discovery.sql`.

### Env / Secrets
- ☑ `.env.example`: `MIKROTIK_DUDE_HOST/PORT/USERNAME/PASSWORD/TIMEOUT_MS/HOST_KEY_POLICY/HOST_KEY_FINGERPRINT`
- ☑ Şifre runtime'dan okunur; repoda boş; `.gitignore` `.secrets`, `*.pem`, `*.key` korur
- ☑ `internal/config.DudeConfig` + `Configured()` yardımcısı

### SSH Discovery Adapter (`internal/dude`)
- ☑ `client.go`: TOFU/Pinned/InsecureIgnore policy, timeout, correlation_id, sanitized error
- ☑ `allowlist.go`: 18 read-only RouterOS komutu — destructive komut yok (test ile garanti)
- ☑ `parser.go`: RouterOS print detail/simple parser (k=v + quoted + flag/index strip)
- ☑ `classify.go`: Heuristic skoru (Dude type, name prefix, wireless-mode, model hint, interface-type) → 7 kategori + confidence 0..100 + Evidence trail
- ☑ `discovery.go`: Run orchestrator — `/dude/device/print/detail` primary, `/ip/neighbor/print/detail` fallback, `/system/identity` self; MAC>IP>Name dedupe; partial-fail tolerant
- ☑ `sanitize.go`: SanitizeAttrs (raw_metadata redaction) + SanitizeMessage (log/UI)

### Schema (`migrations/000008_mikrotik_dude_discovery.sql`)
- ☑ `network_devices`, `discovery_runs`, `network_links`, `device_category_evidence`, `network_automation_jobs`
- ☑ Partial unique indexler — MAC > (host,name) > name ile duplicate koruma
- ☑ `network_automation_jobs.job_type` CHECK = 'discovery' (destructive aksiyonlar bu fazda kapsam dışı)

### Repository (`internal/networkinv`)
- ☑ `CreateRun` / `FinalizeRun` (running → succeeded|partial|failed)
- ☑ `UpsertDevices` tek transaction; per-device evidence refresh
- ☑ `ListDevices(filter)` + `GetDevice` + `ListRuns` + `LatestRun`

### API (`apps/api/internal/http/handlers_network.go`)
- ☑ `POST /api/v1/network/discovery/mikrotik-dude/test-connection`
- ☑ `POST /api/v1/network/discovery/mikrotik-dude/run` (async, 202; 409 zaten çalışıyorsa)
- ☑ `GET /api/v1/network/discovery/runs`
- ☑ `GET /api/v1/network/devices?category=&status=&unknown=&low_confidence=` + summary bloğu
- ☑ `GET /api/v1/network/devices/{id}`
- ☑ Bearer auth middleware'inden geçer; audit `network.dude.test_connection / run.start / run.finish`

### Action Framework (`internal/networkactions`)
- ☑ Kind enum, IsDestructive, Request/Result, MaintenanceWindow tipi
- ☑ Registry + per-device lock + RateLimiter (token bucket)
- ☑ Stub action her Kind için kayıtlı; `Execute` → `ErrActionNotImplemented`
- ☐ Gerçek frequency_check / ap_client_test / link_signal_test / bridge_health_check (Faz 9)

### Web UI
- ☑ `/ag-envanteri` sayfası — Sidebar'a eklendi, "Faz 8" altyazılı
- ☑ 8 stat card (toplam, AP, link, bridge, CPE, router, switch, bilinmeyen) + son discovery zamanı/durumu/hatası
- ☑ "Bağlantıyı Test Et" + "Discovery Çalıştır" butonları (async + 2sn poll)
- ☑ Tablo (Ad/IP/Kategori-badge/Confidence/Status/LastSeen/Source) + filtreler (kategori, status, low_confidence, unknown_only)
- ☑ Empty/loading/error state'leri

### Tests / Quality Gates
- ☑ Unit: parser, classify, sanitize, allowlist, discovery, networkactions, config
- ☑ `gofmt -l` clean; `go vet ./...` clean; `go build ./...` clean
- ☑ `go test ./internal/dude/... ./internal/networkactions/... ./internal/config/...` yeşil
- ☑ `next build` yeşil (`/ag-envanteri` 5.3 kB statik)
- ☐ Live MikroTik bağlantı testi — şifre repoda olmadığı için manuel (lokal `.env` doldur + `POST /test-connection`)

### Açık Borçlar (Faz 7'den taşınan)
- ☐ Server-side gerçek PDF rendering (Faz 9)
- ☐ Otomatik audit retention temizlik scheduler job (Faz 9)

## 7. Phase 9 Planned — AP/Link Tests + Frequency Correction (Controlled Apply)

- ☐ Frequency check + correction — Faz 8 action framework etkinleştirilir, dry-run zorunlu, confirmation policy, audit + rollback metadata
- ☐ AP-client test motoru — Faz 8 envanterindeki `Category=AP` cihazlarına otomatik koşturma
- ☐ Link signal test + bridge health check
- ☐ Cihaz konfig backup motoru (read-only, RouterOS export)
- ☐ Pre-apply doğrulama: maintenance window içi mi, link kapanmaz mı?
- ☐ Onay zinciri (multi-actor)
- ☐ Apply işlemi sonrası verification + otomatik rollback
- ☐ En yüksek risk: hiçbir adım el kayması ile başlamamalı

## 8. Phase 10 Planned — Production Hardening

- ☐ TLS uçtan uca + secret rotasyon prosedürü
- ☐ Vault/KMS entegrasyonu
- ☐ Prometheus + alert rules
- ☐ Backup/restore drill
- ☐ RBAC + multi-tenant izolasyon
- ☐ SOC2 / KVKK uyum kontrol listesi

---

## 9. Definition of Done

- ☑ Code implemented (deterministik, no placeholder)
- ☑ Docs updated (`docs/`, README, TASK_BOARD)
- ☑ Migrations added (idempotent, transactional)
- ☑ Safety preserved (no write, no apply, no fake data)
- ☑ Audit coverage (mutating actions)
- ☑ No fake data (`data_insufficient` doğru üretiliyor)
- ☑ `gofmt -l .` temiz
- ☑ `go vet ./...` temiz
- ☑ `go test ./...` yeşil
- ☑ `go build ./...` yeşil
- ☐ `npx tsc --noEmit` / `npm run build` — yalnız Node toolchain'i mevcutsa
- ☐ API smoke (Postgres çalışırsa) — sandbox kısıtı
- ☑ Blockers reported (final raporda)
- ☑ Branch/commit status reported (`phase/006-customer-signal-scoring`)
