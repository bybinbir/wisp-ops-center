package mikrotik

import (
	"context"
	"strings"
	"time"
)

// Probe verifies device reachability + identity using the configured
// transport. It does NOT collect full telemetry; it only fills enough
// data to update device_capabilities and to confirm "yes, we can talk to
// this device read-only".
func Probe(ctx context.Context, cfg Config, secret string) (*MikroTikProbeResult, CapabilityFlags, error) {
	now := time.Now().UTC()
	out := &MikroTikProbeResult{
		DeviceID:    cfg.DeviceID,
		Transport:   string(cfg.Transport),
		CollectedAt: now,
	}
	caps := CapabilityFlags{}

	switch cfg.Transport {
	case TransportAPISSL:
		c := NewAPIClient(cfg, secret)
		if err := c.Dial(ctx); err != nil {
			out.Error = SanitizeError(err)
			return out, caps, err
		}
		defer c.Close()

		caps.SupportsRouterOSAPI = true

		if rows, err := c.Run(ctx, "/system/identity/print"); err == nil && len(rows) > 0 {
			out.IdentityName = rows[0]["name"]
			caps.CanReadHealth = true
		}

		if rows, err := c.Run(ctx, "/system/resource/print"); err == nil && len(rows) > 0 {
			out.RouterOSVersion = rows[0]["version"]
			out.Architecture = rows[0]["architecture-name"]
			out.Board = rows[0]["board-name"]
			out.UptimeSec = parseUptime(rows[0]["uptime"])
			caps.CanReadHealth = true
		}

		// Detect the wireless package family. We probe each path and
		// keep the first one that returns OK; failures are silent here
		// because some boards have no wireless at all (router/switch).
		if rows, err := c.Run(ctx, "/interface/wireless/print"); err == nil && len(rows) > 0 {
			out.WirelessAvailable = true
			out.WiFiPackage = "legacy"
			caps.CanReadWirelessMetrics = true
			caps.CanReadFrequency = true
		} else if rows, err := c.Run(ctx, "/interface/wifiwave2/print"); err == nil && len(rows) > 0 {
			out.WirelessAvailable = true
			out.WiFiPackage = "wifiwave2"
			caps.CanReadWirelessMetrics = true
			caps.CanReadFrequency = true
			_ = rows
		} else if rows, err := c.Run(ctx, "/interface/wifi/print"); err == nil && len(rows) > 0 {
			out.WirelessAvailable = true
			out.WiFiPackage = "wifi"
			caps.CanReadWirelessMetrics = true
			caps.CanReadFrequency = true
			_ = rows
		}

		// Wireless registration table: only meaningful for AP-style boards.
		if out.WirelessAvailable {
			path := "/interface/wireless/registration-table/print"
			switch out.WiFiPackage {
			case "wifiwave2":
				path = "/interface/wifiwave2/registration-table/print"
			case "wifi":
				path = "/interface/wifi/registration-table/print"
			}
			if rows, err := c.Run(ctx, path); err == nil && rows != nil {
				caps.CanReadClients = true
				_ = rows
			}
		}

		caps.CanRecommendFrequency = caps.CanReadHealth && caps.CanReadFrequency
		out.Reachable = true
		return out, caps, nil

	case TransportSNMP:
		s := NewSNMPClient(cfg, cfg.SNMPCommunity)
		if err := s.Dial(); err != nil {
			out.Error = SanitizeError(err)
			return out, caps, err
		}
		defer s.Close()

		descr, name, up, err := s.SystemInfo()
		if err != nil {
			out.Error = SanitizeError(err)
			return out, caps, err
		}
		caps.SupportsSNMP = true
		caps.CanReadHealth = true

		out.IdentityName = name
		out.UptimeSec = up

		if d := strings.ToLower(descr); strings.Contains(d, "routeros") {
			parts := strings.Fields(descr)
			for i, p := range parts {
				if strings.EqualFold(p, "routeros") && i+1 < len(parts) {
					out.RouterOSVersion = strings.TrimSpace(parts[i+1])
					break
				}
			}
		}
		out.Reachable = true
		return out, caps, nil

	case TransportSSH:
		c := NewSSHClient(cfg, secret)
		if err := c.Dial(ctx); err != nil {
			out.Error = SanitizeError(err)
			return out, caps, err
		}
		defer c.Close()
		caps.SupportsSSH = true
		// Run a single safe identity print to confirm session works.
		if txt, err := c.Exec(ctx, "/system/identity/print"); err == nil {
			caps.CanReadHealth = true
			// Extract "name: foo" line if present.
			for _, line := range strings.Split(txt, "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "name:") {
					out.IdentityName = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
					break
				}
			}
		}
		out.Reachable = true
		return out, caps, nil
	}

	return out, caps, ErrTransportUnsupported
}
