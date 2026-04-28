package networkactions

import (
	"context"
	"log/slog"
	"testing"

	wispssh "github.com/wisp-ops-center/wisp-ops-center/internal/adapters/ssh"
)

func newBridgeAction(f *fakeDialer) *BridgeHealthCheckAction {
	return &BridgeHealthCheckAction{
		Log:        nil,
		KnownHosts: nil,
		Target:     SSHTarget{Host: "10.0.0.1", Username: "admin", Password: "x"},
		NewSession: func(SSHTarget, *slog.Logger, wispssh.KnownHostsStore) Dialer { return f },
	}
}

// TestBridge_HealthyCase — 1 bridge with 3 ports, all running, none
// disabled.
func TestBridge_HealthyCase(t *testing.T) {
	f := &fakeDialer{responses: map[string]struct {
		out string
		err error
	}{
		"/system/identity/print": {"name: br-rtr\n", nil},
		"/interface/bridge/print/detail": {
			` 0 R name="br-lan" mtu=1500 running=true disabled=false`,
			nil,
		},
		"/interface/bridge/port/print/detail": {
			` 0 bridge=br-lan interface=ether2 status=enabled
 1 bridge=br-lan interface=ether3 status=enabled
 2 bridge=br-lan interface=ether4 status=enabled`,
			nil,
		},
		"/interface/print/detail": {
			` 0 name="ether2" running=true disabled=false
 1 name="ether3" running=true disabled=false
 2 name="ether4" running=true disabled=false`,
			nil,
		},
	}}
	a := newBridgeAction(f)
	res, err := a.Execute(context.Background(), Request{Kind: KindBridgeHealthCheck})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	r := res.Result["bridge_health_check"].(BridgeHealthResult)
	if r.Skipped {
		t.Errorf("healthy case should not be skipped")
	}
	if r.BridgeCount != 1 {
		t.Errorf("bridge_count=%d want 1", r.BridgeCount)
	}
	if r.BridgePortsCount != 3 {
		t.Errorf("bridge_ports_count=%d want 3", r.BridgePortsCount)
	}
	if len(r.DownPorts) != 0 {
		t.Errorf("down_ports=%d want 0", len(r.DownPorts))
	}
	if len(r.DisabledPorts) != 0 {
		t.Errorf("disabled_ports=%d want 0", len(r.DisabledPorts))
	}
	if !res.DryRun {
		t.Errorf("dry_run must be true")
	}
}

// TestBridge_DownAndDisabledPorts — interface print shows ether2
// running=false (down) and ether3 disabled=true. Both should land
// in the right bucket.
func TestBridge_DownAndDisabledPorts(t *testing.T) {
	f := &fakeDialer{responses: map[string]struct {
		out string
		err error
	}{
		"/system/identity/print":         {"name: br-edge\n", nil},
		"/interface/bridge/print/detail": {` 0 R name="br-edge" running=true disabled=false`, nil},
		"/interface/bridge/port/print/detail": {
			` 0 bridge=br-edge interface=ether2
 1 bridge=br-edge interface=ether3 disabled=true
 2 bridge=br-edge interface=ether4`,
			nil,
		},
		"/interface/print/detail": {
			` 0 name="ether2" running=false disabled=false
 1 name="ether3" running=false disabled=true
 2 name="ether4" running=true disabled=false`,
			nil,
		},
	}}
	a := newBridgeAction(f)
	res, err := a.Execute(context.Background(), Request{Kind: KindBridgeHealthCheck})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	r := res.Result["bridge_health_check"].(BridgeHealthResult)
	if len(r.DownPorts) != 1 {
		t.Errorf("down_ports=%d want 1 (ether2 not running, not disabled)", len(r.DownPorts))
	}
	if len(r.DisabledPorts) != 1 {
		t.Errorf("disabled_ports=%d want 1 (ether3 disabled)", len(r.DisabledPorts))
	}
	if len(r.Warnings) < 2 {
		t.Errorf("expected at least 2 warnings (down + disabled), got %d", len(r.Warnings))
	}
}

// TestBridge_NoBridgeConfiguredIsSkipped — empty outputs → skipped
// with no_bridge_configured.
func TestBridge_NoBridgeConfiguredIsSkipped(t *testing.T) {
	f := &fakeDialer{responses: map[string]struct {
		out string
		err error
	}{
		"/system/identity/print":              {"name: pure-router\n", nil},
		"/interface/bridge/print/detail":      {"", nil},
		"/interface/bridge/port/print/detail": {"", nil},
		"/interface/print/detail":             {"", nil},
	}}
	a := newBridgeAction(f)
	res, err := a.Execute(context.Background(), Request{Kind: KindBridgeHealthCheck})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	r := res.Result["bridge_health_check"].(BridgeHealthResult)
	if !r.Skipped {
		t.Errorf("expected skipped=true on no-bridge device")
	}
	if r.SkippedReason != "no_bridge_configured" {
		t.Errorf("SkippedReason=%q want no_bridge_configured", r.SkippedReason)
	}
}

// TestBridge_MalformedOutputResilience — junk in response should
// not panic; we get 0 bridges and skipped path.
func TestBridge_MalformedOutputResilience(t *testing.T) {
	f := &fakeDialer{responses: map[string]struct {
		out string
		err error
	}{
		"/system/identity/print":              {"name: malformed\n", nil},
		"/interface/bridge/print/detail":      {"random garbage no records here\nFlags: X\n   ", nil},
		"/interface/bridge/port/print/detail": {"# header only", nil},
		"/interface/print/detail":             {"# nothing", nil},
	}}
	a := newBridgeAction(f)
	res, err := a.Execute(context.Background(), Request{Kind: KindBridgeHealthCheck})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	r := res.Result["bridge_health_check"].(BridgeHealthResult)
	if r.BridgeCount != 0 || r.BridgePortsCount != 0 {
		t.Errorf("expected 0/0, got %d/%d", r.BridgeCount, r.BridgePortsCount)
	}
	if !r.Skipped {
		t.Errorf("malformed output should land in skipped path")
	}
}
