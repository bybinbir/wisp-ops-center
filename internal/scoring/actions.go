package scoring

// ActionFor, tanı → önerilen aksiyon eşlemesidir.
// 10 aksiyon kategorisi:
//   - no_action: aksiyon gerekmiyor
//   - monitor: gözlem altında tut
//   - schedule_field_visit: saha ekibi gönder
//   - check_cpe_alignment: CPE anten yönelimini kontrol et
//   - check_customer_cable: müşteri tarafı kablo/etherneti kontrol
//   - check_ap_interference: AP'de parazit ölç
//   - check_ptp_backhaul: PtP backhaul'a bak
//   - review_frequency_plan: frekans planı gözden geçir
//   - verify_power_or_ethernet: cihaz erişimi/güç sorunu
//   - escalate_network_ops: NOC'a yükselt
func ActionFor(diag Diagnosis, sev Severity) Action {
	switch diag {
	case DiagHealthy:
		return ActionNoAction

	case DiagWeakCustomerSignal:
		if sev == SeverityCritical {
			return ActionScheduleFieldVisit
		}
		return ActionCheckCPEAlignment

	case DiagCPEAlignmentIssue:
		return ActionCheckCPEAlignment

	case DiagAPWideInterference:
		if sev == SeverityCritical {
			return ActionEscalateNetworkOps
		}
		return ActionCheckAPInterference

	case DiagPTPLinkDegradation:
		return ActionCheckPTPBackhaul

	case DiagFrequencyChannelRisk:
		return ActionReviewFrequencyPlan

	case DiagHighLatency:
		if sev == SeverityCritical {
			return ActionCheckPTPBackhaul
		}
		return ActionMonitor

	case DiagPacketLoss:
		if sev == SeverityCritical {
			return ActionScheduleFieldVisit
		}
		return ActionCheckCustomerCable

	case DiagUnstableJitter:
		return ActionMonitor

	case DiagDeviceOffline:
		return ActionVerifyPowerOrEthernet

	case DiagStaleData:
		return ActionMonitor

	case DiagDataInsufficient:
		return ActionMonitor

	default:
		return ActionMonitor
	}
}

// ActionLabel, UI görüntüsü için kısa Türkçe etiket.
func ActionLabel(a Action) string {
	switch a {
	case ActionNoAction:
		return "Aksiyon gerekmiyor"
	case ActionMonitor:
		return "Gözlem"
	case ActionScheduleFieldVisit:
		return "Saha ziyareti planla"
	case ActionCheckCPEAlignment:
		return "CPE anten yönü kontrol"
	case ActionCheckCustomerCable:
		return "Müşteri kablo/Ethernet kontrol"
	case ActionCheckAPInterference:
		return "AP parazit ölçümü"
	case ActionCheckPTPBackhaul:
		return "PtP backhaul kontrolü"
	case ActionReviewFrequencyPlan:
		return "Frekans planı gözden geçir"
	case ActionVerifyPowerOrEthernet:
		return "Güç / Ethernet doğrulama"
	case ActionEscalateNetworkOps:
		return "Network Ops'a yükselt"
	default:
		return string(a)
	}
}

// DiagnosisLabel, UI görüntüsü için kısa Türkçe etiket.
func DiagnosisLabel(d Diagnosis) string {
	switch d {
	case DiagHealthy:
		return "Sağlıklı"
	case DiagWeakCustomerSignal:
		return "Zayıf Müşteri Sinyali"
	case DiagCPEAlignmentIssue:
		return "CPE Yönlendirme Sorunu"
	case DiagAPWideInterference:
		return "AP Genelinde Parazit"
	case DiagPTPLinkDegradation:
		return "PtP Link Kötüleşmesi"
	case DiagFrequencyChannelRisk:
		return "Frekans/Kanal Riski"
	case DiagHighLatency:
		return "Yüksek Gecikme"
	case DiagPacketLoss:
		return "Paket Kaybı"
	case DiagUnstableJitter:
		return "Kararsız Jitter"
	case DiagDeviceOffline:
		return "Cihaz Çevrimdışı"
	case DiagStaleData:
		return "Veri Bayat"
	case DiagDataInsufficient:
		return "Yetersiz Veri"
	default:
		return string(d)
	}
}
