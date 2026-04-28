package networkactions

import (
	"bufio"
	"strconv"
	"strings"
)

// parseDetailPrint decodes a RouterOS `print detail` style output
// into a slice of attribute maps. Same algorithm as the dude
// package; duplicated here so this package has no transport-layer
// dependency on dude.
func parseDetailPrint(out string) []map[string]string {
	var records []map[string]string
	var current map[string]string

	scanner := bufio.NewScanner(strings.NewReader(out))
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "Flags:") {
			continue
		}
		if isRecordStart(line) {
			if current != nil && len(current) > 0 {
				records = append(records, current)
			}
			current = map[string]string{}
			line = stripIndexAndFlags(line)
		} else if current == nil {
			continue
		}
		parseKV(line, current)
	}
	if current != nil && len(current) > 0 {
		records = append(records, current)
	}
	return records
}

func isRecordStart(line string) bool {
	t := strings.TrimLeft(line, " \t")
	if t == "" {
		return false
	}
	i := 0
	for i < len(t) && t[i] >= '0' && t[i] <= '9' {
		i++
	}
	if i == 0 {
		return false
	}
	if i < len(t) && t[i] != ' ' {
		return false
	}
	return true
}

func stripIndexAndFlags(line string) string {
	t := strings.TrimLeft(line, " \t")
	i := 0
	for i < len(t) && t[i] >= '0' && t[i] <= '9' {
		i++
	}
	t = t[i:]
	t = strings.TrimLeft(t, " \t")
	for {
		if len(t) >= 2 && t[1] == ' ' && isLetter(t[0]) {
			t = t[2:]
			t = strings.TrimLeft(t, " \t")
			continue
		}
		break
	}
	return t
}

func isLetter(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}

func parseKV(line string, dst map[string]string) {
	i := 0
	n := len(line)
	for i < n {
		for i < n && (line[i] == ' ' || line[i] == '\t') {
			i++
		}
		if i >= n {
			return
		}
		ks := i
		for i < n && line[i] != '=' && line[i] != ' ' {
			i++
		}
		if i >= n || line[i] != '=' {
			return
		}
		key := strings.TrimSpace(line[ks:i])
		i++
		if i >= n {
			dst[key] = ""
			return
		}
		var val string
		if line[i] == '"' {
			i++
			vs := i
			for i < n && line[i] != '"' {
				i++
			}
			val = line[vs:i]
			if i < n {
				i++
			}
		} else {
			vs := i
			for i < n && line[i] != ' ' && line[i] != '\t' {
				i++
			}
			val = line[vs:i]
		}
		if key != "" {
			dst[key] = val
		}
	}
}

// ----------------------------------------------------------------------------
// Wireless interface parsers
// ----------------------------------------------------------------------------

// parseWirelessLegacy parses /interface/wireless/print/detail
// (RouterOS 6.x). Each record is one wireless interface; we extract
// the fields needed for FrequencyCheckResult. Unknown fields stay in
// raw and are dropped before the result hits the DB.
func parseWirelessLegacy(out string) []WirelessSnapshot {
	records := parseDetailPrint(out)
	snaps := make([]WirelessSnapshot, 0, len(records))
	for _, r := range records {
		s := WirelessSnapshot{
			InterfaceName: pickFirst(r, "name", "default-name"),
			RadioType:     pickFirst(r, "interface-type"),
			Frequency:     pickFirst(r, "frequency"),
			Band:          pickFirst(r, "band"),
			ChannelWidth:  pickFirst(r, "channel-width"),
			Mode:          pickFirst(r, "mode"),
			SSID:          pickFirst(r, "ssid"),
		}
		if v, ok := r["running"]; ok {
			b := isTrue(v)
			s.Running = &b
		}
		if v, ok := r["disabled"]; ok {
			b := isTrue(v)
			s.Disabled = &b
		}
		snaps = append(snaps, s)
	}
	return snaps
}

// parseWifi parses /interface/wifi/print/detail (RouterOS 7.x
// "compact" wifi menu). Field names differ from the legacy menu —
// we accept both spellings where ambiguous.
func parseWifi(out string) []WirelessSnapshot {
	records := parseDetailPrint(out)
	snaps := make([]WirelessSnapshot, 0, len(records))
	for _, r := range records {
		s := WirelessSnapshot{
			InterfaceName: pickFirst(r, "name"),
			RadioType:     "wifi",
			Frequency:     pickFirst(r, "channel.frequency", "frequency"),
			Band:          pickFirst(r, "channel.band", "band"),
			ChannelWidth:  pickFirst(r, "channel.width", "channel-width"),
			Mode:          pickFirst(r, "mode", "configuration.mode"),
			SSID:          pickFirst(r, "configuration.ssid", "ssid"),
		}
		if v, ok := r["running"]; ok {
			b := isTrue(v)
			s.Running = &b
		}
		if v, ok := r["disabled"]; ok {
			b := isTrue(v)
			s.Disabled = &b
		}
		snaps = append(snaps, s)
	}
	return snaps
}

// parseWifiwave2 parses /interface/wifiwave2/print/detail (RouterOS
// 7.x preview menu). Most field names are the same as wifi but some
// are nested under "configuration.".
func parseWifiwave2(out string) []WirelessSnapshot {
	records := parseDetailPrint(out)
	snaps := make([]WirelessSnapshot, 0, len(records))
	for _, r := range records {
		s := WirelessSnapshot{
			InterfaceName: pickFirst(r, "name"),
			RadioType:     "wifiwave2",
			Frequency:     pickFirst(r, "configuration.frequency", "frequency"),
			Band:          pickFirst(r, "configuration.band", "band"),
			ChannelWidth:  pickFirst(r, "configuration.channel-width", "channel-width"),
			Mode:          pickFirst(r, "configuration.mode", "mode"),
			SSID:          pickFirst(r, "configuration.ssid", "ssid"),
		}
		if v, ok := r["running"]; ok {
			b := isTrue(v)
			s.Running = &b
		}
		if v, ok := r["disabled"]; ok {
			b := isTrue(v)
			s.Disabled = &b
		}
		snaps = append(snaps, s)
	}
	return snaps
}

// regSummary is the aggregate for one wireless interface's
// registration table — count of clients + signal/ccq/rate stats.
type regSummary struct {
	count       int
	avgSignal   *int
	worstSignal *int
	avgCCQ      *int
	avgTxMbps   *int
	avgRxMbps   *int
}

// summarizeRegistration aggregates a /registration-table/print/detail
// output across interfaces. The returned map is keyed by interface
// name. Works for legacy + wifi + wifiwave2 — the field name we look
// for is always "interface".
func summarizeRegistration(out string) map[string]regSummary {
	records := parseDetailPrint(out)
	by := map[string][]map[string]string{}
	for _, r := range records {
		iface := pickFirst(r, "interface")
		if iface == "" {
			continue
		}
		by[iface] = append(by[iface], r)
	}
	out2 := make(map[string]regSummary, len(by))
	for iface, list := range by {
		out2[iface] = aggregateRegistration(list)
	}
	return out2
}

func aggregateRegistration(records []map[string]string) regSummary {
	if len(records) == 0 {
		return regSummary{}
	}
	var sumSig, sumCCQ, sumTx, sumRx int
	var nSig, nCCQ, nTx, nRx int
	var worst *int
	for _, r := range records {
		// Signal strength keys vary across menus.
		if v := pickFirst(r, "signal-strength", "signal", "rssi"); v != "" {
			n := parseSignedInt(v)
			if n != nil {
				sumSig += *n
				nSig++
				if worst == nil || *n < *worst {
					nv := *n
					worst = &nv
				}
			}
		}
		if v := pickFirst(r, "ccq", "tx-ccq"); v != "" {
			if n := parsePercent(v); n != nil {
				sumCCQ += *n
				nCCQ++
			}
		}
		if v := pickFirst(r, "tx-rate"); v != "" {
			if n := parseMbps(v); n != nil {
				sumTx += *n
				nTx++
			}
		}
		if v := pickFirst(r, "rx-rate"); v != "" {
			if n := parseMbps(v); n != nil {
				sumRx += *n
				nRx++
			}
		}
	}
	s := regSummary{count: len(records)}
	if nSig > 0 {
		v := sumSig / nSig
		s.avgSignal = &v
	}
	if worst != nil {
		s.worstSignal = worst
	}
	if nCCQ > 0 {
		v := sumCCQ / nCCQ
		s.avgCCQ = &v
	}
	if nTx > 0 {
		v := sumTx / nTx
		s.avgTxMbps = &v
	}
	if nRx > 0 {
		v := sumRx / nRx
		s.avgRxMbps = &v
	}
	return s
}

// ----------------------------------------------------------------------------
// Phase 9 v2 — bridge + per-client parsers
// ----------------------------------------------------------------------------

// parseBridgeList parses /interface/bridge/print/detail.
func parseBridgeList(out string) []BridgeStat {
	records := parseDetailPrint(out)
	bs := make([]BridgeStat, 0, len(records))
	for _, r := range records {
		s := BridgeStat{Name: pickFirst(r, "name")}
		if v, ok := r["running"]; ok {
			b := isTrue(v)
			s.Running = &b
		}
		if v, ok := r["disabled"]; ok {
			b := isTrue(v)
			s.Disabled = &b
		}
		bs = append(bs, s)
	}
	return bs
}

// parseBridgePorts parses /interface/bridge/port/print/detail.
func parseBridgePorts(out string) []BridgePort {
	records := parseDetailPrint(out)
	ports := make([]BridgePort, 0, len(records))
	for _, r := range records {
		p := BridgePort{
			Bridge:        pickFirst(r, "bridge"),
			InterfaceName: pickFirst(r, "interface"),
			Status:        pickFirst(r, "status"),
		}
		if v, ok := r["disabled"]; ok {
			b := isTrue(v)
			p.Disabled = &b
		}
		// running on a bridge port may come from the parent interface
		// listing; we leave it nil here and let the action correlate
		// against /interface/print/detail.
		ports = append(ports, p)
	}
	return ports
}

// extractClients walks a registration-table print/detail output and
// returns one ClientStat per row. Used by ap_client_test for the
// per-client metrics. MAC is masked to the first 5 bytes
// ("AA:BB:CC:DD:EE") so the result jsonb does not carry fully
// resolvable customer device identifiers.
func extractClients(out string) []ClientStat {
	records := parseDetailPrint(out)
	clients := make([]ClientStat, 0, len(records))
	for _, r := range records {
		c := ClientStat{
			MACPrefix:     maskMAC(pickFirst(r, "mac-address")),
			InterfaceName: pickFirst(r, "interface"),
		}
		if v := pickFirst(r, "signal-strength", "signal", "rssi"); v != "" {
			c.Signal = parseSignedInt(v)
		}
		if v := pickFirst(r, "ccq", "tx-ccq"); v != "" {
			c.CCQ = parsePercent(v)
		}
		if v := pickFirst(r, "tx-rate"); v != "" {
			c.TxRateMbps = parseMbps(v)
		}
		if v := pickFirst(r, "rx-rate"); v != "" {
			c.RxRateMbps = parseMbps(v)
		}
		if v := pickFirst(r, "uptime"); v != "" {
			c.UptimeSeconds = parseUptimeToSeconds(v)
		}
		clients = append(clients, c)
	}
	return clients
}

// maskMAC truncates "AA:BB:CC:DD:EE:FF" → "AA:BB:CC:DD:EE:**". Empty
// input passes through. Anything that isn't 6 ":"-separated octets
// is left as-is (defensive).
func maskMAC(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	parts := strings.Split(s, ":")
	if len(parts) != 6 {
		return s
	}
	return strings.Join(parts[:5], ":") + ":**"
}

// parseUptimeToSeconds accepts RouterOS-style "1d2h3m4s" / "2h45m" /
// "30s" and returns total seconds. 0 on parse failure.
func parseUptimeToSeconds(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	var total int64
	var num int64
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= '0' && c <= '9' {
			num = num*10 + int64(c-'0')
			continue
		}
		switch c {
		case 'w':
			total += num * 7 * 86400
		case 'd':
			total += num * 86400
		case 'h':
			total += num * 3600
		case 'm':
			total += num * 60
		case 's':
			total += num
		}
		num = 0
	}
	return total
}

// ----------------------------------------------------------------------------
// helpers
// ----------------------------------------------------------------------------

func pickFirst(r map[string]string, keys ...string) string {
	for _, k := range keys {
		if v, ok := r[k]; ok && v != "" {
			return v
		}
	}
	return ""
}

func isTrue(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "yes", "1", "running":
		return true
	}
	return false
}

// parseSignedInt accepts "-78", "-78dBm", "-78 dBm" and returns -78.
func parseSignedInt(s string) *int {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	// Strip non-numeric suffix.
	end := len(s)
	for i, c := range s {
		if i == 0 && (c == '-' || c == '+') {
			continue
		}
		if c < '0' || c > '9' {
			end = i
			break
		}
	}
	if end == 0 {
		return nil
	}
	n, err := strconv.Atoi(s[:end])
	if err != nil {
		return nil
	}
	return &n
}

// parsePercent accepts "85", "85%", "85.5%" and returns the integer
// part.
func parsePercent(s string) *int {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "%")
	if i := strings.Index(s, "."); i >= 0 {
		s = s[:i]
	}
	if n, err := strconv.Atoi(s); err == nil {
		return &n
	}
	return nil
}

// parseMbps accepts "150Mbps-20MHz/SGI/3S/MCS21",
// "150Mbps", "1.5Gbps" and returns the integer Mbps.
func parseMbps(s string) *int {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	low := strings.ToLower(s)
	// Cut at first non-digit/decimal.
	end := len(low)
	for i, c := range low {
		if (c >= '0' && c <= '9') || c == '.' {
			continue
		}
		end = i
		break
	}
	if end == 0 {
		return nil
	}
	num, err := strconv.ParseFloat(low[:end], 64)
	if err != nil {
		return nil
	}
	rest := low[end:]
	switch {
	case strings.HasPrefix(rest, "gbps"):
		num *= 1000
	case strings.HasPrefix(rest, "mbps"):
		// already Mbps
	case strings.HasPrefix(rest, "kbps"):
		num /= 1000
	}
	v := int(num)
	return &v
}
