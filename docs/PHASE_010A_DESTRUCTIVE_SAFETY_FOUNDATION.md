# Phase 10A — Destructive-action safety foundation

Status: in-flight (branch `phase/010a-destructive-safety-foundation`)
Depends on: Phase 9 v3 merged (main `c2fd134`).

## Why

Phase 9 v3 landed the structural pre-gate for destructive actions but every guardrail still leaned on a process-global `DestructiveActionEnabled = false` constant + a constant `AllowedRolesForDestructive` slice + an inline `MaintenanceWindow` struct. Before Phase 10 can even *think* about flipping the master switch we need:

1. An operator-controlled, audit-logged toggle that defaults closed.
2. An RBAC resolver that consults capabilities (so role lookup can later move to a Postgres store without touching the gate).
3. A maintenance-window domain model + provider with a real validator.
4. A dedicated audit catalog so every Phase 10 lifecycle event has a stable event name.

Phase 10A delivers all four **without enabling any destructive action**. `DestructiveActionEnabled` (legacy global) is preserved for back-compat tests; the new path goes through `DestructiveProviders`. With the providers in place, Phase 10 work can land each guardrail in its own focused PR.

## Components

### `internal/networkactions/toggle.go`
- `DestructiveToggle` interface: `Enabled(ctx)`, `Flip(ctx, enabled, actor, reason)`.
- `MemoryToggle` — concurrent-safe, **default-closed**. Tests use it; the API server can use it as a temporary backing store until a Postgres-backed implementation lands.
- `Flip` refuses empty actor/reason — the audit record can always answer "who/why".
- `IsDestructiveEnabled` helper is **fail-closed**: nil toggle or any store error returns false.
- `FlipReceipt` is the persisted shape (enabled, actor, reason, flipped_at).

### `internal/networkactions/rbac.go`
- `Capability` typed string with three reserved values: `network.destructive.execute`, `network.destructive.dryrun`, `network.destructive.toggle.flip`.
- `RBACResolver` interface; `Principal` carries actor + roles only (never roles read directly).
- `StaticRoleResolver` maps lowercased role → capability list (case + whitespace tolerant).
- `NewDefaultRoleResolver` ships the conservative platform default: `net_admin → toggle+execute+dryrun`, `net_ops → execute+dryrun`, `net_viewer → none`.
- `HasCapability` is fail-closed: nil resolver or any error returns deny.
- `ErrRBACResolverUnavailable` typed sentinel for store outages.

### `internal/networkactions/maintenance.go`
- `MaintenanceRecord` (id, title, start, end, scope[], created_by, created_at) + `IsOpenAt` + `AppliesToDevice`.
- `MaintenanceProvider` (read) + `MaintenanceStore` (write) interfaces.
- `ValidateMaintenanceRecord` enforces: non-empty title, end > start (strict), 1m ≤ duration ≤ 24h.
- `MemoryMaintenanceStore` — concurrent-safe, monotonic-counter ID generator (no collisions under fast `Create` bursts), pluggable clock + id generator for tests.
- 5 typed sentinels: empty title, inverted range, too short, too long, not found.

### `internal/networkactions/audit_destructive.go`
- `DestructiveAuditAction` typed string with 7 reserved event names (catalog).
- `DestructiveAuditCatalog()` returns the canonical list; tests pin the names so renames cannot drift unnoticed.
- `AuditActionForGateError(err)` maps RBAC + window-related sentinels to targeted action names; everything else falls through to `network_action.gate_fail`.

### `internal/networkactions/phase10_pregate.go` (extended)
- New typed sentinels: `ErrToggleProviderRequired`, `ErrRBACProviderRequired`, `ErrWindowProviderRequired`.
- New `DestructiveProviders` bundle.
- New `EnsureDestructiveAllowedWithProviders(ctx, providers, req)`:
  1. providers != nil ✓
  2. Toggle.Enabled == true (master switch)
  3. Kind.IsDestructive() (non-destructive bypass)
  4. RBAC: `CapabilityDestructiveExecute` required
  5. req.Confirm == true
  6. MaintenanceProvider returns >=1 active window
  7. RollbackNote non-empty
  8. IdempotencyKey non-empty

  Toggle store error → fail-closed (returns `ErrDestructiveDisabled`).
  Window provider error → fail-closed (returns `ErrMaintenanceWindowMissing`).

### Migration `000011_phase10a_safety_foundation.sql`
- New table `network_action_toggle_flips`: append-only audit of every flip (id, enabled, actor, reason, flipped_at) + indexes.
- New table `network_action_maintenance_windows`: operator-declared destructive-action change windows (id uuid, title, start_at, end_at, scope uuid[], created_by, created_at) + indexes. **Intentionally separate** from the existing Phase 5 `maintenance_windows` table (different domain semantics).
- Idempotent + transactional. ALTER/CREATE only, **no DROP**. Re-applied 3× in lab; all subsequent runs report "already exists, skipping" and COMMIT cleanly. Existing Phase 5 `maintenance_windows` schema remains untouched (verified column-by-column).

## Test evidence

```
gofmt -l                  clean
go vet ./...              clean
go build ./...            clean
go test ./...             PASS
go test ./internal/networkactions/ -count=1 -v   83 tests PASS
                                                  (Phase 9 v3: 58, Phase 10A: +25)
npm run build             /aksiyonlar 6.26 kB, /ag-envanteri 6.43 kB
migration replay          BEGIN/CREATE×4/COMMIT, then "already exists, skipping" ×6
```

25 new networkactions tests:
- Toggle (5): default closed, Flip requires actor+reason, full round-trip, nil treated as closed, LastFlip returns copy.
- RBAC (6): default mapping, role union, nil resolver unavailable, HasCapability fail-closed, nil resolver denies, case-insensitive lookup.
- Maintenance window (7): validate rejects bad inputs (5 sub-cases), validate accepts happy path, store CRUD round-trip, get-missing typed error, ActiveAt scoping + closed windows, IsOpenAt boundary rules, AppliesToDevice empty-scope semantics.
- Provider gate (5): nil providers fail-closed, default-closed toggle blocks, non-destructive bypass, full guardrail matrix (RBAC×2 / intent / window-provider-nil / window-closed / rollback / idempotency), toggle-store-error fail-closed.
- Audit catalog (2): catalog stable list, AuditActionForGateError mapping.

## Live smoke evidence

```
sanity                                    /api/v1/network/actions  200
read-only frequency_check (regression)    skipped, dry_run=true, command_count=3,
                                           confidence=30 (Phase 9/v2/v3 path
                                           intact)
smuggled "frequency=5180" target_host     400 invalid_target_host (Phase 9 v3
                                           validation still firing)

DB invariants:
  total runs                              11
  dry_run=true / dry_run=false            11 / 0
  destructive Kinds row count             0  (frequency_correction,
                                              maintenance_window stay stub)
  mutation commands ever executed         0
  toggle_flips row count                  0  (default-closed; no flip happened)
  network_action_maintenance_windows      0  (no live destructive request
                                              attempted)
  invalid target_host rows                0
  secret leakage api log                  0
  secret leakage audit metadata           0
  secret leakage result jsonb             0
  raw 6-octet MAC in result               0
```

## Safety invariant proof

- **Master switch invariant**: `MemoryToggle` defaults to `Enabled=false`. `EnsureDestructiveAllowedWithProviders` returns `ErrDestructiveDisabled` for any destructive request when no Flip has happened. `TestEnsureDestructiveAllowedWithProviders_DefaultClosedToggleBlocks` pins this. The legacy `DestructiveActionEnabled` global also stays `false`.
- **Fail-closed posture**: every error path (nil providers, store errors, missing capabilities, missing windows) returns the deny sentinel. No fail-open code path exists.
- **dry_run invariant**: 11 / 11 runs in DB are `dry_run=true`. `0` live destructive runs.
- **Mutation deny-list**: existing Phase 9 allowlist + denylist tests still pass; the new code adds no command paths.
- **Secret hygiene**: 0 leakage anywhere (api log, audit, result jsonb, raw MAC).
- **target_host validation**: still 400's smuggled mutation tokens before DB cast or SSH.

## Phase 10 preconditions (handed forward)

`DestructiveActionEnabled` is no longer the gate; the gate is the toggle store. Phase 10 must satisfy in order:

1. Postgres-backed `DestructiveToggle` (writes to `network_action_toggle_flips` on every Flip; `Enabled()` reads the latest row, defaults closed).
2. Postgres-backed `RBACResolver` (real role store; capability lookup by user → roles → caps).
3. Postgres-backed `MaintenanceStore` (CRUD endpoints reading/writing `network_action_maintenance_windows`).
4. API endpoints for: list/declare maintenance window; flip toggle (only `CapabilityToggleFlip` may call); pre-flight check showing `PreGateChecklist` status.
5. Idempotency-key uniqueness enforced at the DB level (per device + action_type + intent).
6. `runActionAsync` for destructive Kinds: emit `network_action.confirmed` → run pre-gate → on fail emit targeted audit (`AuditActionForGateError`) → on success emit `network_action.dry_run` (or `network_action.live_start_blocked` if Confirm=false).
7. End-to-end smoke that proves: closed toggle → request denied; open toggle + missing window → denied; full happy → executes only the explicit operator-confirmed write set.

## Rollback plan

- Migration is additive only. `network_action_toggle_flips` and `network_action_maintenance_windows` start empty and stay empty in a Phase 10A-only world; nothing depends on them.
- Code rollback = revert this commit. The Phase 9 v3 gate (`EnsureDestructiveAllowed` + `DestructiveActionEnabled` global) keeps blocking destructive actions exactly as it does today.
