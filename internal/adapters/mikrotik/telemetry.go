package mikrotik

import (
	"context"
	"errors"
	"time"
)

// Poll runs a read-only telemetry collection and returns a normalized
// snapshot. Errors collected per command are appended to snapshot.Errors
// in sanitized form; one bad command does not abort the whole poll.
func Poll(ctx context.Context, cfg Config, secret string) (*MikroTikReadOnlySnapshot, CapabilityFlags, error) {
	start := time.Now().UTC()
	snap := &MikroTikReadOnlySnapshot{
		DeviceID:  cfg.DeviceID,
		Transport: string(cfg.Transport),
		StartedAt: start,
	}
	caps := CapabilityFlags{}

	switch cfg.Transport {
	case TransportAPISSL:
		c := NewAPIClient(cfg, secret)
		if err := c.Dial(ctx); err != nil {
			snap.FinishedAt = time.Now().UTC()
			snap.DurationMS = snap.FinishedAt.Sub(start).Milliseconds()
			snap.Errors = append(snap.Errors, SanitizeError(err))
			return snap, caps, err
		}
		defer c.Close()
		caps.SupportsRouterOSAPI = true

		// System
		if rows, err := c.Run(ctx, "/system/resource/print"); err == nil && len(rows) > 0 {
			r := rows[0]
			snap.System = &MikroTikSystemInfo{
				BoardName:    r["board-name"],
				Architecture: r["architecture-name"],
				Version:      r["version"],
				UptimeSec:    parseUptime(r["uptime"]),
				CPULoadPct:   parseFloatPtr(r["cpu-load"]),
				FreeMemoryB: func() *int64 {
					v := parseInt64(r["free-memory"])
					if v > 0 {
						return &v
					}
					return nil
				}(),
				TotalMemoryB: func() *int64 {
					v := parseInt64(r["total-memory"])
					if v > 0 {
						return &v
					}
					return nil
				}(),
				TempCelsius: parseFloatPtr(r["temperature"]),
			}
			caps.CanReadHealth = true
		} else if err != nil {
			snap.Errors = append(snap.Errors, "system_resource:"+SanitizeError(err))
		}

		// Interfaces
		if rows, err := c.Run(ctx, "/interface/print"); err == nil {
			for _, r := range rows {
				snap.Interfaces = append(snap.Interfaces, MikroTikInterfaceMetric{
					Name:        r["name"],
					Type:        r["type"],
					Running:     parseBool(r["running"]),
					Disabled:    parseBool(r["disabled"]),
					MTU:         parseInt(r["mtu"]),
					MAC:         normalizeMAC(r["mac-address"]),
					RxByte:      parseInt64(r["rx-byte"]),
					TxByte:      parseInt64(r["tx-byte"]),
					RxPacket:    parseInt64(r["rx-packet"]),
					TxPacket:    parseInt64(r["tx-packet"]),
					RxError:     parseInt64(r["rx-error"]),
					TxError:     parseInt64(r["tx-error"]),
					LinkDownCnt: parseInt64(r["link-downs"]),
				})
			}
		} else if err != nil {
			snap.Errors = append(snap.Errors, "interface:"+SanitizeError(err))
		}

		// Wireless (legacy first; if that fails silently try wifiwave2/wifi)
		wirelessPath := "/interface/wireless/print"
		regPath := "/interface/wireless/registration-table/print"
		wifiPkg := "legacy"
		if rows, err := c.Run(ctx, wirelessPath); err != nil || len(rows) == 0 {
			if rows2, err2 := c.Run(ctx, "/interface/wifiwave2/print"); err2 == nil && len(rows2) > 0 {
				wirelessPath = "/interface/wifiwave2/print"
				regPath = "/interface/wifiwave2/registration-table/print"
				wifiPkg = "wifiwave2"
			} else if rows3, err3 := c.Run(ctx, "/interface/wifi/print"); err3 == nil && len(rows3) > 0 {
				wirelessPath = "/interface/wifi/print"
				regPath = "/interface/wifi/registration-table/print"
				wifiPkg = "wifi"
			}
		}

		if rows, err := c.Run(ctx, wirelessPath); err == nil && len(rows) > 0 {
			caps.CanReadWirelessMetrics = true
			caps.CanReadFrequency = true
			for _, r := range rows {
				freq := parseIntPtr(r["frequency"])
				width := parseIntPtr(r["channel-width"])
				snap.WirelessInterfaces = append(snap.WirelessInterfaces, MikroTikWirelessInterfaceMetric{
					Name:            r["name"],
					SSID:            r["ssid"],
					Mode:            r["mode"],
					Band:            r["band"],
					FrequencyMHz:    freq,
					ChannelWidthMHz: width,
					TxPowerDBm:      parseFloatPtr(r["tx-power"]),
					NoiseFloor:      parseFloatPtr(r["noise-floor"]),
					Disabled:        parseBool(r["disabled"]),
					Running:         parseBool(r["running"]),
				})
			}
		} else if err != nil {
			snap.Errors = append(snap.Errors, "wireless_print:"+SanitizeError(err))
		}

		if rows, err := c.Run(ctx, regPath); err == nil {
			caps.CanReadClients = len(rows) >= 0 // table accessible even if empty
			for _, r := range rows {
				snap.WirelessClients = append(snap.WirelessClients, MikroTikWirelessClientMetric{
					Interface:     r["interface"],
					MAC:           normalizeMAC(r["mac-address"]),
					IP:            r["last-ip"],
					SSID:          r["ssid"],
					UptimeSec:     parseUptime(r["uptime"]),
					SignalDBm:     parseFloatPtr(r["signal-strength"]),
					SignalToNoise: parseFloatPtr(r["signal-to-noise"]),
					CCQ:           parseFloatPtr(r["tx-ccq"]),
				})
			}
		} else if err != nil {
			snap.Errors = append(snap.Errors, "registration_table:"+SanitizeError(err))
		}

		_ = wifiPkg
		caps.CanRecommendFrequency = caps.CanReadHealth && caps.CanReadFrequency

		snap.FinishedAt = time.Now().UTC()
		snap.DurationMS = snap.FinishedAt.Sub(start).Milliseconds()
		return snap, caps, nil

	case TransportSNMP:
		// SNMP poll: we only have minimal system info in Phase 3.
		s := NewSNMPClient(cfg, cfg.SNMPCommunity)
		if err := s.Dial(); err != nil {
			snap.FinishedAt = time.Now().UTC()
			snap.DurationMS = snap.FinishedAt.Sub(start).Milliseconds()
			snap.Errors = append(snap.Errors, SanitizeError(err))
			return snap, caps, err
		}
		defer s.Close()
		caps.SupportsSNMP = true

		descr, name, up, err := s.SystemInfo()
		if err != nil {
			snap.Errors = append(snap.Errors, SanitizeError(err))
			snap.FinishedAt = time.Now().UTC()
			snap.DurationMS = snap.FinishedAt.Sub(start).Milliseconds()
			return snap, caps, err
		}
		_ = name
		caps.CanReadHealth = true
		snap.System = &MikroTikSystemInfo{
			Version:   descr,
			UptimeSec: up,
		}
		snap.FinishedAt = time.Now().UTC()
		snap.DurationMS = snap.FinishedAt.Sub(start).Milliseconds()
		return snap, caps, nil

	case TransportSSH:
		// SSH path is used as last-resort; we only run identity to confirm
		// session and skip telemetry collection because parsing the full
		// CLI output reliably belongs to a future iteration.
		c := NewSSHClient(cfg, secret)
		if err := c.Dial(ctx); err != nil {
			snap.FinishedAt = time.Now().UTC()
			snap.DurationMS = snap.FinishedAt.Sub(start).Milliseconds()
			snap.Errors = append(snap.Errors, SanitizeError(err))
			return snap, caps, err
		}
		defer c.Close()
		caps.SupportsSSH = true
		if _, err := c.Exec(ctx, "/system/identity/print"); err == nil {
			caps.CanReadHealth = true
		}
		snap.FinishedAt = time.Now().UTC()
		snap.DurationMS = snap.FinishedAt.Sub(start).Milliseconds()
		return snap, caps, nil
	}

	return snap, caps, errors.Join(ErrTransportUnsupported)
}
