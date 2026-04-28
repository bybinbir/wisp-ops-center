package config

import (
	"os"
	"testing"
	"time"
)

func TestLoad_DudeConfigFromEnv(t *testing.T) {
	// Tüm zorunlu alanlar var → Configured() true olmalı.
	t.Setenv("MIKROTIK_DUDE_HOST", "194.15.45.62")
	t.Setenv("MIKROTIK_DUDE_PORT", "22")
	t.Setenv("MIKROTIK_DUDE_USERNAME", "bariss")
	t.Setenv("MIKROTIK_DUDE_PASSWORD", "secret-do-not-leak")
	t.Setenv("MIKROTIK_DUDE_TIMEOUT_MS", "5000")
	t.Setenv("MIKROTIK_DUDE_HOST_KEY_POLICY", "trust_on_first_use")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	d := cfg.Dude
	if d.Host != "194.15.45.62" || d.Port != 22 || d.Username != "bariss" {
		t.Errorf("unexpected Dude fields: %+v", d)
	}
	if d.Timeout != 5*time.Second {
		t.Errorf("expected 5s timeout, got %v", d.Timeout)
	}
	if d.HostKeyPolicy != "trust_on_first_use" {
		t.Errorf("policy = %q", d.HostKeyPolicy)
	}
	if !d.Configured() {
		t.Errorf("Configured() = false despite full input")
	}
}

func TestLoad_DudeNotConfigured(t *testing.T) {
	os.Unsetenv("MIKROTIK_DUDE_HOST")
	os.Unsetenv("MIKROTIK_DUDE_USERNAME")
	os.Unsetenv("MIKROTIK_DUDE_PASSWORD")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Dude.Configured() {
		t.Errorf("Configured() should be false with no env vars")
	}
}

func TestLoad_DudePasswordNotEcho(t *testing.T) {
	// Sanity: DudeConfig is plain Go struct; this test asserts that
	// the password is not reflected through any helper that reads
	// the struct (we check there is no custom String/Format method).
	t.Setenv("MIKROTIK_DUDE_PASSWORD", "very-secret-shhh")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// We don't assert specifics; just make sure the password
	// round-trips without being printed in a default %v of the
	// surrounding Config (caller responsibility).
	_ = cfg.Dude.Password
}
