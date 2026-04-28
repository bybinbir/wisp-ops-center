"use client";
import { useEffect, useState } from "react";
import {
  api,
  ActionRun,
  ApiError,
  APClientTestResult,
  BridgeHealthResult,
  FrequencyCheckResult,
  LinkSignalTestResult,
  WirelessSnapshot,
} from "@/lib/api";
import { StatCard } from "@/components/StatCard";

export function NetworkActionsClient() {
  const [runs, setRuns] = useState<ActionRun[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selected, setSelected] = useState<ActionRun | null>(null);
  const [filter, setFilter] = useState<string>("");

  async function reload() {
    setLoading(true);
    try {
      const path = filter
        ? `/api/v1/network/actions?action_type=${encodeURIComponent(filter)}`
        : "/api/v1/network/actions";
      const res = await api.get<{ data: ActionRun[] }>(path);
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

  useEffect(() => { reload(); }, [filter]);

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
        <StatCard title="Son durum" value={runs[0]?.status ?? "—"} />
      </div>

      <div style={{
        margin: "12px 0", padding: 12,
        background: "#0e1014", border: "1px solid #1f242c",
        borderRadius: 6, fontSize: 12, color: "#cfd3d8",
      }}>
        Bu sayfa sadece <strong>read-only</strong> aksiyonları gösterir. Frekans/kanal apply,
        bandwidth-test, reboot, set/add/remove ve diğer mutating işlemler Faz 10'a kadar
        engelli. Faz 9 v2 ile <strong>AP Client Test, Link Signal Test, Bridge Health
        Check</strong> read-only aksiyonları eklendi.
      </div>

      <div style={{ display: "flex", gap: 8, alignItems: "center", margin: "8px 0" }}>
        <span style={{ color: "#9aa1aa", fontSize: 12 }}>Filtre:</span>
        {[
          { label: "Hepsi", value: "" },
          { label: "Frekans", value: "frequency_check" },
          { label: "AP Client", value: "ap_client_test" },
          { label: "Link Signal", value: "link_signal_test" },
          { label: "Bridge Health", value: "bridge_health_check" },
        ].map((f) => (
          <button
            key={f.value}
            type="button"
            onClick={() => setFilter(f.value)}
            style={{
              fontSize: 11,
              padding: "3px 8px",
              border: "1px solid #1f242c",
              borderRadius: 3,
              background: filter === f.value ? "#1d6" : "#161a20",
              color: filter === f.value ? "#fff" : "#cfd3d8",
              cursor: "pointer",
            }}
          >
            {f.label}
          </button>
        ))}
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
          Henüz çalıştırılmış aksiyon yok. Ağ Envanteri'ndeki cihaz satırlarındaki
          read-only aksiyon butonları ile başlatın.
        </div>
      ) : (
        <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 16 }}>
          <table className="data-table">
            <thead>
              <tr>
                <th>Tip</th>
                <th>Hedef</th>
                <th>Durum</th>
                <th>Confidence</th>
                <th>Süre</th>
              </tr>
            </thead>
            <tbody>
              {runs.map((r) => (
                <tr key={r.id} onClick={() => setSelected(r)}
                  style={{ cursor: "pointer", background: selected?.id === r.id ? "#162028" : "" }}>
                  <td style={{ fontSize: 11 }}>{ACTION_LABELS[r.action_type] ?? r.action_type}</td>
                  <td>
                    <strong>{r.target_label || r.target_host || "—"}</strong>
                    {r.target_host ? <div style={{ fontSize: 10, color: "#888" }}>{r.target_host}</div> : null}
                  </td>
                  <td><StatusBadge s={r.status} /></td>
                  <td style={{ color: r.confidence < 50 ? "#fa3" : "#0c5" }}>{r.confidence}</td>
                  <td style={{ fontSize: 12 }}>{r.duration_ms}ms</td>
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

const ACTION_LABELS: Record<string, string> = {
  frequency_check: "Frekans Kontrol",
  ap_client_test: "AP Client Test",
  link_signal_test: "Link Signal Test",
  bridge_health_check: "Bridge Health",
};

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
  const fc = run.result?.frequency_check as FrequencyCheckResult | undefined;
  const ap = (run.result as { ap_client_test?: APClientTestResult })?.ap_client_test;
  const lr = (run.result as { link_signal_test?: LinkSignalTestResult })?.link_signal_test;
  const br = (run.result as { bridge_health_check?: BridgeHealthResult })?.bridge_health_check;
  return (
    <div style={{ fontSize: 13, color: "#cfd3d8" }}>
      <div style={{ marginBottom: 8 }}>
        <strong>{ACTION_LABELS[run.action_type] ?? run.action_type}</strong>{" "}
        <StatusBadge s={run.status} />
      </div>
      <div style={{ marginBottom: 8 }}>
        <strong>{run.target_label || run.target_host}</strong>
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
      {fc && <FrequencyCheckPanel fc={fc} />}
      {ap && <APClientTestPanel ap={ap} />}
      {lr && <LinkSignalPanel lr={lr} />}
      {br && <BridgeHealthPanel br={br} />}
    </div>
  );
}

function FrequencyCheckPanel({ fc }: { fc: FrequencyCheckResult }) {
  return (
    <div>
      {fc.skipped && (
        <SkippedBox reason={`Kablosuz arayüz bulunamadı (${fc.skipped_reason}). Sahte veri üretmedik.`} />
      )}
      {fc.menu_source && fc.menu_source !== "none" && (
        <div style={{ marginBottom: 4 }}>
          Identity: <strong>{fc.device_identity || "—"}</strong> · Menu: <code>{fc.menu_source}</code>
        </div>
      )}
      {fc.interfaces && fc.interfaces.length > 0 && (
        <div style={{ marginTop: 8 }}>
          <strong>Arayüzler:</strong>
          {fc.interfaces.map((iface) => <InterfaceCard key={iface.interface_name} iface={iface} />)}
        </div>
      )}
      <WarningsAndEvidence warnings={fc.warnings} evidence={fc.evidence} />
    </div>
  );
}

function APClientTestPanel({ ap }: { ap: APClientTestResult }) {
  return (
    <div>
      {ap.skipped && (
        <SkippedBox reason={`İstemci verisi yok (${ap.skipped_reason}). Sahte veri üretmedik.`} />
      )}
      <div style={{ marginBottom: 6 }}>
        Identity: <strong>{ap.device_identity || "—"}</strong> · menu: <code>{ap.menu_source ?? "—"}</code>
      </div>
      <div style={{ display: "flex", gap: 12, flexWrap: "wrap", margin: "8px 0" }}>
        <Metric label="Toplam istemci" value={String(ap.client_count)} />
        {typeof ap.avg_signal === "number" && <Metric label="Avg sinyal" value={`${ap.avg_signal} dBm`} />}
        {typeof ap.worst_signal === "number" && <Metric label="En kötü" value={`${ap.worst_signal} dBm`} />}
        {typeof ap.avg_ccq === "number" && <Metric label="Avg CCQ" value={`${ap.avg_ccq}%`} />}
      </div>
      {ap.weak_clients && ap.weak_clients.length > 0 && (
        <ClientList title={`Zayıf sinyalli istemciler (${ap.weak_clients.length})`} items={ap.weak_clients} />
      )}
      {ap.low_ccq_clients && ap.low_ccq_clients.length > 0 && (
        <ClientList title={`Düşük CCQ istemcileri (${ap.low_ccq_clients.length})`} items={ap.low_ccq_clients} />
      )}
      <WarningsAndEvidence warnings={ap.warnings} evidence={ap.evidence} />
    </div>
  );
}

function LinkSignalPanel({ lr }: { lr: LinkSignalTestResult }) {
  const healthColor = ({
    healthy: "#0c5", warning: "#cc3", critical: "#a22", unknown: "#666",
  } as const)[lr.health_status];
  return (
    <div>
      {lr.skipped && (
        <SkippedBox reason={`Link verisi yok (${lr.skipped_reason}). Sahte metrik üretmedik.`} />
      )}
      <div style={{ marginBottom: 6 }}>
        Identity: <strong>{lr.device_identity || "—"}</strong> · menu: <code>{lr.menu_source ?? "—"}</code>
        <span style={{
          marginLeft: 12, padding: "2px 8px", borderRadius: 4,
          background: healthColor, color: "#fff", fontSize: 12,
        }}>
          health: {lr.health_status}
        </span>
      </div>
      {lr.link_detected && (
        <div style={{ display: "flex", gap: 12, flexWrap: "wrap", margin: "8px 0" }}>
          <Metric label="Local iface" value={lr.local_interface ?? "—"} />
          {lr.remote_identifier ? <Metric label="Remote (mask)" value={lr.remote_identifier} /> : null}
          {typeof lr.signal === "number" && <Metric label="Sinyal" value={`${lr.signal} dBm`} />}
          {typeof lr.tx_rate_mbps === "number" && <Metric label="TX" value={`${lr.tx_rate_mbps} Mbps`} />}
          {typeof lr.rx_rate_mbps === "number" && <Metric label="RX" value={`${lr.rx_rate_mbps} Mbps`} />}
          {typeof lr.ccq === "number" && <Metric label="CCQ" value={`${lr.ccq}%`} />}
        </div>
      )}
      <WarningsAndEvidence warnings={lr.warnings} evidence={lr.evidence} />
    </div>
  );
}

function BridgeHealthPanel({ br }: { br: BridgeHealthResult }) {
  return (
    <div>
      {br.skipped && (
        <SkippedBox reason={`Köprü yapılandırılmamış (${br.skipped_reason}).`} />
      )}
      <div style={{ marginBottom: 6 }}>
        Identity: <strong>{br.device_identity || "—"}</strong>
        {br.running_summary ? <span style={{ color: "#9aa1aa" }}> · {br.running_summary}</span> : null}
      </div>
      <div style={{ display: "flex", gap: 12, flexWrap: "wrap", margin: "8px 0" }}>
        <Metric label="Bridge sayısı" value={String(br.bridge_count)} />
        <Metric label="Port sayısı" value={String(br.bridge_ports_count)} />
        <Metric label="Down port" value={String(br.down_ports?.length ?? 0)} />
        <Metric label="Disabled port" value={String(br.disabled_ports?.length ?? 0)} />
      </div>
      {br.bridges && br.bridges.length > 0 && (
        <div style={{ marginTop: 6 }}>
          <strong>Köprüler:</strong>
          <ul style={{ fontSize: 12, color: "#cfd3d8" }}>
            {br.bridges.map((b) => (
              <li key={b.name}>
                {b.name} · ports={b.port_count}
                {b.disabled ? " · disabled" : ""}
                {b.running === false ? " · not running" : ""}
              </li>
            ))}
          </ul>
        </div>
      )}
      {br.down_ports && br.down_ports.length > 0 && (
        <div style={{ marginTop: 6 }}>
          <strong>Down portlar:</strong>
          <ul style={{ fontSize: 12, color: "#fa3" }}>
            {br.down_ports.map((p) => <li key={`${p.bridge}-${p.interface_name}`}>{p.bridge} → {p.interface_name}</li>)}
          </ul>
        </div>
      )}
      {br.disabled_ports && br.disabled_ports.length > 0 && (
        <div style={{ marginTop: 6 }}>
          <strong>Disabled portlar:</strong>
          <ul style={{ fontSize: 12, color: "#cc3" }}>
            {br.disabled_ports.map((p) => <li key={`${p.bridge}-${p.interface_name}`}>{p.bridge} → {p.interface_name}</li>)}
          </ul>
        </div>
      )}
      <WarningsAndEvidence warnings={br.warnings} evidence={br.evidence} />
    </div>
  );
}

function SkippedBox({ reason }: { reason: string }) {
  return (
    <div style={{ padding: 8, background: "#332", color: "#cc8", borderRadius: 4, marginBottom: 8 }}>
      {reason}
    </div>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div style={{
      padding: "6px 10px", background: "#161a20", borderRadius: 4,
      minWidth: 90, fontSize: 12, color: "#cfd3d8",
    }}>
      <div style={{ color: "#9aa1aa", fontSize: 10 }}>{label}</div>
      <div style={{ marginTop: 2, fontWeight: 600 }}>{value}</div>
    </div>
  );
}

function ClientList({ title, items }: { title: string; items: NonNullable<APClientTestResult["weak_clients"]> }) {
  return (
    <div style={{ marginTop: 6 }}>
      <strong>{title}</strong>
      <ul style={{ fontSize: 12, color: "#cfd3d8", margin: "4px 0 0 16px" }}>
        {items.map((c, i) => (
          <li key={i}>
            {c.mac_prefix ?? "—"} · iface={c.interface_name ?? "—"}
            {typeof c.signal === "number" ? ` · ${c.signal} dBm` : ""}
            {typeof c.ccq === "number" ? ` · ccq=${c.ccq}%` : ""}
            {c.reason ? ` — ${c.reason}` : ""}
          </li>
        ))}
      </ul>
    </div>
  );
}

function WarningsAndEvidence({ warnings, evidence }: { warnings?: string[]; evidence?: string[] }) {
  return (
    <>
      {warnings && warnings.length > 0 && (
        <div style={{ marginTop: 8 }}>
          <strong>Uyarılar:</strong>
          <ul>
            {warnings.map((w, i) => <li key={i} style={{ color: "#fa3" }}>{w}</li>)}
          </ul>
        </div>
      )}
      {evidence && evidence.length > 0 && (
        <div style={{ marginTop: 8 }}>
          <strong>Kanıt:</strong>
          <ul>
            {evidence.map((e, i) => <li key={i} style={{ color: "#9aa1aa", fontSize: 12 }}>{e}</li>)}
          </ul>
        </div>
      )}
    </>
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
