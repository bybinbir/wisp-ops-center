# Phase 9 v2 — Read-only AP, Link and Bridge Actions

Status: in-flight (branch `phase/009-v2-readonly-ap-link-bridge-actions`)
Depends on: Phase 9 merged (main `769426e`).

## Scope

Extend the read-only Network Actions framework with three additional safe operations on top of `frequency_check`:

1. `ap_client_test`        — AP-side client health (signal/ccq/rate) without touching configuration.
2. `link_signal_test`      — Point-to-point link quality from registration table; no bandwidth-test.
3. `bridge_health_check`   — Bridge + bridge port read-only state (no MAC host table, no topology change).

Every action reuses Phase 9's allowlist + sanitize layer + repository + audit pipeline. No migration is required — `network_action_runs.action_type` already declares all four kinds in its CHECK constraint (Phase 9's migration 000010 was forward-compatible).

## Action summary

| Kind                | RouterOS commands attempted (read-only)                                                                                                  | Skipped path                                       |
| ------------------- | ----------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------- |
| ap_client_test      | system/identity, interface/{wireless\|wifi\|wifiwave2}/print/detail, …/registration-table/print/detail                                    | no_wireless_menu \| no_registered_clients          |
| link_signal_test    | same surface as ap_client_test; chooses the best PtP candidate by mode + peer count                                                       | no_wireless_menu \| no_registered_peer_on_any_iface |
| bridge_health_check | system/identity, interface/bridge/print/detail, interface/bridge/port/print/detail, interface/print/detail (cross-reference)              | no_bridge_configured                                |

Allowlist additions (Phase 9 v2):
- `/interface/bridge/print` + `/interface/bridge/print/detail`
- `/interface/bridge/port/print` + `/interface/bridge/port/print/detail`

Bridge **host** table (`/interface/bridge/host/print`) is intentionally NOT allowlisted: it would expose attached customer device MACs through the action result. Tests prove this stays blocked.

## Safety proof

- `EnsureCommandAllowed` continues to gate every `Exec`. New tests added:
  - `TestEnsureCommandAllowed_AcceptsBridgeReadOnly` — bridge read paths accepted.
  - `TestEnsureCommandAllowed_BlocksBridgeMutation` — bridge add/remove/set/enable/disable blocked; even read-only `bridge/host/print` blocked.
  - `TestEnsureCommandAllowed_NoBandwidthTestAnywhere` — every variant of bandwidth-test/torch/sniffer rejected.
- All three new actions set `dry_run=true` unconditionally.
- `MACPrefix` masking truncates client MACs to a 5-octet prefix (`AA:BB:CC:DD:EE:**`) before any result jsonb write — operator can spot repeated bad prefixes without holding fully-resolvable customer device IDs.
- No new mutation token reaches the SSH layer; `denyMutationTokens` continues to reject `frequency=`, `channel-width=`, `disabled=`, `password=`, `secret=`, `token=`, etc.
- `SanitizeResultMap` is applied on every result before persistence (covers nested maps + slices).

## Test evidence

```
gofmt -l                 clean
go vet ./...             clean
go build ./...           clean
go test ./...            PASS
go test ./internal/networkactions/ -v   43 tests PASS  (Phase 9: 23, +20 new)
npm run build            /aksiyonlar 6.26 kB, /ag-envanteri 6.43 kB
```

New networkactions tests added (20):
- Allowlist: `BridgeReadOnly`, `BridgeMutation`, `NoBandwidthTestAnywhere` (3)
- Parser: `BridgeList`, `BridgePorts`, `ExtractClients`, `ParseUptimeToSeconds`, `MaskMAC` (5)
- AP client test: happy/weak+lowccq, no clients, no wireless menu, dial failure (4)
- Link signal test: PtP healthy, critical, no peer skipped, dial failure (4)
- Bridge health check: healthy, down/disabled, no bridge skipped, malformed output (4)

## Live smoke evidence

(filled in by the live-smoke step against the operator's Dude host)

## Known limitations

- The Dude host has no wireless interfaces or bridge ports configured; live smoke for ap_client_test / link_signal_test / bridge_health_check is expected to land in the `skipped` path. This is the honest outcome and matches Phase 9's contract: no fake data.
- Per-device credential profiles still deferred; all three actions reuse Dude SSH credentials.
- Invalid `target_host` still returns 500 from the DB inet-cast layer (defensive — never reaches SSH).

## Rollback plan

- No schema change. Migration 000010 from Phase 9 already covers all four action_type values via its CHECK constraint, so nothing to revert at the DB layer.
- Code rollback = revert this commit. The framework, allowlist additions, parsers, actions, API endpoints and UI page can all be reverted as one diff.
