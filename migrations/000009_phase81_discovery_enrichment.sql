-- 000009_phase81_discovery_enrichment.sql
-- Faz 8.1: Discovery Enrichment + Inventory Classification Evidence.
--
-- Purpose:
--   Phase 8 left 892/893 devices in category=Unknown because the
--   primary /dude/device/print/detail source is name-heavy and rarely
--   reveals MAC, platform, identity or interface. Phase 8.1 adds
--   read-only enrichment sources (/ip/neighbor, /dude/probe,
--   /dude/service) and merges their signals into the same record.
--
--   This migration adds the columns the enrichment pipeline needs:
--
--     - platform           : RouterOS / Mimosa / airOS / etc.
--     - board              : RouterBoard model string when known
--     - interface_name     : upstream interface from neighbor
--     - evidence_summary   : single-line operator-facing summary
--     - enrichment_sources : ordered list of sources that contributed
--     - last_enriched_at   : timestamp of last successful enrichment
--
-- Idempotent (ADD COLUMN IF NOT EXISTS) and transactional. No DROP.

BEGIN;

ALTER TABLE network_devices
  ADD COLUMN IF NOT EXISTS platform           text,
  ADD COLUMN IF NOT EXISTS board              text,
  ADD COLUMN IF NOT EXISTS interface_name     text,
  ADD COLUMN IF NOT EXISTS evidence_summary   text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS enrichment_sources text[] NOT NULL DEFAULT '{}',
  ADD COLUMN IF NOT EXISTS last_enriched_at   timestamptz;

CREATE INDEX IF NOT EXISTS idx_netdev_has_mac
  ON network_devices ((mac IS NOT NULL));

CREATE INDEX IF NOT EXISTS idx_netdev_enriched
  ON network_devices (last_enriched_at DESC NULLS LAST);

CREATE INDEX IF NOT EXISTS idx_netdev_platform
  ON network_devices (platform)
  WHERE platform IS NOT NULL;

-- discovery_runs gains enrichment metadata so the runs API and the
-- audit metadata can report which sources were attempted, succeeded
-- or were skipped — without leaking any RouterOS output.
ALTER TABLE discovery_runs
  ADD COLUMN IF NOT EXISTS enrichment_sources_attempted text[] NOT NULL DEFAULT '{}',
  ADD COLUMN IF NOT EXISTS enrichment_sources_succeeded text[] NOT NULL DEFAULT '{}',
  ADD COLUMN IF NOT EXISTS enrichment_sources_skipped   text[] NOT NULL DEFAULT '{}',
  ADD COLUMN IF NOT EXISTS enrichment_duration_ms       bigint NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS with_mac_count               int    NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS with_host_count              int    NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS enriched_count               int    NOT NULL DEFAULT 0;

COMMIT;
