// Package main is the entrypoint of the wisp-ops-center API server.
package main

import (
	"context"
	"errors"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	httpserver "github.com/wisp-ops-center/wisp-ops-center/apps/api/internal/http"
	"github.com/wisp-ops-center/wisp-ops-center/internal/config"
	"github.com/wisp-ops-center/wisp-ops-center/internal/database"
	"github.com/wisp-ops-center/wisp-ops-center/internal/logger"
)

func main() {
	migrateOnly := flag.Bool("migrate", false, "Run migrations and exit")
	migrationsDir := flag.String("migrations-dir", "migrations", "Path to SQL migrations directory")
	flag.Parse()

	log := logger.New("wisp-ops-api")

	cfg, err := config.Load()
	if err != nil {
		log.Error("config_load_failed", "err", err)
		os.Exit(1)
	}

	log.Info("boot",
		"env", cfg.Env,
		"http_addr", cfg.HTTPAddr,
		"db_configured", cfg.Database.DSN != "",
		"vault_configured", cfg.Vault.Key != "",
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var db *database.Pool
	if cfg.Database.DSN != "" {
		db, err = database.Open(ctx, cfg.Database.DSN, log)
		if err != nil {
			log.Error("db_open_failed", "err", err)
			os.Exit(1)
		}
		defer db.Close()
	} else {
		log.Warn("db_not_configured",
			"hint", "WISP_DATABASE_URL is empty; running without persistence")
	}

	if *migrateOnly {
		if db == nil {
			log.Error("migrate_requires_db", "hint", "set WISP_DATABASE_URL")
			os.Exit(1)
		}
		root, _ := filepath.Abs(*migrationsDir)
		log.Info("migrate_begin", "dir", root)
		if err := db.Migrate(ctx, root, log); err != nil {
			log.Error("migrate_failed", "err", err)
			os.Exit(1)
		}
		log.Info("migrate_done")
		return
	}

	srv := httpserver.New(cfg, db, log)

	go func() {
		log.Info("http_listen", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("http_listen_failed", "err", err)
			os.Exit(1)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	log.Info("shutdown_begin")
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutCancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Error("shutdown_failed", "err", err)
	}
	log.Info("shutdown_complete")
}
