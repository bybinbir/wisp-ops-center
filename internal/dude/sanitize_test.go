package dude

import "testing"

func TestSanitizeAttrs_RedactsSecretLikeKeys(t *testing.T) {
	in := map[string]string{
		"password":       "topsecret",
		"snmp-community": "public-but-secret",
		"name":           "AP1",
		"auth-token":     "abc123",
		"address":        "10.0.0.1",
	}
	out := SanitizeAttrs(in)
	for k, want := range map[string]string{
		"password":       "[redacted]",
		"snmp-community": "[redacted]",
		"auth-token":     "[redacted]",
		"name":           "AP1",
		"address":        "10.0.0.1",
	} {
		if out[k] != want {
			t.Errorf("key=%s got=%q want=%q", k, out[k], want)
		}
	}
	// Original must not be mutated.
	if in["password"] != "topsecret" {
		t.Errorf("input was mutated")
	}
}

func TestSanitizeAttrs_NilSafe(t *testing.T) {
	if SanitizeAttrs(nil) != nil {
		t.Errorf("nil input should return nil")
	}
}

func TestSanitizeMessage_StripsPassword(t *testing.T) {
	in := "ssh: connect failed user=bariss password=hunter2 host=10.0.0.1"
	got := SanitizeMessage(in)
	if got == in {
		t.Errorf("message not redacted: %q", got)
	}
	if got != "ssh: connect failed user=bariss [redacted]" {
		t.Errorf("unexpected sanitized output: %q", got)
	}
}

func TestSanitizeMessage_LengthCap(t *testing.T) {
	long := make([]byte, 500)
	for i := range long {
		long[i] = 'x'
	}
	got := SanitizeMessage(string(long))
	if len(got) > 325 {
		t.Errorf("expected length cap, got %d", len(got))
	}
}
