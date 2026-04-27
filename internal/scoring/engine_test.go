package scoring

import (
	"math"
	"testing"
	"time"
)

func ptr[T any](v T) *T { return &v }

func mkEngine() *Engine {
	now, _ := time.Parse(time.RFC3339, "2026-04-27T12:00:00Z")
	return NewEngine(DefaultThresholds()).WithClock(func() time.Time { return now })
}

func TestScoreCustomer_Healthy(t *testing.T) {
	e := mkEngine()
	now := e.clock()
	r := e.ScoreCustomer(Inputs{
		RSSIdBm:       ptr(-60.0),
		SNRdB:         ptr(35.0),
		AvgLatencyMs:  ptr(15.0),
		PacketLossPct: ptr(0.0),
		JitterMs:      ptr(2.0),
		LastSampleAt:  ptr(now.Add(-1 * time.Minute)),
	})
	if r.Score < 80 {
		t.Fatalf("expected healthy >=80, got %d", r.Score)
	}
	if r.Severity != SeverityHealthy {
		t.Fatalf("expected healthy severity, got %s", r.Severity)
	}
	if r.Diagnosis != DiagHealthy {
		t.Fatalf("expected healthy diagnosis, got %s", r.Diagnosis)
	}
	if r.RecommendedAction != ActionNoAction {
		t.Fatalf("expected no_action, got %s", r.RecommendedAction)
	}
}

func TestScoreCustomer_DataInsufficient(t *testing.T) {
	e := mkEngine()
	r := e.ScoreCustomer(Inputs{})
	if r.Diagnosis != DiagDataInsufficient {
		t.Fatalf("expected data_insufficient, got %s", r.Diagnosis)
	}
	if r.Severity != SeverityUnknown {
		t.Fatalf("expected unknown severity, got %s", r.Severity)
	}
}

func TestScoreCustomer_WeakSignal(t *testing.T) {
	e := mkEngine()
	now := e.clock()
	r := e.ScoreCustomer(Inputs{
		RSSIdBm:      ptr(-85.0),
		SNRdB:        ptr(20.0),
		LastSampleAt: ptr(now.Add(-2 * time.Minute)),
	})
	if r.Severity != SeverityCritical && r.Severity != SeverityWarning {
		t.Fatalf("expected non-healthy severity, got %s", r.Severity)
	}
	if r.Diagnosis != DiagWeakCustomerSignal {
		t.Fatalf("expected weak_customer_signal, got %s", r.Diagnosis)
	}
}

func TestScoreCustomer_CPEAlignment(t *testing.T) {
	e := mkEngine()
	now := e.clock()
	r := e.ScoreCustomer(Inputs{
		RSSIdBm:      ptr(-65.0),
		SNRdB:        ptr(10.0), // < critical 15
		LastSampleAt: ptr(now.Add(-1 * time.Minute)),
	})
	if r.Diagnosis != DiagCPEAlignmentIssue {
		t.Fatalf("expected possible_cpe_alignment_issue, got %s", r.Diagnosis)
	}
}

func TestScoreCustomer_HighLatency(t *testing.T) {
	e := mkEngine()
	now := e.clock()
	r := e.ScoreCustomer(Inputs{
		RSSIdBm:       ptr(-65.0),
		SNRdB:         ptr(30.0),
		AvgLatencyMs:  ptr(250.0),
		PacketLossPct: ptr(3.0), // ek penalty ile severity warning'e iter
		LastTestAt:    ptr(now.Add(-1 * time.Minute)),
	})
	if r.Diagnosis != DiagHighLatency && r.Diagnosis != DiagPacketLoss {
		t.Fatalf("expected high_latency or packet_loss, got %s", r.Diagnosis)
	}
	if r.Severity != SeverityWarning && r.Severity != SeverityCritical {
		t.Fatalf("expected non-healthy severity, got %s (score=%d)", r.Severity, r.Score)
	}
}

func TestScoreCustomer_PacketLoss(t *testing.T) {
	e := mkEngine()
	now := e.clock()
	r := e.ScoreCustomer(Inputs{
		RSSIdBm:       ptr(-65.0),
		SNRdB:         ptr(30.0),
		PacketLossPct: ptr(8.0),
		LastTestAt:    ptr(now.Add(-1 * time.Minute)),
	})
	if r.Diagnosis != DiagPacketLoss {
		t.Fatalf("expected packet_loss, got %s", r.Diagnosis)
	}
}

func TestScoreCustomer_UnstableJitter(t *testing.T) {
	e := mkEngine()
	now := e.clock()
	r := e.ScoreCustomer(Inputs{
		RSSIdBm:    ptr(-65.0),
		SNRdB:      ptr(30.0),
		JitterMs:   ptr(35.0),
		LastTestAt: ptr(now.Add(-1 * time.Minute)),
	})
	if r.Diagnosis != DiagUnstableJitter {
		t.Fatalf("expected unstable_jitter, got %s", r.Diagnosis)
	}
}

func TestScoreCustomer_StaleData(t *testing.T) {
	e := mkEngine()
	now := e.clock()
	r := e.ScoreCustomer(Inputs{
		RSSIdBm:      ptr(-60.0),
		SNRdB:        ptr(30.0),
		LastSampleAt: ptr(now.Add(-3 * time.Hour)),
	})
	if r.Diagnosis != DiagStaleData {
		t.Fatalf("expected stale_data, got %s", r.Diagnosis)
	}
	if !r.IsStale {
		t.Fatalf("expected IsStale=true")
	}
}

func TestScoreCustomer_DeviceOffline(t *testing.T) {
	e := mkEngine()
	now := e.clock()
	r := e.ScoreCustomer(Inputs{
		LastTestSuccess: ptr(false),
		AvgLatencyMs:    ptr(0.0), // teste girdi var ama RSSI/SNR yok
		LastTestAt:      ptr(now.Add(-1 * time.Minute)),
	})
	if r.Diagnosis != DiagDeviceOffline {
		t.Fatalf("expected device_offline, got %s", r.Diagnosis)
	}
}

func TestScoreCustomer_APWideInterference(t *testing.T) {
	e := mkEngine()
	now := e.clock()
	r := e.ScoreCustomer(Inputs{
		RSSIdBm:               ptr(-72.0),
		SNRdB:                 ptr(28.0),
		APWideCustomerCount:   ptr(10),
		APWideDegradedCustCnt: ptr(5), // 50% > 40% critical
		LastSampleAt:          ptr(now.Add(-1 * time.Minute)),
	})
	if r.Diagnosis != DiagAPWideInterference {
		t.Fatalf("expected ap_wide_interference, got %s", r.Diagnosis)
	}
}

func TestScoreCustomer_FrequencyChannelRisk(t *testing.T) {
	e := mkEngine()
	now := e.clock()
	r := e.ScoreCustomer(Inputs{
		RSSIdBm:      ptr(-65.0),
		SNRdB:        ptr(20.0), // 15 ≤ x < 25 → frekans riski
		LastSampleAt: ptr(now.Add(-1 * time.Minute)),
	})
	if r.Diagnosis != DiagFrequencyChannelRisk {
		t.Fatalf("expected frequency_channel_risk, got %s", r.Diagnosis)
	}
}

func TestScoreCustomer_PTPLink(t *testing.T) {
	e := mkEngine()
	now := e.clock()
	r := e.ScoreCustomer(Inputs{
		RSSIdBm:           ptr(-65.0),
		SNRdB:             ptr(28.0),
		LinkCapacityRatio: ptr(0.92),
		LastSampleAt:      ptr(now.Add(-1 * time.Minute)),
	})
	if r.Diagnosis != DiagPTPLinkDegradation {
		t.Fatalf("expected ptp_link_degradation, got %s", r.Diagnosis)
	}
}

func TestSeverityBoundaries(t *testing.T) {
	thr := DefaultThresholds()
	cases := []struct {
		score int
		want  Severity
	}{
		{100, SeverityHealthy},
		{80, SeverityHealthy},
		{79, SeverityWarning},
		{50, SeverityWarning},
		{49, SeverityCritical},
		{0, SeverityCritical},
		{-1, SeverityUnknown},
	}
	for _, c := range cases {
		got := thr.SeverityFromScore(c.score)
		if got != c.want {
			t.Errorf("score %d: want %s got %s", c.score, c.want, got)
		}
	}
}

func TestSignalTrend7d(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-04-27T12:00:00Z")
	// 7 gün boyunca her gün -1 dB düşen sinyal → eğim ≈ -1 dB/gün
	samples := []SignalSample{}
	for i := 0; i < 7; i++ {
		samples = append(samples, SignalSample{
			At:      now.Add(-time.Duration(7-i) * 24 * time.Hour),
			RSSIdBm: -60.0 - float64(i),
		})
	}
	slope := SignalTrend7d(samples, now)
	if slope == nil {
		t.Fatal("expected non-nil slope")
	}
	if math.Abs(*slope-(-1.0)) > 0.1 {
		t.Errorf("expected ~-1 dB/day, got %.3f", *slope)
	}
}

func TestSignalTrend7d_TooFewSamples(t *testing.T) {
	now := time.Now()
	samples := []SignalSample{
		{At: now.Add(-1 * time.Hour), RSSIdBm: -60},
		{At: now, RSSIdBm: -61},
	}
	if SignalTrend7d(samples, now) != nil {
		t.Fatal("expected nil for too few samples")
	}
}

func TestThresholdOverrides(t *testing.T) {
	thr := DefaultThresholds()
	overrides := map[string]float64{
		"rssi_critical_dbm":   -75,
		"latency_critical_ms": 200,
		"severity_healthy_at": 90,
	}
	out := thr.ApplyOverrides(overrides)
	if out.RSSICriticalDbm != -75 {
		t.Errorf("rssi_critical_dbm not applied: %v", out.RSSICriticalDbm)
	}
	if out.LatencyCriticalMs != 200 {
		t.Errorf("latency_critical_ms not applied: %v", out.LatencyCriticalMs)
	}
	if out.HealthyAt != 90 {
		t.Errorf("severity_healthy_at not applied: %v", out.HealthyAt)
	}
}

func TestSeedDefaults_AllKeysCovered(t *testing.T) {
	defaults := SeedDefaults()
	if len(defaults) < 15 {
		t.Errorf("expected at least 15 seed defaults, got %d", len(defaults))
	}
	required := []string{
		"rssi_critical_dbm", "rssi_warning_dbm",
		"snr_critical_db", "snr_warning_db",
		"ccq_critical_percent", "ccq_warning_percent",
		"packet_loss_critical_percent", "packet_loss_warning_percent",
		"latency_critical_ms", "latency_warning_ms",
		"jitter_critical_ms", "jitter_warning_ms",
		"stale_data_minutes",
		"ap_degradation_customer_ratio_warning",
		"ap_degradation_customer_ratio_critical",
	}
	keys := map[string]bool{}
	for _, d := range defaults {
		keys[d.Key] = true
	}
	for _, k := range required {
		if !keys[k] {
			t.Errorf("required default key missing: %s", k)
		}
	}
}

func TestActionFor_AllDiagnoses(t *testing.T) {
	all := []Diagnosis{
		DiagHealthy, DiagWeakCustomerSignal, DiagCPEAlignmentIssue,
		DiagAPWideInterference, DiagPTPLinkDegradation, DiagFrequencyChannelRisk,
		DiagHighLatency, DiagPacketLoss, DiagUnstableJitter,
		DiagDeviceOffline, DiagStaleData, DiagDataInsufficient,
	}
	for _, d := range all {
		a := ActionFor(d, SeverityWarning)
		if a == "" {
			t.Errorf("no action for diagnosis %s", d)
		}
	}
}

func TestPeerSetAnalysis_Critical(t *testing.T) {
	thr := DefaultThresholds()
	peers := PeerSet{
		APDeviceID: "ap-1",
		Customers: []PeerCustomer{
			{CustomerID: "c1", Score: ptr(20)}, // critical
			{CustomerID: "c2", Score: ptr(30)}, // critical
			{CustomerID: "c3", Score: ptr(40)}, // critical
			{CustomerID: "c4", Score: ptr(60)}, // warning
			{CustomerID: "c5", Score: ptr(95)}, // healthy
		},
	}
	stats := thr.AnalyzePeerSet(peers)
	if stats.CriticalCustomers != 3 {
		t.Errorf("expected 3 critical, got %d", stats.CriticalCustomers)
	}
	if !stats.IsCritical {
		t.Errorf("expected IsCritical=true (ratio=%.2f)", stats.DegradationRatio)
	}
}

func TestScoreAP(t *testing.T) {
	e := mkEngine()
	res := e.ScoreAP(APInputs{
		APDeviceID:            "ap-1",
		CustomerScores:        []int{30, 35, 40, 60, 95},
		CriticalCustomerCount: 3,
		HasFreshTelemetry:     true,
	})
	if res.APDeviceID != "ap-1" {
		t.Errorf("device id not preserved")
	}
	if !res.IsAPWideInterference {
		t.Errorf("expected IsAPWideInterference=true (ratio=%.2f)", res.DegradationRatio)
	}
}

func TestScoreLink(t *testing.T) {
	e := mkEngine()
	res := e.ScoreLink(LinkInputs{
		LinkID:            "link-1",
		SignalA:           ptr(-58.0),
		SignalB:           ptr(-60.0),
		SNRA:              ptr(30.0),
		SNRB:              ptr(28.0),
		CapacityRatio:     ptr(0.30),
		HasFreshTelemetry: true,
	})
	if res.Score < 80 {
		t.Errorf("expected healthy link score, got %d", res.Score)
	}
}

func TestScoreLink_Degraded(t *testing.T) {
	e := mkEngine()
	res := e.ScoreLink(LinkInputs{
		LinkID:            "link-2",
		SignalA:           ptr(-82.0),
		SignalB:           ptr(-78.0),
		SNRA:              ptr(12.0),
		SNRB:              ptr(18.0),
		CapacityRatio:     ptr(0.95),
		LossPctA:          ptr(8.0),
		LossPctB:          ptr(3.0),
		HasFreshTelemetry: true,
	})
	if res.Severity != SeverityCritical {
		t.Errorf("expected critical, got %s", res.Severity)
	}
	if res.Diagnosis != DiagPTPLinkDegradation {
		t.Errorf("expected ptp_link_degradation, got %s", res.Diagnosis)
	}
}

func TestScoreTower(t *testing.T) {
	e := mkEngine()
	res := e.ScoreTower(TowerInputs{
		TowerID: "tower-1",
		APResults: []APResult{
			{APDeviceID: "ap-1", APScore: 30, IsAPWideInterference: true},
			{APDeviceID: "ap-2", APScore: 80},
		},
		LinkResults: []LinkResult{
			{LinkID: "link-1", Score: 60, Severity: SeverityWarning},
		},
		CustomerCriticalCount: 8,
		CustomerTotal:         20,
	})
	if res.TowerID != "tower-1" {
		t.Errorf("tower id not preserved")
	}
	if res.RiskScore <= 0 {
		t.Errorf("expected positive risk score, got %d", res.RiskScore)
	}
}

func TestBackwardCompatibleScore(t *testing.T) {
	// Faz 1 imzasının hala çalıştığını doğrula
	r := Score(Inputs{
		RSSIdBm: ptr(-60.0),
		SNRdB:   ptr(35.0),
	})
	if r.Score == 0 {
		t.Errorf("backward compat Score returned 0 for healthy inputs")
	}
}
