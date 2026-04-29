# Product Reality Audit — WISP Ops Center

**Audit ID:** WISP-PRODUCT-REALITY-AUDIT-001 v1.0.0
**Date:** 2026-04-29
**Auditor role:** product owner + senior architect + QA lead + DevOps + WISP ops manager (single-pass)
**Branch / HEAD:** `main` @ `0b987d1` (Phase 10F-A)
**Scope of this pass:** read-only audit. No new features. Tiny diff allowed only for evidence collection.

> The goal is not to defend prior work. The goal is to expose the truth and bring the project back to the intended product.

---

## 1. Executive Summary

The repository is in an unusual state: **engineering work is honest and high-quality, but product completeness has been overstated**. The terms "MERGED ✅", "FULLY CLOSED", "engineering closure" appear repeatedly in `current.md`, session reports and `TASK_BOARD.md`, but a real WISP operator opening the browser today **cannot** use this product end-to-end against the lab device at `194.15.45.62` — not because the safety gate is closed (that part is intentional), but because:

1. The single piece of telemetry the operator must see — *what is actually on the network and what is broken* — is rendered as **893 / 914 "Bilinmeyen" (Unknown) rows**. Classification works on `<1%` of devices in the only live dataset that exists.
2. The four read-only test buttons (`frequency_check`, `ap_client_test`, `link_signal_test`, `bridge_health_check`) are wired front-to-back and dispatch real SSH against `194.15.45.62`, but the live device returns `skipped_unsupported` on every wireless / bridge menu, so the operator gets back "skipped — sahte veri üretmedik" with confidence=30 every time. There is no actionable WISP signal in the UI from these.
3. Customer / signal / scoring / work-order surfaces (`/musteriler`, `/kuleler`, `/linkler`, `/raporlar/yonetici-ozeti`) require a populated database that the lab does not have. With only Dude discovery rows present, those screens are empty or `data_insufficient`.
4. Phase 10E "real frequency_correction Execute" is in `main` but **not wired into `server.go`**. The master switch is closed by design and the only operator-reachable destructive endpoint terminates at `execute_not_implemented` (Phase 10D) or `gate_fail/destructive_disabled` (Phase 10C).

In other words: **the engineering chassis is real, the safety posture is real, but the product the operator can touch is a thin shell over an under-populated database and an under-classified device set**. The previous "engineering-complete" claims are accurate; the implicit "product-complete" claims are not.

This audit separates the two and produces an MVP rescue plan (`docs/MVP_RESCUE_PLAN.md`) focused **only** on making the product usable.

---

## 2. Product-Completeness Verdict

**Engineering-complete:** ✅ for the safety chassis (Phases 10A→10F-A), the read-only adapters (Phases 3, 4, 9, 9 v2), the discovery transport (Phase 8), the scoring/work-order data model (Phases 6, 7).

**Product-complete:** ❌ NOT YET for the operator-facing target product.

| Dimension | Engineering | Product |
|---|---|---|
| Code quality / safety invariants | ✅ green | ✅ |
| Test coverage (138 networkactions + 38 dude unit, 16 packages PASS) | ✅ green | n/a |
| Read-only discovery transport against real Dude | ✅ proven | ✅ |
| Operator can see APs / links / bridges / customer devices, classified | ⚠️ pipeline exists | ❌ 99.6% Unknown on live data |
| Operator can run safe read-only tests with readable output | ⚠️ wired but live target unsupported | ❌ all return `skipped` |
| Operator can identify bad signal / link / problem devices | ⚠️ scoring engine exists | ❌ no signal source ingested for the lab |
| Operator can prepare controlled corrective action | ⚠️ Phase 10E Execute lives in code | ❌ not wired into HTTP server, master switch closed |
| Documented "FULLY CLOSED" matches reality | n/a | ❌ misleading |

**Verdict line:** *Engineering chassis built honestly; product MVP not yet stood up. The next phase must be product-shaped, not engineering-shaped.*

---

## 3. Repository Orientation

### 3.1 What `main` actually contains

`git log --oneline -10` (HEAD on main):

```
0b987d1 feat(phase-10f): harden RBAC retention and alerts (engineering) (#16)
f019cac feat(phase-10e): real frequency_correction Execute + verification + auto-rollback (lab-only) (#15)
416d308 Phase 10D: destructive happy-path lifecycle + Execute reachable (no execution) (#14)
4398e76 chore(docs): sync CLAUDE.md and TASK_BOARD with Phase 10C state (#13)
927c711 Phase 10C: Destructive runtime lifecycle (no execution) (#12)
92f32ac Phase 10B: Postgres-backed safety stores + API (#11)
ff489da Phase 10A: Destructive-action safety foundation (#10)
c2fd134 Phase 9 v3: hardening + Phase 10 pre-gate (#9)
ab97226 Phase 9 v2: read-only AP/link/bridge actions (#8)
769426e Phase 9: read-only network actions + frequency_check (#7)
```

### 3.2 Layer map

| Layer | Real implementation? | Path |
|---|---|---|
| Go API server | yes | `apps/api/cmd/api/main.go` + `apps/api/internal/http/` |
| Go worker (scheduler) | yes (engine present, integration partial) | `apps/worker/` |
| Next.js 14 web | yes, **14 routes** | `apps/web/src/app/` |
| Postgres migrations | yes, **000001 → 000013** | `migrations/` |
| MikroTik Dude SSH client | yes (`golang.org/x/crypto/ssh`, real wire) | `internal/dude/` |
| Read-only network actions | yes (4 kinds: freqcheck/apclient/linksignal/bridge) | `internal/networkactions/` |
| Destructive frequency_correction Execute | yes (lab-only, **not wired into server.go**) | `internal/networkactions/frequency_correction.go` |
| Safety chassis (toggle, RBAC, maintenance window, audit catalog) | yes | `internal/networkactions/` + Phase 10A→10F migrations |
| Customer signal scoring | yes (deterministic engine) | `internal/scoring/` |
| Work-order candidates / work orders | yes, schema + handlers | `internal/workorders/` + handlers_workorders.go |

### 3.3 Documentation vs. implementation drift

`TASK_BOARD.md` has not been re-touched since Phase 10D. Phase 10E PR-A and Phase 10F-A are merged on main but the board still reads "Active: Phase 10D". `current.md` (memory file outside the repo) is up-to-date but the repo's own ground truth document is not.

`CLAUDE.md` line 11 says: *"🟢 **Aktif:** Faz 10D — destructive happy-path lifecycle"*. Reality on main: 10D + 10E PR-A + 10F-A all landed.

---

## 4. Product Surface Audit

### 4.1 Web routes (real, by file)

`apps/web/src/app/` enumerated via `find -name "page.tsx"`:

| Route | Page file | Wired backend | Verdict |
|---|---|---|---|
| `/` | `page.tsx` + `DashboardClient.tsx` | `GET /api/v1/reports/executive-summary` | Real KPI cards. Returns 503 with friendly hint when DB empty/missing. |
| `/musteriler` | `musteriler/page.tsx` + `Client.tsx` | `GET /api/v1/customers-with-issues` | Real if scoring has run. **Empty in lab** because no signal source has been ingested into `customer_signal_scores`. |
| `/musteriler/[id]` | dynamic | `GET /api/v1/customers/<id>` | Detail surface real, depends on data. |
| `/kuleler` | `kuleler/page.tsx` | `/api/v1/towers`, `/api/v1/towers/<id>/risk` | Surface present, depends on tower seeding. |
| `/linkler` | `linkler/page.tsx` | `/api/v1/links` | Same — seeding-dependent. |
| `/cihazlar` | `cihazlar/page.tsx` + `[id]` | `/api/v1/devices` | Real CRUD surface, separate from `network_devices`. |
| `/planli-kontroller` | `planli-kontroller/page.tsx` | `/api/v1/scheduled-checks`, `/api/v1/job-runs`, `/api/v1/maintenance-windows` | Backend rich, UI lists scheduled checks; cron loop exists in worker. |
| `/frekans-onerileri` | `frekans-onerileri/page.tsx` | `/api/v1/frequency-recommendations` | **Stub handler.** `routes.go`: `mux.HandleFunc(..., stub("frequency_recommendations.list"))`. Page renders the empty stub. |
| `/raporlar` | `raporlar/page.tsx` | `/api/v1/reports` | Snapshot list — depends on report runs. |
| `/raporlar/yonetici-ozeti` | nested | `/api/v1/reports/executive-summary` | Same source as dashboard. |
| `/job-runs` | `job-runs/page.tsx` | `/api/v1/job-runs` | Audit-ish. |
| `/ap-client-tests` | `ap-client-tests/page.tsx` | `/api/v1/ap-client-test-results`, `/run-now` | Phase 5 surface, separate code path from Phase 9 v2 ap_client_test. |
| `/is-emirleri` | `is-emirleri/page.tsx` + `[id]` | `/api/v1/work-orders` | Real CRUD + lifecycle. |
| `/ag-envanteri` | `ag-envanteri/page.tsx` + `Client.tsx` | discovery + per-device action POSTs | **Most product-shaped page.** 12 stat cards, filters, two big buttons (Test Connection, Run Discovery), per-row 4 action buttons. |
| `/aksiyonlar` | `aksiyonlar/page.tsx` + `Client.tsx` | `GET /api/v1/network/actions` | Action runs list with detail panels per kind. Auto-polls every 3s while a run is `running`. |
| `/ayarlar` | `ayarlar/page.tsx` + `ScoringThresholds.tsx` | `/api/v1/scoring-thresholds` | Threshold knobs. |

Sidebar shows all 14 visible routes. Layout is dark / Apple-ish, Türkçe labels. **The dashboard is not a router-level "operasyon merkezi" — it is a KPI summary page that depends on data the lab does not have.**

### 4.2 Operator dashboard reality check

Today's `/` page renders these 8 KPI cards: Kritik Müşteri, Uyarıdaki Müşteri, AP-Wide Sorun, Bayat Veri, Açık İş Emri, Urgent / High, ETA Geçenler, Bugün Oluşturulan. **Every single one of these depends on the scoring engine having run against ingested telemetry**. With only Dude discovery rows in the lab DB, the scoring engine has nothing to score. The page either reports `data_insufficient` per card or shows "—".

The page that comes closest to the intended "Operasyon Paneli" today is **`/ag-envanteri`**, not `/`. That page does show real data when discovery runs (893 rows in the live lab), and exposes the four read-only actions per device row.

### 4.3 Action buttons — exist? wired? produce useful output?

| Where | Button | Wired? | Lab-evidence outcome |
|---|---|---|---|
| `/ag-envanteri` toolbar | "Bağlantıyı Test Et" → `POST /api/v1/network/discovery/mikrotik-dude/test-connection` | yes | `reachable=true` against `194.15.45.62` (PR #12 / #14 review smokes). |
| `/ag-envanteri` toolbar | "Discovery Çalıştır" → `POST /api/v1/network/discovery/mikrotik-dude/run` | yes | 893 rows inserted, 893 updated on second run; dedupe stable. |
| `/ag-envanteri` per-row | "Frekans" → `POST /api/v1/network/actions/frequency-check` | yes | Returns `skipped` + `skipped_reason=no_wireless_menu`. |
| `/ag-envanteri` per-row | "AP Client" → `.../ap-client-test` | yes | Same — `skipped`. |
| `/ag-envanteri` per-row | "Link Signal" → `.../link-signal-test` | yes | Same — `skipped`. |
| `/ag-envanteri` per-row | "Bridge Health" → `.../bridge-health-check` | yes | Same — `skipped` (`bridge` menu not present on live target). |
| `/aksiyonlar` | filter + run detail panes | yes (read-only) | Renders runs from `network_action_runs`. |
| Destructive `frequency_correction` | none in UI | dispatch routed at `/api/v1/network/actions/destructive/...` but **registry returns stub** (`server.go` does not call `RegisterFrequencyCorrection`); reachable only via curl, terminates at `gate_fail` / `execute_not_implemented` | by design — Phase 10E PR-B not started |

### 4.4 Scheduled checks

UI exists at `/planli-kontroller` (Phase 5). It lists scheduled checks and lets the operator declare maintenance windows. The worker daemon has the scheduler engine (`apps/worker/internal/`). What is missing is the **glue between the four Phase 9 v2 read-only actions and the scheduler** — they are run-once-on-button-press today; there is no UI control to schedule e.g. "frequency_check every AP, every 6 hours, into a heatmap".

### 4.5 Classification readability for non-developer operators

The category set today is: `AP`, `Link / Backhaul`, `Bridge`, `CPE / Müşteri`, `Router`, `Switch`, `Bilinmeyen`, `Düşük Confidence`. Labels are Turkish. The category field is shown in the inventory table. **The problem is not the labels — it is the per-row evidence.** The `device_category_evidence` table holds the per-source signal, but the UI does not surface "why is this classified as Bilinmeyen?". The operator cannot self-debug a misclassification.

---

## 5. Dude Integration Audit

### 5.1 Real or mocked?

**Real.** The transport is `golang.org/x/crypto/ssh` with TOFU host-key store, password auth, allowlisted commands. No fake fixtures in production code path; mock implementations exist only inside `*_test.go`.

Evidence:
- `internal/dude/client.go` — `(*Client).Dial` wraps `ssh.Dial` with timeout, host-key policy, known-hosts store.
- `.smoke_api_pr12_review.log` (2026-04-29 00:37 TRT): `nwaction_ssh_dial_begin host=194.15.45.62` → `nwaction_ssh_dial_ok`.
- `.smoke_api_pr14_review.log` (2026-04-29 13:06 TRT): same — second independent live dial.

### 5.2 Source coverage

`internal/dude/discovery.go` runs **5 commands** in order against the live Dude:

1. `/dude/device/print/detail` — primary.
2. `/ip/neighbor/print/detail` — MAC + platform + iface.
3. `/dude/probe/print/detail` — service signals.
4. `/dude/service/print/detail` — per-port service hints.
5. `/system/identity/print` — self-record.

Bridge / wireless data is **NOT** part of the discovery enrichment — those live in the per-device read-only actions (`bridge_health_check`, `frequency_check`).

### 5.3 Real-device evidence from `194.15.45.62`

| Aspect | Outcome | Evidence |
|---|---|---|
| SSH reachability | ✅ proven | smoke_api_pr12_review.log + smoke_api_pr14_review.log |
| Discovery run #1 | ✅ 893 devices ingested | `PHASE_008_OPERATOR_SMOKE_RESULT.md` |
| Discovery run #2 dedupe | ✅ 893 updated, 0 inserted | same |
| Phase 8.1 enrichment | ⚠️ executed, low yield | `PHASE_008_1_DISCOVERY_ENRICHMENT.md` |
| MAC enriched / total | **4 / 914** (~0.4%) | enrichment doc + memory |
| Category=Unknown / total | **914 / 914** before fix; **910+ / 914** after Phase 8.1 | enrichment doc states "892/893 stayed Unknown" pre-8.1 and "11 devices left Unknown" post |
| Bridge/wireless enrichment | ❌ not part of discovery | by design |

### 5.4 Classification reliability

**Low for the live lab dataset.** The classifier is evidence-driven and conservative — when the evidence isn't there it does not invent categories (good safety posture, bad UX). For this Dude, neighbor / probe / service entries rarely carry MAC, so the merge keys collapse onto `name` alone, which the heuristic cannot meaningfully classify.

The classifier is **not at fault — the source is thin**. The product gap is that the UI does not say so. It says "Bilinmeyen" without telling the operator why.

### 5.5 Missing evidence — explicit list

- ❌ No telemetry from MikroTik APs / CPEs has been collected in this lab DB. The Phase 3 RouterOS adapter exists, but no live `/api/v1/mikrotik/poll-results` runs against real APs are recorded.
- ❌ No SNMP polls recorded for this lab (`/api/v1/mimosa/poll-results` empty).
- ❌ No bridge port tables, no wireless registration tables — by design these are per-device read-only actions, but those return `skipped` against `194.15.45.62`.
- ❌ No `customer_signal_scores` rows in lab DB — scoring engine has not been run in this environment.
- ❌ No work-order candidates promoted from scores — same.
- ❌ Phase 10E live execute against `194.15.45.62` — by design, master switch closed and registry not wired.

---

## 6. Network Action Audit

### 6.1 Are the four read-only actions real?

| Action | Real Execute? | Source command path | Lab outcome |
|---|---|---|---|
| `frequency_check` | yes — `internal/networkactions/freqcheck.go` | `/interface/wireless/...` (or w60g, w50g) | `skipped` (`no_wireless_menu`) on `194.15.45.62` |
| `ap_client_test` | yes — `apclient.go` | `/interface/wireless/registration-table/...` | `skipped` |
| `link_signal_test` | yes — `linksignal.go` | wireless reg-table + interface stats | `skipped` |
| `bridge_health_check` | yes — `bridge.go` | `/interface/bridge/...` | `skipped` (no bridge menu) |

All four are dispatched through the same `Server.buildAction(kind, target)` factory in `apps/api/internal/http/handlers_actions.go`. Execute(DryRun=true) is always called for read-only kinds. No fake telemetry is ever returned — the convention is `skipped=true` + `skipped_reason` + confidence=30 when the menu does not exist on the device.

### 6.2 UI accessibility

- `/ag-envanteri` per-row buttons → `POST /api/v1/network/actions/<suffix>` with `target_device_id`. UI reads back through `/api/v1/network/actions` and renders detail panels at `/aksiyonlar`.
- `/aksiyonlar` panels render kind-specific output: `FrequencyCheckPanel`, `APClientTestPanel`, `LinkSignalPanel`, `BridgeHealthPanel`. Each shows a `SkippedBox` with the reason when `skipped=true`. Each shows warnings, evidence, command count.

### 6.3 Real targets

Yes — actions hit the same `194.15.45.62` SSH endpoint via `internal/adapters/ssh`. The `FrequencyCheckAction.Target` carries the SSH config built from runtime env (`MIKROTIK_DUDE_HOST/USERNAME/PASSWORD`). There is no live evidence, however, of these actions being run against a **non-Dude wireless RouterOS** target — the lab Dude has no wireless / bridge interfaces, so `skipped` is the only observed outcome.

### 6.4 Operator readability

The detail panels render structured fields: identity, menu source, interface list, weak/low-CCQ client lists, warnings, evidence. **For a populated dataset this would be readable.** With the lab returning `skipped` everywhere, the operator sees only the explanatory `SkippedBox`. That is honest but unhelpful.

### 6.5 Dry-run + read-only safety status

- Read-only kinds always pass `DryRun=true` in `handlers_actions.go`. The action implementations enforce read-only at the adapter level (`mikrotik` allowlist, `denyMutationSegments`, `denyMutationTokens`).
- Destructive `frequency_correction` is gated by **two** master switches (legacy const + Phase 10B `MemoryToggle.Enabled()`), neither of which is on. The Execute code exists in `frequency_correction.go` but is **not registered** with the server's `netActions` registry. Therefore even a curl through the destructive endpoints terminates at `gate_fail` (master closed) or `execute_not_implemented` (registry stub).

This is the safest possible state. It is also why the product is not yet usable for corrective action — that wiring is intentionally deferred to Phase 10E PR-B.

---

## 7. Product Gap Table

| # | Intended capability | Current implementation | UI? | Backend? | Real device evidence? | Operator usable? | Gap severity |
|---|---|---|---|---|---|---|---|
| 1 | Web dashboard answering "what's broken right now" | `/` KPI cards from executive_summary | yes | yes | partial | **no** (depends on scoring/work-orders the lab has not produced) | HIGH |
| 2 | AP / link / bridge / customer device inventory in UI | `/ag-envanteri` 12 stat cards + filtered table | yes | yes | yes (893 rows live) | partial — visible but >99% Bilinmeyen | HIGH |
| 3 | Device classification understandable to non-dev operator | label set OK; per-row evidence not surfaced | yes | yes (`device_category_evidence`) | yes | no — operator cannot self-explain "why Bilinmeyen?" | HIGH |
| 4 | Action buttons that work | per-row 4 buttons + 2 toolbar buttons | yes | yes | yes (HTTP wiring proven) | partial — all four return `skipped` against this lab | HIGH |
| 5 | Identifying bad signal / bad link / problem CPEs | scoring engine exists | partial (`/musteriler`) | yes | **no** in lab | **no** — no telemetry ingested | BLOCKER for product MVP |
| 6 | Scheduled read-only checks visible in UI | `/planli-kontroller` Phase 5 surface | yes | yes (worker scheduler) | yes (Phase 5 historical) | partial — Phase 9 v2 actions not in scheduler catalog | MEDIUM |
| 7 | Reports / executive summary in UI | `/raporlar`, `/raporlar/yonetici-ozeti` | yes | yes | partial | partial — depends on data | MEDIUM |
| 8 | Audit trail surfaced to operator | `audit_logs` table + Phase 10C lifecycle | partial | yes | yes | no read UI | MEDIUM |
| 9 | Controlled corrective frequency_correction | code exists in Phase 10E | no | yes (registry stub) | no — not wired into server | no — by design until PR-B preconditions met | LOW (intentional freeze) |
| 10 | Honest "what is closed vs. what is open" | TASK_BOARD says 10D active; reality 10F-A | n/a | n/a | n/a | misleading | HIGH (process gap) |
| 11 | `.env` hygiene — no real secret in tree | `.env` in .gitignore but contains live lab credentials | n/a | n/a | n/a | safe (gitignored) but risky in lab share | MEDIUM |
| 12 | Frequency recommendations page | route exists, handler is `stub("frequency_recommendations.list")` | yes | no (stub) | no | no | MEDIUM |
| 13 | "Why was this device classified this way?" drill-down | `device_category_evidence` populated, no UI panel | no | yes | yes | no | HIGH |

---

## 8. False-Closure Inventory

The following claims should be re-stated as **engineering-closed only**, not product-closed.

| Source | Claim | Engineering reality | Product reality |
|---|---|---|---|
| `current.md` line 49 | "WispOps engineering scope kapandı" | Phases 10A→10F-A landed | Operator-facing MVP has not been stood up for `194.15.45.62`. |
| `MEMORY.md` line 39 | Phase 10B "MERGED ✅" | true | safety chassis only; preflight + toggle + window CRUD; nothing operator-facing changed in product surface |
| `MEMORY.md` line 45 | Phase 10E PR-A "MERGED ✅ (lab-only, no wiring)" | true | code on main; **does not run because not registered**; product cannot do frequency correction |
| Multiple session reports | "0 destructive succeeded, 0 mutation cmd, 0 secret leak, 0 raw MAC" | true and verifiable from DB | safety invariants — not a product readiness signal |
| `PHASE_008_OPERATOR_SMOKE_RESULT.md` | "Tüm engineering + PG + SSH recovery + test-connection + 1. discovery run tamamen PASS" | true | classification yield 1/893; product-wise this is data, not insight |
| `TASK_BOARD.md §1 Current Status` | "Active: Faz 10D" | stale | Phase 10F-A is on main; doc has not caught up |

**Recommendation:** every "MERGED ✅" line should be paired with a "**Engineering-closed**" qualifier and, where applicable, an "**Operator-usable: not yet (because …)**" note. The MVP rescue plan tracks the operator-usable axis explicitly.

---

## 9. What Actually Works Today

(Confirmed by code reading + artifact review during this audit.)

1. **Live SSH discovery against `194.15.45.62`** — `Test Connection` returns reachable, identity, duration; `Run Discovery` ingests ~893 rows into `network_devices` with stable dedupe across re-runs.
2. **The 14-route Next.js frontend builds and type-checks** — `apps/web/tsc --noEmit` RC=0 in this audit pass.
3. **The Go backend builds, vets clean, and passes 16 packages of tests** — see §11.
4. **Read-only network actions are wired front-to-back.** Pressing a row button POSTs to a real handler that dispatches a real Action.Execute over SSH. The action contract enforces "no fake data — return `skipped` with reason" when the menu doesn't exist.
5. **The safety chassis is real and fail-closed.** Two layers of master switch, audit catalog with 20+ destructive lifecycle events, RBAC capability gating, maintenance windows with disable lifecycle, idempotency keys.
6. **Scoring engine and work-order schema exist and are tested** — 138 networkactions tests, plus scoring/workorders/reports test packages.
7. **Migrations 000001 → 000013 are in tree, all transactional + idempotent (per session reports)**.
8. **CI / lab smoke artifacts exist** for Phase 8, 8.1, 9, 9 v2, 9 v3, 10A, 10B, 10C, 10D, 10E PR-A — concrete log files in repo root (`.smoke_*`).

---

## 10. What Does Not Work Today

1. **Operator opens `/` and sees nothing useful.** Without scoring runs against ingested telemetry, all 8 KPI cards display "—" or report `data_insufficient`.
2. **`/ag-envanteri` shows 99.6%+ Bilinmeyen** for the only live dataset that exists. The classifier is conservative; the source is thin. The operator has no path to ask "why?".
3. **Per-row actions return `skipped` on every device** because the lab Dude is not itself a wireless RouterOS / bridge. No real wireless RouterOS test target has been provisioned in this lab.
4. **No customer/CPE signal data has ever been ingested in this lab DB**, so `/musteriler` is empty, the scoring engine has nothing to score, and `/is-emirleri` has no candidates to promote.
5. **`/frekans-onerileri` is a stub handler** wired to `stub("frequency_recommendations.list")` in `routes.go`. The page renders but returns no data.
6. **`/planli-kontroller` does not yet schedule the four Phase 9 v2 actions.** The `JobCatalog` predates Phase 9; the four read-only actions are run-on-button-press only.
7. **Classification evidence is collected per device but not surfaced** — there is no "why Bilinmeyen?" drill-down.
8. **Audit log has no operator UI** — only the Phase 10C `/lifecycle/<id>` JSON endpoint.
9. **Frequency correction cannot run** — by design (Phase 10E PR-B preconditions unmet).
10. **TASK_BOARD.md is stale by two phases**, which makes triage ambiguous for a new contributor.

---

## 11. Quality Gates — Exact Command Results

**Date:** 2026-04-29 (this audit pass)
**Environment:** Linux 6.x sandbox; Go 1.22.5 fetched into `/tmp/go`; Node 22.22.0; `apps/web/node_modules` already populated; root `node_modules` not present.

| Gate | Command | RC | Output |
|---|---|---|---|
| `gofmt` | `gofmt -l .` | 0 | empty (no files require formatting) |
| `go vet` | `go vet ./...` (after `go mod download`) | 0 | empty |
| `go build` | `go build ./...` | 0 | empty |
| `go test` (short) | `go test -count=1 -short ./...` | 0 | 16 packages PASS, 16 packages no test files. No failures, no skips. |
| `go test` networkactions count | `go test -short ./internal/networkactions/... -v \| grep -c "^--- PASS"` | n/a | **138** |
| `go test` dude count | `go test -short ./internal/dude/... -v \| grep -c "^--- PASS"` | n/a | **38** |
| Frontend typecheck | `apps/web && tsc --noEmit` | 0 | empty |
| Frontend `next build` | `apps/web && next build` | n/a | **NOT RUN** in this pass — sandbox bash 45 s ceiling repeatedly tripped during compile-and-prerender. Last reported green build is documented in `current.md` (post-Phase 10F-A). |
| Frontend `npm test` | n/a | n/a | **No test runner configured** in `apps/web/package.json` (no `test` script). |

**Honest blocker:** the sandbox cannot drive a full `next build` within the per-call wall-clock; this audit relies on the historical PR-A / PR-A-merge artifacts plus the in-pass `tsc --noEmit` PASS to assert the frontend compiles.

---

## 12. Recommended Next Implementation Phase

The next phase **must not be engineering-shaped** (e.g. 10E PR-B, 10F-B). It must be **product-shaped**: an operator-usable MVP for `194.15.45.62`. See `docs/MVP_RESCUE_PLAN.md` for the five-phase rescue plan (R1 → R5).

The minimum viable product target is **R1 + R2 + R3 in sequence**:

1. **R1 — Real dashboard + device inventory UI rework**, including a "why Bilinmeyen?" drill-down and a primary "Operasyon Paneli" that does not depend on scoring data the lab has never had.
2. **R2 — Real Dude classification correction**: ingest enough signal so that fewer than 30% of devices land in Bilinmeyen on this exact dataset. If that means classifier rules need re-shaping for thin-source environments, do it.
3. **R3 — Operator action buttons with readable results on a real wireless RouterOS lab target** (operator must provision; without it the buttons stay honestly `skipped`).

R4 (scheduled checks) and R5 (controlled corrective actions) come after.

---

## 13. Exact Files Changed by This Audit

This audit pass introduces **no source code changes**. Three documents are added or rewritten:

- `docs/PRODUCT_REALITY_AUDIT.md` (this file) — created.
- `docs/MVP_RESCUE_PLAN.md` — created.
- `TASK_BOARD.md` — amended at top with "Audit verdict 2026-04-29" block and corrected current-phase pointer; existing per-phase history preserved.

---

## 14. Exact Commands Run In This Pass

```bash
# repo orientation
git log --oneline -20
git status -s

# layer sweeps
find apps/web/src -name "page.tsx"
find internal/dude -type f
find internal/networkactions -type f
find apps/api -name "*.go"

# evidence
grep -rln "194\\.15\\.45\\.62" internal apps docs migrations .env*
grep -E "frequency_check|ap_client_test|link_signal_test|bridge_health_check" \
     apps/web/src/app/ag-envanteri/Client.tsx apps/web/src/app/aksiyonlar/Client.tsx
grep -B2 -A8 "buildAction" apps/api/internal/http/handlers_actions.go

# quality gates
export PATH=/tmp/go/bin:$PATH GOCACHE=/tmp/gocache GOMODCACHE=/tmp/gomodcache
go mod download
gofmt -l .                     # RC=0, empty
go vet ./...                   # RC=0, empty
go build ./...                 # RC=0, empty
go test -count=1 -short ./...  # RC=0, 16 ok / 16 no-test
go test -short ./internal/networkactions/... -v | grep -c "^--- PASS"  # 138
go test -short ./internal/dude/...           -v | grep -c "^--- PASS"  # 38
cd apps/web && ./node_modules/.bin/tsc --noEmit  # RC=0
# next build — NOT RUN (see §11)
```

---

## 15. Honest Blockers

1. **No real wireless RouterOS test target.** Without one, `frequency_check` / `ap_client_test` / `link_signal_test` / `bridge_health_check` will never produce useful WISP signal — they will keep returning `skipped`. This is the single biggest barrier between "engineering-complete" and "operator-usable".
2. **No telemetry-source other than Dude discovery has been wired up in this lab.** Phase 3 RouterOS adapter exists but no live polls have run. Without polls, scoring cannot run, customers do not appear with issues, and the whole `/musteriler` / `/is-emirleri` / `/raporlar/yonetici-ozeti` chain is empty.
3. **Phase 10E PR-B preconditions are operator-side, not engineer-side.** The four-line freeze in `current.md` (real wireless lab target + maintenance window + rollback owner + operator approval path) is operational, not technical. Until those exist, the destructive code path on main cannot be wired.
4. **`.env` in working tree contains live lab credentials.** It is `.gitignore`d (verified) so not committed, but the file is shared via the F:\ workspace. Recommendation: rotate the lab `bariss` password after the rescue plan completes; tag the file `.env.lab.local` and require operator-side decryption.
5. **Sandbox cannot run `next build` within the bash wall-clock.** Frontend production-build evidence in this pass relies on prior PR-A merge artifacts.
6. **TASK_BOARD has been treating each engineering merge as a victory.** It is not — until R1-R3 land, every merge is incremental engineering, not product progress. The board now reflects this.

---

**End of audit.**
