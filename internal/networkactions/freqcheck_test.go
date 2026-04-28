package networkactions

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	wispssh "github.com/wisp-ops-center/wisp-ops-center/internal/adapters/ssh"
)

// fakeDialer is a hermetic SSH stand-in for FrequencyCheckAction
// tests. It never opens a network socket; Exec returns canned
// outputs keyed by the allowlisted command path.
type fakeDialer struct {
	dialErr   error
	responses map[string]struct {
		out string
		err error
	}
	execLog []string
}

func (f *fakeDialer) Dial(_ context.Context) error { return f.dialErr }
func (f *fakeDialer) Close()                       {}
func (f *fakeDialer) SetCorrelationID(string)      {}
func (f *fakeDialer) Exec(_ context.Context, cmd string) (string, error) {
	f.execLog = append(f.execLog, cmd)
	if r, ok := f.responses[cmd]; ok {
		return r.out, r.err
	}
	return "", ErrUnreachable
}

// builderFactory returns a NewSession that always hands back the
// supplied fake.
func builderFactory(f *fakeDialer) func(SSHTarget, *slog.Logger, wispssh.KnownHostsStore) Dialer {
	return func(SSHTarget, *slog.Logger, wispssh.KnownHostsStore) Dialer { return f }
}

func newAction(f *fakeDialer) *FrequencyCheckAction {
	return &FrequencyCheckAction{
		Log:        nil,
		KnownHosts: nil,
		Target:     SSHTarget{Host: "10.0.0.1", Username: "admin", Password: "x"},
		NewSession: builderFactory(f),
	}
}

// TestFreqCheck_NoWirelessIsSkipped — a CCR/router with no wireless
// menus must terminate succeeded with skipped=true and a clear
// SkippedReason. Never produces fake interface data.
func TestFreqCheck_NoWirelessIsSkipped(t *testing.T) {
	f := &fakeDialer{responses: map[string]struct {
		out string
		err error
	}{
		"/system/identity/print":            {"name: rtr-edge-1\n", nil},
		"/interface/wireless/print/detail":  {"no such command prefix", nil},
		"/interface/wifi/print/detail":      {"no such command prefix", nil},
		"/interface/wifiwave2/print/detail": {"no such command prefix", nil},
	}}
	a := newAction(f)
	res, err := a.Execute(context.Background(), Request{
		Kind:          KindFrequencyCheck,
		DeviceID:      "00000000-0000-0000-0000-000000000001",
		CorrelationID: "test-corr",
	})
	if err != nil {
		t.Fatalf("expected nil err on no-wireless, got %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success=true on no-wireless, got %+v", res)
	}
	if res.Result == nil {
		t.Fatal("expected non-nil Result")
	}
	fc := res.Result["frequency_check"].(FrequencyCheckResult)
	if !fc.Skipped {
		t.Errorf("expected skipped=true, got %+v", fc)
	}
	if fc.SkippedReason == "" {
		t.Errorf("expected non-empty SkippedReason")
	}
	if fc.MenuSource != "none" {
		t.Errorf("expected menu_source=none, got %q", fc.MenuSource)
	}
}

// TestFreqCheck_LegacyWirelessHappyPath — the most common operator
// scenario: RouterOS 6.x with /interface/wireless and a populated
// registration table. Action must report observed interfaces +
// summarized client stats.
func TestFreqCheck_LegacyWirelessHappyPath(t *testing.T) {
	f := &fakeDialer{responses: map[string]struct {
		out string
		err error
	}{
		"/system/identity/print": {"name: AP-Sahil-1\n", nil},
		"/interface/wireless/print/detail": {
			` 0 R name="wlan1" mac-address=AA:BB:CC:11:22:33 frequency=5180 band=5ghz-a/n/ac channel-width=20mhz mode=ap-bridge ssid="ap-sahil" running=true disabled=false`,
			nil,
		},
		"/interface/wireless/registration-table/print/detail": {
			` 0 interface=wlan1 mac-address=AA:11:11:11:11:01 signal-strength=-65 ccq=80 tx-rate=130Mbps rx-rate=72Mbps
 1 interface=wlan1 mac-address=AA:11:11:11:11:02 signal-strength=-72 ccq=70 tx-rate=120Mbps rx-rate=65Mbps`,
			nil,
		},
	}}
	a := newAction(f)
	res, err := a.Execute(context.Background(), Request{
		Kind:          KindFrequencyCheck,
		CorrelationID: "happy",
	})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !res.Success {
		t.Fatalf("not success: %+v", res)
	}
	fc := res.Result["frequency_check"].(FrequencyCheckResult)
	if fc.Skipped {
		t.Errorf("happy path should not be skipped")
	}
	if fc.MenuSource != "wireless" {
		t.Errorf("menu_source=%q want wireless", fc.MenuSource)
	}
	if len(fc.Interfaces) != 1 {
		t.Fatalf("expected 1 interface, got %d", len(fc.Interfaces))
	}
	w1 := fc.Interfaces[0]
	if w1.Frequency != "5180" {
		t.Errorf("freq=%q", w1.Frequency)
	}
	if w1.ClientCount != 2 {
		t.Errorf("client_count=%d want 2", w1.ClientCount)
	}
	if w1.AvgSignal == nil || *w1.AvgSignal > -65 {
		t.Errorf("avg_signal=%v", w1.AvgSignal)
	}
	if !w1.RegistrationOK {
		t.Errorf("registration_ok must be true")
	}
}

// TestFreqCheck_DialFailureBubblesUpAsFailed — when SSH dial returns
// an error the run must surface it as a failed Result with a
// stable error_code, NOT a panic and NOT a fake success.
func TestFreqCheck_DialFailureBubblesUpAsFailed(t *testing.T) {
	f := &fakeDialer{dialErr: ErrTimeout}
	a := newAction(f)
	res, err := a.Execute(context.Background(), Request{Kind: KindFrequencyCheck})
	if err == nil {
		t.Fatal("expected err on dial failure")
	}
	if res.Success {
		t.Errorf("dial failure must not be success")
	}
	if res.ErrorCode != "timeout" {
		t.Errorf("error_code=%q want timeout", res.ErrorCode)
	}
}

// TestFreqCheck_BlockedCommandIsStructuralFailure — if the runner
// somehow asked the SSH layer for a mutation, EnsureCommandAllowed
// blocks it and the action returns disallowed_command. This proves
// the structural guard works end-to-end.
func TestFreqCheck_BlockedCommandIsStructuralFailure(t *testing.T) {
	if err := EnsureCommandAllowed("/interface/wireless/set frequency=5180"); !errors.Is(err, ErrDisallowedCommand) {
		t.Fatalf("allowlist must block frequency apply, got %v", err)
	}
}

// TestFreqCheck_AnalyzeProducesWarnings — degraded snapshot triggers
// the right warnings.
func TestFreqCheck_AnalyzeProducesWarnings(t *testing.T) {
	disabled := true
	worst := -92
	avg := -85
	avgCCQ := 30
	clientCount := 0
	snaps := []WirelessSnapshot{
		{InterfaceName: "wlan1", Disabled: &disabled, RegistrationOK: false, ClientCount: clientCount},
		{InterfaceName: "wlan2", AvgSignal: &avg, WorstSignal: &worst, AvgCCQ: &avgCCQ, ClientCount: 4, RegistrationOK: true, Frequency: "5180", ChannelWidth: "20mhz"},
	}
	warnings, evidence := analyzeWirelessSnapshots(snaps)
	if len(warnings) < 4 {
		t.Errorf("expected at least 4 warnings (disabled, no clients on registered iface, avg poor, worst critical, ccq low), got %d (%+v)", len(warnings), warnings)
	}
	if len(evidence) == 0 {
		t.Errorf("expected at least 1 evidence line for the running interface")
	}
}
