# Destructive-Action Safety Alerting (Phase 10F-A)

This document is the operator-facing companion to
`infra/prometheus/destructive_alerts.rules.yml`.

WispOps does NOT yet expose a Prometheus `/metrics` endpoint that
publishes the counters / gauges referenced by the alert rules; that
metrics surface lands as a follow-up engineering PR (Phase 10F-B
or later). Until then, **every alert in the rules file has an
audit/log query fallback** the oncall operator can run by hand
against Postgres.

The alert rules file is therefore safe to load into Prometheus
today — referenced metric names resolve to empty results so no
alert fires, and the rules become live the moment the metrics
ship.

## Fallback queries

Run these as the application role against the production database
(`wispops_app` is sufficient for SELECT on `audit_logs` and
`network_action_runs`).

### A. WispDestructiveSucceededUnexpected (critical)

Did any destructive Kind succeed in the last 15 minutes?

```sql
SELECT id, action_type, actor, started_at, finished_at, command_count
FROM network_action_runs
WHERE action_type IN ('frequency_correction', 'maintenance_window')
  AND status = 'succeeded'
  AND created_at >= now() - INTERVAL '15 minutes';
```

**Expected: empty result.** Any row is an incident.

### B. WispDestructiveAttemptedWhileDisabled (warning)

How many `live_start_blocked` events fired in the last 15 minutes?

```sql
SELECT actor, COUNT(*) AS blocked_attempts
FROM audit_logs
WHERE action = 'network_action.live_start_blocked'
  AND at >= now() - INTERVAL '15 minutes'
GROUP BY actor
ORDER BY blocked_attempts DESC;
```

A small number is normal (operator drilling). A spike >5 per
single actor is worth investigating.

### C. WispDestructiveVerifyFailedRollbackRecovered (warning)

Did a verify miss + rollback recovery happen in the last 30 minutes?

```sql
SELECT subject AS run_id,
       metadata->>'verified_freq' AS verified_freq,
       metadata->>'target_freq'   AS target_freq,
       metadata->>'verify_error'  AS verify_error,
       at
FROM audit_logs
WHERE action = 'network_action.execute_verification_failed'
  AND at >= now() - INTERVAL '30 minutes';
```

### D. WispDestructiveRollbackFailed (critical, page)

Did the rollback path fail to restore the snapshot?

```sql
SELECT id, action_type, error_code, error_message, started_at
FROM network_action_runs
WHERE error_code = 'rollback_failed'
  AND created_at >= now() - INTERVAL '5 minutes';
```

**Expected: empty result.** Any row means a real device may be
in an unknown state and operator MUST reach the device manually.

### E. WispDestructiveExecuteNotImplementedSpike (warning)

Are clients repeatedly hitting Kinds whose Execute is still a stub?

```sql
SELECT actor, action_type, COUNT(*) AS attempts
FROM network_action_runs
WHERE error_code = 'action_not_implemented'
  AND created_at >= now() - INTERVAL '15 minutes'
GROUP BY actor, action_type
ORDER BY attempts DESC;
```

### F. WispProviderToggleEnabledOutsideWindow (warning)

Is the destructive toggle enabled with no active maintenance window?

```sql
WITH last_toggle AS (
  SELECT enabled, actor, reason, flipped_at
  FROM network_action_toggle_flips
  ORDER BY flipped_at DESC LIMIT 1
),
active_windows AS (
  SELECT COUNT(*) AS n
  FROM network_action_maintenance_windows
  WHERE start_at <= now() AND end_at > now()
    AND disabled_at IS NULL
)
SELECT
  (SELECT enabled FROM last_toggle)         AS toggle_enabled,
  (SELECT actor   FROM last_toggle)         AS toggle_actor,
  (SELECT reason  FROM last_toggle)         AS toggle_reason,
  (SELECT n       FROM active_windows)      AS active_windows;
```

If `toggle_enabled = true` AND `active_windows = 0`, the operator
likely forgot to flip the toggle off. The pre-gate denies destructive
calls without a window so safety holds, but the configuration is
suspicious.

### G. WispSecretLeakDetected (critical, page)

Does any audit row or run-row jsonb contain a secret-shaped substring?

```sql
-- Audit metadata
SELECT id, action, at, metadata
FROM audit_logs
WHERE metadata::text ~* '(password|passwd|secret|token|key|bearer)\s*[=:]'
  AND at >= now() - INTERVAL '1 hour';

-- Run row results
SELECT id, action_type, created_at
FROM network_action_runs
WHERE result::text ~* '(password|passwd|secret|token|key|bearer)\s*[=:]'
  AND created_at >= now() - INTERVAL '1 hour';
```

**Expected: empty result.** Any row means the sanitiser missed a
code path; ship a fix and rotate the leaked secret.

### H. WispRawMacLeakDetected (critical, page)

Does any audit row or run-row jsonb contain a raw 6-octet MAC?

```sql
SELECT id, action, at
FROM audit_logs
WHERE metadata::text ~ '[0-9a-fA-F]{2}(:[0-9a-fA-F]{2}){5}'
  AND at >= now() - INTERVAL '1 hour';
```

**Expected: empty result.** Any row means the MAC masking helper
missed a code path.

### I. WispRetentionDeletedLargeBatch (warning)

Did the retention service delete more rows in the last hour than the
configured horizon should have produced?

```sql
SELECT id, action, at, metadata
FROM audit_logs
WHERE action = 'retention.deleted'
  AND at >= now() - INTERVAL '1 hour'
  AND (metadata->>'deleted')::int > 100000;
```

If a row appears, confirm the operator-configured retention horizon
+ recent ingestion rate. A misconfigured horizon (days set to 0
where execution is enabled) can wipe an entire table.

## Promoting fallback queries to PromQL alerts

When Phase 10F-B (or later) lands the metrics surface:

1. Increment the matching counter at every code path that emits the
   audit event today.
2. Expose the counter via the `/metrics` endpoint.
3. Confirm the counter shows up in `up{job="wisp-ops-api"}=1`.
4. Reload Prometheus; the rules file already references the metric
   names so no further config change is needed.

## What this document does NOT cover

- Alertmanager routing / notifier configuration.
- Pager rotation and oncall handoff.
- Production secret rotation procedures (Phase 10F operator scope).
- KMS / Vault integration.

These belong in the operator runbook, not in the engineering
hardening surface.
