package scoring

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Hydrator, müşteri bağlamında telemetri + AP-client test sonuçlarından
// scoring.Inputs üretir.
//
// Veri kaynakları (mevcut tablolar):
//   - customer_devices       (CPE/AP üyelikleri)
//   - mikrotik_wireless_clients (RouterOS registration table snapshot)
//   - mimosa_wireless_clients   (Mimosa SNMP snapshot)
//   - ap_client_test_results    (Faz 5 testleri)
//   - device_poll_results       (en son poll zamanı için)
type Hydrator struct {
	P *pgxpool.Pool
}

// NewHydrator döner.
func NewHydrator(p *pgxpool.Pool) *Hydrator { return &Hydrator{P: p} }

// CustomerHydratedInput, hydrate sonucu skor motoruna verilecek paket.
type CustomerHydratedInput struct {
	CustomerID string
	APDeviceID *string
	TowerID    *string
	Inputs     Inputs
}

// HydrateCustomer, customer_id'den skor için input toplar.
// Yetersiz veri varsa Inputs alanları nil bırakılır; motor kendisi
// data_insufficient/stale_data tanısı koyar.
func (h *Hydrator) HydrateCustomer(ctx context.Context, customerID string) (*CustomerHydratedInput, error) {
	out := &CustomerHydratedInput{CustomerID: customerID}

	// 1) Tower
	var towerID *string
	if err := h.P.QueryRow(ctx,
		`SELECT tower_id::text FROM customers WHERE id = $1`, customerID,
	).Scan(&towerID); err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	out.TowerID = nullableStr(towerID)

	// 2) AP cihaz: customer_devices.role = 'ap' ya da CPE üzerinden inferred
	var apDeviceID *string
	_ = h.P.QueryRow(ctx, `
		SELECT device_id::text FROM customer_devices
		WHERE customer_id = $1 AND role = 'ap'
		LIMIT 1`, customerID).Scan(&apDeviceID)
	if apDeviceID == nil {
		// CPE'nin connected device'ından AP çıkarılabilir; şimdilik AP
		// doğrudan customer_devices'da yoksa boş bırak.
	}
	out.APDeviceID = nullableStr(apDeviceID)

	// 3) En son MikroTik wireless client snapshot
	//    customer'ın CPE device_id'lerini bul
	cpeIDs, err := h.cpeDeviceIDs(ctx, customerID)
	if err != nil {
		return nil, err
	}

	var lastSampleAt *time.Time
	if len(cpeIDs) > 0 || apDeviceID != nil {
		sample, sampleAt := h.latestWirelessSample(ctx, apDeviceID, cpeIDs)
		if sample != nil {
			out.Inputs.RSSIdBm = sample.RSSIdBm
			out.Inputs.SNRdB = sample.SNRdB
			out.Inputs.CCQ = sample.CCQ
			out.Inputs.TxRateMbps = sample.TxRateMbps
			out.Inputs.RxRateMbps = sample.RxRateMbps
		}
		lastSampleAt = sampleAt
	}
	out.Inputs.LastSampleAt = lastSampleAt

	// 4) En son AP-client test sonucu
	testInputs, lastTestAt, ok := h.latestAPClientTest(ctx, customerID)
	if ok {
		out.Inputs.AvgLatencyMs = testInputs.AvgLatencyMs
		out.Inputs.MaxLatencyMs = testInputs.MaxLatencyMs
		out.Inputs.PacketLossPct = testInputs.PacketLossPct
		out.Inputs.JitterMs = testInputs.JitterMs
		out.Inputs.LastTestSuccess = testInputs.LastTestSuccess
	}
	out.Inputs.LastTestAt = lastTestAt

	// 5) AP-wide degradation: aynı AP altındaki müşterilerden kaç tanesi critical?
	if out.APDeviceID != nil {
		total, critical := h.peerCustomerStats(ctx, *out.APDeviceID)
		if total > 0 {
			t := total
			c := critical
			out.Inputs.APWideCustomerCount = &t
			out.Inputs.APWideDegradedCustCnt = &c
		}
	}

	// 6) Trend (7d): mikrotik_wireless_clients RSSI tarihçesi
	if len(cpeIDs) > 0 {
		samples := h.signalSamples7d(ctx, cpeIDs)
		slope := SignalTrend7d(samples, time.Now())
		out.Inputs.SignalTrend7d = slope
	}

	return out, nil
}

// cpeDeviceIDs, müşteriye bağlı CPE device id'leri.
func (h *Hydrator) cpeDeviceIDs(ctx context.Context, customerID string) ([]string, error) {
	rows, err := h.P.Query(ctx, `
		SELECT device_id::text FROM customer_devices
		WHERE customer_id = $1 AND role = 'cpe' AND device_id IS NOT NULL`, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// wirelessSample, ortak yapı.
type wirelessSample struct {
	RSSIdBm    *float64
	SNRdB      *float64
	CCQ        *float64
	TxRateMbps *float64
	RxRateMbps *float64
	CollectedAt time.Time
}

// latestWirelessSample, müşteriye ilişkin en güncel kablosuz örneği döner.
// AP varsa AP altında müşterinin MAC'i ile eşleşmeyi tercih eder; basitlik
// adına son CPE örneğini de fallback olarak kullanır.
func (h *Hydrator) latestWirelessSample(ctx context.Context, apDeviceID *string, cpeIDs []string) (*wirelessSample, *time.Time) {
	// MikroTik CPE örneği (en son satır)
	if len(cpeIDs) > 0 {
		// Devices listesinde olmayan id'ler atlanacak; ANY kullanıyoruz
		row := h.P.QueryRow(ctx, `
			SELECT signal_strength_dbm::float8, signal_to_noise::float8,
			       ccq::float8, tx_rate_mbps::float8, rx_rate_mbps::float8,
			       collected_at
			FROM mikrotik_wireless_clients
			WHERE device_id = ANY($1::uuid[])
			ORDER BY collected_at DESC
			LIMIT 1`, cpeIDs)
		s := &wirelessSample{}
		if err := row.Scan(&s.RSSIdBm, &s.SNRdB, &s.CCQ, &s.TxRateMbps, &s.RxRateMbps, &s.CollectedAt); err == nil {
			return s, &s.CollectedAt
		}
	}
	// Mimosa fallback
	if apDeviceID != nil {
		row := h.P.QueryRow(ctx, `
			SELECT signal_dbm::float8, snr_db::float8,
			       NULL::float8 AS ccq, NULL::float8 AS tx, NULL::float8 AS rx,
			       collected_at
			FROM mimosa_wireless_clients
			WHERE device_id = $1
			ORDER BY collected_at DESC
			LIMIT 1`, *apDeviceID)
		s := &wirelessSample{}
		if err := row.Scan(&s.RSSIdBm, &s.SNRdB, &s.CCQ, &s.TxRateMbps, &s.RxRateMbps, &s.CollectedAt); err == nil {
			return s, &s.CollectedAt
		}
	}
	return nil, nil
}

// signalSamples7d, son 7 günde RSSI örnekleri.
func (h *Hydrator) signalSamples7d(ctx context.Context, cpeIDs []string) []SignalSample {
	if len(cpeIDs) == 0 {
		return nil
	}
	rows, err := h.P.Query(ctx, `
		SELECT collected_at, signal_strength_dbm::float8
		FROM mikrotik_wireless_clients
		WHERE device_id = ANY($1::uuid[])
		  AND signal_strength_dbm IS NOT NULL
		  AND collected_at >= now() - interval '7 days'
		ORDER BY collected_at ASC`, cpeIDs)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []SignalSample{}
	for rows.Next() {
		var s SignalSample
		if err := rows.Scan(&s.At, &s.RSSIdBm); err == nil {
			out = append(out, s)
		}
	}
	return out
}

// latestAPClientTest, müşterinin son AP-client test sonucu.
func (h *Hydrator) latestAPClientTest(ctx context.Context, customerID string) (Inputs, *time.Time, bool) {
	row := h.P.QueryRow(ctx, `
		SELECT latency_avg_ms::float8, latency_max_ms::float8,
		       packet_loss_pct::float8, jitter_ms::float8,
		       (status = 'success'),
		       executed_at
		FROM ap_client_test_results
		WHERE customer_id = $1
		ORDER BY executed_at DESC
		LIMIT 1`, customerID)
	var (
		lat, latMax, loss, jitter *float64
		ok bool
		execAt time.Time
	)
	if err := row.Scan(&lat, &latMax, &loss, &jitter, &ok, &execAt); err != nil {
		return Inputs{}, nil, false
	}
	return Inputs{
		AvgLatencyMs:    lat,
		MaxLatencyMs:    latMax,
		PacketLossPct:   loss,
		JitterMs:        jitter,
		LastTestSuccess: &ok,
	}, &execAt, true
}

// peerCustomerStats, AP cihaz altındaki toplam ve critical müşteri sayısı.
// customers.last_signal_severity cache'inden okur.
func (h *Hydrator) peerCustomerStats(ctx context.Context, apDeviceID string) (total, critical int) {
	_ = h.P.QueryRow(ctx, `
		SELECT
		  COUNT(*) FILTER (WHERE c.id IS NOT NULL),
		  COUNT(*) FILTER (WHERE c.last_signal_severity = 'critical')
		FROM customers c
		JOIN customer_devices cd ON cd.customer_id = c.id AND cd.role = 'ap'
		WHERE cd.device_id = $1`, apDeviceID).Scan(&total, &critical)
	return
}

func nullableStr(s *string) *string {
	if s == nil || *s == "" {
		return nil
	}
	v := *s
	return &v
}
