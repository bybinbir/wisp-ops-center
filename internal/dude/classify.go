package dude

import "strings"

// Classify assigns a category + confidence to a discovered device
// based on a layered set of heuristics.
func Classify(d *DiscoveredDevice) {
	scores := map[Category]int{}
	var ev []Evidence

	add := func(cat Category, w int, h, reason string) {
		scores[cat] += w
		ev = append(ev, Evidence{Heuristic: h, Weight: w, Reason: reason})
	}

	name := strings.ToLower(d.Name)
	model := strings.ToLower(d.Model)
	dtype := strings.ToLower(d.Type)
	raw := lowerKeyMap(d.Raw)

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

	switch {
	case startsWithAny(name, "ap-", "ap_", "ap "):
		add(CategoryAP, 40, "name_prefix_ap", "Name starts with AP-")
	case startsWithAny(name, "cpe-", "cpe_", "cpe ", "musteri-", "müşteri-"):
		add(CategoryCPE, 40, "name_prefix_cpe", "Name starts with CPE/Müşteri")
	case startsWithAny(name, "ptp-", "ptp_", "link-", "link_", "bh-", "bh_", "backhaul"):
		add(CategoryBackhaul, 45, "name_prefix_link", "Name indicates backhaul/link")
	case startsWithAny(name, "br-", "br_", "bridge-", "bridge_"):
		add(CategoryBridge, 35, "name_prefix_bridge", "Name indicates bridge")
	case startsWithAny(name, "rtr-", "router-", "core-", "edge-", "gw-"):
		add(CategoryRouter, 35, "name_prefix_router", "Name indicates router/core/edge")
	case startsWithAny(name, "sw-", "switch-"):
		add(CategorySwitch, 35, "name_prefix_switch", "Name indicates switch")
	}

	if v, ok := raw["wireless-mode"]; ok {
		v = strings.ToLower(v)
		switch {
		case strings.Contains(v, "ap-bridge") || strings.Contains(v, "wds-slave-ap"):
			add(CategoryAP, 35, "wireless_mode_ap", "Wireless mode reports AP role")
		case strings.Contains(v, "station") || strings.Contains(v, "client"):
			add(CategoryCPE, 30, "wireless_mode_station", "Wireless mode reports station/CPE")
		case strings.Contains(v, "bridge"):
			add(CategoryBackhaul, 25, "wireless_mode_bridge", "Wireless mode bridge — likely PtP backhaul")
		}
	}

	if v, ok := raw["interface-type"]; ok {
		if strings.Contains(strings.ToLower(v), "bridge") {
			add(CategoryBridge, 20, "interface_type_bridge", "Interface-type contains bridge")
		}
	}

	if model != "" {
		switch {
		case strings.Contains(model, "ccr"):
			add(CategoryRouter, 30, "model_hint_router", "Cloud Core Router model")
		case strings.Contains(model, "css") || strings.Contains(model, "crs"):
			add(CategorySwitch, 25, "model_hint_switch", "Cloud Switch model")
		case strings.Contains(model, "sxt") || strings.Contains(model, "ldf") ||
			strings.Contains(model, "lhg") || strings.Contains(model, "dynadish") ||
			strings.Contains(model, "groove"):
			add(CategoryCPE, 25, "model_hint_cpe", "Outdoor CPE model")
		case strings.Contains(model, "wap") || strings.Contains(model, "cap") ||
			strings.Contains(model, "hap"):
			add(CategoryAP, 20, "model_hint_ap", "Indoor/Outdoor AP model")
		case strings.Contains(model, "ptp") || strings.Contains(model, "rb921") ||
			strings.Contains(model, "rb411"):
			add(CategoryBackhaul, 20, "model_hint_link", "Model historically used for PtP")
		}
	}

	if len(scores) == 0 && d.IP != "" {
		add(CategoryUnknown, 5, "ip_only_clue", "Only IP available — falling back to Unknown")
	}

	var best Category = CategoryUnknown
	bestScore := 0
	for c, s := range scores {
		if s > bestScore {
			best = c
			bestScore = s
		}
	}
	if bestScore > 100 {
		bestScore = 100
	}
	d.Classification = Classification{
		Category:   best,
		Confidence: bestScore,
		Evidences:  ev,
	}
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
