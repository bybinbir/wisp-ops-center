"use client";
// Phase R1 — "Why is this device classified this way?" modal.
// Sources from GET /api/v1/network/devices/{id}/evidence.

import { useEffect, useState } from "react";
import {
  api,
  ApiError,
  CATEGORY_LABELS,
  DeviceEvidence,
  NetworkCategory,
} from "@/lib/api";

export function EvidenceModal({
  deviceId,
  onClose,
}: {
  deviceId: string;
  onClose: () => void;
}) {
  const [data, setData] = useState<DeviceEvidence | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const r = await api.get<DeviceEvidence>(
          `/api/v1/network/devices/${deviceId}/evidence`,
        );
        if (!cancelled) setData(r);
      } catch (e) {
        if (cancelled) return;
        setError(
          e instanceof ApiError ? e.message : (e as Error).message,
        );
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [deviceId]);

  return (
    <div
      onClick={onClose}
      style={{
        position: "fixed",
        inset: 0,
        background: "rgba(0,0,0,0.65)",
        zIndex: 50,
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        padding: 16,
      }}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        style={{
          background: "#11141a",
          border: "1px solid #1f242c",
          borderRadius: 8,
          width: "min(880px, 100%)",
          maxHeight: "92vh",
          overflowY: "auto",
          padding: 18,
          color: "#cfd3d8",
        }}
      >
        <div style={{ display: "flex", justifyContent: "space-between" }}>
          <h2 style={{ margin: 0, fontSize: 18 }}>
            Cihaz Detayı — Sınıflandırma Kanıtı
          </h2>
          <button
            type="button"
            onClick={onClose}
            style={{
              background: "transparent",
              color: "#9aa1aa",
              border: "1px solid #1f242c",
              borderRadius: 4,
              padding: "2px 10px",
              cursor: "pointer",
            }}
          >
            ×
          </button>
        </div>

        {error && (
          <div
            style={{
              margin: "12px 0",
              padding: 10,
              background: "#400",
              color: "#fbb",
              borderRadius: 4,
              fontSize: 12,
            }}
          >
            {error}
          </div>
        )}

        {!data && !error && (
          <div style={{ marginTop: 14, color: "#9aa1aa" }}>Yükleniyor…</div>
        )}

        {data && (
          <div style={{ marginTop: 14 }}>
            <DeviceHeader d={data} />
            <Why d={data} />
            <MissingSignals d={data} />
            <ApplicableActions d={data} />
            <RecentActions d={data} />
            <RawEvidence d={data} />
          </div>
        )}
      </div>
    </div>
  );
}

function DeviceHeader({ d }: { d: DeviceEvidence }) {
  const cat = d.device.category as NetworkCategory;
  return (
    <div
      style={{
        padding: 10,
        background: "#0e1014",
        border: "1px solid #1f242c",
        borderRadius: 6,
        fontSize: 13,
      }}
    >
      <div>
        <strong>{d.device.name || "(isimsiz)"}</strong> ·{" "}
        <span
          style={{
            padding: "1px 6px",
            borderRadius: 3,
            fontSize: 11,
            background: "#222",
            color: "#fff",
          }}
        >
          {CATEGORY_LABELS[cat] ?? d.device.category}
        </span>{" "}
        · confidence{" "}
        <span
          style={{
            color:
              d.device.confidence < 50
                ? "#fa3"
                : d.device.confidence < 80
                  ? "#cc3"
                  : "#0c5",
          }}
        >
          {d.device.confidence}
        </span>
      </div>
      <div style={{ marginTop: 4, fontSize: 11, color: "#9aa1aa" }}>
        {d.device.host && <>IP: {d.device.host} · </>}
        {d.device.mac && <>MAC: {d.device.mac} · </>}
        {d.device.platform && <>{d.device.platform} · </>}
        {d.device.interface_name && <>iface: {d.device.interface_name} · </>}
        son görüldü:{" "}
        {new Date(d.device.last_seen_at).toLocaleString("tr-TR")}
      </div>
    </div>
  );
}

function Why({ d }: { d: DeviceEvidence }) {
  const s = d.evidence_summary;
  const cat = d.device.category;
  const operatorBlurb =
    cat === "Unknown"
      ? `Bu cihaz Bilinmeyen, çünkü hiçbir kanıt eşiği aşmadı (en yüksek ağırlık: ${s.winner} = ${s.winner_weight}).`
      : `Bu cihaz "${cat}" olarak sınıflandı (ağırlık: ${s.winner_weight})${s.runner_up ? `; ikinci en yakın aday: ${s.runner_up} (${s.runner_up_weight})` : ""}.`;
  return (
    <section style={{ marginTop: 12 }}>
      <h3 style={{ margin: 0, fontSize: 14 }}>Neden bu kategori?</h3>
      <div style={{ marginTop: 6, fontSize: 13 }}>{operatorBlurb}</div>
      <div style={{ marginTop: 6, fontSize: 12, color: "#9aa1aa" }}>
        Toplam kanıt satırı: {s.total_rows} · benzersiz heuristic:{" "}
        {s.unique_heuristics.length === 0
          ? "—"
          : s.unique_heuristics.join(", ")}
      </div>
      {Object.keys(s.weight_by_category).length > 0 && (
        <div style={{ marginTop: 6, fontSize: 12 }}>
          Kategori ağırlıkları:{" "}
          {Object.entries(s.weight_by_category)
            .sort((a, b) => b[1] - a[1])
            .map(([k, v]) => `${k}=${v}`)
            .join(" · ")}
        </div>
      )}
    </section>
  );
}

function MissingSignals({ d }: { d: DeviceEvidence }) {
  if (d.missing_signals.length === 0) {
    return (
      <section style={{ marginTop: 12 }}>
        <h3 style={{ margin: 0, fontSize: 14 }}>Eksik Sinyaller</h3>
        <div style={{ marginTop: 6, fontSize: 13, color: "#0c5" }}>
          Tüm enrichment alanları dolu — sınıflandırma için yeterli sinyal var.
        </div>
      </section>
    );
  }
  return (
    <section style={{ marginTop: 12 }}>
      <h3 style={{ margin: 0, fontSize: 14 }}>Eksik Sinyaller</h3>
      <div style={{ display: "grid", gap: 6, marginTop: 6 }}>
        {d.missing_signals.map((m) => (
          <div
            key={m.signal}
            style={{
              padding: 8,
              background: "#241c0e",
              border: "1px solid #443018",
              borderRadius: 4,
              fontSize: 12,
            }}
          >
            <div style={{ fontWeight: 600, color: "#fda" }}>{m.signal}</div>
            <div style={{ marginTop: 2, color: "#cfd3d8" }}>
              {m.explanation}
            </div>
            <div style={{ marginTop: 2, color: "#9aa1aa" }}>
              → {m.would_help}
            </div>
          </div>
        ))}
      </div>
    </section>
  );
}

function ApplicableActions({ d }: { d: DeviceEvidence }) {
  return (
    <section style={{ marginTop: 12 }}>
      <h3 style={{ margin: 0, fontSize: 14 }}>Uygulanabilir Aksiyonlar</h3>
      <div
        style={{
          display: "grid",
          gridTemplateColumns: "1fr 1fr",
          gap: 6,
          marginTop: 6,
        }}
      >
        {d.applicable_actions.map((a) => (
          <div
            key={a.kind}
            style={{
              padding: 8,
              background: "#0e1014",
              border:
                "1px solid " +
                (a.applicable === "likely_yes"
                  ? "#0c5"
                  : a.applicable === "likely_no"
                    ? "#a22"
                    : "#444"),
              borderRadius: 4,
              fontSize: 12,
            }}
          >
            <div style={{ fontWeight: 600 }}>
              {a.label}{" "}
              <span
                style={{
                  fontSize: 10,
                  padding: "1px 6px",
                  borderRadius: 3,
                  marginLeft: 4,
                  background:
                    a.applicable === "likely_yes"
                      ? "#0c5"
                      : a.applicable === "likely_no"
                        ? "#a22"
                        : "#666",
                  color: "#fff",
                }}
              >
                {labelFor(a.applicable)}
              </span>
            </div>
            <div style={{ marginTop: 4, color: "#cfd3d8" }}>{a.reason}</div>
            <div style={{ marginTop: 2, color: "#9aa1aa" }}>
              güvenlik: {a.safety_status}
            </div>
          </div>
        ))}
      </div>
    </section>
  );
}

function labelFor(a: string) {
  if (a === "likely_yes") return "büyük olasılıkla çalışır";
  if (a === "likely_no") return "büyük olasılıkla skipped";
  return "belirsiz — denenebilir";
}

function RecentActions({ d }: { d: DeviceEvidence }) {
  if (!d.recent_actions || d.recent_actions.length === 0) {
    return (
      <section style={{ marginTop: 12 }}>
        <h3 style={{ margin: 0, fontSize: 14 }}>Aksiyon Geçmişi</h3>
        <div style={{ marginTop: 6, fontSize: 12, color: "#9aa1aa" }}>
          Bu cihaz için henüz aksiyon koşturulmadı.
        </div>
      </section>
    );
  }
  return (
    <section style={{ marginTop: 12 }}>
      <h3 style={{ margin: 0, fontSize: 14 }}>Aksiyon Geçmişi (son 10)</h3>
      <table
        className="data-table"
        style={{ width: "100%", marginTop: 6, fontSize: 12 }}
      >
        <thead>
          <tr>
            <th>Tip</th>
            <th>Durum</th>
            <th>Süre</th>
            <th>Conf.</th>
            <th>Tarih</th>
            <th>Sebep</th>
          </tr>
        </thead>
        <tbody>
          {d.recent_actions.map((a) => (
            <tr key={a.id}>
              <td>{a.action_type}</td>
              <td>{a.status}</td>
              <td>{a.duration_ms}ms</td>
              <td>{a.confidence}</td>
              <td>{new Date(a.started_at).toLocaleString("tr-TR")}</td>
              <td style={{ color: "#fda" }}>
                {a.skipped ? a.skip_reason || "skipped" : ""}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}

function RawEvidence({ d }: { d: DeviceEvidence }) {
  if (d.evidence.length === 0) {
    return (
      <section style={{ marginTop: 12 }}>
        <h3 style={{ margin: 0, fontSize: 14 }}>Ham Kanıt</h3>
        <div style={{ marginTop: 6, fontSize: 12, color: "#9aa1aa" }}>
          device_category_evidence boş — bu cihaz için sınıflandırıcı
          hiçbir ipucu kayıt etmedi.
        </div>
      </section>
    );
  }
  return (
    <section style={{ marginTop: 12 }}>
      <h3 style={{ margin: 0, fontSize: 14 }}>
        Ham Kanıt (teknik detay)
      </h3>
      <details style={{ marginTop: 6, fontSize: 12 }}>
        <summary style={{ cursor: "pointer", color: "#9aa1aa" }}>
          {d.evidence.length} satır göster
        </summary>
        <table
          className="data-table"
          style={{ width: "100%", marginTop: 6 }}
        >
          <thead>
            <tr>
              <th>Heuristic</th>
              <th>Kategori</th>
              <th>Ağırlık</th>
              <th>Sebep</th>
              <th>Run</th>
            </tr>
          </thead>
          <tbody>
            {d.evidence.map((e) => (
              <tr key={e.id}>
                <td>{e.heuristic}</td>
                <td>{e.category}</td>
                <td>{e.weight}</td>
                <td>{e.reason}</td>
                <td style={{ color: "#566071" }}>
                  {e.run_id ? e.run_id.slice(0, 8) + "…" : "—"}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </details>
    </section>
  );
}
