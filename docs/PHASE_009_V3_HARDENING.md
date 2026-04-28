# Phase 9 v3 — Credential resolver + target_host validation + Phase 10 pre-gate

Status: in-flight (branch `phase/009-v3-hardening-credential-target-validation`)
Depends on: Phase 9 v2 merged (main `ab97226`).

## Why

Phase 9 / 9 v2 left three follow-ups marked as "non-blocking but should land before Phase 10":

1. **Credential reuse is implicit.** Every action uses the Dude admin password directly from server config. Phase 10 (frequency_correction) needs an explicit place to plug per-device credential profiles, and an explicit typed error when the profile is missing.
2. **Invalid `target_host` returns 500** from the DB inet-cast layer. We never reach SSH (defensive), but a 400 is the correct status and protects us from accidentally logging the smuggled payload as a server error.
3. **Phase 10 pre-gate** (RBAC, maintenance window, rollback note, idempotency, master switch) has no central enforcement point. Spreading the checks across Phase 10 implementation files is how guardrails go missing.

Phase 9 v3 closes all three with **no destructive code**, **no schema change**, and **dry_run=true preserved**.

## Components

### `internal/networkactions/credentials.go`

- `CredentialResolver` interface — resolves SSH credentials per-target.
- `SecretProvider` interface — hides where the password material comes from (vault, keyring, env).
- `MemorySecretProvider` — hermetic in-memory provider for tests.
- `DudeFallbackResolver` — backward-compatible Dude-reuse path. Tags every consultation with the constant profile id `DudeStaticProfile = "dude_static_admin"`, so the audit log can name the credential bucket without ever logging the secret value.
- `ErrCredentialNotFound` — typed sentinel; bubbles up through `ErrorCode()` as `credential_not_found`. The action runner MUST NOT dial when this is returned.

The plaintext password lives only inside `SSHTarget.Password` for the lifetime of one action. It never crosses the API/DB/audit boundary; tripwire test `TestSecretsNotInExportedTypeStrings` guards against a future Stringer that would silently print it.

### `internal/networkactions/host_validate.go`

- `ValidateTargetHost(host)` returns `ErrInvalidTargetHost` for: empty/whitespace, contains `=`/space/`,`/`;`/`?`/`#`/quotes/backslash, malformed IPv4/IPv6, hostname rule violations (label > 63, total > 253, leading/trailing `-` or `.`, underscores).
- Runs BEFORE the DB `inet` cast and BEFORE any SSH dial. Smuggled mutation tokens (`frequency=5180`, `disabled=no`) get a clean 400.

### `internal/networkactions/phase10_pregate.go`

- `EnsureDestructiveAllowed(ctx, DestructiveRequest)` is the single entry point Phase 10 must call before executing any `IsDestructive()` action.
- Master switch `DestructiveActionEnabled = false` blocks every destructive request in Phase 9 v3 — even a perfectly-shaped one. Phase 10 will flip this only after the rest of the gate is satisfied.
- Guardrail order: master switch → Kind.IsDestructive() → RBAC → explicit Confirm → maintenance window present → window open at Now → rollback note non-empty → idempotency key non-empty.
- `PreGateChecklist()` enumerates 14 invariants for docs / TASK_BOARD; numerical guard test (`TestPreGateChecklist_LeastFourteenItems`) prevents accidental shrinkage.
- `DestructiveErrorCode(err)` maps every typed sentinel to a stable, short label.

### Wiring

- `apps/api/internal/http/server.go` instantiates `s.actionCreds = &networkactions.DudeFallbackResolver{...}` at boot.
- `apps/api/internal/http/handlers_actions.go`:
  - target resolution now calls `networkactions.ValidateTargetHost(host)` between the inventory lookup and the DB write — invalid host → 400 with `error: invalid_target_host`.
  - `runActionAsync` no longer takes the credentials as parameters; it calls `s.actionCreds.Resolve(ctx, deviceID, host)`. A typed `ErrCredentialNotFound` triggers `handleActionCredentialFailure` which finalizes the run row + emits `network_action.failed` with `error_code=credential_not_found` — and **never dials SSH**.
  - audit metadata names the credential bucket via the constant `DudeStaticProfile` (no secret value).

## Test evidence

```
gofmt -l                  clean
go vet ./...              clean
go build ./...            clean
go test ./...             PASS
go test ./internal/networkactions/ -v -count=1
                          58 tests PASS
                          (Phase 9: 23 + Phase 9 v2: 20 + Phase 9 v3: 15)
npm run build             /aksiyonlar 6.26 kB, /ag-envanteri 6.43 kB
```

15 new networkactions tests:
- target_host validation: literals, smuggled-payload rejection, empty rejection, RFC1123 edge cases, port guard.
- credential resolver: happy path, typed error when missing, ErrNotConfigured when username empty, secret-leak tripwire.
- Phase 10 pre-gate: master switch blocks all, non-destructive bypass, every guardrail (RBAC, intent, window-missing, window-closed, rollback, idempotency), checklist size, error code mapping.

## Live smoke evidence

(filled in by the live-smoke step against the operator's Dude host)

## Migration

**No migration in this PR.** Phase 9 v3 is pure code (resolver abstraction + validator + pre-gate file). Schema unchanged.

## Phase 10 preconditions (the work this PR unblocks)

For Phase 10 (`frequency_correction`) to flip `DestructiveActionEnabled = true`, every item below must be in place:

- [ ] Operator-controlled runtime toggle (config + audit-logged flip), wired through `EnsureDestructiveAllowed`.
- [ ] RBAC store (real role lookup beyond the constant `AllowedRolesForDestructive`).
- [ ] Maintenance window CRUD + verification at action create-time AND start-time.
- [ ] Rollback metadata captured in `network_action_runs` (column or jsonb) and surfaced in the API response.
- [ ] Idempotency key uniqueness enforced at the DB level (unique partial index per device + action_type + intent).
- [ ] Audit event coverage for: confirm received, gate failure, dry-run completed, live execution started, rollback executed.
- [ ] Per-device lock + rate limit re-verified end-to-end (Registry already provides both).
- [ ] Deny-list regression test: every variant of frequency apply / set / add / remove / reboot / bandwidth-test / torch / sniffer remains blocked AFTER frequency_correction is wired (test fails if a code-path slips a write past the allowlist).

## Rollback plan

- No schema change — nothing to revert at the DB layer.
- Code rollback = revert this commit. The action runner falls back to the old "credentials handed by-value to runActionAsync" pattern; tests at HEAD continue to PASS.
