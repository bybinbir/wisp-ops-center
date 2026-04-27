"use client";
import { useEffect, useState, FormEvent } from "react";
import { api } from "@/lib/api";
import { PageHeader } from "@/components/PageHeader";
import { Button, Toolbar } from "@/components/Toolbar";
import { Field, Select, TextInput } from "@/components/Field";

type Result = {
  id: number;
  test_type: string;
  target_ip: string;
  latency_avg_ms?: number | null;
  packet_loss_percent?: number | null;
  jitter_ms?: number | null;
  hop_count?: number | null;
  diagnosis: string;
  status: string;
  error_code?: string;
  created_at: string;
};

const ALLOWED = ["ping_latency", "packet_loss", "jitter", "traceroute"] as const;
const DISABLED = ["limited_throughput", "mikrotik_bandwidth_test"] as const;

export default function APClientTestsPage() {
  const [list, setList] = useState<Result[] | null>(null);
  const [form, setForm] = useState({
    ap_device_id: "",
    target_ip: "",
    test_type: "ping_latency",
    count: 5,
    timeout_ms: 1500,
    max_duration_seconds: 30
  });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function reload() {
    try {
      const r = await api.get<{ data: Result[] }>("/api/v1/ap-client-test-results");
      setList(r.data ?? []);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Bilinmeyen hata");
    }
  }
  useEffect(() => { reload(); }, []);

  async function runNow(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await api.post("/api/v1/ap-client-test-runs/run-now", { ...form, risk_level: "low" });
      await reload();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Test başarısız");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div>
      <PageHeader
        title="AP → Client Testleri"
        subtitle="Faz 5'te güvenli, sınırlı, sunucu tarafından başlatılan ping/loss/jitter/traceroute testleri."
      />
      <div className="banner">
        <strong>Sözleşme:</strong> Bu testler wisp-ops-center sunucusundan
        hedef IP'ye doğru çalışır (gerçek AP-üzerinde icra Faz 5'in dışında
        kaldı). count, timeout ve süre üst sınırları sabittir; yüksek riskli{" "}
        <span className="kbd">mikrotik_bandwidth_test</span> ve{" "}
        <span className="kbd">limited_throughput</span> kapalıdır.
      </div>

      <form onSubmit={runNow} style={{ display: "grid", gridTemplateColumns: "1fr 1fr 1fr 1fr 1fr auto", gap: 8, alignItems: "end", marginBottom: 12 }}>
        <Field label="AP Device ID">
          <TextInput value={form.ap_device_id} onChange={(e) => setForm({ ...form, ap_device_id: e.target.value })} />
        </Field>
        <Field label="Target IP">
          <TextInput required value={form.target_ip} onChange={(e) => setForm({ ...form, target_ip: e.target.value })} />
        </Field>
        <Field label="Test Tipi">
          <Select value={form.test_type} onChange={(e) => setForm({ ...form, test_type: e.target.value })}>
            {ALLOWED.map((t) => <option key={t} value={t}>{t}</option>)}
            {DISABLED.map((t) => <option key={t} disabled value={t}>{t} (kapalı)</option>)}
          </Select>
        </Field>
        <Field label="Count">
          <TextInput type="number" value={String(form.count)} onChange={(e) => setForm({ ...form, count: Number(e.target.value) })} />
        </Field>
        <Field label="Timeout (ms)">
          <TextInput type="number" value={String(form.timeout_ms)} onChange={(e) => setForm({ ...form, timeout_ms: Number(e.target.value) })} />
        </Field>
        <Toolbar>
          <Button type="submit" disabled={busy}>{busy ? "Çalışıyor…" : "Run Now"}</Button>
        </Toolbar>
      </form>

      {error ? <div className="banner">{error}</div> : null}

      <table>
        <thead>
          <tr>
            <th>Zaman</th><th>Tip</th><th>Hedef</th><th>Avg Latency</th><th>Loss%</th><th>Jitter</th><th>Hops</th><th>Tanı</th><th>Status</th>
          </tr>
        </thead>
        <tbody>
          {list && list.length === 0 ? (<tr><td colSpan={9} className="empty">Henüz sonuç yok.</td></tr>) : null}
          {(list ?? []).map((r) => (
            <tr key={r.id}>
              <td>{new Date(r.created_at).toLocaleString("tr-TR")}</td>
              <td>{r.test_type}</td>
              <td>{r.target_ip}</td>
              <td>{r.latency_avg_ms != null ? `${r.latency_avg_ms} ms` : "—"}</td>
              <td>{r.packet_loss_percent != null ? `${r.packet_loss_percent}%` : "—"}</td>
              <td>{r.jitter_ms != null ? `${r.jitter_ms} ms` : "—"}</td>
              <td>{r.hop_count ?? "—"}</td>
              <td>
                <span className={`badge ${r.diagnosis === "healthy" ? "good" : r.diagnosis === "data_insufficient" ? "warn" : "bad"}`}>
                  {r.diagnosis}
                </span>
              </td>
              <td>
                <span className={`badge ${r.status === "success" ? "good" : r.status === "blocked" ? "bad" : "warn"}`}>
                  {r.status}
                </span>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
