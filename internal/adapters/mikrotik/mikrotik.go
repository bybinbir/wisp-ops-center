// Package mikrotik provides read-only RouterOS access for wisp-ops-center.
//
// Phase 3 contract: every code path is read-only. Disallowed commands are
// rejected at the adapter boundary regardless of how they were
// constructed; see allowlist.go.
package mikrotik

// Transport names the wire we use to talk to RouterOS.
type Transport string

const (
	TransportAPISSL Transport = "api-ssl" // 8729/TCP, preferred
	TransportSSH    Transport = "ssh"     // fallback
	TransportSNMP   Transport = "snmp"    // read-only
)

// Config carries per-device settings. The secret itself never lives here;
// callers fetch it via the credential vault and pass it as a separate
// parameter to NewAPIClient/NewSSHClient/NewSNMPClient.
type Config struct {
	DeviceID      string
	Host          string
	Port          int
	Transport     Transport
	Username      string
	UseTLS        bool
	VerifyTLS     bool
	SNMPCommunity string
	SNMPVersion   string // "v2c" or "v3"
	TimeoutSec    int    // per-call timeout; default 8s

	// Faz 6 — SSH host key policy runtime alanları (yalnız SSH transport
	// kullanılırsa yorumlanır). SSHKnownHostsStore Service tarafından
	// SSHClient'a iletilir; bu yapıda yer almaz.
	//   "insecure_ignore" | "trust_on_first_use" | "pinned"
	SSHHostKeyPolicy      string
	SSHHostKeyFingerprint string

	// Faz 7 — RouterOS API-SSL TLS hardening.
	//   CACertificatePEM   : PEM kodlu özel CA (opsiyonel). VerifyTLS=true
	//                        ise RootCAs olarak kullanılır.
	//   ServerNameOverride : SNI/peer doğrulamada kullanılacak hostname.
	//                        Cihazın IP adresi sertifika SAN'da yer almıyorsa
	//                        operatör buraya sertifika CN/SAN değerini koyar.
	CACertificatePEM   string
	ServerNameOverride string
}

// Vendor returns the constant adapter identifier.
func Vendor() string { return "mikrotik" }
