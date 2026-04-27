package workerinternal

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wisp-ops-center/wisp-ops-center/internal/logger"
	"github.com/wisp-ops-center/wisp-ops-center/internal/reports"
)

// DailyExecutiveSummaryPayload, daily_executive_summary job payload'u.
type DailyExecutiveSummaryPayload struct {
	GeneratedBy string `json:"generated_by,omitempty"`
}

// DailyExecutiveSummaryHandler, executive summary anlık görüntüsünü
// üretip report_snapshots tablosuna yazar.
//
// Faz 7 kuralları:
//   - Cihaza yazma yapmaz; yalnızca okur ve özet hesaplar.
//   - Frekans değiştirmez; bandwidth-test çalıştırmaz.
//   - Hata durumunda job_runs satırı üst katmanda failed olarak işaretlenir.
func DailyExecutiveSummaryHandler(p *pgxpool.Pool, log *logger.Logger) Handler {
	return func(ctx context.Context, payload []byte) error {
		if p == nil {
			return errors.New("daily_executive_summary: db pool unavailable")
		}
		var pl DailyExecutiveSummaryPayload
		if len(payload) > 0 {
			if err := json.Unmarshal(payload, &pl); err != nil {
				return err
			}
		}
		if pl.GeneratedBy == "" {
			pl.GeneratedBy = "scheduler"
		}

		ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()

		repo := reports.NewRepository(p)
		es, err := repo.BuildExecutiveSummary(ctx)
		if err != nil {
			log.Warn("daily_exec_summary_build_failed", "err", err)
			return err
		}
		id, err := repo.SaveSnapshot(ctx, "executive_summary",
			es.PeriodStart, es.PeriodEnd, es, pl.GeneratedBy)
		if err != nil {
			log.Warn("daily_exec_summary_save_failed", "err", err)
			return err
		}
		log.Info("daily_exec_summary_done",
			"snapshot_id", id,
			"critical", es.CriticalCustomers,
			"warning", es.WarningCustomers,
			"open_work_orders", es.OpenWorkOrders,
		)
		return nil
	}
}
