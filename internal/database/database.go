// Package database provides a thin wrapper around jackc/pgx/v5 pool
// for PostgreSQL access.
package database

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool is the shared pgxpool wrapper.
type Pool struct {
	P   *pgxpool.Pool
	log *slog.Logger
}

// Open connects to PostgreSQL using the given DSN. Returns an error
// if the DSN is empty or the pool cannot ping the database.
func Open(ctx context.Context, dsn string, log *slog.Logger) (*Pool, error) {
	if dsn == "" {
		return nil, errors.New("empty DSN")
	}

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}

	cfg.MaxConns = 10
	cfg.MinConns = 1
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute

	p, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("pool new: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := p.Ping(pingCtx); err != nil {
		p.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}

	log.Info("db_connected", "max_conns", cfg.MaxConns)
	return &Pool{P: p, log: log}, nil
}

// Close releases the underlying pool.
func (p *Pool) Close() {
	if p == nil || p.P == nil {
		return
	}
	p.P.Close()
}

// Ping verifies database connectivity.
func (p *Pool) Ping(ctx context.Context) error {
	if p == nil || p.P == nil {
		return errors.New("pool not initialized")
	}
	return p.P.Ping(ctx)
}
