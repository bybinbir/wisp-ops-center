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
	Classification Classification
	Raw            map[string]string
	LastSeen       time.Time
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
	}
}
