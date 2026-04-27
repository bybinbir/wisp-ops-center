package mikrotik

import "strings"

// AllowlistedCommands enumerates the read-only RouterOS API paths we are
// permitted to invoke. Anything outside this set is rejected at the adapter
// boundary, regardless of how it was constructed.
var AllowlistedCommands = []string{
	"/system/identity/print",
	"/system/resource/print",
	"/system/routerboard/print",
	"/interface/print",
	"/interface/wireless/print",
	"/interface/wireless/registration-table/print",
	"/interface/wifi/print",
	"/interface/wifi/registration-table/print",
	"/interface/wifiwave2/print",
	"/interface/wifiwave2/registration-table/print",
	"/ip/address/print",
}

// ForbiddenSegments are RouterOS command verbs/sub-paths that must NEVER
// appear as a path segment, no matter what the prefix is. We compare
// segment-by-segment so a benign "/ip/address/print" is not rejected just
// because "address" begins with "add".
var ForbiddenSegments = []string{
	"set", "add", "remove", "enable", "disable",
	"scan", "frequency-monitor",
	"bandwidth-test",
	"reset", "reboot", "shutdown", "upgrade",
	"import", "export",
	"file", "tool",
}

// IsAllowed returns true when cmd exactly matches an allowlisted RouterOS
// path. The check is case-insensitive. Any forbidden segment vetoes the
// command even if it would otherwise look allowlisted.
func IsAllowed(cmd string) bool {
	cmd = strings.TrimSpace(strings.ToLower(cmd))
	if cmd == "" {
		return false
	}
	for _, seg := range strings.Split(cmd, "/") {
		if seg == "" {
			continue
		}
		for _, forbidden := range ForbiddenSegments {
			if seg == forbidden {
				return false
			}
		}
	}
	for _, ok := range AllowlistedCommands {
		if cmd == ok {
			return true
		}
	}
	return false
}

// EnsureAllowed returns ErrDisallowedCommand for any non-allowlisted path.
func EnsureAllowed(cmd string) error {
	if IsAllowed(cmd) {
		return nil
	}
	return ErrDisallowedCommand
}
