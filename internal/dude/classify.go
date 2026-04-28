package dude

import (
	"sort"
	"strings"
)

// Classify assigns a category + confidence to a discovered device
// using evidence weights. Phase 8.1 reworked this from a simple
// per-bucket score into a "best-of-evidence" model that also
// distinguishes name-only weak signals from MAC+platform strong
// signals, and detects conflicting evidence to drop confidence.
//
// Rules of thumb the design tries to honor:
//
//   - Name-only weak hint (e.g. "AP-Sahil-1" with nothing else) is
//     not enough for high confidence — it stays under 50.
//   - MAC + platform + identity + interface evidence pushes confidence
//     into the 75-95 range.
//   - When two categories tie within ~20 points, confidence is
//     reduced by the conflict factor and a "conflict" evidence note
//     is appended.
//   - Without ANY signal the device stays Unknown with confidence=0
//     so the operator sees the gap clearly.
func Classify(d *DiscoveredDevice) {
	scores := map[Category]int{}
	var ev []Evidence

	add := func(cat Category, w int, h, reason string) {
		scores[cat] += w
		ev = append(ev, Evidence{Heuristic: h, Weight: w, Reason: reason})
	}

	name := strings.ToLower(d.Name)
	model := strings.ToLower(d.Model)
	board := strings.ToLower(d.Board)
	dtype := strings.ToLower(d.Type)
	platform := strings.ToLower(d.Platform)
	identity := strings.ToLower(d.Identity)
	iface := strings.ToLower(d.InterfaceName)
	raw := lowerKeyMap(d.Raw)

	// ---- Dude-reported type (strongest signal when present) ------------
	switch dtype {
	case "ap", "wifiap", "wireless-ap":
		add(CategoryAP, 50, "dude_type", "Dude reports type=ap")
	case "router":
		add(CategoryRouter, 35, "dude_type", "Dude reports type=router")
	case "switch":
		add(CategorySwitch, 35, "dude_type", "Dude reports type=switch")
	case "bridge":
		add(CategoryBridge, 35, "dude_type", "Dude reports type=bridge")
	case "cpe", "client":
		add(CategoryCPE, 35, "dude_type", "Dude reports type=cpe")
	}

	// ---- Name patterns (weak by themselves) -----------------------------
	switch {
	case containsAny(name, "ap-", "ap_", " ap ", "sektor", "sektör", "sector", "tower", "baz-"):
		add(CategoryAP, 30, "name_hint_ap", "Name hints AP/sector/tower")
	case containsAny(name, "cpe-", "cpe_", "musteri", "müşteri", "customer", "abone", "client-"):
		add(CategoryCPE, 30, "name_hint_cpe", "Name hints CPE/customer/abone")
	case containsAny(name, "ptp-", "ptp_", "link-", "link_", "bh-", "bh_", "backhaul", "uplink", "p2p", "relay"):
		add(CategoryBackhaul, 35, "name_hint_link", "Name hints backhaul/link/relay")
	case startsWithAny(name, "br-", "br_", "bridge-", "bridge_", "switch-bridge"):
		add(CategoryBridge, 25, "name_hint_bridge", "Name hints bridge")
	case startsWithAny(name, "rtr-", "router-", "core-", "edge-", "gw-", "gateway-"):
		add(CategoryRouter, 30, "name_hint_router", "Name hints router/core/edge")
	case startsWithAny(name, "sw-", "switch-"):
		add(CategorySwitch, 25, "name_hint_switch", "Name hints switch")
	case startsWithAny(name, "pop-"):
		add(CategoryRouter, 20, "name_hint_pop", "Name hints PoP/aggregation")
	}

	// ---- Wireless mode (RouterOS interface evidence) --------------------
	if v, ok := raw["wireless-mode"]; ok {
		v = strings.ToLower(v)
		switch {
		case strings.Contains(v, "ap-bridge") || strings.Contains(v, "wds-slave-ap") || strings.Contains(v, "bridge-ap"):
			add(CategoryAP, 35, "wireless_mode_ap", "Wireless mode reports AP role")
		case strings.Contains(v, "station-pseudobridge") || strings.Contains(v, "station-bridge"):
			add(CategoryCPE, 25, "wireless_mode_station_bridge", "Station+bridge = client/CPE")
		case strings.Contains(v, "station") || strings.Contains(v, "client"):
			add(CategoryCPE, 30, "wireless_mode_station", "Wireless mode reports station/CPE")
		case strings.Contains(v, "bridge"):
			add(CategoryBackhaul, 25, "wireless_mode_bridge", "Wireless mode bridge — likely PtP backhaul")
		}
	}

	// ---- Interface type ------------------------------------------------
	if v, ok := raw["interface-type"]; ok {
		if strings.Contains(strings.ToLower(v), "bridge") {
			add(CategoryBridge, 20, "interface_type_bridge", "Interface-type contains bridge")
		}
	}

	// ---- Platform / identity / board (RouterOS provenance) -------------
	if platform != "" {
		switch {
		case strings.Contains(platform, "routeros") || strings.Contains(platform, "mikrotik"):
			// RouterOS does not by itself imply a category, but it
			// strengthens whichever name/wireless hint is present —
			// add a small boost to the leading bucket via the
			// accumulator below (no direct add here).
			ev = append(ev, Evidence{Heuristic: "platform_routeros", Weight: 5,
				Reason: "Platform reports RouterOS / MikroTik"})
		case strings.Contains(platform, "mimosa"):
			ev = append(ev, Evidence{Heuristic: "platform_mimosa", Weight: 5,
				Reason: "Platform reports Mimosa"})
		case strings.Contains(platform, "ubiquiti") || strings.Contains(platform, "airos"):
			ev = append(ev, Evidence{Heuristic: "platform_airos", Weight: 5,
				Reason: "Platform reports Ubiquiti / airOS"})
		}
	}
	if identity != "" {
		switch {
		case strings.Contains(identity, "mikrotik") || strings.Contains(identity, "routerboard"):
			add(CategoryRouter, 15, "identity_routerboard", "Identity reports RouterBoard")
		}
	}

	// ---- Board / model hints (CCR/CRS/CSS/SXT/wAP/etc.) ---------------
	combined := model + " " + board
	if strings.TrimSpace(combined) != "" {
		switch {
		case containsAny(combined, "ccr"):
			add(CategoryRouter, 30, "model_ccr", "Cloud Core Router model")
		case containsAny(combined, "css", "crs"):
			add(CategorySwitch, 25, "model_switch", "Cloud Switch model")
		case containsAny(combined, "sxt", "ldf", "lhg", "dynadish", "groove", "metal", "sxtsq"):
			add(CategoryCPE, 25, "model_outdoor_cpe", "Outdoor CPE model")
		case containsAny(combined, "wap", "cap", "hap", "audience"):
			add(CategoryAP, 25, "model_indoor_ap", "Indoor/Outdoor AP model")
		case containsAny(combined, "ptp", "rb921", "rb411", "rb711", "qrt"):
			add(CategoryBackhaul, 25, "model_ptp", "Model historically used for PtP")
		}
	}

	// ---- Interface-name hint (e.g. wlan1-ap, ether1-uplink) ------------
	if iface != "" {
		switch {
		case strings.Contains(iface, "ap"):
			add(CategoryAP, 10, "iface_hint_ap", "Interface name contains 'ap'")
		case strings.Contains(iface, "uplink") || strings.Contains(iface, "wan"):
			add(CategoryRouter, 10, "iface_hint_uplink", "Interface name suggests uplink")
		case strings.Contains(iface, "ptp") || strings.Contains(iface, "bh"):
			add(CategoryBackhaul, 10, "iface_hint_link", "Interface name suggests backhaul")
		}
	}

	// ---- Choose best + apply conflict + signal-strength penalties ------
	best := CategoryUnknown
	bestScore := 0
	secondScore := 0
	for c, s := range scores {
		if s > bestScore {
			secondScore = bestScore
			bestScore = s
			best = c
		} else if s > secondScore {
			secondScore = s
		}
	}

	// Strong-signal multiplier: full RouterOS-platform-identified host
	// gets a bonus that nudges confidence into the high band.
	signalBonus := 0
	if d.MAC != "" {
		signalBonus += 10
	}
	if d.IP != "" {
		signalBonus += 5
	}
	if d.Platform != "" {
		signalBonus += 8
	}
	if d.Identity != "" {
		signalBonus += 5
	}
	if d.Board != "" {
		signalBonus += 5
	}
	if d.InterfaceName != "" {
		signalBonus += 3
	}
	if best != CategoryUnknown {
		bestScore += signalBonus
	}

	// Conflict penalty when the second-place score is close to the
	// best — operators should see uncertainty rather than a falsely
	// confident assignment.
	if secondScore > 0 && bestScore-secondScore < 15 {
		bestScore -= 15
		ev = append(ev, Evidence{
			Heuristic: "conflict_penalty",
			Weight:    -15,
			Reason:    "Second-place evidence within 15 pts — confidence reduced",
		})
	}

	if bestScore > 100 {
		bestScore = 100
	}
	if bestScore < 0 {
		bestScore = 0
	}

	// Insufficient evidence: don't pretend.
	if bestScore < 20 {
		best = CategoryUnknown
	}
	if best == CategoryUnknown {
		// A name-only "name suggests router" with no other evidence
		// should still be Unknown; reset the score so the operator
		// can clearly see "no idea".
		if bestScore < 25 {
			bestScore = 0
		}
	}

	sort.SliceStable(ev, func(i, j int) bool { return ev[i].Weight > ev[j].Weight })
	d.Classification = Classification{
		Category:   best,
		Confidence: bestScore,
		Evidences:  ev,
	}
}

func containsAny(s string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}

func startsWithAny(s string, prefixes ...string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func lowerKeyMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[strings.ToLower(k)] = v
	}
	return out
}
