package dude

import (
	"testing"
	"time"
)

func TestDevicesFromDudePrint_ClassifiesAndDedupes(t *testing.T) {
	out := ` 0 name="AP-Sahil-1" address=10.0.0.10 mac-address=AA:BB:CC:11:22:33 type=ap model=hAP-ac
 1 name="CPE-Demir" address=10.0.0.20 mac-address=AA:BB:CC:44:55:66 type=cpe model=SXTsq
 2 name="PTP-link-core" address=10.0.0.30 mac-address=AA:BB:CC:77:88:99 type=bridge model=RB921`
	devs := devicesFromDudePrint(out, time.Now())
	if len(devs) != 3 {
		t.Fatalf("expected 3, got %d", len(devs))
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

func TestDedupe_MACWinsOverIP(t *testing.T) {
	in := []DiscoveredDevice{
		{Name: "A", IP: "10.0.0.1", MAC: "AA:BB:CC:11:22:33", Classification: Classification{Category: CategoryAP, Confidence: 60}},
		{Name: "A-dup", IP: "10.0.0.99", MAC: "AA:BB:CC:11:22:33", Classification: Classification{Category: CategoryAP, Confidence: 80}},
	}
	out := dedupeDevices(in)
	if len(out) != 1 {
		t.Fatalf("dedupe failed: %d entries", len(out))
	}
	// Higher-confidence classification should win after merge.
	if out[0].Classification.Confidence != 80 {
		t.Errorf("expected confidence 80, got %d", out[0].Classification.Confidence)
	}
}

func TestDedupe_NoKeyDoesNotMerge(t *testing.T) {
	in := []DiscoveredDevice{{}, {}}
	out := dedupeDevices(in)
	if len(out) != 2 {
		t.Fatalf("expected no merge for keyless devices, got %d", len(out))
	}
}

func TestStats_Tally(t *testing.T) {
	in := []DiscoveredDevice{
		{Classification: Classification{Category: CategoryAP, Confidence: 60}},
		{Classification: Classification{Category: CategoryCPE, Confidence: 30}},
		{Classification: Classification{Category: CategoryUnknown, Confidence: 0}},
	}
	var s DiscoveryStats
	s.Tally(in)
	if s.Total != 3 || s.APs != 1 || s.CPEs != 1 || s.Unknown != 1 {
		t.Errorf("unexpected stats: %+v", s)
	}
	if s.LowConfidence != 2 {
		t.Errorf("expected LowConfidence=2, got %d", s.LowConfidence)
	}
}
