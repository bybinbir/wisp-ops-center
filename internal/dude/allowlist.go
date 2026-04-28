package dude

import "strings"

// allowed holds the exact set of RouterOS CLI commands the Dude
// adapter is permitted to run. EVERY command must be read-only.
//
// Phase 8.1 enrichment: only NEW read-only print/detail commands
// were added. No mutation verb (set/add/remove/enable/disable/reset/
// reboot/bandwidth-test) appears anywhere on this list.
var allowed = map[string]struct{}{
	// --- system & inventory (Phase 8) ---------------------------------
	"/system/identity/print":     {},
	"/system/resource/print":     {},
	"/system/routerboard/print":  {},
	"/system/package/print":      {},
	"/ip/address/print":          {},
	"/ip/neighbor/print":         {},
	"/ip/neighbor/print/detail":  {},
	"/ip/arp/print":              {},
	"/interface/print":           {},
	"/interface/print/detail":    {},
	"/interface/bridge/print":    {},
	"/interface/wireless/print":  {},
	"/interface/ethernet/print":  {},
	"/dude/device/print":         {},
	"/dude/device/print/detail":  {},
	"/dude/network/print":        {},
	"/dude/network/print/detail": {},
	"/dude/probe/print":          {},

	// --- Phase 8.1 enrichment additions -------------------------------
	// Probes carry per-device "what is up" semantics (snmp/dns/http/
	// icmp etc.). Reading them helps classify a host as router vs AP
	// vs CPE without ever sending mutation traffic.
	"/dude/probe/print/detail": {},
	// Services are the per-device service table inside Dude (e.g.
	// "snmp on 161/udp", "winbox on 8291/tcp"). Useful evidence for
	// "this device runs RouterOS" or "this is an AP managed by Dude"
	// — purely read-only.
	"/dude/service/print":        {},
	"/dude/service/print/detail": {},
}

// EnsureAllowed returns ErrDisallowedCommand for any command not on
// the allowlist.
func EnsureAllowed(cmd string) error {
	cmd = strings.TrimSpace(cmd)
	if _, ok := allowed[cmd]; ok {
		return nil
	}
	return ErrDisallowedCommand
}

// AllowedCommands returns a copy of the allowlist for tests and
// docs.
func AllowedCommands() []string {
	out := make([]string, 0, len(allowed))
	for k := range allowed {
		out = append(out, k)
	}
	return out
}

// EnrichmentCommands lists the commands that the Phase 8.1 enrichment
// pipeline attempts after the primary /dude/device/print/detail. Each
// is read-only and may be unsupported on a given RouterOS/Dude build,
// in which case the discovery run records a skipped source rather
// than failing the entire run.
var EnrichmentCommands = []string{
	"/ip/neighbor/print/detail",
	"/dude/probe/print/detail",
	"/dude/service/print/detail",
}
