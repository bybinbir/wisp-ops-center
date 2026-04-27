package dude

import "strings"

// allowed holds the exact set of RouterOS CLI commands the Dude
// adapter is permitted to run. EVERY command must be read-only.
var allowed = map[string]struct{}{
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
