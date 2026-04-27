// Package workorders, Faz 7 gerçek iş emri (work order) tablo modeli ve
// state machine validasyonunu tutar. Repository pgxpool kullanır; testler
// ise tabloya hiç dokunmadan çalıştırılabilen state-machine helper'ı
// üzerinden yapılır.
//
// Tasarım kuralları:
//   - Bütün mutating çağrılar audit ve work_order_events satırı üretebilmek
//     için actor parametresi alır.
//   - Status machine deterministiktir; geçersiz geçiş ErrInvalidTransition
//     döner — repository'ye hiç gitmez.
//   - Cihaza yazma yapan veya cihazdan veri çeken hiçbir kod yoktur.
package workorders

import (
	"errors"
	"strings"
)

// Status, work_orders.status alanı.
type Status string

const (
	StatusOpen       Status = "open"
	StatusAssigned   Status = "assigned"
	StatusInProgress Status = "in_progress"
	StatusResolved   Status = "resolved"
	StatusCancelled  Status = "cancelled"
)

// AllStatuses, izin verilen status değerleri.
func AllStatuses() []Status {
	return []Status{StatusOpen, StatusAssigned, StatusInProgress, StatusResolved, StatusCancelled}
}

// IsValidStatus, s tanınan bir status mı.
func IsValidStatus(s string) bool {
	for _, v := range AllStatuses() {
		if string(v) == s {
			return true
		}
	}
	return false
}

// Priority, work_orders.priority alanı.
type Priority string

const (
	PriorityLow    Priority = "low"
	PriorityMedium Priority = "medium"
	PriorityHigh   Priority = "high"
	PriorityUrgent Priority = "urgent"
)

// AllPriorities, izin verilen priority değerleri.
func AllPriorities() []Priority {
	return []Priority{PriorityLow, PriorityMedium, PriorityHigh, PriorityUrgent}
}

// IsValidPriority, p tanınan bir priority mı.
func IsValidPriority(p string) bool {
	for _, v := range AllPriorities() {
		if string(v) == p {
			return true
		}
	}
	return false
}

// PriorityFromSeverity, scoring severity → work order priority varsayılanı.
// Operatör daha sonra UI'da değiştirebilir.
func PriorityFromSeverity(severity string) Priority {
	switch strings.ToLower(severity) {
	case "critical":
		return PriorityHigh
	case "warning":
		return PriorityMedium
	}
	return PriorityLow
}

// ErrInvalidTransition, izin verilmeyen status geçişlerinde döner.
var ErrInvalidTransition = errors.New("workorders: invalid status transition")

// CanTransition, eski → yeni status geçişine izin verilip verilmediğini
// söyler. Faz 7 state machine'i:
//
//	open → assigned | in_progress | cancelled
//	assigned → in_progress | open | cancelled
//	in_progress → resolved | open | cancelled
//	resolved → (terminal)
//	cancelled → (terminal)
//
// Aynı state'e geçiş (no-op) izinli kabul edilmez; çağıran tarafın
// gereksiz UPDATE çekmemesi gerekir.
func CanTransition(from, to Status) bool {
	if from == to {
		return false
	}
	switch from {
	case StatusOpen:
		return to == StatusAssigned || to == StatusInProgress || to == StatusCancelled
	case StatusAssigned:
		return to == StatusInProgress || to == StatusOpen || to == StatusCancelled
	case StatusInProgress:
		return to == StatusResolved || to == StatusOpen || to == StatusCancelled
	case StatusResolved, StatusCancelled:
		return false
	}
	return false
}

// IsTerminal, status terminal mi (resolved | cancelled).
func IsTerminal(s Status) bool {
	return s == StatusResolved || s == StatusCancelled
}

// EventType, work_order_events.event_type alanı için bilinen tipler.
type EventType string

const (
	EventCreated         EventType = "created"
	EventStatusChanged   EventType = "status_changed"
	EventAssigned        EventType = "assigned"
	EventUnassigned      EventType = "unassigned"
	EventPriorityChanged EventType = "priority_changed"
	EventETAUpdated      EventType = "eta_updated"
	EventNoteAdded       EventType = "note_added"
	EventResolved        EventType = "resolved"
	EventCancelled       EventType = "cancelled"
	EventReopened        EventType = "reopened"
)
