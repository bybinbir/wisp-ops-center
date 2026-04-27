// Package mimosa, Mimosa kablosuz cihazları için salt-okuma SNMP
// adapter'ıdır. Faz 4 sözleşmesi: yalnızca okuma. Yazma yetenekleri
// tanım, kod veya UI seviyesinde mevcut değildir.
package mimosa

import "time"

// Transport names the wire we use to talk to Mimosa devices. Faz 4'te
// yalnızca SNMP desteklenir; vendor API ileri fazlara bırakıldı.
type Transport string

const (
	TransportSNMP      Transport = "snmp"
	TransportVendorAPI Transport = "vendor-api" // ileri faz; Faz 4'te kullanılmaz
)

// SNMPVersion mirrors the credential profile auth_type.
type SNMPVersion string

const (
	SNMPv2c SNMPVersion = "v2c"
	SNMPv3  SNMPVersion = "v3"
)

// SNMPv3SecurityLevel matches RFC 3411 USM.
type SNMPv3SecurityLevel string

const (
	NoAuthNoPriv SNMPv3SecurityLevel = "noAuthNoPriv"
	AuthNoPriv   SNMPv3SecurityLevel = "authNoPriv"
	AuthPriv     SNMPv3SecurityLevel = "authPriv"
)

// SNMPv3AuthProtocol is the message-auth algorithm.
type SNMPv3AuthProtocol string

const (
	AuthMD5    SNMPv3AuthProtocol = "MD5"
	AuthSHA    SNMPv3AuthProtocol = "SHA"
	AuthSHA256 SNMPv3AuthProtocol = "SHA256"
)

// SNMPv3PrivProtocol is the message-priv algorithm.
type SNMPv3PrivProtocol string

const (
	PrivDES    SNMPv3PrivProtocol = "DES"
	PrivAES    SNMPv3PrivProtocol = "AES"
	PrivAES192 SNMPv3PrivProtocol = "AES192"
	PrivAES256 SNMPv3PrivProtocol = "AES256"
)

// Config carries per-device wiring. Secrets are passed separately from
// the credential vault, never embedded here.
type Config struct {
	DeviceID    string
	Host        string
	Port        int
	Transport   Transport
	SNMPVersion SNMPVersion

	// SNMPv2c
	Community string

	// SNMPv3 USM
	V3Username      string
	V3SecurityLevel SNMPv3SecurityLevel
	V3AuthProtocol  SNMPv3AuthProtocol
	V3AuthSecret    string
	V3PrivProtocol  SNMPv3PrivProtocol
	V3PrivSecret    string

	TimeoutSec int
}

// Vendor returns the constant adapter id.
func Vendor() string { return "mimosa" }

// MimosaProbeResult is the normalized output of Probe.
type MimosaProbeResult struct {
	DeviceID          string    `json:"device_id"`
	Reachable         bool      `json:"reachable"`
	Transport         string    `json:"transport"`
	SystemName        string    `json:"system_name,omitempty"`
	SystemDescr       string    `json:"system_descr,omitempty"`
	UptimeSec         int64     `json:"uptime_sec,omitempty"`
	Model             string    `json:"model,omitempty"`             // best-effort from sysDescr
	Firmware          string    `json:"firmware,omitempty"`          // best-effort from sysDescr
	WirelessAvailable bool      `json:"wireless_available"`          // partial: vendor MIB uncertain
	VendorMIBStatus   string    `json:"vendor_mib_status,omitempty"` // ok|unverified|unsupported
	Partial           bool      `json:"partial"`                     // true: yalnız standart SNMP verisi
	Error             string    `json:"error,omitempty"`
	CollectedAt       time.Time `json:"collected_at"`
}

// MimosaSystemInfo carries the standard system snapshot.
type MimosaSystemInfo struct {
	UptimeSec    int64  `json:"uptime_sec,omitempty"`
	SystemName   string `json:"system_name,omitempty"`
	SystemDescr  string `json:"system_descr,omitempty"`
	Model        string `json:"model,omitempty"`
	Firmware     string `json:"firmware,omitempty"`
	Architecture string `json:"architecture,omitempty"`
}

// MimosaInterfaceMetric is a row from ifTable/ifXTable.
type MimosaInterfaceMetric struct {
	Index       int    `json:"index"`
	Name        string `json:"name"`
	Descr       string `json:"descr,omitempty"`
	Type        int    `json:"type,omitempty"`
	MTU         int    `json:"mtu,omitempty"`
	AdminUp     bool   `json:"admin_up"`
	OperUp      bool   `json:"oper_up"`
	SpeedBps    int64  `json:"speed_bps,omitempty"`
	InOctets    int64  `json:"in_octets,omitempty"`
	OutOctets   int64  `json:"out_octets,omitempty"`
	InErrors    int64  `json:"in_errors,omitempty"`
	OutErrors   int64  `json:"out_errors,omitempty"`
	InDiscards  int64  `json:"in_discards,omitempty"`
	OutDiscards int64  `json:"out_discards,omitempty"`
}

// MimosaRadioMetric represents a radio status row. Faz 4'te vendor MIB
// olmadığında bu yapı boş döner; sadece sözleşmeyi sabitler.
type MimosaRadioMetric struct {
	Name            string   `json:"name"`
	FrequencyMHz    *int     `json:"frequency_mhz,omitempty"`
	ChannelWidthMHz *int     `json:"channel_width_mhz,omitempty"`
	TxPowerDBm      *float64 `json:"tx_power_dbm,omitempty"`
	NoiseFloor      *float64 `json:"noise_floor_dbm,omitempty"`
}

// MimosaLinkMetric is the per-link backhaul snapshot.
type MimosaLinkMetric struct {
	Name          string   `json:"name"`
	PeerIP        string   `json:"peer_ip,omitempty"`
	SignalDBm     *float64 `json:"signal_dbm,omitempty"`
	SignalToNoise *float64 `json:"snr_db,omitempty"`
	CapacityMbps  *float64 `json:"capacity_mbps,omitempty"`
	UptimeSec     int64    `json:"uptime_sec,omitempty"`
	StationCount  *int     `json:"station_count,omitempty"`
}

// MimosaClientMetric is a single connected station entry.
type MimosaClientMetric struct {
	MAC           string   `json:"mac"`
	IP            string   `json:"ip,omitempty"`
	Hostname      string   `json:"hostname,omitempty"`
	SignalDBm     *float64 `json:"signal_dbm,omitempty"`
	SignalToNoise *float64 `json:"snr_db,omitempty"`
	TxRateMbps    *float64 `json:"tx_rate_mbps,omitempty"`
	RxRateMbps    *float64 `json:"rx_rate_mbps,omitempty"`
	UptimeSec     int64    `json:"uptime_sec,omitempty"`
}

// MimosaReadOnlySnapshot is the bundle returned by Poll.
type MimosaReadOnlySnapshot struct {
	DeviceID        string                  `json:"device_id"`
	Transport       string                  `json:"transport"`
	StartedAt       time.Time               `json:"started_at"`
	FinishedAt      time.Time               `json:"finished_at"`
	DurationMS      int64                   `json:"duration_ms"`
	System          *MimosaSystemInfo       `json:"system,omitempty"`
	Interfaces      []MimosaInterfaceMetric `json:"interfaces,omitempty"`
	Radios          []MimosaRadioMetric     `json:"radios,omitempty"`
	Links           []MimosaLinkMetric      `json:"links,omitempty"`
	Clients         []MimosaClientMetric    `json:"clients,omitempty"`
	VendorMIBStatus string                  `json:"vendor_mib_status,omitempty"`
	Partial         bool                    `json:"partial"`
	Errors          []string                `json:"errors,omitempty"` // sanitized
}

// CapabilityFlags drives device_capabilities updates after a successful probe.
// Yazma alanları KASITLI olarak yoktur — Faz 4'te Mimosa için yazma
// yeteneği desteklenmez.
type CapabilityFlags struct {
	SupportsSNMP           bool
	SupportsVendorAPI      bool // Faz 4: daima false; sözleşme korunur
	CanReadHealth          bool
	CanReadWirelessMetrics bool
	CanReadClients         bool
	CanReadFrequency       bool
	CanRecommendFrequency  bool
}
