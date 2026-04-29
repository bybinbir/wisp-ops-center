# Phase 10E — Real destructive Execute (frequency_correction lab-only)

Status: planned (branch `phase/010e-destructive-execute-frequency-correction`).
Depends on: Phase 10D merged (main `416d3081`).

## Why

Phase 10D opened the runner's authority transfer to `action.Execute` but kept every destructive Kind on the `stubAction` that returns `ErrActionNotImplemented`. The lifecycle is fully wired (`live_start_blocked → execute_attempted → execute_not_implemented → finalize failed/action_not_implemented`); the only missing piece is the action that actually drives the device.

Phase 10E replaces the stub for **exactly one** Kind — `KindFrequencyCorrection` — with a real Execute that:

1. Reads the current operational frequency on the target wireless interface (snapshot for rollback).
2. Issues `/interface/wireless/set <iface> frequency=<MHz>` over the existing MikroTik SSH client, gated by a **separate write-allowlist** (`destructive_write.go`) that lists every byte the destructive runtime is allowed to send.
3. Re-reads the frequency to verify the device accepted the change.
4. If verification fails, automatically issues the rollback command (write the snapshot value back) and re-verifies.
5. Emits a 7-event lifecycle (`execute_started → write_succeeded → verified → /` or `verification_failed → rollback_started → rollback_succeeded` / `rollback_failed`) so an auditor can reconstruct what happened on the wire.

What Phase 10E does NOT do:

- It does **not** flip `var DestructiveActionEnabled = false` (`internal/networkactions/phase10_pregate.go:67`). That legacy global is the second of two fail-closed gates and stays `false`. Toggling it is a deliberate, separate operator decision — the first PR after this one.
- It does **not** open the production toggle. The provider toggle (`MemoryToggle`/`PgToggleStore`) also stays `enabled=false` post-merge. Lab smoke flips it under operator supervision; production flip is its own change-control event.
- It does **not** unify wireless and wifi/wifiwave2 yet. Phase 10E targets `/interface/wireless/...` (legacy CAPsMAN/RouterOS 6+7 wireless package). wifi/wifiwave2 path lands in a follow-up once wireless is proven safe.
- It does **not** add a new Kind. `frequency_correction` already exists (`KindFrequencyCorrection`); we replace its registry entry only.
- It does **not** change the Phase 10D handler control flow on the gate-fail / dry-run paths. Only the `errors.Is(execErr, ErrActionNotImplemented)` branch shrinks (real Execute returns the result instead of the sentinel).

## Master switch protocol — two layers, both fail-closed

Phase 10A established two independent destructive-action master switches, both default-OFF:

1. **Legacy global**: `var DestructiveActionEnabled = false` in `internal/networkactions/phase10_pregate.go:67`. Compile-time constant; flipping requires a code change + PR + review.
2. **Provider toggle**: `MemoryToggle` (in-memory) or `PgToggleStore` (Postgres-backed `network_action_toggle_flips` append-only ledger). Flipping requires `CapabilityToggleFlip` + actor + reason; persisted with `flipped_at`.

Phase 10E **does not flip either**. Both stay `false` after merge. The expected sequence for the first real lab run:

```
1. Operator opens a maintenance window (PgMaintenanceStore).
2. Operator flips the provider toggle to enabled=true with a recorded reason.
3. Operator commits a one-line code change setting DestructiveActionEnabled=true.
   (This is its own PR. Phase 10E will not include it.)
4. Operator runs the destructive happy-path POST.
5. Operator immediately reverts step 3 + flips the toggle off.
```

The gate's `IsDestructiveEnabled` helper already checks **both** layers — either being `false` denies the request. So even if Phase 10E ships and the operator flips only the provider toggle, the legacy const keeps the runtime silent. This is the intended belt-and-suspenders posture.

## Components

### `internal/adapters/mikrotik/destructive_write.go` (new)

Read-only `allowlist.go` is untouched. The new file declares a **separate** allowlist of paths the destructive runtime may execute, plus an `ExecWrite` helper layered on top of the SSH client that enforces it.

```go
var WriteAllowlistedCommands = []string{
    // Phase 10E: only the frequency adjust path. Every other byte
    // the destructive runtime tries to send is rejected here.
    "/interface/wireless/set",
}

var WriteRequiredArgs = map[string][]string{
    "/interface/wireless/set": {"number=", "frequency="},
}

func IsWriteAllowed(cmd string, args map[string]string) bool { … }
func EnsureWriteAllowed(cmd string, args map[string]string) error { … }
```

`ExecWrite` builds the RouterOS command line *server-side* from a typed struct (no operator-supplied free-form string is concatenated), passes it through `EnsureWriteAllowed`, then sends it via the existing SSH client. The result is the device's `:put` reply, parsed strictly.

### `internal/networkactions/frequency_correction.go` (new)

```go
type frequencyCorrectionAction struct {
    devices  DeviceLookup            // device id → IP, interface, credentials
    ssh      MikroTikDestructiveSSH  // ExecWrite + ExecRead
    log      Logger
    auditEm  AuditEmitter            // emits the 7 Phase 10E lifecycle events
}

func (a frequencyCorrectionAction) Kind() Kind { return KindFrequencyCorrection }

func (a frequencyCorrectionAction) Execute(ctx context.Context, req Request) (Result, error) {
    // 1. Resolve target device (id → host, iface, credentials, target_freq).
    // 2. Snapshot current frequency: ExecRead /interface/wireless/print where=name=<iface>.
    //    Pre-condition: snapshot.frequency MUST be a valid integer in MHz.
    // 3. Emit network_action.execute_started.
    // 4. ExecWrite /interface/wireless/set number=<iface> frequency=<target>.
    //    On error: emit execute_write_failed, return Result{Success:false, ErrorCode:"write_failed"}.
    // 5. Emit network_action.execute_write_succeeded.
    // 6. Verification: ExecRead snapshot again, parse frequency, compare to target.
    //    - Match: emit network_action.execute_verified, Result{Success:true, …}.
    //    - Mismatch: emit network_action.execute_verification_failed, fall through to rollback.
    // 7. Rollback (only on verification mismatch):
    //    - Emit network_action.execute_rollback_started.
    //    - ExecWrite /interface/wireless/set number=<iface> frequency=<snapshot>.
    //    - On write error or repeated mismatch: emit execute_rollback_failed.
    //      Result{Success:false, ErrorCode:"rollback_failed", … operator review required}.
    //    - On success: re-verify the snapshot value, emit execute_rollback_succeeded.
    //      Result{Success:false, ErrorCode:"verification_failed_rollback_recovered", …}.
}
```

The action is **single-step at the device boundary**: at most one frequency-write goes out per request; rollback adds one more write only if the first verification missed. Operator can read the audit lifecycle and see exactly which writes the runtime issued in what order.

### `internal/networkactions/audit_destructive.go` (extended)

Seven new constants, all stable strings:

```
network_action.execute_started               (success, before the write)
network_action.execute_write_succeeded       (success, after write OK)
network_action.execute_write_failed          (failure, write threw)
network_action.execute_verified              (success, terminal happy path)
network_action.execute_verification_failed   (failure, falls through to rollback)
network_action.execute_rollback_started      (success, rollback write begins)
network_action.execute_rollback_succeeded    (success, rollback OK + re-verified)
network_action.execute_rollback_failed       (failure, terminal — operator review)
```

`DestructiveAuditCatalog()` grows from 12 → 20.

The Phase 10D events (`execute_attempted`, `execute_not_implemented`) stay; `execute_attempted` still fires before `Execute` is called. The new events live **inside** the action and replace the single `execute_not_implemented` row Phase 10D produced.

### `apps/api/internal/http/handlers_destructive.go` (small surgical change)

Phase 10D's switch:

```go
if errors.Is(execErr, ErrActionNotImplemented) { … }   // expected path
// else: invariant violation → log + destructive_denied + finalize
```

becomes:

```go
if errors.Is(execErr, ErrActionNotImplemented) {       // legacy stubs (other Kinds)
    … existing Phase 10D path …
}
// Real Execute returned. The action emitted its own lifecycle events.
// The handler just persists the terminal status from result.Success.
status := networkactions.StatusFailed
if result.Success { status = networkactions.StatusSucceeded }
_ = s.actionRepo.FinalizeRun(gateCtx, runID, networkactions.FinalizeInput{
    Status:       status,
    DurationMS:   time.Since(startedAt).Milliseconds(),
    Result:       networkactions.SanitizeResultMap(result.Result),
    ErrorCode:    result.ErrorCode,
    ErrorMessage: networkactions.SanitizeMessage(result.Message),
})
```

Phase 10D's invariant-violation branch stays in place but is unreachable for `frequency_correction`; it covers any future Kind whose registry slot still points at the stub.

### Registry update

`NewRegistry()` keeps registering `stubAction` for every Kind. Real implementations land via a new wiring point in the API server (`apps/api/cmd/api/main.go` or its DI seam): when the destructive runtime has the right dependencies (SSH client, device repo), it calls `registry.Register(frequencyCorrectionAction{…})`. This isolates the production wiring decision from the network-action package itself.

## Lifecycle event order — Phase 10E happy path

For `POST /destructive/frequency-correction/confirm` with toggle ON + window ACTIVE + RBAC granted + idempotency new + verification PASS:

1. `network_action.confirmed` (handler)
2. `network_action.rollback_metadata_recorded` (handler)
3. `network_action.live_start_blocked` (runner, pre-gate; fires for EVERY confirm POST regardless of gate outcome — Phase 10C invariant preserved)
4. *(gate runs and passes — no audit event for gate-pass by design; absence of `gate_fail` is the signal)*
5. `network_action.execute_attempted` (runner, before Execute call — Phase 10D invariant preserved)
6. `network_action.execute_started` (action, after device resolve + snapshot, before write) **← Phase 10E NEW**
7. `network_action.execute_write_succeeded` (action, after device-side `:put` parse) **← Phase 10E NEW**
8. `network_action.execute_verified` (action, snapshot==target after second read) **← Phase 10E NEW**

Verification-fail-but-rollback-succeeded path replaces (8) with:

8a. `network_action.execute_verification_failed`
9. `network_action.execute_rollback_started`
10. `network_action.execute_rollback_succeeded`
   (terminal status: failed, error_code=verification_failed_rollback_recovered)

Rollback-fail path replaces (10) with `execute_rollback_failed` (terminal status: failed, error_code=rollback_failed; operator manual review).

## Senaryo matrix (lab smoke — Phase 10E PR-B)

| # | Toggle | Window | RBAC | Wireless target | Mode | Expected lifecycle tail |
|---|---|---|---|---|---|---|
| 1 | OFF | - | granted | n/a | confirm | `gate_fail(destructive_disabled)` (regression — same as Phase 10D senaryo 1) |
| 2 | ON | NONE | granted | n/a | confirm | `maintenance_window_denied → gate_fail` |
| 3 | ON | ACTIVE | denied | n/a | confirm | HTTP 403 / `rbac_denied` (HTTP-layer) |
| 4 | ON | ACTIVE | granted | available | dry-run | `dry_run` succeeded with planned `commands` array; **no write goes out** |
| 5 | ON | ACTIVE | granted | available | confirm | `execute_started → execute_write_succeeded → execute_verified` (succeeded) — HEADLINE |
| 6 | ON | ACTIVE | granted | available, target_freq invalid | confirm | `execute_started → execute_write_failed` (write rejected by device); rollback NOT attempted (no successful write to reverse) |
| 7 | ON | ACTIVE | granted | available, target_freq accepted but device snaps to nearest channel | confirm | `execute_started → execute_write_succeeded → execute_verification_failed → execute_rollback_started → execute_rollback_succeeded` (failed, error_code=verification_failed_rollback_recovered) |
| 8 | ON | ACTIVE | granted | available, simulated rollback failure | confirm | `… → execute_rollback_failed` (failed, error_code=rollback_failed; operator alert) |
| 9 | ON | ACTIVE | granted | unavailable (snapshot read fails) | confirm | `execute_started → execute_write_failed(device_unreachable)`; **no write attempted** |

Senaryo 5 is the headline. Senaryos 7 and 8 are the rollback safety harness — these MUST be exercised in the lab before any production toggle conversation.

PR-B prerequisite: operator provides a wireless RouterOS device whose frequency may be flipped between two well-known channels (e.g. 5180 ↔ 5200 MHz) without affecting any subscriber. The current Dude SSH host (Phase 9 v2 evidence) has no wireless interface; PR-B is BLOCKED on operator providing this target.

## DB invariant assertions (post-PR-A merge)

```sql
-- Master switches stay closed post-merge.
SELECT enabled FROM network_action_toggle_flips ORDER BY flipped_at DESC LIMIT 1;
-- Expected: f

-- legacy global stays false (proof: git diff main..HEAD on phase10_pregate.go:67 is empty)

-- Phase 10E catalog grew to 20 (12 + 8 new)
SELECT COUNT(DISTINCT action) FROM audit_logs
WHERE action LIKE 'network_action.execute_%';
-- Pre-merge: 2 (Phase 10D events). Post-PR-A merge but pre-PR-B smoke:
-- still 2 (no Execute path actually fired). Post-PR-B: 2 + new events.

-- 0 destructive succeeded so far
SELECT COUNT(*) FROM network_action_runs
WHERE action_type='frequency_correction' AND status='succeeded';
-- Expected through Phase 10E PR-A: 0 (no run reaches Execute)
-- Expected after PR-B senaryo 5: exactly 1 per smoke iteration.

-- 0 mutation cmd in result jsonb (PR-A only — no Execute path runs)
SELECT COUNT(*) FROM network_action_runs
WHERE result::text ~* E'(/interface/wireless/set|frequency=)';
-- Expected post-PR-A: 0. Post-PR-B senaryo 5: rows reflecting write events
-- but the column command_count >= 2 (write + verify-read at minimum).
```

## Test plan

PR-A (engineering, sandbox-doable):

- `internal/adapters/mikrotik/destructive_write_test.go` (new):
  - Write allowlist accepts `/interface/wireless/set` with required args.
  - Rejects every Forbidden segment from the read-only allowlist (`add`, `remove`, `enable`, `disable`, `reset`, `reboot`, …).
  - Rejects bare paths missing required args (`number=`, `frequency=`).
  - Rejects path with extra unexpected args.
- `internal/networkactions/frequency_correction_test.go` (new, mock device):
  - Happy path: snapshot → write → verify → succeeded.
  - Write fails (device returns error) → write_failed terminal, no rollback.
  - Verification fail + rollback succeeds → terminal failed, error_code=verification_failed_rollback_recovered.
  - Verification fail + rollback fails → terminal failed, error_code=rollback_failed.
  - Snapshot read fails before any write → write_failed (device_unreachable), no write attempted.
  - Audit emitter receives every event in expected order for each path.
- `internal/networkactions/phase10e_audit_catalog_test.go` (new):
  - `DestructiveAuditCatalog()` length == 20.
  - All 8 new event names are present and stable strings.
  - Phase 10D entries (`execute_attempted`, `execute_not_implemented`) still present (regression guard).
- `internal/networkactions/phase10c_lifecycle_test.go` + `phase10d_audit_catalog_test.go`: catalog size assertions stay as lower bounds (`>= 10` and `>= 12`).
- `internal/networkactions/phase10d_runtime_test.go`: registry stub assertion for `KindFrequencyCorrection` updates — the test now expects either the stub OR a real implementation that still satisfies the contract; we keep the lower-bound check (`Execute returns either ErrActionNotImplemented or a non-nil Result`).

Target test count: 116 → ~140-145 (+25-30 across mikrotik destructive_write + networkactions Phase 10E).

PR-B (lab smoke, operator-target-required): the 9-row matrix above. Hermetic state assertions:

```
0 destructive succeeded BEFORE the lab run.
1 destructive succeeded AFTER senaryo 5 (and matching write commands in result).
0 raw 6-octet MAC in audit metadata or result.
0 secret leak (3 sources: api log, audit metadata, result jsonb).
toggle row immediately post-smoke flipped back to enabled=false with reason="PR-B smoke complete".
legacy DestructiveActionEnabled=false re-asserted by post-merge git diff.
```

## Migration plan

**No new migration.** Phase 10C migration `000013` supplies all schema. The new audit events use the existing `audit_logs` table (no schema change needed; `action` is `TEXT`).

3× post-merge replay of `000011` / `000012` / `000013` MUST stay clean (errors=0).

## Rollback plan (PR-A merge gone wrong)

`git revert <merge sha>` reverts the action-package changes. The handler keeps the Phase 10D code path (`errors.Is(ErrActionNotImplemented)` branch) so reverting the registry wiring restores Phase 10D behaviour exactly. No DB rollback needed (no migration). No data is at risk because both master switches stay closed across the change.

## Phase 10F — handed forward

Phase 10F is operational hardening, mostly outside engineering:

- TLS end-to-end + secret rotation procedure
- Vault/KMS integration (`wisp-ops-center/secret/...` path)
- Prometheus + alert rules (gate denied rate, destructive run rate, audit event spike)
- Backup/restore drill (Postgres physical + logical, point-in-time recovery test)
- RBAC + multi-tenant isolation (PgRoleResolver SQL-first, RequireSQL=true on production)
- KVKK retention scheduler (audit_logs + network_action_runs older than retention_days)
- SOC2 compliance checklist
- wifi/wifiwave2 destructive-write parity once wireless is proven safe in lab

The engineering-doable portions of Phase 10F (PgRoleResolver RequireSQL flip, KVKK retention scheduler, alert rule definitions) ship as small follow-up PRs after Phase 10E PR-B is green.

## Operational guard — how to spot a misuse

If audit_logs ever shows `network_action.execute_verified` with `target_host` matching a production tower, **and** the corresponding toggle row's `actor` is not on the approved-operators list, that is an incident. The `toggle_flipped` event is the audit anchor for who authorized the moment. The `live_start_blocked` event is the second anchor (every confirm POST gets one regardless of gate outcome). Both are append-only.
