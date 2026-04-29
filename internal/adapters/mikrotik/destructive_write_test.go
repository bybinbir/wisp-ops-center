package mikrotik

import (
	"errors"
	"testing"
)

// TestPhase10E_WriteAllowlist_Path asserts the allowlist contains
// exactly /interface/wireless/set on Phase 10E. Drift here is a
// deliberate scope expansion and MUST land with security review.
func TestPhase10E_WriteAllowlist_Path(t *testing.T) {
	if len(WriteAllowlistedCommands) != 1 {
		t.Fatalf("WriteAllowlistedCommands grew unexpectedly: %v", WriteAllowlistedCommands)
	}
	if WriteAllowlistedCommands[0] != "/interface/wireless/set" {
		t.Errorf("path drifted: %q", WriteAllowlistedCommands[0])
	}
}

// TestPhase10E_WriteAllowed_HappyPath exercises the typical Phase
// 10E call shape: set frequency on a numbered wireless interface.
func TestPhase10E_WriteAllowed_HappyPath(t *testing.T) {
	if !IsWriteAllowed("/interface/wireless/set", map[string]string{
		"number":    "wlan1",
		"frequency": "5180",
	}) {
		t.Fatal("happy-path Phase 10E call should be allowed")
	}
	if err := EnsureWriteAllowed("/interface/wireless/set", map[string]string{
		"number":    "wlan1",
		"frequency": "5180",
	}); err != nil {
		t.Fatalf("EnsureWriteAllowed returned %v, want nil", err)
	}
}

// TestPhase10E_WriteAllowed_RejectsForbiddenSegments mirrors the
// read-only allowlist's ForbiddenSegments. Even a path that LOOKS
// like a write (set/add/remove/...) is rejected here unless it is
// the explicit /interface/wireless/set path.
func TestPhase10E_WriteAllowed_RejectsForbiddenSegments(t *testing.T) {
	cases := []string{
		"/interface/wireless/add",
		"/interface/wireless/remove",
		"/interface/wireless/enable",
		"/interface/wireless/disable",
		"/interface/wireless/reset",
		"/system/reboot",
		"/system/shutdown",
		"/file/print",
		"/tool/bandwidth-test",
		"/ip/address/add",
	}
	for _, c := range cases {
		if IsWriteAllowed(c, map[string]string{"number": "wlan1", "frequency": "5180"}) {
			t.Errorf("path %q should be rejected", c)
		}
	}
}

// TestPhase10E_WriteAllowed_RejectsMissingRequiredArg locks in the
// "every required identifier MUST be present" rule. A path-only
// /interface/wireless/set (or one missing 'number' or 'frequency')
// is rejected before any device dial.
func TestPhase10E_WriteAllowed_RejectsMissingRequiredArg(t *testing.T) {
	cases := []map[string]string{
		nil,
		{},
		{"number": "wlan1"},                  // missing frequency
		{"frequency": "5180"},                // missing number
		{"number": "wlan1", "frequency": ""}, // empty value
		{"number": "", "frequency": "5180"},  // empty value
		{"number": "wlan1", "frequency": "   "},
	}
	for _, args := range cases {
		if IsWriteAllowed("/interface/wireless/set", args) {
			t.Errorf("args %v should be rejected", args)
		}
	}
}

// TestPhase10E_WriteAllowed_RejectsExtraArg locks in the arg-key
// allowlist. A second mutation (SSID change, passphrase, mode flip)
// MUST NOT smuggle through the same path because both required
// args are also present.
func TestPhase10E_WriteAllowed_RejectsExtraArg(t *testing.T) {
	cases := []map[string]string{
		{"number": "wlan1", "frequency": "5180", "ssid": "new-ssid"},
		{"number": "wlan1", "frequency": "5180", "passphrase": "secret"},
		{"number": "wlan1", "frequency": "5180", "mode": "ap-bridge"},
		{"number": "wlan1", "frequency": "5180", "country": "turkey"},
	}
	for _, args := range cases {
		if IsWriteAllowed("/interface/wireless/set", args) {
			t.Errorf("args %v carried an extra mutation key, must be rejected", args)
		}
	}
}

// TestPhase10E_WriteAllowed_CaseInsensitive locks in path/arg key
// normalization. Operator typos ("/INTERFACE/Wireless/SET",
// "Frequency") MUST NOT bypass the allowlist by case mismatch.
func TestPhase10E_WriteAllowed_CaseInsensitive(t *testing.T) {
	if !IsWriteAllowed("/INTERFACE/Wireless/SET", map[string]string{
		"NUMBER":    "wlan1",
		"frequency": "5180",
	}) {
		t.Error("uppercased path/key should still be allowed")
	}
}

// TestPhase10E_EnsureWriteAllowed_Sentinel asserts the rejection is
// always wrapped in ErrDisallowedWrite, so callers can branch on a
// stable sentinel rather than parse messages.
func TestPhase10E_EnsureWriteAllowed_Sentinel(t *testing.T) {
	err := EnsureWriteAllowed("/interface/wireless/scan", map[string]string{
		"number":    "wlan1",
		"frequency": "5180",
	})
	if !errors.Is(err, ErrDisallowedWrite) {
		t.Errorf("err = %v, want errors.Is(ErrDisallowedWrite)", err)
	}
}

// TestPhase10E_FormatWriteCmd_Deterministic locks the rendered CLI
// form. Sorted args mean the audit metadata + test snapshots are
// stable across Go map iteration order.
func TestPhase10E_FormatWriteCmd_Deterministic(t *testing.T) {
	got := FormatWriteCmd("/interface/wireless/set", map[string]string{
		"frequency": "5180",
		"number":    "wlan1",
	})
	want := "interface wireless set frequency=5180 number=wlan1"
	if got != want {
		t.Errorf("FormatWriteCmd = %q, want %q", got, want)
	}
}

// TestPhase10E_FormatWriteCmd_NoArgs covers the (defensive) empty
// args path. The function MUST still return the CLI prefix.
func TestPhase10E_FormatWriteCmd_NoArgs(t *testing.T) {
	got := FormatWriteCmd("/interface/wireless/set", nil)
	if got != "interface wireless set" {
		t.Errorf("FormatWriteCmd(nil) = %q, want %q", got, "interface wireless set")
	}
}

// TestPhase10E_WriteAllowed_TrimsWhitespace asserts the path is
// trimmed before the allowlist check; a leading/trailing space
// MUST NOT defeat the comparison.
func TestPhase10E_WriteAllowed_TrimsWhitespace(t *testing.T) {
	if !IsWriteAllowed("  /interface/wireless/set  ", map[string]string{
		"number":    "wlan1",
		"frequency": "5180",
	}) {
		t.Error("whitespace-padded path should be trimmed and allowed")
	}
}
