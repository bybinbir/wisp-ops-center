// Package main, wisp-ops-center worker prosesinin giriş noktasıdır.
//
// Faz 1: Worker yalnızca iskelet olarak çalışır. Asynq/Redis bağlantısı
// kurulmaz; iş tipleri kayıt edilir, ayda bir tetik döngüsü test
// amacıyla loglara not düşer ve hiçbir cihaz I/O'su yapılmaz.
package main

import (
	"context"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	workerinternal "github.com/wisp-ops-center/wisp-ops-center/apps/worker/internal"
	"github.com/wisp-ops-center/wisp-ops-center/internal/config"
	"github.com/wisp-ops-center/wisp-ops-center/internal/database"
	"github.com/wisp-ops-center/wisp-ops-center/internal/logger"
	"github.com/wisp-ops-center/wisp-ops-center/internal/scheduler"
)

func main() {
	log := logger.New("wisp-ops-worker")

	cfg, err := config.Load()
	if err != nil {
		log.Error("config_load_failed", "err", err)
		os.Exit(1)
	}

	log.Info("worker_boot",
		"env", cfg.Env,
		"redis_configured", cfg.Redis.Addr != "",
	)

	if cfg.Redis.Addr == "" {
		log.Warn("redis_not_configured",
			"hint", "REDIS_ADDR boş; worker iskelet modunda çalışıyor, gerçek kuyruk yok")
	}

	registry := workerinternal.NewRegistry()
	for _, jt := range scheduler.AllJobTypes() {
		registry.Register(jt, workerinternal.SkeletonHandler(jt, log))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go heartbeat(ctx, log, registry)

	if strings.EqualFold(os.Getenv("WISP_SCHEDULER_ENABLED"), "true") && cfg.Database.DSN != "" {
		db, err := database.Open(ctx, cfg.Database.DSN, log)
		if err != nil {
			log.Warn("scheduler_loop_db_open_failed", "err", err)
		} else {
			defer db.Close()
			// Faz 6: customer_signal_check için gerçek handler
			registry.Register(scheduler.JobCustomerSignalCheck,
				workerinternal.CustomerSignalCheckHandler(db.P, log))
			// Faz 7: daily_executive_summary handler'ı.
			registry.Register(scheduler.JobDailyExecutiveSummary,
				workerinternal.DailyExecutiveSummaryHandler(db.P, log))
			go workerinternal.RunSchedulerLoop(ctx, db.P, log, registry, workerinternal.SchedulerLoopConfig{})
		}
	} else {
		log.Info("scheduler_loop_disabled", "hint", "set WISP_SCHEDULER_ENABLED=true and WISP_DATABASE_URL")
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Info("worker_shutdown")
}

func heartbeat(ctx context.Context, log *logger.Logger, r *workerinternal.Registry) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			log.Info("worker_heartbeat",
				"registered_job_types", r.Len(),
				"phase", 1,
				"mode", "skeleton",
			)
		}
	}
}
