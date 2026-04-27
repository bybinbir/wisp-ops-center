package scoring

import "time"

// Thresholds, skor motorunun karar eşikleridir. Veritabanından override
// edilebilir; varsayılan değerler `DefaultThresholds()` içinde tanımlıdır.
//
// Tüm penalty'ler skor 100'den çıkartılır; alt sınır 0'a clamp edilir.
type Thresholds struct {
	// RSSI (dBm). RSSI <= critical → ağır penalty.
	RSSICriticalDbm float64
	RSSIWarningDbm  float64

	// SNR (dB). SNR <  critical → ağır penalty.
	SNRCriticalDb float64
	SNRWarningDb  float64

	// CCQ (yüzde 0..100). CCQ < critical → orta penalty.
	CCQCriticalPercent float64
	CCQWarningPercent  float64

	// Paket kaybı yüzdesi.
	PacketLossCriticalPercent float64
	PacketLossWarningPercent  float64

	// Gecikme (ms).
	LatencyCriticalMs float64
	LatencyWarningMs  float64

	// Jitter (ms).
	JitterCriticalMs float64
	JitterWarningMs  float64

	// Veri tazeliği (dakika). LastSampleAt veya LastTestAt bu süreden eski olursa
	// stale_data tanısı oluşur.
	StaleDataMinutes int

	// AP-wide degradation oranları (peer-group).
	// degraded_count / total_count ≥ critical_ratio → ap_wide_interference.
	APDegradationCustomerRatioWarning  float64
	APDegradationCustomerRatioCritical float64

	// Severity breakpoints (0..100).
	// score >= HealthyAt → healthy
	// score >= WarningAt → warning
	// aksi halde critical
	HealthyAt int
	WarningAt int
}

// DefaultThresholds, görevde verilen varsayılan eşikleri döner.
func DefaultThresholds() Thresholds {
	return Thresholds{
		RSSICriticalDbm:                    -80,
		RSSIWarningDbm:                     -70,
		SNRCriticalDb:                      15,
		SNRWarningDb:                       25,
		CCQCriticalPercent:                 50,
		CCQWarningPercent:                  75,
		PacketLossCriticalPercent:          5,
		PacketLossWarningPercent:           2,
		LatencyCriticalMs:                  100,
		LatencyWarningMs:                   50,
		JitterCriticalMs:                   30,
		JitterWarningMs:                    15,
		StaleDataMinutes:                   60,
		APDegradationCustomerRatioWarning:  0.25,
		APDegradationCustomerRatioCritical: 0.40,
		HealthyAt:                          80,
		WarningAt:                          50,
	}
}

// SeverityFromScore, eşiklere göre severity döner.
func (t Thresholds) SeverityFromScore(score int) Severity {
	switch {
	case score < 0:
		return SeverityUnknown
	case score >= t.HealthyAt:
		return SeverityHealthy
	case score >= t.WarningAt:
		return SeverityWarning
	default:
		return SeverityCritical
	}
}

// StaleDataDuration, eşiği time.Duration olarak döner.
func (t Thresholds) StaleDataDuration() time.Duration {
	return time.Duration(t.StaleDataMinutes) * time.Minute
}

// SeedDefaults, migration tarafından ekilecek anahtar/değer çiftleri.
// Veritabanı satırı olarak: (key text PRIMARY KEY, value double precision, description text).
func SeedDefaults() []struct {
	Key         string
	Value       float64
	Description string
} {
	d := DefaultThresholds()
	return []struct {
		Key         string
		Value       float64
		Description string
	}{
		{"rssi_critical_dbm", d.RSSICriticalDbm, "RSSI altında ağır penalty (dBm)"},
		{"rssi_warning_dbm", d.RSSIWarningDbm, "RSSI sınırda penalty (dBm)"},
		{"snr_critical_db", d.SNRCriticalDb, "SNR altında ağır penalty (dB)"},
		{"snr_warning_db", d.SNRWarningDb, "SNR sınırda penalty (dB)"},
		{"ccq_critical_percent", d.CCQCriticalPercent, "CCQ kritik (%)"},
		{"ccq_warning_percent", d.CCQWarningPercent, "CCQ sınırda (%)"},
		{"packet_loss_critical_percent", d.PacketLossCriticalPercent, "Paket kaybı kritik (%)"},
		{"packet_loss_warning_percent", d.PacketLossWarningPercent, "Paket kaybı uyarı (%)"},
		{"latency_critical_ms", d.LatencyCriticalMs, "Gecikme kritik (ms)"},
		{"latency_warning_ms", d.LatencyWarningMs, "Gecikme uyarı (ms)"},
		{"jitter_critical_ms", d.JitterCriticalMs, "Jitter kritik (ms)"},
		{"jitter_warning_ms", d.JitterWarningMs, "Jitter uyarı (ms)"},
		{"stale_data_minutes", float64(d.StaleDataMinutes), "Veri tazelik eşiği (dk)"},
		{"ap_degradation_customer_ratio_warning", d.APDegradationCustomerRatioWarning, "AP peer-group degradation uyarı oranı"},
		{"ap_degradation_customer_ratio_critical", d.APDegradationCustomerRatioCritical, "AP peer-group degradation kritik oranı"},
		{"severity_healthy_at", float64(d.HealthyAt), "score >= bu değer → healthy"},
		{"severity_warning_at", float64(d.WarningAt), "score >= bu değer → warning"},
	}
}

// thresholdSpec, eşik anahtarının kabul edilebilir [min,max] aralığıdır.
// API üzerinden gelen güncellemeler bu spec'e göre doğrulanır.
type thresholdSpec struct {
	Min float64
	Max float64
}

var thresholdSpecs = map[string]thresholdSpec{
	"rssi_critical_dbm":                      {-120, 0},
	"rssi_warning_dbm":                       {-120, 0},
	"snr_critical_db":                        {0, 60},
	"snr_warning_db":                         {0, 60},
	"ccq_critical_percent":                   {0, 100},
	"ccq_warning_percent":                    {0, 100},
	"packet_loss_critical_percent":           {0, 100},
	"packet_loss_warning_percent":            {0, 100},
	"latency_critical_ms":                    {0, 10000},
	"latency_warning_ms":                     {0, 10000},
	"jitter_critical_ms":                     {0, 10000},
	"jitter_warning_ms":                      {0, 10000},
	"stale_data_minutes":                     {1, 10080},
	"ap_degradation_customer_ratio_warning":  {0, 1},
	"ap_degradation_customer_ratio_critical": {0, 1},
	"severity_healthy_at":                    {0, 100},
	"severity_warning_at":                    {0, 100},
}

// IsKnownThresholdKey, bilinen bir eşik anahtarı mıdır?
func IsKnownThresholdKey(k string) bool {
	_, ok := thresholdSpecs[k]
	return ok
}

// IsValidThresholdValue, anahtar için değer aralık içinde mi?
// Bilinmeyen anahtar için false döner (önce IsKnownThresholdKey kontrol edin).
func IsValidThresholdValue(k string, v float64) bool {
	spec, ok := thresholdSpecs[k]
	if !ok {
		return false
	}
	return v >= spec.Min && v <= spec.Max
}

// ApplyOverrides, key/value mapinden Thresholds alanlarını günceller.
// Bilinmeyen anahtarlar sessizce yok sayılır.
func (t Thresholds) ApplyOverrides(overrides map[string]float64) Thresholds {
	out := t
	for k, v := range overrides {
		switch k {
		case "rssi_critical_dbm":
			out.RSSICriticalDbm = v
		case "rssi_warning_dbm":
			out.RSSIWarningDbm = v
		case "snr_critical_db":
			out.SNRCriticalDb = v
		case "snr_warning_db":
			out.SNRWarningDb = v
		case "ccq_critical_percent":
			out.CCQCriticalPercent = v
		case "ccq_warning_percent":
			out.CCQWarningPercent = v
		case "packet_loss_critical_percent":
			out.PacketLossCriticalPercent = v
		case "packet_loss_warning_percent":
			out.PacketLossWarningPercent = v
		case "latency_critical_ms":
			out.LatencyCriticalMs = v
		case "latency_warning_ms":
			out.LatencyWarningMs = v
		case "jitter_critical_ms":
			out.JitterCriticalMs = v
		case "jitter_warning_ms":
			out.JitterWarningMs = v
		case "stale_data_minutes":
			out.StaleDataMinutes = int(v)
		case "ap_degradation_customer_ratio_warning":
			out.APDegradationCustomerRatioWarning = v
		case "ap_degradation_customer_ratio_critical":
			out.APDegradationCustomerRatioCritical = v
		case "severity_healthy_at":
			out.HealthyAt = int(v)
		case "severity_warning_at":
			out.WarningAt = int(v)
		}
	}
	return out
}
