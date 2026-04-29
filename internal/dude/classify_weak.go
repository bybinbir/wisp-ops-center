package dude

// Phase R2 — weak_name_pattern heuristic tier.
//
// Sebep: Lab Dude'unda 893 cihazın 892'si Phase 8 sonrası, 910+'ı
// Phase 8.1 enrichment sonrası `Unknown` kaldı. Sebep:
// MAC / platform / board / interface_name alanları çoğu cihaz için
// boş gelir, dolayısıyla mevcut `Classify` skor eşiğinin altında
// kalır ve Unknown'a düşer. R2 bu boşluğu, **isim pattern'inden**
// türetilen, **confidence cap'i 50 olan**, **strong evidence
// tarafından override edilen** bir ikincil katman ile dolduruyor.
//
// Tasarım kuralları:
//
//   - weak_name_pattern *yalnızca* primary `Classify` Unknown
//     döndürdüğünde devreye girer. Strong evidence (Dude type, MAC +
//     platform, wireless mode AP-bridge, vs.) varsa weak hiç
//     çalışmaz.
//   - Bir match olduğunda confidence en fazla 50'dir. Frontend
//     bunu "düşük güven / zayıf isim sınıflandırması" rozetiyle
//     gösterir.
//   - Birden fazla farklı kategoride pattern eşleşirse (ambiguous)
//     cihaz Unknown olarak kalır — sahte kesinlik yasak.
//   - Her weak match `device_category_evidence` satırı doğurur;
//     `Heuristic = "weak_name_pattern"`, Reason alanında eşleşen
//     token + Türkçe açıklama.
//
// Bu katman destructive kod yoluna dokunmaz, schema değiştirmez,
// migration üretmez. Yalnız sınıflandırma davranışını değiştirir.

import "strings"

// weakPatternTier categories'i isim token'larından türetir. Pattern
// listeleri operatörün gözlemlediği lab isim deseninden hareketle
// **muhafazakâr** seçildi: tek-token match yetmez, ambiguous match
// Unknown'a düşer. Her token sözcük başında veya tire ile ayrılmış
// halde ararız (pure substring değil) — "rocket" rastgele bir
// isimde "rockets" olarak da geçebilir, ama yanlış pozitif riskini
// azaltmak için kelime sınırına bağlı eşleme tercih ediyoruz.
//
// Her grup bir `Category` üretir; isim aynı anda birden fazla
// gruba uyarsa weak match iptal edilir.
var weakPatternGroups = []weakPatternGroup{
	{
		Category: CategoryAP,
		Tokens: []string{
			"ap", "sektor", "sektör", "sector", "tower", "kule",
			"baz", "base", "omni", "wifi", "wlan",
		},
		// Reason text shown in evidence row + EvidenceModal.
		Reason: "İsim pattern'i AP/sektör/kule kelimeleri içeriyor",
	},
	{
		Category: CategoryBackhaul,
		Tokens: []string{
			"ptp", "ptmp", "bh", "link", "relay", "backhaul",
			"airfiber", "mimosa", "rocket", "powerbeam", "lhg",
			"sxt", "dish", "nano", "gemi",
		},
		Reason: "İsim pattern'i PtP/backhaul/relay kelimeleri içeriyor",
	},
	{
		Category: CategoryBridge,
		Tokens: []string{
			"bridge", "br", "core-bridge", "switch-bridge",
		},
		Reason: "İsim pattern'i bridge kelimesi içeriyor",
	},
	{
		Category: CategoryCPE,
		Tokens: []string{
			"cpe", "client", "musteri", "müşteri", "abone", "ev",
			"home", "user", "station", "sta", "konut",
		},
		Reason: "İsim pattern'i müşteri/abone/CPE kelimeleri içeriyor",
	},
	{
		Category: CategoryRouter,
		Tokens: []string{
			"router", "gw", "gateway", "core", "pop", "rb",
			"ccr", "chr", "edge", "agg", "aggregation",
		},
		Reason: "İsim pattern'i router/gateway/core kelimeleri içeriyor",
	},
}

type weakPatternGroup struct {
	Category Category
	Tokens   []string
	Reason   string
}

// classifyWeakNamePattern tries each group against the device name
// and returns the chosen category (or CategoryUnknown when no group
// or more than one group matched). The matched token is also
// returned so the evidence row can show "matched: 'sektor'".
func classifyWeakNamePattern(name string) (Category, string, string) {
	if strings.TrimSpace(name) == "" {
		return CategoryUnknown, "", ""
	}
	low := strings.ToLower(name)
	tokens := tokenizeName(low)
	var (
		matchedCat    Category
		matchedToken  string
		matchedReason string
		matchCount    int
	)
	for _, g := range weakPatternGroups {
		for _, t := range g.Tokens {
			if hasToken(tokens, t) {
				if matchCount == 0 {
					matchedCat = g.Category
					matchedToken = t
					matchedReason = g.Reason
				} else if g.Category != matchedCat {
					// ambiguous: belongs to two distinct
					// categories → bail.
					return CategoryUnknown, "", "ambiguous_multi_category"
				}
				matchCount++
				break
			}
		}
	}
	if matchCount == 0 {
		return CategoryUnknown, "", ""
	}
	return matchedCat, matchedToken, matchedReason
}

// tokenizeName splits a device name into tokens by typical
// separators used by RouterOS labels. Numeric-only segments are
// kept (so "5g", "2.4", "300" can be matched if they appear in a
// pattern token list later — currently they don't, but the routine
// stays general).
func tokenizeName(s string) []string {
	out := []string{}
	cur := strings.Builder{}
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for _, r := range s {
		switch {
		case r == '-' || r == '_' || r == ' ' || r == '.' || r == '/' || r == ':' || r == ',':
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return out
}

// hasToken returns true if `tokens` contains `t` exactly OR if any
// token starts with `t` followed by a digit (so "ap1" matches token
// "ap"). We intentionally do NOT allow substring match anywhere
// inside a token — "tap" must not match "ap".
func hasToken(tokens []string, t string) bool {
	for _, tok := range tokens {
		if tok == t {
			return true
		}
		if strings.HasPrefix(tok, t) {
			rest := tok[len(t):]
			if rest != "" && isAllDigits(rest) {
				return true
			}
		}
	}
	return false
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// applyWeakNamePattern is the public entry-point Classify calls
// after running the strong-tier scoring. It mutates `d` in place by
// upgrading classification *only* when the device would otherwise be
// Unknown. The new evidence row is appended; the heuristic name is
// "weak_name_pattern" so downstream queries / UIs can filter on it.
func applyWeakNamePattern(d *DiscoveredDevice) {
	// Honour strong evidence: if Classify already committed to a
	// non-Unknown category, never override.
	if d.Classification.Category != CategoryUnknown {
		return
	}
	cat, token, reason := classifyWeakNamePattern(d.Name)
	if cat == CategoryUnknown {
		return
	}
	// Confidence cap = 50. We pick 45 specifically so the
	// "düşük güven" (<50) UI bucket lights up immediately and the
	// operator sees the weak label without doing math.
	const weakConfidence = 45
	d.Classification.Category = cat
	d.Classification.Confidence = weakConfidence
	d.Classification.Evidences = append(d.Classification.Evidences, Evidence{
		Heuristic: "weak_name_pattern",
		Weight:    weakConfidence,
		Reason: "Zayıf sınıflandırma: '" + token + "' token'ı eşleşti — " + reason +
			". Confidence 50 ile sınırlı; MAC / wireless-mode / platform doğrulaması yok.",
	})
}
