# Phase 8 PR #5 Review & Smoke Result

## Baseline

- **PR:** https://github.com/bybinbir/wisp-ops-center/pull/5
- **Base:** `main` @ `95e94b6ce2e47200fb7cb54e7ac30fd2030d1016`
- **Head (review başlangıcı):** `771a3e434805e1903f40f68ceb8e1bc04da627aa`
- **Reviewed commit (after hardening fixes):** `e7915d6` *(Phase 8 PR review fix: panic recovery + status/UUID validation)*
- **Date:** 2026-04-28
- **Operator:** Claude Opus 4.7 (1M context) çalışan otonom review (kullanıcı: malii / marskocas@gmail.com)
- **PR mergeStateStatus / mergeable:** `CLEAN` / `MERGEABLE`
- **Changed files:** 34, additions 3668, deletions 17

## Merge Readiness

- **Status:** READY *(engineering)* — kod & gate'ler temiz; smoke ENVIRONMENT BLOCKED.
- **Reason:** Tüm gate testleri (gofmt, vet, test, build, next build) yeşil. 3 küçük hardening fix uygulandı (panic recovery, status validation, UUID shape check). PostgreSQL E2E ve live MikroTik Dude smoke için bu ortamda araç yok (psql/pg_isready bulunamadı, `.env` mevcut değil) — sandbox dışı; bu nedenle smoke kanıtı **ENVIRONMENT BLOCKER**. Phase 7'deki PR #3 ile aynı sınıflandırma.

## Gate Tests

| Check | Result | Evidence |
|---|---:|---|
| gofmt -l . | PASS | Çıktı boş (Phase 8 dosyalarında format ihlali yok). |
| go vet ./... | PASS | Çıktı boş; tüm paketler temiz. |
| go test ./... | PASS | `internal/dude`, `internal/networkactions`, `internal/config`, ve mevcut paketler (mikrotik, mimosa, ssh, apclienttest, credentials, scoring, scheduler, workorders, reports) `ok`. Test sayısı: parser 5, classify 6, sanitize 4, allowlist 3, discovery 4, networkactions 4, config 3. |
| go build ./... | PASS | Tüm paketler derlendi; cmd/api ve cmd/worker dahil. |
| npm run build | PASS | Next 14 production build; `/ag-envanteri` 5.34 kB statik prerender. ✓ Compiled successfully. |

## Security Review

| Area | Result | Notes |
|---|---:|---|
| SSH secrets in repo | PASS | `.env.example` içinde `MIKROTIK_DUDE_PASSWORD=` boş; gerçek şifre branch ağacında bulunmuyor. `.gitignore` `.env`, `.env.local`, `.secrets`, `*.pem`, `*.key` korur. `git log -p -- .env.example` boş şifre bırakıldığını doğrular. |
| Host-key policy default | PASS | `internal/config.DudeConfig.HostKeyPolicy` env'den `firstNonEmpty(..., "trust_on_first_use")`. `dude.Client.Dial` boş policy gelirse `wispssh.PolicyTOFU` uygular; tanımsız policy `ErrHostKey` döndürür. `insecure_ignore` **default değil**. |
| Allowlist (read-only) | PASS | `internal/dude/allowlist.go` 18 girişten oluşur, hepsi `/print` veya `/print/detail` ile biter. `TestAllowlist_NoDestructiveCommands` her girişin `print|detail` terminal segmentine sahip olduğunu doğrular (false-positive `address`/`add` koruması). `EnsureAllowed` exact-match; injection yüzeyi yok. |
| Sanitization (logs/raw_metadata) | PASS | `dude.SanitizeAttrs` `password|passwd|secret|community|key|token|bearer|auth` substring eşleşmelerini `[redacted]` ile değiştirir; orijinal map mutate edilmez (`TestSanitizeAttrs_RedactsSecretLikeKeys`). `dude.SanitizeMessage` `password=`/`passwd=`/`secret=`/`token=` öncesini keser ve >320 byte mesajları kapar. Tüm error log/UI çıkışları sanitize'den geçer. |
| API auth | PASS (with caveat) | `apps/api/internal/http/server.go` middleware `Authorization: Bearer <WISP_API_TOKEN>` zorunlu kılar; `/api/v1/health` ve `/` istisna. Token boşsa middleware bypass — bu Faz 8'in yeni davranışı değil, mevcut kontrat (Faz 1'den beri). Production hardening Faz 10'a ait. **Bütün Faz 8 endpoint'leri** middleware'den geçer (routes.go). |
| Audit / correlation_id | PASS | 3 audit eventi: `network.dude.test_connection`, `network.dude.run.start`, `network.dude.run.finish`. Her biri `Subject=cfg.Host`, `Metadata={run_id, correlation_id, error_code, ...}`. correlation_id `dude.NewCorrelationID()` (8-byte crypto/rand → "dude-<hex>"); SSH client log'larına `c.correlationID` etiketi olarak iliştirilir; `discovery_runs.correlation_id` aynı id'yi taşır. End-to-end korunuyor. |
| No destructive ops | PASS | `Client.Exec` yalnızca `EnsureAllowed`'tan geçen komutları çalıştırır. Ayrıca: networkactions `Action.Execute` her Kind için `ErrActionNotImplemented` döner; `KindFrequencyCorrection.IsDestructive() == true` flag'i mevcut ama Faz 8'de wire edilmiş tek bir destructive yol yok. Migration `network_automation_jobs.job_type CHECK = 'discovery'`. |

### Diğer review notları (non-blocking)

- **Goroutine recovery (FIXED in `e7915d6`)**: `runDudeDiscoveryAsync` artık `defer func(){ recover() ... }()` ile panic emniyetinde; recovered run satırı `failed` + `error_code=panic_recovered` olarak finalize edilir.
- **Filter validation (FIXED in `e7915d6`)**: `?status=` `validNetworkStatuses` map'i ile doğrulanır; geçersiz değerler 400 döner.
- **UUID shape check (FIXED in `e7915d6`)**: `handleNetworkDeviceItem` `looksLikeUUID(id)` ile doğrular, 500 yerine 400 döner.
- **Race window — Find→Insert**: `UpsertDevices` tek tx içinde önce SELECT sonra INSERT yapıyor. İki paralel run aynı yeni cihazı upsert ederse partial unique index (`uq_netdev_source_mac` veya `uq_netdev_source_host_name`) ihlali olur. In-process `dudeRunMu` "tek koşu" garantisi bu race'i etkili biçimde önlüyor; çoklu API replica için `INSERT ... ON CONFLICT DO UPDATE` Faz 9 hardening'e bırakıldı (limit kabul edilebilir).
- **Auth-no-token bypass**: `WISP_API_TOKEN=""` ise tüm endpoint'ler açık. Bu PR'a özel değil, Phase 1'den kalma kontrat. Production deployment runbook'unda zorunlu token. README'de işaretli.
- **Goroutine timer leak (Exec)**: `time.After(c.timeout)` kullanımı her Exec çağrısında yeni timer yaratır; allowlistteki 18 komut Run başına en fazla 3 kez çalıştırılır, leak küçüktür ve goroutine + session kapanışıyla GC tarafından toplanır. NIT.

## PostgreSQL Smoke

| Scenario | Result | Evidence |
|---|---:|---|
| Migration apply | BLOCKED | Lokal sandbox'ta `psql` / `pg_isready` yok; PG bağlantısı denenemez. |
| Idempotent re-run | BLOCKED | Aynı sebep. Migration kodu inceleme: `CREATE TABLE IF NOT EXISTS` × 5, `CREATE [UNIQUE] INDEX IF NOT EXISTS` × 12, BEGIN/COMMIT, DROP yok — yapısal olarak idempotent. |
| Run lifecycle | BLOCKED | Repository unit-level: `CreateRun` INSERT + RETURNING; `FinalizeRun` UPDATE status (succeeded/partial/failed) + counters + commands_run + error_code/message; `ListRuns` 50 default limit. PR'da goroutine içinde `defer FinalizeRun` zaten. |
| Device upsert | BLOCKED | Repo katmanı (mac > host+name > name) önceliği + tek tx içinde JSONB raw_metadata persist + per-device evidence refresh. Migration'daki 3 partial unique index DB tarafında duplicate'i geri çevirir. |
| MAC dedupe | BLOCKED (in-memory PASS) | `dude.dedupeDevices` MAC > IP > Name önceliği `TestDedupe_MACWinsOverIP` ile doğrulandı; daha yüksek confidence sınıflandırması korunur. DB tarafında `uq_netdev_source_mac` partial unique index var. |
| Evidence refresh | BLOCKED | `UpsertDevices` her cihaz için `DELETE FROM device_category_evidence WHERE device_id=$1 AND run_id=$2` ardından her Evidence için INSERT — tek tx içinde, atomik. |
| Filters | BLOCKED (parametrik PASS) | `ListDevices(filter)` parametrik `WHERE` (kategori, status, low_confidence, unknown) + limit/offset; tüm parametreler `$N` olarak verilir, SQL injection yok. Status hardening fix'iyle 400 döner. |

**Smoke sınıflandırması:** ENVIRONMENT BLOCKED (Phase 7 PR #3 ile aynı sebep — sandbox dışı PG yok).

## Live MikroTik Dude Read-Only Smoke

| Scenario | Result | Evidence |
|---|---:|---|
| Test connection | BLOCKED | `.env` mevcut değil; `MIKROTIK_DUDE_PASSWORD` yok. `Configured()` → false → API 412 not_configured döner (bu codepath'i `TestLoad_DudeNotConfigured` doğrular). |
| Discovery run | BLOCKED | Aynı sebep. |
| Runs endpoint | BLOCKED | DB'siz çalışmaz. |
| Devices endpoint | BLOCKED | DB'siz çalışmaz. |
| Category distribution | BLOCKED | Live data yok; in-memory `TestDevicesFromDudePrint_ClassifiesAndDedupes` 3 cihazlık tipik bir Dude çıktısını AP/CPE/BackhaulLink olarak doğru sınıflandırır. |
| Unknown/low confidence filters | BLOCKED | Repo katmanı parametrik filtreleri `e7915d6` sonrası 400 ile validate eder; live veri olmadan uçtan uca sayım yapılamadı. |
| Secret leakage check | PASS (statik) | Bütün error/log/audit yolları sanitize'den geçer. `git grep` ile şifre/host fingerprint repo ağacında bulunmuyor. `.env` repoda yok. |

**Smoke sınıflandırması:** ENVIRONMENT BLOCKED (kullanıcı tarafından sağlanan şifre yok). Operatör manuel olarak `.env`'i doldurup `POST /api/v1/network/discovery/mikrotik-dude/test-connection` çalıştırarak kabul kriterini doğrulayabilir.

## Findings

| Severity | File/Area | Finding | Required Fix |
|---|---|---|---|
| WARNING (FIXED) | `apps/api/internal/http/handlers_network.go::runDudeDiscoveryAsync` | Goroutine'de panic recovery yoktu; bir panic API process'ini çökertirdi. | `defer recover()` eklendi; recovered run `failed`+`panic_recovered` olarak finalize ediliyor. — `e7915d6` |
| WARNING (FIXED) | `apps/api/internal/http/handlers_network.go::handleNetworkDevices` | `?status=` validate edilmiyordu; geçersiz değerler sessizce boş liste döndürürdü (UX bug). | `validNetworkStatuses` map'i ile 400 invalid_status. — `e7915d6` |
| NIT (FIXED) | `apps/api/internal/http/handlers_network.go::handleNetworkDeviceItem` | UUID shape check yoktu; bozuk id PG query layer'a gidip 500 döndürüyordu. | `looksLikeUUID()` helper, malformed → 400 invalid_id. — `e7915d6` |
| NIT (kabul) | `internal/dude/client.go::Exec` | `time.After(c.timeout)` her çağrıda yeni timer yaratır; küçük leak. | Run başına ≤3 komut; GC ile toplanır. Faz 9 hardening'inde `time.NewTimer` + `defer t.Stop()`. |
| NIT (kabul) | `internal/networkinv/repository.go::UpsertDevices` | Find→Insert race window'u var; tek tx içinde ama paralel iki run partial unique violation alabilir. | In-process `dudeRunMu` mevcutta tek-koşu garantisi. Çoklu replica için `INSERT ... ON CONFLICT DO UPDATE` Faz 9'a bırakıldı. |
| NIT (kabul) | `apps/api/internal/http/server.go::middleware` | `WISP_API_TOKEN=""` tüm rotaları açık bırakır. | Phase 1'den beri mevcut kontrat; production runbook bunu zorunlu kılar. Faz 10'a ait. |
| NIT (kabul) | `internal/networkinv/repository.go` | JSON unmarshal hatası ignore (`_ = json.Unmarshal(raw, &d.RawMetadata)`). | raw_metadata bizim sanitize'den geçen veridir; bozulma riski düşük. Loglama hardening'i ileri faz. |

**BLOCKER bulgusu yok.** 3 WARNING/NIT bu PR'da düzeltildi (`e7915d6`); kalan 4 NIT bilinçli olarak kabul edildi ve kapsam dışı/ileri faz olarak belgelendi.

## Safety Confirmation

- ✅ No write/apply command executed.
- ✅ No bandwidth-test executed.
- ✅ No frequency change executed.
- ✅ No reboot/reset/config mutation executed.
- ✅ No secret committed or logged.
- ✅ Only read-only allowlisted commands (allowlist.go × 18 entries × `print`/`detail` terminal) — bu review sırasında hiçbir live komut çalıştırılmadı (smoke environment-blocked).
- ✅ `git diff main..e7915d6 -- '*.env*' '*.pem' '*.key' '*.secret*'` boş.
- ✅ Audit eventleri sadece read-only fiiller (`test_connection`, `run.start`, `run.finish`).

## Final Decision

- **Status:** READY TO MERGE (engineering).
- **Smoke kanıtı:** **BLOCKED — ENVIRONMENT** (lokal PG yok, `.env` yok). Sınıflandırma Phase 7 PR #3'le aynı.
- **Recommendation:** Operatör (malii) lab Postgres + MikroTik Dude erişimi olan ortamda manuel olarak şu adımları yürütmeli: (1) migration 000008 apply, (2) `.env` doldur, (3) UI üzerinden "Bağlantıyı Test Et" → identity dönmeli, (4) "Discovery Çalıştır" → run terminal duruma gelmeli, cihaz tablosu dolmalı, (5) `?unknown=true` ve `?low_confidence=true` filtreleri çalışmalı. Smoke sonucu yeşilse PR #5 merge edilebilir; sonra Faz 9'a geçilir.
- **Engineering blocker:** Yok. Code, tests, docs ve PR description merge için hazır.
- **Environmental blocker:** Sandbox sınırı; üretim ortamında çözülecek.
- **External blocker:** Yok.
