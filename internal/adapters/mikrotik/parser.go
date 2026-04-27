package mikrotik

import (
	"strconv"
	"strings"
)

// parseInt64 parses a RouterOS counter value tolerantly. Empty or invalid
// strings return 0; we never panic on telemetry text.
func parseInt64(s string) int64 {
	if s == "" {
		return 0
	}
	s = strings.ReplaceAll(s, " ", "")
	if v, err := strconv.ParseInt(s, 10, 64); err == nil {
		return v
	}
	return 0
}

// parseInt is a shorthand for typical interface MTU/channel width fields.
func parseInt(s string) int { return int(parseInt64(s)) }

// parseFloatPtr returns nil for empty input. A non-empty unparseable value
// also returns nil so we never store junk in the wireless metric table.
func parseFloatPtr(s string) *float64 {
	if s == "" {
		return nil
	}
	s = strings.TrimSuffix(s, "dBm")
	s = strings.TrimSuffix(s, "dB")
	s = strings.ReplaceAll(s, " ", "")
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		return &v
	}
	return nil
}

// parseIntPtr is the integer counterpart of parseFloatPtr.
func parseIntPtr(s string) *int {
	if s == "" {
		return nil
	}
	if v, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
		return &v
	}
	return nil
}

// parseBool maps RouterOS "true"/"false"/yes/no into Go booleans.
func parseBool(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "yes", "1", "running":
		return true
	}
	return false
}

// parseUptime parses RouterOS uptime strings like "1w2d3h4m5s" into seconds.
//
// We deliberately only accept the documented MikroTik suffixes; weeks,
// days, hours, minutes, seconds.
func parseUptime(s string) int64 {
	if s == "" {
		return 0
	}
	var total int64
	var num strings.Builder
	for _, ch := range s {
		switch {
		case ch >= '0' && ch <= '9':
			num.WriteRune(ch)
		default:
			if num.Len() == 0 {
				continue
			}
			n, _ := strconv.ParseInt(num.String(), 10, 64)
			num.Reset()
			switch ch {
			case 'w', 'W':
				total += n * 7 * 24 * 3600
			case 'd', 'D':
				total += n * 24 * 3600
			case 'h', 'H':
				total += n * 3600
			case 'm', 'M':
				total += n * 60
			case 's', 'S':
				total += n
			}
		}
	}
	return total
}

// normalizeMAC trims and uppercases a MAC address. RouterOS sometimes
// reports lowercase with colons; we standardize for downstream joins.
func normalizeMAC(mac string) string {
	return strings.ToUpper(strings.TrimSpace(mac))
}
