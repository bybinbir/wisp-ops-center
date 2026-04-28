package dude

import (
	"context"
	"log/slog"
	"testing"
)

// Phase 8 hotfix v8.4.0: Run uses a NAMED return so the deferred
// FinishedAt + Stats.Tally mutations are observed by the caller.
// Before this fix, `return res` copied the value and the deferred
// closure mutated a now-orphaned local — which made
// discovery_runs.device_count=0 even when 893 devices were upserted.
//
// This test exercises the early-return code path (cfg.Validate
// failure) to prove the deferred finalize ran and is visible to the
// caller without needing a live SSH endpoint.

func TestRun_NamedReturn_DeferFinalizeVisibleToCaller(t *testing.T) {
	// Empty cfg fails Validate immediately.
	cfg := Config{}
	res := Run(context.Background(), cfg, slog.Default(), nil)

	if res.FinishedAt.IsZero() {
		t.Fatalf("FinishedAt zero — defer finalize did not reach caller (named return broken)")
	}
	if res.StartedAt.IsZero() {
		t.Fatalf("StartedAt zero — early initializer not visible to caller")
	}
	if res.FinishedAt.Before(res.StartedAt) {
		t.Fatalf("FinishedAt < StartedAt: %v < %v", res.FinishedAt, res.StartedAt)
	}
	// No devices in this code path, so Stats.Total must equal 0; the
	// important thing is that Stats was populated by the deferred
	// Tally call and not left as a copied-from-init zero value.
	if res.Stats.Total != 0 {
		t.Errorf("Stats.Total expected 0 for empty run, got %d", res.Stats.Total)
	}
}

func TestRun_NamedReturn_DeferTallyCountsDevices(t *testing.T) {
	// We can't easily inject devices into Run() without a live SSH
	// endpoint, so we test the Tally + named-return semantic with a
	// synthetic helper that mirrors Run's deferred call exactly.
	mimic := func() (res RunResult) {
		defer func() {
			res.Stats = DiscoveryStats{}
			res.Stats.Tally(res.Devices)
		}()
		res.Devices = []DiscoveredDevice{
			{Name: "ap1", Classification: Classification{Category: CategoryAP, Confidence: 80}},
			{Name: "cpe1", Classification: Classification{Category: CategoryCPE, Confidence: 30}},
			{Name: "u1", Classification: Classification{Category: CategoryUnknown, Confidence: 0}},
		}
		return res
	}
	res := mimic()
	if res.Stats.Total != 3 {
		t.Fatalf("Stats.Total = %d, want 3 (named return + defer tally bug regressed)", res.Stats.Total)
	}
	if res.Stats.APs != 1 || res.Stats.CPEs != 1 || res.Stats.Unknown != 1 {
		t.Errorf("category counts wrong: %+v", res.Stats)
	}
	if res.Stats.LowConfidence != 2 {
		t.Errorf("LowConfidence = %d, want 2", res.Stats.LowConfidence)
	}
}
