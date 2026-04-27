# wisp-ops-center

WISP operasyon karar platformu. Genel bir cihaz yönetim paneli **değildir**.

> **Tek bir soruyu cevaplar:** *Bugün ağda ne bozuk, kime müdahale etmeliyim, hangi link riskli?*

## Faz Durumu

**Aktif faz:** **Faz 6 — Customer Signal Scoring + Problem Customer Detection.** Faz 1-5 tamamlandı.

Tam yol haritası: `TASK_BOARD.md`.

## Stack

| Katman | Teknoloji |
|---|---|
| Backend | Go 1.22 + jackc/pgx/v5 + go-routeros/v3 + gosnmp + x/crypto/ssh |
| Frontend | Next.js 14 + TypeScript (strict) |
| Veritabanı | PostgreSQL 14+ |
| Şifreleme | AES-256-GCM (golang.org/x/crypto) |
| Zamanlayıcı | Asynq uyumlu sözleşme (Faz 5'te aktif) |

## Repo Yapısı

```
wisp-ops-center/
  apps/
    api/      Go HTTP API (apps/api/cmd/api)
    web/      Next.js + TypeScript
    worker/   Go worker (apps/worker/cmd/worker)
  internal/
    audit/  config/  credentials/  database/
    devices/  inventory/  links/  customers/  logger/
    adapters/{mikrotik,mimosa,snmp,ssh}
    devicectl/  telemetry/
    scheduler/  scoring/  reports/  recommendations/
  migrations/    000001_initial_schema.sql
                 000002_phase2_inventory_hardening.sql
                 000003_mikrotik_readonly.sql
                 000004_mimosa_readonly_and_credentials.sql
                 000005_scheduled_checks_ap_client_tests.sql
                 000006_customer_signal_scoring.sql
  docs/          ARCHITECTURE / DATA_MODEL / DEVICE_CAPABILITY_MODEL
                 SCORING_MODEL / SCHEDULER_MODEL / SAFETY_MODEL
                 AP_CLIENT_TEST_ENGINE / GITHUB_WORKFLOW
                 MIKROTIK_READONLY_INTEGRATION / MIMOSA_READONLY_INTEGRATION
                 SCHEDULED_CHECKS_ENGINE / AP_CLIENT_TESTS_RUNTIME
                 MAINTENANCE_WINDOWS / SSH_HOST_KEY_POLICY
                 CUSTOMER_SIGNAL_SCORING / PROBLEM_CUSTOMER_DETECTION
                 WORK_ORDER_CANDIDATES / VAULT_ROTATION
  infra/         nginx, systemd, prometheus
  scripts/       dev_run_*, db_migrate
```

## Lokal Çalıştırma

### Önkoşullar
- Go 1.22+, Node 20+ ve npm, PostgreSQL 14+

### Komutlar

```bash
cp .env.example .env
# .env: WISP_DATABASE_URL, WISP_VAULT_KEY (openssl rand -base64 32), WISP_API_TOKEN

# Veritabanı
psql -U postgres -c "CREATE DATABASE wispops"
psql -U postgres -c "CREATE USER wispops_app WITH PASSWORD '...';"

# Migration (Faz 1+2+3 birlikte uygulanır)
go run ./apps/api/cmd/api -migrate -migrations-dir migrations

# API
bash scripts/dev_run_api.sh    # http://localhost:8080/api/v1/health

# Worker (ayrı terminal)
bash scripts/dev_run_worker.sh

# Web (ayrı terminal)
bash scripts/dev_run_web.sh    # http://localhost:3000
```

### MikroTik Probe / Read-only Poll Kullanımı

1. Cihazlar sayfasında bir MikroTik cihaz ekleyin.
2. Ayarlar → Kimlik Profilleri'nden `routeros_api_ssl` tipinde bir profil ekleyip secret'ı girin.
3. (Geçici, Faz 4'te UI'a taşınacak) `device_credentials` tablosuna SQL ile profili cihaza bağlayın:
   ```sql
   INSERT INTO device_credentials(device_id, profile_id, transport)
   VALUES ('<device-id>','<profile-id>','api-ssl');
   ```
4. Cihazlar sayfasında **Probe** ve **Read-only Poll** butonlarını kullanın.
5. `/cihazlar/<id>` detay sayfasında son sağlık + arayüzler + kablosuz istemciler + poll geçmişini görün.

### Test ve Format

```bash
gofmt -l .
go vet ./...
go test ./...
go build ./...

cd apps/web
npm install
npm run typecheck
npm run lint
npm run build
```

## Ortam Değişkenleri

| Değişken | Açıklama | Faz 3 zorunlu mu? |
|---|---|---|
| `WISP_HTTP_ADDR` | API adresi | hayır (default `:8080`) |
| `WISP_API_TOKEN` | Bearer token | önerilir |
| `WISP_DATABASE_URL` | Postgres DSN | **evet** |
| `WISP_VAULT_KEY` | AES-256 anahtarı (base64/hex 32 bayt) | **evet** |
| `LOG_FORMAT` | text / json | hayır |
| `NEXT_PUBLIC_API_BASE` | Web tarafının API hedefi | hayır |
| `NEXT_PUBLIC_API_TOKEN` | Web → API bearer token | API token açıksa |

## Faz 6'da Yapılan

- `internal/scoring`: deterministik kural tabanlı skor motoru (engine,
  thresholds, diagnosis, actions, ap_degradation, trend, hydrator, repository).
  ML kullanılmaz, fake skor üretilmez; veri yoksa `data_insufficient` döner.
- 12 tanı + 10 önerilen aksiyon kategorisi sıralı karar ağacı.
- Migration 000006: `scoring_thresholds`, `customer_signal_scores`,
  `ap_health_scores`, `tower_risk_scores`, `work_order_candidates`,
  `customers.last_signal_*` cache.
- API: `POST /scoring/run`, `GET/PATCH /scoring-thresholds` (key+range
  validation, audit), `POST /customers/{id}/calculate-score`,
  `GET /customers/{id}/signal-score`, `GET /customers/{id}/signal-history`,
  `GET /customers-with-issues`, `GET /devices/{id}/ap-health`,
  `GET /towers/{id}/risk-score`, `GET/PATCH /work-order-candidates(/{id})`,
  `POST /customers/{id}/create-work-order-from-score` (duplicate guard).
- Worker: `customer_signal_check` handler — hydrate → engine → save döngüsü.
- Transport hardening kapanışı: SSH host key TOFU/Pinned **runtime
  enforcement** (mikrotik adapter `Dial` çağrısında `EnforcePolicy`),
  RouterOS API **TLS verify runtime tüketimi** (`InsecureSkipVerify =
  !VerifyTLS`).
- Frontend: gerçek **Sorunlu Müşteriler** tablosu (filtre + skor yenile +
  iş emri adayı), `/musteriler/[id]` müşteri detay (skor, evidence,
  geçmiş, aday listesi), Dashboard real kart sayıları, Ayarlar — Skor
  Eşikleri, Kuleler risk badge, Cihaz detay AP-health kartı.
- Dokümantasyon: `CUSTOMER_SIGNAL_SCORING.md`,
  `PROBLEM_CUSTOMER_DETECTION.md`, `WORK_ORDER_CANDIDATES.md`.

## Faz 5'te Yapılan

- `internal/scheduler` engine: JobCatalog, planner (cron alt kümesi), MaintenanceWindow, GuardWindow, repository (CRUD + job_runs).
- `internal/apclienttest` runner: bounded server-originated ping/loss/jitter/traceroute. Sentinel hata sözleşmesi + diagnosis sınıfları.
- `internal/adapters/ssh` host key TOFU foundation: `insecure_ignore | trust_on_first_use | pinned` politikaları + `ssh_known_hosts` Postgres store.
- `internal/devicectl/snmpv3.go`: SNMPv3 USM secret'larını Vault üzerinden çözüp Mimosa adapter'a aktarır.
- migrations/000005: `maintenance_windows`, `scheduler_locks`, `ssh_known_hosts` + `scheduled_checks` Phase 5 alanları + `ap_client_test_results` ek alanları + `job_runs` ek alanları.
- API: scheduled-checks CRUD + run-now, job-runs, maintenance-windows CRUD, ap-client-test-runs/run-now, ap-client-test-results.
- Worker: env-gated `RunSchedulerLoop` (WISP_SCHEDULER_ENABLED) — concurrency 4, per-job timeout, job_runs persistence.
- Frontend: yeni `/planli-kontroller` (gerçek CRUD), `/job-runs`, `/ap-client-tests` sayfaları + sidebar nav.
- Dokümantasyon: SCHEDULED_CHECKS_ENGINE.md, AP_CLIENT_TESTS_RUNTIME.md, MAINTENANCE_WINDOWS.md, SSH_HOST_KEY_POLICY.md.

## Faz 4'te Yapılan

- Mimosa SNMP read-only adapter — standart MIB OID'leri, IF-MIB ifTable/ifXTable; vendor MIB "unverified" olarak işaretli, partial poll sonucu döner.
- SNMPv2c + SNMPv3 USM credential profile şeması (MD5/SHA/SHA256 + DES/AES/AES192/AES256); UI'da yalnızca `secret_set` rozetleri görünür.
- RouterOS API TLS verify alanı + SSH host key policy alanı (`insecure_ignore | trust_on_first_use | pinned`).
- Migration 000004: 3 yeni Mimosa tablosu (`mimosa_wireless_interfaces`, `mimosa_wireless_clients`, `mimosa_links`), credential profile SNMPv3 + TLS/SSH alanları, `device_credentials` priority+purpose+enabled.
- Devicectl vendor dispatch (Probe/Poll Mimosa için aktif), credential binding API (GET/PUT/DELETE /devices/{id}/credentials).
- Frontend: Cihazlar Mimosa Probe/Poll butonları, /cihazlar/[id] üzerinde CredentialPanel + Faz 4 banner, Ayarlar SNMPv3 + transport hardening form alanları.
- Dokümantasyon: `MIMOSA_READONLY_INTEGRATION.md`.

## Faz 3'te Yapılan

- MikroTik salt-okuma adapter'ı (API-SSL / SSH / SNMP) — segment-bazlı komut allowlist'i, sanitized error sınıflandırıcı, normalize tipler.
- Probe + Read-only Poll iş akışı (`internal/devicectl`).
- Telemetry persistence (`internal/telemetry`) — `device_poll_results`, `interface_metrics`, `mikrotik_wireless_clients/interfaces`, `telemetry_snapshots`, `wireless_metrics`.
- Yeni HTTP uçları: `/api/v1/devices/{id}/{probe,poll,telemetry/latest,wireless-clients/latest,interfaces/latest}` ve `/api/v1/mikrotik/poll-results`.
- Worker handler `mikrotik_readonly_poll` (concurrency + per-device timeout).
- Frontend: Cihazlar listesinde Probe/Poll butonları, `/cihazlar/[id]` detay sayfası (sağlık, arayüzler, istemciler, poll geçmişi), Faz 3 güvenlik banner'ı.
- Migration 000003 + capability matrix sözleşmesinin korunması (yazma bayrakları hard-locked false).
- Dokümantasyon: `MIKROTIK_READONLY_INTEGRATION.md`, `VAULT_ROTATION.md`.

## Faz 3'te Bilinçli Yapılmayan

- **Mimosa salt-okuma (Faz 4).**
- **Asynq/Redis bağlantısı (Faz 5).**
- **Zamanlanmış otomatik poll** — Faz 3'te manuel tetikleme.
- **AP→Client aktif testler (Faz 5).**
- **Frekans uygulama / config apply / rollback (Faz 9).**
- **TLS sertifikası doğrulaması** — RouterOS self-signed kullanımı yaygın olduğu için varsayılan kapalı.

## Güvenlik Uyarıları

- Komut allowlist'i bypass edilemez — segment-bazlı forbidden veto vardır.
- Hata mesajları `mikrotik.SanitizeError` ile maskelenir; ham parola/community/token loga düşmez.
- Probe başarılı olsa bile yazma bayrakları (`canApply*`, `canBackupConfig`, `canRollback`, `canRunScan`) FALSE kalır.
- `WISP_VAULT_KEY` rotasyon runbook'u: `docs/VAULT_ROTATION.md`.
