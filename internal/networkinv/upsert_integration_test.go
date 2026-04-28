package networkinv

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wisp-ops-center/wisp-ops-center/internal/dude"
)

// Phase 8 hotfix v8.4.0 — DB-bound dedupe/upsert + run finalize tests.
//
// These tests require a live PostgreSQL with migration 000008 applied.
// Set WISP_TEST_DATABASE_URL to a throwaway DB; the suite TRUNCATEs
// network_devices, discovery_runs, device_category_evidence, audit_logs
// before each subtest. CI skips when the env var is unset.

func openTestPool(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()
	dsn := os.Getenv("WISP_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("WISP_TEST_DATABASE_URL not set; skipping DB integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping: %v", err)
	}
	cleanup := func() { pool.Close() }
	return pool, cleanup
}

func resetPhase8Tables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := pool.Exec(ctx, `TRUNCATE device_category_evidence, network_devices, discovery_runs RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

func mkDevice(name, ip, mac string, cat dude.Category, conf int) dude.DiscoveredDevice {
	return dude.DiscoveredDevice{
		Source: "mikrotik_dude",
		Name:   name,
		IP:     ip,
		MAC:    mac,
		Classification: dude.Classification{
			Category:   cat,
			Confidence: conf,
		},
		LastSeen: time.Now().UTC(),
		Raw:      map[string]string{"name": name},
	}
}

func TestUpsert_NameOnlyIdempotent(t *testing.T) {
	pool, cleanup := openTestPool(t)
	defer cleanup()
	resetPhase8Tables(t, pool)
	r := NewRepository(pool)
	ctx := context.Background()

	devs := []dude.DiscoveredDevice{
		mkDevice("400", "", "", dude.CategoryUnknown, 0),
		mkDevice("300-OREN", "", "", dude.CategoryUnknown, 0),
	}

	ids1, stats1, err := r.UpsertDevices(ctx, "", devs)
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if stats1.Inserted != 2 || stats1.Updated != 0 || stats1.Skipped != 0 {
		t.Errorf("first stats wrong: %+v", stats1)
	}
	for i, id := range ids1 {
		if id == "" {
			t.Errorf("id[%d] empty", i)
		}
	}

	ids2, stats2, err := r.UpsertDevices(ctx, "", devs)
	if err != nil {
		t.Fatalf("second upsert (idempotency): %v", err)
	}
	if stats2.Inserted != 0 || stats2.Updated != 2 || stats2.Skipped != 0 {
		t.Errorf("second stats wrong (expected all updates): %+v", stats2)
	}
	for i := range ids2 {
		if ids2[i] != ids1[i] {
			t.Errorf("device %d id changed across runs: %s -> %s", i, ids1[i], ids2[i])
		}
	}

	// Row count must stay 2.
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM network_devices`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 2 {
		t.Errorf("row count after two runs = %d, want 2", n)
	}
}

func TestUpsert_MACDedupe(t *testing.T) {
	pool, cleanup := openTestPool(t)
	defer cleanup()
	resetPhase8Tables(t, pool)
	r := NewRepository(pool)
	ctx := context.Background()

	mac := "AA:BB:CC:11:22:33"
	first := []dude.DiscoveredDevice{mkDevice("router-old", "10.0.0.10", mac, dude.CategoryRouter, 70)}
	second := []dude.DiscoveredDevice{mkDevice("router-renamed", "10.0.0.20", mac, dude.CategoryRouter, 90)}

	if _, _, err := r.UpsertDevices(ctx, "", first); err != nil {
		t.Fatalf("first: %v", err)
	}
	ids, stats, err := r.UpsertDevices(ctx, "", second)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if stats.Updated != 1 || stats.Inserted != 0 {
		t.Errorf("expected 1 update, 0 insert; got %+v", stats)
	}

	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM network_devices WHERE mac=$1`, mac).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("row count for mac = %d, want 1", n)
	}
	// Most recent metadata should win.
	var name string
	if err := pool.QueryRow(ctx, `SELECT name FROM network_devices WHERE mac=$1`, mac).Scan(&name); err != nil {
		t.Fatalf("scan name: %v", err)
	}
	if name != "router-renamed" {
		t.Errorf("update did not refresh name: got %q", name)
	}
	_ = ids
}

func TestUpsert_HostNameDedupe(t *testing.T) {
	pool, cleanup := openTestPool(t)
	defer cleanup()
	resetPhase8Tables(t, pool)
	r := NewRepository(pool)
	ctx := context.Background()

	devs := []dude.DiscoveredDevice{mkDevice("backbone-1", "10.0.5.1", "", dude.CategoryRouter, 60)}

	if _, _, err := r.UpsertDevices(ctx, "", devs); err != nil {
		t.Fatalf("first: %v", err)
	}
	_, stats, err := r.UpsertDevices(ctx, "", devs)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if stats.Updated != 1 || stats.Inserted != 0 {
		t.Errorf("expected 1 update, got %+v", stats)
	}
	var n int
	pool.QueryRow(ctx, `SELECT count(*) FROM network_devices`).Scan(&n)
	if n != 1 {
		t.Errorf("row count = %d, want 1", n)
	}
}

func TestUpsert_SkipsUnidentifiableDevice(t *testing.T) {
	pool, cleanup := openTestPool(t)
	defer cleanup()
	resetPhase8Tables(t, pool)
	r := NewRepository(pool)
	ctx := context.Background()

	// No mac, no host, no name -> skipped (no stable identity).
	devs := []dude.DiscoveredDevice{
		{Source: "mikrotik_dude", LastSeen: time.Now().UTC()},
		mkDevice("real", "", "", dude.CategoryUnknown, 0),
	}
	ids, stats, err := r.UpsertDevices(ctx, "", devs)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if stats.Skipped != 1 {
		t.Errorf("expected 1 skipped, got %+v", stats)
	}
	if stats.Inserted != 1 {
		t.Errorf("expected 1 inserted, got %+v", stats)
	}
	if ids[0] != "" {
		t.Errorf("skipped device should have empty id, got %q", ids[0])
	}
	if ids[1] == "" {
		t.Errorf("real device id should be populated")
	}
	var n int
	pool.QueryRow(ctx, `SELECT count(*) FROM network_devices`).Scan(&n)
	if n != 1 {
		t.Errorf("row count = %d, want 1", n)
	}
}

func TestFinalizeRun_UsesPersistedDeviceCounts(t *testing.T) {
	pool, cleanup := openTestPool(t)
	defer cleanup()
	resetPhase8Tables(t, pool)
	r := NewRepository(pool)
	ctx := context.Background()

	run, err := r.CreateRun(ctx, "test-corr", "system")
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	devs := []dude.DiscoveredDevice{
		mkDevice("ap-1", "10.0.0.10", "AA:BB:CC:00:00:01", dude.CategoryAP, 80),
		mkDevice("cpe-1", "10.0.0.20", "AA:BB:CC:00:00:02", dude.CategoryCPE, 30),
		mkDevice("u-1", "", "", dude.CategoryUnknown, 0),
	}
	if _, _, err := r.UpsertDevices(ctx, run.ID, devs); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	// Tally happens at orchestrator level (named return + defer fix).
	// Here we mimic what runDudeDiscoveryAsync passes to FinalizeRun.
	var stats dude.DiscoveryStats
	stats.Tally(devs)

	if err := r.FinalizeRun(ctx, run.ID, dude.RunResult{
		Success: true,
		Devices: devs,
		Stats:   stats,
	}); err != nil {
		t.Fatalf("FinalizeRun: %v", err)
	}

	row := pool.QueryRow(ctx, `SELECT status, device_count, ap_count, cpe_count, unknown_count, low_conf_count
	                            FROM discovery_runs WHERE id=$1`, run.ID)
	var status string
	var dc, ap, cpe, unk, low int
	if err := row.Scan(&status, &dc, &ap, &cpe, &unk, &low); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if status != "succeeded" {
		t.Errorf("status = %q, want succeeded", status)
	}
	if dc != 3 {
		t.Errorf("device_count = %d, want 3", dc)
	}
	if ap != 1 || cpe != 1 || unk != 1 {
		t.Errorf("category counts wrong: ap=%d cpe=%d unknown=%d", ap, cpe, unk)
	}
	if low != 2 {
		t.Errorf("low_conf_count = %d, want 2 (cpe<50, unknown<50)", low)
	}
}

func TestFinalizeRun_CategoryDistribution(t *testing.T) {
	pool, cleanup := openTestPool(t)
	defer cleanup()
	resetPhase8Tables(t, pool)
	r := NewRepository(pool)
	ctx := context.Background()

	devs := []dude.DiscoveredDevice{
		mkDevice("ap1", "10.0.0.1", "AA:00:00:00:00:01", dude.CategoryAP, 80),
		mkDevice("ap2", "10.0.0.2", "AA:00:00:00:00:02", dude.CategoryAP, 80),
		mkDevice("br1", "10.0.0.3", "AA:00:00:00:00:03", dude.CategoryBackhaul, 70),
		mkDevice("rt1", "10.0.0.4", "AA:00:00:00:00:04", dude.CategoryRouter, 75),
		mkDevice("sw1", "10.0.0.5", "AA:00:00:00:00:05", dude.CategorySwitch, 65),
		mkDevice("bg1", "10.0.0.6", "AA:00:00:00:00:06", dude.CategoryBridge, 60),
	}

	run, _ := r.CreateRun(ctx, "test-cat", "system")
	if _, _, err := r.UpsertDevices(ctx, run.ID, devs); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	var stats dude.DiscoveryStats
	stats.Tally(devs)
	if err := r.FinalizeRun(ctx, run.ID, dude.RunResult{Success: true, Devices: devs, Stats: stats}); err != nil {
		t.Fatalf("finalize: %v", err)
	}

	row := pool.QueryRow(ctx, `SELECT ap_count, cpe_count, bridge_count, link_count, router_count, switch_count, unknown_count
	                            FROM discovery_runs WHERE id=$1`, run.ID)
	var ap, cpe, br, link, rt, sw, unk int
	if err := row.Scan(&ap, &cpe, &br, &link, &rt, &sw, &unk); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if ap != 2 || cpe != 0 || br != 1 || link != 1 || rt != 1 || sw != 1 || unk != 0 {
		t.Errorf("counts: ap=%d cpe=%d bridge=%d link=%d router=%d switch=%d unknown=%d",
			ap, cpe, br, link, rt, sw, unk)
	}
}

func TestFinalizeRun_LowConfidenceAndUnknownCounts(t *testing.T) {
	pool, cleanup := openTestPool(t)
	defer cleanup()
	resetPhase8Tables(t, pool)
	r := NewRepository(pool)
	ctx := context.Background()

	devs := []dude.DiscoveredDevice{
		mkDevice("u1", "", "", dude.CategoryUnknown, 0),
		mkDevice("u2", "", "", dude.CategoryUnknown, 10),
		mkDevice("u3", "", "", dude.CategoryUnknown, 49),
		mkDevice("ap-good", "10.0.0.1", "AA:00:00:00:00:99", dude.CategoryAP, 80),
	}
	run, _ := r.CreateRun(ctx, "test-lowconf", "system")
	if _, _, err := r.UpsertDevices(ctx, run.ID, devs); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	var stats dude.DiscoveryStats
	stats.Tally(devs)
	if err := r.FinalizeRun(ctx, run.ID, dude.RunResult{Success: true, Devices: devs, Stats: stats}); err != nil {
		t.Fatalf("finalize: %v", err)
	}
	row := pool.QueryRow(ctx, `SELECT unknown_count, low_conf_count FROM discovery_runs WHERE id=$1`, run.ID)
	var unk, low int
	if err := row.Scan(&unk, &low); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if unk != 3 {
		t.Errorf("unknown_count = %d, want 3", unk)
	}
	if low != 3 {
		t.Errorf("low_conf_count = %d, want 3 (3 < 50, 1 ap >= 50)", low)
	}
}

func TestFinalizeRun_PersistFailedMarksFailedOrPartial(t *testing.T) {
	pool, cleanup := openTestPool(t)
	defer cleanup()
	resetPhase8Tables(t, pool)
	r := NewRepository(pool)
	ctx := context.Background()

	run, _ := r.CreateRun(ctx, "test-persistfail", "system")
	devs := []dude.DiscoveredDevice{
		mkDevice("ap1", "10.0.0.1", "AA:11:11:11:11:11", dude.CategoryAP, 80),
	}
	var stats dude.DiscoveryStats
	stats.Tally(devs)
	if err := r.FinalizeRun(ctx, run.ID, dude.RunResult{
		Success:   false,
		ErrorCode: "persist_failed",
		Error:     "persist_failed: synthetic",
		Devices:   devs,
		Stats:     stats,
	}); err != nil {
		t.Fatalf("finalize: %v", err)
	}
	var status, errCode string
	if err := pool.QueryRow(ctx, `SELECT status, COALESCE(error_code,'') FROM discovery_runs WHERE id=$1`, run.ID).Scan(&status, &errCode); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if status == "succeeded" {
		t.Fatalf("invariant violated: persist_failed yet status=succeeded")
	}
	if errCode != "persist_failed" {
		t.Errorf("error_code = %q, want persist_failed", errCode)
	}
}
