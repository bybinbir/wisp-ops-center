"use client";
import { useEffect, useState } from "react";
import { api, ActionRun, ApiError, FrequencyCheckResult, WirelessSnapshot } from "@/lib/api";
import { StatCard } from "@/components/StatCard";

export function NetworkActionsClient() {
  const [runs, setRuns] = useState<ActionRun[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selected, setSelected] = useState<ActionRun | null>(null);

  async function reload() {
    setLoading(true);
    try {
      const res = await api.get<{ data: ActionRun[] }>(
        "/api/v1/network/actions?action_type=frequency_check"
      );
      setRuns(res.data ?? []);
      setError(null);
    } catch (e) {
      const msg = e instanceof ApiError ? e.message : (e as Error).message;
      setError(msg);
      setRuns([]);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => { reload(); }, []);

  // Auto-refresh while any run is still running.
  useEffect(() => {
    const anyRunning = runs.some((r) => r.status === "running" || r.status === "queued");
    if (!anyRunning) return;
    const t = setInterval(reload, 3000);
    return () => clearInterval(t);
  }, [runs]);

  const completed = runs.filter((r) => r.status === "succeeded" || r.status === "skipped");
  const failed = runs.filter((r) => r.status === "failed");

  return (
    <div>
      <div className="grid grid-4" style={{ marginBottom: 16 }}>
        <StatCard title="Toplam Çalıştırma" value={runs.length} />
        <StatCard title="Başarılı" value={completed.length} />
        <StatCard title="Başarısız" value={failed.length} />
        <StatCard
          title="Son durum"
          value={runs[0]?.status ?? "—"}
        />
      </div>

      <div style={{
        margin: "12px 0", padding: 12,
        background: "#0e1014", border: "1px solid #1f242c",
        borderRadius: 6, fontSize: 12, color: "#cfd3d8",
      }}>
        Bu sayfa sadece <strong>read-only</strong> aksiyonları gösterir. Frekans/kanal apply,
        bandwidth-test, reboot ve diğer mutating işlemler Faz 10'a kadar engelli.
      </div>

      {error && (
        <div style={{ margin: "12px 0", padding: 12, background: "#400", color: "#fbb", borderRadius: 6 }}>
          {error}
        </div>
      )}

      {loading ? (
        <div style={{ padding: 24, color: "#888" }}>Yükleniyor…</div>
      ) : runs.length === 0 ? (
        <div style={{ padding: 24, color: "#888" }}>
          Henüz çalıştırılmış aksiyon yok. Ağ Envanteri'ndeki bir cihazın yanındaki
          "Frekans Kontrol" butonu ile başlatın.
        </div>
      ) : (
        <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 16 }}>
          <table className="data-table">
            <thead>
              <tr>
                <th>Hedef</th>
                <th>Durum</th>
                <th>Confidence</th>
                <th>Süre (ms)</th>
                <th>Başlangıç</th>
              </tr>
            </thead>
            <tbody>
              {runs.map((r) => (
                <tr key={r.id} onClick={() => setSelected(r)}
                  style={{ cursor: "pointer", background: selected?.id === r.id ? "#162028" : "" }}>
                  <td>
                    <strong>{r.target_label || r.target_host || "—"}</strong>
                    {r.target_host ? <div style={{ fontSize: 11, color: "#888" }}>{r.target_host}</div> : null}
                  </td>
                  <td><StatusBadge s={r.status} /></td>
                  <td style={{ color: r.confidence < 50 ? "#fa3" : "#0c5" }}>{r.confidence}</td>
                  <td>{r.duration_ms}</td>
                  <td style={{ fontSize: 12 }}>
                    {r.started_at ? new Date(r.started_at).toLocaleString("tr-TR") : "—"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>

          <div style={{
            padding: 16, background: "#0e1014", border: "1px solid #1f242c", borderRadius: 6,
          }}>
            {selected ? (
              <RunDetail run={selected} />
            ) : (
              <div style={{ color: "#888", padding: 12 }}>
                Detayları görmek için bir çalıştırma seçin.
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

function StatusBadge({ s }: { s: ActionRun["status"] }) {
  const colorByStatus: Record<ActionRun["status"], string> = {
    queued: "#666",
    running: "#36c",
    succeeded: "#0c5",
    failed: "#a22",
    skipped: "#cc3",
  };
  return (
    <span style={{
      padding: "2px 8px", borderRadius: 4, fontSize: 12,
      background: colorByStatus[s], color: "#fff",
    }}>
      {s}
    </span>
  );
}

function RunDetail({ run }: { run: ActionRun }) {
  const fc: FrequencyCheckResult | undefined = run.result?.frequency_check;
  return (
    <div style={{ fontSize: 13, color: "#cfd3d8" }}>
      <div style={{ marginBottom: 12 }}>
        <strong>{run.target_label || run.target_host}</strong>{" "}
        <StatusBadge s={run.status} />
      </div>
      <div style={{ fontSize: 12, color: "#9aa1aa", marginBottom: 8 }}>
        ID: {run.id} · correlation: {run.correlation_id}<br/>
        Süre: {run.duration_ms}ms · komut: {run.command_count} · uyarı: {run.warning_count} · confidence: {run.confidence}
        {run.dry_run ? " · DRY-RUN" : ""}
      </div>
      {run.error_code && (
        <div style={{ padding: 8, background: "#400", color: "#fbb", borderRadius: 4, marginBottom: 8 }}>
          {run.error_code}: {run.error_message ?? ""}
        </div>
      )}
      {fc?.skipped && (
        <div style={{ padding: 8, background: "#332", color: "#cc8", borderRadius: 4, marginBottom: 8 }}>
          Kablosuz arayüz bulunamadı ({fc.skipped_reason}). Sahte veri üretmedik.
        </div>
      )}
      {fc?.menu_source && fc.menu_source !== "none" && (
        <div style={{ marginBottom: 4 }}>
          Identity: <strong>{fc.device_identity || "—"}</strong> · Menu: <code>{fc.menu_source}</code>
        </div>
      )}
      {fc?.interfaces && fc.interfaces.length > 0 && (
        <div style={{ marginTop: 8 }}>
          <strong>Arayüzler:</strong>
          {fc.interfaces.map((iface) => (
            <InterfaceCard key={iface.interface_name} iface={iface} />
          ))}
        </div>
      )}
      {fc?.warnings && fc.warnings.length > 0 && (
        <div style={{ marginTop: 8 }}>
          <strong>Uyarılar:</strong>
          <ul>
            {fc.warnings.map((w, i) => <li key={i} style={{ color: "#fa3" }}>{w}</li>)}
          </ul>
        </div>
      )}
      {fc?.evidence && fc.evidence.length > 0 && (
        <div style={{ marginTop: 8 }}>
          <strong>Kanıt:</strong>
          <ul>
            {fc.evidence.map((e, i) => <li key={i} style={{ color: "#9aa1aa", fontSize: 12 }}>{e}</li>)}
          </ul>
        </div>
      )}
    </div>
  );
}

function InterfaceCard({ iface }: { iface: WirelessSnapshot }) {
  return (
    <div style={{
      marginTop: 6, padding: 10, background: "#161a20",
      borderRadius: 4, fontSize: 12,
    }}>
      <div><strong>{iface.interface_name}</strong> {iface.ssid ? `· ${iface.ssid}` : ""}</div>
      <div style={{ color: "#9aa1aa", marginTop: 2 }}>
        {iface.frequency ? `freq=${iface.frequency} ` : ""}
        {iface.band ? `band=${iface.band} ` : ""}
        {iface.channel_width ? `width=${iface.channel_width} ` : ""}
        {iface.mode ? `mode=${iface.mode}` : ""}
      </div>
      {iface.registration_ok && (
        <div style={{ color: "#9aa1aa", marginTop: 2 }}>
          clients={iface.client_count}
          {iface.avg_signal !== undefined ? ` · avg=${iface.avg_signal} dBm` : ""}
          {iface.worst_signal !== undefined ? ` · worst=${iface.worst_signal} dBm` : ""}
          {iface.avg_ccq !== undefined ? ` · ccq=${iface.avg_ccq}%` : ""}
        </div>
      )}
    </div>
  );
}
