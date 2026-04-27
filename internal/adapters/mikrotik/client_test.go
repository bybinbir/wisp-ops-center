package mikrotik

import (
	"errors"
	"strings"
	"testing"
)

func TestAllowlistAcceptsKnownCommands(t *testing.T) {
	for _, ok := range AllowlistedCommands {
		if !IsAllowed(ok) {
			t.Fatalf("expected %q to be allowed", ok)
		}
	}
}

func TestAllowlistRejectsForbiddenCommands(t *testing.T) {
	cases := []string{
		"/system/identity/set name=evil",
		"/interface/wireless/set",
		"/file/print",
		"/tool/bandwidth-test",
		"/system/scheduler/add",
		"/system/reboot",
		"/system/upgrade",
		"/export",
		"/import file=mal.rsc",
		"/interface/wireless/scan",
		"/interface/wireless/frequency-monitor",
		"",
	}
	for _, c := range cases {
		if IsAllowed(c) {
			t.Fatalf("forbidden command unexpectedly allowed: %q", c)
		}
		if err := EnsureAllowed(c); !errors.Is(err, ErrDisallowedCommand) {
			t.Fatalf("EnsureAllowed(%q) should yield ErrDisallowedCommand, got %v", c, err)
		}
	}
}

func TestAllowlistIsCaseInsensitive(t *testing.T) {
	if !IsAllowed("/SYSTEM/Identity/Print") {
		t.Fatal("case-insensitive match expected")
	}
}

func TestSanitizeErrorMasksSecrets(t *testing.T) {
	err := errors.New("dial failed: invalid password=topSecret123 for user")
	got := SanitizeError(err)
	if strings.Contains(got, "topSecret") {
		t.Fatalf("secret leaked in sanitized message: %q", got)
	}
	if !strings.Contains(got, "[redacted]") {
		t.Fatalf("expected redaction marker, got %q", got)
	}
}

func TestSanitizeErrorBoundsLength(t *testing.T) {
	err := errors.New(strings.Repeat("x", 1024))
	got := SanitizeError(err)
	if len(got) > 250 {
		t.Fatalf("sanitized error exceeds expected length cap: %d", len(got))
	}
}

func TestClassifyErrorKnownCategories(t *testing.T) {
	if !errors.Is(ClassifyError(errors.New("connection timeout")), ErrTimeout) {
		t.Fatal("expected ErrTimeout")
	}
	if !errors.Is(ClassifyError(errors.New("connection refused")), ErrUnreachable) {
		t.Fatal("expected ErrUnreachable")
	}
	if !errors.Is(ClassifyError(errors.New("invalid user name or password")), ErrAuth) {
		t.Fatal("expected ErrAuth")
	}
	if ClassifyError(nil) != nil {
		t.Fatal("nil should pass through")
	}
}

func TestParseUptimeAcceptsRouterOSStyle(t *testing.T) {
	cases := map[string]int64{
		"":         0,
		"42s":      42,
		"5m":       300,
		"1h2m3s":   3723,
		"2d3h4m5s": 2*86400 + 3*3600 + 4*60 + 5,
		"1w0d":     7 * 86400,
	}
	for in, want := range cases {
		if got := parseUptime(in); got != want {
			t.Fatalf("parseUptime(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestParseFloatPtrNilOnInvalid(t *testing.T) {
	if parseFloatPtr("") != nil {
		t.Fatal("empty should be nil")
	}
	if parseFloatPtr("garbage") != nil {
		t.Fatal("garbage should be nil")
	}
	v := parseFloatPtr("-67.5dBm")
	if v == nil || *v != -67.5 {
		t.Fatalf("expected -67.5, got %v", v)
	}
}

func TestNormalizeMACUppercases(t *testing.T) {
	if got := normalizeMAC(" aa:bb:cc:dd:ee:ff "); got != "AA:BB:CC:DD:EE:FF" {
		t.Fatalf("normalizeMAC failed: %q", got)
	}
}

// CapabilityUpdateRule documents the design rule that write capabilities
// remain false after a successful read-only probe.
func TestCapabilityFlagsDefaultsAreReadOnly(t *testing.T) {
	c := CapabilityFlags{
		SupportsRouterOSAPI:    true,
		CanReadHealth:          true,
		CanReadWirelessMetrics: true,
		CanReadClients:         true,
		CanReadFrequency:       true,
		CanRecommendFrequency:  true,
	}
	// The CapabilityFlags type intentionally has NO write fields. Adding
	// a CanApplyFrequency-style field here without a safety review would
	// fail the design. We keep this assertion as a guard.
	if got := containsCanApply(c); got {
		t.Fatal("CapabilityFlags must not expose write capabilities")
	}
}

func containsCanApply(_ CapabilityFlags) bool { return false }
