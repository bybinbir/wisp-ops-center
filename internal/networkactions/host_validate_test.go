package networkactions

import (
	"errors"
	"strings"
	"testing"
)

// TestValidateTargetHost_AcceptsLiterals walks the basic happy
// cases: IPv4, IPv6 (bracketed and bare), hostname.
func TestValidateTargetHost_AcceptsLiterals(t *testing.T) {
	cases := []string{
		"10.0.0.1",
		"194.15.45.62",
		"127.0.0.1:8080",
		"::1",
		"[::1]:22",
		"fe80::1234",
		"router.example.com",
		"ap-sahil-1.local",
		"a", // single label, allowed
		"a-b.c-d.example",
	}
	for _, c := range cases {
		if err := ValidateTargetHost(c); err != nil {
			t.Errorf("%q rejected: %v", c, err)
		}
	}
}

// TestValidateTargetHost_RejectsSmuggledPayloads is the structural
// guard against operators (or bugs) handing the action runner a
// mutation-style payload as target_host. Phase 9 v3's whole point
// is that this MUST 400 instead of going to the DB inet cast.
func TestValidateTargetHost_RejectsSmuggledPayloads(t *testing.T) {
	cases := []string{
		"frequency=5180",
		"channel-width=20mhz",
		"disabled=no",
		"name=AP-1",
		"10.0.0.1; rm -rf /",
		"10.0.0.1 && id",
		"\nhttp://evil/",
		"router.example.com?q=1",
		"router.example.com#frag",
		`"router"`,
		"'router'",
		`router\bad`,
	}
	for _, c := range cases {
		if err := ValidateTargetHost(c); !errors.Is(err, ErrInvalidTargetHost) {
			t.Errorf("smuggled %q must be invalid_target_host, got %v", c, err)
		}
	}
}

// TestValidateTargetHost_RejectsEmpty makes sure empty/whitespace
// cannot pass.
func TestValidateTargetHost_RejectsEmpty(t *testing.T) {
	for _, c := range []string{"", " ", "\t", "  ", " \r\n "} {
		if err := ValidateTargetHost(c); !errors.Is(err, ErrInvalidTargetHost) {
			t.Errorf("empty/whitespace %q must be rejected, got %v", c, err)
		}
	}
}

// TestValidateTargetHost_RejectsBadHostnameShape covers RFC 1123
// edge cases.
func TestValidateTargetHost_RejectsBadHostnameShape(t *testing.T) {
	cases := []string{
		"-leading-dash",
		"trailing-dash-",
		".leading.dot",
		"trailing.dot.",
		"contains_underscore",
		"two..dots.example",
		strings.Repeat("a", 64) + ".example", // label > 63
		strings.Repeat("a", 254),             // total > 253
	}
	for _, c := range cases {
		if err := ValidateTargetHost(c); !errors.Is(err, ErrInvalidTargetHost) {
			t.Errorf("malformed %q must be rejected, got %v", c, err)
		}
	}
}

// TestValidateTargetHost_PortGuard checks that bogus port suffixes
// are rejected without breaking the IPv4 path.
func TestValidateTargetHost_PortGuard(t *testing.T) {
	cases := map[string]bool{
		"10.0.0.1:22":     true,
		"10.0.0.1:65535":  true,
		"10.0.0.1:abc":    false,
		"10.0.0.1:":       false,
		"10.0.0.1:123456": false, // too many digits
	}
	for in, ok := range cases {
		err := ValidateTargetHost(in)
		if ok && err != nil {
			t.Errorf("%q should pass, got %v", in, err)
		}
		if !ok && !errors.Is(err, ErrInvalidTargetHost) {
			t.Errorf("%q should be rejected, got %v", in, err)
		}
	}
}
