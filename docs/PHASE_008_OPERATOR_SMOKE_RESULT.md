# Phase 8 Operator Smoke Result

## Baseline

- **PR:** https://github.com/bybinbir/wisp-ops-center/pull/5
- **Head:** `36a27a243501b2a9275c8b6fb4e01506e0f00efa` *(docs: add phase 8 PR review and smoke result)*
- **Date:** 2026-04-28
- **Environment:** Otonom dev sandbox (Windows 11 / msys-bash). PostgreSQL client (`psql`/`pg_isready`) **kurulu değil**, lokal PostgreSQL daemon dinlemiyor (TCP 127.0.0.1:5432 → kapalı). MikroTik Dude SSH şifresi sandbox'a aktarılmadı; `.env` dosyası mevcut değil. Hem PG hem live RouterOS bağlantısı sandbox dışında.
- **Operator:** Claude Opus 4.7 (1M context) çalışan otonom akış (kullanıcı: malii / marskocas@gmail.com)
- **PR mergeable / mergeStateStatus:** `MERGEABLE` / `CLEAN`
- **Reviewer (engineering) raporu:** `docs/PHASE_008_PR5_REVIEW_SMOKE_RESULT.md` — READY *(engineering)*, smoke ENVIRONMENT BLOCKED.

> Bu doküman dürüst sandbox raporudur. Sahte başarı kaydı yazılmamıştır. PG ve MikroTik smoke için "PASS" demek için canlı kanıt gerekir; sandbox'ta kanıt üretilemediği için ilgili satırlar **BLOCKED** olarak işaretlendi.

## Gate Tests

Bu kısım sandbox içinde çalıştırılabilir ve **gerçek olarak yeniden koşturuldu**.

| Check | Result | Evidence |
|---|---:|---|
| gofmt -l . | PASS | Çıktı boş (Phase 8 dosyalarında format ihlali yok). |
| go vet -buildvcs=false ./... | PASS | Çıktı boş; tüm paketler temiz. |
| go test -buildvcs=false ./... | PASS | `internal/dude`, `internal/networkactions`, `internal/config` + mevcut paketler (`mikrotik`, `mimosa`, `ssh`, `apclienttest`, `credentials`, `scoring`, `scheduler`, `workorders`, `reports`) `ok`. |
| go build -buildvcs=false ./... | PASS | Tüm paketler derlendi; `cmd/api` ve `cmd/worker` dahil. |
| npm run build | PASS | Next 14 production build; `/ag-envanteri` 5.34 kB statik prerender. ✓ Compiled successfully. |

## PostgreSQL Smoke

Smoke için sandbox'ta `psql` kurulu değil; TCP 127.0.0.1:5432 kapalı; `WISP_DATABASE_URL` env değişkeni tanımlı değil. Sandbox dışı operatör müdahalesi gerekir.

| Scenario | Result | Evidence |
|---|---:|---|
| Migration apply | BLOCKED | Sandbox'ta `psql` yok; lokal PG dinlemiyor. Migration kodu yapısal olarak idempotent + transactional doğrulandı (5 × `CREATE TABLE IF NOT EXISTS`, 12 × `CREATE [UNIQUE] INDEX IF NOT EXISTS`, BEGIN/COMMIT, DROP yok). |
| Idempotent re-run | BLOCKED | Aynı sebep. |
| Run lifecycle | BLOCKED | Repo katmanı (`CreateRun`/`FinalizeRun`) review'da temizdi; canlı kanıt yok. |
| Device upsert | BLOCKED | Repo katmanı (mac > host+name > name) önceliği review'da temizdi; canlı kanıt yok. |
| MAC dedupe | BLOCKED (in-memory PASS) | `dude.dedupeDevices` `TestDedupe_MACWinsOverIP` ile doğrulandı. DB tarafında `uq_netdev_source_mac` partial unique index var. |
| Evidence refresh | BLOCKED | `UpsertDevices` her cihaz için `DELETE … run_id` + per-Evidence INSERT — review'da doğrulandı; canlı tx kanıtı yok. |
| Filters | BLOCKED (parametrik PASS) | `ListDevices(filter)` parametrik; status/category artık `e7915d6` sonrası 400 ile validate ediliyor. |
| GetDevice/ListRuns | BLOCKED | UUID shape check `looksLikeUUID()` review fix'iyle eklendi; canlı kanıt yok. |

## MikroTik Dude Read-Only Smoke

Smoke için sandbox'a MikroTik Dude SSH şifresi aktarılmadı; `.env` dosyası mevcut değil. `Configured()` `false` döner; API her uçta `412 not_configured` yanıtlar.

| Scenario | Result | Evidence |
|---|---:|---|
| Test connection | BLOCKED | `.env` yok; şifre sağlanmadı. `TestLoad_DudeNotConfigured` `Configured()=false` codepath'ini birim seviyede doğruluyor. |
| Discovery run | BLOCKED | Aynı sebep. |
| Runs endpoint | BLOCKED | DB yok. |
| Devices endpoint | BLOCKED | DB yok. |
| Unknown filter | BLOCKED | DB yok. |
| Low confidence filter | BLOCKED | DB yok. |
| Audit events | BLOCKED | Audit sink Postgres'te. |
| Secret leakage check | PASS (statik) | Tüm error/log/audit yolları sanitize'den geçer. `git ls-files | xargs grep -E '(password|secret|token|fingerprint)\\s*=\\s*[^<]'` repo ağacında gerçek değer içermez (örnek/redact-only). `.env` repoda yok ve `.gitignore` korur. |

## Inventory Summary

Live discovery yapılmadı; sandbox dışı.

- Discovered device count: **N/A — BLOCKED** (live discovery yapılmadı)
- Category distribution:
  - AP: N/A
  - CPE: N/A
  - BackhaulLink: N/A
  - Bridge: N/A
  - Router: N/A
  - Switch: N/A
  - Unknown: N/A
- Low confidence count: N/A
- Unknown count: N/A

## Safety Confirmation

- ✅ **No write/apply command executed.** Hiçbir RouterOS write komutu çalıştırılmadı; allowlist ihlali yok.
- ✅ **No frequency change executed.** Frequency apply Phase 8'de zaten kod yolu olarak yok; networkactions stub'lar `ErrActionNotImplemented` döner.
- ✅ **No bandwidth-test executed.** `/tool/bandwidth-test` allowlist'te yok; herhangi bir Exec çağrısı yapılmadı.
- ✅ **No reboot/reset/config mutation executed.** Hiçbir mutating komut allowlist'te yok; hiçbir live SSH session açılmadı (şifre yok).
- ✅ **No secret committed or logged.** `.env` repo dışı; `.gitignore` `.env`, `.env.local`, `.secrets`, `*.pem`, `*.key` koruması var. Bu dökümanda hiçbir gerçek şifre/host fingerprint geçmiyor.
- ✅ **Only read-only allowlisted commands (would be) executed.** Sandbox'ta hiçbir live komut çalıştırılmadı; allowlist `internal/dude/allowlist.go` × 18 girişi `print`/`detail` ile bitiyor (test kanıtlı).

## Final Decision

- **Status:** **BLOCKED — ENVIRONMENT.**
- **Reason:** Engineering tarafı READY (gofmt/vet/test/build/next build hepsi PASS, review hardening commit'i `e7915d6` mevcut). Ancak operatör smoke için kanıt üretilemiyor: sandbox'ta (1) `psql` ve `pg_isready` kurulu değil, (2) lokal PostgreSQL daemon dinlemiyor, (3) MikroTik Dude SSH şifresi sandbox'a aktarılmadı, (4) `.env` dosyası mevcut değil. Bu nedenle prompt'taki "smoke PASS ise merge et" kapısı geçilemez. Sahte başarı kaydı yazılmadı; PR otomatik merge edilmedi.
- **Engineering blocker:** Yok.
- **Environment blocker:** Sandbox'ta lab ortamı yok — operatör müdahalesi gerekir.
- **External blocker:** Yok.

### Operatör için sonraki adım (sandbox dışı)

1. Lab makinesinde `psql` + PostgreSQL 14+ kur, `WISP_DATABASE_URL` set et.
2. `cp .env.example .env` ve `MIKROTIK_DUDE_PASSWORD` doldur (asla commit etme; `.gitignore` korur).
3. `./scripts/db_migrate.sh` ile migration `000008_mikrotik_dude_discovery.sql` apply et; aynı komutu tekrar çalıştırarak idempotency doğrula.
4. API + worker + web başlat (`scripts/dev_run_*.sh` + `apps/web && npm run dev`).
5. `POST /api/v1/network/discovery/mikrotik-dude/test-connection` → `reachable=true` + identity gözle.
6. `POST /run` → 202 + run_id; `GET /runs` ile terminal duruma gelmesini izle (`succeeded` veya `partial`).
7. `GET /devices` ve filtreler (`?unknown=true`, `?low_confidence=true`) doğrula.
8. `audit_logs` tablosunda 3 event görünüyor olmalı; `network_devices.raw_metadata` JSONB içinde `password|secret|token|community|key|bearer|auth` substring'leri `[redacted]` olmalı.
9. Smoke yeşil → `gh pr merge 5 --repo bybinbir/wisp-ops-center --squash --delete-branch` ile main'e al.

Otonom oturum bu adımları yerine getirmeden PR'ı merge etmeyecek; sandbox kuralları gereği lab ortamı zorunluluğu var.
