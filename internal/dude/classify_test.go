package dude

import "testing"

// ---- Phase 8.1 evidence-based classifier tests ---------------------------

func TestClassifier_RouterFromPlatform(t *testing.T) {
	d := DiscoveredDevice{
		Name:     "core-edge-1",
		IP:       "10.0.0.1",
		MAC:      "AA:11:22:33:44:55",
		Platform: "MikroTik",
		Identity: "RouterBoard CCR1009",
		Board:    "CCR1009-7G-1C-1S+",
		Model:    "CCR1009",
	}
	Classify(&d)
	if d.Classification.Category != CategoryRouter {
		t.Fatalf("expected Router, got %s (%+v)", d.Classification.Category, d.Classification)
	}
	if d.Classification.Confidence < 60 {
		t.Errorf("router with full evidence should be >=60, got %d", d.Classification.Confidence)
	}
}

func TestClassifier_APFromNameAndWirelessEvidence(t *testing.T) {
	d := DiscoveredDevice{
		Name: "AP-Sahil-3",
		IP:   "10.0.0.10",
		MAC:  "AA:11:22:33:44:01",
		Raw:  map[string]string{"wireless-mode": "ap-bridge"},
	}
	Classify(&d)
	if d.Classification.Category != CategoryAP {
		t.Fatalf("expected AP, got %s", d.Classification.Category)
	}
	if d.Classification.Confidence < 50 {
		t.Errorf("AP w/ wireless evidence should be >=50, got %d", d.Classification.Confidence)
	}
}

func TestClassifier_CPEFromClientEvidence(t *testing.T) {
	d := DiscoveredDevice{
		Name:  "cpe-musteri-117",
		IP:    "10.0.0.117",
		MAC:   "AA:11:22:33:44:99",
		Model: "SXTsq-5",
		Raw:   map[string]string{"wireless-mode": "station"},
	}
	Classify(&d)
	if d.Classification.Category != CategoryCPE {
		t.Fatalf("expected CPE, got %s", d.Classification.Category)
	}
}

func TestClassifier_BackhaulFromLinkEvidence(t *testing.T) {
	d := DiscoveredDevice{
		Name:          "PTP-link-tower-3-to-edge",
		IP:            "10.0.10.1",
		MAC:           "AA:11:22:33:44:21",
		Model:         "RB921",
		InterfaceName: "wlan1-bh",
	}
	Classify(&d)
	if d.Classification.Category != CategoryBackhaul {
		t.Fatalf("expected BackhaulLink, got %s", d.Classification.Category)
	}
}

func TestClassifier_BridgeFromBridgeEvidence(t *testing.T) {
	d := DiscoveredDevice{
		Name: "br-pop-aggregate",
		IP:   "10.0.0.50",
		Raw:  map[string]string{"interface-type": "bridge"},
	}
	Classify(&d)
	if d.Classification.Category != CategoryBridge {
		t.Fatalf("expected Bridge, got %s (%+v)", d.Classification.Category, d.Classification)
	}
}

func TestClassifier_SwitchFromSwitchEvidence(t *testing.T) {
	d := DiscoveredDevice{
		Name:  "sw-edge-1",
		IP:    "10.0.0.40",
		Model: "CRS328",
	}
	Classify(&d)
	if d.Classification.Category != CategorySwitch {
		t.Fatalf("expected Switch, got %s", d.Classification.Category)
	}
}

func TestClassifier_UnknownWhenInsufficientEvidence(t *testing.T) {
	d := DiscoveredDevice{Name: "host-37"}
	Classify(&d)
	if d.Classification.Category != CategoryUnknown {
		t.Fatalf("expected Unknown for nameless-pattern host, got %s", d.Classification.Category)
	}
	if d.Classification.Confidence != 0 {
		t.Errorf("expected confidence=0, got %d", d.Classification.Confidence)
	}
}

func TestClassifier_ConfidenceIncreasesWithEnrichment(t *testing.T) {
	weak := DiscoveredDevice{Name: "AP-1"}
	strong := DiscoveredDevice{
		Name:          "AP-1",
		MAC:           "AA:11:22:33:44:55",
		IP:            "10.0.0.5",
		Platform:      "MikroTik",
		Board:         "RB962UiGS-5HacT2HnT",
		InterfaceName: "wlan1-ap",
		Identity:      "AP-1",
		Raw:           map[string]string{"wireless-mode": "ap-bridge"},
	}
	Classify(&weak)
	Classify(&strong)
	if strong.Classification.Confidence <= weak.Classification.Confidence {
		t.Errorf("strong evidence (%d) should beat weak (%d)",
			strong.Classification.Confidence, weak.Classification.Confidence)
	}
	if strong.Classification.Confidence < 60 {
		t.Errorf("AP with full enrichment should be >=60, got %d", strong.Classification.Confidence)
	}
}

func TestClassifier_ConflictingEvidenceLowConfidence(t *testing.T) {
	// Name says CPE, Dude type says Router — within ~15 pts.
	d := DiscoveredDevice{
		Name: "cpe-mystery",
		Type: "router",
	}
	Classify(&d)
	if d.Classification.Confidence > 50 {
		t.Errorf("conflicting evidence should keep confidence low, got %d", d.Classification.Confidence)
	}
	// Verify the conflict-penalty evidence note was appended.
	hasConflict := false
	for _, ev := range d.Classification.Evidences {
		if ev.Heuristic == "conflict_penalty" {
			hasConflict = true
		}
	}
	if !hasConflict {
		t.Errorf("expected conflict_penalty evidence note, got %+v", d.Classification.Evidences)
	}
}

// ---- Pre-existing checks kept for parity with Phase 8 ---------------------

func TestClassify_DudeTypeAP(t *testing.T) {
	d := DiscoveredDevice{Name: "node-1", Type: "ap", IP: "10.0.0.1"}
	Classify(&d)
	if d.Classification.Category != CategoryAP {
		t.Fatalf("got %s, want AP", d.Classification.Category)
	}
	if d.Classification.Confidence < 50 {
		t.Errorf("confidence too low: %d", d.Classification.Confidence)
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

func TestClassify_ConfidenceCappedAt100(t *testing.T) {
	d := DiscoveredDevice{
		Name:          "AP-zone-3",
		Type:          "ap",
		Model:         "wAP",
		MAC:           "AA:11:22:33:44:99",
		IP:            "10.0.0.10",
		Platform:      "MikroTik",
		Identity:      "AP-zone-3",
		Board:         "wAP-LTE-kit",
		InterfaceName: "wlan1-ap",
		Raw:           map[string]string{"wireless-mode": "ap-bridge"},
	}
	Classify(&d)
	if d.Classification.Confidence > 100 {
		t.Errorf("confidence %d > 100", d.Classification.Confidence)
	}
}
