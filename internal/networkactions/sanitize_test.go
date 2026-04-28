package networkactions

import (
	"strings"
	"testing"
)

func TestSanitizeAttrs_RedactsSecretLikeKeys(t *testing.T) {
	in := map[string]string{
		"name":      "ap-1",
		"password":  "topsecret",
		"PASSWD":    "topsecret",
		"secret":    "shh",
		"token":     "ya29.abc",
		"community": "public-but-still",
		"frequency": "5180",
	}
	out := SanitizeAttrs(in)
	for k, v := range out {
		lk := strings.ToLower(k)
		switch {
		case strings.Contains(lk, "password"),
			strings.Contains(lk, "passwd"),
			strings.Contains(lk, "secret"),
			strings.Contains(lk, "token"),
			strings.Contains(lk, "community"):
			if v != "[redacted]" {
				t.Errorf("key %q value not redacted: %q", k, v)
			}
		}
	}
	if out["name"] != "ap-1" || out["frequency"] != "5180" {
		t.Errorf("non-secret values must round-trip: %+v", out)
	}
}

func TestSanitizeMessage_StripsSecretPrefixes(t *testing.T) {
	cases := []struct{ in, must string }{
		{"login failed: password=hunter2 wrong account", "[redacted]"},
		{"token=abc123 expired", "[redacted]"},
	}
	for _, c := range cases {
		got := SanitizeMessage(c.in)
		if !strings.Contains(got, c.must) {
			t.Errorf("SanitizeMessage(%q) = %q, want contains %q", c.in, got, c.must)
		}
		// Ensure literal secret material is gone from the trimmed output.
		if strings.Contains(got, "hunter2") || strings.Contains(got, "abc123") {
			t.Errorf("secret leaked through SanitizeMessage: %q", got)
		}
	}
}

// TestSanitizeResultMap_NestedRedaction proves nested maps + slices
// are walked.
func TestSanitizeResultMap_NestedRedaction(t *testing.T) {
	in := map[string]any{
		"identity": "rtr-1",
		"ssh": map[string]any{
			"username": "admin",
			"password": "supersecret",
		},
		"clients": []any{
			map[string]any{"mac": "AA:11", "auth": "leaked"},
			map[string]any{"mac": "BB:22"},
		},
	}
	out := SanitizeResultMap(in)
	ssh := out["ssh"].(map[string]any)
	if ssh["password"] != "[redacted]" {
		t.Errorf("nested password not redacted: %+v", ssh)
	}
	clients := out["clients"].([]any)
	c0 := clients[0].(map[string]any)
	if c0["auth"] != "[redacted]" {
		t.Errorf("auth field in slice not redacted: %+v", c0)
	}
	if out["identity"] != "rtr-1" {
		t.Errorf("non-secret string lost: %+v", out)
	}
}
