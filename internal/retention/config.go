// Package retention is the Phase 10F-A KVKK-safe retention foundation.
//
// What this package DOES:
//   - Counts candidate rows that would be deleted under a given
//     retention policy (audit_logs + network_action_runs).
//   - Deletes candidate rows ONLY when the operator has explicitly
//     enabled retention AND turned dry-run off AND configured a
//     positive retention horizon.
//   - Emits an audit row for every cleanup invocation describing
//     mode, table, candidate count, executed count, and the cutoff
//     timestamp — without leaking row content.
//
// What this package does NOT do:
//   - It does NOT install a cron / systemd unit. Phase 10F-A ships
//     the engineering primitive only; the operator-facing scheduler
//     (cron expression, run-as user, lock file) lands once the policy
//     is reviewed.
//   - It does NOT delete from audit_logs by default. Migration 000002
//     REVOKEd UPDATE/DELETE on audit_logs from wispops_app to
//     guarantee append-only semantics; retention against audit_logs
//     stays in count-only mode until a deliberate, separately-
//     reviewed migration grants a retention role and partitions the
//     table.
//   - It does NOT log row contents. Sanitisation is structural: the
//     service NEVER reads the actor / subject / metadata columns;
//     the only DB call is a COUNT(*) or DELETE … WHERE bounded by
//     the cutoff timestamp.
//   - It does NOT touch tables outside the explicit allowlist below.
//
// Default posture: every flag default-disabled, dry-run default
// true, retention horizons default zero (interpreted as "no
// retention configured"). Calling Cleanup() with the zero-value
// config is a no-op that emits a single "retention.disabled" audit
// row.
package retention

import (
	"errors"
	"strings"
	"time"
)

// Table is the explicit allowlist of tables retention is allowed to
// touch. Adding a Kind here is a deliberate, security-reviewed
// change (data-loss potential).
type Table string

const (
	// TableAuditLogs is COUNT-ONLY today. The service refuses to
	// issue a DELETE against this table even when Enabled=true and
	// DryRun=false; the operator MUST run a separately-reviewed
	// migration to bypass the wispops_app DELETE revoke.
	TableAuditLogs Table = "audit_logs"
	// TableNetworkActionRuns is the per-action run row table. The
	// app role has DELETE here; the service may delete when fully
	// enabled.
	TableNetworkActionRuns Table = "network_action_runs"
)

// Mode is the retention service mode. The two non-disabled modes
// MUST be set explicitly by the operator; default is ModeDisabled.
type Mode string

const (
	// ModeDisabled is the default. Cleanup() is a no-op except for
	// emitting a "retention.disabled" audit row.
	ModeDisabled Mode = "disabled"
	// ModeDryRun counts candidates per table and emits an audit row
	// per table without issuing any DELETE.
	ModeDryRun Mode = "dry_run"
	// ModeExecute counts candidates AND deletes them on tables that
	// permit it. Requires explicit positive retention horizons.
	ModeExecute Mode = "execute"
)

// TableConfig is the per-table retention horizon. RetentionDays must
// be > 0 for the service to consider candidate rows. A horizon of
// 0 is interpreted as "this table is opted out, even if Mode is
// not Disabled".
type TableConfig struct {
	// Table is the canonical name; MUST match one of the constants
	// declared above.
	Table Table
	// RetentionDays is the cutoff: rows older than (now() - days)
	// are candidates. Zero / negative → table opt-out.
	RetentionDays int
}

// Config aggregates the per-table policies + the global mode +
// the actor that authorised the run (audit metadata).
type Config struct {
	// Mode sets the global service behaviour. Default ModeDisabled.
	Mode Mode
	// Tables is the per-table policy list. Tables not present here
	// are opted out of retention regardless of Mode.
	Tables []TableConfig
	// Actor is recorded in the audit row for every cleanup
	// invocation. Required when Mode != ModeDisabled.
	Actor string
}

// Validate returns an error when the configuration would put the
// service into an unsafe or ambiguous state. Validation is
// intentionally strict: a typo in the table name MUST fail closed
// instead of silently no-op'ing.
func (c Config) Validate() error {
	if c.Mode == "" {
		c.Mode = ModeDisabled
	}
	switch c.Mode {
	case ModeDisabled, ModeDryRun, ModeExecute:
		// ok
	default:
		return errors.New("retention: unknown mode (allowed: disabled, dry_run, execute)")
	}
	if c.Mode != ModeDisabled && strings.TrimSpace(c.Actor) == "" {
		return errors.New("retention: actor required when mode != disabled")
	}
	seen := map[Table]struct{}{}
	for _, t := range c.Tables {
		switch t.Table {
		case TableAuditLogs, TableNetworkActionRuns:
			// ok
		default:
			return errors.New("retention: table not in allowlist: " + string(t.Table))
		}
		if _, dup := seen[t.Table]; dup {
			return errors.New("retention: table listed twice: " + string(t.Table))
		}
		seen[t.Table] = struct{}{}
		if t.RetentionDays < 0 {
			return errors.New("retention: retention_days must be >= 0 for " + string(t.Table))
		}
	}
	return nil
}

// CutoffFor returns the timestamp before which rows are candidates
// for the given table. Returns zero Time when the table is opted
// out (RetentionDays == 0) or not present in the config.
func (c Config) CutoffFor(now time.Time, table Table) time.Time {
	for _, t := range c.Tables {
		if t.Table == table && t.RetentionDays > 0 {
			return now.Add(-time.Duration(t.RetentionDays) * 24 * time.Hour)
		}
	}
	return time.Time{}
}
