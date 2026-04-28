package networkactions

import "time"

// RunStatus enumerates the lifecycle of a single action run as
// persisted in network_action_runs.status.
type RunStatus string

const (
	StatusQueued    RunStatus = "queued"
	StatusRunning   RunStatus = "running"
	StatusSucceeded RunStatus = "succeeded"
	StatusFailed    RunStatus = "failed"
	StatusSkipped   RunStatus = "skipped"
)

// IsValidStatus is true for any allowed RunStatus value.
func IsValidStatus(s string) bool {
	switch RunStatus(s) {
	case StatusQueued, StatusRunning, StatusSucceeded, StatusFailed, StatusSkipped:
		return true
	}
	return false
}

// IsValidKind is true for any registered action Kind.
func IsValidKind(s string) bool {
	switch Kind(s) {
	case KindFrequencyCheck, KindFrequencyCorrection,
		KindAPClientTest, KindLinkSignalTest,
		KindBridgeHealthCheck, KindMaintenanceWindow:
		return true
	}
	return false
}

// ActionRun is the DB-row projection of a network_action_runs entry.
// It is the JSON shape returned by the API.
type ActionRun struct {
	ID             string         `json:"id"`
	ActionType     Kind           `json:"action_type"`
	TargetDeviceID string         `json:"target_device_id,omitempty"`
	TargetHost     string         `json:"target_host,omitempty"`
	TargetLabel    string         `json:"target_label,omitempty"`
	Status         RunStatus      `json:"status"`
	StartedAt      *time.Time     `json:"started_at,omitempty"`
	FinishedAt     *time.Time     `json:"finished_at,omitempty"`
	DurationMS     int64          `json:"duration_ms"`
	Actor          string         `json:"actor"`
	CorrelationID  string         `json:"correlation_id"`
	DryRun         bool           `json:"dry_run"`
	Result         map[string]any `json:"result"`
	CommandCount   int            `json:"command_count"`
	WarningCount   int            `json:"warning_count"`
	Confidence     int            `json:"confidence"`
	ErrorCode      string         `json:"error_code,omitempty"`
	ErrorMessage   string         `json:"error_message,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// FrequencyCheckResult is the structured payload that
// FrequencyCheckAction stores under ActionRun.Result. Every field is
// optional so a device with no wireless interfaces can still produce
// a meaningful (though mostly empty) record.
type FrequencyCheckResult struct {
	DeviceIdentity string             `json:"device_identity,omitempty"`
	MenuSource     string             `json:"menu_source,omitempty"` // "wireless" / "wifi" / "wifiwave2" / "none"
	Interfaces     []WirelessSnapshot `json:"interfaces"`
	Warnings       []string           `json:"warnings"`
	Evidence       []string           `json:"evidence"`
	Skipped        bool               `json:"skipped,omitempty"`
	SkippedReason  string             `json:"skipped_reason,omitempty"`
}

// WirelessSnapshot is one wireless interface's read-only state at
// the moment of the check.
type WirelessSnapshot struct {
	InterfaceName  string `json:"interface_name"`
	RadioType      string `json:"radio_type,omitempty"`
	Frequency      string `json:"frequency,omitempty"`
	Band           string `json:"band,omitempty"`
	ChannelWidth   string `json:"channel_width,omitempty"`
	Mode           string `json:"mode,omitempty"`
	SSID           string `json:"ssid,omitempty"`
	Running        *bool  `json:"running,omitempty"`
	Disabled       *bool  `json:"disabled,omitempty"`
	ClientCount    int    `json:"client_count"`
	AvgSignal      *int   `json:"avg_signal,omitempty"`
	WorstSignal    *int   `json:"worst_signal,omitempty"`
	AvgCCQ         *int   `json:"avg_ccq,omitempty"`
	NoiseFloor     *int   `json:"noise_floor,omitempty"`
	TxRateMbps     *int   `json:"tx_rate_mbps,omitempty"`
	RxRateMbps     *int   `json:"rx_rate_mbps,omitempty"`
	RegistrationOK bool   `json:"registration_ok"`
}

// SourceCommand records one allowed command attempted by the action,
// plus its sanitized status. Used only inside the audit metadata —
// never includes the raw output, since RouterOS print/detail bodies
// can carry SSID/MAC/identity that, while not a secret, we still
// keep at the inventory layer (network_devices.raw_metadata).
type SourceCommand struct {
	Command  string `json:"command"`
	Status   string `json:"status"` // "executed" / "skipped_unsupported" / "failed"
	Records  int    `json:"records"`
	ElapsedM int64  `json:"elapsed_ms"`
}
