package retention

import (
	"context"
	"fmt"
	"time"
)

// Phase 10F-A retention service entry point.
//
// The service is intentionally thin. It does ONE thing per call to
// Cleanup: walk the configured tables, decide per-table what to do
// based on the Mode + cutoff, and emit a single audit event per
// table summarising what happened. Any exception (DB error, refused
// table, invalid config) emits a clearly-named failure audit event
// and returns a non-nil error to the caller.

// Store is the DB seam Cleanup uses. Production wiring (Phase 10F-B
// or later) implements this against pgxpool; tests inject a fake
// that drives every branch deterministically.
//
// All methods are bounded by the cutoff timestamp; the service NEVER
// asks the store for row contents.
type Store interface {
	// CountCandidates returns the number of rows in the given table
	// strictly older than the cutoff. Implementations MUST issue
	// `WHERE <ts> < $1` against the canonical timestamp column for
	// that table (audit_logs.at, network_action_runs.created_at).
	CountCandidates(ctx context.Context, table Table, cutoff time.Time) (int64, error)
	// DeleteOlderThan deletes rows older than the cutoff and
	// returns the affected row count. Implementations MUST refuse
	// (return ErrTableProtected) any table for which the deployment
	// has revoked DELETE on the application role; the service then
	// records the table as count-only.
	DeleteOlderThan(ctx context.Context, table Table, cutoff time.Time) (int64, error)
}

// AuditEmitter receives one entry per table per Cleanup call. The
// retention package never includes row contents in metadata; the
// per-table summary contains only counts + the cutoff timestamp +
// the table name.
type AuditEmitter interface {
	// Emit MUST NOT block on IO that the operator considers part of
	// the retention transaction. Implementations typically write to
	// the audit_logs table (which is itself a retention target —
	// see ModeExecute notes) but the service does not depend on
	// that being the only sink.
	Emit(ctx context.Context, action string, outcome string, metadata map[string]any)
}

// Logger is the structured-log seam. Like the action layer's logger,
// the production wiring is internal/logger.Logger and tests use a
// silent stub.
type Logger interface {
	Info(msg string, attrs ...any)
	Warn(msg string, attrs ...any)
	Error(msg string, attrs ...any)
}

// ErrTableProtected is returned by Store.DeleteOlderThan for tables
// the deployment has revoked DELETE on. The service catches this
// and records the table as count-only without raising the run as
// an error.
var ErrTableProtected = fmt.Errorf("retention: table is protected from DELETE by DB role policy")

// Audit event names. Stable strings for downstream alerting.
const (
	AuditActionRetentionDisabled       = "retention.disabled"
	AuditActionRetentionDryRunCounted  = "retention.dry_run_counted"
	AuditActionRetentionDeleted        = "retention.deleted"
	AuditActionRetentionTableProtected = "retention.table_protected"
	AuditActionRetentionFailed         = "retention.failed"
)

// Service is the cleanup runner. Thread-safe; a single instance can
// service multiple concurrent invocations because every method is
// stateless beyond the injected dependencies.
type Service struct {
	cfg   Config
	store Store
	audit AuditEmitter
	log   Logger
	now   func() time.Time // injection seam for deterministic tests
}

// New wires a Service with the supplied dependencies. The Service
// validates the config eagerly so the operator gets a clear error
// before any DB call is issued.
func New(cfg Config, store Store, audit AuditEmitter, log Logger) (*Service, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Service{
		cfg:   cfg,
		store: store,
		audit: audit,
		log:   log,
		now:   func() time.Time { return time.Now().UTC() },
	}, nil
}

// SetClock injects a deterministic time source. Tests use this; the
// production constructor leaves time.Now in place.
func (s *Service) SetClock(now func() time.Time) {
	if now != nil {
		s.now = now
	}
}

// TableSummary is the per-table outcome of a Cleanup call. The
// service returns the slice in the order Tables was configured so
// callers (and tests) can read it deterministically.
type TableSummary struct {
	Table      Table
	Cutoff     time.Time
	Mode       Mode
	Candidates int64
	Deleted    int64
	// Protected is true when the store refused DELETE because the
	// table is append-only (audit_logs default).
	Protected bool
	// Err is non-nil when the per-table action failed; the service
	// continues with the remaining tables and surfaces the error
	// in the aggregate Cleanup return.
	Err error
}

// Cleanup walks every configured table, counts candidates, and (in
// ModeExecute) deletes them. It NEVER reads row contents.
//
// Invocation contract:
//   - ModeDisabled: emits a single retention.disabled audit row +
//     returns an empty summary. No DB calls.
//   - ModeDryRun: emits one retention.dry_run_counted row per
//     configured table. NO DELETE issued.
//   - ModeExecute: emits one retention.deleted (or
//     retention.table_protected) row per configured table. The
//     service refuses DELETE on tables flagged Protected by the
//     store and records that as protected, NOT as failure.
//   - Any DB error on a single table emits retention.failed for
//     that table and continues with the next one. The aggregate
//     return reports the first error.
func (s *Service) Cleanup(ctx context.Context) ([]TableSummary, error) {
	if s.cfg.Mode == "" || s.cfg.Mode == ModeDisabled {
		s.audit.Emit(ctx, AuditActionRetentionDisabled, "success", map[string]any{
			"mode":   string(ModeDisabled),
			"reason": "retention is disabled by configuration",
		})
		return nil, nil
	}
	now := s.now()
	summaries := make([]TableSummary, 0, len(s.cfg.Tables))
	var firstErr error
	for _, t := range s.cfg.Tables {
		summary := TableSummary{Table: t.Table, Mode: s.cfg.Mode}
		cutoff := s.cfg.CutoffFor(now, t.Table)
		if cutoff.IsZero() {
			// Table is opted out (RetentionDays <= 0). Skip silently;
			// the audit row would add noise without information.
			continue
		}
		summary.Cutoff = cutoff

		count, err := s.store.CountCandidates(ctx, t.Table, cutoff)
		if err != nil {
			summary.Err = err
			s.audit.Emit(ctx, AuditActionRetentionFailed, "failure", map[string]any{
				"table":  string(t.Table),
				"phase":  "count",
				"cutoff": cutoff.Format(time.RFC3339),
				"actor":  s.cfg.Actor,
				"error":  sanitiseStoreError(err),
			})
			s.log.Error("retention count failed",
				"table", string(t.Table), "actor", s.cfg.Actor)
			if firstErr == nil {
				firstErr = err
			}
			summaries = append(summaries, summary)
			continue
		}
		summary.Candidates = count

		if s.cfg.Mode == ModeDryRun {
			s.audit.Emit(ctx, AuditActionRetentionDryRunCounted, "success", map[string]any{
				"table":      string(t.Table),
				"candidates": count,
				"cutoff":     cutoff.Format(time.RFC3339),
				"actor":      s.cfg.Actor,
			})
			summaries = append(summaries, summary)
			continue
		}

		// ModeExecute path. Issue the bounded DELETE; the store
		// returns ErrTableProtected for tables we must not touch.
		deleted, delErr := s.store.DeleteOlderThan(ctx, t.Table, cutoff)
		switch {
		case delErr == nil:
			summary.Deleted = deleted
			s.audit.Emit(ctx, AuditActionRetentionDeleted, "success", map[string]any{
				"table":      string(t.Table),
				"deleted":    deleted,
				"candidates": count,
				"cutoff":     cutoff.Format(time.RFC3339),
				"actor":      s.cfg.Actor,
			})
		case isProtected(delErr):
			summary.Protected = true
			s.audit.Emit(ctx, AuditActionRetentionTableProtected, "blocked", map[string]any{
				"table":      string(t.Table),
				"candidates": count,
				"cutoff":     cutoff.Format(time.RFC3339),
				"actor":      s.cfg.Actor,
				"reason":     "table is append-only by DB role policy; retention requires a separately-reviewed migration",
			})
		default:
			summary.Err = delErr
			s.audit.Emit(ctx, AuditActionRetentionFailed, "failure", map[string]any{
				"table":  string(t.Table),
				"phase":  "delete",
				"cutoff": cutoff.Format(time.RFC3339),
				"actor":  s.cfg.Actor,
				"error":  sanitiseStoreError(delErr),
			})
			s.log.Error("retention delete failed",
				"table", string(t.Table), "actor", s.cfg.Actor)
			if firstErr == nil {
				firstErr = delErr
			}
		}
		summaries = append(summaries, summary)
	}
	return summaries, firstErr
}

// sanitiseStoreError keeps the error class short and strips secret-
// shaped substrings the driver might have included (DSN credentials,
// password=, token=, etc.). The retention service NEVER allows raw
// driver output into the audit row; this is the single boundary
// where every store error gets redacted.
func sanitiseStoreError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	// Drop anything past the first newline; pgx error messages can
	// embed query text after the first line.
	if idx := indexByte(msg, '\n'); idx >= 0 {
		msg = msg[:idx]
	}
	msg = redactDSN(msg)
	msg = redactKeyValueSecrets(msg)
	const maxLen = 160
	if len(msg) > maxLen {
		msg = msg[:maxLen] + "…"
	}
	return msg
}

// redactDSN replaces user:pass embedded in a URL-style DSN with
// [redacted]. Pattern: protocol://user:pass@host → protocol://[redacted]@host.
// We do not import a regex package: a small hand scan keeps the
// code dependency-light and easier to audit.
func redactDSN(s string) string {
	for i := 0; i+3 < len(s); i++ {
		if s[i] != ':' || s[i+1] != '/' || s[i+2] != '/' {
			continue
		}
		// Found "://"; scan for the next '@' before whitespace or
		// end of string to detect a credentials block.
		j := i + 3
		atIdx := -1
		for j < len(s) {
			c := s[j]
			if c == '@' {
				atIdx = j
				break
			}
			if c == ' ' || c == '\t' || c == ')' || c == '"' || c == '\'' {
				break
			}
			j++
		}
		if atIdx > i+3 {
			s = s[:i+3] + "[redacted]" + s[atIdx:]
		}
		// Continue past the redaction; one DSN per error is the
		// realistic case but the loop tolerates more.
	}
	return s
}

// redactKeyValueSecrets replaces obvious secret-shaped key=value
// substrings with key=[redacted]. Recognised keys: password, passwd,
// secret, token, key, bearer, auth, sslcert, sslkey.
func redactKeyValueSecrets(s string) string {
	keys := []string{"password=", "passwd=", "secret=", "token=", "bearer=", "auth=", "sslcert=", "sslkey="}
	low := toLowerASCII(s)
	for _, k := range keys {
		idx := 0
		for {
			at := indexAt(low, k, idx)
			if at < 0 {
				break
			}
			end := at + len(k)
			// Find end of value: whitespace, ')', or end.
			j := end
			for j < len(s) {
				c := s[j]
				if c == ' ' || c == '\t' || c == ')' || c == '"' || c == '\'' || c == ',' {
					break
				}
				j++
			}
			s = s[:end] + "[redacted]" + s[j:]
			low = low[:end] + "[redacted]" + low[j:]
			idx = end + len("[redacted]")
		}
	}
	return s
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

func indexAt(s, sub string, from int) int {
	if from > len(s) {
		return -1
	}
	for i := from; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func toLowerASCII(s string) string {
	b := []byte(s)
	for i := 0; i < len(b); i++ {
		c := b[i]
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

// isProtected returns true when err carries (or wraps) the
// ErrTableProtected sentinel.
func isProtected(err error) bool {
	for e := err; e != nil; {
		if e == ErrTableProtected {
			return true
		}
		type wrapper interface{ Unwrap() error }
		w, ok := e.(wrapper)
		if !ok {
			break
		}
		e = w.Unwrap()
	}
	return false
}
