package http

// Phase R1 — Unit tests for the device-evidence pure helpers.
//
// We deliberately keep these tests hermetic (no DB) and focused on
// the operator-facing contract: missing-signal explanations,
// classification summary, and per-action applicability hints. The
// handler-level wiring is exercised by the existing handler tests
// once a database is attached; these tests guarantee the pure logic
// stays honest.

import (
	"strings"
	"testing"

	"github.com/wisp-ops-center/wisp-ops-center/internal/dude"
	"github.com/wisp-ops-center/wisp-ops-center/internal/networkinv"
)

func TestSummarizeEvidence_EmptyReturnsUnknownWinner(t *testing.T) {
	got := summarizeEvidence(nil)
	if got.Winner != string(dude.CategoryUnknown) {
		t.Fatalf("empty summary winner = %q, want Unknown", got.Winner)
	}
	if got.TotalRows != 0 {
		t.Fatalf("empty summary TotalRows = %d, want 0", got.TotalRows)
	}
	if got.WinnerWeight != 0 {
		t.Fatalf("empty summary WinnerWeight = %d, want 0", got.WinnerWeight)
	}
}

func TestSummarizeEvidence_PicksHighestCategory(t *testing.T) {
	rows := []evidenceRow{
		{Heuristic: "name_pattern", Category: "AP", Weight: 30, Reason: "name has -AP-"},
		{Heuristic: "neighbor_mac", Category: "AP", Weight: 10, Reason: "MAC family"},
		{Heuristic: "platform", Category: "Router", Weight: 25, Reason: "platform RouterOS"},
		{Heuristic: "platform", Category: "Router", Weight: 5, Reason: "version >=7"},
	}
	got := summarizeEvidence(rows)
	if got.Winner != "AP" {
		t.Fatalf("winner = %q, want AP", got.Winner)
	}
	if got.WinnerWeight != 40 {
		t.Fatalf("winner_weight = %d, want 40", got.WinnerWeight)
	}
	if got.RunnerUp != "Router" {
		t.Fatalf("runner_up = %q, want Router", got.RunnerUp)
	}
	if got.RunnerUpWeight != 30 {
		t.Fatalf("runner_up_weight = %d, want 30", got.RunnerUpWeight)
	}
	if len(got.UniqueHeuristics) != 3 {
		t.Fatalf("unique heuristics = %d, want 3", len(got.UniqueHeuristics))
	}
}

func TestDeriveMissingSignals_AllAbsentProducesFiveNotes(t *testing.T) {
	d := &networkinv.Device{}
	got := deriveMissingSignals(d)
	if len(got) != 5 {
		t.Fatalf("missing signals count = %d, want 5 (mac+platform+board+iface+evidence_summary)", len(got))
	}
	want := map[string]bool{
		"mac": false, "neighbor_platform": false, "board": false,
		"interface_name": false, "evidence_summary": false,
	}
	for _, m := range got {
		if _, ok := want[m.Signal]; !ok {
			t.Errorf("unexpected signal %q", m.Signal)
		}
		want[m.Signal] = true
		if m.Explanation == "" {
			t.Errorf("signal %q has empty Explanation", m.Signal)
		}
		if m.WouldHelp == "" {
			t.Errorf("signal %q has empty WouldHelp", m.Signal)
		}
	}
	for sig, seen := range want {
		if !seen {
			t.Errorf("expected signal %q not present", sig)
		}
	}
}

func TestDeriveMissingSignals_FullDeviceProducesNoNotes(t *testing.T) {
	d := &networkinv.Device{
		MAC:             "AA:BB:CC:DD:EE:FF",
		Platform:        "MikroTik",
		Board:           "RB951",
		InterfaceName:   "ether1",
		EvidenceSummary: "name+neighbor",
	}
	got := deriveMissingSignals(d)
	if len(got) != 0 {
		t.Fatalf("fully-enriched device produced %d missing signals, want 0", len(got))
	}
}

func TestDeriveActionApplicability_APDeviceLikesFreqAndAPClient(t *testing.T) {
	d := &networkinv.Device{Category: dude.CategoryAP, Confidence: 80}
	apps := deriveActionApplicability(d)
	if len(apps) != 4 {
		t.Fatalf("apps count = %d, want 4", len(apps))
	}
	for _, a := range apps {
		switch a.Kind {
		case "frequency_check", "ap_client_test":
			if a.Applicable != "likely_yes" {
				t.Errorf("%s applicable = %q, want likely_yes", a.Kind, a.Applicable)
			}
		case "link_signal_test", "bridge_health_check":
			if a.Applicable == "likely_yes" {
				t.Errorf("%s applicable = likely_yes for AP device — should not be", a.Kind)
			}
		}
		if a.SafetyStatus != "read_only_dry_run" {
			t.Errorf("%s safety = %q, want read_only_dry_run", a.Kind, a.SafetyStatus)
		}
	}
}

func TestDeriveActionApplicability_BridgeDeviceLikesBridgeHealthOnly(t *testing.T) {
	d := &networkinv.Device{Category: dude.CategoryBridge, Confidence: 80}
	apps := deriveActionApplicability(d)
	for _, a := range apps {
		if a.Kind == "bridge_health_check" && a.Applicable != "likely_yes" {
			t.Errorf("bridge_health_check applicable = %q, want likely_yes", a.Applicable)
		}
		if a.Kind == "frequency_check" && a.Applicable != "likely_no" {
			t.Errorf("frequency_check on Bridge should be likely_no, got %q", a.Applicable)
		}
	}
}

func TestDeriveActionApplicability_LowConfidenceDowngradesToUnknown(t *testing.T) {
	// AP-classified but confidence<50 → behave like Unknown.
	d := &networkinv.Device{Category: dude.CategoryAP, Confidence: 30}
	apps := deriveActionApplicability(d)
	for _, a := range apps {
		if a.Applicable == "likely_yes" {
			t.Errorf("low-confidence device produced likely_yes for %s — should be unknown", a.Kind)
		}
	}
}

func TestDeriveActionApplicability_UnknownDeviceReturnsUnknownWithReason(t *testing.T) {
	d := &networkinv.Device{Category: dude.CategoryUnknown, Confidence: 0}
	apps := deriveActionApplicability(d)
	for _, a := range apps {
		if a.Applicable != "unknown" {
			t.Errorf("%s applicable = %q, want unknown", a.Kind, a.Applicable)
		}
		if !strings.Contains(a.Reason, "Bilinmeyen") {
			t.Errorf("%s reason should mention Bilinmeyen, got %q", a.Kind, a.Reason)
		}
	}
}

func TestExtractDeviceIDForEvidence_HappyPath(t *testing.T) {
	id := "11111111-2222-3333-4444-555555555555"
	got := extractDeviceIDForEvidence("/api/v1/network/devices/" + id + "/evidence")
	if got != id {
		t.Fatalf("got %q, want %q", got, id)
	}
}

func TestExtractDeviceIDForEvidence_TrailingSlash(t *testing.T) {
	id := "11111111-2222-3333-4444-555555555555"
	got := extractDeviceIDForEvidence("/api/v1/network/devices/" + id + "/evidence/")
	if got != id {
		t.Fatalf("got %q, want %q", got, id)
	}
}

func TestExtractDeviceIDForEvidence_RejectsNestedPath(t *testing.T) {
	got := extractDeviceIDForEvidence("/api/v1/network/devices/foo/bar/evidence")
	if got != "" {
		t.Fatalf("nested path should yield empty, got %q", got)
	}
}

func TestRound1(t *testing.T) {
	tests := []struct {
		in   float64
		want float64
	}{
		{0, 0},
		{1.0, 1.0},
		{99.96, 100.0},
		{99.94, 99.9},
		{99.95, 100.0}, // round half up
		{50.05, 50.1},
	}
	for _, tc := range tests {
		got := round1(tc.in)
		if got != tc.want {
			t.Errorf("round1(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// TestApplicabilityFor_HitsHotPath exercises the matrix tail to make
// sure changes to the disqualifying list keep matching cleanly.
func TestApplicabilityFor_HitsHotPath(t *testing.T) {
	cases := []struct {
		deviceCat string
		expected  string
		want      string
	}{
		{"AP", "AP", "likely_yes"},
		{"Bridge", "AP", "likely_no"},
		{"Router", "BackhaulLink", "likely_no"},
		{"BackhaulLink", "BackhaulLink", "likely_yes"},
		{"CPE", "Bridge", "likely_no"},
		{"Switch", "BackhaulLink", "likely_no"},
		{"Unknown", "AP", "unknown"},
	}
	for _, c := range cases {
		got := applicabilityFor("kind", "kind", "Label", c.deviceCat,
			c.expected, "Bridge,Switch,Router,CPE,BackhaulLink,AP")
		// strip the matched expected from the disqualifying list to
		// avoid false positives.
		dq := strings.ReplaceAll(",Bridge,Switch,Router,CPE,BackhaulLink,AP,",
			","+c.expected+",", ",")
		got = applicabilityFor("kind", "kind", "Label", c.deviceCat,
			c.expected, strings.Trim(dq, ","))
		if got.Applicable != c.want {
			t.Errorf("dev=%s exp=%s got=%s, want=%s", c.deviceCat, c.expected, got.Applicable, c.want)
		}
	}
}
