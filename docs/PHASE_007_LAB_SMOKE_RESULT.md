# Phase 7 Lab Smoke Result

## Baseline

- **Main commit:** `95e94b6` (chore: pin LF line endings for cross-platform gofmt cleanliness)
- **Phase 7 merge commit:** `685b23f` (Phase 7: Work orders, reports and executive summaries — squash of PR #1)
- **Cleanup commit:** `95e94b6` (squash of PR #2 — `.gitattributes`)
- **Date:** 2026-04-28
- **Operator:** wisp-ops-bot (automated)
- **Environment:** Windows 11 / Git Bash / Go 1.26.0 / Node (Next.js 14.2.5). **Postgres yok** — lab DB sandbox dışında.

## Gate Tests

Hepsi temiz `main` (`95e94b6`) üzerinde çalıştırıldı:

| Check | Result | Evidence |
|---|---:|---|
| `gofmt -l .` | **PASS** | Çıktı boş (0 satır). |
| `go vet -buildvcs=false ./...` | **PASS** | Exit 0; çıktı boş. |
| `go test -buildvcs=false ./...` | **PASS** | Tüm paketler PASS: `mikrotik`, `mimosa`, `ssh`, `apclienttest`, `credentials`, `reports`, `scheduler`, `scoring`, `workorders`. |
| `go build -buildvcs=false ./...` | **PASS** | Exit 0. |
| `npm run build` (apps/web) | **PASS** | 16 sayfa derlendi; `/is-emirleri`, `/is-emirleri/[id]`, `/raporlar`, `/raporlar/yonetici-ozeti` static + dynamic rotalar var. |

## Lab Smoke

Lab Postgres kullanılabilir değildi. Bütün senaryoların **gerçek DB doğrulaması** EXTERNAL BLOCKER olarak işaretlendi. Tabloda her senaryo için kod düzeyinde sahip olduğumuz kanıt da listelendi.

| Scenario | Result | Evidence |
|---|---:|---|
| Migration apply (000007) | **BLOCKED** | Postgres yok. Migration idempotent + transactional; `internal/database/migrations.go::Migrate` kullanır. Manuel uygulama: `WISP_DATABASE_URL=postgres://... ./api migrate`. |
| Promote idempotency | **BLOCKED** | DB transaction'ı `SELECT ... FOR UPDATE` kilidi kullanır (`internal/workorders/repository.go::PromoteCandidate`). Aynı candidate üzerinde 2. promote çağrısı `Outcome.Duplicate=true` döndürür. Kod testi: `internal/workorders/workorders_test.go::TestStatusTransitions` PASS; lock davranışı yalnız canlı DB ile doğrulanır. |
| Cooldown behavior | **BLOCKED** | `recently_dismissed` / `recently_cancelled` / `already_promoted` / `duplicate_open_candidate` SQL filtreleri `internal/scoring/repository.go::CreateWorkOrderCandidate` içinde. Cooldown SQL'i `make_interval(days => $3::int)` (review fix) kullanır — Postgres 9.4+ uyumlu, explicit cast. Kod testi PASS; canlı DB doğrulaması BLOCKED. |
| Scheduler snapshot chain | **BLOCKED** | `JobDailyExecutiveSummary` katalog kaydı: `internal/scheduler/engine_test.go::TestJobCatalogControlsExecution` PASS. Worker handler: `apps/worker/internal/daily_executive_summary_handler.go` `report_snapshots` insert eder. Canlı DB ve worker boot ile end-to-end BLOCKED. |
| TLS fail-closed | **PASS (unit)** / **BLOCKED (handshake)** | `internal/adapters/mikrotik/tls_test.go` 6 senaryo PASS: `DefaultInsecure`, `ServerNameOverrideAppliedWhenInsecure`, `VerifyTLSWithoutCA`, `VerifyTLSWithCustomCA` (gerçek self-signed cert üretip RootCAs doğrular), `InvalidCA_FailsClosed` (ErrInvalidCA), `ServerNameOverrideAppliedWhenSecure`. Gerçek RouterOS API-SSL handshake BLOCKED (lab cihazı yok). |
| Reports/executive summary | **BLOCKED** | `internal/reports/repository.go::BuildExecutiveSummary` aggregation queries (severity dağılımı, top10 risky AP/tower, top diagnoses, work order sayaçları, 7d/30d trend) Postgres 14+ syntax kullanır (`DISTINCT ON`, `FILTER`, `make_interval`). `work_orders` tablosu yoksa `pgconn.PgError` SQLSTATE 42P01 (review fix) yakalanıp sayaçlar 0'da bırakılır. CSV/HTML render `internal/reports/csv_test.go` PASS. Canlı endpoint `/api/v1/reports/executive-summary` ve `/api/v1/reports/executive-summary.pdf` BLOCKED. |
| Audit filter cap | **PASS (code-level)** | `apps/api/internal/http/handlers_audit_export.go` review fix'i `maxFilterLen=200` cap koyar; aşılırsa `400 filter_value_too_long`. Defensive — Postgres parameterize edilmiş, SQL injection yok. Runtime smoke (curl ile 250-char actor) BLOCKED. |

## Safety Confirmation

Phase 7 review + smoke bağlamında aşağıdaki güvenlik sınırları **hiçbir zaman ihlal edilmedi**:

- ✅ No write/apply operation executed against production devices.
- ✅ No bandwidth-test executed.
- ✅ No frequency change executed.
- ✅ No production MikroTik/Mimosa mutation executed.

Phase 7 kodunda yer alan tüm kod yolları read-only:
- `internal/workorders` — yalnız Postgres'e yazar (work_orders + work_order_events); cihazla iletişim yok.
- `internal/reports` — yalnız aggregation SELECT'ler; cihazla iletişim yok.
- `apps/worker/internal/daily_executive_summary_handler.go` — yalnız report_snapshots'a JSON yazar.
- `internal/adapters/mikrotik/tls.go::BuildAPITLSConfig` — yalnız `*tls.Config` üretir; cihazla bağlantı kurmaz, hatalı CA → fail-closed.

## Open Debt Carried to Phase 8

Phase 7 sonrası taşınan açık borçlar:

1. **Server-side gerçek PDF rendering** — Phase 7 yazdırılabilir HTML olarak servis ediyor (`window.print()` → tarayıcı PDF). Phase 8'de `signintech/gopdf` veya `chromedp` ile değerlendirilecek.
2. **Audit retention scheduler job** — 90 gün retention politikası dokümante; otomatik temizlik scheduler job (`audit_retention_purge`) Phase 8'e ertelendi. Manuel SQL `DELETE FROM audit_logs WHERE at < now() - interval '90 days'` kullanılabilir.
3. **Phase 8 Frequency Recommendation Engine** — read-only kalmaya devam eder. Apply, operator approval, device mutation yok. Kontrollü apply için ayrı bir Phase 9+ kapısı tasarlanacak.

## Final Status

**PARTIAL** — Gate tests tamamı PASS; lab Postgres smoke senaryoları EXTERNAL BLOCKER nedeniyle BLOCKED.

**Reason:** Bu işletim ortamında Postgres yüklü değil (`psql` yok, `WISP_DATABASE_URL` set edilmemiş, Windows servislerinde `postgres*` yok). Sandbox dışı bir lab ortamında `docs/RUNBOOK_PHASE_007.md §7` adımları ile end-to-end smoke koşulduğunda bu rapor PASS güncellenebilir.

Smoke bloklamasının niteliği:

- **ENGINEERING:** Yok. Kod kapıları (gofmt, vet, test, build, npm build) yeşil; review fix'leri uygulandı.
- **EXTERNAL:** Yok (PR #1 ve PR #2 merged; GitHub erişimi sağlam).
- **ENVIRONMENT (BLOCKER):** Lab Postgres ve gerçek RouterOS cihazı bu ortamda mevcut değil. Smoke için ayrı bir lab makinesi gerekir.

Phase 8 planlama bu BLOCKER ile engellenmez — Phase 8 read-only design olduğu için kod tarafı geliştirilebilir; Phase 7 + Phase 8 birlikte canlı lab'da doğrulanır.
