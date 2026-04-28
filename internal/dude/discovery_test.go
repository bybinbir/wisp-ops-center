package dude

import (
	"testing"
	"time"
)

// helper — wrap the per-source converter the way the real Run loop
// does: parse → convert → tag source.
func runPrimaryDude(out string, ts time.Time) []DiscoveredDevice {
	return recordsToDeviceList(ts, "mikrotik_dude", deviceFromDude)(out)
}

func runNeighbor(out string, ts time.Time) []DiscoveredDevice {
	return recordsToDeviceList(ts, "mikrotik_dude", deviceFromNeighbor)(out)
}

func TestDevicesFromDudePrint_ClassifiesAndDedupes(t *testing.T) {
	out := ` 0 name="AP-Sahil-1" address=10.0.0.10 mac-address=AA:BB:CC:11:22:33 type=ap model=hAP-ac
 1 name="CPE-Demir" address=10.0.0.20 mac-address=AA:BB:CC:44:55:66 type=cpe model=SXTsq
 2 name="PTP-link-core" address=10.0.0.30 mac-address=AA:BB:CC:77:88:99 type=bridge model=RB921`
	devs := runPrimaryDude(out, time.Now())
	if len(devs) != 3 {
		t.Fatalf("expected 3, got %d", len(devs))
	}
	for i := range devs {
		Classify(&devs[i])
	}
	if devs[0].Classification.Category != CategoryAP {
		t.Errorf("device0 category = %s", devs[0].Classification.Category)
	}
	if devs[1].Classification.Category != CategoryCPE {
		t.Errorf("device1 category = %s", devs[1].Classification.Category)
	}
	if devs[2].Classification.Category != CategoryBackhaul {
		t.Errorf("device2 category = %s", devs[2].Classification.Category)
	}
}

func TestMerge_MACWinsOverIP(t *testing.T) {
	in := []DiscoveredDevice{
		{Name: "A", IP: "10.0.0.1", MAC: "AA:BB:CC:11:22:33", Sources: []string{"dude_device"}},
		{Name: "A-dup", IP: "10.0.0.99", MAC: "AA:BB:CC:11:22:33", Platform: "RouterOS", Sources: []string{"ip_neighbor"}},
	}
	out := mergeDeviceList(in)
	if len(out) != 1 {
		t.Fatalf("merge failed: %d entries", len(out))
	}
	if out[0].Platform != "RouterOS" {
		t.Errorf("platform should have merged in: %+v", out[0])
	}
	if len(out[0].Sources) != 2 {
		t.Errorf("expected 2 sources, got %d", len(out[0].Sources))
	}
}

func TestMerge_HostNameFolds(t *testing.T) {
	// Two records of the same host: one came from /dude/device with
	// only name+host, the other came from /ip/neighbor with name+host
	// AND a MAC. They must collapse to a single record.
	in := []DiscoveredDevice{
		{Name: "ap-zone-3", IP: "10.0.0.1", Sources: []string{"dude_device"}},
		{Name: "ap-zone-3", IP: "10.0.0.1", MAC: "AA:11:22:33:44:55", Platform: "RouterOS", Sources: []string{"ip_neighbor"}},
	}
	out := mergeDeviceList(in)
	if len(out) != 1 {
		t.Fatalf("expected 1 merged record, got %d (%+v)", len(out), out)
	}
	if out[0].MAC == "" {
		t.Errorf("merge should have lifted MAC: %+v", out[0])
	}
	if out[0].EnrichedAt.IsZero() {
		t.Errorf("EnrichedAt should be set after MAC lift")
	}
}

func TestMerge_NameOnlyFallsBack(t *testing.T) {
	in := []DiscoveredDevice{
		{Name: "core-rtr", Sources: []string{"dude_device"}},
		{Name: "core-rtr", Platform: "RouterOS", Identity: "core-rtr", Sources: []string{"ip_neighbor"}},
	}
	out := mergeDeviceList(in)
	if len(out) != 1 {
		t.Fatalf("name-only fallback failed: %d entries", len(out))
	}
	if out[0].Platform != "RouterOS" {
		t.Errorf("platform should have lifted: %+v", out[0])
	}
}

func TestMerge_NoKeyDoesNotMerge(t *testing.T) {
	in := []DiscoveredDevice{{}, {}}
	out := mergeDeviceList(in)
	if len(out) != 2 {
		t.Fatalf("expected no merge for keyless devices, got %d", len(out))
	}
}

func TestStats_Tally_IncludesEnrichmentCounts(t *testing.T) {
	in := []DiscoveredDevice{
		{Classification: Classification{Category: CategoryAP, Confidence: 60},
			MAC: "AA:11:22:33:44:55", IP: "10.0.0.1", EnrichedAt: time.Now()},
		{Classification: Classification{Category: CategoryCPE, Confidence: 30},
			Name: "cpe-only"},
		{Classification: Classification{Category: CategoryUnknown, Confidence: 0},
			Name: "noname"},
	}
	var s DiscoveryStats
	s.Tally(in)
	if s.Total != 3 || s.APs != 1 || s.CPEs != 1 || s.Unknown != 1 {
		t.Errorf("unexpected stats: %+v", s)
	}
	if s.LowConfidence != 2 {
		t.Errorf("expected LowConfidence=2, got %d", s.LowConfidence)
	}
	if s.WithMAC != 1 {
		t.Errorf("expected WithMAC=1, got %d", s.WithMAC)
	}
	if s.WithHost != 1 {
		t.Errorf("expected WithHost=1, got %d", s.WithHost)
	}
	if s.EnrichedCount != 1 {
		t.Errorf("expected EnrichedCount=1, got %d", s.EnrichedCount)
	}
}

func TestNeighborDevice_HasInterfaceAndPlatform(t *testing.T) {
	out := ` 0 interface=ether1 address4=10.0.0.5 mac-address=AA:BB:CC:DD:EE:FF identity="rtr-edge" platform="MikroTik" board="CCR1009" version="6.49.10"`
	devs := runNeighbor(out, time.Now())
	if len(devs) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devs))
	}
	d := devs[0]
	if d.MAC != "AA:BB:CC:DD:EE:FF" {
		t.Errorf("MAC missing: %+v", d)
	}
	if d.Platform != "MikroTik" {
		t.Errorf("Platform missing: %+v", d)
	}
	if d.InterfaceName != "ether1" {
		t.Errorf("InterfaceName missing: %+v", d)
	}
	if d.Identity != "rtr-edge" {
		t.Errorf("Identity missing: %+v", d)
	}
}

// TestEvidenceSummary_NoSecrets ensures the human-readable evidence
// string contains only normalized field-presence flags, never raw
// content from RawMetadata that could carry secrets.
func TestEvidenceSummary_NoSecrets(t *testing.T) {
	d := DiscoveredDevice{
		Name:     "ap-1",
		MAC:      "AA:11:22:33:44:55",
		IP:       "10.0.0.1",
		Platform: "RouterOS",
		Raw: map[string]string{
			"password": "supersecret123",
			"token":    "ya29.abcdef",
		},
	}
	s := buildEvidenceSummary(d)
	if s == "" {
		t.Fatal("summary empty")
	}
	if containsAny(s, "supersecret", "ya29", "password", "token") {
		t.Errorf("summary leaked secret material: %q", s)
	}
}
