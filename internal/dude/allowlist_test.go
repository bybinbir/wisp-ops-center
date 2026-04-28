package dude

import (
	"errors"
	"strings"
	"testing"
)

func TestEnsureAllowed_AcceptsKnown(t *testing.T) {
	cases := []string{
		"/system/identity/print",
		"/dude/device/print/detail",
		"/ip/neighbor/print/detail",
		"/dude/probe/print/detail",
		"/dude/service/print/detail",
	}
	for _, c := range cases {
		if err := EnsureAllowed(c); err != nil {
			t.Errorf("%q rejected: %v", c, err)
		}
	}
}

func TestEnsureAllowed_RejectsUnknown(t *testing.T) {
	cases := []string{
		"/system/reboot",
		"/interface/wireless/set",
		"/dude/device/remove",
		"",
		"some-random-string",
	}
	for _, c := range cases {
		err := EnsureAllowed(c)
		if !errors.Is(err, ErrDisallowedCommand) {
			t.Errorf("%q should be rejected, got %v", c, err)
		}
	}
}

func TestAllowlist_NoDestructiveCommands(t *testing.T) {
	// Belt-and-suspenders: every allowlist entry must be a path
	// whose terminal segment is a known read-only verb. We split
	// on '/' to avoid false positives like "address" containing
	// "add".
	readOnlyTerminals := map[string]struct{}{
		"print": {}, "detail": {},
	}
	for _, cmd := range AllowedCommands() {
		segs := strings.Split(strings.Trim(cmd, "/"), "/")
		if len(segs) == 0 {
			t.Errorf("empty allowlist entry: %q", cmd)
			continue
		}
		last := strings.ToLower(segs[len(segs)-1])
		if _, ok := readOnlyTerminals[last]; !ok {
			t.Errorf("allowlist entry %q does not end in a read-only verb (got %q)", cmd, last)
		}
	}
}

// TestAllowlist_EnrichmentCommandsAreReadOnly ensures every command
// declared in EnrichmentCommands is on the allowlist AND ends in
// a read-only verb. If someone slips a mutation source in, this
// test fails before it can dial a device.
func TestAllowlist_EnrichmentCommandsAreReadOnly(t *testing.T) {
	if len(EnrichmentCommands) == 0 {
		t.Fatal("EnrichmentCommands must not be empty in Phase 8.1")
	}
	readOnlyTerminals := map[string]struct{}{
		"print": {}, "detail": {},
	}
	for _, cmd := range EnrichmentCommands {
		if err := EnsureAllowed(cmd); err != nil {
			t.Errorf("enrichment cmd %q is NOT on allowlist: %v", cmd, err)
		}
		segs := strings.Split(strings.Trim(cmd, "/"), "/")
		if len(segs) == 0 {
			t.Errorf("malformed enrichment cmd: %q", cmd)
			continue
		}
		last := strings.ToLower(segs[len(segs)-1])
		if _, ok := readOnlyTerminals[last]; !ok {
			t.Errorf("enrichment cmd %q must end in print/detail, got %q", cmd, last)
		}
	}
}

// TestAllowlist_BlocksMutationCommands rejects every common
// destructive verb across system/dude/interface/wireless namespaces.
// This is the primary guard against an enrichment author accidentally
// pasting in a write command.
func TestAllowlist_BlocksMutationCommands(t *testing.T) {
	blocked := []string{
		// system/admin
		"/system/reboot",
		"/system/reset-configuration",
		"/system/shutdown",
		"/system/scheduler/add",
		"/system/scheduler/set",
		"/system/scheduler/remove",
		// dude
		"/dude/device/add",
		"/dude/device/set",
		"/dude/device/remove",
		"/dude/device/enable",
		"/dude/device/disable",
		"/dude/probe/add",
		"/dude/probe/set",
		"/dude/probe/remove",
		"/dude/service/add",
		"/dude/service/remove",
		// wireless
		"/interface/wireless/set",
		"/interface/wireless/enable",
		"/interface/wireless/disable",
		"/interface/wireless/reset-configuration",
		// ip
		"/ip/address/add",
		"/ip/address/remove",
		"/ip/route/add",
		"/ip/route/remove",
		"/ip/firewall/filter/add",
		"/ip/firewall/filter/remove",
	}
	for _, cmd := range blocked {
		if err := EnsureAllowed(cmd); !errors.Is(err, ErrDisallowedCommand) {
			t.Errorf("mutation cmd %q must be blocked, got %v", cmd, err)
		}
	}
}

// TestAllowlist_BlocksBandwidthAndFrequencyApply specifically guards
// the two highest-blast-radius RouterOS operations the platform must
// NEVER perform: bandwidth-test (saturates a link) and
// frequency/channel apply (may take a customer offline).
func TestAllowlist_BlocksBandwidthAndFrequencyApply(t *testing.T) {
	blocked := []string{
		"/tool/bandwidth-test",
		"/tool/bandwidth-server/set",
		"/interface/wireless/set frequency=5180",
		"/interface/wireless/scan",
		"/interface/wireless/snooper",
		"/interface/wireless/registration-table/remove",
	}
	for _, cmd := range blocked {
		if err := EnsureAllowed(cmd); !errors.Is(err, ErrDisallowedCommand) {
			t.Errorf("bandwidth/frequency cmd %q must be blocked, got %v", cmd, err)
		}
	}
}

// TestAllowlist_AllCommandsEndWithPrintDetailOrSafeReadOnlyEquivalent
// is the structural invariant: every allowlist entry must terminate
// in a read-only verb. Today that verb is print or detail. If we ever
// add a new read-only RouterOS verb (e.g. "monitor-once"), this test
// must be updated explicitly — denying drift through unintentional
// new tokens.
func TestAllowlist_AllCommandsEndWithPrintDetailOrSafeReadOnlyEquivalent(t *testing.T) {
	allowedTerminals := map[string]struct{}{
		"print":  {},
		"detail": {},
	}
	for _, cmd := range AllowedCommands() {
		segs := strings.Split(strings.Trim(cmd, "/"), "/")
		last := strings.ToLower(segs[len(segs)-1])
		if _, ok := allowedTerminals[last]; !ok {
			t.Errorf("allowlist entry %q ends in %q which is not a recognized read-only verb", cmd, last)
		}
		// Reject any segment that contains a known mutation token.
		// We tokenize on '/' so 'address' won't match 'add'.
		mutationSegs := map[string]struct{}{
			"set": {}, "add": {}, "remove": {}, "enable": {}, "disable": {},
			"reset": {}, "reboot": {}, "shutdown": {},
			"reset-configuration": {}, "bandwidth-test": {},
		}
		for _, s := range segs {
			if _, hit := mutationSegs[strings.ToLower(s)]; hit {
				t.Errorf("allowlist entry %q contains mutation token %q", cmd, s)
			}
		}
	}
}
