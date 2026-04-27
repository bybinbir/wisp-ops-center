package telemetry

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/wisp-ops-center/wisp-ops-center/internal/adapters/mimosa"
)

// PersistMimosaSnapshot writes the Mimosa snapshot into vendor + shared
// telemetry tables. It is best-effort: a single insert error does not
// abort the whole persistence — the caller already has the snapshot.
func (r *Repository) PersistMimosaSnapshot(ctx context.Context, pollID int64, snap *mimosa.MimosaReadOnlySnapshot) error {
	collected := snap.FinishedAt
	if collected.IsZero() {
		collected = time.Now().UTC()
	}

	if snap.System != nil {
		if _, err := r.P.Exec(ctx, `INSERT INTO telemetry_snapshots
			(device_id, collected_at, online, uptime_sec)
			VALUES ($1,$2,TRUE,$3)`,
			snap.DeviceID, collected,
			func() any {
				if snap.System.UptimeSec > 0 {
					return snap.System.UptimeSec
				}
				return nil
			}(),
		); err != nil {
			return fmt.Errorf("telemetry_snapshots: %w", err)
		}
	}

	for _, m := range snap.Interfaces {
		if _, err := r.P.Exec(ctx, `INSERT INTO interface_metrics
			(device_id, poll_result_id, name, type, running, disabled, mtu, rx_bytes, tx_bytes, rx_errors, tx_errors, collected_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
			snap.DeviceID, pollID, mimosaIfName(m), fmt.Sprintf("%d", m.Type),
			m.OperUp, !m.AdminUp, m.MTU,
			m.InOctets, m.OutOctets, m.InErrors, m.OutErrors, collected,
		); err != nil {
			return fmt.Errorf("interface_metrics: %w", err)
		}
	}

	for _, w := range snap.Radios {
		if _, err := r.P.Exec(ctx, `INSERT INTO mimosa_wireless_interfaces
			(device_id, poll_result_id, name, frequency_mhz, channel_width_mhz, tx_power_dbm, noise_floor_dbm, collected_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			snap.DeviceID, pollID, w.Name,
			intPtrToAny(w.FrequencyMHz), intPtrToAny(w.ChannelWidthMHz),
			floatPtrToAny(w.TxPowerDBm), floatPtrToAny(w.NoiseFloor),
			collected,
		); err != nil {
			return fmt.Errorf("mimosa_wireless_interfaces: %w", err)
		}
	}

	for _, l := range snap.Links {
		if _, err := r.P.Exec(ctx, `INSERT INTO mimosa_links
			(device_id, poll_result_id, name, peer_ip, signal_dbm, snr_db, capacity_mbps, uptime_sec, station_count, collected_at)
			VALUES ($1,$2,$3,NULLIF($4,'')::inet,$5,$6,$7,$8,$9,$10)`,
			snap.DeviceID, pollID, l.Name, l.PeerIP,
			floatPtrToAny(l.SignalDBm), floatPtrToAny(l.SignalToNoise),
			floatPtrToAny(l.CapacityMbps), l.UptimeSec, intPtrToAny(l.StationCount),
			collected,
		); err != nil {
			return fmt.Errorf("mimosa_links: %w", err)
		}
	}

	for _, c := range snap.Clients {
		if _, err := r.P.Exec(ctx, `INSERT INTO mimosa_wireless_clients
			(device_id, poll_result_id, mac, ip, hostname, signal_dbm, snr_db, tx_rate_mbps, rx_rate_mbps, uptime_sec, collected_at)
			VALUES ($1,$2,NULLIF($3,'')::macaddr,NULLIF($4,'')::inet,$5,$6,$7,$8,$9,$10,$11)`,
			snap.DeviceID, pollID, c.MAC, c.IP, c.Hostname,
			floatPtrToAny(c.SignalDBm), floatPtrToAny(c.SignalToNoise),
			floatPtrToAny(c.TxRateMbps), floatPtrToAny(c.RxRateMbps),
			c.UptimeSec, collected,
		); err != nil {
			return fmt.Errorf("mimosa_wireless_clients: %w", err)
		}
	}

	if snap.VendorMIBStatus != "" {
		_, _ = r.P.Exec(ctx,
			`UPDATE device_poll_results SET vendor_mib_status = $2 WHERE id = $1`,
			pollID, snap.VendorMIBStatus,
		)
	}

	return nil
}

// CapabilitiesUpsertMimosa writes Mimosa capability flags. Mirror of
// the MikroTik upsert, but never sets RouterOS/SSH-specific flags. Like
// the MikroTik path, it never sets write capabilities.
func (r *Repository) CapabilitiesUpsertMimosa(ctx context.Context, deviceID string, caps mimosa.CapabilityFlags) error {
	_, err := r.P.Exec(ctx, `
INSERT INTO device_capabilities (
    device_id, can_read_health, can_read_wireless_metrics, can_read_clients,
    can_read_frequency, can_recommend_frequency, supports_snmp,
    supports_vendor_api, last_verified_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8, now())
ON CONFLICT (device_id) DO UPDATE SET
    can_read_health           = EXCLUDED.can_read_health OR device_capabilities.can_read_health,
    can_read_wireless_metrics = EXCLUDED.can_read_wireless_metrics OR device_capabilities.can_read_wireless_metrics,
    can_read_clients          = EXCLUDED.can_read_clients OR device_capabilities.can_read_clients,
    can_read_frequency        = EXCLUDED.can_read_frequency OR device_capabilities.can_read_frequency,
    can_recommend_frequency   = EXCLUDED.can_recommend_frequency OR device_capabilities.can_recommend_frequency,
    supports_snmp             = EXCLUDED.supports_snmp OR device_capabilities.supports_snmp,
    supports_vendor_api       = EXCLUDED.supports_vendor_api OR device_capabilities.supports_vendor_api,
    last_verified_at          = now()
`,
		deviceID,
		caps.CanReadHealth, caps.CanReadWirelessMetrics, caps.CanReadClients,
		caps.CanReadFrequency, caps.CanRecommendFrequency,
		caps.SupportsSNMP, caps.SupportsVendorAPI,
	)
	return err
}

// LatestMimosaClients returns the most recent Mimosa station rows.
func (r *Repository) LatestMimosaClients(ctx context.Context, deviceID string) ([]map[string]any, error) {
	rows, err := r.P.Query(ctx, `
SELECT COALESCE(mac::text,''), COALESCE(host(ip),''), COALESCE(hostname,''),
       signal_dbm, snr_db, tx_rate_mbps, rx_rate_mbps, uptime_sec, collected_at
  FROM mimosa_wireless_clients
 WHERE device_id = $1
   AND collected_at = (SELECT MAX(collected_at) FROM mimosa_wireless_clients WHERE device_id = $1)
 ORDER BY signal_dbm DESC NULLS LAST`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var mac, ip, host string
		var sig, snr, tx, rx *float64
		var up *int64
		var at time.Time
		if err := rows.Scan(&mac, &ip, &host, &sig, &snr, &tx, &rx, &up, &at); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"mac": mac, "ip": ip, "hostname": host,
			"signal_dbm": sig, "snr_db": snr,
			"tx_rate_mbps": tx, "rx_rate_mbps": rx,
			"uptime_sec": up, "collected_at": at,
		})
	}
	return out, rows.Err()
}

// LatestMimosaLinks returns the most recent backhaul link rows.
func (r *Repository) LatestMimosaLinks(ctx context.Context, deviceID string) ([]map[string]any, error) {
	rows, err := r.P.Query(ctx, `
SELECT name, COALESCE(host(peer_ip),''), signal_dbm, snr_db, capacity_mbps, uptime_sec, station_count, collected_at
  FROM mimosa_links
 WHERE device_id = $1
   AND collected_at = (SELECT MAX(collected_at) FROM mimosa_links WHERE device_id = $1)
 ORDER BY name`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var name, peer string
		var sig, snr, cap *float64
		var up *int64
		var stations *int
		var at time.Time
		if err := rows.Scan(&name, &peer, &sig, &snr, &cap, &up, &stations, &at); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"name": name, "peer_ip": peer,
			"signal_dbm": sig, "snr_db": snr,
			"capacity_mbps": cap, "uptime_sec": up,
			"station_count": stations, "collected_at": at,
		})
	}
	return out, rows.Err()
}

// helpers used only by the Mimosa side ------------------------------

func mimosaIfName(m mimosa.MimosaInterfaceMetric) string {
	if m.Name != "" {
		return m.Name
	}
	if m.Descr != "" {
		return m.Descr
	}
	return fmt.Sprintf("if%d", m.Index)
}

// guard against unused-import on some build paths
var _ = errors.New
