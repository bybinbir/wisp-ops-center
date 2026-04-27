package scoring

import "testing"

func TestIsKnownThresholdKey(t *testing.T) {
	known := []string{
		"rssi_critical_dbm", "rssi_warning_dbm",
		"snr_critical_db", "snr_warning_db",
		"ccq_critical_percent", "ccq_warning_percent",
		"packet_loss_critical_percent", "packet_loss_warning_percent",
		"latency_critical_ms", "latency_warning_ms",
		"jitter_critical_ms", "jitter_warning_ms",
		"stale_data_minutes",
		"ap_degradation_customer_ratio_warning",
		"ap_degradation_customer_ratio_critical",
		"severity_healthy_at", "severity_warning_at",
		// Phase 7
		"work_order_duplicate_cooldown_days",
		"work_order_default_eta_hours",
	}
	for _, k := range known {
		if !IsKnownThresholdKey(k) {
			t.Errorf("expected %q to be known", k)
		}
	}
	if IsKnownThresholdKey("totally_made_up_key") {
		t.Errorf("expected unknown key to be rejected")
	}
}

func TestIsValidThresholdValue(t *testing.T) {
	tests := []struct {
		key   string
		value float64
		want  bool
	}{
		{"rssi_critical_dbm", -80, true},
		{"rssi_critical_dbm", -200, false}, // below min
		{"rssi_critical_dbm", 5, false},    // above max
		{"snr_critical_db", 15, true},
		{"snr_critical_db", 100, false}, // out of range
		{"ccq_critical_percent", 50, true},
		{"ccq_critical_percent", 150, false},
		{"ap_degradation_customer_ratio_critical", 0.40, true},
		{"ap_degradation_customer_ratio_critical", 1.5, false},
		{"stale_data_minutes", 60, true},
		{"stale_data_minutes", 0, false},
		{"work_order_duplicate_cooldown_days", 7, true},
		{"work_order_duplicate_cooldown_days", -1, false},
		{"work_order_duplicate_cooldown_days", 1000, false},
		{"work_order_default_eta_hours", 24, true},
		{"work_order_default_eta_hours", -5, false},
		{"unknown_key", 1, false}, // unknown -> false
	}
	for _, tc := range tests {
		got := IsValidThresholdValue(tc.key, tc.value)
		if got != tc.want {
			t.Errorf("IsValidThresholdValue(%q, %v) = %v, want %v",
				tc.key, tc.value, got, tc.want)
		}
	}
}

func TestApplyOverridesIgnoresUnknown(t *testing.T) {
	d := DefaultThresholds()
	out := d.ApplyOverrides(map[string]float64{
		"rssi_warning_dbm":       -65, // known, applied
		"definitely_not_a_field": 999, // unknown, ignored
	})
	if out.RSSIWarningDbm != -65 {
		t.Errorf("expected RSSIWarningDbm=-65, got %v", out.RSSIWarningDbm)
	}
	if out.RSSICriticalDbm != d.RSSICriticalDbm {
		t.Errorf("unrelated field should not change")
	}
}

func TestSeedDefaultsMatchesDefaultThresholds(t *testing.T) {
	d := DefaultThresholds()
	seeds := SeedDefaults()
	got := map[string]float64{}
	for _, s := range seeds {
		got[s.Key] = s.Value
	}
	if got["rssi_critical_dbm"] != d.RSSICriticalDbm {
		t.Errorf("seed rssi_critical_dbm mismatch: %v vs %v", got["rssi_critical_dbm"], d.RSSICriticalDbm)
	}
	if got["severity_healthy_at"] != float64(d.HealthyAt) {
		t.Errorf("seed severity_healthy_at mismatch")
	}
}
