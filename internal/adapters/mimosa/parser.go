package mimosa

import (
	"strconv"
	"strings"
)

// extractModelFromSysDescr tries to identify a Mimosa model name from
// the SNMP sysDescr string. Mimosa firmware historically reports
// strings like "Mimosa B5c v2.5.4" or "Mimosa A5-360 1.5.6". We are
// conservative: if no recognizable token is present, return "".
func extractModelFromSysDescr(descr string) string {
	if descr == "" {
		return ""
	}
	parts := strings.Fields(descr)
	if len(parts) == 0 {
		return ""
	}
	// Prefer the second token if the first is "Mimosa".
	if strings.EqualFold(parts[0], "mimosa") && len(parts) > 1 {
		return parts[1]
	}
	// Fall back to the first non-empty token; saves something
	// recognizable for non-Mimosa SNMP devices that share the
	// adapter (e.g. a third-party SNMP router).
	return parts[0]
}

// extractFirmwareFromSysDescr returns the trailing version-like token
// in sysDescr ("v2.5.4", "1.5.6", "RouterOS 7.13"). If none, "".
func extractFirmwareFromSysDescr(descr string) string {
	tokens := strings.Fields(descr)
	for i := len(tokens) - 1; i >= 0; i-- {
		t := strings.TrimPrefix(tokens[i], "v")
		if isVersionLike(t) {
			return tokens[i]
		}
	}
	return ""
}

func isVersionLike(s string) bool {
	if s == "" {
		return false
	}
	dots := 0
	for _, ch := range s {
		switch {
		case ch >= '0' && ch <= '9':
			// ok
		case ch == '.':
			dots++
		default:
			return false
		}
	}
	return dots >= 1
}

// timeTicksToSeconds converts SNMP TimeTicks (1/100 sec) to seconds.
func timeTicksToSeconds(ticks uint32) int64 { return int64(ticks) / 100 }

// parseInt64 — defensive integer reader used for IF-MIB counters.
func parseInt64(s string) int64 {
	if s == "" {
		return 0
	}
	if v, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err == nil {
		return v
	}
	return 0
}

// SNMP ifAdminStatus / ifOperStatus enums per RFC 2233:
// up(1), down(2), testing(3), unknown(4), dormant(5),
// notPresent(6), lowerLayerDown(7).
func ifStatusUp(v int) bool { return v == 1 }
