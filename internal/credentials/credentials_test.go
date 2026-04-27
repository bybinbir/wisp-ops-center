package credentials

import (
	"crypto/rand"
	"encoding/base64"
	"testing"
)

func newTestVault(t *testing.T) *AESGCMVault {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	v, err := NewAESGCMVault(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestRoundTrip(t *testing.T) {
	v := newTestVault(t)
	plaintext := "super-secret-routeros-password!"
	ct, err := v.Encrypt(plaintext)
	if err != nil {
		t.Fatal(err)
	}
	got, err := v.Decrypt(ct)
	if err != nil {
		t.Fatal(err)
	}
	if got != plaintext {
		t.Fatalf("roundtrip mismatch: %q vs %q", got, plaintext)
	}
}

func TestDecryptShortFails(t *testing.T) {
	v := newTestVault(t)
	if _, err := v.Decrypt([]byte("x")); err == nil {
		t.Fatal("expected error")
	}
}

func TestKeyFromHexAndBase64(t *testing.T) {
	if _, err := NewAESGCMVault(""); err == nil {
		t.Fatal("expected error on empty")
	}
	hex32 := "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"
	if _, err := NewAESGCMVault(hex32); err != nil {
		t.Fatalf("hex key should work: %v", err)
	}
	if _, err := NewAESGCMVault("toolong"); err == nil {
		t.Fatal("expected key length error")
	}
}

func TestSanitizeMasksSecret(t *testing.T) {
	p := Profile{Name: "X", Secret: "topsecret"}
	if got := Sanitize(p).Secret; got != "***" {
		t.Fatalf("expected mask, got %q", got)
	}
}

func TestNoopVaultRefuses(t *testing.T) {
	if _, err := (NoopVault{}).Encrypt("x"); err == nil {
		t.Fatal("expected refusal")
	}
}
