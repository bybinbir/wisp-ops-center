# Phase 10D — Destructive happy-path lifecycle (no execution)

Status: planned (branch `phase/010d-destructive-happy-path-lifecycle`).
Depends on: Phase 10C merged (main `927c711`).

## Why

Phase 10C wired the destructive lifecycle but nailed the runtime to a single invariant: **live execution NEVER starts.** Even with toggle ON + maintenance window active + RBAC granted, the `confirm=true + gate pass` branch emitted `destructive_denied` and finalized the run as failed. That invariant proved the gate, the audit catalog, and the lifecycle persistence behave correctly *when the master switch is closed.*

Phase 10D opens exactly one new branch in the runner: when toggle is ON + window is active + the principal holds `CapabilityDestructiveExecute` AND `Confirm=true`, the runner SHOULD reach `action.Execute`. The destructive registry still maps every Kind to a stub returning `ErrActionNotImplemented`, so **cihaza tek byte yazılmaz**. What Phase 10D proves:

1. The gate's pass branch is reachable and audit-complete.
2. `action.Execute` is invoked under controlled conditions.
3. The stub returns `ErrActionNotImplemented` and the runner finalizes the run as failed with `error_code=action_not_implemented`.
4. Two new audit events fire in order: `network_action.execute_attempted` (immediately before the Execute call) and `network_action.execute_not_implemented` (immediately after the stub returns).
5. Even with the master switch ON, all safety invariants hold: 0 destructive succeeded, 0 mutation cmd, 0 secret leak, 0 raw 6-octet MAC.

The legacy `var DestructiveActionEnabled = false` stays untouched — Phase 10D uses the provider toggle (`MemoryToggle` / `PgToggleStore`). Flipping the legacy global is reserved for Phase 10E, behind a separate review.

## Non-Goals (deferred to Phase 10E)

- Real `Execute` implementation for any Kind. `frequency_correction` et al stay registry stubs.
- Real device I/O, SSH dial, RouterOS write commands.
- Production toggle flip (operator decision; lab only in Phase 10D).
- New HTTP endpoints. Phase 10D reuses `/destructive/{kind}/dry-run`, `/destructive/{kind}/confirm`, `/lifecycle/{run_id}`.
- Migration changes. Phase 10C migration `000013` supplies all schema needed.

## Components

### `internal/networkactions/audit_destructive.go` (extended)

Two new constants in `DestructiveAuditAction`:

- `AuditActionExecuteAttempted = "network_action.execute_attempted"` — emitted in the runner immediately before the `action.Execute` call. Metadata: `run_id`, `action_type`, `correlation_id`, `target_host`, `intent`, `kind_destructive=true`.
- `AuditActionExecuteNotImplemented = "network_action.execute_not_implemented"` — emitted when `Execute` returns `ErrActionNotImplemented`. Metadata: same as Attempted + `error_code=action_not_implemented`.

`DestructiveAuditCatalog()` extended from 10 to **12 events**. A catalog test asserts the new entries are present and stable so a downstream log consumer cannot break silently.

### `apps/api/internal/http/handlers_destructive.go` (rewired)

The `confirm=true + gate pass` branch is the only structural change. Phase 10C version (lines ~457-481):

```go
// Phase 10C: gate passed but live execution is blocked unconditionally.
s.audit(...AuditActionDestructiveDenied...)
_ = s.actionRepo.FinalizeRun(..., StatusFailed, error_code="live_start_blocked")
```

Phase 10D replacement:

```go
// Phase 10D: gate passed, Execute is reachable. The destructive
// registry still maps every Kind to a stub returning
// ErrActionNotImplemented — Execute runs but cannot mutate.
s.audit(...AuditActionExecuteAttempted...)

action, ok := s.actionRegistry.Lookup(kind)
if !ok {
    // Registry MUST have a stub for every Kind. Defensive only.
    s.audit(...AuditActionExecuteNotImplemented... reason="registry_miss")
    s.actionRepo.FinalizeRun(..., StatusFailed, error_code="registry_miss")
    return
}

execCtx, execCancel := context.WithTimeout(context.Background(), 30*time.Second)
defer execCancel()
result, err := action.Execute(execCtx, networkactions.Request{
    Kind:           kind,
    TargetHost:     host,
    DryRun:         false,
    CorrelationID:  correlationID,
    IdempotencyKey: idempotencyKey,
    Intent:         intent,
})

if errors.Is(err, networkactions.ErrActionNotImplemented) {
    s.audit(...AuditActionExecuteNotImplemented...)
    s.actionRepo.FinalizeRun(..., StatusFailed,
        error_code="action_not_implemented",
        error_message="Phase 10D ships lifecycle wiring; Execute reserved for Phase 10E.")
    return
}

// Phase 10D MUST NOT reach the success branch. Every destructive
// Kind is a stub. Reaching here means a Kind was implemented
// before its safety review.
panic("phase 10d invariant: destructive Execute returned success — review required")
```

`live_start_blocked` stays in its current pre-gate position (handlers_destructive.go:346-362). Semantically it now means *"operator asked for live destructive execution; this audit row pins the intent regardless of what the gate decided."* A rename to `live_start_attempted` is reserved for Phase 10E when Execute can actually succeed.

## Lifecycle event order (Phase 10D happy-path)

For `POST /destructive/frequency-correction/confirm` with toggle ON + window ACTIVE + RBAC granted + idempotency new:

1. `network_action.confirmed` (handler, before runner spawn)
2. `network_action.rollback_metadata_recorded` (handler, before runner spawn)
3. `network_action.live_start_blocked` (runner, before gate; Phase 10C invariant preserved)
4. *(gate runs and passes — no audit event for "gate pass" by design; absence of `gate_fail` is the signal)*
5. `network_action.execute_attempted` (runner, before Execute call) **← Phase 10D NEW**
6. `network_action.execute_not_implemented` (runner, after Execute returns ErrActionNotImplemented) **← Phase 10D NEW**
7. `network_action.finish` (repo, terminal status=failed/error_code=action_not_implemented)

Total per happy-path confirm run: **6 destructive lifecycle events** + 1 generic finish event.

Dry-run path is unchanged from Phase 10C: gate pass → `dry_run` → `finish(succeeded/confidence=30)`. Dry-run NEVER calls Execute.

## Senaryo matrix (Phase 10D smoke)

| # | Toggle | Window | RBAC | Mode | Idempotency | Expected Lifecycle Tail |
|---|--------|--------|------|------|-------------|-------------------------|
| 1 | OFF    | -      | granted | confirm | new    | live_start_blocked → gate_fail(destructive_disabled) → destructive_denied → finish(failed) |
| 2 | ON     | NONE   | granted | confirm | new    | live_start_blocked → maintenance_window_denied → gate_fail → destructive_denied → finish(failed) |
| 3 | ON     | ACTIVE | denied  | confirm | new    | live_start_blocked → rbac_denied → gate_fail → destructive_denied → finish(failed) |
| 4 | ON     | ACTIVE | granted | confirm | new    | live_start_blocked → **execute_attempted** → **execute_not_implemented** → finish(failed/action_not_implemented) **← NEW** |
| 5 | ON     | ACTIVE | granted | confirm | reused | idempotency_reused (no runner invocation) |
| 6 | ON     | ACTIVE | granted | dry-run | new    | dry_run → finish(succeeded/confidence=30) |
| 7 | OFF    | -      | granted | dry-run | new    | gate_fail(destructive_disabled) → destructive_denied → finish(failed) |
| 8 | ON     | NONE   | granted | dry-run | new    | maintenance_window_denied → gate_fail → destructive_denied → finish(failed) |
| 9 | ON     | ACTIVE | denied  | dry-run | new    | rbac_denied → gate_fail → destructive_denied → finish(failed) |

Senaryo 4 is the Phase 10D headline — the first time Execute is reached on an actual run. Senaryos 1-3, 5-9 are regression coverage of Phase 10C invariants under the new code path.

## DB invariant assertions (post-smoke)

```sql
-- 0 destructive succeeded (Phase 10D MUST preserve this)
SELECT COUNT(*) FROM network_actions
WHERE action_type IN ('frequency_correction', /* future destructive Kinds */)
  AND status = 'succeeded';
-- Expected: 0

-- 0 mutation command in result jsonb (no SSH dial in destructive runs)
SELECT COUNT(*) FROM network_actions
WHERE result::text ~* E'(^|\\W)(set|enable|disable|remove|add|reboot|reset)\\s';
-- Expected: 0

-- 0 raw 6-octet MAC in audit metadata or result
SELECT COUNT(*) FROM audit_logs
WHERE metadata::text ~ E'[0-9a-f]{2}(:[0-9a-f]{2}){5}';
-- Expected: 0

-- Phase 10D NEW event counts (per smoke run of senaryo 4)
SELECT action, COUNT(*) FROM audit_logs
WHERE action IN ('network_action.execute_attempted',
                 'network_action.execute_not_implemented')
GROUP BY action;
-- Expected: 1 each per senaryo 4 run; 0 for senaryos 1-3, 5-9.

-- error_code distribution for destructive runs
SELECT error_code, COUNT(*) FROM network_actions
WHERE action_type = 'frequency_correction'
GROUP BY error_code;
-- Expected (after full smoke): destructive_disabled, no_active_maintenance_window,
--                              rbac_denied, action_not_implemented (senaryo 4),
--                              live_start_blocked (Phase 10C residual).
```

## Test plan

New test files:

- `internal/networkactions/phase10d_runtime_test.go` — runtime branch decomposition: assert `execute_attempted` precedes the Execute call; assert `execute_not_implemented` fires only when Execute returns `ErrActionNotImplemented`; assert the panic guard fires if Execute returns nil error (defensive — should be unreachable).
- `internal/networkactions/phase10d_audit_catalog_test.go` — `DestructiveAuditCatalog()` includes the 12 entries; the two new constants are present, are stable strings, and survive a round-trip through `audit.Action`.

Extended:

- `internal/networkactions/phase10c_lifecycle_test.go` — regression: senaryos 1-3, 5-9 produce identical event sequences to Phase 10C.
- `apps/api/internal/http/...` integration test (if one already exists for the destructive HTTP surface) — happy-path POST receives 202 + run_id; lifecycle GET returns the 6-event Phase 10D sequence.

Hedef test diff: Phase 10C **108** → Phase 10D **~120-123** (+12-15 yeni test). All existing tests MUST continue to pass.

## Migration plan

**No new migration required.** Phase 10C migration `000013` supplies all schema needed:
- `network_actions.idempotency_key` partial unique index ✓
- `network_actions.intent` ✓
- `network_actions.rollback_note` ✓
- `network_action_role_grants` ✓

If a defensive version-stamp migration is wanted (`000014_phase_10d_marker.sql` — INSERT a row into a metadata table), it stays optional and gated on a separate review.

## Quality gates

- `gofmt -l .` — clean.
- `go vet ./...` — clean.
- `go test ./...` — full repo PASS, including the 12-15 new tests.
- `go build ./...` — clean.
- `cd apps/web && npm run build` — green.
- Lab smoke: 9-row matrix, all senaryos confirm expected lifecycle tail.
- DB invariants post-smoke: zeros above hold.

## Rollback plan

If Phase 10D smoke shows an unexpected audit event ordering, an Execute call leaks a mutation, or the panic guard fires:

1. Revert the merge commit on main. Branch is squash-merged so revert is one command (`git revert -m 1 <sha>`).
2. Phase 10C state is restored: `confirm=true + gate pass` re-emits `destructive_denied + live_start_blocked` and finalizes failed.
3. No DB schema changes to roll back. No data loss.
4. Audit catalog drops back to 10 events; consumers that started parsing `execute_attempted` / `execute_not_implemented` see 0 rows after revert (graceful — every consumer MUST treat absence as zero).

## Phase 10E handed forward

- Real `Execute` implementation for `frequency_correction`: read current frequency via SSH allowlist (`/interface wireless print detail`), compute target frequency, write via `/interface wireless set frequency=<MHz>`. Per-device lock (already in framework). Pre-write verification (frequency in allowed channel set). Post-write verification (read back + compare). Auto-rollback if customer signal degrades within N seconds.
- New audit events: `network_action.execute_succeeded`, `network_action.execute_failed`, `network_action.rollback_initiated`, `network_action.rollback_completed`.
- Operator-facing endpoint to flip `var DestructiveActionEnabled` (today: const). Likely an env var read at boot — flipping requires a deploy, not a runtime toggle.
- Lab smoke: real RouterOS device, real frequency change, verification, rollback drill.
- Production gate: separate decision; Phase 10E lab pass is necessary but not sufficient. Production toggle flip is a Phase 10F decision.

---

**Definition of Done (Phase 10D):**

1. PR opened against `main`, base `927c711`.
2. Two new audit constants added; catalog returns 12 entries.
3. `confirm=true + gate pass` branch rewired to call `Execute` + emit new events.
4. 12-15 new tests; full repo `go test ./...` PASS.
5. `gofmt`, `vet`, `build`, `npm run build` all clean.
6. Lab smoke matrix (9 senaryo) executed; senaryo 4 produces the headline lifecycle.
7. DB invariants verified: 0 destructive succeeded, 0 mutation cmd, 0 secret leak, 0 raw MAC.
8. Final report following the format in `CLAUDE.md` § 9.
9. Memory updated: `current.md` reflects Phase 10D MERGED + commit SHA + Phase 10E as the next.
