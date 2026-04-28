package networkactions

import (
	"context"
	"testing"
)

// TestMemoryToggle_DefaultClosed proves the freshly constructed
// toggle reports Enabled=false. Phase 10A's contract: a server that
// has never seen a Flip MUST stay fail-closed.
func TestMemoryToggle_DefaultClosed(t *testing.T) {
	tg := NewMemoryToggle()
	enabled, err := tg.Enabled(context.Background())
	if err != nil {
		t.Fatalf("Enabled err=%v", err)
	}
	if enabled {
		t.Fatalf("default toggle MUST be closed")
	}
	if tg.LastFlip() != nil {
		t.Errorf("expected no last flip on fresh toggle")
	}
}

// TestMemoryToggle_FlipRequiresActorAndReason — Flip MUST refuse
// empty actor/reason so the audit log can answer "who/why" later.
func TestMemoryToggle_FlipRequiresActorAndReason(t *testing.T) {
	tg := NewMemoryToggle()
	if _, err := tg.Flip(context.Background(), true, "", "x"); err == nil {
		t.Errorf("empty actor must be rejected")
	}
	if _, err := tg.Flip(context.Background(), true, "alice", ""); err == nil {
		t.Errorf("empty reason must be rejected")
	}
	enabled, _ := tg.Enabled(context.Background())
	if enabled {
		t.Errorf("rejected flips MUST NOT change state")
	}
}

// TestMemoryToggle_FlipRoundTrip — happy path: flip on, observe;
// flip off, observe; LastFlip() returns the most recent receipt.
func TestMemoryToggle_FlipRoundTrip(t *testing.T) {
	tg := NewMemoryToggle()
	r, err := tg.Flip(context.Background(), true, "alice", "phase 10A test")
	if err != nil {
		t.Fatalf("flip err=%v", err)
	}
	if !r.Enabled || r.Actor != "alice" {
		t.Errorf("receipt fields wrong: %+v", r)
	}
	if r.FlippedAt.IsZero() {
		t.Errorf("FlippedAt must be set")
	}
	enabled, _ := tg.Enabled(context.Background())
	if !enabled {
		t.Errorf("after flip(true), Enabled must be true")
	}
	if last := tg.LastFlip(); last == nil || !last.Enabled {
		t.Errorf("LastFlip should report the receipt: %+v", last)
	}

	r2, err := tg.Flip(context.Background(), false, "alice", "rolling back")
	if err != nil {
		t.Fatalf("second flip err=%v", err)
	}
	if r2.Enabled {
		t.Errorf("second flip should set enabled=false")
	}
	enabled, _ = tg.Enabled(context.Background())
	if enabled {
		t.Errorf("after flip(false), Enabled must be false")
	}
}

// TestIsDestructiveEnabled_NilTreatedAsClosed — defensive: a nil
// toggle MUST be treated as fail-closed.
func TestIsDestructiveEnabled_NilTreatedAsClosed(t *testing.T) {
	if IsDestructiveEnabled(context.Background(), nil) {
		t.Errorf("nil toggle MUST be fail-closed")
	}
}

// TestMemoryToggle_LastFlipReturnsCopy — caller MUST NOT be able to
// mutate internal state via the returned pointer.
func TestMemoryToggle_LastFlipReturnsCopy(t *testing.T) {
	tg := NewMemoryToggle()
	if _, err := tg.Flip(context.Background(), true, "alice", "ok"); err != nil {
		t.Fatalf("flip err=%v", err)
	}
	last := tg.LastFlip()
	if last == nil {
		t.Fatalf("nil last")
	}
	last.Actor = "mallory"
	again := tg.LastFlip()
	if again == nil || again.Actor != "alice" {
		t.Errorf("internal state mutated through returned pointer: %+v", again)
	}
}
