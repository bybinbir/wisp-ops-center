package mimosa

import (
	"context"
	"strings"
	"time"
)

// Probe verifies device reachability via SNMP and returns a normalized
// probe result. Vendor-specific MIBs are NOT queried in Phase 4 — the
// adapter only reads standard SNMP MIBs and reports a "partial"
// snapshot.
//
// The returned CapabilityFlags reflect ONLY what we proved during the
// probe; write capabilities are never set.
func Probe(ctx context.Context, cfg Config) (*MimosaProbeResult, CapabilityFlags, error) {
	now := time.Now().UTC()
	out := &MimosaProbeResult{
		DeviceID:        cfg.DeviceID,
		Transport:       string(cfg.Transport),
		CollectedAt:     now,
		Partial:         true, // vendor MIB unverified in Phase 4
		VendorMIBStatus: VendorMIBPlaceholder,
	}
	caps := CapabilityFlags{}

	if cfg.Transport == "" {
		cfg.Transport = TransportSNMP
		out.Transport = string(TransportSNMP)
	}
	if cfg.Transport != TransportSNMP {
		out.Error = "transport_unsupported"
		return out, caps, ErrTransportUnsupported
	}

	c := NewSNMPClient(cfg)
	if err := c.Dial(); err != nil {
		out.Error = SanitizeError(err)
		return out, caps, err
	}
	defer c.Close()

	descr, name, up, err := c.SystemInfo()
	if err != nil {
		out.Error = SanitizeError(err)
		return out, caps, err
	}

	caps.SupportsSNMP = true
	caps.CanReadHealth = true
	out.SystemName = name
	out.SystemDescr = descr
	out.UptimeSec = up
	out.Reachable = true

	if descr != "" {
		out.Model = extractModelFromSysDescr(descr)
		out.Firmware = extractFirmwareFromSysDescr(descr)
	}

	// Wireless availability is a best-effort guess in Phase 4 — we only
	// claim it if sysDescr advertises a Mimosa product family token.
	if descrLower := strings.ToLower(descr); strings.Contains(descrLower, "mimosa") {
		// Identity confirms vendor; capability stays partial because
		// vendor MIB is not used. UI shows VendorMIBStatus="unverified".
		out.WirelessAvailable = false
	}

	// Vendor MIB hard-locked in Faz 4: never set canReadFrequency,
	// canReadWirelessMetrics, canReadClients here. They will only flip
	// to TRUE in Phase 5+ when the verified MIB integration lands.

	return out, caps, nil
}
