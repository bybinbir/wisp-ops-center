// Package networkinv is the Phase 8 persistence layer for the
// MikroTik Dude discovery results. It manages four tables:
//
//   - discovery_runs        : metadata for a single discovery pass
//   - network_devices       : the inventory itself, deduped by MAC/IP/name
//   - network_links         : (skeleton) link records between devices
//   - device_category_evidence : per-device classification trail
//
// The repository deliberately speaks `dude.DiscoveredDevice` so the
// API/worker layer doesn't need to know about pgx pools.
package networkinv

import (
	"time"

	"github.com/wisp-ops-center/wisp-ops-center/internal/dude"
)

// Device is the DB-row projection of a discovered host.
type Device struct {
	ID          string            `json:"id"`
	Source      string            `json:"source"`
	Host        string            `json:"host,omitempty"`
	Name        string            `json:"name"`
	MAC         string            `json:"mac,omitempty"`
	Model       string            `json:"model,omitempty"`
	OSVersion   string            `json:"os_version,omitempty"`
	Identity    string            `json:"identity,omitempty"`
	DeviceType  string            `json:"device_type,omitempty"`
	Category    dude.Category     `json:"category"`
	Confidence  int               `json:"confidence"`
	Status      string            `json:"status"`
	LastSeenAt  time.Time         `json:"last_seen_at"`
	FirstSeenAt time.Time         `json:"first_seen_at"`
	RawMetadata map[string]string `json:"raw_metadata"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// Run is the DB-row projection of a discovery_runs entry.
type Run struct {
	ID            string     `json:"id"`
	Source        string     `json:"source"`
	CorrelationID string     `json:"correlation_id"`
	StartedAt     time.Time  `json:"started_at"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
	Status        string     `json:"status"`
	DeviceCount   int        `json:"device_count"`
	APCount       int        `json:"ap_count"`
	CPECount      int        `json:"cpe_count"`
	BridgeCount   int        `json:"bridge_count"`
	LinkCount     int        `json:"link_count"`
	RouterCount   int        `json:"router_count"`
	SwitchCount   int        `json:"switch_count"`
	UnknownCount  int        `json:"unknown_count"`
	LowConfCount  int        `json:"low_conf_count"`
	ErrorCode     string     `json:"error_code,omitempty"`
	ErrorMessage  string     `json:"error_message,omitempty"`
	CommandsRun   []string   `json:"commands_run"`
	TriggeredBy   string     `json:"triggered_by"`
	CreatedAt     time.Time  `json:"created_at"`
}

// Filter narrows down a device listing.
type Filter struct {
	Category    string
	Status      string
	OnlyLowConf bool
	OnlyUnknown bool
	Limit       int
	Offset      int
}
