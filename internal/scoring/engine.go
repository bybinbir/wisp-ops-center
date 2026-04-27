package scoring

import (
	"fmt"
	"math"
	"time"
)

// Clock, test edilebilirlik için zaman kaynağıdır.
type Clock func() time.Time

// Engine, deterministik skor motorunun ana giriş noktasıdır.
type Engine struct {
	thr   Thresholds
	clock Clock
}

// NewEngine, varsayılan saatle bir motor üretir.
func NewEngine(thr Thresholds) *Engine {
	return &Engine{thr: thr, clock: time.Now}
}

// WithClock, test için saati değiştirir.
func (e *Engine) WithClock(c Clock) *Engine {
	if c == nil {
		c = time.Now
	}
	return &Engine{thr: e.thr, clock: c}
}

// Thresholds, motorun kullandığı eşikleri verir.
func (e *Engine) Thresholds() Thresholds { return e.thr }

// ScoreCustomer, tek bir müşteri için skor + tanı + aksiyon üretir.
//
// Algoritma:
//  1. Inputs eksikse data_insufficient sonucu üret.
//  2. score = 100; her metrik için penalty düş.
//  3. score'u 0..100'e clamp et.
//  4. severity = thresholds.SeverityFromScore(score)
//  5. diagnosis = thresholds.Classify(inputs)
//  6. action = ActionFor(diagnosis, severity)
func (e *Engine) ScoreCustomer(in Inputs) Result {
	now := e.clock()
	res := Result{
		Score:               100,
		Severity:            SeverityUnknown,
		Diagnosis:           DiagDataInsufficient,
		RecommendedAction:   ActionMonitor,
		Reasons:             []string{},
		ContributingMetrics: map[string]float64{},
		CalculatedAt:        now,
	}

	// 1) Yetersiz veri
	if in.RSSIdBm == nil && in.SNRdB == nil &&
		in.AvgLatencyMs == nil && in.PacketLossPct == nil && in.JitterMs == nil {
		res.Score = 0
		res.Severity = SeverityUnknown
		res.Diagnosis = DiagDataInsufficient
		res.RecommendedAction = ActionFor(DiagDataInsufficient, SeverityUnknown)
		res.Reasons = []string{"Telemetri veya test sonucu yok"}
		return res
	}

	// 2) Stale data — ayrıca işaretle ama skor hesabını yine de yap
	if e.thr.isStale(in, now) {
		res.IsStale = true
		res.Reasons = append(res.Reasons, "Son ölçüm/test eski (stale)")
	}

	score := 100.0

	// RSSI (dBm)
	if in.RSSIdBm != nil {
		v := *in.RSSIdBm
		switch {
		case v <= e.thr.RSSICriticalDbm:
			pen := 40.0
			score -= pen
			res.Reasons = append(res.Reasons, fmt.Sprintf("RSSI kritik (%.1f dBm ≤ %.0f)", v, e.thr.RSSICriticalDbm))
			res.ContributingMetrics["rssi_dbm"] = -pen
		case v <= e.thr.RSSIWarningDbm:
			pen := 18.0
			score -= pen
			res.Reasons = append(res.Reasons, fmt.Sprintf("RSSI sınırda (%.1f dBm ≤ %.0f)", v, e.thr.RSSIWarningDbm))
			res.ContributingMetrics["rssi_dbm"] = -pen
		}
	}

	// SNR (dB)
	if in.SNRdB != nil {
		v := *in.SNRdB
		switch {
		case v < e.thr.SNRCriticalDb:
			pen := 30.0
			score -= pen
			res.Reasons = append(res.Reasons, fmt.Sprintf("SNR kritik (%.1f < %.0f)", v, e.thr.SNRCriticalDb))
			res.ContributingMetrics["snr_db"] = -pen
		case v < e.thr.SNRWarningDb:
			pen := 12.0
			score -= pen
			res.Reasons = append(res.Reasons, fmt.Sprintf("SNR sınırda (%.1f < %.0f)", v, e.thr.SNRWarningDb))
			res.ContributingMetrics["snr_db"] = -pen
		}
	}

	// CCQ (%)
	if in.CCQ != nil {
		v := *in.CCQ
		switch {
		case v < e.thr.CCQCriticalPercent:
			pen := 12.0
			score -= pen
			res.Reasons = append(res.Reasons, fmt.Sprintf("CCQ kritik (%.0f%% < %.0f)", v, e.thr.CCQCriticalPercent))
			res.ContributingMetrics["ccq"] = -pen
		case v < e.thr.CCQWarningPercent:
			pen := 5.0
			score -= pen
			res.Reasons = append(res.Reasons, fmt.Sprintf("CCQ sınırda (%.0f%% < %.0f)", v, e.thr.CCQWarningPercent))
			res.ContributingMetrics["ccq"] = -pen
		}
	}

	// Paket kaybı (%)
	if in.PacketLossPct != nil {
		v := *in.PacketLossPct
		switch {
		case v >= e.thr.PacketLossCriticalPercent:
			pen := 25.0
			score -= pen
			res.Reasons = append(res.Reasons, fmt.Sprintf("Paket kaybı kritik (%.1f%% ≥ %.0f)", v, e.thr.PacketLossCriticalPercent))
			res.ContributingMetrics["packet_loss_pct"] = -pen
		case v >= e.thr.PacketLossWarningPercent:
			pen := 10.0
			score -= pen
			res.Reasons = append(res.Reasons, fmt.Sprintf("Paket kaybı sınırda (%.1f%% ≥ %.0f)", v, e.thr.PacketLossWarningPercent))
			res.ContributingMetrics["packet_loss_pct"] = -pen
		}
	}

	// Latency (ms)
	if in.AvgLatencyMs != nil {
		v := *in.AvgLatencyMs
		switch {
		case v >= e.thr.LatencyCriticalMs:
			pen := 18.0
			score -= pen
			res.Reasons = append(res.Reasons, fmt.Sprintf("Gecikme kritik (%.0f ms ≥ %.0f)", v, e.thr.LatencyCriticalMs))
			res.ContributingMetrics["avg_latency_ms"] = -pen
		case v >= e.thr.LatencyWarningMs:
			pen := 8.0
			score -= pen
			res.Reasons = append(res.Reasons, fmt.Sprintf("Gecikme sınırda (%.0f ms ≥ %.0f)", v, e.thr.LatencyWarningMs))
			res.ContributingMetrics["avg_latency_ms"] = -pen
		}
	}

	// Jitter (ms)
	if in.JitterMs != nil {
		v := *in.JitterMs
		switch {
		case v >= e.thr.JitterCriticalMs:
			pen := 12.0
			score -= pen
			res.Reasons = append(res.Reasons, fmt.Sprintf("Jitter kritik (%.0f ms ≥ %.0f)", v, e.thr.JitterCriticalMs))
			res.ContributingMetrics["jitter_ms"] = -pen
		case v >= e.thr.JitterWarningMs:
			pen := 5.0
			score -= pen
			res.Reasons = append(res.Reasons, fmt.Sprintf("Jitter sınırda (%.0f ms ≥ %.0f)", v, e.thr.JitterWarningMs))
			res.ContributingMetrics["jitter_ms"] = -pen
		}
	}

	// Disconnect frekansı
	if in.DisconnectsLastDay != nil && *in.DisconnectsLastDay >= 5 {
		pen := 8.0
		score -= pen
		res.Reasons = append(res.Reasons, fmt.Sprintf("Son 24 saatte %d kez bağlantı kesildi", *in.DisconnectsLastDay))
		res.ContributingMetrics["disconnects_last_day"] = -pen
	}

	// Trend
	if in.SignalTrend7d != nil && *in.SignalTrend7d < -0.5 {
		pen := 5.0
		score -= pen
		res.Reasons = append(res.Reasons, fmt.Sprintf("7 günde sinyal düşüyor (%.2f dB/gün)", *in.SignalTrend7d))
		res.ContributingMetrics["signal_trend_7d"] = -pen
	}

	// AP-wide degradation
	if in.APWideCustomerCount != nil && *in.APWideCustomerCount > 0 &&
		in.APWideDegradedCustCnt != nil {
		ratio := float64(*in.APWideDegradedCustCnt) / float64(*in.APWideCustomerCount)
		switch {
		case ratio >= e.thr.APDegradationCustomerRatioCritical:
			pen := 12.0
			score -= pen
			res.Reasons = append(res.Reasons, fmt.Sprintf("AP genelinde kritik müşteri oranı %.0f%%", ratio*100))
			res.ContributingMetrics["ap_degradation_ratio"] = -pen
		case ratio >= e.thr.APDegradationCustomerRatioWarning:
			pen := 5.0
			score -= pen
			res.Reasons = append(res.Reasons, fmt.Sprintf("AP genelinde uyarı müşteri oranı %.0f%%", ratio*100))
			res.ContributingMetrics["ap_degradation_ratio"] = -pen
		}
	}

	// Link kapasitesi
	if in.LinkCapacityRatio != nil && *in.LinkCapacityRatio >= 0.85 {
		pen := 6.0
		score -= pen
		res.Reasons = append(res.Reasons, fmt.Sprintf("Backhaul kapasite oranı yüksek (%.0f%%)", *in.LinkCapacityRatio*100))
		res.ContributingMetrics["link_capacity_ratio"] = -pen
	}

	// 3) Clamp ve severity
	score = clampF(score, 0, 100)
	res.Score = int(math.Round(score))
	res.Severity = e.thr.SeverityFromScore(res.Score)

	// 4) Diagnosis (eşikten klasifiye)
	res.Diagnosis = e.thr.Classify(in, now)
	if res.IsStale && res.Diagnosis != DiagStaleData {
		// stale işaretliyiz ama daha spesifik bir tanı bulduk → reason ekle
		res.Reasons = append(res.Reasons, "(uyarı: bazı veriler eski olabilir)")
	}

	// 5) Action
	res.RecommendedAction = ActionFor(res.Diagnosis, res.Severity)

	// healthy ise reasons listesini sade tut
	if res.Severity == SeverityHealthy && res.Diagnosis == DiagHealthy {
		res.Reasons = []string{"Tüm metrikler eşikler içinde"}
	}
	return res
}

// ScoreBatch, çoklu Inputs üzerinde sırayla skor üretir.
func (e *Engine) ScoreBatch(items []Inputs) []Result {
	out := make([]Result, len(items))
	for i, in := range items {
		out[i] = e.ScoreCustomer(in)
	}
	return out
}
