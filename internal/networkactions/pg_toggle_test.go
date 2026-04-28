package networkactions

import (
	"context"
	"errors"
	"testing"
)

// TestPgToggleStore_NilPoolFailsClosed — defensive: nil pool MUST
// surface ErrToggleStoreUnavailable (not nil enabled). Phase 10B's
// contract is fail-closed on every failure path.
func TestPgToggleStore_NilPoolFailsClosed(t *testing.T) {
	var s *PgToggleStore
	enabled, err := s.Enabled(context.Background())
	if !errors.Is(err, ErrToggleStoreUnavailable) {
		t.Errorf("nil store: err=%v want ErrToggleStoreUnavailable", err)
	}
	if enabled {
		t.Errorf("nil store MUST report enabled=false")
	}

	s2 := &PgToggleStore{P: nil}
	enabled, err = s2.Enabled(context.Background())
	if !errors.Is(err, ErrToggleStoreUnavailable) {
		t.Errorf("nil P: err=%v want ErrToggleStoreUnavailable", err)
	}
	if enabled {
		t.Errorf("nil P MUST report enabled=false")
	}
}

// TestPgToggleStore_FlipRequiresActorAndReason — input validation
// runs BEFORE any DB write, so it works even when the pool is nil.
func TestPgToggleStore_FlipRequiresActorAndReason(t *testing.T) {
	s := &PgToggleStore{P: nil}
	if _, err := s.Flip(context.Background(), true, "  ", "x"); err == nil {
		t.Errorf("empty actor must be rejected")
	}
	if _, err := s.Flip(context.Background(), true, "alice", "  "); err == nil {
		t.Errorf("empty reason must be rejected")
	}
}

// TestPgToggleStore_ImplementsDestructiveToggle — compile-time check
// the type satisfies the interface.
func TestPgToggleStore_ImplementsDestructiveToggle(t *testing.T) {
	var _ DestructiveToggle = (*PgToggleStore)(nil)
}
