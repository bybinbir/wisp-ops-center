package mikrotik

import "time"

// MikroTikProbeResult is the normalized output of Probe.
type MikroTikProbeResult struct {
	DeviceID          string    `json:"device_id"`
	Reachable         bool      `json:"reachable"`
	Transport         string    `json:"transport"` // api-ssl|ssh|snmp
	IdentityName      string    `json:"identity_name,omitempty"`
	RouterOSVersion   string    `json:"routeros_version,omitempty"`
	Board             string    `json:"board,omitempty"`
	Architecture      string    `json:"architecture,omitempty"`
	UptimeSec         int64     `json:"uptime_sec,omitempty"`
	WirelessAvailable bool      `json:"wireless_available"`
	WiFiPackage       string    `json:"wifi_package,omitempty"` // legacy|wifi|wifiwave2
	Error             string    `json:"error,omitempty"`
	CollectedAt       time.Time `json:"collected_at"`
}

// MikroTikSystemInfo is the system/resource snapshot.
type MikroTikSystemInfo struct {
	UptimeSec    int64    `json:"uptime_sec,omitempty"`
	CPULoadPct   *float64 `json:"cpu_load_pct,omitempty"`
	FreeMemoryB  *int64   `json:"free_memory_bytes,omitempty"`
	TotalMemoryB *int64   `json:"total_memory_bytes,omitempty"`
	TempCelsius  *float64 `json:"temp_c,omitempty"`
	BoardName    string   `json:"board_name,omitempty"`
	Architecture string   `json:"architecture,omitempty"`
	Version      string   `json:"version,omitempty"`
}

// MikroTikInterfaceMetric is a single interface-level metric row.
type MikroTikInterfaceMetric struct {
	Name        string `json:"name"`
	Type        string `json:"type,omitempty"`
	Running     bool   `json:"running"`
	Disabled    bool   `json:"disabled"`
	MTU         int    `json:"mtu,omitempty"`
	MAC         string `json:"mac,omitempty"`
	RxByte      int64  `json:"rx_bytes,omitempty"`
	TxByte      int64  `json:"tx_bytes,omitempty"`
	RxPacket    int64  `json:"rx_packets,omitempty"`
	TxPacket    int64  `json:"tx_packets,omitempty"`
	RxError     int64  `json:"rx_errors,omitempty"`
	TxError     int64  `json:"tx_errors,omitempty"`
	LinkDownCnt int64  `json:"link_downs,omitempty"`
}

// MikroTikWirelessInterfaceMetric is the per-radio summary.
type MikroTikWirelessInterfaceMetric struct {
	Name            string   `json:"name"`
	SSID            string   `json:"ssid,omitempty"`
	Mode            string   `json:"mode,omitempty"`
	Band            string   `json:"band,omitempty"`
	FrequencyMHz    *int     `json:"frequency_mhz,omitempty"`
	ChannelWidthMHz *int     `json:"channel_width_mhz,omitempty"`
	TxPowerDBm      *float64 `json:"tx_power_dbm,omitempty"`
	NoiseFloor      *float64 `json:"noise_floor_dbm,omitempty"`
	Disabled        bool     `json:"disabled"`
	Running         bool     `json:"running"`
}

// MikroTikWirelessClientMetric is a single registration-table row.
type MikroTikWirelessClientMetric struct {
	Interface     string   `json:"interface"`
	MAC           string   `json:"mac"`
	IP            string   `json:"ip,omitempty"`
	SSID          string   `json:"ssid,omitempty"`
	UptimeSec     int64    `json:"uptime_sec,omitempty"`
	SignalDBm     *float64 `json:"signal_dbm,omitempty"`
	SignalToNoise *float64 `json:"snr_db,omitempty"`
	TxRateMbps    *float64 `json:"tx_rate_mbps,omitempty"`
	RxRateMbps    *float64 `json:"rx_rate_mbps,omitempty"`
	CCQ           *float64 `json:"ccq,omitempty"`
}

// MikroTikReadOnlySnapshot is the bundle returned by Poll.
type MikroTikReadOnlySnapshot struct {
	DeviceID           string                            `json:"device_id"`
	Transport          string                            `json:"transport"`
	StartedAt          time.Time                         `json:"started_at"`
	FinishedAt         time.Time                         `json:"finished_at"`
	DurationMS         int64                             `json:"duration_ms"`
	System             *MikroTikSystemInfo               `json:"system,omitempty"`
	Interfaces         []MikroTikInterfaceMetric         `json:"interfaces,omitempty"`
	WirelessInterfaces []MikroTikWirelessInterfaceMetric `json:"wireless_interfaces,omitempty"`
	WirelessClients    []MikroTikWirelessClientMetric    `json:"wireless_clients,omitempty"`
	Errors             []string                          `json:"errors,omitempty"` // sanitized
}

// CapabilityFlags drives device_capabilities updates after a successful probe.
type CapabilityFlags struct {
	SupportsRouterOSAPI    bool
	SupportsSSH            bool
	SupportsSNMP           bool
	CanReadHealth          bool
	CanReadWirelessMetrics bool
	CanReadClients         bool
	CanReadFrequency       bool
	CanRecommendFrequency  bool
}
