package workerinternal

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wisp-ops-center/wisp-ops-center/internal/logger"
	"github.com/wisp-ops-center/wisp-ops-center/internal/scoring"
)

// CustomerSignalPayload, customer_signal_check job payload'u.
type CustomerSignalPayload struct {
	CustomerIDs  []string `json:"customer_ids,omitempty"`
	AllActive    bool     `json:"all_active,omitempty"`
	MaxCustomers int      `json:"max_customers,omitempty"`
	TimeoutSec   int      `json:"timeout_sec,omitempty"`
}

// CustomerSignalCheckHandler, müşteri sinyal skor üretici worker handler'ı.
//
// Faz 6: salt-okuma. Her müşteri için telemetri+test verilerinden Inputs
// hydrate eder, deterministik motor ile skor üretir, customer_signal_scores
// tablosuna yazar. Cihaza yazma yapmaz.
func CustomerSignalCheckHandler(p *pgxpool.Pool, log *logger.Logger) Handler {
	return func(ctx context.Context, payload []byte) error {
		if p == nil {
			return errors.New("customer_signal_check: db pool unavailable")
		}
		var pl CustomerSignalPayload
		if len(payload) > 0 {
			if err := json.Unmarshal(payload, &pl); err != nil {
				return err
			}
		}
		max := pl.MaxCustomers
		if max <= 0 || max > 1000 {
			max = 200
		}
		timeout := time.Duration(pl.TimeoutSec) * time.Second
		if timeout <= 0 || timeout > 5*time.Minute {
			timeout = 90 * time.Second
		}
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		repo := scoring.NewRepository(p)
		hyd := scoring.NewHydrator(p)

		ids := pl.CustomerIDs
		if len(ids) == 0 && pl.AllActive {
			rows, err := p.Query(ctx,
				`SELECT id::text FROM customers WHERE status = 'active' LIMIT $1`, max)
			if err != nil {
				return err
			}
			for rows.Next() {
				var id string
				if err := rows.Scan(&id); err == nil {
					ids = append(ids, id)
				}
			}
			rows.Close()
		}
		if len(ids) == 0 {
			log.Info("customer_signal_check_empty", "hint", "neither customer_ids nor all_active provided")
			return nil
		}

		thr, err := repo.LoadThresholds(ctx)
		if err != nil {
			thr = scoring.DefaultThresholds()
		}
		eng := scoring.NewEngine(thr)
		processed, failed := 0, 0
		for _, cid := range ids {
			h, err := hyd.HydrateCustomer(ctx, cid)
			if err != nil {
				failed++
				log.Warn("hydrate_failed", "customer_id", cid, "err", err)
				continue
			}
			res := eng.ScoreCustomer(h.Inputs)
			if _, err := repo.SaveCustomerScore(ctx, cid, h.APDeviceID, h.TowerID, h.Inputs, res); err != nil {
				failed++
				log.Warn("save_score_failed", "customer_id", cid, "err", err)
				continue
			}
			processed++
		}
		log.Info("customer_signal_check_done", "processed", processed, "failed", failed, "total", len(ids))
		return nil
	}
}
