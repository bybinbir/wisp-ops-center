# TASK_BOARD — wisp-ops-center

Notasyon: ☐ yapılmadı · ▣ kısmen · ☑ tamamlandı.

---

## 1. Current Status

- ☑ Completed: **Phase 1, 2, 3, 4, 5**
- ▣ Active: **Phase 6 — Customer Signal Scoring + Problem Customer Detection** (kod + UI + docs hazır; gerçek lab Postgres ile end-to-end doğrulama bekliyor)
- ☐ Remaining: **Phase 7, 8, 9, 10**

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

## 4. Phase 6 Active Tasks

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

## 5. Phase 7 Planned — Reports + Work Orders + Executive Summaries

- ☐ `reports` tablosu + günlük/haftalık özet hesaplayıcı
- ☐ Operatör onayıyla aday → gerçek iş emri promosyonu (`promoted_work_order_id`)
- ☐ Saha ekibi atama, ETA, status workflow
- ☐ Yönetici özeti UI (kule × hafta heat-map, kritik müşteri trendi)
- ☐ PDF/CSV export endpoint'leri
- ☐ ca_certificate_pem + server_name_override runtime tüketimi
- ☐ Audit retention politikası

## 6. Phase 8 Planned — Frequency Recommendation Engine

- ☐ Read-only sürveyans verilerinden frekans riski analizi
- ☐ Kanal/genişlik **önerisi** üret — apply YOK
- ☐ AP/PtP konfigine yansıtma yapılmaz; yalnız UI'de gösterilir
- ☐ Operatör onayı gerekmeden hiçbir öneri otomatik çalışmaz

## 7. Phase 9 Planned — Controlled Apply + Backup + Rollback

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
