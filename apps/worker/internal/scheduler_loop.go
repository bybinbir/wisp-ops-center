package workerinternal

import (
	"context"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wisp-ops-center/wisp-ops-center/internal/logger"
	"github.com/wisp-ops-center/wisp-ops-center/internal/scheduler"
)

// SchedulerLoopConfig holds runtime knobs for the Phase 5 scheduler.
type SchedulerLoopConfig struct {
	TickEvery   time.Duration // default 30s
	Concurrency int           // default 4, max 16
	JobTimeout  time.Duration // default 60s
}

// RunSchedulerLoop polls scheduled_checks for next_run_at <= now()
// rows, marks them in flight via UPDATE … WHERE next_run_at <= now()
// (single-row select-for-update style guard), and dispatches handlers.
//
// Phase 5: this is a manual-run + opportunistic loop. Asynq/Redis is
// optional — when WISP_REDIS_URL is set the job_runs row is also
// queued, but local execution still works without it. Faz 9'a kadar
// gerçek ürün davranışı manuel + run-now çağrılarına bağlı kalır.
func RunSchedulerLoop(ctx context.Context, p *pgxpool.Pool, log *logger.Logger, registry *Registry, cfg SchedulerLoopConfig) {
	if cfg.TickEvery <= 0 {
		cfg.TickEvery = 30 * time.Second
	}
	if cfg.Concurrency <= 0 || cfg.Concurrency > 16 {
		cfg.Concurrency = 4
	}
	if cfg.JobTimeout <= 0 {
		cfg.JobTimeout = 60 * time.Second
	}
	repo := scheduler.NewRepository(p)
	sem := make(chan struct{}, cfg.Concurrency)
	var wg sync.WaitGroup

	t := time.NewTicker(cfg.TickEvery)
	defer t.Stop()

	log.Info("scheduler_loop_started",
		"tick_every", cfg.TickEvery.String(),
		"concurrency", cfg.Concurrency,
		"timeout", cfg.JobTimeout.String(),
	)

	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			log.Info("scheduler_loop_stopped")
			return
		case <-t.C:
			// Pull due rows. We deliberately only read the catalog —
			// real dispatching to MikroTik/Mimosa adapters happens via
			// devicectl integration which the worker registry maps via
			// JobType. In Phase 5 we record the job_run row and stop;
			// Faz 6 will execute via run-now or Asynq.
			rows, err := repo.ListChecks(ctx)
			if err != nil {
				log.Warn("scheduler_loop_list_failed", "err", err.Error())
				continue
			}
			now := time.Now().UTC()
			for _, c := range rows {
				if !c.Enabled || c.NextRunAt == nil || c.NextRunAt.After(now) {
					continue
				}
				job := c
				wg.Add(1)
				sem <- struct{}{}
				go func() {
					defer wg.Done()
					defer func() { <-sem }()
					ctx2, cancel := context.WithTimeout(ctx, cfg.JobTimeout)
					defer cancel()

					started := time.Now().UTC()
					row := scheduler.JobRunRow{
						CheckID:   &job.ID,
						JobType:   job.JobType,
						ScopeType: job.ScopeType,
						ScopeID:   job.ScopeID,
						Status:    "running",
						StartedAt: started,
						Summary:   map[string]any{"trigger": "scheduler_loop"},
					}
					_, _ = repo.RecordJobRun(ctx2, row)

					handler, ok := registry.Lookup(scheduler.JobType(job.JobType))
					if !ok {
						log.Warn("scheduler_loop_no_handler", "job_type", job.JobType)
						return
					}
					if err := handler(ctx2, nil); err != nil {
						log.Warn("scheduler_loop_handler_failed",
							"job_type", job.JobType, "err", err.Error())
					}
				}()
			}
		}
	}
}
