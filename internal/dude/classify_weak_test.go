package dude

import (
	"strings"
	"testing"
)

func TestWeak_AP_KuleMatchesViaFallback(t *testing.T) {
	d := &DiscoveredDevice{Name: "Kule-12-Anamur"}
	Classify(d)
	if d.Classification.Category != CategoryAP {
		t.Fatalf("got %q, want AP (kule weak match)", d.Classification.Category)
	}
	if d.Classification.Confidence != 45 {
		t.Fatalf("confidence = %d, want 45", d.Classification.Confidence)
	}
	if !hasEvidenceHeuristic(d, "weak_name_pattern") {
		t.Fatal("missing weak_name_pattern evidence row")
	}
}

func TestWeak_AP_OmniMatchesViaFallback(t *testing.T) {
	d := &DiscoveredDevice{Name: "Omni-Merkez"}
	Classify(d)
	if d.Classification.Category != CategoryAP {
		t.Fatalf("got %q, want AP", d.Classification.Category)
	}
	if d.Classification.Confidence != 45 {
		t.Fatalf("confidence = %d, want 45", d.Classification.Confidence)
	}
}

func TestWeak_BackhaulLink_MimosaMatchesViaFallback(t *testing.T) {
	d := &DiscoveredDevice{Name: "Mimosa-Iskele"}
	Classify(d)
	if d.Classification.Category != CategoryBackhaul {
		t.Fatalf("got %q, want BackhaulLink", d.Classification.Category)
	}
	if d.Classification.Confidence != 45 {
		t.Fatalf("confidence = %d, want 45", d.Classification.Confidence)
	}
	if !hasEvidenceHeuristic(d, "weak_name_pattern") {
		t.Fatal("missing weak evidence")
	}
}

func TestWeak_BackhaulLink_RocketMatches(t *testing.T) {
	d := &DiscoveredDevice{Name: "Rocket-Tepe"}
	Classify(d)
	if d.Classification.Category != CategoryBackhaul {
		t.Fatalf("got %q, want BackhaulLink", d.Classification.Category)
	}
}

func TestWeak_Bridge_TrailingBridgeMatches(t *testing.T) {
	d := &DiscoveredDevice{Name: "merkez-bridge"}
	Classify(d)
	if d.Classification.Category != CategoryBridge {
		t.Fatalf("got %q, want Bridge", d.Classification.Category)
	}
	if d.Classification.Confidence != 45 {
		t.Fatalf("confidence = %d, want 45", d.Classification.Confidence)
	}
}

func TestWeak_CPE_KonutMatchesViaFallback(t *testing.T) {
	d := &DiscoveredDevice{Name: "Konut-902"}
	Classify(d)
	if d.Classification.Category != CategoryCPE {
		t.Fatalf("got %q, want CPE", d.Classification.Category)
	}
	if d.Classification.Confidence != 45 {
		t.Fatalf("confidence = %d, want 45", d.Classification.Confidence)
	}
}

func TestWeak_CPE_HomeMatchesViaFallback(t *testing.T) {
	d := &DiscoveredDevice{Name: "Home-Saglik"}
	Classify(d)
	if d.Classification.Category != CategoryCPE {
		t.Fatalf("got %q, want CPE", d.Classification.Category)
	}
}

func TestWeak_Router_AggMatchesViaFallback(t *testing.T) {
	d := &DiscoveredDevice{Name: "Agg-Pop-Anamur"}
	Classify(d)
	if d.Classification.Category != CategoryRouter {
		t.Fatalf("got %q, want Router", d.Classification.Category)
	}
	if d.Classification.Confidence != 45 {
		t.Fatalf("confidence = %d, want 45", d.Classification.Confidence)
	}
}

func TestWeak_Router_RbMatchesViaFallback(t *testing.T) {
	d := &DiscoveredDevice{Name: "Rb-Anamur"}
	Classify(d)
	if d.Classification.Category != CategoryRouter {
		t.Fatalf("got %q, want Router", d.Classification.Category)
	}
}

func TestWeak_Ambiguous_KuleMimosaRemainsUnknown(t *testing.T) {
	d := &DiscoveredDevice{Name: "Kule-Mimosa"}
	Classify(d)
	if d.Classification.Category != CategoryUnknown {
		t.Fatalf("ambiguous (kule+mimosa) should stay Unknown, got %q", d.Classification.Category)
	}
	if d.Classification.Confidence != 0 {
		t.Fatalf("ambiguous confidence = %d, want 0", d.Classification.Confidence)
	}
}

func TestWeak_Ambiguous_OmniKonutRemainsUnknown(t *testing.T) {
	d := &DiscoveredDevice{Name: "Omni-Konut"}
	Classify(d)
	if d.Classification.Category != CategoryUnknown {
		t.Fatalf("ambiguous (omni+konut) should stay Unknown, got %q", d.Classification.Category)
	}
}

func TestWeak_NoMatch_StaysUnknown(t *testing.T) {
	d := &DiscoveredDevice{Name: "300-OREN"}
	Classify(d)
	if d.Classification.Category != CategoryUnknown {
		t.Fatalf("no pattern token should stay Unknown, got %q", d.Classification.Category)
	}
}

func TestWeak_EmptyName_StaysUnknown(t *testing.T) {
	d := &DiscoveredDevice{Name: ""}
	Classify(d)
	if d.Classification.Category != CategoryUnknown {
		t.Fatalf("empty name should stay Unknown, got %q", d.Classification.Category)
	}
}

func TestWeak_StrongEvidenceOverridesWeak_DudeTypeRouter(t *testing.T) {
	d := &DiscoveredDevice{
		Name: "Kule-12", Type: "router",
		MAC: "00:11:22:33:44:55", Platform: "MikroTik",
	}
	Classify(d)
	if d.Classification.Category != CategoryRouter {
		t.Fatalf("strong type=router should override weak AP, got %q", d.Classification.Category)
	}
	if hasEvidenceHeuristic(d, "weak_name_pattern") {
		t.Fatal("weak should not fire when strong already classified")
	}
	if d.Classification.Confidence < 50 {
		t.Fatalf("strong confidence should exceed 50, got %d", d.Classification.Confidence)
	}
}

func TestWeak_StrongEvidenceOverridesWeak_WirelessModeAP(t *testing.T) {
	d := &DiscoveredDevice{
		Name: "Konut-Saglik",
		Raw:  map[string]string{"wireless-mode": "ap-bridge"},
		MAC:  "00:aa:bb:cc:dd:ee", Platform: "MikroTik",
	}
	Classify(d)
	if d.Classification.Category != CategoryAP {
		t.Fatalf("strong wireless ap-bridge should win, got %q", d.Classification.Category)
	}
	if hasEvidenceHeuristic(d, "weak_name_pattern") {
		t.Fatal("weak should not fire when strong AP signal present")
	}
}

func TestWeak_ConfidenceCapEnforced(t *testing.T) {
	cases := []struct {
		name string
		want Category
	}{
		{"kule-12", CategoryAP},
		{"omni-merkez", CategoryAP},
		{"mimosa-koy", CategoryBackhaul},
		{"konut-anamur", CategoryCPE},
		{"home-12", CategoryCPE},
		{"agg-tepe", CategoryRouter},
		{"rb-anamur", CategoryRouter},
	}
	for _, tc := range cases {
		d := &DiscoveredDevice{Name: tc.name}
		Classify(d)
		if d.Classification.Category != tc.want {
			t.Errorf("name=%q got %q want %q", tc.name, d.Classification.Category, tc.want)
		}
		if d.Classification.Category != CategoryUnknown {
			if d.Classification.Confidence != 45 {
				t.Errorf("name=%q weak confidence = %d, want 45", tc.name, d.Classification.Confidence)
			}
			if d.Classification.Confidence > 50 {
				t.Errorf("name=%q exceeds cap: %d", tc.name, d.Classification.Confidence)
			}
		}
	}
}

func TestWeak_EvidenceRowExposesHeuristicAndReason(t *testing.T) {
	d := &DiscoveredDevice{Name: "kule-iskele"}
	Classify(d)
	var weakRow *Evidence
	for i := range d.Classification.Evidences {
		if d.Classification.Evidences[i].Heuristic == "weak_name_pattern" {
			weakRow = &d.Classification.Evidences[i]
			break
		}
	}
	if weakRow == nil {
		t.Fatal("weak_name_pattern evidence row missing")
	}
	if weakRow.Weight != 45 {
		t.Fatalf("evidence weight = %d, want 45", weakRow.Weight)
	}
	if !strings.Contains(weakRow.Reason, "kule") {
		t.Errorf("reason should mention 'kule', got %q", weakRow.Reason)
	}
	if !strings.Contains(weakRow.Reason, "Confidence 50") {
		t.Errorf("reason should explain cap, got %q", weakRow.Reason)
	}
}

func TestWeak_TokenBoundary_TapDoesNotMatchAP(t *testing.T) {
	// Critical false-positive guard for the WEAK tier: "tap" tokeni
	// "ap" token'ına eşleşmemeli. Mevcut Classify "ap-" substring
	// match'i yapar — bu yüzden "tap-merkez" örneği kullanmıyoruz;
	// onun yerine ayrı (boşluk veya nokta ile) "tap" tokenını
	// yalnızlaştırıp weak tier'ın yanlış pozitif vermediğini
	// kanıtlıyoruz.
	d := &DiscoveredDevice{Name: "tap.merkez"}
	Classify(d)
	if d.Classification.Category == CategoryAP {
		t.Fatalf("'tap' should not match AP via weak; got %q", d.Classification.Category)
	}
}

func TestWeak_TokenWithDigitSuffix_AP1Matches(t *testing.T) {
	d := &DiscoveredDevice{Name: "ap1-anamur"}
	Classify(d)
	if d.Classification.Category != CategoryAP {
		t.Fatalf("'ap1' should match AP via digit-suffix, got %q", d.Classification.Category)
	}
}

func TestWeak_PrimaryNameHintAlreadyClassifies_WeakStaysOff(t *testing.T) {
	d := &DiscoveredDevice{Name: "AP-Sahil-1"}
	Classify(d)
	if d.Classification.Category != CategoryAP {
		t.Fatalf("AP-Sahil-1 should classify AP via primary, got %q", d.Classification.Category)
	}
	if hasEvidenceHeuristic(d, "weak_name_pattern") {
		t.Fatal("weak should not fire when primary already classified")
	}
}

func TestWeak_LowConfidencePrimaryStillWeakBucket(t *testing.T) {
	d := &DiscoveredDevice{Name: "Cpe-12"}
	Classify(d)
	if d.Classification.Category != CategoryCPE {
		t.Fatalf("got %q, want CPE", d.Classification.Category)
	}
	if d.Classification.Confidence >= 50 {
		t.Fatalf("name-only confidence should stay <50 for weak bucket, got %d", d.Classification.Confidence)
	}
}

func TestWeak_AP_OmnTokenMatchesViaFallback(t *testing.T) {
	// R3 tuning: lab data shows "OMN" / "OMN2" suffix is operator's
	// shorthand for an AP/omni device (12 devices observed in the
	// 194.15.45.62 dataset). Adding "omn" to the AP token list.
	d := &DiscoveredDevice{Name: "596_KADILAR_OMN"}
	Classify(d)
	if d.Classification.Category != CategoryAP {
		t.Fatalf("got %q, want AP (omn token)", d.Classification.Category)
	}
	if d.Classification.Confidence != 45 {
		t.Fatalf("confidence = %d, want 45", d.Classification.Confidence)
	}
}

func hasEvidenceHeuristic(d *DiscoveredDevice, h string) bool {
	for _, e := range d.Classification.Evidences {
		if e.Heuristic == h {
			return true
		}
	}
	return false
}
