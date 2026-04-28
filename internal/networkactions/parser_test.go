package networkactions

import "testing"

// TestParseWirelessLegacy walks the RouterOS 6.x menu output and
// proves the freq/band/channel-width fields land in the snapshot.
func TestParseWirelessLegacy(t *testing.T) {
	in := `Flags: X - disabled, R - running
 0 R name="wlan1" mac-address=AA:BB:CC:11:22:33 frequency=5180 band=5ghz-a/n/ac channel-width=20mhz mode=ap-bridge ssid="my-ssid" running=true disabled=false`
	got := parseWirelessLegacy(in)
	if len(got) != 1 {
		t.Fatalf("expected 1 record, got %d (%+v)", len(got), got)
	}
	g := got[0]
	if g.InterfaceName != "wlan1" {
		t.Errorf("name=%q", g.InterfaceName)
	}
	if g.Frequency != "5180" {
		t.Errorf("frequency=%q", g.Frequency)
	}
	if g.Band != "5ghz-a/n/ac" {
		t.Errorf("band=%q", g.Band)
	}
	if g.ChannelWidth != "20mhz" {
		t.Errorf("width=%q", g.ChannelWidth)
	}
	if g.Mode != "ap-bridge" {
		t.Errorf("mode=%q", g.Mode)
	}
	if g.SSID != "my-ssid" {
		t.Errorf("ssid=%q", g.SSID)
	}
	if g.Running == nil || !*g.Running {
		t.Errorf("running not parsed")
	}
}

// TestParseWifi covers the RouterOS 7.x compact /interface/wifi menu
// where common fields live under "channel.frequency" / "channel.band".
func TestParseWifi(t *testing.T) {
	in := ` 0 name=wifi1 channel.frequency=5180 channel.band=5ghz-ax channel.width=80mhz configuration.mode=ap configuration.ssid="acme" running=true disabled=false`
	got := parseWifi(in)
	if len(got) != 1 {
		t.Fatalf("expected 1 wifi record, got %d", len(got))
	}
	g := got[0]
	if g.RadioType != "wifi" {
		t.Errorf("radio_type=%q", g.RadioType)
	}
	if g.Frequency != "5180" {
		t.Errorf("frequency=%q", g.Frequency)
	}
	if g.Band != "5ghz-ax" {
		t.Errorf("band=%q", g.Band)
	}
	if g.ChannelWidth != "80mhz" {
		t.Errorf("width=%q", g.ChannelWidth)
	}
	if g.SSID != "acme" {
		t.Errorf("ssid=%q", g.SSID)
	}
}

// TestParseWifiwave2 proves the wifiwave2 preview menu parses with
// configuration.* prefixed fields.
func TestParseWifiwave2(t *testing.T) {
	in := ` 0 name=wifi-w2-1 configuration.frequency=5500 configuration.band=5ghz-ax configuration.channel-width=40mhz configuration.mode=ap configuration.ssid="lab" running=true`
	got := parseWifiwave2(in)
	if len(got) != 1 {
		t.Fatalf("expected 1 wifiwave2 record, got %d", len(got))
	}
	g := got[0]
	if g.RadioType != "wifiwave2" {
		t.Errorf("radio_type=%q", g.RadioType)
	}
	if g.Frequency != "5500" {
		t.Errorf("frequency=%q", g.Frequency)
	}
	if g.Band != "5ghz-ax" {
		t.Errorf("band=%q", g.Band)
	}
	if g.ChannelWidth != "40mhz" {
		t.Errorf("width=%q", g.ChannelWidth)
	}
}

// TestParseRegistration_AggregatesPerInterface validates the
// per-interface aggregator: count, avg signal, worst signal, ccq.
func TestParseRegistration_AggregatesPerInterface(t *testing.T) {
	in := ` 0 interface=wlan1 mac-address=AA:11:11:11:11:01 signal-strength=-65 ccq=80 tx-rate=130Mbps rx-rate=72Mbps
 1 interface=wlan1 mac-address=AA:11:11:11:11:02 signal-strength=-72 ccq=70 tx-rate=120Mbps rx-rate=65Mbps
 2 interface=wlan1 mac-address=AA:11:11:11:11:03 signal-strength=-90 ccq=40 tx-rate=10Mbps rx-rate=5Mbps
 3 interface=wlan2 mac-address=AA:11:11:11:11:04 signal-strength=-55 ccq=85 tx-rate=200Mbps rx-rate=150Mbps`
	got := summarizeRegistration(in)
	w1, ok := got["wlan1"]
	if !ok {
		t.Fatalf("wlan1 missing")
	}
	if w1.count != 3 {
		t.Errorf("wlan1 count=%d, want 3", w1.count)
	}
	if w1.avgSignal == nil || *w1.avgSignal > -70 || *w1.avgSignal < -80 {
		t.Errorf("wlan1 avgSignal off: %v", w1.avgSignal)
	}
	if w1.worstSignal == nil || *w1.worstSignal != -90 {
		t.Errorf("wlan1 worstSignal=%v want -90", w1.worstSignal)
	}
	if w1.avgCCQ == nil || *w1.avgCCQ != 63 {
		t.Errorf("wlan1 avgCCQ=%v want 63", w1.avgCCQ)
	}
	w2, ok := got["wlan2"]
	if !ok || w2.count != 1 {
		t.Errorf("wlan2 missing or count wrong: %+v", w2)
	}
}

// TestParseSignedInt covers stripped-suffix variants.
func TestParseSignedInt(t *testing.T) {
	cases := map[string]int{"-78": -78, "-78dBm": -78, "-78 dBm": -78, "85": 85}
	for in, want := range cases {
		got := parseSignedInt(in)
		if got == nil || *got != want {
			t.Errorf("parseSignedInt(%q)=%v want %d", in, got, want)
		}
	}
	if parseSignedInt("garbage") != nil {
		t.Errorf("garbage should be nil")
	}
}

// TestParseMbps covers Mbps/Kbps/Gbps suffixes.
func TestParseMbps(t *testing.T) {
	cases := map[string]int{"150Mbps": 150, "1.5Gbps": 1500, "500kbps": 0, "150Mbps-20MHz/SGI/3S/MCS21": 150}
	for in, want := range cases {
		got := parseMbps(in)
		if got == nil || *got != want {
			t.Errorf("parseMbps(%q)=%v want %d", in, got, want)
		}
	}
}
