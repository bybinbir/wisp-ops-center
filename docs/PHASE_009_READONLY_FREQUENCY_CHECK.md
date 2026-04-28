# Phase 9 — Read-only Network Actions + frequency_check

Status: in-flight (branch `phase/009-readonly-frequency-check-actions`)
Depends on: Phase 8.1 merged (main `10cc360`).

## Scope

Convert `internal/networkactions` from Phase 8 scaffolding into a real, production-grade read-only action framework. Ship the first concrete action: `frequency_check`. No mutation paths are exposed; the allowlist + a separate segment denylist veto every write verb.

## Components

### `internal/networkactions`

| File | Role |
| ---- | ---- |
| `framework.go` | Existing: action Kind enum, Registry, RateLimiter, MaintenanceWindow. Phase 9 added `Result.Result map[string]any` so concrete actions can attach a structured payload. |
| `types.go`     | `RunStatus`, `ActionRun` (DB row projection), `FrequencyCheckResult`, `WirelessSnapshot`, `SourceCommand`. |
| `errors.go`    | Stable sentinels + `ErrorCode` mapper. |
| `allowlist.go` | Closed allowlist of read-only commands + segment denylist + token denylist. `EnsureCommandAllowed` is the only entry point. |
| `sanitize.go`  | `SanitizeAttrs`, `SanitizeMessage`, `SanitizeResultMap` — secret redaction before audit/DB. |
| `parser.go`    | RouterOS `print detail` parser + per-menu wireless converters (`parseWirelessLegacy`, `parseWifi`, `parseWifiwave2`) + registration aggregator. |
| `ssh.go`       | `SSHTarget`, `SSHSession`. Mirrors the dude SSH client; honors TOFU/Pinned host-key policy. |
| `freqcheck.go` | `FrequencyCheckAction.Execute`. 3-menu probe with soft-skip on unsupported builds. |
| `repository.go`| `network_action_runs` CRUD. |

### `apps/api/internal/http/handlers_actions.go`

| Endpoint | Method | Purpose |
| -------- | ------ | ------- |
| `/api/v1/network/actions/frequency-check` | POST | Create + start a read-only frequency_check run; 202 with run_id. |
| `/api/v1/network/actions/{id}` | GET | Fetch a single run with structured result. |
| `/api/v1/network/actions`      | GET | List recent runs, filter by `?action_type=&status=&device_id=`. |

The async flow matches Phase 8 dude discovery: handler persists a `queued` row, emits `network_action.start`, launches a goroutine that runs `FrequencyCheckAction.Execute`, finalizes the row + emits the terminal audit event.

### Migration `000010_network_action_runs.sql`

Idempotent + transactional. ALTER/CREATE only, no DROP. Adds:
- `network_action_runs` table with action_type CHECK, status CHECK, sanitized result jsonb, command_count, warning_count, confidence (0-100), correlation_id, dry_run boolean.
- 4 indexes (started_at, status, action_type, target_device_id).

### UI

- New `/aksiyonlar` page with a master/detail layout. Lists frequency_check runs with auto-refresh while any run is `queued`/`running`. Detail panel shows interfaces, warnings, evidence.
- `/ag-envanteri` table gets a **Frekans Kontrol** button per row. Disabled when the device has no host. Click → POST → success links to `/aksiyonlar`.
- The page is clearly labeled read-only and notes that mutation flows are deferred to Phase 10.

## Safety proof

- `EnsureCommandAllowed` is the SINGLE entry point for SSH `Exec`. Tests cover:
  - `TestEnsureCommandAllowed_AcceptsKnownReadOnly` — every allowlisted command passes.
  - `TestEnsureCommandAllowed_BlocksMutationCommands` — set/add/remove/enable/disable/reset/reboot/shutdown/import/export/file/tool blocked.
  - `TestEnsureCommandAllowed_BlocksFrequencyApply` — `set frequency=`, `set channel-width=`, `set disabled=` blocked.
  - `TestAllowlist_AllReadOnlyEndingsAreSafe` — every allowlist entry terminates in print/detail.
  - `TestEnsureCommandAllowed_RejectsEmpty` — empty/garbage rejected.
- `denyMutationTokens` covers smuggled `frequency=`, `channel-width=`, `disabled=`, `password=`, `secret=`, `token=`, etc. — even if a caller crafts a bogus path.
- `SanitizeAttrs` / `SanitizeMessage` / `SanitizeResultMap` redact secret keys + secret prefixes before audit/DB persistence. Tests prove nested map + slice walking.
- The action sets `dry_run=true` unconditionally; mutating actions are still stub-only in the registry.

## Test evidence

```
go test ./internal/networkactions/ -v   PASS  (23 tests)
go test ./...                            PASS
go vet ./...                             clean
gofmt -l                                 clean
go build ./...                           clean
npm run build                            /aksiyonlar 4.77 kB, /ag-envanteri 5.94 kB
```

Phase 9 added these required tests:

- Allowlist (5): accepts known, blocks mutation, blocks frequency apply, structural read-only verb invariant, rejects empty.
- Parser (6): legacy wireless, wifi, wifiwave2, registration aggregation, parseSignedInt, parseMbps.
- Sanitize (3): attrs redaction, message redaction, nested result map redaction.
- Frequency check (5): no-wireless skipped, legacy happy path, dial failure → failed, blocked command guard, analyze produces warnings.

## Live smoke evidence

(to be filled in by the next step — apply migration 000010, run frequency_check against the operator's Dude host)

## Known limitations

- The action targets `network_devices` (Phase 8 inventory). Devices with no `host` cannot be checked; the UI shows the button disabled.
- Phase 9 reuses Dude SSH credentials for target devices. A future phase will add per-device credential profiles.
- The Dude host itself may not have wireless interfaces; in that case the run terminates `skipped` with `skipped_reason="no_wireless_menu_or_no_interface"` — no fake data.
- The result payload is sanitized but NOT encrypted; do not expose this API to untrusted networks without TLS.

## Rollback plan

- Migration is additive only (CREATE TABLE IF NOT EXISTS + indexes). To roll back, run:
  ```sql
  -- emergency rollback (only if Phase 9 must be removed):
  -- DROP TABLE network_action_runs;
  ```
  The drop is intentionally NOT in the migration file (Phase 9 contract is no DROPs).
- Code-level rollback: revert the merge commit. The framework, allowlist, parser, and tests live in `internal/networkactions/`; removing the API routes + UI page is sufficient to disable the surface area without dropping the DB table.
