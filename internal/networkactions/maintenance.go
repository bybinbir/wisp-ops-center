package networkactions

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

// MaintenanceWindowID is a stable identifier for a window record.
// Phase 10A keeps this as `string` so a future SQL-backed store can
// use uuids without changing the interface.
type MaintenanceWindowID = string

// MaintenanceRecord is the persisted shape of one window. It
// supersedes the in-memory `MaintenanceWindow` struct in
// framework.go for callers that need scope/actor/state.
//
// `Scope` is the optional list of `network_devices.id` strings the
// window applies to. An empty Scope means "all devices in this
// tenant". Phase 10A does not enforce per-device scope yet, but the
// field is wired so Phase 10's API can populate it without a
// schema change.
type MaintenanceRecord struct {
	ID        MaintenanceWindowID `json:"id"`
	Title     string              `json:"title"`
	Start     time.Time           `json:"start"`
	End       time.Time           `json:"end"`
	Scope     []string            `json:"scope,omitempty"`
	CreatedBy string              `json:"created_by"`
	CreatedAt time.Time           `json:"created_at"`
}

// IsOpenAt reports whether `at` falls inside [Start, End).
func (m MaintenanceRecord) IsOpenAt(at time.Time) bool {
	if m.Start.IsZero() || m.End.IsZero() {
		return false
	}
	return !at.Before(m.Start) && at.Before(m.End)
}

// AppliesToDevice reports whether this window covers the given
// device id. Empty Scope means "applies to all".
func (m MaintenanceRecord) AppliesToDevice(deviceID string) bool {
	if len(m.Scope) == 0 {
		return true
	}
	for _, s := range m.Scope {
		if strings.EqualFold(s, deviceID) {
			return true
		}
	}
	return false
}

// MaintenanceProvider is the read interface the destructive gate
// consults. A repository-backed implementation can swap in later.
type MaintenanceProvider interface {
	// ActiveAt returns every window that is open at `at` and
	// applies to `deviceID` (empty deviceID = "any device").
	ActiveAt(ctx context.Context, deviceID string, at time.Time) ([]MaintenanceRecord, error)
}

// MaintenanceStore extends the read interface with the write
// surface needed for Phase 10's CRUD endpoint. Phase 10A ships an
// in-memory implementation only; Phase 10 will add a Postgres
// store backed by the new `maintenance_windows` table.
type MaintenanceStore interface {
	MaintenanceProvider
	Create(ctx context.Context, rec MaintenanceRecord) (MaintenanceRecord, error)
	Get(ctx context.Context, id MaintenanceWindowID) (MaintenanceRecord, error)
	List(ctx context.Context) ([]MaintenanceRecord, error)
}

// Validation sentinels.
var (
	ErrMaintenanceWindowEmptyTitle       = errors.New("networkactions: maintenance_window_empty_title")
	ErrMaintenanceWindowInvertedRange    = errors.New("networkactions: maintenance_window_inverted_range")
	ErrMaintenanceWindowDurationTooLong  = errors.New("networkactions: maintenance_window_duration_too_long")
	ErrMaintenanceWindowDurationTooShort = errors.New("networkactions: maintenance_window_duration_too_short")
	ErrMaintenanceWindowNotFound         = errors.New("networkactions: maintenance_window_not_found")
)

// MaxMaintenanceWindowDuration caps how long a single window can
// span. Phase 10A picks 24h: any single change window longer than
// a day deserves operator review (and probably a sequence of
// shorter windows).
const MaxMaintenanceWindowDuration = 24 * time.Hour

// MinMaintenanceWindowDuration is the floor that prevents
// zero/sub-second windows that would never be open in practice.
const MinMaintenanceWindowDuration = 1 * time.Minute

// ValidateMaintenanceRecord checks the structural invariants for a
// new window record before it reaches storage. Phase 10A enforces
// these in MemoryMaintenanceStore.Create; Phase 10 will reuse the
// same validator in the API handler.
func ValidateMaintenanceRecord(r MaintenanceRecord) error {
	if strings.TrimSpace(r.Title) == "" {
		return ErrMaintenanceWindowEmptyTitle
	}
	if r.Start.IsZero() || r.End.IsZero() || !r.End.After(r.Start) {
		return ErrMaintenanceWindowInvertedRange
	}
	dur := r.End.Sub(r.Start)
	if dur < MinMaintenanceWindowDuration {
		return ErrMaintenanceWindowDurationTooShort
	}
	if dur > MaxMaintenanceWindowDuration {
		return ErrMaintenanceWindowDurationTooLong
	}
	return nil
}

// MemoryMaintenanceStore is the canonical hermetic implementation
// of MaintenanceStore. Tests use it; the API server can fall back
// to it when no DB is configured. Concurrent-safe.
type MemoryMaintenanceStore struct {
	mu      sync.RWMutex
	records map[MaintenanceWindowID]MaintenanceRecord
	idGen   func() string
	clock   func() time.Time
	seq     uint64 // monotonic id counter
}

// NewMemoryMaintenanceStore returns an empty in-memory store.
func NewMemoryMaintenanceStore() *MemoryMaintenanceStore {
	return &MemoryMaintenanceStore{
		records: map[MaintenanceWindowID]MaintenanceRecord{},
	}
}

// SetIDGenerator overrides the id generator (default: timestamp +
// monotonic counter so back-to-back Create calls cannot collide).
// Useful for hermetic tests.
func (s *MemoryMaintenanceStore) SetIDGenerator(fn func() string) { s.idGen = fn }

// SetClock overrides the now() source.
func (s *MemoryMaintenanceStore) SetClock(fn func() time.Time) { s.clock = fn }

func (s *MemoryMaintenanceStore) now() time.Time {
	if s.clock != nil {
		return s.clock()
	}
	return time.Now().UTC()
}

// generateID returns a unique id. Lock MUST be held by the caller
// so the seq increment is consistent with the records map write.
// The default format combines timestamp (for human readability)
// with a monotonic counter (for collision safety in fast tests).
func (s *MemoryMaintenanceStore) generateID() string {
	if s.idGen != nil {
		return s.idGen()
	}
	s.seq++
	return "mw-" + s.now().UTC().Format("20060102T150405") + "-" + uintToStr(s.seq)
}

func uintToStr(n uint64) string {
	if n == 0 {
		return "0"
	}
	const digits = "0123456789"
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = digits[n%10]
		n /= 10
	}
	return string(buf[i:])
}

// Create implements MaintenanceStore. Validates the record, fills
// id + timestamps, and persists. The seq + records write happen
// under the same lock so Create is collision-safe under contention.
func (s *MemoryMaintenanceStore) Create(_ context.Context, rec MaintenanceRecord) (MaintenanceRecord, error) {
	if err := ValidateMaintenanceRecord(rec); err != nil {
		return MaintenanceRecord{}, err
	}
	if strings.TrimSpace(rec.CreatedBy) == "" {
		rec.CreatedBy = "system"
	}
	s.mu.Lock()
	rec.ID = s.generateID()
	rec.CreatedAt = s.now()
	s.records[rec.ID] = rec
	s.mu.Unlock()
	return rec, nil
}

// Get implements MaintenanceStore.
func (s *MemoryMaintenanceStore) Get(_ context.Context, id MaintenanceWindowID) (MaintenanceRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.records[id]
	if !ok {
		return MaintenanceRecord{}, ErrMaintenanceWindowNotFound
	}
	return rec, nil
}

// List implements MaintenanceStore. Returns records ordered by Start
// ascending so the API can render a stable timeline.
func (s *MemoryMaintenanceStore) List(_ context.Context) ([]MaintenanceRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]MaintenanceRecord, 0, len(s.records))
	for _, r := range s.records {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	return out, nil
}

// ActiveAt implements MaintenanceProvider.
func (s *MemoryMaintenanceStore) ActiveAt(_ context.Context, deviceID string, at time.Time) ([]MaintenanceRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]MaintenanceRecord, 0)
	for _, r := range s.records {
		if !r.IsOpenAt(at) {
			continue
		}
		if !r.AppliesToDevice(deviceID) {
			continue
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	return out, nil
}
