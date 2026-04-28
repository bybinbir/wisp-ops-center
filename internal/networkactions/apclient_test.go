package networkactions

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	wispssh "github.com/wisp-ops-center/wisp-ops-center/internal/adapters/ssh"
)

func newAPClientAction(f *fakeDialer) *APClientTestAction {
	return &APClientTestAction{
		Log:        nil,
		KnownHosts: nil,
		Target:     SSHTarget{Host: "10.0.0.1", Username: "admin", Password: "x"},
		NewSession: func(SSHTarget, *slog.Logger, wispssh.KnownHostsStore) Dialer { return f },
	}
}

// TestAPClient_HappyPathWithWeakAndLowCCQClients — registration table
// has multiple clients; action must classify weak (signal < -80) and
// low-ccq (< 50%) ones into the right buckets.
func TestAPClient_HappyPathWithWeakAndLowCCQClients(t *testing.T) {
	f := &fakeDialer{responses: map[string]struct {
		out string
		err error
	}{
		"/system/identity/print": {"name: AP-Sahil-1\n", nil},
		"/interface/wireless/print/detail": {
			` 0 R name="wlan1" frequency=5180 band=5ghz-a/n/ac channel-width=20mhz mode=ap-bridge ssid="x" running=true disabled=false`,
			nil,
		},
		"/interface/wireless/registration-table/print/detail": {
			` 0 interface=wlan1 mac-address=AA:11:11:11:11:01 signal-strength=-65 ccq=80 tx-rate=130Mbps rx-rate=72Mbps uptime=2h45m
 1 interface=wlan1 mac-address=AA:11:11:11:11:02 signal-strength=-85 ccq=70 tx-rate=20Mbps rx-rate=10Mbps uptime=15m
 2 interface=wlan1 mac-address=AA:11:11:11:11:03 signal-strength=-72 ccq=40 tx-rate=60Mbps rx-rate=30Mbps uptime=1d`,
			nil,
		},
	}}
	a := newAPClientAction(f)
	res, err := a.Execute(context.Background(), Request{
		Kind:          KindAPClientTest,
		CorrelationID: "ap-happy",
	})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !res.Success {
		t.Fatalf("not success: %+v", res)
	}
	if !res.DryRun {
		t.Errorf("dry_run must be true unconditionally")
	}
	r := res.Result["ap_client_test"].(APClientTestResult)
	if r.Skipped {
		t.Errorf("happy path should not be skipped")
	}
	if r.MenuSource != "wireless" {
		t.Errorf("menu_source=%q want wireless", r.MenuSource)
	}
	if r.ClientCount != 3 {
		t.Errorf("client_count=%d want 3", r.ClientCount)
	}
	if len(r.WeakClients) != 1 {
		t.Errorf("weak clients = %d want 1 (only -85 below -80 threshold)", len(r.WeakClients))
	}
	if len(r.LowCCQClients) != 1 {
		t.Errorf("low ccq clients = %d want 1 (only 40%% below 50%% threshold)", len(r.LowCCQClients))
	}
	if r.AvgSignal == nil || *r.AvgSignal > -65 {
		t.Errorf("avg_signal off: %v", r.AvgSignal)
	}
	if r.WorstSignal == nil || *r.WorstSignal != -85 {
		t.Errorf("worst_signal=%v want -85", r.WorstSignal)
	}
	// MAC must be masked in result jsonb.
	for _, c := range r.WeakClients {
		if !strings.HasSuffix(c.MACPrefix, "**") {
			t.Errorf("weak client MAC not masked: %q", c.MACPrefix)
		}
	}
}

// TestAPClient_NoClientsIsSkipped — happy SSH but empty registration
// table → succeeded + skipped + reason. No fake clients.
func TestAPClient_NoClientsIsSkipped(t *testing.T) {
	f := &fakeDialer{responses: map[string]struct {
		out string
		err error
	}{
		"/system/identity/print":                              {"name: rtr-1\n", nil},
		"/interface/wireless/print/detail":                    {` 0 R name="wlan1" frequency=5180 mode=ap-bridge running=true`, nil},
		"/interface/wireless/registration-table/print/detail": {"", nil},
	}}
	a := newAPClientAction(f)
	res, err := a.Execute(context.Background(), Request{Kind: KindAPClientTest})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	r := res.Result["ap_client_test"].(APClientTestResult)
	if !r.Skipped {
		t.Errorf("expected skipped=true on empty registration")
	}
	if r.SkippedReason == "" {
		t.Errorf("expected non-empty SkippedReason")
	}
	if r.ClientCount != 0 {
		t.Errorf("client_count=%d want 0", r.ClientCount)
	}
}

// TestAPClient_NoWirelessMenuIsSkipped — RouterOS without wireless
// menus → skipped with no_wireless_menu.
func TestAPClient_NoWirelessMenuIsSkipped(t *testing.T) {
	f := &fakeDialer{responses: map[string]struct {
		out string
		err error
	}{
		"/system/identity/print":            {"name: core-1\n", nil},
		"/interface/wireless/print/detail":  {"no such command prefix", nil},
		"/interface/wifi/print/detail":      {"no such command prefix", nil},
		"/interface/wifiwave2/print/detail": {"no such command prefix", nil},
	}}
	a := newAPClientAction(f)
	res, err := a.Execute(context.Background(), Request{Kind: KindAPClientTest})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	r := res.Result["ap_client_test"].(APClientTestResult)
	if !r.Skipped || r.SkippedReason != "no_wireless_menu" {
		t.Errorf("expected skipped=no_wireless_menu, got %+v", r)
	}
}

// TestAPClient_DialFailureBubblesUpAsFailed — SSH dial error must
// surface as failed with stable error_code; no panic, no fake data.
func TestAPClient_DialFailureBubblesUpAsFailed(t *testing.T) {
	f := &fakeDialer{dialErr: ErrTimeout}
	a := newAPClientAction(f)
	res, err := a.Execute(context.Background(), Request{Kind: KindAPClientTest})
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
