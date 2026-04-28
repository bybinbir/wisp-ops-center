package networkactions

import (
	"net"
	"strings"
	"unicode"
)

// ValidateTargetHost returns ErrInvalidTargetHost when host is not a
// syntactically valid IPv4, IPv6 or hostname literal. Phase 9 v3
// runs this BEFORE the database inet cast and BEFORE any SSH dial,
// so a malformed payload (e.g. a smuggled mutation token like
// "frequency=5180") results in a clean HTTP 400, never a 500 from
// the DB layer.
//
// Rules:
//   - Empty / whitespace-only         → invalid.
//   - Contains '=' / ' ' / ',' / ';'  → invalid (defends against
//     smuggled mutation payloads).
//   - net.ParseIP                      → accept.
//   - hostname (RFC 1123-ish):
//   - 1-253 chars, letters/digits/'.'/'-' only,
//   - each label 1-63 chars, no leading/trailing '-',
//   - cannot be a single ':' or '/'.
//
// Notes:
//   - We deliberately do NOT do DNS resolution here. The action
//     runner gets to discover unreachability through SSH dial; the
//     point of this validator is structural input safety only.
//   - Length cap (253) is the DNS limit per RFC 1035.
func ValidateTargetHost(host string) error {
	h := strings.TrimSpace(host)
	if h == "" {
		return ErrInvalidTargetHost
	}
	if len(h) > 253 {
		return ErrInvalidTargetHost
	}
	// Smuggled-token guard. None of these byte classes have any
	// business in a hostname/IP; rejecting them early means the
	// rest of the validator can stay simple.
	for _, b := range []byte{'=', ' ', '\t', ',', ';', '\n', '\r', '"', '\'', '?', '#', '\\'} {
		if strings.IndexByte(h, b) >= 0 {
			return ErrInvalidTargetHost
		}
	}
	// Fast path: literal IPv4/IPv6.
	if ip := net.ParseIP(h); ip != nil {
		return nil
	}
	// Strip optional :port.
	hostOnly, port, hasPort := splitHostPort(h)
	if hasPort {
		if !isShortDigits(port, 5) {
			return ErrInvalidTargetHost
		}
		// re-check IP literal after port strip.
		if ip := net.ParseIP(hostOnly); ip != nil {
			return nil
		}
		h = hostOnly
	}
	// Hostname rules (RFC 1123-ish).
	if h == "" || h[0] == '-' || h[0] == '.' || h[len(h)-1] == '-' || h[len(h)-1] == '.' {
		return ErrInvalidTargetHost
	}
	for _, label := range strings.Split(h, ".") {
		if len(label) == 0 || len(label) > 63 {
			return ErrInvalidTargetHost
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return ErrInvalidTargetHost
		}
		for _, r := range label {
			if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-') {
				return ErrInvalidTargetHost
			}
		}
	}
	return nil
}

// splitHostPort returns (host, port, true) when h ends in :NUM that
// looks like a port. Bracketed IPv6 forms ([::1]:22) are also
// supported. Returns (h, "", false) when no port suffix is found.
func splitHostPort(h string) (string, string, bool) {
	if strings.HasPrefix(h, "[") {
		end := strings.Index(h, "]")
		if end < 0 {
			return h, "", false
		}
		body := h[1:end]
		rest := h[end+1:]
		if strings.HasPrefix(rest, ":") {
			return body, rest[1:], true
		}
		return body, "", false
	}
	colons := strings.Count(h, ":")
	if colons == 0 {
		return h, "", false
	}
	if colons > 1 {
		// Probably a bare IPv6 literal (more than 1 colon, no brackets);
		// don't try to extract a port.
		return h, "", false
	}
	idx := strings.LastIndexByte(h, ':')
	return h[:idx], h[idx+1:], true
}

func isShortDigits(s string, max int) bool {
	if s == "" || len(s) > max {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
