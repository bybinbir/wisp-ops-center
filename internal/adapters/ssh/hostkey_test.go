package ssh

import (
	"errors"
	"strings"
	"testing"
)

func TestPolicyInsecureIgnoreAlwaysPasses(t *testing.T) {
	if err := EnforcePolicy(PolicyInsecureIgnore, "host1", "fp1", "", nil); err != nil {
		t.Fatalf("insecure_ignore must pass: %v", err)
	}
}

func TestPolicyPinnedRequiresFingerprint(t *testing.T) {
	if err := EnforcePolicy(PolicyPinned, "h", "fp1", "", nil); !errors.Is(err, ErrPinnedMissing) {
		t.Fatalf("pinned without fp must fail: %v", err)
	}
	if err := EnforcePolicy(PolicyPinned, "h", "fp1", "fp1", nil); err != nil {
		t.Fatalf("pinned matching must pass: %v", err)
	}
	err := EnforcePolicy(PolicyPinned, "h", "fp1", "fp2", nil)
	if !errors.Is(err, ErrFingerprintMismatch) {
		t.Fatalf("pinned mismatch must fail: %v", err)
	}
	if strings.Contains(err.Error(), "fp1") || strings.Contains(err.Error(), "fp2") {
		t.Fatalf("error must not leak raw fingerprints: %v", err)
	}
}

func TestPolicyTOFUStoresFirstAndBlocksMismatch(t *testing.T) {
	store := NewMemoryStore()
	if err := EnforcePolicy(PolicyTOFU, "device-a", "fp1", "", store); err != nil {
		t.Fatalf("first contact must store: %v", err)
	}
	if got, ok, _ := store.Get("device-a"); !ok || got != "fp1" {
		t.Fatalf("store missing entry: %v", got)
	}
	// Same fingerprint on next visit passes.
	if err := EnforcePolicy(PolicyTOFU, "device-a", "fp1", "", store); err != nil {
		t.Fatalf("matching tofu must pass: %v", err)
	}
	// Mismatch blocks.
	err := EnforcePolicy(PolicyTOFU, "device-a", "fp2", "", store)
	if !errors.Is(err, ErrFingerprintMismatch) {
		t.Fatalf("mismatch must fail: %v", err)
	}
}

func TestUnknownPolicy(t *testing.T) {
	if err := EnforcePolicy("rocket", "h", "fp1", "", nil); !errors.Is(err, ErrUnknownPolicy) {
		t.Fatalf("unknown policy should fail: %v", err)
	}
}
