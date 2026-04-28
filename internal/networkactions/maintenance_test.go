package networkactions

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestValidateMaintenanceRecord_RejectsBadInputs — every structural
// invariant produces a typed sentinel.
func TestValidateMaintenanceRecord_RejectsBadInputs(t *testing.T) {
	now := time.Now().UTC()

	cases := []struct {
		name string
		rec  MaintenanceRecord
		want error
	}{
		{"empty title", MaintenanceRecord{
			Title: "  ", Start: now, End: now.Add(2 * time.Hour),
		}, ErrMaintenanceWindowEmptyTitle},
		{"inverted range", MaintenanceRecord{
			Title: "ok", Start: now.Add(time.Hour), End: now,
		}, ErrMaintenanceWindowInvertedRange},
		{"zero range", MaintenanceRecord{
			Title: "ok", Start: now, End: now,
		}, ErrMaintenanceWindowInvertedRange},
		{"too short", MaintenanceRecord{
			Title: "ok", Start: now, End: now.Add(30 * time.Second),
		}, ErrMaintenanceWindowDurationTooShort},
		{"too long", MaintenanceRecord{
			Title: "ok", Start: now, End: now.Add(48 * time.Hour),
		}, ErrMaintenanceWindowDurationTooLong},
	}
	for _, c := range cases {
		err := ValidateMaintenanceRecord(c.rec)
		if !errors.Is(err, c.want) {
			t.Errorf("%s: got %v want %v", c.name, err, c.want)
		}
	}
}

// TestValidateMaintenanceRecord_AcceptsHappyPath — typical 1h
// window passes.
func TestValidateMaintenanceRecord_AcceptsHappyPath(t *testing.T) {
	now := time.Now().UTC()
	rec := MaintenanceRecord{
		Title: "frequency tweak window",
		Start: now,
		End:   now.Add(time.Hour),
	}
	if err := ValidateMaintenanceRecord(rec); err != nil {
		t.Errorf("happy record rejected: %v", err)
	}
}

// TestMemoryMaintenanceStore_CreateGetList — round-trip + listing
// in chronological order.
func TestMemoryMaintenanceStore_CreateGetList(t *testing.T) {
	s := NewMemoryMaintenanceStore()
	now := time.Now().UTC()

	r1, err := s.Create(context.Background(), MaintenanceRecord{
		Title: "later", Start: now.Add(2 * time.Hour), End: now.Add(3 * time.Hour),
	})
	if err != nil {
		t.Fatalf("create r1: %v", err)
	}
	r2, err := s.Create(context.Background(), MaintenanceRecord{
		Title: "now", Start: now.Add(-30 * time.Minute), End: now.Add(30 * time.Minute),
	})
	if err != nil {
		t.Fatalf("create r2: %v", err)
	}
	got, err := s.Get(context.Background(), r1.ID)
	if err != nil || got.Title != "later" {
		t.Errorf("get r1 failed: %+v err=%v", got, err)
	}
	list, err := s.List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 || list[0].Title != "now" || list[1].Title != "later" {
		t.Errorf("list order wrong: %+v", list)
	}
	_ = r2
}

// TestMemoryMaintenanceStore_GetMissing — typed not-found sentinel.
func TestMemoryMaintenanceStore_GetMissing(t *testing.T) {
	s := NewMemoryMaintenanceStore()
	_, err := s.Get(context.Background(), "no-such-id")
	if !errors.Is(err, ErrMaintenanceWindowNotFound) {
		t.Errorf("expected not-found, got %v", err)
	}
}

// TestMemoryMaintenanceStore_ActiveAt — only windows that are open
// at `at` AND apply to the device are returned.
func TestMemoryMaintenanceStore_ActiveAt(t *testing.T) {
	s := NewMemoryMaintenanceStore()
	now := time.Now().UTC()
	_, _ = s.Create(context.Background(), MaintenanceRecord{
		Title: "open-all",
		Start: now.Add(-1 * time.Minute), End: now.Add(time.Hour),
	})
	_, _ = s.Create(context.Background(), MaintenanceRecord{
		Title: "open-scoped",
		Start: now.Add(-1 * time.Minute), End: now.Add(time.Hour),
		Scope: []string{"dev-1"},
	})
	_, _ = s.Create(context.Background(), MaintenanceRecord{
		Title: "future",
		Start: now.Add(time.Hour), End: now.Add(2 * time.Hour),
	})

	got, err := s.ActiveAt(context.Background(), "dev-1", now)
	if err != nil {
		t.Fatalf("active err=%v", err)
	}
	if len(got) != 2 {
		t.Errorf("dev-1: expected open-all + open-scoped, got %d (%+v)", len(got), got)
	}
	got, _ = s.ActiveAt(context.Background(), "dev-other", now)
	if len(got) != 1 || got[0].Title != "open-all" {
		t.Errorf("dev-other: expected only open-all, got %+v", got)
	}
	got, _ = s.ActiveAt(context.Background(), "dev-1", now.Add(2*time.Hour))
	if len(got) != 0 {
		t.Errorf("after future window closes: expected 0, got %+v", got)
	}
}

// TestMaintenanceRecord_IsOpenAt — the [Start, End) interval rule
// must hold (End is EXCLUSIVE).
func TestMaintenanceRecord_IsOpenAt(t *testing.T) {
	now := time.Now().UTC()
	rec := MaintenanceRecord{Start: now, End: now.Add(time.Hour)}
	if !rec.IsOpenAt(now) {
		t.Errorf("Start should be inclusive")
	}
	if !rec.IsOpenAt(now.Add(30 * time.Minute)) {
		t.Errorf("midpoint should be open")
	}
	if rec.IsOpenAt(now.Add(time.Hour)) {
		t.Errorf("End must be EXCLUSIVE")
	}
	if rec.IsOpenAt(now.Add(-time.Second)) {
		t.Errorf("before Start must be closed")
	}
}

// TestMaintenanceRecord_AppliesToDevice — empty scope = all
// devices; populated scope = that device.
func TestMaintenanceRecord_AppliesToDevice(t *testing.T) {
	rec := MaintenanceRecord{}
	if !rec.AppliesToDevice("any") {
		t.Errorf("empty scope must apply to any device")
	}
	rec.Scope = []string{"dev-1", "dev-2"}
	if !rec.AppliesToDevice("dev-2") {
		t.Errorf("dev-2 must match")
	}
	if rec.AppliesToDevice("dev-3") {
		t.Errorf("dev-3 must not match")
	}
}
