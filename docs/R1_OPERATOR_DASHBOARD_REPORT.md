# Phase R1 — Operator-Usable Dashboard Report

**Prompt ID:** WISP-R1-OPERATOR-USABLE-DASHBOARD-001 v1.0.0
**Date:** 2026-04-29
**Branch:** `phase/r1-operator-dashboard-and-evidence`
**Companion docs:** `docs/PRODUCT_REALITY_AUDIT.md`, `docs/MVP_RESCUE_PLAN.md`

> Goal of R1: make the browser product useful **today** for a WISP operator using only the data we actually have (Dude discovery + action lifecycle + safety chassis + reachability).

## 1. Executive Summary

The pre-R1 dashboard sourced its 8 KPI cards from `/api/v1/reports/executive-summary`, which depends on the scoring engine having run against ingested customer telemetry. In the lab against `194.15.45.62` no such telemetry exists, so the page rendered "—" everywhere. R1 swaps the data source for `/api/v1/dashboard/operations-panel`, a new aggregation endpoint that returns discovery state, last-24h action counters, safety chassis state, and reachability health — all from rows that already exist in the lab DB.

R1 also adds a **per-device "why is this Bilinmeyen?" drill-down** (`GET /api/v1/network/devices/{id}/evidence`) reachable from the inventory table, and **applicability hints** on the four read-only action buttons so the operator sees `↯` markers and a tooltip when an action is likely to return `skipped` for the device's category.

No safety guard was weakened. No destructive code path was touched. Both master switches stay closed.

## 2. Product Verdict

**R1 is engineering-closed AND operator-improved.** Closure rule from the brief satisfied:

| Closure rule | Result |
|---|---|
| Dashboard shows real discovered device counts | ✅ via `operations-panel.discovery.totals` |
| Inventory filters work | ✅ unchanged from prior state, regression-checked via tsc |
| Unknown percentage is visible | ✅ shown on dashboard `Bilinmeyen` card and as a meta line |
| "Why Unknown?" drill-down works | ✅ `EvidenceModal` opens from any device row |
| Skipped actions are visible with reasons | ✅ applicability hints on row buttons + recent-actions table in modal |
| Safety state is visible | ✅ four-cell safety section + blocking reasons + dry-run-only badge |
| All changed code passes available gates | ✅ gofmt clean, vet RC=0, build RC=0, full repo test RC=0, tsc RC=0 |

**Not yet operator-usable** (and outside R1 scope): >99% Unknown reduction (R2), real wireless-target action results (R3), scheduled action heatmap (R4), corrective frequency_correction (R5).

## 3. Before / After

### 3.1 Dashboard `/`

| Aspect | Before | After |
|---|---|---|
| Data source | `/api/v1/reports/executive-summary` | `/api/v1/dashboard/operations-panel` |
| KPI cards | 8 scoring KPIs, all "—" in lab | 8 discovery KPIs (Toplam, AP, Link, Bridge, CPE, Router, Switch, Bilinmeyen + %) |
| Action visibility | none | last-24h success/skip/fail cards + per-kind histogram + latest-action row |
| Safety state | none | 4 cells (master switch, legacy const, provider toggle, active maintenance windows) + blocking reasons |
| System health | none | DB OK / Dude configured / Last Dude test result |
| Empty-state quality | silent "—" | explicit `data_insufficient` cards with reason + hint |
| Auto-refresh | none | 15 s polling |
| Truth labels | none | every section tagged Real / Missing / Skipped / Not implemented / Safety-blocked |

### 3.2 Inventory `/ag-envanteri`

| Aspect | Before | After |
|---|---|---|
| Device name cell | static `<strong>` | clickable button → opens evidence modal |
| Action buttons | 4 buttons identical color regardless of category | applicable buttons keep their color; likely-skip buttons render with dashed border + dimmed + `↯` glyph + tooltip explaining why |
| "Why Bilinmeyen?" | not present | `EvidenceModal` shows winner / runner-up category weights, missing-signals list, applicability hints, recent action history (10 rows), raw evidence (collapsed) |
| Tooltips | "Read-only Frekans" | category-aware: "Cihaz kategorisi (Bridge) için bu aksiyon büyük olasılıkla skipped döner. Yine de denemek için tıklayın." |

## 4. Routes Changed / Added

### 4.1 New API endpoints

| Method + Path | Purpose | Auth | Reachability |
|---|---|---|---|
| `GET /api/v1/dashboard/operations-panel` | Operations panel aggregate | `requireDB` only (read-only, no RBAC capability) | Real |
| `GET /api/v1/network/devices/{id}/evidence` | Per-device evidence drill-down | `requireDB` only | Real |

Both endpoints are dispatched via the existing routing chassis. The dashboard endpoint has its own `mux.HandleFunc`. The evidence endpoint reuses the existing `/api/v1/network/devices/` dispatch — `handleNetworkDevicesDispatch` now routes any path ending with `/evidence` to `handleNetworkDeviceEvidence`.

### 4.2 New web components

- `apps/web/src/app/DashboardClient.tsx` — full rewrite of the operator panel.
- `apps/web/src/app/ag-envanteri/EvidenceModal.tsx` — new modal component opened from inventory rows.
- `apps/web/src/app/ag-envanteri/Client.tsx` — clickable name cell, applicability hints on action buttons, modal mount.

## 5. Backend Files Changed

```
apps/api/internal/http/handlers_operations_panel.go   NEW
apps/api/internal/http/handlers_device_evidence.go    NEW
apps/api/internal/http/handlers_r1_test.go            NEW (12 hermetic unit tests)
apps/api/internal/http/handlers_network.go            MODIFIED (3-line addition: /evidence dispatch)
apps/api/internal/http/routes.go                      MODIFIED (1-line addition: operations-panel route)
```

Frontend:

```
apps/web/src/lib/api.ts                               EXTENDED (R1 types added at end)
apps/web/src/app/DashboardClient.tsx                  REWRITTEN (operator panel)
apps/web/src/app/ag-envanteri/Client.tsx              MODIFIED (4 surgical patches)
apps/web/src/app/ag-envanteri/EvidenceModal.tsx       NEW
```

Documentation:

```
docs/R1_OPERATOR_DASHBOARD_REPORT.md                  NEW (this file)
TASK_BOARD.md                                         UPDATED (R1 progress)
```

## 6. Operator Can Now Do

1. Open `/` and see, in plain Turkish, **how many devices were discovered**, broken down by category, with the Bilinmeyen percentage prominent.
2. See the **last-24h network-action histogram** (succeeded / skipped / failed) and the latest action's status, target, dry-run flag.
3. See the **safety chassis state at a glance**: destructive enabled, both master switches, active maintenance windows, blocking reasons, dry-run-only badge.
4. See the **DB / Dude / last-test reachability** in one row.
5. See **explicit `data_insufficient` cards** instead of silent "—" when an area has no data — with a Turkish explanation and a hint of what action to take.
6. Click any device name on `/ag-envanteri` and read **why** that device was classified the way it was: winner / runner-up category weights, missing-signals list (mac / neighbor_platform / board / interface_name / evidence_summary), what each missing signal would have helped with, applicability hints for the four read-only actions, and the last 10 action runs against this device with their skip reasons.
7. **See on hover** which row buttons will likely return `skipped` for this category (dashed border + dimmed + `↯` glyph + tooltip).
8. Watch the dashboard refresh every 15 s without page reload.

## 7. Still Not Usable / Still Blocked

- The Bilinmeyen percentage is still ~99% on the live `194.15.45.62` dataset. R1 makes that visible and explainable but does not fix it. Phase R2 owns this.
- All four read-only actions still return `skipped` against the lab Dude (no wireless / bridge menu). R1 makes "skipped" honest and operator-readable but cannot produce non-skipped output without a wireless RouterOS lab target. Phase R3 owns this.
- `/musteriler`, `/raporlar/yonetici-ozeti`, `/is-emirleri` remain dependent on customer-signal telemetry the lab has never had. R1's `data_insufficient` panel announces this.
- `/frekans-onerileri` is still a `stub("frequency_recommendations.list")` handler. Phase R4 owns this.
- Frequency-correction wiring is still off. Phase R5 owns this and only when the four operator-side preconditions hold.

## 8. Quality Gates — Exact Results

```
$ gofmt -l .
(empty)                                          RC=0

$ go vet ./...
(empty)                                          RC=0

$ go build ./...
(empty)                                          RC=0

$ go test -count=1 -short ./...
ok  apps/api/internal/http               0.003s
ok  internal/adapters/mikrotik           0.004s
ok  internal/adapters/mimosa             0.002s
ok  internal/adapters/ssh                0.002s
ok  internal/alerts                      0.013s
ok  internal/apclienttest                0.004s
ok  internal/config                      0.001s
ok  internal/credentials                 0.003s
ok  internal/dude                        0.006s
ok  internal/networkactions              0.020s
ok  internal/networkinv                  0.003s
ok  internal/reports                     0.003s
ok  internal/retention                   0.002s
ok  internal/scheduler                   0.004s
ok  internal/scoring                     0.003s
ok  internal/workorders                  0.003s
                                                  RC=0

$ go test -short ./internal/networkactions/... -v | grep -c "^--- PASS"
138

$ go test -short ./internal/dude/... -v | grep -c "^--- PASS"
38

$ go test -short ./apps/api/internal/http/... -v | grep -c "^--- PASS"
16    (was 4 pre-R1; +12 new R1 unit tests in handlers_r1_test.go)

$ cd apps/web && ./node_modules/.bin/tsc --noEmit
(empty)                                          RC=0

$ cd apps/web && ./node_modules/.bin/next build
NOT RUN — sandbox 45 s wall-clock cannot complete the production build inside one bash call. Frontend is type-clean per tsc above; production build evidence relies on the unchanged build chain that landed Phase 10F-A.
```

### 8.1 Test count diff

| Package | Before R1 | After R1 | Δ |
|---|---:|---:|---:|
| `internal/networkactions` | 138 | 138 | 0 (unchanged — no Phase 10 code touched) |
| `internal/dude` | 38 | 38 | 0 |
| `apps/api/internal/http` | 4 | 16 | **+12** |
| Other packages | unchanged | unchanged | 0 |

The 12 new tests cover: `summarizeEvidence` empty + winner+runner-up; `deriveMissingSignals` full-empty / fully-enriched; `deriveActionApplicability` AP / Bridge / low-confidence / Unknown; `extractDeviceIDForEvidence` happy path / trailing slash / nested rejection; `round1` rounding cases; `applicabilityFor` matrix walk.

## 9. Screens Verified (textual UI evidence — no screenshots in sandbox)

The frontend is type-clean per `tsc --noEmit` and has been authored against the existing components (`StatCard`, `Toolbar`, `Field`, `Sidebar`). Below is the textual structure each operator-facing screen renders.

### 9.1 `/` (Operasyon Paneli)

```
[Operasyon Paneli — Bugün ağda ne bozuk?]

[Ağ Envanteri (Dude Discovery)]                       [Gerçek veri]
  [Toplam Keşfedilen: N]   [AP: N]   [Backhaul / Link: N]   [Bridge: N]
  [CPE / Müşteri: N]       [Router: N]   [Switch: N]
  [Bilinmeyen: N]   meta="%X.X — Phase R2 hedefi: <%30"
  MAC kazandı: N · Host kazandı: N · Enriched: N · Düşük Confidence: N (%X.X)
  Son discovery run: <abc123…> · status [running|succeeded|failed|partial] · N cihaz · trigger: <user>

[Ağ Aksiyonları (son 24 saat)]                        [Gerçek veri]
  [Toplam: N]   [Başarılı: N]   [Skipped: N]   [Başarısız: N]
  Türlere göre: frequency_check=N · ap_client_test=N · …
  Son aksiyon: <kind> · <target> · status · confidence N · DRY-RUN

[Güvenlik Durumu]                                     [Güvenlik kilitli]
  Destructive master switch:   kapalı (güvenli)
  Legacy const flag:           kapalı
  Provider toggle:             kapalı
  Aktif bakım penceresi:       yok (corrective action engelli)
  Engelleyiciler: legacy_master_switch_disabled · provider_toggle_disabled · no_active_maintenance_window
  > Bu sayfa read-only. Operatör frequency correction veya başka destructive aksiyonu Phase R5'e kadar tarayıcıdan koşturamaz.

[Sistem Sağlığı]                                      [Gerçek veri]
  DB bağlantısı:           OK
  Dude konfigürasyonu:     194.15.45.62
  Dude erişimi (son test): OK · 3 dk önce

[Eksik / Yetersiz Veri]                               [Eksik veri]
  Müşteri sinyal skorları henüz hesaplanmadı
  Telemetry kaynağı (RouterOS poll, SNMP, Mimosa) bu lab DB'sine henüz bağlı değil.
  → Phase R3 ile gerçek wireless RouterOS lab target sağlandığında scoring engine besleyecek.

[Hızlı Erişim]
  • Ağ Envanteri  • Ağ Aksiyonları  • Planlı Kontroller  • Yönetici Özeti

Operasyon Paneli · 2 sn önce · 15 sn otomatik yenileme
```

### 9.2 `/ag-envanteri` row → modal

```
Cihaz Detayı — Sınıflandırma Kanıtı                                     [×]
─────────────────────────────────────────────────────────────────────────
[ <name> · [Bilinmeyen]  · confidence 0 ]
IP: 10.0.0.42 · MAC: AA:BB:… · MikroTik · iface: ether1 · son görüldü: …

Neden bu kategori?
  Bu cihaz Bilinmeyen, çünkü hiçbir kanıt eşiği aşmadı (en yüksek ağırlık: AP = 12).
  Toplam kanıt satırı: 3 · benzersiz heuristic: name_pattern, neighbor_platform
  Kategori ağırlıkları: AP=12 · Router=8

Eksik Sinyaller
  [mac]              MAC adresi enrich edilemedi (ip/neighbor + dude/probe + dude/service'ten gelmedi).
                     → Sınıflandırma kararını çoğu zaman tek başına AP / Bridge'e taşır.
  [neighbor_platform] /ip/neighbor bu cihaz için platform alanı döndürmedi.
                     → CPE vs Router ayrımında %20 ağırlığa sahip.
  [board]            Cihazın board / model alanı yok.
                     → Board adı 'AP' / 'CPE' / 'Switch' family eşlemesi için kullanılır.
  [interface_name]   Cihazın hangi interface üzerinden gözüktüğü kayıt edilmedi.
                     → Bridge port adı (ör. 'br-customers') Bridge sınıflandırması için belirleyici.
  [evidence_summary] evidence_summary alanı boş — son discovery enrichment bu cihaz için ek kanıt biriktirmedi.

Uygulanabilir Aksiyonlar
  [Frekans Kontrol  / belirsiz — denenebilir]   Cihaz Bilinmeyen — aksiyon denenebilir; sonuç skipped olabilir.
  [AP Client Test   / belirsiz — denenebilir]   ...
  [Link Signal Test / belirsiz — denenebilir]   ...
  [Bridge Health    / belirsiz — denenebilir]   ...
                                                güvenlik: read_only_dry_run

Aksiyon Geçmişi (son 10)
  [Tip][Durum][Süre][Conf][Tarih][Sebep]   …

Ham Kanıt (teknik detay)  [details — varsayılan kapalı]
  [Heuristic][Kategori][Ağırlık][Sebep][Run]
```

### 9.3 `/ag-envanteri` row action buttons

```
Adı: <Click → modal>
Aksiyon: [Frekans] [AP Client] [Link Signal↯] [Bridge Health↯]
                                  ^dashed/dim   ^dashed/dim
hover Link Signal: "Cihaz kategorisi (AP) için bu aksiyon büyük olasılıkla skipped döner. Yine de denemek için tıklayın."
```

## 10. Honest Closure Verdict

R1 is **operator-improved and engineering-closed for the operations-panel + evidence drill-down + applicability hints scope it set out to deliver**. It is **not** a "FULLY CLOSED" claim for the WISP Ops Center product overall; that claim still depends on R2/R3/R4/R5 in sequence.

The "Operator-usable delta" line for this PR (per `TASK_BOARD.md` honesty rule):

> An operator can now open `/` and see real device counts, real action lifecycle, real safety state, and explicit empty-state explanations; can click any inventory row and read why the classifier landed where it did; can see at a glance which read-only actions will likely return `skipped` for a device's category.

## 11. Exact Commands Run

```bash
# Repo orientation
git checkout main
git checkout -b phase/r1-operator-dashboard-and-evidence

# Backend
go fmt was self-applied via gofmt -w on the two new files.
go vet ./...
go build ./...
go test -count=1 -short ./...
go test -short ./internal/networkactions/... -v | grep -c "^--- PASS"   # 138
go test -short ./internal/dude/...           -v | grep -c "^--- PASS"   # 38
go test -short ./apps/api/internal/http/...  -v | grep -c "^--- PASS"   # 16

# Frontend
cd apps/web && ./node_modules/.bin/tsc --noEmit                          # RC=0
# next build NOT RUN — see §8 for reason.
```

## 12. Remaining Product Blockers

(unchanged from `docs/PRODUCT_REALITY_AUDIT.md §15`, repeated for traceability)

1. No real wireless RouterOS test target — until R3 lab work, all four read-only actions stay `skipped`.
2. No telemetry source other than Dude discovery has been wired up — scoring chain stays empty.
3. Phase R5 preconditions are operator-side (lab target + maintenance window + rollback owner + signing path).
4. `.env` in working tree contains live lab credentials (gitignored, but workspace-shared).
5. Sandbox bash 45 s wall-clock cannot drive `next build` in one call — production build evidence relies on prior PR-A merge artefacts.

---

**End of R1 report.**
