# Phase 8 Operator Smoke Result

## Baseline

- **PR:** https://github.com/bybinbir/wisp-ops-center/pull/5
- **Head:** `35fbe6ab5a4ccaf64b62716ca0e5c6ff16c4fc84` *(docs(phase-8): refresh operator smoke result with real lab evidence)*
- **Commit zinciri:** `36a27a2` → `e7915d6` → `6882960` → `35fbe6a` — HEAD ile uyumlu.
- **Date:** 2026-04-28 (live MikroTik smoke turu)
- **Environment:** Windows 11 lab makinesi. PostgreSQL 16.13 lokal (cluster: `C:\Program Files\PostgreSQL\16\data`, lokal `trust` auth, port 5432). Go 1.26.0, Node + pnpm + npm. Operatör `MIKROTIK_DUDE_PASSWORD` değerini lokal `.env`'e elle koydu; live SSH dial denendi.
- **Operator:** Otonom akış (kullanıcı: malii / marskocas@gmail.com), prompt: WISP-P8-LIVE-DUDE-SMOKE-MERGE v8.2.1.
- **PR mergeable / mergeStateStatus:** `MERGEABLE` / `CLEAN`
- **Reviewer (engineering) raporu:** `docs/PHASE_008_PR5_REVIEW_SMOKE_RESULT.md` — READY *(engineering)*.

> Bu doküman gerçek lab kanıtlarıyla güncellendi. Engineering, migration, API+DB integration ve config validation tamamen yeşil. Live MikroTik smoke FAIL: hedef cihaz SSH banner göndermiyor (handshake EOF). Sahte başarı kaydı yok; PR merge **edilmedi**.

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

## MikroTik Dude Read-Only Smoke (Live Lab Attempt)

Operatör `MIKROTIK_DUDE_PASSWORD` değerini lokal `.env`'e elle koydu. API canlı SSH dial denedi; **hedef cihaz SSH banner göndermeden bağlantıyı kapattı** (handshake EOF). Cihaz tarafında SSH server problemi (devre dışı / brute-force koruma / port forwarding farklı servise) — kod tarafı temiz davrandı ve sanitize hata raporladı.

| Scenario | Result | Evidence |
|---|---:|---|
| `POST /test-connection` HTTP cevabı | PASS (HTTP layer) | HTTP 200, JSON `{reachable:false, error_code:"unreachable", error:"dude: device_unreachable", duration_ms:0, host:"194.15.45.62", started_at:"..."}`. Sanitize: parola/secret yok, sadece host (zaten kullanıcı tarafından bildirilen). |
| `POST /test-connection` reachable=true | **FAIL — SSH HANDSHAKE EOF** | API log: `dude_dial_begin host=194.15.45.62 policy=trust_on_first_use` → `dude_handshake_failed err="ssh: handshake failed: EOF"` (94ms). Manual cross-check: TCP/22 bağlanıyor (TcpTestSucceeded=True, RTT 45ms ICMP) ama 2sn beklendikten sonra cihaz hiç SSH banner göndermiyor. Hedef cihaz SSH server'ı yanıt vermiyor. |
| `POST /run` discovery | **BLOCKED — prerequisite fail** | test-connection reachable=false olduğu için orchestrator çağırılmadı (operatör akışı, prerequisite başarısız). |
| Terminal state success | **N/A** | Discovery run başlatılmadı. |
| Discovered device count | **N/A — BLOCKED** | Live cihaz envanteri çekilemedi; hayali sayı uydurulmadı. |
| Category distribution | **N/A — BLOCKED** | Aynı sebep. |
| AP / CPE / BackhaulLink / Bridge / Router / Switch / Unknown count | **N/A — BLOCKED** | Aynı sebep. |
| Low confidence count / Unknown count | **N/A — BLOCKED** | Aynı sebep. |
| `unknown=true` / `low_confidence=true` filtre | PASS (parametrik/HTTP) | Endpoint+filter HTTP 200 döndü, summary boş (önceki turdan kanıtlı). Live cihaz olmadığı için sonuç semantiği doğrulanamadı. |
| `audit_logs` `test_connection` event | PASS | DB query: 1 satır, `action=network.dude.test_connection`, `subject=194.15.45.62`, `outcome=failure`, `actor=system`, `metadata={"error_code":"unreachable","duration_ms":0}`. Sanitize: parola/secret yok. |
| `audit_logs` `discovery_run_started/finished` | **N/A — BLOCKED** | Discovery run yapılmadı; bu event'ler beklenmedi. |
| `network_devices.raw_metadata` redaction | **N/A — BLOCKED** | Live cihaz yok. Statik kanıt: `internal/dude/sanitize.go` testleri (`internal/dude/sanitize_test.go`) yeşil. |
| Dedupe/upsert (mac > host+name > name) | **N/A — BLOCKED (in-memory PASS)** | `dude.dedupeDevices` `TestDedupe_MACWinsOverIP` PASS; DB tarafında `uq_netdev_source_mac` partial unique index doğrulandı. Live ikinci run yapılmadı (test-connection fail). |
| Secret leakage check | PASS | `.smoke_api.log` `password|secret|token|fingerprint|MIKROTIK_DUDE_PASSWORD|<username>` aramasında 0 hit. audit_logs metadata sadece `error_code` + `duration_ms` (parola/host_key yok). `.env` `.gitignore`'da (line 20) — `git check-ignore -v .env` doğruladı. Bu dökümanda hiçbir gerçek şifre/host fingerprint yok. |
| Cihaz tarafı tanı (sanitize) | INFO | TCP/22 erişilebilir; ICMP echo cevap yok (firewall — MikroTik için tipik). SSH banner alınmıyor (2sn bekleme sonrası `NO_BANNER_RECEIVED`). Olası nedenler: (1) RouterOS `/ip service` SSH disabled, (2) SSH `allowed-addresses` listesi bizim IP'yi reddediyor, (3) brute-force/Drop blocker kuralı bizi banlıyor, (4) TCP/22 farklı bir cihaza/servise port-forward. Bu PR'ın kodu değil, cihaz konfigürasyon problemi. |

## Inventory Summary

Live discovery yapılmadı; SSH handshake EOF nedeniyle test-connection fail (cihaz tarafı).

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

- **Status:** **BLOCKED — DEVICE-SIDE SSH UNREACHABLE.** (İlerleme: önceki "OPERATOR CREDENTIALS"tan ileri taşındı — operatör parolayı koydu, kod canlı SSH dial yaptı; cihaz tarafı SSH banner göndermiyor.)
- **Reason:** Engineering, migration, API+DB integration ve config validation tamamen yeşil; operatör parolayı `.env`'e koydu; API doğru env ile çalıştı. Ancak hedef MikroTik cihazı (`194.15.45.62`) SSH handshake'i kabul etmiyor — TCP/22 açık, banner yok, EOF. Prompt'un conditional merge kapısı `POST /test-connection reachable=true` ve `POST /run terminal success` PASS'ı zorunlu kılıyor; ikisi de sağlanmadı. Sahte başarı raporu yok; PR merge **edilmedi**.
- **Engineering blocker:** Yok. Kod tarafı temiz: dial → handshake fail → sanitized error → audit `failure` event → HTTP 200 + reachable=false JSON. Hiçbir secret loga/audit'e/cevaba düşmedi.
- **Environment blocker:** Yok (lokal PG16 + API + tüm Phase 8 endpoint'leri çalışıyor).
- **External blocker:** **Lab cihazı tarafı.** Hedef MikroTik (`194.15.45.62`) SSH server'ı yanıt vermiyor. Operatörün cihaz tarafında doğrulaması gereken konular:
  - `/ip service print` → `ssh` enabled mı? `disabled=no`?
  - `/ip service set ssh address=<bizim_lab_subnet>` veya `address=""` (boş = herkes; lab için OK).
  - `/ip firewall filter` ve `/ip firewall raw` zincirlerinde input chain'de SSH'a `drop`/`tarpit` kuralı var mı? (Brute-force protection bizi banlamış olabilir — `/ip firewall address-list` listesinde bizim public IP var mı?)
  - Cihaz gerçekten MikroTik Dude master mı, yoksa farklı bir router mı? (Identity doğrulaması.)
  - TCP/22 başka bir cihaza port-forward edilmiyor mu? (NAT kuralları.)
- **Out-of-scope blocker'lar (Faz 8 değil):** (1) Faz 7 `work_orders` şema çakışması (000007 vs 000001), (2) `scripts/db_migrate.sh` Windows psql URL-DSN parsing sorunu. PR #5 live smoke'unu bozmuyorlar.

### Operatör için sonraki adım

1. Lab cihazına RouterOS Winbox/SSH/Console üzerinden eriş (alternatif yol).
2. `/ip service print` → ssh enabled + `address` listesi kontrol et.
3. `/ip firewall filter print where chain=input` → SSH'ı engelleyen kural var mı?
4. `/ip firewall address-list print where list~"banned|blacklist"` → bizim IP banlı mı? Banlı ise `/ip firewall address-list remove [find ...]`.
5. SSH service düzeldikten sonra:
   - `F:\WispOps\wisp-ops-center\.env` içindeki **duplicate** `MIKROTIK_DUDE_PASSWORD` satırlarını temizle (sadece bir tane bırak; duplicate dotenv parsing'inde "last wins" davranışı sorun yapabilir).
   - PG16 zaten ayakta; `wispops_smoke` DB hazır.
   - API'yi yeniden başlat (binary `F:\WispOps\wisp-ops-center\.smoke_api.exe` mevcut).
   - `POST /test-connection` → `reachable=true` + `identity` gözle.
   - `POST /run` → 202 + run_id; `GET /runs` terminal state.
   - `GET /devices` + filtreler doğrula; ikinci `POST /run` ile dedupe.
   - `audit_logs` `test_connection`, `discovery_run_started/finished` doğrula; `network_devices.raw_metadata` redaction kontrol.
6. Smoke yeşil → `gh pr merge 5 --repo bybinbir/wisp-ops-center --squash --delete-branch`.

Otonom oturum smoke yeşil olmadan PR'ı merge etmedi; prompt kuralı: **live MikroTik smoke PASS olmadan merge yok**.
