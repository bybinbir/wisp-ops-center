package devicectl

// Faz R4 — read-only allowlist (devre-dışı bırakma yok, **sadece izin
// verilen** komutlar geçer). WISP-R4-DUDE-TO-POP-OPS-FINISH şu
// invariant'ı zorunlu kılar:
//
//   • Set / add / remove / enable / disable / reset / reboot /
//     upgrade / import / export show-sensitive / password / user
//     write|update / certificate private-key export ve genel olarak
//     herhangi bir mutation komutu cihaza ASLA gönderilmez.
//   • SQL CHECK constraint (frequency_plan_runs.mutation = false)
//     ile birlikte ikinci kilit.
//
// İki katmanlı savunma:
//
//   1. **Whitelist** — sadece açıkça izin verilen RouterOS / SSH /
//      SNMP / Mimosa rotaları geçer.
//   2. **Mutation guard** — whitelist içinde bile bir mutation
//      token'ı (set, add, remove, ...) görüldüyse komut reddedilir.
//
// Uyumluluk: `internal/adapters/mikrotik/allowlist.go` Phase 8'den
// beri var ve daha dar bir set tutuyor. Bu paket onun **üst kümesi**;
// mikrotik adapter'ı kendi paketini değişmeden tutar, R4 yeni probe
// motoru bu paketi kullanır.

import (
	"errors"
	"fmt"
	"strings"
)

// ErrCommandNotAllowed allowlist dışında bir komut denenince döner.
var ErrCommandNotAllowed = errors.New("komut R4 read-only allowlist dışında")

// ErrMutationCommandBlocked komut bir mutation token'ı içerirse döner.
var ErrMutationCommandBlocked = errors.New("mutation komutu reddedildi (R4 read-only)")

// MikrotikReadOnlyCommands operatörün prompt'ta kabul ettiği RouterOS
// read-only komutları. Hem RouterOS API path'i (örn.
// "/system/identity/print"), hem de SSH/CLI satırı (örn.
// "/system identity print") olarak kabul edilir; normalize edilir.
var MikrotikReadOnlyCommands = []string{
	"/system/identity/print",
	"/system/resource/print",
	"/system/routerboard/print",
	"/system/package/print",
	"/interface/print",
	"/interface/print/detail",
	"/interface/wireless/print",
	"/interface/wireless/print/detail",
	"/interface/wireless/registration-table/print",
	"/interface/wireless/registration-table/print/detail",
	"/interface/wifi/print",
	"/interface/wifi/print/detail",
	"/interface/wifi/registration-table/print",
	"/interface/wifi/registration-table/print/detail",
	"/interface/wifiwave2/print",
	"/interface/wifiwave2/print/detail",
	"/interface/wifiwave2/registration-table/print",
	"/interface/wifiwave2/registration-table/print/detail",
	"/interface/bridge/print",
	"/interface/bridge/print/detail",
	"/interface/bridge/port/print",
	"/interface/bridge/port/print/detail",
	"/ip/address/print",
	"/ip/address/print/detail",
	"/ip/route/print",
	"/ip/route/print/detail",
	"/ip/neighbor/print",
	"/ip/neighbor/print/detail",
	"/log/print",
}

// MutationTokens bir komut path segment'i olarak görüldüğünde komutu
// veto eden mutation fiilleri. Whitelist'e ekleme yaparken bile
// "set/add/remove" gibi bir segment varsa komut reddedilir.
var MutationTokens = []string{
	"set", "add", "remove", "edit", "move",
	"enable", "disable",
	"reset", "reset-configuration", "reboot", "shutdown",
	"upgrade", "downgrade",
	"import",
	"password", "passwd",
	// 'export' tek başına bilgi verme amaçlı kabul edilebilirdi ama
	// 'export show-sensitive' tehlikeli; ikisini de reddediyoruz —
	// R4 prompt'u export'u açıkça yasakladı.
	"export",
	"file",
	"tool",
	"script", "scheduler",
	"certificate",
	"snmp-set", "snmpset",
}

// LogPrintAllowedFilters /log/print için izin verilen `where`
// filtrelerinin allowlist'i. Sınırsız log dump etmek istemiyoruz;
// operatör prompt'unda sadece wireless|error|critical topic'leri
// kabul edildi.
var LogPrintAllowedFilters = []string{
	`topics~"wireless|error|critical"`,
	`topics~"wireless"`,
	`topics~"error"`,
	`topics~"critical"`,
}

// MimosaReadOnlyEndpoints Mimosa HTTP API'nin read-only rotaları.
// Mimosa B5/B5c/C5 modelleri /cgi-bin/* ve /api/v1/* iki ayrı
// jenerasyon kullanıyor; ikisini de allowlist'e alıyoruz.
var MimosaReadOnlyEndpoints = []string{
	"/api/v1/status",
	"/api/v1/status/wireless",
	"/api/v1/status/ethernet",
	"/api/v1/status/system",
	"/api/v1/status/link",
	"/api/v1/info",
	"/cgi-bin/status",
	"/cgi-bin/status.cgi",
	"/cgi-bin/system_info.cgi",
}

// SNMPReadOnlyOIDPrefixes SNMP probe katmanı için izin verilen OID
// önek kümeleri. Sadece read-only metrik OID'leri; community/auth
// secret'ları zaten env'den geliyor, bu allowlist OID düzeyinde
// "neyi sorgulayabiliriz"i sınırlar.
var SNMPReadOnlyOIDPrefixes = []string{
	"1.3.6.1.2.1.1",     // system MIB
	"1.3.6.1.2.1.2",     // interfaces MIB
	"1.3.6.1.2.1.4",     // ip MIB
	"1.3.6.1.2.1.25",    // host MIB
	"1.3.6.1.4.1.14988", // MikroTik MIB
	"1.3.6.1.4.1.43356", // Mimosa MIB
}

// NormalizeMikrotikCommand SSH ve API formatlarını ortak path'e
// çevirir:
//
//	"/system identity print"        → "/system/identity/print"
//	"/interface wireless print detail" → "/interface/wireless/print/detail"
//	"/log print where topics~..."   → "/log/print"  (where filter
//	                                    ayrıca kontrol edilir)
//
// `where ...` kısmı path'ten ayrılır, döndürülen ikinci string olur.
func NormalizeMikrotikCommand(cmd string) (path, filter string) {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return "", ""
	}
	// `where` ayrımı (case-insensitive)
	low := strings.ToLower(cmd)
	if idx := strings.Index(low, " where "); idx > 0 {
		filter = strings.TrimSpace(cmd[idx+len(" where "):])
		cmd = strings.TrimSpace(cmd[:idx])
	}
	// Boşluğu '/' ile değiştir; "/system identity print" → "/system/identity/print"
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return "", filter
	}
	first := parts[0]
	rest := parts[1:]
	if !strings.HasPrefix(first, "/") {
		first = "/" + first
	}
	full := first
	for _, p := range rest {
		full += "/" + p
	}
	// "//" duplikasyonunu temizle
	for strings.Contains(full, "//") {
		full = strings.ReplaceAll(full, "//", "/")
	}
	return strings.ToLower(full), filter
}

// IsMikrotikCommandAllowed verilen RouterOS komutu için iki katmanlı
// kontrolü uygular. Whitelist + mutation veto.
func IsMikrotikCommandAllowed(cmd string) bool {
	path, filter := NormalizeMikrotikCommand(cmd)
	if path == "" {
		return false
	}
	for _, seg := range strings.Split(path, "/") {
		if seg == "" {
			continue
		}
		for _, mut := range MutationTokens {
			if seg == mut {
				return false
			}
		}
	}
	if !inSet(MikrotikReadOnlyCommands, path) {
		return false
	}
	if path == "/log/print" && filter != "" {
		return inSet(LogPrintAllowedFilters, normalizeFilter(filter))
	}
	return true
}

// EnsureMikrotikCommand whitelist + mutation guard sonucu hata olarak
// üretir. Probe katmanı bunu kullanır.
func EnsureMikrotikCommand(cmd string) error {
	if IsMikrotikCommandAllowed(cmd) {
		return nil
	}
	if hasMutationToken(cmd) {
		return fmt.Errorf("%w: %q", ErrMutationCommandBlocked, cmd)
	}
	return fmt.Errorf("%w: %q", ErrCommandNotAllowed, cmd)
}

// IsMimosaEndpointAllowed Mimosa HTTP probe path'lerini kontrol eder.
func IsMimosaEndpointAllowed(path string) bool {
	p := strings.ToLower(strings.TrimSpace(path))
	if p == "" {
		return false
	}
	if hasMutationToken(p) {
		return false
	}
	for _, ok := range MimosaReadOnlyEndpoints {
		if p == ok {
			return true
		}
	}
	return false
}

// EnsureMimosaEndpoint Mimosa endpoint için hata üretir.
func EnsureMimosaEndpoint(path string) error {
	if IsMimosaEndpointAllowed(path) {
		return nil
	}
	if hasMutationToken(path) {
		return fmt.Errorf("%w: %q", ErrMutationCommandBlocked, path)
	}
	return fmt.Errorf("%w: %q", ErrCommandNotAllowed, path)
}

// IsSNMPOIDAllowed verilen OID'in read-only allowlist'te olup
// olmadığını söyler. OID prefix-match'tir.
func IsSNMPOIDAllowed(oid string) bool {
	o := strings.TrimSpace(oid)
	if o == "" {
		return false
	}
	for _, p := range SNMPReadOnlyOIDPrefixes {
		if o == p || strings.HasPrefix(o, p+".") {
			return true
		}
	}
	return false
}

// EnsureSNMPOID SNMP OID için hata üretir.
func EnsureSNMPOID(oid string) error {
	if IsSNMPOIDAllowed(oid) {
		return nil
	}
	return fmt.Errorf("%w: %q", ErrCommandNotAllowed, oid)
}

// hasMutationToken serbest metin içinde herhangi bir mutation
// token'ının görünüp görünmediğini söyler. Belirsiz/yanlış-pozitif
// olabilir (örn. "address" içinde "add" geçer); bu yüzden segment
// bazlı whitelist'i FIRST OF DEFENSE olarak kullanıyoruz, bu
// fonksiyon sadece "olabilir mi?" hata mesajını süslemek için.
func hasMutationToken(s string) bool {
	low := strings.ToLower(s)
	for _, t := range MutationTokens {
		// segment veya kelime sınırı: " set ", "/set/", " set\n"
		patterns := []string{" " + t + " ", "/" + t + "/", "/" + t, " " + t}
		for _, p := range patterns {
			if strings.Contains(low, p) {
				return true
			}
		}
	}
	return false
}

func inSet(set []string, v string) bool {
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}

func normalizeFilter(s string) string {
	// Boşlukları sıkıştır, küçük harf yap.
	s = strings.ToLower(strings.Join(strings.Fields(s), " "))
	return s
}
