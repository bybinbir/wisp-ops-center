# Phase 10C — Destructive runtime lifecycle (no execution)

Status: in-flight (branch `phase/010c-destructive-runtime-lifecycle`)
Depends on: Phase 10B merged (main `92f32ac`).

## Why

Phase 10B persisted the safety stores (toggle, maintenance windows) and shipped the API surface. Phase 10C wires the destructive *runtime*:

1. SQL-backed RBAC grants — header roles are no longer trusted for actors that exist in the grants table.
2. Idempotency at the DB layer — a duplicate POST cannot accidentally re-execute the same destructive intent.
3. Rollback metadata persistence — every destructive run row carries `intent` + `rollback_note` BEFORE the gate runs.
4. Three new lifecycle audit events — `idempotency_reused`, `rollback_metadata_recorded`, `destructive_denied`.
5. A destructive runner that runs the pre-gate, emits the lifecycle catalog, and **NEVER** calls `action.Execute`.

`var DestructiveActionEnabled = false` (legacy global) and the latest `network_action_toggle_flips` row (`enabled=false`) stay untouched. No destructive execution path is opened.

## Components

### `internal/networkactions/pg_role_grants.go` (new)

`PgRoleResolver` queries `network_action_role_grants` for the principal's actor.

- nil pool → `ErrRBACResolverUnavailable` (fail-closed).
- DB error → `ErrRBACResolverUnavailable`.
- Empty actor → `ErrPrincipalUnknown`.
- Actor missing in grants → `ErrPrincipalUnknown`.
- Actor in grants → SQL roles drive caps; header roles are IGNORED.

`SeedActor(ctx, actor, roles, grantedBy, notes)` is the operator-facing helper used by tests + a future admin endpoint.

### `internal/networkactions/pg_rbac.go` (extended)

`PgRBACResolver` now holds an `RBACResolver` SQL field (interface, not concrete) so tests can mock. Constructor auto-wires `*PgRoleResolver` against the live pool. Lookup order:

1. SQL resolver (when wired). Found → those caps.
2. SQL says `ErrPrincipalUnknown`:
   - `RequireSQL=true` → `ErrPrincipalUnknown` (deny, production posture).
   - `RequireSQL=false` → fall back to header-roles resolver (Phase 10B parity for unseeded deployments).
3. SQL says any other error → return that error (DB outage MUST NOT silently fall through to headers; that would let an attacker degrade the DB connection to escalate via spoofed roles).
4. SQL not wired → header fallback.

### `internal/networkactions/repository_destructive.go` (new)

- `CreateDestructiveRun(input)` validates non-empty `idempotency_key`, `intent`, `rollback_note` BEFORE INSERT. On `23505` (duplicate-key) returns `ErrIdempotencyConflict`.
- `FindByIdempotencyKey(kind, key)` returns the existing run for `(action_type, idempotency_key)` or `ErrNotFound`.
- `GetLifecycle(runID)` returns the run + all `audit_logs` rows whose `metadata->>'correlation_id'` or `metadata->>'run_id'` match.

### `internal/networkactions/audit_destructive.go` (extended)

Three new constants + catalog membership:

- `AuditActionIdempotencyReused` = `"network_action.idempotency_reused"`
- `AuditActionRollbackMetadataRecorded` = `"network_action.rollback_metadata_recorded"`
- `AuditActionDestructiveDenied` = `"network_action.destructive_denied"`

`DestructiveAuditCatalog()` now returns 10 events (Phase 10A: 7 + Phase 10C: 3).

### `apps/api/internal/http/handlers_destructive.go` (new)

Two endpoints + the lifecycle GET. Critical Phase 10C invariant: **the runner never calls `action.Execute`**. Destructive Kinds short-circuit before any SSH dial:

| Endpoint | Method | Capability | Behavior |
| --- | --- | --- | --- |
| `/api/v1/network/actions/destructive/{kind}/dry-run` | POST | `network_action.destructive.dryrun` | Records run, runs gate, emits `confirmed` + `rollback_metadata_recorded` then on failure `gate_fail` + specific subtype + `destructive_denied`; on pass `dry_run`. |
| `/api/v1/network/actions/destructive/{kind}/confirm` | POST | `network_action.destructive.execute` | Same shape; on gate pass emits `live_start_blocked` + `destructive_denied` (Phase 10C blocks live execution by design). |
| `/api/v1/network/actions/lifecycle/{run_id}` | GET | `network_action.preflight.read` | Returns the run row + every `audit_logs` event tagged with the run's `correlation_id` or `run_id`. |

### Request body (dry-run / confirm)

```json
{
  "target_device_id": "<uuid>",
  "target_host": "<ip|hostname>",
  "idempotency_key": "<operator-chosen unique key>",
  "intent": "<short human-readable intent>",
  "rollback_note": "<rollback plan; non-empty>",
  "reason": "<optional context>"
}
```

Either `target_device_id` (with non-empty host on the inventory row) or `target_host` is required. Missing `idempotency_key`, `intent`, or `rollback_note` → 400. `target_host` must pass `ValidateTargetHost` (Phase 9 v3) BEFORE the DB inet cast.

### Migration `000013_phase10c_destructive_runtime_lifecycle.sql`

Idempotent + transactional. ALTER/CREATE only, no DROP. Phase 5 `maintenance_windows` table is NOT touched.

```sql
-- network_action_role_grants — actor → roles binding
CREATE TABLE IF NOT EXISTS network_action_role_grants (...);
CREATE INDEX IF NOT EXISTS idx_narg_updated ...;

-- network_action_runs additive columns
ALTER TABLE network_action_runs
  ADD COLUMN IF NOT EXISTS idempotency_key text,
  ADD COLUMN IF NOT EXISTS intent          text,
  ADD COLUMN IF NOT EXISTS rollback_note   text;

-- (action_type, idempotency_key) uniqueness for destructive intent dedupe
CREATE UNIQUE INDEX IF NOT EXISTS uniq_nar_action_idem
  ON network_action_runs (action_type, idempotency_key)
  WHERE idempotency_key IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_nar_intent ON network_action_runs (intent)
  WHERE intent IS NOT NULL;
```

3× ardışık replay: `BEGIN`, NOTICE: already exists/skipping (every column + index after first apply), `COMMIT`, exit=0.

## Lifecycle event order (current closed-toggle smoke)

For a destructive POST while master switch is closed (`enabled=false`):

1. `network_action.rollback_metadata_recorded` (success) — run row carries intent + rollback_note.
2. `network_action.confirmed` (success) — operator passed the explicit-intent shape.
3. (gate runs) `EnsureDestructiveAllowedWithProviders` returns `ErrDestructiveDisabled`.
4. `network_action.gate_fail` (failure) — `error_code=destructive_disabled`.
5. `network_action.destructive_denied` (failure) — terminal denial, mirrors gate_fail at a higher level.
6. (run row finalized: `status=failed`, `error_code=destructive_disabled`)

For a duplicate POST with the same idempotency_key:

1. `network_action.idempotency_reused` (success) — original run id returned, no new INSERT.

For a hermetic gate-pass test (toggle flipped + window present, full guardrails) on the **confirm** path:

1. `rollback_metadata_recorded` → `confirmed` → (gate pass) → `live_start_blocked` (failure) → `destructive_denied` (failure).
2. Run finalized as `status=failed`, `error_code=live_start_blocked`. Phase 10C **NEVER** calls Execute.

For the same hermetic scenario on the **dry-run** path:

1. `rollback_metadata_recorded` → `confirmed` → (gate pass) → `dry_run` (success).
2. Run finalized as `status=succeeded`. Phase 10C **NEVER** calls Execute even on dry-run.

## RBAC capability matrix (Phase 10C)

| Capability | dry-run | confirm | preflight | maintenance | toggle |
| --- | --- | --- | --- | --- | --- |
| `network_action.destructive.dryrun` | required | — | — | — | — |
| `network_action.destructive.execute` | — | required | — | — | — |
| `network_action.preflight.read` | — | — | required | (list) | — |
| `network_action.maintenance.manage` | — | — | — | required (create/disable) | — |
| `network_action.toggle.flip` | — | — | — | — | required |

Default static role mapping (used when grants table is empty AND `RequireSQL=false`):

- `net_admin` → all 5 capabilities.
- `net_ops` → execute + dryrun + maintenance + preflight (no toggle).
- `net_viewer` → preflight only.

## Phase 10D handed forward

- Real destructive `Execute` implementation for `frequency_correction` (gated behind `var DestructiveActionEnabled` flip + master switch flip + RBAC SQL grants seeded). Phase 10C ships every guardrail; flipping the switch is one line + a deployment-time decision.
- Operator-facing endpoint to seed `network_action_role_grants` (today: `PgRoleResolver.SeedActor` is internal only).
- A `WISP_RBAC_REQUIRE_SQL=true` env wired into `Server.New` so production deployments can flip from permissive to strict mode without code changes.
- Audit retention scheduler (Phase 7 carry-over) — destructive lifecycle events are append-only; a retention sweeper is needed before we cross 90 days at scale.

## Rollback plan

Schema:
- Every change is `ADD COLUMN IF NOT EXISTS` / `CREATE … IF NOT EXISTS`. Reverting Phase 10C code makes the new columns and the grants table unused but harmless.
- The `uniq_nar_action_idem` partial index only constrains rows where `idempotency_key IS NOT NULL`; pre-Phase-10C runs (Phase 9 read-only) are unaffected.

Code:
- Revert the merge commit. Phase 10B behavior is restored exactly: header-based RBAC, no idempotency uniqueness, no destructive runner, no `/destructive/*` or `/lifecycle/*` routes.
- The grants table can be left in place (no destructive caller); a `TRUNCATE network_action_role_grants` is enough if the operator wants a clean state.

Operational:
- The destructive runner produces no live mutation, so a rollback never has to worry about "in-flight live destructive run" — every destructive row in `network_action_runs` from Phase 10C is `status=failed` with `error_code IN ('destructive_disabled','live_start_blocked')`.
