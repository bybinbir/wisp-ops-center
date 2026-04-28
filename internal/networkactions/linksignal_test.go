package networkactions

import (
	"context"
	"log/slog"
	"testing"

	wispssh "github.com/wisp-ops-center/wisp-ops-center/internal/adapters/ssh"
)

func newLinkAction(f *fakeDialer) *LinkSignalTestAction {
	return &LinkSignalTestAction{
		Log:        nil,
		KnownHosts: nil,
		Target:     SSHTarget{Host: "10.0.0.1", Username: "admin", Password: "x"},
		NewSession: func(SSHTarget, *slog.Logger, wispssh.KnownHostsStore) Dialer { return f },
	}
}

// TestLinkSignal_PtPHealthy — wireless interface in bridge mode
// with one peer at -55 dBm should classify as healthy.
func TestLinkSignal_PtPHealthy(t *testing.T) {
	f := &fakeDialer{responses: map[string]struct {
		out string
		err error
	}{
		"/system/identity/print": {"name: PTP-link-1\n", nil},
		"/interface/wireless/print/detail": {
			` 0 R name="wlan1" frequency=5500 channel-width=20mhz mode=bridge ssid="ptp-1" running=true disabled=false`,
			nil,
		},
		"/interface/wireless/registration-table/print/detail": {
			` 0 interface=wlan1 mac-address=BB:11:22:33:44:55 signal-strength=-55 ccq=85 tx-rate=300Mbps rx-rate=270Mbps uptime=1d3h`,
			nil,
		},
	}}
	a := newLinkAction(f)
	res, err := a.Execute(context.Background(), Request{Kind: KindLinkSignalTest, CorrelationID: "link-healthy"})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	r := res.Result["link_signal_test"].(LinkSignalTestResult)
	if !r.LinkDetected {
		t.Fatalf("expected link_detected=true: %+v", r)
	}
	if r.HealthStatus != "healthy" {
		t.Errorf("health_status=%q want healthy", r.HealthStatus)
	}
	if r.LocalInterface != "wlan1" {
		t.Errorf("local_interface=%q", r.LocalInterface)
	}
	if r.Signal == nil || *r.Signal != -55 {
		t.Errorf("signal=%v want -55", r.Signal)
	}
	if r.RemoteIdentifier == "" {
		t.Errorf("expected remote_identifier (masked MAC)")
	}
	if !res.DryRun {
		t.Errorf("dry_run must be true")
	}
}

// TestLinkSignal_CriticalSignal — peer at -90 dBm → critical.
func TestLinkSignal_CriticalSignal(t *testing.T) {
	f := &fakeDialer{responses: map[string]struct {
		out string
		err error
	}{
		"/system/identity/print":           {"name: weak-link\n", nil},
		"/interface/wireless/print/detail": {` 0 R name="wlan1" mode=bridge running=true`, nil},
		"/interface/wireless/registration-table/print/detail": {
			` 0 interface=wlan1 mac-address=AA:11:22:33:44:01 signal-strength=-90 ccq=30 tx-rate=10Mbps rx-rate=5Mbps`,
			nil,
		},
	}}
	a := newLinkAction(f)
	res, err := a.Execute(context.Background(), Request{Kind: KindLinkSignalTest})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	r := res.Result["link_signal_test"].(LinkSignalTestResult)
	if r.HealthStatus != "critical" {
		t.Errorf("health_status=%q want critical", r.HealthStatus)
	}
	if len(r.Warnings) == 0 {
		t.Errorf("expected at least one warning")
	}
}

// TestLinkSignal_NoPeerIsSkipped — no registered peer on any
// interface → succeeded with skipped=true.
func TestLinkSignal_NoPeerIsSkipped(t *testing.T) {
	f := &fakeDialer{responses: map[string]struct {
		out string
		err error
	}{
		"/system/identity/print":                              {"name: lonely\n", nil},
		"/interface/wireless/print/detail":                    {` 0 R name="wlan1" mode=ap-bridge running=true`, nil},
		"/interface/wireless/registration-table/print/detail": {"", nil},
	}}
	a := newLinkAction(f)
	res, err := a.Execute(context.Background(), Request{Kind: KindLinkSignalTest})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	r := res.Result["link_signal_test"].(LinkSignalTestResult)
	if !r.Skipped {
		t.Errorf("expected skipped=true")
	}
	if r.HealthStatus != "unknown" {
		t.Errorf("health_status=%q want unknown", r.HealthStatus)
	}
}

// TestLinkSignal_DialFailureBubblesUpAsFailed — same shape as
// other actions for stable error_code.
func TestLinkSignal_DialFailureBubblesUpAsFailed(t *testing.T) {
	f := &fakeDialer{dialErr: ErrAuth}
	a := newLinkAction(f)
	res, err := a.Execute(context.Background(), Request{Kind: KindLinkSignalTest})
	if err == nil {
		t.Fatal("expected err on dial failure")
	}
	if res.Success {
		t.Errorf("dial failure must not be success")
	}
	if res.ErrorCode != "auth_failed" {
		t.Errorf("error_code=%q want auth_failed", res.ErrorCode)
	}
}
