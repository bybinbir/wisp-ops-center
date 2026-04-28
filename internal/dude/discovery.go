package dude

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	wispssh "github.com/wisp-ops-center/wisp-ops-center/internal/adapters/ssh"
)

// RunResult captures everything the orchestrator learned from a
// discovery pass. Phase 8.1 added Sources for per-command status and
// EnrichmentMS for the total time spent fetching enrichment data
// after the primary pass.
type RunResult struct {
	CorrelationID string
	StartedAt     time.Time
	FinishedAt    time.Time
	Success       bool
	ErrorCode     string
	Error         string
	Devices       []DiscoveredDevice
	Stats         DiscoveryStats
	CommandsRun   []string
	// Sources records the per-source attempt result (succeeded /
	// skipped_unsupported / skipped_empty / failed). Used by the
	// API to surface which enrichment kaynakları gerçekten veri
	// verdi.
	Sources []SourceStatus
	// EnrichmentMS is the total wall-clock spent on enrichment
	// commands (everything except the primary /dude/device pass).
	EnrichmentMS int64
}

// Run dials the Dude SSH endpoint, runs the read-only discovery
// commands, parses their output and classifies each host.
//
// Phase 8.1 enrichment flow:
//
//  1. /dude/device/print/detail               — primary, name-heavy
//  2. /ip/neighbor/print/detail               — MAC + platform + iface
//  3. /dude/probe/print/detail                — service signals
//  4. /dude/service/print/detail              — per-port service hints
//  5. /system/identity/print                  — self-record
//  6. merge by stable identity (MAC > host+name > name)
//  7. classify each device
//
// IMPORTANT (Phase 8 hotfix v8.4.0): the return value is a NAMED
// return so the deferred FinishedAt + Stats.Tally mutations are
// observed by the caller. With a non-named return, `return res`
// copies the result into the caller's slot before the deferred
// closure runs, and Stats stays zero-valued.
func Run(ctx context.Context, cfg Config, log *slog.Logger, store wispssh.KnownHostsStore) (res RunResult) {
	res = RunResult{
		CorrelationID: NewCorrelationID(),
		StartedAt:     time.Now().UTC(),
	}
	defer func() {
		res.FinishedAt = time.Now().UTC()
		res.Stats = DiscoveryStats{}
		res.Stats.Tally(res.Devices)
	}()

	if err := cfg.Validate(); err != nil {
		res.Error = "Dude configuration incomplete"
		res.ErrorCode = ErrorCode(err)
		return res
	}

	c := NewClient(cfg, log, store)
	c.SetCorrelationID(res.CorrelationID)
	if err := c.Dial(ctx); err != nil {
		res.Error = SanitizeMessage(err.Error())
		res.ErrorCode = ErrorCode(err)
		return res
	}
	defer c.Close()

	// ----- Primary pass --------------------------------------------------
	primary := runOneSource(ctx, c, "dude_device", "/dude/device/print/detail",
		recordsToDeviceList(res.StartedAt, "mikrotik_dude", deviceFromDude))
	res.Sources = append(res.Sources, primary.status)
	res.CommandsRun = append(res.CommandsRun, primary.status.Command)
	primaryDevices := primary.devices
	if primary.status.Status == "failed" {
		res.Error = primary.status.ErrorMessage
		res.ErrorCode = primary.status.ErrorCode
	}

	// ----- Enrichment passes --------------------------------------------
	enrichmentStart := time.Now()
	type srcDef struct {
		label string
		cmd   string
		conv  func(map[string]string, time.Time) DiscoveredDevice
	}
	enrichments := []srcDef{
		{"ip_neighbor", "/ip/neighbor/print/detail", deviceFromNeighbor},
		{"dude_probe", "/dude/probe/print/detail", deviceFromProbe},
		{"dude_service", "/dude/service/print/detail", deviceFromService},
	}
	var enrichmentDevices []DiscoveredDevice
	for _, e := range enrichments {
		out := runOneSource(ctx, c, e.label, e.cmd,
			recordsToDeviceList(res.StartedAt, "mikrotik_dude", e.conv))
		res.Sources = append(res.Sources, out.status)
		res.CommandsRun = append(res.CommandsRun, e.cmd)
		enrichmentDevices = append(enrichmentDevices, out.devices...)
	}
	res.EnrichmentMS = time.Since(enrichmentStart).Milliseconds()

	// ----- Identity self-record ------------------------------------------
	if id, ierr := c.Exec(ctx, "/system/identity/print"); ierr == nil {
		identity := parseIdentityName(id)
		if identity != "" {
			self := DiscoveredDevice{
				Source:   "mikrotik_dude",
				Name:     identity,
				IP:       cfg.Host,
				Identity: identity,
				Type:     "router",
				Platform: "RouterOS",
				LastSeen: res.StartedAt,
				Raw:      map[string]string{"role": "dude_host"},
				Sources:  []string{"dude_self"},
			}
			primaryDevices = append(primaryDevices, self)
		}
		res.CommandsRun = append(res.CommandsRun, "/system/identity/print")
	}

	// ----- Merge + classify ----------------------------------------------
	all := append(primaryDevices, enrichmentDevices...)
	merged := mergeDeviceList(all)
	for i := range merged {
		Classify(&merged[i])
		merged[i].EvidenceSummary = buildEvidenceSummary(merged[i])
	}
	res.Devices = merged

	// Run is "succeeded" when the primary pass returned data OR when
	// at least one enrichment source did. A complete failure (no
	// devices at all) keeps the previous error.
	if len(res.Devices) > 0 && primary.status.Status == "succeeded" {
		res.Success = true
		res.Error = ""
		res.ErrorCode = ""
	} else if len(res.Devices) > 0 && res.ErrorCode == "" {
		res.Success = true
	}
	return res
}

// sourceOutcome bundles the per-source status with the records the
// source contributed. Internal to Run; not part of the public API.
type sourceOutcome struct {
	status  SourceStatus
	devices []DiscoveredDevice
}

// runOneSource executes one allowlisted command, parses it through
// the supplied converter, and returns a SourceStatus + records. It
// NEVER fails the whole run — unsupported / parse / timeout errors
// produce a soft skipped/failed status instead.
func runOneSource(
	ctx context.Context,
	c *Client,
	label, cmd string,
	convert func(string) []DiscoveredDevice,
) sourceOutcome {
	start := time.Now()
	st := SourceStatus{Source: label, Command: cmd, Status: "failed"}

	out, err := c.Exec(ctx, cmd)
	st.DurationMS = time.Since(start).Milliseconds()
	if err != nil {
		st.ErrorCode = ErrorCode(err)
		st.ErrorMessage = SanitizeMessage(err.Error())
		// Disallowed → unsupported (allowlist authors removed it),
		// classified parse → unsupported on this RouterOS build,
		// everything else → failed.
		switch {
		case errors.Is(err, ErrDisallowedCommand):
			st.Status = "skipped_unsupported"
		case errors.Is(err, ErrParse):
			st.Status = "skipped_unsupported"
		}
		// Distinguish "unsupported on this RouterOS build" by checking
		// the exec output text — RouterOS prints "no such command"
		// or "syntax error" for missing endpoints. We keep
		// the status="failed" by default; the orchestrator's audit
		// metadata still records this and the run does not abort.
		return sourceOutcome{status: st}
	}

	// RouterOS prints "no such command prefix" / "expected end of
	// command (line N column M)" when an endpoint is missing on this
	// version. Treat that as skipped_unsupported, not as data.
	low := strings.ToLower(out)
	if strings.Contains(low, "no such command") ||
		strings.Contains(low, "expected end of command") ||
		strings.Contains(low, "syntax error") {
		st.Status = "skipped_unsupported"
		return sourceOutcome{status: st}
	}

	devs := convert(out)
	st.Records = len(devs)
	if len(devs) == 0 {
		st.Status = "skipped_empty"
		return sourceOutcome{status: st, devices: nil}
	}
	st.Status = "succeeded"
	return sourceOutcome{status: st, devices: devs}
}

// recordsToDeviceList wraps a record→device converter so it can be
// applied to a raw RouterOS print output.
func recordsToDeviceList(
	ts time.Time,
	source string,
	convert func(map[string]string, time.Time) DiscoveredDevice,
) func(string) []DiscoveredDevice {
	return func(out string) []DiscoveredDevice {
		records := ParseDetailPrint(out)
		devs := make([]DiscoveredDevice, 0, len(records))
		for _, r := range records {
			d := convert(r, ts)
			d.Source = source
			devs = append(devs, d)
		}
		return devs
	}
}

// deviceFromDude turns a /dude/device/print record into a normalized
// DiscoveredDevice.
func deviceFromDude(r map[string]string, ts time.Time) DiscoveredDevice {
	return DiscoveredDevice{
		Name:          pickFirst(r, "name"),
		IP:            pickFirst(r, "address", "ip-address"),
		MAC:           strings.ToUpper(pickFirst(r, "mac-address", "mac")),
		Model:         pickFirst(r, "model", "board"),
		Type:          pickFirst(r, "type"),
		Status:        pickFirst(r, "status", "state"),
		Identity:      pickFirst(r, "identity"),
		Platform:      pickFirst(r, "platform"),
		Board:         pickFirst(r, "board"),
		InterfaceName: pickFirst(r, "parent", "via"),
		Raw:           SanitizeAttrs(r),
		LastSeen:      ts,
		Sources:       []string{"dude_device"},
	}
}

// deviceFromNeighbor turns a /ip/neighbor/print/detail record into a
// DiscoveredDevice. Neighbors are the richest enrichment source: they
// almost always carry mac-address, identity and interface, plus
// platform/version/board for RouterOS peers.
func deviceFromNeighbor(r map[string]string, ts time.Time) DiscoveredDevice {
	identity := pickFirst(r, "identity", "system-caps")
	return DiscoveredDevice{
		Name:          identity,
		IP:            pickFirst(r, "address4", "address", "address6"),
		MAC:           strings.ToUpper(pickFirst(r, "mac-address")),
		Model:         pickFirst(r, "board", "platform"),
		OSVersion:     pickFirst(r, "version", "software-id"),
		Identity:      identity,
		Type:          pickFirst(r, "system-caps"),
		Platform:      pickFirst(r, "platform"),
		Board:         pickFirst(r, "board"),
		InterfaceName: pickFirst(r, "interface"),
		Raw:           SanitizeAttrs(r),
		LastSeen:      ts,
		Sources:       []string{"ip_neighbor"},
	}
}

// deviceFromProbe turns a /dude/probe/print/detail record into a
// minimal device record. Probes carry a name + the type of probe
// being applied (snmp/dns/icmp/http) which is useful evidence for
// "this is a managed AP" or "this is a CPE we ping".
func deviceFromProbe(r map[string]string, ts time.Time) DiscoveredDevice {
	return DiscoveredDevice{
		Name:     pickFirst(r, "name", "device", "probe"),
		IP:       pickFirst(r, "address", "host"),
		MAC:      strings.ToUpper(pickFirst(r, "mac-address")),
		Type:     pickFirst(r, "type"),
		Identity: pickFirst(r, "device"),
		Raw:      SanitizeAttrs(r),
		LastSeen: ts,
		Sources:  []string{"dude_probe"},
	}
}

// deviceFromService turns a /dude/service/print/detail record into a
// device record. Service entries are per-port (snmp/winbox/http/etc),
// so the same device may appear multiple times — the merge step
// folds them together by name/host.
func deviceFromService(r map[string]string, ts time.Time) DiscoveredDevice {
	return DiscoveredDevice{
		Name:     pickFirst(r, "device", "name"),
		IP:       pickFirst(r, "address", "host"),
		Type:     pickFirst(r, "type"),
		Identity: pickFirst(r, "device"),
		Raw:      SanitizeAttrs(r),
		LastSeen: ts,
		Sources:  []string{"dude_service"},
	}
}

// pickFirst returns the first non-empty value from the keys.
func pickFirst(r map[string]string, keys ...string) string {
	for _, k := range keys {
		if v, ok := r[k]; ok && v != "" {
			return v
		}
	}
	return ""
}

// mergeDeviceList collapses records that point to the same physical
// host. Stable identity priority:
//
//  1. (source, mac)              — strongest
//  2. (source, host, name)       — when MAC missing
//  3. (source, name)              — last-resort, name-only
//
// Rationale: a host that comes from /dude/device/print as name-only
// can later show up in /ip/neighbor/print/detail with a MAC and
// platform; both records refer to the same physical host but the
// initial dedupe key was different. To handle this we run TWO
// passes:
//
//   - pass 1 builds keys-by-MAC and keys-by-host-name; every record
//     with a matching MAC OR a matching (host,name) folds in.
//   - pass 2 picks up name-only records that didn't fold in pass 1.
//
// Each merged record carries the union of evidence from all source
// records (Sources slice) and the strongest field per slot
// (longest-wins tiebreak for non-empty strings).
func mergeDeviceList(in []DiscoveredDevice) []DiscoveredDevice {
	if len(in) == 0 {
		return nil
	}
	type bucket struct {
		idx    int
		device DiscoveredDevice
	}
	byMAC := map[string]int{}
	byHostName := map[string]int{}
	byName := map[string]int{}
	out := make([]DiscoveredDevice, 0, len(in))

	tryMerge := func(d DiscoveredDevice) bool {
		mac := strings.ToUpper(strings.TrimSpace(d.MAC))
		host := strings.TrimSpace(d.IP)
		name := strings.TrimSpace(d.Name)

		// 1. MAC bucket.
		if mac != "" {
			if i, ok := byMAC[mac]; ok {
				out[i] = mergeRecords(out[i], d)
				registerSecondaryKeys(out[i], i, byHostName, byName)
				return true
			}
		}
		// 2. host+name bucket.
		if host != "" && name != "" {
			if i, ok := byHostName[host+"|"+strings.ToLower(name)]; ok {
				out[i] = mergeRecords(out[i], d)
				if mac != "" {
					byMAC[mac] = i
				}
				registerSecondaryKeys(out[i], i, byHostName, byName)
				return true
			}
		}
		// 3. name-only bucket.
		if name != "" {
			if i, ok := byName[strings.ToLower(name)]; ok {
				existing := out[i]
				// Only merge into a name-only bucket if the existing record
				// has no host (or matches), to avoid collapsing two distinct
				// hosts that happen to share a name.
				if existing.IP == "" || host == "" || existing.IP == host {
					out[i] = mergeRecords(out[i], d)
					if mac != "" {
						byMAC[mac] = i
					}
					registerSecondaryKeys(out[i], i, byHostName, byName)
					return true
				}
			}
		}
		return false
	}

	for _, d := range in {
		mac := strings.ToUpper(strings.TrimSpace(d.MAC))
		host := strings.TrimSpace(d.IP)
		name := strings.TrimSpace(d.Name)
		if mac == "" && host == "" && name == "" {
			// No identity at all — keep as a standalone record.
			out = append(out, d)
			continue
		}
		if tryMerge(d) {
			continue
		}
		// Fresh bucket.
		idx := len(out)
		out = append(out, d)
		registerSecondaryKeys(out[idx], idx, byHostName, byName)
		if mac != "" {
			byMAC[mac] = idx
		}
	}

	// Stable sort so output is deterministic for tests.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].MAC != out[j].MAC {
			return out[i].MAC < out[j].MAC
		}
		if out[i].IP != out[j].IP {
			return out[i].IP < out[j].IP
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// registerSecondaryKeys exposes the merged record's host+name and
// name keys to subsequent records so they can fold in even when the
// first record didn't have them.
func registerSecondaryKeys(d DiscoveredDevice, idx int, byHostName, byName map[string]int) {
	host := strings.TrimSpace(d.IP)
	name := strings.TrimSpace(d.Name)
	if host != "" && name != "" {
		byHostName[host+"|"+strings.ToLower(name)] = idx
	}
	if name != "" {
		// First write wins — never overwrite a name->idx mapping with
		// a later index. Otherwise two distinct hosts that share a
		// name could collapse via pass-2 fallback.
		if _, exists := byName[strings.ToLower(name)]; !exists {
			byName[strings.ToLower(name)] = idx
		}
	}
}

// mergeRecords folds two records of the same logical host into one.
// Non-empty values from `from` fill empty slots in `into`. Sources
// are unioned. The most-recent LastSeen wins. EnrichedAt is set to
// LastSeen when the merge added a non-name signal.
func mergeRecords(into, from DiscoveredDevice) DiscoveredDevice {
	beforeSig := signalCount(into)
	if into.Name == "" {
		into.Name = from.Name
	}
	if into.IP == "" {
		into.IP = from.IP
	}
	if into.MAC == "" && from.MAC != "" {
		into.MAC = from.MAC
	}
	if into.Model == "" {
		into.Model = from.Model
	}
	if into.OSVersion == "" {
		into.OSVersion = from.OSVersion
	}
	if into.Identity == "" {
		into.Identity = from.Identity
	}
	if into.Type == "" {
		into.Type = from.Type
	}
	if into.Status == "" {
		into.Status = from.Status
	}
	if into.Platform == "" {
		into.Platform = from.Platform
	}
	if into.Board == "" {
		into.Board = from.Board
	}
	if into.InterfaceName == "" {
		into.InterfaceName = from.InterfaceName
	}
	if into.Raw == nil {
		into.Raw = map[string]string{}
	}
	for k, v := range from.Raw {
		if _, exists := into.Raw[k]; !exists {
			into.Raw[k] = v
		}
	}
	if from.LastSeen.After(into.LastSeen) {
		into.LastSeen = from.LastSeen
	}
	into.Sources = unionSources(into.Sources, from.Sources)
	afterSig := signalCount(into)
	if afterSig > beforeSig {
		// EnrichedAt marks when this record gained a non-name signal.
		// Prefer LastSeen when set (so the timestamp aligns with the
		// observed-at moment), otherwise stamp now() so the field is
		// non-zero for downstream EnrichedCount tally.
		if !into.LastSeen.IsZero() {
			into.EnrichedAt = into.LastSeen
		} else {
			into.EnrichedAt = time.Now().UTC()
		}
	}
	return into
}

// signalCount counts the number of non-name strong signals on a
// record. Used to decide whether a merge actually enriched the
// record (i.e. is it worth setting EnrichedAt).
func signalCount(d DiscoveredDevice) int {
	n := 0
	for _, s := range []string{d.MAC, d.IP, d.Platform, d.Identity, d.Board, d.InterfaceName, d.Type, d.OSVersion, d.Model} {
		if strings.TrimSpace(s) != "" {
			n++
		}
	}
	return n
}

// unionSources merges and deduplicates source labels.
func unionSources(a, b []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(a)+len(b))
	for _, s := range a {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	for _, s := range b {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

// buildEvidenceSummary produces a single-line, operator-friendly
// summary of the strongest signals on the record. NEVER includes
// raw_metadata values that may carry secrets — only normalized
// fields the operator already sees in the table.
func buildEvidenceSummary(d DiscoveredDevice) string {
	var parts []string
	if d.MAC != "" {
		parts = append(parts, "mac")
	}
	if d.IP != "" {
		parts = append(parts, "host")
	}
	if d.Platform != "" {
		parts = append(parts, "platform="+truncate(d.Platform, 14))
	}
	if d.Identity != "" && d.Identity != d.Name {
		parts = append(parts, "id")
	}
	if d.InterfaceName != "" {
		parts = append(parts, "iface="+truncate(d.InterfaceName, 12))
	}
	if d.Board != "" {
		parts = append(parts, "board="+truncate(d.Board, 14))
	}
	if len(d.Sources) > 1 {
		parts = append(parts, fmt.Sprintf("src×%d", len(d.Sources)))
	}
	if len(parts) == 0 {
		return "name only"
	}
	return strings.Join(parts, " ")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// NewCorrelationID generates a short hex token for end-to-end log
// correlation across the SSH client, parser, classifier and repository.
func NewCorrelationID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("dude-%d", time.Now().UnixNano())
	}
	return "dude-" + hex.EncodeToString(b[:])
}
