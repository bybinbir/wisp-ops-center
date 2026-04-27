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
