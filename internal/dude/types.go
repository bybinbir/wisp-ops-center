// Package dude is the Phase 8 read-only discovery adapter for the
// MikroTik Dude. It connects over SSH, runs an allowlisted set of
// RouterOS commands, parses the textual output and classifies each
// host as AP, CPE, bridge, link, router, switch or unknown.
//
// IMPORTANT: This package is strictly read-only. It NEVER edits
// configuration, sets frequencies, reboots devices or runs any
// destructive command. The action framework that may eventually
// support those flows lives in internal/networkactions and is
// scaffolding-only in Phase 8.
//
// Phase 8.1 adds enrichment: after the primary /dude/device/print
// pass, the orchestrator also tries /ip/neighbor + /dude/probe +
// /dude/service and merges results by stable identity (MAC > host+
// name > name). Each enrichment source records attempted/succeeded/
// skipped status so unsupported devices fail soft instead of failing
// the run.
package dude

import "time"

// Category classifies a discovered host.
type Category string

const (
	CategoryAP       Category = "AP"
	CategoryBackhaul Category = "BackhaulLink"
	CategoryBridge   Category = "Bridge"
	CategoryCPE      Category = "CPE"
	CategoryRouter   Category = "Router"
	CategorySwitch   Category = "Switch"
	CategoryUnknown  Category = "Unknown"
)

var AllCategories = []Category{
	CategoryAP, CategoryBackhaul, CategoryBridge,
	CategoryCPE, CategoryRouter, CategorySwitch, CategoryUnknown,
}

func IsValidCategory(s string) bool {
	for _, c := range AllCategories {
		if string(c) == s {
			return true
		}
	}
	return false
}

// DiscoveredDevice is the normalized record produced by Discover.
//
// Phase 8.1: Platform, InterfaceName, EvidenceSummary, Sources and
// EnrichedAt fields were added so downstream consumers (repository,
// API, classifier) can express richer signals than name-only.
type DiscoveredDevice struct {
	Source         string
	Name           string
	IP             string
	MAC            string
	Model          string
	OSVersion      string
	Identity       string
	Type           string
	Status         string
	Platform       string // e.g. "MikroTik", "RouterOS", "Mimosa"
	Board          string // e.g. "RB922UAGS-5HPacD"
	InterfaceName  string // upstream interface from neighbor/probe
	Classification Classification
	Raw            map[string]string
	LastSeen       time.Time
	// Sources is the ordered list of enrichment sources that
	// contributed to this device record (e.g. ["dude_device",
	// "ip_neighbor", "dude_probe"]). Empty when only the primary
	// pass observed the host.
	Sources []string
	// EvidenceSummary is a short human-readable string summarizing
	// the strongest signals (mac=AA:..  platform=RouterOS  name_hint=AP-).
	// It is what the inventory grid surfaces in the "Kanıt" column.
	EvidenceSummary string
	// EnrichedAt is the timestamp of the last enrichment pass that
	// successfully added a non-name signal (MAC, platform, identity,
	// interface). Stays zero when enrichment yielded nothing for this
	// host.
	EnrichedAt time.Time
}

type Evidence struct {
	Heuristic string
	Weight    int
	Reason    string
}

type Classification struct {
	Category   Category
	Confidence int
	Evidences  []Evidence
}

type DiscoveryStats struct {
	Total         int
	APs           int
	BackhaulLinks int
	Bridges       int
	CPEs          int
	Routers       int
	Switches      int
	Unknown       int
	LowConfidence int
	// WithMAC counts devices that ended up with a non-empty MAC after
	// enrichment. Used in the audit metadata as the headline indicator
	// of how much enrichment actually contributed.
	WithMAC int
	// WithHost counts devices that ended up with a non-empty IP/host.
	WithHost int
	// EnrichedCount counts devices whose EnrichedAt is non-zero.
	EnrichedCount int
}

func (s *DiscoveryStats) Tally(devs []DiscoveredDevice) {
	for _, d := range devs {
		s.Total++
		switch d.Classification.Category {
		case CategoryAP:
			s.APs++
		case CategoryBackhaul:
			s.BackhaulLinks++
		case CategoryBridge:
			s.Bridges++
		case CategoryCPE:
			s.CPEs++
		case CategoryRouter:
			s.Routers++
		case CategorySwitch:
			s.Switches++
		default:
			s.Unknown++
		}
		if d.Classification.Confidence < 50 {
			s.LowConfidence++
		}
		if d.MAC != "" {
			s.WithMAC++
		}
		if d.IP != "" {
			s.WithHost++
		}
		if !d.EnrichedAt.IsZero() {
			s.EnrichedCount++
		}
	}
}

// SourceStatus represents how one enrichment command performed in a
// single run. Captured for every attempted source so the operator can
// see "neighbor returned 217 records, probe was unsupported, service
// timed out" without leaking secret material.
type SourceStatus struct {
	// Source is the human-readable label, e.g. "dude_device" or
	// "ip_neighbor".
	Source string `json:"source"`
	// Command is the exact RouterOS command that was attempted.
	Command string `json:"command"`
	// Status is one of: "succeeded", "skipped_unsupported",
	// "skipped_empty", "failed".
	Status string `json:"status"`
	// Records is the number of parsed records returned (regardless of
	// merge result).
	Records int `json:"records"`
	// DurationMS is the elapsed milliseconds spent talking to the
	// remote device for this source.
	DurationMS int64 `json:"duration_ms"`
	// ErrorCode is a stable short code (allowed by ErrorCode()) when
	// the source failed, "" when succeeded or skipped.
	ErrorCode string `json:"error_code,omitempty"`
	// ErrorMessage is sanitized (SanitizeMessage) so secrets cannot
	// leak even if the SSH transport wrote one into the message.
	ErrorMessage string `json:"error_message,omitempty"`
}
