package mikrotik

import (
	"errors"
	"time"

	g "github.com/gosnmp/gosnmp"
)

// SNMPClient wraps a single-shot read-only SNMP query against MikroTik.
// Phase 3 supports v2c only; v3 lands when credential profile schema is
// extended with SNMPv3 USM fields (planned for late Phase 3 / Phase 4).
type SNMPClient struct {
	cfg       Config
	community string
	gs        *g.GoSNMP
}

// NewSNMPClient builds the wrapper without dialing.
func NewSNMPClient(cfg Config, community string) *SNMPClient {
	return &SNMPClient{cfg: cfg, community: community}
}

// Dial establishes the UDP transport.
func (c *SNMPClient) Dial() error {
	if c.cfg.Host == "" {
		return ErrUnreachable
	}
	port := uint16(c.cfg.Port)
	if port == 0 {
		port = 161
	}
	c.gs = &g.GoSNMP{
		Target:    c.cfg.Host,
		Port:      port,
		Community: c.community,
		Version:   g.Version2c,
		Timeout:   3 * time.Second,
		Retries:   1,
		MaxOids:   60,
	}
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

// SystemInfo returns sysDescr/sysUpTime/sysName via SNMP. We deliberately
// keep this small — Phase 3 SNMP is only a fallback for "is this device
// alive" + minimal identity when API and SSH both fail.
func (c *SNMPClient) SystemInfo() (descr string, sysName string, uptimeSec int64, err error) {
	if c.gs == nil {
		return "", "", 0, errors.New("snmp: not connected")
	}
	res, gerr := c.gs.Get([]string{
		"1.3.6.1.2.1.1.1.0", // sysDescr
		"1.3.6.1.2.1.1.5.0", // sysName
		"1.3.6.1.2.1.1.3.0", // sysUpTime (TimeTicks, 1/100 sec)
	})
	if gerr != nil {
		return "", "", 0, ClassifyError(gerr)
	}
	for _, v := range res.Variables {
		switch v.Name {
		case ".1.3.6.1.2.1.1.1.0":
			if b, ok := v.Value.([]byte); ok {
				descr = string(b)
			}
		case ".1.3.6.1.2.1.1.5.0":
			if b, ok := v.Value.([]byte); ok {
				sysName = string(b)
			}
		case ".1.3.6.1.2.1.1.3.0":
			if t, ok := v.Value.(uint32); ok {
				uptimeSec = int64(t) / 100
			}
		}
	}
	return descr, sysName, uptimeSec, nil
}
