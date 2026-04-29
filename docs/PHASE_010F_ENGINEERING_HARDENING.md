# Phase 10F-A — Engineering hardening: RBAC + retention + alerts

Status: planned (branch `phase/010f-a-engineering-hardening`).
Depends on: Phase 10E PR-A merged at main `f019cac`.

## Why

Phase 10E PR-A landed the real `frequency_correction` Execute path
but kept production wiring deferred so destructive execution remains
unreachable from the public API. The deployment now needs the
remaining engineering primitives that PR-B and operator workflows
will rely on, and that the security review explicitly flagged as
soft spots:

1. **SQL-backed RBAC**, fail-closed under operator promise. The
   resolver already had `RequireSQL` but a missing pool silently
   degraded to header roles — a fail-open path.
2. **KVKK-safe retention foundation** so the operator can count or
   delete old audit + run rows under explicit configuration. Today's
   default is "do nothing"; the foundation makes the intent
   reviewable.
3. **Destructive-action alert rule pack** so an oncall pager fires
   on the worst-case shapes (succeeded unexpectedly, rollback failed,
   secret/MAC leak detector). Metrics surface lands later; until then
   the rules reference forward-compatible counter names + an audit
   query fallback book.

Phase 10F-A is **engineering only**. Production wiring (system unit,
metrics surface, retention cron) is intentionally NOT included; each
of those is a small follow-up PR.

## Non-goals (intentionally deferred)

- Phase 10E PR-B: mikrotik adapter `FrequencyCorrectionWriter`
  implementation, `server.go` registry wiring, lab smoke against a
  real wireless target.
- Toggling `var DestructiveActionEnabled = false` (`internal/networkactions/phase10_pregate.go:67`).
- Toggling the provider master switch (`MemoryToggle` /
  `PgToggleStore`).
- Performing any device write.
- Adding a Prometheus `/metrics` endpoint to the API server (the
  alert rules reference forward-compatible names; the surface lands
  in Phase 10F-B or later).
- Installing a cron / systemd unit for the retention service.
- TLS / KMS / Vault rotation (operator scope).

## Components

### `internal/networkactions/pg_rbac.go` (extended)

`PgRBACResolver.Capabilities` now refuses the request with
`ErrRBACResolverUnavailable` when `RequireSQL=true` AND the SQL seam
is unwired. Previously the resolver fell through to the header-based
static fallback, which violated the RequireSQL contract: the
operator's promise was that no header role would authorise a
destructive call once strict mode was on. Phase 10F-A makes that
promise enforceable: forgetting to wire the SQL pool is a hard deny.

The change is a single `if r.RequireSQL && r.SQL == nil { return …
ErrRBACResolverUnavailable }` guard before the header fallback
branch. Existing behaviour for `RequireSQL=false` is preserved
exactly.

### `internal/retention/` (new package)

Foundation only — no scheduler, no DB pool. The package ships:

- `Config` with `Mode` (`disabled` / `dry_run` / `execute`) +
  per-table `RetentionDays`. Default is `Mode=disabled`,
  `RetentionDays=0` — calling `Cleanup()` against the zero-value
  config emits a single `retention.disabled` audit row and returns
  no summaries.
- `Store` interface (`CountCandidates`, `DeleteOlderThan`) so
  production wiring can plug in a pgxpool implementation while
  tests stay hermetic.
- `Service.Cleanup()` walks each configured table, counts
  candidates, and (in `ModeExecute` only) issues bounded DELETEs.
  Tables flagged `Protected` by the store (audit_logs, where the
  application role has had DELETE revoked since migration `000002`)
  are recorded as count-only without raising the run as failed.
- 5 audit event names: `retention.disabled`,
  `retention.dry_run_counted`, `retention.deleted`,
  `retention.table_protected`, `retention.failed`. Each row carries
  table + cutoff (RFC3339) + actor + counts; never a row sample.
- A defensive sanitiser that strips DSN credentials + `password=`
  / `token=` style substrings out of any store error before it
  reaches the audit row.

The package's allowlist is hard-coded:

```
TableAuditLogs            (count-only by default; protected by DB role)
TableNetworkActionRuns    (count + delete eligible)
```

Adding a third table requires a code change + review. Anything else
fails `Config.Validate()`.

### `infra/prometheus/destructive_alerts.rules.yml` (new)

Nine alert rules grouped under `wisp-destructive-safety` and
`wisp-retention-housekeeping`:

| Alert | Severity | Trigger |
|---|---|---|
| WispDestructiveSucceededUnexpected | critical | any successful destructive run |
| WispDestructiveAttemptedWhileDisabled | warning | `live_start_blocked` spike |
| WispDestructiveVerifyFailedRollbackRecovered | warning | verify miss + rollback success |
| WispDestructiveRollbackFailed | critical | rollback could not restore snapshot |
| WispDestructiveExecuteNotImplementedSpike | warning | client retry against stub Kinds |
| WispProviderToggleEnabledOutsideWindow | warning | toggle ON + no active maintenance window |
| WispSecretLeakDetected | critical | secret-shaped substring in audit/result jsonb |
| WispRawMacLeakDetected | critical | raw 6-octet MAC in audit/result jsonb |
| WispRetentionDeletedLargeBatch | warning | retention deleted > 100k rows in 1h |

Every rule references a `wisp_*` metric name that the API server
does not yet export. The metric layer lands in a separate PR
(Phase 10F-B or later); until then the rules are inert (PromQL on
an unknown metric returns empty), which is safe.

### `docs/SAFETY_ALERTING.md` (new)

The operator-facing companion to the rules file. For every alert
above the doc gives the SQL fallback the oncall operator runs by
hand against `audit_logs` + `network_action_runs` until the
metrics surface ships. This is intentional — the deployment must
not lose alerting just because the PromQL backend is not ready.

### `internal/alerts/` (new package)

Test-only Go package that walks up to `infra/prometheus/`,
asserts the rules file exists, contains every named alert, has the
expected YAML shape (groups, expr, labels, summary, description),
and that severities are populated. Catches drift (rename, drop)
in CI without pulling in a YAML parser dependency.

## Test plan

PR-A new tests (engineering, sandbox-doable):

- `internal/networkactions/phase10f_rbac_strict_test.go` — 8 tests:
  - allow under SQL grants with header role overridden
  - deny on `ErrPrincipalUnknown` + RequireSQL=true
  - deny on DB error + RequireSQL=true
  - deny when SQL seam unwired + RequireSQL=true (the new hardening)
  - deny on nil Fallback + RequireSQL=true
  - deny on empty actor + RequireSQL=true
  - regression: RequireSQL=false still falls back to header roles
  - HasCapability returns false on every misconfig under RequireSQL=true
- `internal/retention/service_test.go` — 8 tests:
  - disabled mode is a no-op
  - empty mode equals disabled
  - dry-run counts but does not delete
  - cutoff boundary date is correct
  - execute respects per-table protection
  - zero retention days opts out
  - invalid config fails closed at `New()` (5 sub-cases)
  - count error is sanitised + does not abort the next table
- `internal/alerts/rules_test.go` — 4 tests:
  - rules file exists at the expected path
  - every named alert is present
  - YAML shape is sane (groups + expr + severity)
  - every alert carries summary + description annotations

Total new tests: ~20.

## Safety invariants (post-merge, hermetic)

- `var DestructiveActionEnabled = false` (line 67) untouched.
- Provider toggle latest row remains `enabled=false`.
- `apps/api/internal/http/server.go` untouched — no
  `RegisterFrequencyCorrection` call lands.
- `internal/networkactions/Registry.NewRegistry()` still maps
  `KindFrequencyCorrection` to the Phase 8 stub; the API path
  cannot reach the real Phase 10E Execute.
- `internal/adapters/mikrotik/allowlist.go` untouched — read-only
  ForbiddenSegments preserved.
- `internal/adapters/mikrotik/destructive_write.go` referenced
  only by Phase 10E action; this PR does not extend it.
- No migration ships (no schema change).
- 0 destructive succeeded reachable from API.
- 0 mutation cmd, 0 secret leak, 0 raw 6-octet MAC.

## Migration

None. Phase 10C migration `000013` already supplies all schema.

## Rollback plan

`git revert <merge sha>` removes:
- the new `internal/retention/` + `internal/alerts/` packages,
- the new test file `phase10f_rbac_strict_test.go`,
- the alert rules file + safety alerting doc,
- the single `if r.RequireSQL && r.SQL == nil` guard in
  `pg_rbac.go`.

No DB rollback needed. No data is at risk because both master
switches stay closed across the change and the retention service
has no production wiring.

## Phase 10F-B — handed forward

- Wire `RegisterFrequencyCorrection` once a wireless lab target
  is available (Phase 10E PR-B).
- Add the API server's `/metrics` endpoint and the counters /
  gauges referenced by the alert rules.
- Add the retention scheduler invocation (cron / systemd unit
  pattern that fits the deployment) once an operator has signed
  off on a non-zero retention horizon.
- Flip `RequireSQL=true` in the production-wired `PgRBACResolver`
  after the `network_action_role_grants` table is seeded.
