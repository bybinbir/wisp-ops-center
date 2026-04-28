# Phase 8 Operator Smoke Result

## Baseline

- **PR:** https://github.com/bybinbir/wisp-ops-center/pull/5
- **Head:** `68829601381746f60dbd940b8526e81f1e56d9b8` *(docs: add phase 8 operator smoke result)*
- **Commit zinciri (PR raporundaki):** `36a27a2` → `e7915d6` → `6882960` — HEAD ile uyumlu.
- **Date:** 2026-04-28
- **Environment:** Windows 11 lab makinesi. PostgreSQL 16.13 lokal (cluster: `C:\Program Files\PostgreSQL\16\data`, lokal `trust` auth, port 5432). Go 1.26.0, Node + pnpm + npm. MikroTik Dude SSH şifresi `.env`'e elle girilmedi (boş kaldı).
- **Operator:** Otonom akış (kullanıcı: malii / marskocas@gmail.com), prompt: WISP-P8-LAB-SMOKE-MERGE.
- **PR mergeable / mergeStateStatus:** `MERGEABLE` / `CLEAN`
- **Reviewer (engineering) raporu:** `docs/PHASE_008_PR5_REVIEW_SMOKE_RESULT.md` — READY *(engineering)*.

> Bu doküman gerçek lab kanıtlarıyla güncellendi. Engineering, migration ve API+DB integration kanıtlandı; canlı RouterOS/Dude smoke ise operatör elle parolayı `.env`'e koymadığı için **BLOCKED** kaldı. Sahte başarı kaydı yok.

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

## MikroTik Dude Read-Only Smoke

`MIKROTIK_DUDE_PASSWORD` operatör tarafından elle `.env`'e girilmedi (prompt kuralı: "Gerçek şifreyi yalnızca lokal .env içine operatör elle girecek"). Boş parolayla canlı SSH bağlantısı denenmedi; config-validation katmanı 412 ile geri çevirdi.

| Scenario | Result | Evidence |
|---|---:|---|
| `POST /test-connection` config validation | PASS (negative path) | HTTP 412 `{error:"not_configured", hint:"MIKROTIK_DUDE_HOST/USERNAME/PASSWORD eksik; .env değerlerini doldurup servisi yeniden başlatın."}`. Hiçbir SSH session açılmadı; hint sanitize. |
| `POST /test-connection` reachable=true | **BLOCKED — OPERATOR CREDENTIALS** | Operatör parolası girene kadar canlı SSH yapılamaz. |
| `POST /run` discovery | **BLOCKED — OPERATOR CREDENTIALS** | Aynı sebep; canlı discovery yapılmadı. |
| Terminal state success | **BLOCKED** | Live run yok. |
| Discovered device count | **BLOCKED** | Live run yok; hayali sayı uydurulmadı. |
| Category distribution | **BLOCKED** | Live run yok. |
| AP / CPE / BackhaulLink / Bridge / Router / Switch / Unknown count | **BLOCKED** | Live run yok. |
| `unknown=true` / `low_confidence=true` filtre semantiği | **BLOCKED** (HTTP/parametrik PASS) | Endpoint+filter HTTP 200 döner; live cihaz olmadığı için sonuç semantiği doğrulanamadı. |
| `audit_logs` event'leri (test_connection / discovery_run_started/finished) | **BLOCKED** | Live SSH denemesi yapılmadığı için audit yazılmadı (412 config-validation aşamasında dönüyor; tasarım gereği). |
| `network_devices.raw_metadata` redaction | **BLOCKED** | Live cihaz yok. Statik kanıt: `internal/dude/sanitize.go` ve `dude.SanitizeAttrs/SanitizeMessage` testleri (`internal/dude/sanitize_test.go`) yeşil. |
| Dedupe/upsert (mac > host+name > name) | **BLOCKED (in-memory PASS)** | `dude.dedupeDevices` `TestDedupe_MACWinsOverIP` PASS; DB tarafında `uq_netdev_source_mac` partial unique index doğrulandı. |
| Secret leakage check | PASS | API log dosyası (`.smoke_api.log`) `password|secret|token|fingerprint` aramasında 0 hit. `.env` `.gitignore`'da (line 20) — `git check-ignore -v .env` doğruladı. Bu dökümanda hiçbir gerçek şifre/host fingerprint yok. |

## Inventory Summary

Live discovery yapılmadı; operatör credentials gerek.

- Discovered device count: **N/A — BLOCKED** (live discovery yapılmadı; hayali sayı uydurulmadı)
- Category distribution: AP / CPE / BackhaulLink / Bridge / Router / Switch / Unknown — **N/A**
- Low confidence count: **N/A**
- Unknown count: **N/A**

## Safety Confirmation

- ✅ **No write/apply command executed.** Hiçbir RouterOS write komutu çalıştırılmadı; allowlist ihlali yok.
- ✅ **No frequency change executed.** Frequency apply Phase 8'de zaten kod yolu olarak yok; networkactions stub'lar `ErrActionNotImplemented` döner.
- ✅ **No bandwidth-test executed.** `/tool/bandwidth-test` allowlist'te yok; herhangi bir Exec çağrısı yapılmadı.
- ✅ **No reboot/reset/config mutation executed.** Hiçbir mutating komut allowlist'te yok; hiçbir live SSH session açılmadı (parola yok + 412 config-validation reddi).
- ✅ **No secret committed or logged.** `.env` repo dışı; `.gitignore` `.env`, `.env.local`, `.env.*.local` koruması var. `.smoke_api.log`'ta `password|secret|token|fingerprint` substring'ine karşı 0 hit. Bu dökümanda hiçbir gerçek şifre/host fingerprint geçmiyor.
- ✅ **Only read-only allowlisted commands (would be) executed.** Lab'da hiçbir live komut çalıştırılmadı; allowlist `internal/dude/allowlist.go` × 18 girişi `print`/`detail` ile bitiyor (test kanıtlı).

## Final Decision

- **Status:** **BLOCKED — OPERATOR CREDENTIALS REQUIRED.** (Önceki "ENVIRONMENT BLOCKED"tan ilerleme: PostgreSQL + API + DB integration artık PASS.)
- **Reason:** Engineering, migration ve API+DB integration tamamen yeşil; ancak prompt'taki **conditional merge** kapısı `POST /test-connection reachable=true` ve `POST /run terminal success` PASS'ı zorunlu kılıyor. Bunlar canlı MikroTik Dude SSH bağlantısı gerektirir. Operatör (kullanıcı) parolayı elle `.env`'e girmediği için canlı smoke yapılamadı. Sahte başarı raporu yazılmadı; PR otomatik merge **edilmedi**.
- **Engineering blocker:** Yok.
- **Environment blocker:** Yok (lokal PG16 + API + tüm Phase 8 endpoint'leri çalışıyor).
- **External blocker:** **Operatör eylemi.** `MIKROTIK_DUDE_PASSWORD` `.env`'e elle yazılmalı.
- **Out-of-scope blocker'lar (Faz 8 değil):** (1) Faz 7 `work_orders` şema çakışması (000007 vs 000001), (2) `scripts/db_migrate.sh` Windows psql URL-DSN parsing sorunu.

### Operatör için sonraki adım (canlı MikroTik smoke)

1. `F:\WispOps\wisp-ops-center\.env` dosyasını aç (mevcut, smoke harness oluşturdu, .gitignore korumasında).
2. `MIKROTIK_DUDE_PASSWORD=<lab_dude_parolasi>` satırını elle doldur (commit etme).
3. PG16 zaten ayakta; `wispops_smoke` DB'sinde Phase 8 tabloları hazır.
4. API'yi yeniden başlat: env değişkenleri inject ederek `./.smoke_api.exe` çalıştır (binary `F:\WispOps\wisp-ops-center\.smoke_api.exe` mevcut, build edildi).
5. `POST /api/v1/network/discovery/mikrotik-dude/test-connection` → `reachable=true` + identity gözle.
6. `POST /run` → 202 + run_id; `GET /api/v1/network/discovery/runs` ile terminal duruma gelmesini izle (`succeeded` veya `partial`).
7. `GET /api/v1/network/devices` ve filtreler (`?unknown=true`, `?low_confidence=true`) doğrula.
8. `audit_logs` tablosunda `test_connection`, `discovery_run_started`, `discovery_run_finished|failed` event'lerini doğrula; `network_devices.raw_metadata` JSONB içinde `password|secret|token|community|key|bearer|auth` substring'leri `[redacted]` olmalı.
9. Smoke yeşil → `gh pr merge 5 --repo bybinbir/wisp-ops-center --squash --delete-branch` ile main'e al.

Otonom oturum bu adımları yerine getirmeden PR'ı merge etmedi; prompt kuralı: **smoke PASS olmadan merge yok**.
