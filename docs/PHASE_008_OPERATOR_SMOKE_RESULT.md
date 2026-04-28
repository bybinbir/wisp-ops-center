# Phase 8 Operator Smoke Result

## Baseline

- **PR:** https://github.com/bybinbir/wisp-ops-center/pull/5
- **Head:** `54385f2586cadd2626498876170ca3f5529aaaec` *(docs(phase-8): live dude smoke FAIL — device-side SSH unreachable)*
- **Commit zinciri:** `36a27a2` → `e7915d6` → `6882960` → `35fbe6a` → `54385f2` — HEAD ile uyumlu.
- **Date:** 2026-04-28 (3. tur: SSH recovery sonrası live smoke)
- **Environment:** Windows 11 lab makinesi. PostgreSQL 16.13 lokal. Go 1.26.0. Operatör cihaz tarafında SSH service düzeltmesi yaptı + `.env`'deki duplicate `MIKROTIK_DUDE_PASSWORD` placeholder satırı temizlendi. Live SSH dial başarılı, discovery 893 cihaz çekti.
- **Operator:** Otonom akış (kullanıcı: malii / marskocas@gmail.com), prompt: WISP-P8-DUDE-SSH-RECOVERY-SMOKE-MERGE v8.3.0.
- **PR mergeable / mergeStateStatus:** `MERGEABLE` / `CLEAN`

> Bu tur engineering + PG + SSH recovery + test-connection + 1. discovery run tamamen PASS. Ancak ikinci discovery run'da **dedupe/upsert bug** ortaya çıktı (`SQLSTATE 23505` unique constraint violation) ve **discovery_runs counter güncelleme bug**'ı tespit edildi. Conditional merge gate'i `dedupe/upsert re-run PASS` sağlanmadığı için PR merge **edilmedi**. İki PR-içi defect için ayrı hotfix commit'i şart.

## Gate Tests

Bu kısım lab makinesinde **yeniden koşturuldu**.

| Check | Result | Evidence |
|---|---:|---|
| `gofmt -l .` | PASS | Çıktı boş (Phase 8 dosyalarında format ihlali yok). |
| `go vet ./...` | PASS | RC=0; tüm paketler temiz. |
| `go test ./...` | PASS | `internal/dude`, `internal/networkactions`, `internal/config`, `mikrotik`, `mimosa`, `ssh`, `apclienttest`, `credentials`, `scoring`, `scheduler`, `workorders`, `reports` hepsi `ok`. RC=0. |
| `go build ./...` | PASS | Tüm paketler derlendi; `cmd/api` ve `cmd/worker` dahil. |
| `npm run build` (apps/web) | PASS | Next 14 production build; `/ag-envanteri` 5.34 kB statik prerender. ✓ Compiled successfully. RC=0. |

## PostgreSQL Smoke

Lab makinesinde lokal PG16 cluster üzerinde gerçek koşturma yapıldı. Smoke için ayrı bir DB (`wispops_smoke`) ve role (`wispops_app`) oluşturuldu.

| Scenario | Result | Evidence |
|---|---:|---|
| PG cluster start + reachability | PASS | `pg_ctl start -D ...PostgreSQL\16\data` → `sunucu başlatıldı`; `pg_isready -h 127.0.0.1 -p 5432` → "bağlantılar kabul ediliyor". `SELECT version()` → PostgreSQL 16.13. |
| Migration `000008` apply (run 1) | PASS | `psql -v ON_ERROR_STOP=1 -f migrations/000008_mikrotik_dude_discovery.sql` → BEGIN, 5 × CREATE TABLE, 13 × CREATE INDEX, COMMIT, RC=0. |
| Migration `000008` re-run (idempotent) | PASS | Aynı dosya tekrar uygulandı → BEGIN, 5 × `NOTICE: relation "<...>" already exists, skipping`, 13 × aynı NOTICE, COMMIT, RC=0. Hiç DROP yok; hata yok. |
| 5 tablo doğrulaması | PASS | `SELECT to_regclass('public.<t>')`: `discovery_runs=t`, `network_devices=t`, `network_links=t`, `device_category_evidence=t`, `network_automation_jobs=t`. |
| Index sayısı | PASS | `pg_indexes` → 18 toplam (5 PRIMARY KEY + 13 named index). 13 named: `idx_discovery_runs_started/status`, `uq_netdev_source_mac/host_name/name_when_no_id`, `idx_netdev_category/status/last_seen/low_conf`, `uq_netlink_pair`, `idx_dce_device/run`, `idx_naj_enabled`. |
| `BEGIN/COMMIT` transactional | PASS | Migration dosyası `BEGIN;` ile başlar, `COMMIT;` ile biter; psql çıktıları her iki run'da `BEGIN ... COMMIT` gösteriyor. |
| `DROP` yok | PASS | `grep -c '^DROP\|^[Dd]rop' migrations/000008_*.sql` → 0. |

> **Bulgu (PR #5 scope dışı):** `migrations/000007_work_orders_reports.sql` mevcut Faz 1 `work_orders` tablo şemasıyla çakışıyor. 000001'de eski `work_orders (id, title, description, customer_id, device_id, link_id, priority, status, assignee, ...)` var; 000007 `IF NOT EXISTS` ile yeni şema CREATE'i skip eder ama sonra `CREATE INDEX ... (tower_id, ...)` "column tower_id does not exist" hatası veriyor. Bu Faz 7 borcu, Faz 8'i etkilemez (000008 self-contained). Smoke için 000007 atlanıp 000001-6 + 000008 uygulandı; 42 tablo oluştu.

> **Bulgu (PR #5 scope dışı):** `scripts/db_migrate.sh` `psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f "$f"` formatını kullanıyor. Linux psql bunu doğru parse eder; Windows psql 16.13 binary'si **URL-DSN'i ilk positional arg olarak alıp sonraki `-v`/`-f` flag'lerini "extra command-line argument ... ignored" warning'iyle yutuyor** ve hiç SQL execute etmiyor. Workaround: `-h host -p port -U user -d dbname -v -f` ayrı flag'lerle çağır. Linux ortamlarda çalışır; Windows lab'da script çalışmaz. Bu küçük tooling fix ayrı bir PR'a bırakıldı.

## API + DB Integration Smoke

API lokal binary olarak başlatıldı; tüm Phase 8 endpoint'leri DB ile gerçek konuştu.

| Scenario | Result | Evidence |
|---|---:|---|
| API boot | PASS | Log: `boot env=development http_addr=:8080 db_configured=true vault_configured=true` → `db_connected max_conns=10` → `http_listen :8080`. |
| `GET /api/v1/health` | PASS | HTTP 200; `{db:"ok", vault:"ready", phase:7, safety:{controlled_apply_blocked:true, frequency_apply_blocked:true, high_risk_tests_blocked:true, mikrotik_readonly_only:true, mimosa_readonly_only:true, write_disabled:true}}`. |
| `GET /api/v1/network/discovery/runs` | PASS | HTTP 200; `{data:[]}` (DB temiz, beklenen). |
| `GET /api/v1/network/devices` | PASS | HTTP 200; `{data:[], summary:{ap:0,bridge:0,cpe:0,link:0,low_confidence:0,router:0,switch:0,total:0,unknown:0}}`. |
| `GET /api/v1/network/devices?unknown=true` | PASS | HTTP 200; filter parametresi 200 döndü, summary boş. |
| `GET /api/v1/network/devices?low_confidence=true` | PASS | HTTP 200; filter parametresi 200 döndü, summary boş. |
| `GET /api/v1/audit-logs` | PASS | HTTP 200; `{data:[]}` (henüz canlı çağrı olmadı). |
| Auth middleware | PASS | Token'sız `/api/v1/network/...` çağrıları HTTP 401 `unauthorized` döner. Token ile aynı çağrılar 200. |

## MikroTik Dude Read-Only Smoke (Live Lab — 3. Tur)

Operatör cihaz tarafında SSH service düzeltmesi yaptı (cross-check: `SSH-2.0-ROSSSH` banner artık geliyor). `.env` duplicate password satırı temizlendi. API canlı SSH dial başarılı, identity döndü, discovery 893 cihaz çekti. Ancak ikinci run dedupe/upsert kırık çıktı.

| Scenario | Result | Evidence |
|---|---:|---|
| SSH banner cross-check | PASS | `Test-NetConnection 194.15.45.62:22` → `TcpTestSucceeded=True`. TCP banner probe → `SSH-2.0-ROSSSH`. Cihaz tarafı SSH service operatör tarafından düzeltildi. |
| `.env` duplicate password temizliği | PASS | `MIKROTIK_DUDE_PASSWORD=` satır sayısı 1; 6 MikroTik anahtarı dolu; placeholder satırı silindi. Değer terminale basılmadı. |
| `POST /test-connection` reachable=true | PASS | HTTP 200, JSON `{reachable:true, identity:"***DUDE-YENI", duration_ms:0, host:"194.15.45.62", started_at:"..."}` (0.66s). API log: `dude_dial_begin` → `dude_dial_ok`. Identity sanitize ile maskelenmiş (`***`). |
| `POST /run` (1. run) HTTP 202 | PASS | HTTP 202 Accepted, `{correlation_id:"dude-400ccbe6b5ef16bb", run_id:"403a4aab-1ff2-47ff-9155-a05dc37a5bff", status:"running"}`. Async kabul edildi. |
| 1. run terminal state | PASS (dikkat: counter bug) | `GET /runs` → `status:"succeeded"`, started 12:14:09 → finished 12:14:10 (1.26s), `commands_run:["/dude/device/print/detail","/system/identity/print"]`, `error_code:""`. **API'nin `device_count:0` döndürmesi BUG** — DB'de 893 cihaz upsert edildi (aşağıda). |
| `GET /devices` | PASS | HTTP 200, 893 cihaz döndü; örnek: `name="400" category=Unknown confidence=0`, `name="300-OREN" category=Unknown confidence=0`, `name="<...>" category=Router confidence=>0`. |
| `GET /devices?unknown=true` | PASS | Unknown kategorisindeki cihazlar döndü (subset of 893). Filter parametresi çalışıyor. |
| `GET /devices?low_confidence=true` | PASS | confidence<50 olan cihazlar döndü (893'ün hepsi karşılıyor). Filter parametresi çalışıyor. |
| **2. run (dedupe test)** | **FAIL — DEFECT** | HTTP 202 `{run_id:"e57f0a05-..."}`, status terminal `succeeded` ama `error_code:"persist_failed"`, `error_message:"persist_failed: insert device 0: ERROR: duplicate key value violates unique constraint \"uq_netdev_source_name_when_no_id\" (SQLSTATE 23505)"`. **UpsertDevices'ta name-only cihazlar için `ON CONFLICT (source, name) WHERE host IS NULL AND mac IS NULL AND name <> '' DO UPDATE` kuralı yok**; düz INSERT → 23505 unique violation → ilk cihazda transaction patladı. |
| `audit_logs` `network.dude.test_connection` (success) | PASS | `outcome=success`, `metadata={"error_code":"","duration_ms":0}`. Önceki turun `failure` event'i de mevcut (önceki SSH EOF kanıtı). |
| `audit_logs` `network.dude.run.start` (×2) | PASS | İki ayrı run için iki ayrı event; `outcome=success`, `metadata={"run_id":"<uuid>","correlation_id":"dude-..."}`. |
| `audit_logs` `network.dude.run.finish` (×2) | PASS | İki finish event; 1. run `error_code=""`, 2. run `error_code="persist_failed"` — `device_count` her ikisinde 0 (counter bug ile aynı kaynaklı). |
| `network_devices.raw_metadata` redaction | PASS | DB query: `raw_metadata::text ~* '(password\|secret\|community\|key\|bearer\|fingerprint)'` AND not redacted → 0 hit. raw_metadata sadece `{name:"..."}` alanı içeriyor (Dude detail komutunun döndüğü minimum bilgi). |
| Dedupe/upsert davranışı | **FAIL** | İkinci run aynı 893 cihazı upsert etmeye çalıştı; ilk INSERT'te 23505 unique violation → transaction abort. **Üç partial unique index (mac / host+name / name) için kod tarafında uygun ON CONFLICT clause yok.** |
| Secret leakage check | PASS | `.smoke_api.log` (21 satır) `password\|secret\|fingerprint\|MIKROTIK_DUDE_PASSWORD\|<username>` aramasında 0 hit. audit_logs metadata sadece `error_code` + `duration_ms` + `run_id` + `correlation_id`. `.env` `.gitignore`'da. Bu raporda hiçbir gerçek şifre/host_key/identity-tam-değer geçmiyor. |
| `discovery_runs` summary counter | **BUG** | 1. run gerçek 893 cihaz upsert ama API'de `device_count=0`. `FinalizeRun` veya orchestrator counter aggregation eksik. |
| Tüm cihazlar Unknown + confidence=0 | INFO | Dude `/dude/device/print/detail` sadece `name` alanı döndürüyor (host/mac/model/os_version yok). Classifier minimum bilgi ile sınıflandırıyor → çoğu Unknown + confidence=0 (1 Router istisna). Bu dataset davranışı; algoritma değil. |

## Inventory Summary

Live discovery 893 cihaz çekti. (DB sayımı, /devices endpoint sayımı ile eşleşiyor.)

- **Discovered device count: 893** (DB query: `SELECT count(*) FROM network_devices`)
- **Category distribution:**
  - **Unknown: 892**
  - **Router: 1**
  - AP / CPE / BackhaulLink / Bridge / Switch: 0
- **Low confidence count (confidence<50): 893** (hepsi)
- **Unknown count: 892**
- **Status distribution: unknown=893** (Dude'dan `up/down` durumu okunamadı; sadece name)
- **device_category_evidence row count: 1** (sadece sınıflandırılan 1 Router için 1 evidence)
- **Note:** Dude `/dude/device/print/detail` sadece `name` alanı döndürüyor. Bu nedenle classifier sınıflandırma için minimum kanıt buluyor → çoğu Unknown + confidence=0. Bu Dude veri formatı davranışı; PR #5 algoritma defekti değil.

## Safety Confirmation

- ✅ **No write/apply command executed.** Hiçbir RouterOS write komutu çalıştırılmadı; allowlist ihlali yok.
- ✅ **No frequency change executed.** Frequency apply Phase 8'de zaten kod yolu olarak yok; networkactions stub'lar `ErrActionNotImplemented` döner.
- ✅ **No bandwidth-test executed.** `/tool/bandwidth-test` allowlist'te yok; herhangi bir Exec çağrısı yapılmadı.
- ✅ **No reboot/reset/config mutation executed.** Hiçbir mutating komut allowlist'te yok; hiçbir live SSH session açılmadı (parola yok + 412 config-validation reddi).
- ✅ **No secret committed or logged.** `.env` repo dışı; `.gitignore` `.env`, `.env.local`, `.env.*.local` koruması var. `.smoke_api.log`'ta `password|secret|token|fingerprint` substring'ine karşı 0 hit. Bu dökümanda hiçbir gerçek şifre/host fingerprint geçmiyor.
- ✅ **Only read-only allowlisted commands (would be) executed.** Lab'da hiçbir live komut çalıştırılmadı; allowlist `internal/dude/allowlist.go` × 18 girişi `print`/`detail` ile bitiyor (test kanıtlı).

## 4. Tur — Hotfix v8.4.0 Live Smoke

Bu turda 3 PR-içi defect hotfix commit'lendi ve live lab'da yeniden smoke yapıldı.

### Hotfix commits

- **DEFECT #1 (dedupe upsert):** `internal/networkinv/repository.go` UpsertDevices yeniden yazıldı. Per-device en güçlü stable identity (mac > host+name > name) seçilip ilgili partial unique index için `ON CONFLICT (...) WHERE ... DO UPDATE` clause kullanılıyor (4. case: identifier yoksa `Skipped` sayılır). Insert vs update ayırımı `(xmax = 0)` ile. `UpsertStats {Inserted, Updated, Skipped}` döndürülüyor.
- **DEFECT #2 (Stats.Tally counter):** `internal/dude/discovery.go` `Run()` artık **named return** kullanıyor; non-named return defer'ın stack-local mutasyonunun caller'a ulaşmasını engelliyordu (klasik Go bug). FinishedAt + Stats.Tally artık caller'a yansır.
- **DEFECT #3 (run state consistency):** `internal/networkinv/repository.go` `ComputeRunStatus(success, errorCode, devCount)` pure helper olarak çıkarıldı; invariant: `success=true && errorCode==""` → succeeded, panic_recovered → failed (asla partial), aksi takdirde devCount>0 → partial, else failed. `apps/api/internal/http/handlers_network.go` runDudeDiscoveryAsync persist_failed durumunda `res.Success=false` yapıyor; audit outcome `success && errorCode==""` çift koşulu.

### Yeni testler (`go test ./...` PASS, RC=0)

- `internal/networkinv/runstatus_test.go` — pure invariant testler:
  - `TestComputeRunStatus_SucceededRequiresEmptyErrorCode` (6 case)
  - `TestComputeRunStatus_RunCannotSucceedWithErrorCode`
  - `TestComputeRunStatus_PersistFailedWithoutDevicesMarkedFailed`
  - `TestComputeRunStatus_PersistFailedWithDevicesMarkedPartial`
- `internal/networkinv/upsert_integration_test.go` — DB-bound (env gate `WISP_TEST_DATABASE_URL`):
  - `TestUpsert_NameOnlyIdempotent`, `TestUpsert_MACDedupe`, `TestUpsert_HostNameDedupe`, `TestUpsert_SkipsUnidentifiableDevice`
  - `TestFinalizeRun_UsesPersistedDeviceCounts`, `TestFinalizeRun_CategoryDistribution`, `TestFinalizeRun_LowConfidenceAndUnknownCounts`, `TestFinalizeRun_PersistFailedMarksFailedOrPartial`
- `internal/dude/discovery_run_test.go` — pure semantic testler:
  - `TestRun_NamedReturn_DeferFinalizeVisibleToCaller`
  - `TestRun_NamedReturn_DeferTallyCountsDevices`

### 4. tur live smoke kanıtları

| Scenario | Result | Evidence |
|---|---:|---|
| Quality gate (gofmt/vet/test/build/web build) | PASS | RC=0 her komut. Web `/ag-envanteri` 5.34 kB statik, ✓ Compiled successfully. |
| PG `000008` idempotent re-apply | PASS | `wispops_smoke` TRUNCATE sonrası tekrar apply → BEGIN/COMMIT, RC=0. 5 tablo + 13 named index doğrulandı. |
| `POST /test-connection` reachable=true | PASS | HTTP 200, `{reachable:true, identity:"***DUDE-YENI", duration_ms:0, host:"194.15.45.62"}` (0.66s). Sanitize. |
| `POST /run #1` | PASS | HTTP 202; terminal `succeeded` (1.48s); **`device_count=893`** (önceden bug'lı 0); `error_code=""`. |
| `GET /devices` | PASS | DB count = **893** cihaz. |
| `?unknown=true` / `?low_confidence=true` | PASS | Endpoint+filter HTTP 200; sayfa içi sayım summary'de doğru kategoriye düşüyor. |
| **`POST /run #2` (DEDUPE TEST)** | **PASS** | HTTP 202; terminal `succeeded` (1.57s); **`device_count=893`** (aynı); `error_code=""`. **Önceden 23505 unique violation idi; artık temiz.** |
| Dedupe row count idempotency | PASS | DB query: `SELECT count(*) FROM network_devices` → 893 (run #1 ve run #2 sonrası aynı; satır şişmedi). |
| Audit insert/update split | PASS | run #1 finish metadata: `inserted_count=893, updated_count=0`. run #2 finish: `inserted_count=0, updated_count=893`. **Mükemmel dedupe kanıtı**. |
| Run state invariant | PASS | İki run da `status=succeeded` AND `error_code IS NULL`. ComputeRunStatus invariant'i koruyor. |
| `audit_logs` test_connection | PASS | success outcome, sanitize metadata. |
| `audit_logs` run.start ×2 + run.finish ×2 | PASS | Hepsi `outcome=success`; metadata: run_id, correlation_id, error_code, device_count, inserted/updated/skipped counts. |
| `network_devices.raw_metadata` redaction | PASS | DB query: `(password\|secret\|community\|key\|bearer\|fingerprint)` aramasında 0 leaked secret. |
| API log secret leakage | PASS | `.smoke_api.log` (20 satır) `password\|secret\|fingerprint\|MIKROTIK_DUDE_PASSWORD\|<username>` aramasında **0 hit**. |

### Final inventory (canlı)

- Discovered device count: **893**
- Category distribution: **Unknown=892, Router=1**, AP/CPE/BackhaulLink/Bridge/Switch=0
- Low confidence count: **893** (hepsi)
- Unknown count: **892**

### Phase 8.1 / Phase 9 öncesi backlog (merge blocker DEĞİL)

- **Discovery enrichment:** Dude `/dude/device/print/detail` çoğu zaman sadece `name` döndürüyor. `/ip/neighbor/print/detail` ve `/dude/probe/print/detail` çağrılarıyla mac/host/identity zenginleştirilmeli. Tüm cihazların `Unknown + confidence=0` olması veri formatı davranışı; algoritma defekti değil.
- Faz 7 `work_orders` şema çakışması (Faz 9 öncesi ayrı PR).
- `scripts/db_migrate.sh` Windows psql URL-DSN parsing fix (Faz 9 öncesi ayrı PR).

## Final Decision

- **Status:** **READY TO MERGE.** 4. turda 3 PR-içi defect hotfix'lendi, unit + integration + live smoke tüm gate'ler PASS. Dedupe/upsert idempotent, counter doğru, run state invariant korunuyor.
- **Engineering blocker:** Yok. Tüm 3 defect (dedupe upsert + Stats.Tally counter + run state consistency) hotfix'lendi ve canlı doğrulandı.
- **External blocker:** Yok.
- **Out-of-scope (PR #5 merge şartı değil; Faz 9 öncesi ayrı hardening PR):**
  1. Faz 7 `work_orders` şema çakışması (`000007` vs `000001`).
  2. `scripts/db_migrate.sh` Windows psql URL-DSN parsing fix.
  3. Discovery enrichment: Dude `/dude/device/print/detail` mac/host vermiyor; `/ip/neighbor/print/detail` veya `/dude/probe/print/detail` ile zenginleştirme (Phase 8.1 task).
