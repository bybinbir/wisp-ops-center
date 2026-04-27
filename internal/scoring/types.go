package scoring

import "time"

// Severity, skorun operasyonel öncelik sınıfıdır.
type Severity string

const (
	SeverityHealthy  Severity = "healthy"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
	SeverityUnknown  Severity = "unknown"
)

// Diagnosis, sınıflandırma sonucudur. Faz 6 ile genişletilmiş 12 kategori.
type Diagnosis string

const (
	DiagHealthy              Diagnosis = "healthy"
	DiagWeakCustomerSignal   Diagnosis = "weak_customer_signal"
	DiagCPEAlignmentIssue    Diagnosis = "possible_cpe_alignment_issue"
	DiagAPWideInterference   Diagnosis = "ap_wide_interference"
	DiagPTPLinkDegradation   Diagnosis = "ptp_link_degradation"
	DiagFrequencyChannelRisk Diagnosis = "frequency_channel_risk"
	DiagHighLatency          Diagnosis = "high_latency"
	DiagPacketLoss           Diagnosis = "packet_loss"
	DiagUnstableJitter       Diagnosis = "unstable_jitter"
	DiagDeviceOffline        Diagnosis = "device_offline"
	DiagStaleData            Diagnosis = "stale_data"
	DiagDataInsufficient     Diagnosis = "data_insufficient"
)

// Action, tanı sonucu önerilen operasyonel aksiyondur.
type Action string

const (
	ActionNoAction              Action = "no_action"
	ActionMonitor               Action = "monitor"
	ActionScheduleFieldVisit    Action = "schedule_field_visit"
	ActionCheckCPEAlignment     Action = "check_cpe_alignment"
	ActionCheckCustomerCable    Action = "check_customer_cable"
	ActionCheckAPInterference   Action = "check_ap_interference"
	ActionCheckPTPBackhaul      Action = "check_ptp_backhaul"
	ActionReviewFrequencyPlan   Action = "review_frequency_plan"
	ActionVerifyPowerOrEthernet Action = "verify_power_or_ethernet"
	ActionEscalateNetworkOps    Action = "escalate_network_ops"
)

// Inputs, tek bir müşteri için skor hesaplamasının ihtiyaç duyduğu
// telemetri ve test sonuçlarıdır. Eksik alanlar nil olarak verilir.
//
// Tüm zaman alanları UTC saklanır.
type Inputs struct {
	// Doğrudan radyo telemetrisi
	RSSIdBm    *float64 // dBm (negatif). Kuvvet: -50 mükemmel, -90 felaket.
	SNRdB      *float64 // dB. Yüksek iyi.
	CCQ        *float64 // 0..100. RouterOS Wi-Fi.
	TxRateMbps *float64 // Mbps
	RxRateMbps *float64 // Mbps

	// Stabilite
	DisconnectsLastDay *int     // son 24 saat
	UptimeStability    *float64 // 0..1

	// Aktif test (AP→Client) sonuçları
	AvgLatencyMs    *float64 // ms
	MaxLatencyMs    *float64 // ms
	PacketLossPct   *float64 // 0..100
	JitterMs        *float64 // ms (mdev)
	TraceHopsCount  *int
	LastTestSuccess *bool

	// Çevresel / AP-genel veriler
	APWideDegradation     *float64 // 0..1, peer-group bazlı kötüleşme oranı
	APWideCustomerCount   *int     // AP'ye bağlı toplam müşteri (peer set boyutu)
	APWideDegradedCustCnt *int     // AP'de skor < critical olan müşteri sayısı
	LinkCapacityRatio     *float64 // 0..1, taşınan / nominal (PtP backhaul)

	// Trend
	SignalTrend7d *float64 // dB/gün, negatif = kötüleşme

	// Veri tazeliği
	LastSampleAt    *time.Time
	LastTestAt      *time.Time
	StaleDataMinute int // override yoksa Thresholds.StaleDataMinutes kullanılır
}

// Result, tek bir müşteri (veya birim) için üretilen skordur.
type Result struct {
	Score             int       // 0..100 (yüksek iyi)
	Severity          Severity  // healthy / warning / critical / unknown
	Diagnosis         Diagnosis // 12 kategoriden biri
	RecommendedAction Action    // 10 kategoriden biri
	Reasons           []string  // operatörün okuyabileceği açıklamalar
	ContributingMetrics map[string]float64 // skoru etkileyen ölçüm değerleri (-X puan)
	CalculatedAt      time.Time
	IsStale           bool
}

// APInputs, AP cihaz seviyesinde özet skor hesaplamasının girdileridir.
type APInputs struct {
	APDeviceID            string
	CustomerScores        []int    // bu AP'ye bağlı tüm müşterilerin skorları
	CriticalCustomerCount int      // skoru critical olan
	WarningCustomerCount  int
	HealthyCustomerCount  int
	AvgRSSIdBm            *float64 // peer ortalaması
	AvgSNRdB              *float64
	HasFreshTelemetry     bool
}

// APResult, AP düzey skoru.
type APResult struct {
	APDeviceID            string
	APScore               int
	Severity              Severity
	DegradationRatio      float64 // critical / total
	IsAPWideInterference  bool
	Reasons               []string
	CalculatedAt          time.Time
}

// LinkInputs, PtP/PtMP backhaul link skorlama girdileri.
type LinkInputs struct {
	LinkID            string
	SignalA, SignalB  *float64 // dBm her iki uç
	SNRA, SNRB        *float64
	CapacityRatio     *float64 // taşınan / nominal
	LossPctA, LossPctB *float64
	HasFreshTelemetry bool
}

// LinkResult, PtP link skoru.
type LinkResult struct {
	LinkID       string
	Score        int
	Severity     Severity
	Diagnosis    Diagnosis
	Reasons      []string
	CalculatedAt time.Time
}

// TowerInputs, kule risk skorlamasının girdileri (alt birimlerin agregasyonu).
type TowerInputs struct {
	TowerID                   string
	APResults                 []APResult
	LinkResults               []LinkResult
	CustomerCriticalCount     int
	CustomerWarningCount      int
	CustomerTotal             int
}

// TowerResult, kule operasyonel risk skoru.
type TowerResult struct {
	TowerID      string
	RiskScore    int // 0..100 (yüksek = riskli)
	Severity     Severity
	Reasons      []string
	CalculatedAt time.Time
}

// SeverityFromScore, 0..100 skor → severity dönüşümü (varsayılan eşikler).
// Override gerektiğinde Thresholds.SeverityFromScore kullanılır.
func SeverityFromScore(score int) Severity {
	switch {
	case score < 0:
		return SeverityUnknown
	case score >= 80:
		return SeverityHealthy
	case score >= 50:
		return SeverityWarning
	default:
		return SeverityCritical
	}
}
