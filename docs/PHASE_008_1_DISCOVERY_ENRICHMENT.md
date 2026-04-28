# Phase 8.1 — Discovery Enrichment + Inventory Classification Evidence

Status: in-flight (branch `phase/008-1-discovery-enrichment`)
Depends on: Phase 8 merged (main `392ce1e8`).

## Problem

Phase 8 shipped MikroTik Dude SSH discovery and persisted 893 devices in lab smoke. But the primary RouterOS source (`/dude/device/print/detail`) on this Dude install only returns name + a few attributes per row — most rows had no `mac-address`, no `platform`, no `identity`. As a result:

- 892/893 devices stayed in `category=Unknown`.
- 893/893 devices stayed below `confidence=50` (low confidence).
- Operators cannot trust the inventory enough to build network-action workflows on top of it (Phase 9).

## Solution

Phase 8.1 adds **read-only enrichment sources** that run after the primary pass and merge their signals into the same record by stable identity. No new mutating commands. No frequency apply. No bandwidth-test. Allowlist gains exactly three read-only entries.

### Enrichment sources

| Order | Command | Purpose |
| ----- | ------- | ------- |
| 1 | `/dude/device/print/detail`  | Primary (Phase 8) — name + address + type when set in Dude. |
| 2 | `/ip/neighbor/print/detail`  | NEW — MAC, identity, platform, board, version, interface for adjacent RouterOS hosts. |
| 3 | `/dude/probe/print/detail`   | NEW — per-device probe table (snmp/dns/icmp/http) for "what is reachable & how". |
| 4 | `/dude/service/print/detail` | NEW — per-port service entries; supplementary platform/identity hints. |
| 5 | `/system/identity/print`     | Self-record (Phase 8 unchanged). |

Every command is on the dude allowlist (`internal/dude/allowlist.go`) and has unit-test coverage:

- `TestAllowlist_EnrichmentCommandsAreReadOnly`
- `TestAllowlist_BlocksMutationCommands`
- `TestAllowlist_BlocksBandwidthAndFrequencyApply`
- `TestAllowlist_AllCommandsEndWithPrintDetailOrSafeReadOnlyEquivalent`

If the RouterOS build does not support a probe/service endpoint, `runOneSource` records `status="skipped_unsupported"` instead of failing the run.

### Merge algorithm

`mergeDeviceList` in `internal/dude/discovery.go` collapses records by stable identity. Priority:

1. `(source, mac)` — strongest.
2. `(source, host, name)` — when MAC missing.
3. `(source, name)` — last-resort, name-only.

Records folded into a bucket update `into.Sources` (union) and lift the strongest field per slot. When a merge contributes a non-name signal (MAC, platform, identity, board, interface) the record gains a `EnrichedAt` timestamp.

### Classifier rebuild

`internal/dude/classify.go` now treats name as a weak hint and combines:

- Dude type (when explicit) — strongest deterministic signal.
- Name patterns — weak hint.
- Wireless-mode evidence (ap-bridge / station / station-bridge / bridge).
- Interface-type / board / model hints.
- Platform / identity (RouterOS, Mimosa, AirOS).
- Interface-name hint (wlan1-ap, ether1-uplink, wlan1-bh).

Confidence:

- Name-only stays under 50.
- MAC + platform + identity + interface pushes confidence to 60+.
- When the second-place category is within 15 pts, the record gets a -15 conflict penalty and an explicit `conflict_penalty` evidence note.
- Devices with insufficient evidence stay `Unknown` with `confidence=0`.

Required tests:

- `TestClassifier_RouterFromPlatform`
- `TestClassifier_APFromNameAndWirelessEvidence`
- `TestClassifier_CPEFromClientEvidence`
- `TestClassifier_BackhaulFromLinkEvidence`
- `TestClassifier_BridgeFromBridgeEvidence`
- `TestClassifier_SwitchFromSwitchEvidence`
- `TestClassifier_UnknownWhenInsufficientEvidence`
- `TestClassifier_ConfidenceIncreasesWithEnrichment`
- `TestClassifier_ConflictingEvidenceLowConfidence`

### DB / API

Migration `000009_phase81_discovery_enrichment.sql` (idempotent + transactional) adds:

- `network_devices`: `platform`, `board`, `interface_name`, `evidence_summary`, `enrichment_sources text[]`, `last_enriched_at`.
- `discovery_runs`: `enrichment_sources_attempted`, `enrichment_sources_succeeded`, `enrichment_sources_skipped`, `enrichment_duration_ms`, `with_mac_count`, `with_host_count`, `enriched_count`.

API filters (`GET /api/v1/network/devices`) gain `?has_mac=1`, `?enriched=1`, `?source=mikrotik_dude`. Response payload exposes `platform`, `interface_name`, `evidence_summary`, `enrichment_sources`, `last_enriched_at`. Summary block adds `with_mac`, `with_host`, `enriched`.

### Audit

`network.dude.run.finish` audit event metadata now includes `enrichment_sources_attempted/succeeded/skipped`, `enrichment_duration_ms`, `with_mac_count`, `with_host_count`, `enriched_count`, `category_unknown`, `low_confidence_count`, `source_status` (per-source dict). All values are integers, hard-coded labels, or sanitized error codes — no raw RouterOS text.

### Web

`/ag-envanteri` adds:

- An "Enrichment kaynakları" card showing the attempted/succeeded/skipped status of each source, plus the wall-clock and the with_mac / with_host / enriched counters.
- A 4th stat-card row: MAC kazandı, Host kazandı, Enriched, Düşük Confidence.
- New filters: "MAC var", "Enriched".
- Two new table columns: "Kanıt" (evidence_summary), "Platform" (platform + interface_name).

## Safety

- `internal/dude/allowlist.go` keeps a closed allowlist; new tests reject `set/add/remove/enable/disable/reset/reboot/bandwidth-test/frequency-apply` and verify every entry ends in a `print` or `detail` verb.
- `SanitizeAttrs` redacts secret-like keys before any record reaches `raw_metadata`.
- `SanitizeMessage` redacts known secret prefixes before any error reaches the API or audit log.
- The discovery worker wraps Run/UpsertDevices/FinalizeRun in a panic recovery; the run row is finalized as `failed` with `error_code=panic_recovered` if the worker dies.

## Limitations

- If the operator's RouterOS build truly does not return MAC on `/ip/neighbor/print/detail` or has the dude packages disabled, Unknown count may stay high — the system reports this honestly via `enrichment_sources_skipped`. The PR is still mergeable in that case because the existing 893-device baseline + dedupe + audit + safety remain intact.
- No fake categories are produced. A device with no signal stays `Unknown`.
