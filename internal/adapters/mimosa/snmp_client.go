package mimosa

import (
	"errors"
	"strings"
	"time"

	g "github.com/gosnmp/gosnmp"
)

// SNMPClient bir Mimosa cihazına yapılan tek-atımlık salt-okuma SNMP
// sorgularını sarar. SNMPv2c ve SNMPv3 (USM) destekler; v3 desteği
// gosnmp kütüphanesinin sağladığı parametrelere bağlıdır (MD5/SHA/
// SHA256 + DES/AES/AES192/AES256). Kütüphane SHA256/AES256'yı bütün
// derlemelerde sunmuyorsa, runtime ErrSNMPv3Misconfigured ile durur
// ve UI'a "library_limited" sinyali bırakır.
type SNMPClient struct {
	cfg Config
	gs  *g.GoSNMP
}

// NewSNMPClient builds the wrapper without dialing.
func NewSNMPClient(cfg Config) *SNMPClient { return &SNMPClient{cfg: cfg} }

// Dial establishes the UDP transport.
func (c *SNMPClient) Dial() error {
	if c.cfg.Host == "" {
		return ErrUnreachable
	}
	port := uint16(c.cfg.Port)
	if port == 0 {
		port = 161
	}
	timeout := time.Duration(c.cfg.TimeoutSec) * time.Second
	if timeout <= 0 || timeout > 30*time.Second {
		timeout = 5 * time.Second
	}
	gs := &g.GoSNMP{
		Target:  c.cfg.Host,
		Port:    port,
		Timeout: timeout,
		Retries: 1,
		MaxOids: 60,
	}
	switch c.cfg.SNMPVersion {
	case SNMPv2c, "":
		gs.Version = g.Version2c
		gs.Community = c.cfg.Community
	case SNMPv3:
		gs.Version = g.Version3
		gs.SecurityModel = g.UserSecurityModel
		usm, err := buildUSM(c.cfg)
		if err != nil {
			return err
		}
		gs.MsgFlags = msgFlagsFor(c.cfg.V3SecurityLevel)
		gs.SecurityParameters = usm
	default:
		return ErrTransportUnsupported
	}
	c.gs = gs
	if err := c.gs.Connect(); err != nil {
		return ClassifyError(err)
	}
	return nil
}

// Close releases the SNMP transport.
func (c *SNMPClient) Close() {
	if c.gs != nil && c.gs.Conn != nil {
		_ = c.gs.Conn.Close()
		c.gs = nil
	}
}

func msgFlagsFor(level SNMPv3SecurityLevel) g.SnmpV3MsgFlags {
	switch level {
	case AuthNoPriv:
		return g.AuthNoPriv
	case AuthPriv:
		return g.AuthPriv
	case NoAuthNoPriv, "":
		return g.NoAuthNoPriv
	}
	return g.NoAuthNoPriv
}

func buildUSM(cfg Config) (*g.UsmSecurityParameters, error) {
	if cfg.V3Username == "" {
		return nil, ErrSNMPv3Misconfigured
	}
	usm := &g.UsmSecurityParameters{
		UserName: cfg.V3Username,
	}
	switch cfg.V3SecurityLevel {
	case AuthNoPriv, AuthPriv:
		ap, err := authProto(cfg.V3AuthProtocol)
		if err != nil {
			return nil, err
		}
		usm.AuthenticationProtocol = ap
		usm.AuthenticationPassphrase = cfg.V3AuthSecret
	}
	if cfg.V3SecurityLevel == AuthPriv {
		pp, err := privProto(cfg.V3PrivProtocol)
		if err != nil {
			return nil, err
		}
		usm.PrivacyProtocol = pp
		usm.PrivacyPassphrase = cfg.V3PrivSecret
	}
	return usm, nil
}

func authProto(p SNMPv3AuthProtocol) (g.SnmpV3AuthProtocol, error) {
	switch p {
	case AuthMD5:
		return g.MD5, nil
	case AuthSHA:
		return g.SHA, nil
	case AuthSHA256:
		return g.SHA256, nil
	case "":
		return g.NoAuth, ErrSNMPv3Misconfigured
	}
	return g.NoAuth, ErrSNMPv3Misconfigured
}

func privProto(p SNMPv3PrivProtocol) (g.SnmpV3PrivProtocol, error) {
	switch p {
	case PrivDES:
		return g.DES, nil
	case PrivAES:
		return g.AES, nil
	case PrivAES192:
		return g.AES192, nil
	case PrivAES256:
		return g.AES256, nil
	case "":
		return g.NoPriv, ErrSNMPv3Misconfigured
	}
	return g.NoPriv, ErrSNMPv3Misconfigured
}

// SystemInfo reads sysDescr/sysName/sysUpTime in one Get.
func (c *SNMPClient) SystemInfo() (descr, sysName string, uptimeSec int64, err error) {
	if c.gs == nil {
		return "", "", 0, errors.New("snmp: not connected")
	}
	res, gerr := c.gs.Get([]string{OIDSysDescr, OIDSysName, OIDSysUpTime})
	if gerr != nil {
		return "", "", 0, ClassifyError(gerr)
	}
	for _, v := range res.Variables {
		switch v.Name {
		case "." + OIDSysDescr:
			if b, ok := v.Value.([]byte); ok {
				descr = string(b)
			}
		case "." + OIDSysName:
			if b, ok := v.Value.([]byte); ok {
				sysName = string(b)
			}
		case "." + OIDSysUpTime:
			if t, ok := v.Value.(uint32); ok {
				uptimeSec = timeTicksToSeconds(t)
			}
		}
	}
	return descr, sysName, uptimeSec, nil
}

// InterfaceTable walks ifTable + ifXTable and returns normalized rows.
// Mimosa cihazlarında bu tablo cihazın Ethernet/wireless arayüzlerini
// (genelde 2-4 girdi) listeler. Yüksek hacim beklenmez.
func (c *SNMPClient) InterfaceTable() ([]MimosaInterfaceMetric, error) {
	if c.gs == nil {
		return nil, errors.New("snmp: not connected")
	}
	rows := map[int]*MimosaInterfaceMetric{}
	get := func(idx int) *MimosaInterfaceMetric {
		r, ok := rows[idx]
		if !ok {
			r = &MimosaInterfaceMetric{Index: idx}
			rows[idx] = r
		}
		return r
	}
	walks := []struct {
		oid    string
		assign func(idx int, v interface{})
	}{
		{OIDIfDescr, func(idx int, v interface{}) {
			if b, ok := v.([]byte); ok {
				get(idx).Descr = string(b)
				if get(idx).Name == "" {
					get(idx).Name = string(b)
				}
			}
		}},
		{OIDIfName, func(idx int, v interface{}) {
			if b, ok := v.([]byte); ok && len(b) > 0 {
				get(idx).Name = string(b)
			}
		}},
		{OIDIfType, func(idx int, v interface{}) {
			if n, ok := v.(int); ok {
				get(idx).Type = n
			}
		}},
		{OIDIfMtu, func(idx int, v interface{}) {
			if n, ok := v.(int); ok {
				get(idx).MTU = n
			}
		}},
		{OIDIfSpeed, func(idx int, v interface{}) {
			if n, ok := v.(uint); ok {
				get(idx).SpeedBps = int64(n)
			}
		}},
		{OIDIfAdminStatus, func(idx int, v interface{}) {
			if n, ok := v.(int); ok {
				get(idx).AdminUp = ifStatusUp(n)
			}
		}},
		{OIDIfOperStatus, func(idx int, v interface{}) {
			if n, ok := v.(int); ok {
				get(idx).OperUp = ifStatusUp(n)
			}
		}},
		{OIDIfInOctets, func(idx int, v interface{}) {
			if n, ok := v.(uint); ok {
				get(idx).InOctets = int64(n)
			}
		}},
		{OIDIfOutOctets, func(idx int, v interface{}) {
			if n, ok := v.(uint); ok {
				get(idx).OutOctets = int64(n)
			}
		}},
		{OIDIfInErrors, func(idx int, v interface{}) {
			if n, ok := v.(uint); ok {
				get(idx).InErrors = int64(n)
			}
		}},
		{OIDIfOutErrors, func(idx int, v interface{}) {
			if n, ok := v.(uint); ok {
				get(idx).OutErrors = int64(n)
			}
		}},
	}
	for _, w := range walks {
		err := c.gs.BulkWalk(w.oid, func(pdu g.SnmpPDU) error {
			idx := lastIndexFromOID(pdu.Name)
			if idx <= 0 {
				return nil
			}
			w.assign(idx, pdu.Value)
			return nil
		})
		if err != nil {
			// Some Mimosa firmwares return noSuchInstance for ifXTable.
			// We do not fail the whole walk; the row falls back to ifTable.
			continue
		}
	}
	out := make([]MimosaInterfaceMetric, 0, len(rows))
	for _, r := range rows {
		out = append(out, *r)
	}
	return out, nil
}

func lastIndexFromOID(name string) int {
	dot := strings.LastIndexByte(name, '.')
	if dot < 0 || dot == len(name)-1 {
		return 0
	}
	return int(parseInt64(name[dot+1:]))
}
