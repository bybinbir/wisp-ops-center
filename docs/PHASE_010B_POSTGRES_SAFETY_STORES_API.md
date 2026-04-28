# Phase 10B — Postgres-backed safety stores + API surface

Status: in-flight (branch `phase/010b-postgres-safety-stores-api`)
Depends on: Phase 10A merged (main `ff489da`).

## Why

Phase 10A landed the interface scaffolding (`DestructiveToggle`, `RBACResolver`, `MaintenanceProvider`/`Store`, audit catalog) but every implementation was in-memory. Phase 10B carries those interfaces to a production-shaped layer:

1. `PgToggleStore` reads/writes `network_action_toggle_flips` so a flip survives a restart and every flip is captured in an append-only audit table.
2. `PgMaintenanceStore` reads/writes `network_action_maintenance_windows` so windows persist and can be disabled with a full audit trail.
3. `PgRBACResolver` is a typed seam wrapping the static fallback; the SQL hook is reserved for Phase 10C without changing the gate or handlers.
4. Five new HTTP endpoints expose the safety surface: preflight, toggle flip, maintenance window CRUD + disable.
5. New capabilities (`network_action.maintenance.manage`, `network_action.preflight.read`) plus capability-name normalization (`network_action.*` namespace).

`DestructiveActionEnabled = false` stays untouched; the new toggle ALSO defaults closed (empty `network_action_toggle_flips` → `Enabled() = false`). No destructive execution path is opened.

## Components

### `internal/networkactions/pg_toggle.go`
- `PgToggleStore` implements `DestructiveToggle`.
- `Enabled()` reads the latest flip; nil pool → `ErrToggleStoreUnavailable`; empty table → `(false, nil)`; DB error → `(false, err)` (fail-closed).
- `Flip()` validates non-empty actor + reason BEFORE any INSERT; on DB error returns the error so the API surfaces 500 (no fail-open).
- `LastFlip()` returns the most recent receipt; `nil` when no flip has happened.

### `internal/networkactions/pg_maintenance.go`
- `PgMaintenanceStore` implements `MaintenanceProvider` + extended `MaintenanceStore` semantics.
- `Create(CreateInput)` validates through `ValidateMaintenanceRecord` (1m..24h, end > start, non-empty title) BEFORE INSERT.
- `Get`, `List` return all records (active + disabled); `ActiveAt` skips `disabled_at IS NOT NULL` rows and rows whose `[start_at, end_at)` does not contain `at`.
- `Disable(DisableInput)` UPDATE ... WHERE id AND disabled_at IS NULL → `ErrMaintenanceWindowNotFound` if not found or already disabled.

### `internal/networkactions/pg_rbac.go`
- `PgRBACResolver` is a seam: it always delegates to a `Fallback` `RBACResolver`. The SQL hook is reserved for Phase 10C; the type already lives in production wiring so the swap requires no caller changes.
- Constructor auto-wires `NewDefaultRoleResolver()` if the caller passes `nil`, so the seam is never silently fail-open.

### `internal/networkactions/rbac.go` (extended)
- Capability namespace normalized to `network_action.*` (was `network.*` in Phase 10A).
- Two new capabilities: `CapabilityMaintenanceManage`, `CapabilityPreflightRead`.
- Default role mapping updated: every role with destructive responsibility also gets `MaintenanceManage` + `PreflightRead`; `net_viewer` gets `PreflightRead` only.

### `apps/api/internal/http/handlers_safety.go`
| Endpoint | Method | Capability | Purpose |
| --- | --- | --- | --- |
| `/api/v1/network/actions/preflight` | GET | `preflight.read` | Returns toggle state, last_flip, active windows, pre-gate checklist, blocking_reasons, caller capabilities. NEVER runs an SSH command. |
| `/api/v1/network/actions/toggle` | POST | `toggle.flip` | Flips the master switch with audit-logged actor + reason. Default deny: empty reason → 400. |
| `/api/v1/network/actions/maintenance-windows` | GET | `preflight.read` | Lists all windows (active + disabled) ordered by start_at. |
| `/api/v1/network/actions/maintenance-windows` | POST | `maintenance.manage` | Creates a window. Validation runs Phase 10A's `ValidateMaintenanceRecord`. |
| `/api/v1/network/actions/maintenance-windows/{id}/disable` | PATCH | `maintenance.manage` | Disables a window with audit-logged reason. |

`requireCapability` is the single authorization choke-point; on deny it emits the `network_action.rbac_denied` audit event and writes 403.

### Migration `000012_phase10b_maintenance_window_disable.sql`
Adds `disabled_at`, `disabled_by`, `disable_reason`, `notes` columns to `network_action_maintenance_windows` + a partial index `idx_namw_active_only` over `(start_at, end_at) WHERE disabled_at IS NULL`. Idempotent (`ADD COLUMN IF NOT EXISTS`), transactional, no DROP. Re-applied 3× in lab; subsequent runs report "already exists, skipping" and COMMIT cleanly.

## API contract (illustrative)

```http
GET /api/v1/network/actions/preflight
X-Actor: alice
X-Roles: net_viewer

200 OK
{
  "destructive_enabled": false,
  "toggle_source": "store",
  "last_flip": { "enabled": false, "actor": "alice", "reason": "...", "flipped_at": "..." },
  "active_maintenance_windows": [],
  "pregate_checklist": [ ... 14 items ... ],
  "caller_capabilities": ["network_action.preflight.read"],
  "blocking_reasons": ["destructive_disabled", "no_active_maintenance_window"],
  "now": "2026-04-28T..."
}
```

```http
POST /api/v1/network/actions/toggle
X-Actor: alice
X-Roles: net_admin
Content-Type: application/json
{ "enabled": false, "reason": "rolling back" }

200 OK
{ "actor": "alice", "enabled": false, "flipped_at": "..." }
```

## Audit coverage

| Event | When |
| --- | --- |
| `network_action.preflight_checked` | Every successful preflight read. |
| `network_action.toggle_flipped` | Every successful flip (sanitized reason). |
| `network_action.maintenance_window_created` | Successful POST. |
| `network_action.maintenance_window_disabled` | Successful PATCH disable. |
| `network_action.maintenance_window_denied` | Validator-failed create (sanitized error). |
| `network_action.rbac_denied` | Any 403 from `requireCapability`. |
| `network_action.gate_failed` | Reserved by audit catalog (used by destructive runtime in 10C). |

## Test evidence

```
gofmt -l ./internal ./apps                  clean
go vet ./...                                clean
go build ./...                              clean
go test ./...                               PASS (every package)
go test ./internal/networkactions/ -v -count=1   91 tests PASS
                                                  (Phase 10A: 83, Phase 10B: +8)
go test ./apps/api/internal/http/ -v        PASS (3 helper tests)
npm run build                               /aksiyonlar 6.26 kB,
                                            /ag-envanteri 6.43 kB
migration 000012 — 3× ardışık apply         "already exists, skipping" + COMMIT
```

8 new networkactions tests:
- PgToggleStore (3): nil pool fail-closed, Flip requires actor+reason, interface conformance.
- PgRBACResolver (3): nil pool delegates to fallback, nil fallback fail-closed, constructor auto-wires default fallback.
- RBAC (2): default mapping includes Phase 10B caps, capability names stable.

3 new handler helper tests:
- principalFromRequest: defaults + headers parsed.
- blockingReasonsFromState: 4 scenarios.

## Live smoke evidence

```
GET  /preflight (no roles)                  403 rbac_denied
GET  /preflight (X-Roles=net_viewer)        200 destructive_enabled=false
                                            blocking_reasons=[destructive_disabled, no_active_maintenance_window]
POST /toggle  (X-Roles=net_viewer)          403 rbac_denied
POST /toggle  (X-Roles=net_admin, no reason) 400 reason_required
POST /toggle  (X-Roles=net_admin, valid)    200 enabled=false flipped_at=...
GET  /preflight (post-flip)                 200 last_flip.actor=alice reason="..."
POST /maintenance-windows (X-Roles=net_viewer) 403 rbac_denied
POST /maintenance-windows (inverted range)  400 invalid_window
POST /maintenance-windows (1h, valid)       201 data.id=<uuid>
GET  /maintenance-windows                   200 count=1
PATCH /maintenance-windows/{id}/disable     200 (window disabled)

DB invariants (post-smoke):
  network_action_runs total / dry / live    12 / 12 / 0
  destructive Kind row count                0
  mutation cmds executed                     0
  toggle_flips count                         1   (enabled=false, actor=alice)
  maintenance windows total / disabled       1 / 1
  audit events                               12 finish + 12 start +
                                              2 preflight_checked +
                                              3 rbac_denied + 1 toggle_flipped +
                                              1 mw_created + 1 mw_denied + 1 mw_disabled
  secret leakage api log                     0
  secret leakage audit metadata              0
  secret leakage result jsonb                0
  raw 6-octet MAC in result                  0
  DestructiveActionEnabled global             false (line 67 unchanged)
```

## Safety invariant proof

- Master switch invariant: `network_action_toggle_flips` empty → toggle.Enabled() returns false; even after a Flip, the Phase 10A pre-gate `EnsureDestructiveAllowedWithProviders` still gates RBAC + maintenance + intent + rollback + idempotency. Live smoke confirmed `enabled=false` after the smoke flip.
- Fail-closed posture: every endpoint runs through `requireCapability` BEFORE touching the store. Empty `X-Roles` → 403 with audit. Store error → 500 (never silently 200).
- No SSH execution: handlers only call `actionToggle.Flip`, `actionWindows.Create/Disable`, and read methods. They never construct an SSH session.
- target_host validation (Phase 9 v3) unchanged; smuggled mutation still 400.
- Sanitization: `SanitizeMessage` runs on every operator-supplied reason BEFORE persistence; tested in Phase 9 v3.

## Phase 10C handed forward

- Postgres-backed `RBACResolver` (real role lookup beyond static fallback).
- Idempotency-key DB-level uniqueness (per device + action_type + intent).
- `runActionAsync` for destructive Kinds: emit `network_action.confirmed` → `EnsureDestructiveAllowedWithProviders` → on fail emit `AuditActionForGateError(err)` event → on success emit `network_action.dry_run` (or `network_action.live_start_blocked` if Confirm=false).
- End-to-end smoke that proves: closed toggle → request denied; open toggle + missing window → denied; full happy → only explicit operator-confirmed write set.

## Rollback plan

- Migration 000012 is additive (`ADD COLUMN IF NOT EXISTS` + `CREATE INDEX IF NOT EXISTS`). Nothing depends on the new columns being present; rolling back to Phase 10A keeps the Phase 10A behavior because the new columns simply go unused.
- Code rollback = revert this commit. Phase 10A behavior is restored exactly: in-memory toggle, in-memory maintenance store, capability namespace `network.*` (vs Phase 10B's `network_action.*`).
