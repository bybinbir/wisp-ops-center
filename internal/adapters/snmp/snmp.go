// Package snmp, salt-okuma SNMP istemci taslağıdır. Faz 1'de gerçek
// SNMP istemcisi (örn. gosnmp) eklenmez; sadece OID koleksiyonları
// ve istek tip tanımları sabitlenir.
package snmp

// Version, SNMP protokol versiyonu.
type Version string

const (
	V2c Version = "v2c"
	V3  Version = "v3"
)

// Request, salt-okuma SNMP isteğini tanımlar.
type Request struct {
	Host       string
	Port       int
	Version    Version
	Community  string // v2c için
	Username   string // v3 için
	OIDs       []string
	TimeoutSec int
}

// CommonOIDs, vendor'dan bağımsız sık kullanılan OID'leri listeler.
// Vendor-spesifik OID'ler ileri fazlarda mikrotik / mimosa
// paketlerine taşınacaktır.
var CommonOIDs = struct {
	SysDescr   string
	SysUpTime  string
	SysName    string
	IfTable    string
	IfHCInOct  string
	IfHCOutOct string
}{
	SysDescr:   "1.3.6.1.2.1.1.1.0",
	SysUpTime:  "1.3.6.1.2.1.1.3.0",
	SysName:    "1.3.6.1.2.1.1.5.0",
	IfTable:    "1.3.6.1.2.1.2.2",
	IfHCInOct:  "1.3.6.1.2.1.31.1.1.1.6",
	IfHCOutOct: "1.3.6.1.2.1.31.1.1.1.10",
}
