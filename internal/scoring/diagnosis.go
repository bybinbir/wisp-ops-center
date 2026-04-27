package scoring

import "time"

// Classify, müşteri girdilerinden 12 kategoriden birini seçer.
// Sıralama önemlidir: en spesifik durum ilk işaretlenir.
//
// Karar ağacı:
//
//  1. data_insufficient   → RSSI/SNR ve test verisi yoksa
//  2. stale_data          → en son örnek/test çok eski
//  3. device_offline      → LastTestSuccess=false + telemetri ölü
//  4. ap_wide_interference → AP geneli kötüleşme oranı eşik üstü
//  5. ptp_link_degradation → PtP backhaul yüklü/zayıf
//  6. weak_customer_signal → RSSI critical
//  7. possible_cpe_alignment_issue → SNR çok düşük (alignment kaybı işareti)
//  8. frequency_channel_risk → SNR sınır (parazit/kanal sorunu)
//  9. high_latency        → AP→Client testte gecikme kritik
//  10. packet_loss        → AP→Client testte loss kritik
//  11. unstable_jitter    → AP→Client testte jitter kritik
//  12. healthy            → tüm sinyaller iyi
func (t Thresholds) Classify(in Inputs, now time.Time) Diagnosis {
	// 1. Yetersiz veri
	if in.RSSIdBm == nil && in.SNRdB == nil &&
		in.AvgLatencyMs == nil && in.PacketLossPct == nil && in.JitterMs == nil {
		return DiagDataInsufficient
	}

	// 2. Veri tazeliği — örnek/test ikisi de var ama ikisi de eski
	if t.isStale(in, now) {
		return DiagStaleData
	}

	// 3. Cihaz offline — en son test başarısız VE telemetri yok/eski
	if in.LastTestSuccess != nil && !*in.LastTestSuccess {
		if in.RSSIdBm == nil && in.SNRdB == nil {
			return DiagDeviceOffline
		}
	}

	// 4. AP-wide interference — peer-group oranı eşik üstü
	if in.APWideCustomerCount != nil && *in.APWideCustomerCount > 0 &&
		in.APWideDegradedCustCnt != nil {
		ratio := float64(*in.APWideDegradedCustCnt) / float64(*in.APWideCustomerCount)
		if ratio >= t.APDegradationCustomerRatioCritical {
			return DiagAPWideInterference
		}
	}
	if in.APWideDegradation != nil && *in.APWideDegradation >= t.APDegradationCustomerRatioCritical {
		return DiagAPWideInterference
	}

	// 5. PtP link degradation
	if in.LinkCapacityRatio != nil && *in.LinkCapacityRatio >= 0.85 {
		return DiagPTPLinkDegradation
	}

	// 6. Zayıf müşteri sinyali — RSSI critical
	if in.RSSIdBm != nil && *in.RSSIdBm <= t.RSSICriticalDbm {
		return DiagWeakCustomerSignal
	}

	// 7. CPE alignment — SNR critical
	if in.SNRdB != nil && *in.SNRdB < t.SNRCriticalDb {
		return DiagCPEAlignmentIssue
	}

	// 9-11. Test sonuçları kritik mi?
	if in.PacketLossPct != nil && *in.PacketLossPct >= t.PacketLossCriticalPercent {
		return DiagPacketLoss
	}
	if in.AvgLatencyMs != nil && *in.AvgLatencyMs >= t.LatencyCriticalMs {
		return DiagHighLatency
	}
	if in.JitterMs != nil && *in.JitterMs >= t.JitterCriticalMs {
		return DiagUnstableJitter
	}

	// 8. Frekans/kanal — SNR sınırda
	if in.SNRdB != nil && *in.SNRdB < t.SNRWarningDb {
		return DiagFrequencyChannelRisk
	}

	// Uyarı seviyesi: RSSI sınırda → weak_customer_signal (warning)
	if in.RSSIdBm != nil && *in.RSSIdBm <= t.RSSIWarningDbm {
		return DiagWeakCustomerSignal
	}

	// Uyarı seviyesi: latency/loss/jitter warning eşiği üstü
	if in.PacketLossPct != nil && *in.PacketLossPct >= t.PacketLossWarningPercent {
		return DiagPacketLoss
	}
	if in.AvgLatencyMs != nil && *in.AvgLatencyMs >= t.LatencyWarningMs {
		return DiagHighLatency
	}
	if in.JitterMs != nil && *in.JitterMs >= t.JitterWarningMs {
		return DiagUnstableJitter
	}

	return DiagHealthy
}

// isStale, en son veri eski mi?
func (t Thresholds) isStale(in Inputs, now time.Time) bool {
	threshold := t.StaleDataDuration()
	if in.StaleDataMinute > 0 {
		threshold = time.Duration(in.StaleDataMinute) * time.Minute
	}
	if threshold <= 0 {
		return false
	}
	mostRecent := time.Time{}
	if in.LastSampleAt != nil && in.LastSampleAt.After(mostRecent) {
		mostRecent = *in.LastSampleAt
	}
	if in.LastTestAt != nil && in.LastTestAt.After(mostRecent) {
		mostRecent = *in.LastTestAt
	}
	if mostRecent.IsZero() {
		// Hiç zaman damgası yoksa stale kabul etmeyelim — ayrı kategori (data_insufficient).
		return false
	}
	return now.Sub(mostRecent) > threshold
}
