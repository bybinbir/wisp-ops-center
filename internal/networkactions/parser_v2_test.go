package networkactions

import "testing"

// TestParseBridgeList_RunningAndDisabled covers the basic bridge
// detail parser shape.
func TestParseBridgeList_RunningAndDisabled(t *testing.T) {
	in := ` 0 R name="br-lan" mtu=1500 running=true disabled=false
 1 X name="br-mgmt" running=false disabled=true`
	got := parseBridgeList(in)
	if len(got) != 2 {
		t.Fatalf("expected 2 bridges, got %d", len(got))
	}
	if got[0].Name != "br-lan" {
		t.Errorf("got[0].name=%q", got[0].Name)
	}
	if got[0].Running == nil || !*got[0].Running {
		t.Errorf("got[0].running not parsed")
	}
	if got[1].Disabled == nil || !*got[1].Disabled {
		t.Errorf("got[1].disabled not parsed")
	}
}

// TestParseBridgePorts_BridgeAndIface covers the port parser.
func TestParseBridgePorts_BridgeAndIface(t *testing.T) {
	in := ` 0 bridge=br-lan interface=ether2 status=enabled
 1 bridge=br-lan interface=ether3 disabled=true`
	got := parseBridgePorts(in)
	if len(got) != 2 {
		t.Fatalf("expected 2 ports, got %d", len(got))
	}
	if got[0].Bridge != "br-lan" || got[0].InterfaceName != "ether2" {
		t.Errorf("got[0]=%+v", got[0])
	}
	if got[1].Disabled == nil || !*got[1].Disabled {
		t.Errorf("got[1].disabled not parsed")
	}
}

// TestExtractClients_MaskedMACAndMetrics — client MAC must be
// masked; signal/ccq/rate parsed correctly.
func TestExtractClients_MaskedMACAndMetrics(t *testing.T) {
	in := ` 0 interface=wlan1 mac-address=AA:BB:CC:DD:EE:FF signal-strength=-70 ccq=75 tx-rate=130Mbps rx-rate=72Mbps uptime=2h45m
 1 interface=wlan2 mac-address=11:22:33:44:55:66 signal-strength=-58 ccq=88 tx-rate=270Mbps rx-rate=170Mbps`
	got := extractClients(in)
	if len(got) != 2 {
		t.Fatalf("expected 2 clients, got %d", len(got))
	}
	if got[0].MACPrefix != "AA:BB:CC:DD:EE:**" {
		t.Errorf("MAC mask wrong: %q", got[0].MACPrefix)
	}
	if got[0].Signal == nil || *got[0].Signal != -70 {
		t.Errorf("signal parse failed: %v", got[0].Signal)
	}
	if got[0].CCQ == nil || *got[0].CCQ != 75 {
		t.Errorf("ccq parse failed: %v", got[0].CCQ)
	}
	if got[0].TxRateMbps == nil || *got[0].TxRateMbps != 130 {
		t.Errorf("tx_rate parse failed: %v", got[0].TxRateMbps)
	}
	if got[0].UptimeSeconds == 0 {
		t.Errorf("uptime parse failed")
	}
	expectedSec := int64(2*3600 + 45*60)
	if got[0].UptimeSeconds != expectedSec {
		t.Errorf("uptime_seconds=%d want %d", got[0].UptimeSeconds, expectedSec)
	}
}

// TestParseUptimeToSeconds covers the units we expect from RouterOS.
func TestParseUptimeToSeconds(t *testing.T) {
	cases := map[string]int64{
		"30s":     30,
		"1m30s":   90,
		"2h45m":   2*3600 + 45*60,
		"1d2h":    1*86400 + 2*3600,
		"1w":      7 * 86400,
		"":        0,
		"garbage": 0,
	}
	for in, want := range cases {
		got := parseUptimeToSeconds(in)
		if got != want {
			t.Errorf("parseUptimeToSeconds(%q)=%d want %d", in, got, want)
		}
	}
}

// TestMaskMAC defensive cases.
func TestMaskMAC(t *testing.T) {
	cases := map[string]string{
		"AA:BB:CC:DD:EE:FF": "AA:BB:CC:DD:EE:**",
		"":                  "",
		"not-a-mac":         "not-a-mac",
		"AA:BB:CC":          "AA:BB:CC",
	}
	for in, want := range cases {
		if got := maskMAC(in); got != want {
			t.Errorf("maskMAC(%q)=%q want %q", in, got, want)
		}
	}
}
