package scoring

import "time"

// PeerSet, bir AP'nin altındaki tüm müşterilerin ölçümleridir.
// AP-genel kötüleşme tespiti için kullanılır.
type PeerSet struct {
	APDeviceID string
	Customers  []PeerCustomer
}

// PeerCustomer, peer-group analizinde tek bir müşteri.
type PeerCustomer struct {
	CustomerID string
	RSSIdBm    *float64
	SNRdB      *float64
	Score      *int // pre-computed score (varsa)
}

// APDegradationStats, peer-group analiz çıktısı.
type APDegradationStats struct {
	APDeviceID         string
	TotalCustomers     int
	CriticalCustomers  int // skor < WarningAt
	WarningCustomers   int // WarningAt <= skor < HealthyAt
	DegradationRatio   float64 // critical / total
	IsCritical         bool   // ratio >= APDegradationCustomerRatioCritical
	IsWarning          bool   // ratio >= APDegradationCustomerRatioWarning
	AvgRSSIdBm         *float64
	AvgSNRdB           *float64
}

// AnalyzePeerSet, AP altındaki müşterilerden agregasyon üretir.
// Her müşterinin Score değeri zaten hesaplanmış olmalıdır; aksi halde
// peer-group analizine giremez.
func (t Thresholds) AnalyzePeerSet(p PeerSet) APDegradationStats {
	stats := APDegradationStats{
		APDeviceID:     p.APDeviceID,
		TotalCustomers: len(p.Customers),
	}
	if stats.TotalCustomers == 0 {
		return stats
	}
	var rssiSum, snrSum float64
	var rssiCount, snrCount int
	for _, c := range p.Customers {
		if c.Score != nil {
			sev := t.SeverityFromScore(*c.Score)
			switch sev {
			case SeverityCritical:
				stats.CriticalCustomers++
			case SeverityWarning:
				stats.WarningCustomers++
			}
		}
		if c.RSSIdBm != nil {
			rssiSum += *c.RSSIdBm
			rssiCount++
		}
		if c.SNRdB != nil {
			snrSum += *c.SNRdB
			snrCount++
		}
	}
	if rssiCount > 0 {
		avg := rssiSum / float64(rssiCount)
		stats.AvgRSSIdBm = &avg
	}
	if snrCount > 0 {
		avg := snrSum / float64(snrCount)
		stats.AvgSNRdB = &avg
	}
	stats.DegradationRatio = float64(stats.CriticalCustomers) / float64(stats.TotalCustomers)
	stats.IsCritical = stats.DegradationRatio >= t.APDegradationCustomerRatioCritical
	stats.IsWarning = stats.DegradationRatio >= t.APDegradationCustomerRatioWarning
	return stats
}

// ScoreAP, AP cihaz seviyesinde özet skor üretir.
func (e *Engine) ScoreAP(in APInputs) APResult {
	out := APResult{
		APDeviceID:   in.APDeviceID,
		CalculatedAt: e.clock(),
	}
	total := len(in.CustomerScores)
	if total == 0 || !in.HasFreshTelemetry {
		out.APScore = 0
		out.Severity = SeverityUnknown
		out.Reasons = append(out.Reasons, "AP altında müşteri yok ya da telemetri taze değil")
		return out
	}
	out.DegradationRatio = float64(in.CriticalCustomerCount) / float64(total)
	avg := 0
	for _, s := range in.CustomerScores {
		avg += s
	}
	avg /= total
	out.APScore = avg

	// AP genel kötüleşme tespiti
	if out.DegradationRatio >= e.thr.APDegradationCustomerRatioCritical {
		out.IsAPWideInterference = true
		out.Reasons = append(out.Reasons,
			"AP genelinde kritik müşteri oranı eşik üstü")
		// AP skoru critical ratio kadar düşür
		out.APScore = clampInt(out.APScore-int(out.DegradationRatio*40), 0, 100)
	} else if out.DegradationRatio >= e.thr.APDegradationCustomerRatioWarning {
		out.Reasons = append(out.Reasons, "AP genelinde uyarı seviyesi müşteri yoğunluğu")
		out.APScore = clampInt(out.APScore-int(out.DegradationRatio*20), 0, 100)
	}
	out.Severity = e.thr.SeverityFromScore(out.APScore)
	return out
}

// ScoreLink, PtP backhaul link skorunu üretir.
func (e *Engine) ScoreLink(in LinkInputs) LinkResult {
	out := LinkResult{
		LinkID:       in.LinkID,
		Score:        100,
		CalculatedAt: e.clock(),
	}
	reasons := []string{}
	score := 100.0

	// Her iki uç da var mı?
	if !in.HasFreshTelemetry {
		out.Diagnosis = DiagStaleData
		out.Score = 0
		out.Severity = SeverityUnknown
		out.Reasons = []string{"Link telemetrisi taze değil"}
		return out
	}

	for _, sig := range []*float64{in.SignalA, in.SignalB} {
		if sig == nil {
			continue
		}
		if *sig <= e.thr.RSSICriticalDbm {
			score -= 30
			reasons = append(reasons, "Link uç sinyali kritik düşük")
		} else if *sig <= e.thr.RSSIWarningDbm {
			score -= 12
			reasons = append(reasons, "Link uç sinyali sınırda")
		}
	}
	for _, snr := range []*float64{in.SNRA, in.SNRB} {
		if snr == nil {
			continue
		}
		if *snr < e.thr.SNRCriticalDb {
			score -= 25
			reasons = append(reasons, "Link uç SNR kritik")
		} else if *snr < e.thr.SNRWarningDb {
			score -= 10
			reasons = append(reasons, "Link uç SNR sınırda")
		}
	}
	for _, loss := range []*float64{in.LossPctA, in.LossPctB} {
		if loss == nil {
			continue
		}
		if *loss >= e.thr.PacketLossCriticalPercent {
			score -= 25
			reasons = append(reasons, "Link uçta paket kaybı kritik")
		} else if *loss >= e.thr.PacketLossWarningPercent {
			score -= 8
			reasons = append(reasons, "Link uçta paket kaybı uyarı")
		}
	}
	if in.CapacityRatio != nil {
		if *in.CapacityRatio >= 0.95 {
			score -= 15
			reasons = append(reasons, "Link kapasitesi neredeyse dolu")
		} else if *in.CapacityRatio >= 0.85 {
			score -= 8
			reasons = append(reasons, "Link kapasitesi yüksek (>%85)")
		}
	}
	score = clampF(score, 0, 100)
	out.Score = int(score)
	out.Severity = e.thr.SeverityFromScore(out.Score)
	if out.Severity == SeverityCritical {
		out.Diagnosis = DiagPTPLinkDegradation
	} else if out.Severity == SeverityWarning {
		out.Diagnosis = DiagPTPLinkDegradation
	} else {
		out.Diagnosis = DiagHealthy
	}
	out.Reasons = reasons
	return out
}

// ScoreTower, alt birimlerden kule risk skoru üretir.
// RiskScore = 100 - clamp(weighted average of (100 - sub_score)).
// Yüksek risk = yüksek skor.
func (e *Engine) ScoreTower(in TowerInputs) TowerResult {
	out := TowerResult{
		TowerID:      in.TowerID,
		CalculatedAt: e.clock(),
	}
	totalUnits := len(in.APResults) + len(in.LinkResults)
	if totalUnits == 0 && in.CustomerTotal == 0 {
		out.RiskScore = 0
		out.Severity = SeverityUnknown
		out.Reasons = []string{"Kule altında ölçüm yok"}
		return out
	}
	// Alt birim skorlarının "tersi" risk olarak alınır
	risk := 0.0
	count := 0
	for _, ap := range in.APResults {
		risk += 100.0 - float64(ap.APScore)
		count++
		if ap.IsAPWideInterference {
			out.Reasons = append(out.Reasons,
				"AP "+ap.APDeviceID+" üzerinde AP-wide kötüleşme")
		}
	}
	for _, l := range in.LinkResults {
		risk += 100.0 - float64(l.Score)
		count++
		if l.Severity == SeverityCritical {
			out.Reasons = append(out.Reasons,
				"Link "+l.LinkID+" kritik")
		}
	}
	// Müşteri katkısı (kritik müşteri yoğunluğu)
	if in.CustomerTotal > 0 {
		critPct := float64(in.CustomerCriticalCount) / float64(in.CustomerTotal)
		risk += critPct * 50
		count++
		if critPct >= 0.40 {
			out.Reasons = append(out.Reasons, "Kule altında kritik müşteri oranı yüksek")
		}
	}
	if count == 0 {
		out.RiskScore = 0
	} else {
		out.RiskScore = clampInt(int(risk/float64(count)), 0, 100)
	}
	switch {
	case out.RiskScore >= 60:
		out.Severity = SeverityCritical
	case out.RiskScore >= 30:
		out.Severity = SeverityWarning
	default:
		out.Severity = SeverityHealthy
	}
	return out
}

func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// _ tüm import'ları minimum tutmak için.
var _ time.Time
