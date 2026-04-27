package mimosa

import (
	"context"
	"time"
)

// Poll runs a read-only SNMP collection. In Phase 4 we collect:
//   - Standard system info (sysDescr/sysName/sysUpTime)
//   - IF-MIB ifTable/ifXTable interface counters
//
// Vendor-specific MIB values are NOT read; the snapshot is marked
// partial and VendorMIBStatus="unverified".
func Poll(ctx context.Context, cfg Config) (*MimosaReadOnlySnapshot, CapabilityFlags, error) {
	start := time.Now().UTC()
	snap := &MimosaReadOnlySnapshot{
		DeviceID:        cfg.DeviceID,
		Transport:       string(cfg.Transport),
		StartedAt:       start,
		Partial:         true,
		VendorMIBStatus: VendorMIBPlaceholder,
	}
	caps := CapabilityFlags{}

	if cfg.Transport == "" {
		cfg.Transport = TransportSNMP
		snap.Transport = string(TransportSNMP)
	}
	if cfg.Transport != TransportSNMP {
		snap.FinishedAt = time.Now().UTC()
		snap.DurationMS = snap.FinishedAt.Sub(start).Milliseconds()
		snap.Errors = append(snap.Errors, "transport_unsupported")
		return snap, caps, ErrTransportUnsupported
	}

	c := NewSNMPClient(cfg)
	if err := c.Dial(); err != nil {
		snap.FinishedAt = time.Now().UTC()
		snap.DurationMS = snap.FinishedAt.Sub(start).Milliseconds()
		snap.Errors = append(snap.Errors, SanitizeError(err))
		return snap, caps, err
	}
	defer c.Close()
	caps.SupportsSNMP = true

	descr, name, up, err := c.SystemInfo()
	if err != nil {
		snap.Errors = append(snap.Errors, "system_info:"+SanitizeError(err))
	} else {
		caps.CanReadHealth = true
		snap.System = &MimosaSystemInfo{
			SystemName:  name,
			SystemDescr: descr,
			UptimeSec:   up,
			Model:       extractModelFromSysDescr(descr),
			Firmware:    extractFirmwareFromSysDescr(descr),
		}
	}

	if rows, ifErr := c.InterfaceTable(); ifErr == nil {
		snap.Interfaces = rows
	} else {
		snap.Errors = append(snap.Errors, "if_table:"+SanitizeError(ifErr))
	}

	snap.FinishedAt = time.Now().UTC()
	snap.DurationMS = snap.FinishedAt.Sub(start).Milliseconds()
	return snap, caps, nil
}
