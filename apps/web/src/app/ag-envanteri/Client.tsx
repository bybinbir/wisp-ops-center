"use client";
import { useEffect, useState } from "react";
import {
  api,
  ApiError,
  CATEGORY_LABELS,
  DiscoveryRun,
  DudeTestResult,
  NetworkCategory,
  NetworkDevice,
  NETWORK_CATEGORIES,
  NetworkInventorySummary,
} from "@/lib/api";
import { StatCard } from "@/components/StatCard";
import { Toolbar, Button } from "@/components/Toolbar";
import { Field, Select } from "@/components/Field";

type Filters = {
  category: NetworkCategory | "";
  status: string;
  unknownOnly: boolean;
  lowConf: boolean;
  hasMac: boolean;
  enriched: boolean;
};

const emptyFilters: Filters = {
  category: "",
  status: "",
  unknownOnly: false,
  lowConf: false,
  hasMac: false,
  enriched: false,
};

export function NetworkInventoryClient() {
  const [devices, setDevices] = useState<NetworkDevice[] | null>(null);
  const [summary, setSummary] = useState<NetworkInventorySummary | null>(null);
  const [runs, setRuns] = useState<DiscoveryRun[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [filters, setFilters] = useState<Filters>(emptyFilters);
  const [test, setTest] = useState<DudeTestResult | null>(null);
  const [testing, setTesting] = useState(false);
  const [running, setRunning] = useState(false);

  async function loadDevices(f: Filters) {
    const params = new URLSearchParams();
    if (f.category) params.set("category", f.category);
    if (f.status) params.set("status", f.status);
    if (f.lowConf) params.set("low_confidence", "1");
    if (f.unknownOnly) params.set("unknown", "1");
    if (f.hasMac) params.set("has_mac", "1");
    if (f.enriched) params.set("enriched", "1");
    const qs = params.toString();
    const path = qs ? `/api/v1/network/devices?${qs}` : "/api/v1/network/devices";
    return api.get<{ data: NetworkDevice[]; summary: NetworkInventorySummary }>(path);
  }

  async function reload() {
    setLoading(true);
    try {
      const [devRes, runRes] = await Promise.all([
        loadDevices(filters),
        api.get<{ data: DiscoveryRun[] }>("/api/v1/network/discovery/runs").catch(() => ({ data: [] })),
      ]);
      setDevices(devRes.data ?? []);
      setSummary(devRes.summary ?? null);
      setRuns(runRes.data ?? []);
      setError(null);
    } catch (e) {
      const msg = e instanceof ApiError
        ? e.status === 503
          ? "Veritabanı bağlı değil. WISP_DATABASE_URL ayarlayıp migration çalıştırın."
          : e.message
        : e instanceof Error
        ? e.message
        : "Bilinmeyen hata";
      setError(msg);
      setDevices([]);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    reload();
  }, [
    filters.category,
    filters.status,
    filters.lowConf,
    filters.unknownOnly,
    filters.hasMac,
    filters.enriched,
  ]);

  async function runTest() {
    setTesting(true);
    setTest(null);
    try {
      const res = await api.post<DudeTestResult>(
        "/api/v1/network/discovery/mikrotik-dude/test-connection"
      );
      setTest(res);
    } catch (e) {
      if (e instanceof ApiError) {
        setTest({
          reachable: false,
          duration_ms: 0,
          host: "",
          started_at: new Date().toISOString(),
          error: e.message,
          error_code: String(e.status),
        });
      }
    } finally {
      setTesting(false);
    }
  }

  async function runDiscovery() {
    setRunning(true);
    try {
      await api.post<{ run_id: string; status: string }>(
        "/api/v1/network/discovery/mikrotik-dude/run"
      );
      let attempts = 0;
      const tick = setInterval(async () => {
        attempts++;
        await reload();
        if (attempts >= 15) clearInterval(tick);
        const newest = runs[0];
        if (newest && newest.status !== "running") clearInterval(tick);
      }, 2000);
    } catch (e) {
      if (e instanceof ApiError) {
        setError(e.message);
      }
    } finally {
      setRunning(false);
    }
  }

  const lastRun = runs[0];
  const lastError = lastRun?.status === "failed" || lastRun?.status === "partial"
    ? lastRun.error_message
    : "";

  return (
    <div>
      <div className="grid grid-4" style={{ marginBottom: 16 }}>
        <StatCard title="Toplam Cihaz" value={summary?.total ?? 0} />
        <StatCard title="AP" value={summary?.ap ?? 0} />
        <StatCard title="Backhaul / Link" value={summary?.link ?? 0} />
        <StatCard title="Bridge" value={summary?.bridge ?? 0} />
      </div>
      <div className="grid grid-4" style={{ marginBottom: 16 }}>
        <StatCard title="CPE / Müşteri" value={summary?.cpe ?? 0} />
        <StatCard title="Router" value={summary?.router ?? 0} />
        <StatCard title="Switch" value={summary?.switch ?? 0} />
        <StatCard title="Bilinmeyen" value={summary?.unknown ?? 0} />
      </div>
      <div className="grid grid-4" style={{ marginBottom: 16 }}>
        <StatCard title="MAC kazandı" value={summary?.with_mac ?? 0} />
        <StatCard title="Host kazandı" value={summary?.with_host ?? 0} />
        <StatCard title="Enriched" value={summary?.enriched ?? 0} />
        <StatCard title="Düşük Confidence" value={summary?.low_confidence ?? 0} />
      </div>

      <Toolbar>
        <Button onClick={runTest} disabled={testing}>
          {testing ? "Test ediliyor..." : "Bağlantıyı Test Et"}
        </Button>
        <Button onClick={runDiscovery} disabled={running} variant="primary">
          {running ? "Discovery başlatılıyor..." : "Discovery Çalıştır"}
        </Button>
        <span style={{ marginLeft: "auto", color: "#888", fontSize: 12 }}>
          Son discovery: {lastRun ? new Date(lastRun.started_at).toLocaleString("tr-TR") : "—"}
          {lastRun?.status ? ` · ${lastRun.status}` : ""}
        </span>
      </Toolbar>

      {test && (
        <div style={{
          margin: "12px 0",
          padding: 12,
          background: test.reachable ? "#0e3" : "#a22",
          color: "#fff",
          borderRadius: 6,
        }}>
          {test.reachable
            ? `Erişim OK · ${test.host} · identity=${test.identity ?? "-"} · ${test.duration_ms}ms`
            : `Erişilemiyor · ${test.host} · ${test.error_code ?? "?"} · ${test.error ?? ""}`}
        </div>
      )}

      {lastRun && (
        <EnrichmentCard run={lastRun} />
      )}

      {lastError && (
        <div style={{ margin: "12px 0", padding: 8, background: "#400", color: "#fbb", borderRadius: 6 }}>
          Son discovery hatası: {lastError}
        </div>
      )}

      <Toolbar>
        <Field label="Kategori">
          <Select
            value={filters.category}
            onChange={(e) =>
              setFilters((f) => ({ ...f, category: e.target.value as NetworkCategory | "" }))
            }
          >
            <option value="">Hepsi</option>
            {NETWORK_CATEGORIES.map((c) => (
              <option key={c} value={c}>{CATEGORY_LABELS[c]}</option>
            ))}
          </Select>
        </Field>
        <Field label="Durum">
          <Select
            value={filters.status}
            onChange={(e) => setFilters((f) => ({ ...f, status: e.target.value }))}
          >
            <option value="">Hepsi</option>
            <option value="up">Up</option>
            <option value="down">Down</option>
            <option value="partial">Partial</option>
            <option value="unknown">Unknown</option>
          </Select>
        </Field>
        <label style={{ display: "flex", alignItems: "center", gap: 6 }}>
          <input
            type="checkbox"
            checked={filters.lowConf}
            onChange={(e) => setFilters((f) => ({ ...f, lowConf: e.target.checked }))}
          />
          Düşük confidence
        </label>
        <label style={{ display: "flex", alignItems: "center", gap: 6 }}>
          <input
            type="checkbox"
            checked={filters.unknownOnly}
            onChange={(e) => setFilters((f) => ({ ...f, unknownOnly: e.target.checked }))}
          />
          Sadece bilinmeyen
        </label>
        <label style={{ display: "flex", alignItems: "center", gap: 6 }}>
          <input
            type="checkbox"
            checked={filters.hasMac}
            onChange={(e) => setFilters((f) => ({ ...f, hasMac: e.target.checked }))}
          />
          MAC var
        </label>
        <label style={{ display: "flex", alignItems: "center", gap: 6 }}>
          <input
            type="checkbox"
            checked={filters.enriched}
            onChange={(e) => setFilters((f) => ({ ...f, enriched: e.target.checked }))}
          />
          Enriched
        </label>
        {(filters.category || filters.status || filters.lowConf || filters.unknownOnly ||
          filters.hasMac || filters.enriched) && (
          <Button onClick={() => setFilters(emptyFilters)}>Filtreleri Temizle</Button>
        )}
      </Toolbar>

      {error && (
        <div style={{ padding: 12, background: "#400", color: "#fbb", borderRadius: 6, margin: "12px 0" }}>
          {error}
        </div>
      )}

      {loading ? (
        <div style={{ padding: 24, color: "#888" }}>Yükleniyor…</div>
      ) : (devices ?? []).length === 0 ? (
        <div style={{ padding: 24, color: "#888" }}>
          Cihaz bulunamadı. "Discovery Çalıştır" ile MikroTik Dude'tan envanteri çekin.
        </div>
      ) : (
        <table className="data-table">
          <thead>
            <tr>
              <th>Ad</th>
              <th>IP</th>
              <th>Kategori</th>
              <th>Confidence</th>
              <th>Kanıt</th>
              <th>Platform</th>
              <th>Durum</th>
              <th>Son Görüldü</th>
            </tr>
          </thead>
          <tbody>
            {(devices ?? []).map((d) => (
              <tr key={d.id}>
                <td>
                  <strong>{d.name || "(isimsiz)"}</strong>
                  {d.mac ? <div style={{ fontSize: 11, color: "#888" }}>{d.mac}</div> : null}
                </td>
                <td>{d.host || "—"}</td>
                <td>
                  <span style={{
                    padding: "2px 8px",
                    borderRadius: 4,
                    fontSize: 12,
                    background: badgeColor(d.category),
                  }}>
                    {CATEGORY_LABELS[d.category]}
                  </span>
                </td>
                <td>
                  <span style={{
                    color: d.confidence < 50 ? "#fa3" : d.confidence < 80 ? "#cc3" : "#0c5",
                  }}>{d.confidence}</span>
                </td>
                <td style={{ fontSize: 11, color: "#aaa" }}>
                  {d.evidence_summary || "—"}
                </td>
                <td style={{ fontSize: 12 }}>
                  {d.platform || "—"}
                  {d.interface_name ? <div style={{ fontSize: 10, color: "#888" }}>{d.interface_name}</div> : null}
                </td>
                <td>{d.status}</td>
                <td style={{ fontSize: 12 }}>{new Date(d.last_seen_at).toLocaleString("tr-TR")}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}

function EnrichmentCard({ run }: { run: DiscoveryRun }) {
  const attempted = run.enrichment_sources_attempted ?? [];
  const succeeded = new Set(run.enrichment_sources_succeeded ?? []);
  const skipped = new Set(run.enrichment_sources_skipped ?? []);
  if (attempted.length === 0) {
    return null;
  }
  return (
    <div style={{
      margin: "12px 0",
      padding: 12,
      background: "#0e1014",
      border: "1px solid #1f242c",
      borderRadius: 6,
      fontSize: 12,
      color: "#cfd3d8",
    }}>
      <div style={{ marginBottom: 6, color: "#9aa1aa" }}>
        Enrichment kaynakları · {run.enrichment_duration_ms ?? 0}ms ·
        MAC: {run.with_mac_count ?? 0} · Host: {run.with_host_count ?? 0} ·
        Enriched: {run.enriched_count ?? 0}
      </div>
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
        {attempted.map((src) => {
          const ok = succeeded.has(src);
          const skip = skipped.has(src);
          const color = ok ? "#0c5" : skip ? "#cc3" : "#a22";
          const label = ok ? "✓" : skip ? "skipped" : "failed";
          return (
            <span key={src} style={{
              padding: "2px 8px",
              borderRadius: 4,
              background: "#161a20",
              border: `1px solid ${color}`,
              color,
            }}>
              {src} · {label}
            </span>
          );
        })}
      </div>
    </div>
  );
}

function badgeColor(c: NetworkCategory): string {
  switch (c) {
    case "AP": return "#1d6";
    case "CPE": return "#28a";
    case "BackhaulLink": return "#a3a";
    case "Bridge": return "#770";
    case "Router": return "#a52";
    case "Switch": return "#444";
    default: return "#555";
  }
}
