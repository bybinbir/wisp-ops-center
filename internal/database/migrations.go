package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// MigrationFile represents a single .sql migration on disk.
type MigrationFile struct {
	Index    int
	Name     string
	Path     string
	Checksum string
}

// Discover walks the given directory and returns sql migrations
// sorted by their NNNNNN_ prefix.
func Discover(root string) ([]MigrationFile, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}

	var out []MigrationFile
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}
		idx := 0
		if _, err := fmt.Sscanf(name, "%06d_", &idx); err != nil {
			continue
		}
		path := filepath.Join(root, name)
		body, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil, fmt.Errorf("read %s: %w", path, rerr)
		}
		sum := sha256.Sum256(body)
		out = append(out, MigrationFile{
			Index:    idx,
			Name:     name,
			Path:     path,
			Checksum: hex.EncodeToString(sum[:]),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Index < out[j].Index })
	return out, nil
}

// Discover with embed.FS support.
func DiscoverFS(fsys fs.FS, root string) ([]MigrationFile, error) {
	entries, err := fs.ReadDir(fsys, root)
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}

	var out []MigrationFile
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}
		idx := 0
		if _, err := fmt.Sscanf(name, "%06d_", &idx); err != nil {
			continue
		}
		path := filepath.ToSlash(filepath.Join(root, name))
		body, rerr := fs.ReadFile(fsys, path)
		if rerr != nil {
			return nil, fmt.Errorf("read %s: %w", path, rerr)
		}
		sum := sha256.Sum256(body)
		out = append(out, MigrationFile{
			Index:    idx,
			Name:     name,
			Path:     path,
			Checksum: hex.EncodeToString(sum[:]),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Index < out[j].Index })
	return out, nil
}

// EnsureSchemaMigrationsTable creates the bookkeeping table.
func (p *Pool) EnsureSchemaMigrationsTable(ctx context.Context) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    BIGINT PRIMARY KEY,
    name       TEXT NOT NULL,
    checksum   TEXT NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);`
	_, err := p.P.Exec(ctx, ddl)
	return err
}

// AppliedMigration is one record from schema_migrations.
type AppliedMigration struct {
	Version   int
	Name      string
	Checksum  string
	AppliedAt time.Time
}

// ListApplied returns all rows in schema_migrations.
func (p *Pool) ListApplied(ctx context.Context) ([]AppliedMigration, error) {
	rows, err := p.P.Query(ctx, `SELECT version, name, checksum, applied_at FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AppliedMigration
	for rows.Next() {
		var m AppliedMigration
		if err := rows.Scan(&m.Version, &m.Name, &m.Checksum, &m.AppliedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// MigrationDriftError is returned when a migration that has been
// applied differs from the file checksum on disk.
type MigrationDriftError struct {
	Version int
	Name    string
	Want    string
	Got     string
}

func (e *MigrationDriftError) Error() string {
	return fmt.Sprintf("migration drift: version %d (%s) checksum mismatch want=%s got=%s", e.Version, e.Name, e.Want, e.Got)
}

// Migrate runs pending migrations from disk in order. It is
// idempotent: previously applied versions are skipped if their
// checksum matches; a mismatch returns MigrationDriftError.
func (p *Pool) Migrate(ctx context.Context, root string, log *slog.Logger) error {
	if err := p.EnsureSchemaMigrationsTable(ctx); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}

	files, err := Discover(root)
	if err != nil {
		return err
	}

	applied, err := p.ListApplied(ctx)
	if err != nil {
		return fmt.Errorf("list applied: %w", err)
	}
	appliedByVersion := make(map[int]AppliedMigration, len(applied))
	for _, a := range applied {
		appliedByVersion[a.Version] = a
	}

	for _, f := range files {
		if a, ok := appliedByVersion[f.Index]; ok {
			if a.Checksum != f.Checksum {
				return &MigrationDriftError{Version: f.Index, Name: f.Name, Want: a.Checksum, Got: f.Checksum}
			}
			log.Info("migration_skip_already_applied", "version", f.Index, "name", f.Name)
			continue
		}

		log.Info("migration_apply_begin", "version", f.Index, "name", f.Name)
		body, rerr := os.ReadFile(f.Path)
		if rerr != nil {
			return fmt.Errorf("read %s: %w", f.Path, rerr)
		}

		// Run the migration in a transaction so a partial failure
		// does not leave the schema in an unknown state.
		tx, terr := p.P.Begin(ctx)
		if terr != nil {
			return fmt.Errorf("begin tx: %w", terr)
		}
		if _, err := tx.Exec(ctx, string(body)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply %s: %w", f.Name, err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO schema_migrations(version,name,checksum) VALUES ($1,$2,$3)`,
			f.Index, f.Name, f.Checksum,
		); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record %s: %w", f.Name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit %s: %w", f.Name, err)
		}
		log.Info("migration_apply_done", "version", f.Index, "name", f.Name)
	}

	return nil
}

// Errors returned by Migrate.
var (
	ErrNoMigrations = errors.New("no migrations found")
)
