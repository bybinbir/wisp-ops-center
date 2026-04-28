package networkactions

import (
	"errors"
	"strings"
	"testing"
)

func TestEnsureCommandAllowed_AcceptsKnownReadOnly(t *testing.T) {
	cases := []string{
		"/system/identity/print",
		"/interface/wireless/print/detail",
		"/interface/wireless/registration-table/print/detail",
		"/interface/wifi/print/detail",
		"/interface/wifi/registration-table/print/detail",
		"/interface/wifiwave2/print/detail",
		"/interface/wifiwave2/registration-table/print/detail",
	}
	for _, c := range cases {
		if err := EnsureCommandAllowed(c); err != nil {
			t.Errorf("%q rejected: %v", c, err)
		}
	}
}

// TestEnsureCommandAllowed_BlocksMutationCommands proves the runner
// refuses every common destructive verb in every namespace.
func TestEnsureCommandAllowed_BlocksMutationCommands(t *testing.T) {
	blocked := []string{
		"/interface/wireless/set",
		"/interface/wireless/set frequency=5180",
		"/interface/wireless/disable",
		"/interface/wireless/enable",
		"/interface/wifi/set",
		"/interface/wifiwave2/set",
		"/system/reboot",
		"/system/shutdown",
		"/system/reset-configuration",
		"/file/print",
		"/tool/bandwidth-test",
		"/tool/scan",
		"/ip/firewall/filter/add",
		"/ip/route/add",
		"/system/scheduler/add",
	}
	for _, c := range blocked {
		err := EnsureCommandAllowed(c)
		if !errors.Is(err, ErrDisallowedCommand) {
			t.Errorf("mutation cmd %q must be blocked, got %v", c, err)
		}
	}
}

// TestEnsureCommandAllowed_BlocksFrequencyApply specifically guards
// the highest-blast-radius RouterOS write the platform must NEVER
// perform: setting a frequency or channel-width or "disabled" flag.
func TestEnsureCommandAllowed_BlocksFrequencyApply(t *testing.T) {
	blocked := []string{
		"/interface/wireless/set frequency=5180",
		"/interface/wireless/set channel-width=20mhz",
		"/interface/wireless/set disabled=no",
		"/interface/wifi/set frequency=5180",
		"/interface/wifiwave2/set channel-width=40mhz",
		"/interface/wireless/scan",
		"/interface/wireless/snooper",
		"/interface/wireless/registration-table/remove",
	}
	for _, c := range blocked {
		err := EnsureCommandAllowed(c)
		if !errors.Is(err, ErrDisallowedCommand) {
			t.Errorf("frequency apply %q must be blocked, got %v", c, err)
		}
	}
}

// TestAllowlist_AllReadOnlyEndingsAreSafe walks the public allowlist
// and proves every entry terminates in print or detail. If a future
// edit slips a non-read terminal in, this fails immediately.
func TestAllowlist_AllReadOnlyEndingsAreSafe(t *testing.T) {
	safe := map[string]struct{}{"print": {}, "detail": {}}
	for _, c := range AllowedCommands() {
		segs := strings.Split(strings.Trim(c, "/"), "/")
		last := strings.ToLower(segs[len(segs)-1])
		if _, ok := safe[last]; !ok {
			t.Errorf("allowlist entry %q does not end in print/detail (got %q)", c, last)
		}
		// Reject any segment that contains a mutation token.
		for _, s := range segs {
			for _, deny := range denyMutationSegments {
				if s == deny {
					t.Errorf("allowlist entry %q contains denied segment %q", c, s)
				}
			}
		}
	}
}

// TestEnsureCommandAllowed_RejectsEmpty makes sure the empty-string
// case can never sneak past the allowlist.
func TestEnsureCommandAllowed_RejectsEmpty(t *testing.T) {
	for _, c := range []string{"", " ", "\t", "//"} {
		if err := EnsureCommandAllowed(c); !errors.Is(err, ErrDisallowedCommand) {
			t.Errorf("empty/garbage %q must be blocked, got %v", c, err)
		}
	}
}
