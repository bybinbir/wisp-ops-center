package mimosa

// OID kataloğu — Faz 4'te yalnızca standart MIB OID'leri kullanılır.
// Mimosa vendor MIB'i fonksiyonel olarak entegre edilene kadar bu
// dosyaya YENİ vendor OID eklemek YASAKTIR; mevcut placeholder'lar
// "unverified" olarak işaretlidir ve runtime'da çağrılmaz.

// Standard SNMP OIDs (RFC 1213 / RFC 2233). Bunlar her SNMP-uyumlu
// cihazda mevcuttur ve Mimosa cihazlarında da çalışır.
const (
	OIDSysDescr    = "1.3.6.1.2.1.1.1.0"
	OIDSysObjectID = "1.3.6.1.2.1.1.2.0"
	OIDSysUpTime   = "1.3.6.1.2.1.1.3.0"
	OIDSysName     = "1.3.6.1.2.1.1.5.0"
	OIDSysLocation = "1.3.6.1.2.1.1.6.0"

	// IF-MIB ifTable
	OIDIfIndex       = "1.3.6.1.2.1.2.2.1.1"
	OIDIfDescr       = "1.3.6.1.2.1.2.2.1.2"
	OIDIfType        = "1.3.6.1.2.1.2.2.1.3"
	OIDIfMtu         = "1.3.6.1.2.1.2.2.1.4"
	OIDIfSpeed       = "1.3.6.1.2.1.2.2.1.5"
	OIDIfAdminStatus = "1.3.6.1.2.1.2.2.1.7"
	OIDIfOperStatus  = "1.3.6.1.2.1.2.2.1.8"
	OIDIfInOctets    = "1.3.6.1.2.1.2.2.1.10"
	OIDIfInErrors    = "1.3.6.1.2.1.2.2.1.14"
	OIDIfOutOctets   = "1.3.6.1.2.1.2.2.1.16"
	OIDIfOutErrors   = "1.3.6.1.2.1.2.2.1.20"
	OIDIfInDiscards  = "1.3.6.1.2.1.2.2.1.13"
	OIDIfOutDiscards = "1.3.6.1.2.1.2.2.1.19"

	// IF-MIB ifXTable (RFC 2233; HC counters)
	OIDIfName      = "1.3.6.1.2.1.31.1.1.1.1"
	OIDIfHighSpeed = "1.3.6.1.2.1.31.1.1.1.15"
)

// VendorMIBPlaceholder, Mimosa-spesifik OID'lerin entegrasyon
// durumunu temsil eder. Faz 4 boyunca daima "unverified" döner; runtime
// kodu bu durumu görünce vendor OID çağrısı YAPMAZ ve telemetri
// snapshot'ını "partial" işaretler.
//
// Vendor MIB elde edildiğinde:
//  1. Bu dosyaya isimlendirilmiş yeni sabitler eklenecek.
//  2. probe.go ve telemetry.go vendor OID çağrısını koşullu olarak
//     etkinleştirecek.
//  3. capability matrisi `canReadFrequency`, `canReadClients`,
//     `canReadWirelessMetrics` bayraklarını ayrı ayrı kanıtlayacak.
const VendorMIBPlaceholder = "unverified"
