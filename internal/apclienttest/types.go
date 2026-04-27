// Package apclienttest implements bounded, server-originated reachability
// tests against customer/CPE/client targets. Phase 5 contract:
//
//   - Tests run from the wisp-ops-center server, NOT from inside the AP
//     device. The UI calls them "AP→Client" for operator clarity, but
//     the runtime path is server→target. Future AP-side execution is
//     deferred until a safe allowlisted model lands.
//   - count, timeout, and total duration are all bounded.
//   - test_type is allowlisted; high-risk tests stay disabled.
//   - No device configuration is read or written.
//
// See docs/AP_CLIENT_TESTS_RUNTIME.md.
package apclienttest

import (
	"errors"
	"net"
	"time"
)

type TestType string

const (
	TypePingLatency TestType = "ping_latency"
	TypePacketLoss  TestType = "packet_loss"
	TypeJitter      TestType = "jitter"
	TypeTraceroute  TestType = "traceroute"

	// Disabled in Phase 5 — kept here so the API can return ErrTestDisabled
	// instead of "unknown test_type" when an operator UI tries to schedule them.
	TypeLimitedThroughput     TestType = "limited_throughput"
	TypeMikroTikBandwidthTest TestType = "mikrotik_bandwidth_test"
)

// AllowedTypes returns the test types Phase 5 will execute.
func AllowedTypes() []TestType {
	return []TestType{TypePingLatency, TypePacketLoss, TypeJitter, TypeTraceroute}
}

// IsAllowed reports whether a test type is enabled in this phase.
func IsAllowed(t TestType) bool {
	for _, k := range AllowedTypes() {
		if k == t {
			return true
		}
	}
	return false
}

// IsDisabledInPhase5 differentiates "we know about this but it's off"
// from "we have no idea what this is".
func IsDisabledInPhase5(t TestType) bool {
	return t == TypeLimitedThroughput || t == TypeMikroTikBandwidthTest
}

// TestRequest carries the parameters for one test invocation.
type TestRequest struct {
	APDeviceID       string
	CustomerID       string
	CustomerDeviceID string
	TargetIP         string
	Type             TestType

	// Bounds — defaults applied by Validate().
	Count       int           // ICMP packets (1..20)
	Timeout     time.Duration // per packet (50ms..5s)
	MaxDuration time.Duration // total wall clock (1s..60s)
	RiskLevel   string        // expected to be "low" in Phase 5
}

// Diagnosis categories surfaced to UI/audit.
type Diagnosis string

const (
	DiagHealthy          Diagnosis = "healthy"
	DiagHighLatency      Diagnosis = "high_latency"
	DiagPacketLoss       Diagnosis = "packet_loss"
	DiagUnstableJitter   Diagnosis = "unstable_jitter"
	DiagUnreachable      Diagnosis = "unreachable"
	DiagRouteIssue       Diagnosis = "route_issue"
	DiagDataInsufficient Diagnosis = "data_insufficient"
)

// TestResult is the bundled outcome.
type TestResult struct {
	Type             TestType `json:"test_type"`
	APDeviceID       string   `json:"ap_device_id"`
	CustomerID       string   `json:"customer_id,omitempty"`
	CustomerDeviceID string   `json:"customer_device_id,omitempty"`
	TargetIP         string   `json:"target_ip"`

	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	DurationMS int64     `json:"duration_ms"`

	LatencyMinMs      *float64 `json:"latency_min_ms,omitempty"`
	LatencyAvgMs      *float64 `json:"latency_avg_ms,omitempty"`
	LatencyMaxMs      *float64 `json:"latency_max_ms,omitempty"`
	PacketLossPercent *float64 `json:"packet_loss_percent,omitempty"`
	JitterMs          *float64 `json:"jitter_ms,omitempty"`
	HopCount          *int     `json:"hop_count,omitempty"`

	Status       string    `json:"status"` // success | partial | failed | blocked
	Diagnosis    Diagnosis `json:"diagnosis"`
	RiskLevel    string    `json:"risk_level"`
	ErrorCode    string    `json:"error_code,omitempty"`
	ErrorMessage string    `json:"error_message,omitempty"`
}

// Sentinel errors. Hata sınıfları audit/UI tarafından sınıflandırma için kullanılır.
var (
	ErrTestDisabled       = errors.New("apclienttest: test type disabled in this phase")
	ErrTestUnknown        = errors.New("apclienttest: unknown test type")
	ErrInvalidTarget      = errors.New("apclienttest: target_ip invalid")
	ErrCountOutOfRange    = errors.New("apclienttest: count out of bounds")
	ErrTimeoutOutOfRange  = errors.New("apclienttest: timeout out of bounds")
	ErrDurationOutOfRange = errors.New("apclienttest: max_duration out of bounds")
)

// Validate clamps + checks bounds. Returns a sentinel on violation.
func (r *TestRequest) Validate() error {
	if r.Type == "" {
		return ErrTestUnknown
	}
	if IsDisabledInPhase5(r.Type) {
		return ErrTestDisabled
	}
	if !IsAllowed(r.Type) {
		return ErrTestUnknown
	}
	if r.TargetIP == "" || net.ParseIP(r.TargetIP) == nil {
		return ErrInvalidTarget
	}
	if r.Count == 0 {
		r.Count = 5
	}
	if r.Count < 1 || r.Count > 20 {
		return ErrCountOutOfRange
	}
	if r.Timeout == 0 {
		r.Timeout = 1500 * time.Millisecond
	}
	if r.Timeout < 50*time.Millisecond || r.Timeout > 5*time.Second {
		return ErrTimeoutOutOfRange
	}
	if r.MaxDuration == 0 {
		r.MaxDuration = 30 * time.Second
	}
	if r.MaxDuration < time.Second || r.MaxDuration > 60*time.Second {
		return ErrDurationOutOfRange
	}
	if r.RiskLevel == "" {
		r.RiskLevel = "low"
	}
	return nil
}
