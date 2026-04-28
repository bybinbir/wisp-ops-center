package networkactions

import "strings"

// readOnlyCommands is the closed allowlist of RouterOS CLI paths the
// action runner is permitted to execute. EVERY entry must terminate
// in a `print`, `print detail`, or other documented read-only verb.
//
// Phase 9 frequency_check uses this set; later phases will extend it
// only with explicit read paths. Mutation verbs (set/add/remove/
// enable/disable/reset/reboot/upgrade/import/export/bandwidth-test)
// are forbidden by both the structural allowlist (this file) AND the
// segment denylist (denyMutationSegments below).
var readOnlyCommands = map[string]struct{}{
	// system & inventory
	"/system/identity/print":    {},
	"/system/resource/print":    {},
	"/system/routerboard/print": {},

	// frequency_check legacy wireless menu (RouterOS <=6.x).
	"/interface/print":                                    {},
	"/interface/print/detail":                             {},
	"/interface/wireless/print":                           {},
	"/interface/wireless/print/detail":                    {},
	"/interface/wireless/registration-table/print":        {},
	"/interface/wireless/registration-table/print/detail": {},

	// frequency_check wifi (RouterOS >=7.x compact menu).
	"/interface/wifi/print":                           {},
	"/interface/wifi/print/detail":                    {},
	"/interface/wifi/registration-table/print":        {},
	"/interface/wifi/registration-table/print/detail": {},

	// frequency_check wifiwave2 (RouterOS 7.x preview menu).
	"/interface/wifiwave2/print":                           {},
	"/interface/wifiwave2/print/detail":                    {},
	"/interface/wifiwave2/registration-table/print":        {},
	"/interface/wifiwave2/registration-table/print/detail": {},
}

// denyMutationSegments is the segment-level veto list. Even a path
// that somehow slips past readOnlyCommands MUST NOT contain any of
// these segments, period. Comparison is segment-by-segment so a
// benign "/ip/address/print" is not rejected just because "address"
// starts with "add".
var denyMutationSegments = []string{
	"set", "add", "remove", "enable", "disable",
	"reset", "reboot", "shutdown", "upgrade",
	"import", "export",
	"bandwidth-test", "scan", "snooper",
	"frequency-monitor", "monitor-traffic",
	"reset-configuration", "reset-counters",
	"file", "tool",
}

// denyMutationTokens are full-string substrings that, regardless of
// where they appear, indicate a write. We check these AFTER the
// segment scan so callers cannot smuggle "frequency=5180" into a
// crafted "monitor" command.
var denyMutationTokens = []string{
	"frequency=", "channel-width=", "disabled=",
	"name=", "ssid=", "country=",
	"password=", "secret=", "token=",
}

// EnsureCommandAllowed returns ErrDisallowedCommand for any command
// not on the read-only allowlist OR containing any deny segment/
// token. The check is case-insensitive.
func EnsureCommandAllowed(cmd string) error {
	c := strings.TrimSpace(strings.ToLower(cmd))
	if c == "" {
		return ErrDisallowedCommand
	}
	for _, t := range denyMutationTokens {
		if strings.Contains(c, t) {
			return ErrDisallowedCommand
		}
	}
	for _, seg := range strings.Split(c, "/") {
		if seg == "" {
			continue
		}
		// Tokenize on space too — "system reboot" smuggled as one
		// segment is still a write.
		for _, sub := range strings.Fields(seg) {
			for _, deny := range denyMutationSegments {
				if sub == deny {
					return ErrDisallowedCommand
				}
			}
		}
	}
	if _, ok := readOnlyCommands[c]; ok {
		return nil
	}
	return ErrDisallowedCommand
}

// AllowedCommands returns the closed allowlist as a slice for tests
// and documentation. Order is not stable.
func AllowedCommands() []string {
	out := make([]string, 0, len(readOnlyCommands))
	for k := range readOnlyCommands {
		out = append(out, k)
	}
	return out
}

// IsReadOnly is the public accessor IsAllowed semantics for callers
// that don't care about the specific error reason.
func IsReadOnly(cmd string) bool {
	return EnsureCommandAllowed(cmd) == nil
}
