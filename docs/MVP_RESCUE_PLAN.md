# MVP Rescue Plan — WISP Ops Center

**Plan ID:** WISP-MVP-RESCUE-001 v1.0.0
**Date:** 2026-04-29
**Companion document:** `docs/PRODUCT_REALITY_AUDIT.md`
**Branch HEAD this plan was written against:** `main` @ `0b987d1`

> Goal: bring the project back to the **intended product** — a simple, usable web-based WISP Operations Center for `194.15.45.62`. No new engineering excursions until the operator can actually use the browser end-to-end.

## 0. Operating Rules For The Rescue

1. **Product first, engineering second.** Each phase is judged by whether the operator can do something new from the browser, not by test count or audit-event count.
2. **No master-switch flips.** The legacy `DestructiveActionEnabled` and the `MemoryToggle` stay closed through R1–R4. R5 is the only phase that touches them, and only after its four explicit preconditions are met.
3. **Honest empty states.** If a screen depends on data that does not exist, the screen says exactly that, with the SQL-shape the operator can run to confirm.
4. **One small PR per sub-step.** Each phase below is a sequence of small PRs, not one big phase PR.
5. **Stop and reassess if any phase reveals the assumption was wrong.** Re-audit before continuing.

## Phase Sequencing

```
R1  Operator-usable dashboard + inventory UI    ← starts here
R2  Real Dude classification correction
R3  Real wireless test target + readable results
R4  Scheduled checks + reports
R5  Controlled corrective action (frequency_correction wiring)
```

R1–R2 unblock the operator's "what is on my network?" question. R3–R4 answer "what is broken?". R5 finally answers "fix this one safely". Skipping ahead is forbidden — each later phase has prerequisites that earlier phases produce.

---

## R1 — Operator-usable dashboard + inventory UI

**Objective:** an operator who opens `/` (and `/ag-envanteri`) sees something *useful* against the only data we have today (Dude discovery rows from `194.15.45.62`).

### R1.1 Re-target `/` to a real "Operasyon Paneli"

- Stop sourcing all 8 KPI cards from `executive-summary` (which depends on scoring).
- Add `GET /api/v1/dashboard/operations-panel` that returns:
  - `discovery.last_run_at` + `discovery.devices_total` + `discovery.devices_classified` + `discovery.devices_unknown` + `discovery.with_mac_count`
  - `actions.last_24h_success` + `last_24h_skipped` + `last_24h_failed`
  - `safety.master_switch_state` + `safety.active_maintenance_windows`
  - `health.api_db_ok` + `health.api_dude_reachable`
- Render this on `/` instead of the scoring KPIs. Move the scoring KPIs into a "Müşteri Sağlığı" section that is visibly disabled with the line *"Skor verisi yok — Phase R3'te eklenecek"*.

**Acceptance:** opening `/` against the lab DB shows non-empty, real numbers within 200 ms.

### R1.2 "Why is this Bilinmeyen?" drill-down

- Add `GET /api/v1/network/devices/<id>/evidence` returning the rows of `device_category_evidence` for that device, plus the merged source list from `discovery_runs` for the latest run.
- Add a `Cihaz Detayı` modal opened from the `/ag-envanteri` table row click that renders evidence per source (`dude_device`, `ip_neighbor`, `dude_probe`, `dude_service`), the heuristic decision, the conflict penalty, and the final category + confidence.

**Acceptance:** operator clicks a Bilinmeyen row, reads in plain Turkish why the classifier did not commit. Closes the modal, picks another row, repeats.

### R1.3 Inventory filters that actually filter

- Verify each existing checkbox (`unknown`, `low_confidence`, `has_mac`, `enriched`) round-trips against the live 893-row dataset. Fix any that returns `total=0` for non-empty filter combinations.
- Add a `Discovered today / yesterday / last 7d` time filter — without it, the operator cannot focus on freshly seen devices.

**Acceptance:** every filter combination returns at least one row when the underlying SQL has a row to return; `total` matches `len(data)` exactly.

### R1.4 Empty-state hardening

- For every card on every page that depends on data we don't have yet (scoring, work-orders), render an explicit `data_insufficient` panel with one sentence describing what is missing and one click-through to the relevant ingest control. No more silent "—".

**Acceptance:** the operator never wonders "is this broken or is there no data?".

### R1 size estimate

- 3 small backend endpoints, 2 frontend route refactors, 1 modal component, ~10 new tests.
- Single feature branch `phase/r1-operator-dashboard-and-evidence`.

---

## R2 — Real Dude classification correction

**Objective:** on the live `194.15.45.62` dataset, **fewer than 30%** of devices land in `Bilinmeyen`. Classification is conservative-but-useful, not conservative-and-empty.

### R2.1 Source-yield audit

- Run a targeted query: per source (`dude_device`, `ip_neighbor`, `dude_probe`, `dude_service`), how many of the 893 devices contributed *what* signal (MAC, host, platform, service)? Output as a static table in `docs/PHASE_R2_SOURCE_YIELD.md` — no fake fixtures, real lab numbers only.
- Identify which Dude commands carry the missing signal we need. Candidates: `/dude/network-map`, `/dude/agent`, `/dude/notification`, `/system/routerboard/print`, `/ip/arp/print`. Pick the one with the highest predicted yield, prove it on the live device, then add it as a 6th enrichment pass.

**Acceptance:** post-R2.1, the source-yield table is filled with real numbers, and the team has picked an evidence-backed 6th source.

### R2.2 Classifier reshape for thin-source data

- Today the classifier needs strong evidence to commit. For thin-source environments, add a *secondary* tier of weak heuristics that can lift `Bilinmeyen → Router/CPE/AP` with `confidence ≤ 50` (so they still surface as low-confidence in the UI) when the primary evidence is missing but the name pattern is highly suggestive (`-AP-`, `-PtP-`, `-CPE-`, `-OREN-`, etc.).
- All such heuristic decisions must record an explicit `weak_name_pattern` evidence row with the regex that matched. R1.2 will surface it.

**Acceptance:** post-R2.2 lab smoke shows ≥70% of devices with a non-Unknown category. Every weak-tier classification has an evidence row explaining itself.

### R2.3 Confidence model + UI

- Show `confidence` as a small bar inside each `/ag-envanteri` row, color-coded: 0–30 red, 31–60 amber, 61+ green. Allow `low_confidence` filter to keep working but make the color the primary visual signal.

**Acceptance:** an operator scanning the inventory page sees three colors and can answer "which of these classifications should I trust?" in under five seconds.

### R2 size estimate

- 1 source-yield script (read-only), 1 classifier rule additions, 1 evidence persistence path, UI bar component.
- Single branch `phase/r2-classification-correction`.

---

## R3 — Real wireless test target + readable action results

**Objective:** an operator clicks Frekans / AP Client / Link Signal / Bridge Health on a row and sees real wireless telemetry, not `skipped`.

### R3.0 Operator-side prerequisite (cannot be skipped)

- Provision a **real wireless RouterOS device** (e.g. RB951, hAP, mAP, Audience) in the lab, reachable from the API server, with read-only credentials in `MIKROTIK_DUDE_USERNAME` / equivalent. Without this, R3 cannot complete.
- The device must have at least one wireless interface and one bridge for the four actions to produce non-`skipped` output.

### R3.1 Run all four actions against the new target

- Wire the new target as a row in `network_devices` (manual seed allowed — this is a lab).
- From `/ag-envanteri`, run each of the four actions. Capture the result panel screenshots into `docs/PHASE_R3_ACTION_EVIDENCE.md`.

**Acceptance:** each of the four actions produces a non-skipped, structured result against the real wireless target. No fake values.

### R3.2 Result panel polish

- Today panels show identity, menu, interfaces, weak/low-CCQ clients, warnings. Add:
  - A summary banner ("AP'de N istemci, en kötü sinyal X dBm, ortalama CCQ Y%, Z uyarı") at the top of every panel.
  - A "İlk müdahale önerisi" line that maps the result to a one-sentence actionable next step. (Read-only, advisory — not an action button yet.)

**Acceptance:** an operator with no developer background can read each panel and say "this AP is fine / this link is bad / this bridge has flapping ports" without help.

### R3.3 Action history surface

- `/aksiyonlar` already lists runs. Add a per-device tab that shows the timeline of actions for one device (across all kinds), so the operator can see drift.

**Acceptance:** clicking on a device in `/ag-envanteri` and switching to "Aksiyon Geçmişi" tab shows a chronological list with confidence trends.

### R3 size estimate

- Operator-side lab provisioning (out-of-band).
- Backend: 1 read-only handler for per-device action history.
- Frontend: panel summary banner + advisory line + history tab.
- Branch `phase/r3-readable-action-results`.

---

## R4 — Scheduled checks + reports

**Objective:** the four read-only actions can be scheduled, results accumulate, and a daily / weekly report is generated automatically.

### R4.1 Add the four read-only actions to `JobCatalog`

- The Phase 5 `JobCatalog` predates Phase 9; add `frequency_check`, `ap_client_test`, `link_signal_test`, `bridge_health_check` as schedulable jobs with risk policy `read_only_low`.
- Default schedule template: every classified AP gets `frequency_check` + `ap_client_test` every 6 hours; every classified Link gets `link_signal_test` every 4 hours; every classified Bridge gets `bridge_health_check` every 6 hours.
- Operator UI at `/planli-kontroller` can create + pause + resume per-device schedules.

**Acceptance:** scheduled actions run without operator clicks; results land in `network_action_runs`; UI shows next-run / last-run.

### R4.2 Heatmap report

- A new `/raporlar/sinyal-haritasi` page that aggregates `ap_client_test` / `link_signal_test` results across the last N hours and renders signal/CCQ heatmaps per AP and per link.
- Backed by `GET /api/v1/reports/signal-heatmap?since=...&group_by=ap|link`.

**Acceptance:** operator opens the page, sees which APs / links degraded over time without writing SQL.

### R4.3 Wire up `/frekans-onerileri`

- Replace the `stub("frequency_recommendations.list")` handler with a real one that derives recommendations from accumulated `frequency_check` results (per-AP histogram of channels, occupancy, recommended channel + confidence).
- This is **read-only advisory** only. Acting on it remains R5.

**Acceptance:** page shows real recommendations against the lab's wireless target.

### R4 size estimate

- 4 new jobs in `JobCatalog`, 1 new aggregation handler, 1 new heatmap page, 1 stub-replacement handler.
- Branch `phase/r4-scheduled-checks-and-reports`.

---

## R5 — Controlled corrective action (frequency_correction wiring)

**Objective:** the operator can apply a frequency correction recommendation through the browser, with full safety chassis, on the lab wireless target.

### R5.0 Preconditions (the four-line freeze in `current.md`)

All four must be satisfied before R5 starts. None can be inferred or assumed — each is an operator-side artefact:

1. ✅/❌ Real wireless RouterOS lab target (provisioned in R3).
2. ✅/❌ Active `MaintenanceWindow` row covering this target.
3. ✅/❌ Named rollback owner with physical/SSH access to the device.
4. ✅/❌ Two-layer master-switch approval path (legacy const flip + provider toggle ON), with documented signer for each.

### R5.1 Wire `frequency_correction` into the registry

- In `apps/api/internal/http/server.go`, call `networkactions.RegisterFrequencyCorrection(s.netActions, ...)` after `s.netActions = networkactions.NewRegistry()`.
- Implement `mikrotik.FrequencyCorrectionWriter` that wraps `SnapshotFrequency` + `SetFrequency` over `ExecRead` / `ExecWrite`.

### R5.2 Lab smoke against the real wireless target

- Execute the 9-row matrix from `docs/PHASE_010E_DESTRUCTIVE_EXECUTE.md` against the real device, with the master switch on, in the active maintenance window.
- Capture artefacts: snapshot result, write result, verify result, audit lifecycle, post-run state. Compare to expectations row by row.
- One run **must** include a deliberately wrong target frequency to prove auto-rollback.

**Acceptance:** all 9 rows pass; auto-rollback fires on the deliberately wrong row; audit lifecycle is complete; safety invariants stay green.

### R5.3 Operator UI control

- Add a "Frekans Düzelt" button on `/frekans-onerileri` rows. Clicking opens a confirmation dialog with: target device, current frequency, recommended frequency, snapshot diff, RBAC actor, idempotency key, expected duration. Confirms POST to the destructive endpoint.
- Result panel renders the lifecycle (pre-gate → confirmed → execute_attempted → succeeded/rolled_back) in plain Turkish.

**Acceptance:** an operator can apply one corrective action through the browser, see the lifecycle, and roll back.

### R5.4 Master switch cool-down

- Post-action, the master switch automatically returns to `enabled=false` after a configurable cool-down (default 60 minutes). Documented in `docs/PHASE_R5_OPERATOR_GUIDE.md`.

**Acceptance:** the destructive path is closed by default again after every successful action.

### R5 size estimate

- 1 wiring change in `server.go`, 1 mikrotik adapter helper, 1 destructive button + dialog component, lifecycle renderer.
- Branch `phase/r5-frequency-correction-wiring`.

---

## Tracking

For each phase, the gating exit criteria are mirrored in `TASK_BOARD.md §1 Current Status` so an outside observer (auditor, new engineer, operator) can read what is true and what is open without consulting memory files.

**Process change carried into the rescue:** every PR description must end with a one-line **Operator-usable delta** statement — what an operator can do in the browser today that they could not do before this PR. PRs whose Operator-usable delta is "none" are engineering-only and must be tagged `engineering-only` (and gated against R1–R5 progress count).

---

## Out-of-scope For The Rescue

- Multi-tenant RBAC isolation (Phase 10F-B+).
- KMS / Vault rotation.
- TLS-everywhere uplift.
- SOC2 / KVKK external compliance work.
- Mimosa parity with MikroTik in actions.

These are real, important, and tracked in the existing hardening backlog. They are not what makes the product usable for `194.15.45.62` today, so they sit behind R1–R5.

---

**End of plan.**
