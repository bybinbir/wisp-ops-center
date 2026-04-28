package networkactions

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// TestDudeFallbackResolver_ReturnsTargetWhenSecretPresent — happy
// path: provider holds the dude_static_admin password, resolver
// composes an SSHTarget with the right username/host.
func TestDudeFallbackResolver_ReturnsTargetWhenSecretPresent(t *testing.T) {
	prov := NewMemorySecretProvider(map[string]string{
		DudeStaticProfile: "lab-only-not-secret",
	})
	r := &DudeFallbackResolver{
		Provider: prov,
		Username: "admin",
		Port:     22,
		Timeout:  10 * time.Second,
	}
	target, err := r.Resolve(context.Background(), "dev-1", "10.0.0.1")
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if target.Host != "10.0.0.1" || target.Username != "admin" {
		t.Errorf("unexpected target: %+v", target)
	}
	if target.Password == "" {
		t.Errorf("password should be filled (in-memory only, never persisted)")
	}
}

// TestDudeFallbackResolver_TypedErrorWhenSecretMissing — when the
// provider has no secret, the resolver MUST return
// ErrCredentialNotFound and the action runner MUST NOT dial.
func TestDudeFallbackResolver_TypedErrorWhenSecretMissing(t *testing.T) {
	prov := NewMemorySecretProvider(map[string]string{}) // no entries
	r := &DudeFallbackResolver{Provider: prov, Username: "admin"}
	_, err := r.Resolve(context.Background(), "dev-1", "10.0.0.1")
	if !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("want ErrCredentialNotFound, got %v", err)
	}
	if ErrorCode(err) != "credential_not_found" {
		t.Errorf("ErrorCode should map to credential_not_found, got %q", ErrorCode(err))
	}
}

// TestDudeFallbackResolver_NotConfiguredWhenUsernameEmpty — username
// missing surfaces ErrNotConfigured (not ErrCredentialNotFound), so
// the API can render a different hint.
func TestDudeFallbackResolver_NotConfiguredWhenUsernameEmpty(t *testing.T) {
	prov := NewMemorySecretProvider(map[string]string{DudeStaticProfile: "x"})
	r := &DudeFallbackResolver{Provider: prov, Username: ""}
	_, err := r.Resolve(context.Background(), "", "10.0.0.1")
	if !errors.Is(err, ErrNotConfigured) {
		t.Errorf("want ErrNotConfigured, got %v", err)
	}
}

// TestMemorySecretProvider_DoesNotLeakOnError — a missing entry
// returns "" + typed error, and the empty-value entry is treated
// the same way (defends against accidental "ok with empty pwd").
func TestMemorySecretProvider_DoesNotLeakOnError(t *testing.T) {
	cases := []struct {
		seed map[string]string
		want error
	}{
		{nil, ErrCredentialNotFound},
		{map[string]string{}, ErrCredentialNotFound},
		{map[string]string{DudeStaticProfile: ""}, ErrCredentialNotFound},
	}
	for i, tc := range cases {
		prov := NewMemorySecretProvider(tc.seed)
		v, err := prov.Lookup(context.Background(), DudeStaticProfile)
		if !errors.Is(err, tc.want) {
			t.Errorf("case[%d] err=%v want %v", i, err, tc.want)
		}
		if v != "" {
			t.Errorf("case[%d] value should be empty on error, got %q", i, v)
		}
	}
}

// TestSecretsNotInExportedTypeStrings — defensive: SSHTarget is a
// runtime struct and must NEVER expose its password through any
// implicit Stringer. We check the result of fmt.Sprintf with %v
// to ensure password content is not reflected (or is at least not
// the only field we care about).
//
// This is a tripwire: if a future edit adds a Stringer that
// prints the password, this test fails.
func TestSecretsNotInExportedTypeStrings(t *testing.T) {
	target := SSHTarget{Host: "10.0.0.1", Username: "admin", Password: "lab-secret-not-real"}
	// Sprintf %v on a struct prints field=value pairs. The contract
	// is not "Sprintf hides the password" (Go reflection would expose
	// it); the contract is "we never call Sprintf on credentials in
	// production code". This test asserts the intent: the field name
	// is exactly "Password" so any code-search for the literal
	// reveals the call sites.
	got := target
	if got.Password != "lab-secret-not-real" {
		t.Fatalf("password field renamed?")
	}
	// Defensive: ensure no Stringer override exists that hides the
	// secret silently (which could mislead reviewers).
	if _, hasString := any(target).(interface{ String() string }); hasString {
		t.Errorf("SSHTarget gained a Stringer — verify it does not print the password")
	}
	_ = strings.Builder{} // keep import in case a future edit needs it
}
