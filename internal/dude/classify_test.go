package dude

import "testing"

func TestClassify_DudeTypeAP(t *testing.T) {
	d := DiscoveredDevice{Name: "node-1", Type: "ap"}
	Classify(&d)
	if d.Classification.Category != CategoryAP {
		t.Fatalf("got %s, want AP", d.Classification.Category)
	}
	if d.Classification.Confidence < 50 {
		t.Errorf("confidence too low: %d", d.Classification.Confidence)
	}
}

func TestClassify_NamePrefixCPE(t *testing.T) {
	d := DiscoveredDevice{Name: "CPE-müşteri-117"}
	Classify(&d)
	if d.Classification.Category != CategoryCPE {
		t.Fatalf("got %s, want CPE", d.Classification.Category)
	}
}

func TestClassify_NamePrefixBackhaul(t *testing.T) {
	d := DiscoveredDevice{Name: "PTP-link-core-edge", Model: "RB921"}
	Classify(&d)
	if d.Classification.Category != CategoryBackhaul {
		t.Fatalf("got %s, want BackhaulLink", d.Classification.Category)
	}
}

func TestClassify_WirelessModeAP(t *testing.T) {
	d := DiscoveredDevice{
		Name: "node",
		Raw:  map[string]string{"wireless-mode": "ap-bridge"},
	}
	Classify(&d)
	if d.Classification.Category != CategoryAP {
		t.Fatalf("got %s, want AP from wireless-mode", d.Classification.Category)
	}
}

func TestClassify_NoSignalsIsUnknown(t *testing.T) {
	d := DiscoveredDevice{}
	Classify(&d)
	if d.Classification.Category != CategoryUnknown {
		t.Fatalf("got %s, want Unknown", d.Classification.Category)
	}
	if d.Classification.Confidence != 0 {
		t.Errorf("confidence should be 0, got %d", d.Classification.Confidence)
	}
}

func TestClassify_ConflictResolvedByHighestScore(t *testing.T) {
	// Both 'router' name AND 'ap' Dude type. Dude type is heavier (50 vs 35).
	d := DiscoveredDevice{Name: "rtr-edge-1", Type: "ap"}
	Classify(&d)
	if d.Classification.Category != CategoryAP {
		t.Fatalf("expected AP to win, got %s", d.Classification.Category)
	}
}

func TestClassify_ConfidenceCappedAt100(t *testing.T) {
	d := DiscoveredDevice{
		Name:  "AP-zone-3",
		Type:  "ap",
		Model: "wAP",
		Raw:   map[string]string{"wireless-mode": "ap-bridge"},
	}
	Classify(&d)
	if d.Classification.Confidence > 100 {
		t.Errorf("confidence %d > 100", d.Classification.Confidence)
	}
}
