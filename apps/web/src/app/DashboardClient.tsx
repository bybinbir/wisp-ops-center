"use client";
// Phase R1 — Operasyon Paneli.
// See docs/R1_OPERATOR_DASHBOARD_REPORT.md for design rationale.
// Sources from /api/v1/dashboard/operations-panel which aggregates
// discovery state + action lifecycle + safety chassis + reachability
// — i.e. what we actually have today, not the scoring KPIs the lab
// has never produced.

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import {
  api,
  ApiError,
  OperationsPanel,
  OpsDataInsufficient,
} from "@/lib/api";
import { StatCard } from "@/components/StatCard";

const POLL_MS = 15000;

type DataCode =
  | "real"
  | "missing"
  | "skipped"
  | "not_implemented"
  | "safety_blocked";

export function DashboardClient() {
  const [op, setOp] = useState<OperationsPanel | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  const reload = useCallback(async () => {
    try {
      const r = await api.get<OperationsPanel>(
        "/api/v1/dashboard/operations-panel",
      );
      setOp(r);
      setError(null);
    } catch (e) {
      const msg =
        e instanceof ApiError && e.status === 503
          ? "Veritabanı bağlı değil. Operasyon Paneli pasif kaldı."
          : e instanceof Error
            ? e.message
            : "Bilinmeyen hata";
      setError(msg);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    reload();
    const t = setInterval(reload, POLL_MS);
    return () => clearInterval(t);
  }, [reload]);

  if (loading) {
    return <div className="banner">Operasyon Paneli yükleniyor…</div>;
  }
  if (error || !op) {
    return <div className="banner">{error ?? "Veri yok"}</div>;
  }

  const disc = op.discovery;
  const totals = disc.totals;
  const acts = op.actions;
  const safety = op.safety;
  const health = op.health;

  return (
    <div>
      <section className="section">
        <div className="section-header">
          <h3 className="section-title">Ağ Envanteri (Dude Discovery)</h3>
          <DataSource code="real" />
        </div>
        <div className="cards">
          <StatCard
            title="Toplam Keşfedilen"
            value={totals.total}
            meta={
              disc.last_run
                ? `son discovery: ${formatRelative(disc.last_run.started_at)}`
                : "henüz koşturulmadı"
            }
          />
          <StatCard title="AP" value={totals.ap} meta="kategori = AP" />
          <StatCard
            title="Backhaul / Link"
            value={totals.link}
            meta="kategori = BackhaulLink"
          />
          <StatCard
            title="Bridge"
            value={totals.bridge}
            meta="kategori = Bridge"
          />
          <StatCard
            title="CPE / Müşteri"
            value={totals.cpe}
            meta="kategori = CPE"
          />
          <StatCard title="Router" value={totals.router} meta="kategori = Router" />
          <StatCard title="Switch" value={totals.switch} meta="kategori = Switch" />
          <StatCard
            title="Bilinmeyen"
            value={totals.unknown}
            meta={`%${disc.unknown_percentage.toFixed(1)} — Phase R2 hedefi: <%30`}
          />
        </div>
        <div style={{ marginTop: 8, fontSize: 12, color: "#9aa1aa" }}>
          MAC kazandı: <strong>{totals.with_mac}</strong> · Host kazandı:{" "}
          <strong>{totals.with_host}</strong> · Enriched:{" "}
          <strong>{totals.enriched}</strong> · Düşük Confidence:{" "}
          <strong>{totals.low_confidence}</strong> (%
          {disc.low_confidence_percentage.toFixed(1)})
        </div>
        {disc.last_run && <RunBriefRow run={disc.last_run} />}
      </section>

      <section className="section">
        <div className="section-header">
          <h3 className="section-title">Ağ Aksiyonları (son 24 saat)</h3>
          <DataSource code="real" />
        </div>
        <div className="cards">
          <StatCard title="Toplam çalıştırma" value={acts.last_24h.total} />
          <StatCard
            title="Başarılı"
            value={acts.last_24h.succeeded}
            meta="status = succeeded"
          />
          <StatCard
            title="Skipped"
            value={acts.last_24h.skipped}
            meta="hedef üzerinde menü yok / veri yok"
          />
          <StatCard
            title="Başarısız"
            value={acts.last_24h.failed}
            meta="status = failed"
          />
        </div>
        <div style={{ marginTop: 8, fontSize: 12, color: "#9aa1aa" }}>
          Türlere göre:{" "}
          {Object.keys(acts.last_24h.by_kind).length === 0
            ? "—"
            : Object.entries(acts.last_24h.by_kind)
                .map(([k, n]) => `${k}=${n}`)
                .join(" · ")}
        </div>
        {acts.latest_run && (
          <div style={{ marginTop: 12 }}>
            <div style={{ fontSize: 12, color: "#9aa1aa" }}>Son aksiyon:</div>
            <div className="latest-action-row">
              <strong>{acts.latest_run.action_type}</strong> ·{" "}
              {acts.latest_run.target_label ||
                acts.latest_run.target_host ||
                "—"}{" "}
              · <StatusChip s={acts.latest_run.status} /> · confidence{" "}
              {acts.latest_run.confidence}
              {acts.latest_run.dry_run && (
                <span style={{ marginLeft: 8, color: "#cc3" }}>· DRY-RUN</span>
              )}
            </div>
          </div>
        )}
      </section>

      <section className="section">
        <div className="section-header">
          <h3 className="section-title">Güvenlik Durumu</h3>
          <DataSource code={safety.dry_run_only ? "safety_blocked" : "real"} />
        </div>
        <div className="grid-two">
          <SafetyCell
            label="Destructive master switch"
            ok={!safety.destructive_enabled}
            okText="kapalı (güvenli)"
            failText="AÇIK (dikkat)"
          />
          <SafetyCell
            label="Legacy const flag"
            ok={!safety.legacy_master_switch_enabled}
            okText="kapalı"
            failText="AÇIK"
          />
          <SafetyCell
            label="Provider toggle"
            ok={!safety.provider_toggle_enabled}
            okText="kapalı"
            failText="AÇIK"
          />
          <SafetyCell
            label="Aktif bakım penceresi"
            ok={safety.active_maintenance_windows.length === 0}
            okText="yok (corrective action engelli)"
            failText={`${safety.active_maintenance_windows.length} aktif`}
          />
        </div>
        <div style={{ marginTop: 8, fontSize: 12, color: "#9aa1aa" }}>
          Engelleyiciler:{" "}
          {safety.blocking_reasons.length === 0
            ? "yok"
            : safety.blocking_reasons.join(" · ")}
        </div>
        <div className="info-box">
          Bu sayfa <strong>read-only</strong>. Operatör frequency
          correction veya başka destructive aksiyonu Phase R5'e kadar
          tarayıcıdan koşturamaz; iki katman master switch fail-closed.
        </div>
      </section>

      <section className="section">
        <div className="section-header">
          <h3 className="section-title">Sistem Sağlığı</h3>
          <DataSource code="real" />
        </div>
        <div className="grid-three">
          <SafetyCell
            label="DB bağlantısı"
            ok={health.db_ok}
            okText="OK"
            failText="bağlı değil"
          />
          <SafetyCell
            label="Dude konfigürasyonu"
            ok={health.dude_configured}
            okText={health.dude_host ?? "OK"}
            failText="MIKROTIK_DUDE_* env eksik"
          />
          <SafetyCell
            label="Dude erişimi (son test)"
            ok={!!health.last_dude_test_reachable}
            okText={
              health.last_dude_test_at
                ? `OK · ${formatRelative(health.last_dude_test_at)}`
                : "OK"
            }
            failText={
              health.last_dude_test_error_code
                ? health.last_dude_test_error_code
                : "test edilmedi"
            }
          />
        </div>
      </section>

      {op.data_insufficient.length > 0 && (
        <section className="section">
          <div className="section-header">
            <h3 className="section-title">Eksik / Yetersiz Veri</h3>
            <DataSource code="missing" />
          </div>
          <div style={{ display: "grid", gap: 10 }}>
            {op.data_insufficient.map((d) => (
              <DataInsufficientCard key={d.area_code} d={d} />
            ))}
          </div>
        </section>
      )}

      <section className="section">
        <h3 className="section-title">Hızlı Erişim</h3>
        <ul style={{ lineHeight: 1.8 }}>
          <li>
            <Link href="/ag-envanteri" style={{ color: "var(--accent)" }}>
              Ağ Envanteri
            </Link>{" "}
            — Dude discovery sonuçları, kategori filtreleri, "neden
            Bilinmeyen?" drill-down
          </li>
          <li>
            <Link href="/aksiyonlar" style={{ color: "var(--accent)" }}>
              Ağ Aksiyonları
            </Link>{" "}
            — read-only frequency / AP-client / link-signal / bridge-
            health çalıştırma geçmişi
          </li>
          <li>
            <Link href="/planli-kontroller" style={{ color: "var(--accent)" }}>
              Planlı Kontroller
            </Link>{" "}
            — scheduler + bakım pencereleri (Phase 5)
          </li>
          <li>
            <Link
              href="/raporlar/yonetici-ozeti"
              style={{ color: "var(--accent)" }}
            >
              Yönetici Özeti
            </Link>{" "}
            — risk + skor (telemetry yoksa boş; Phase R3 sonrası dolar)
          </li>
        </ul>
      </section>

      <div className="footer-note">
        Operasyon Paneli · {formatRelative(op.generated_at)} · 15 sn
        otomatik yenileme
      </div>

      <style jsx>{`
        .latest-action-row {
          margin-top: 4px;
          padding: 10px;
          background: #0e1014;
          border: 1px solid #1f242c;
          border-radius: 6px;
          font-size: 13px;
        }
        .grid-two {
          display: grid;
          grid-template-columns: 1fr 1fr;
          gap: 12px;
          font-size: 13px;
        }
        .grid-three {
          display: grid;
          grid-template-columns: 1fr 1fr 1fr;
          gap: 12px;
          font-size: 13px;
        }
        .info-box {
          margin-top: 8px;
          padding: 10px;
          background: #0e1014;
          border: 1px solid #1f242c;
          border-radius: 6px;
          font-size: 12px;
          color: #cfd3d8;
        }
        .footer-note {
          margin-top: 18px;
          font-size: 11px;
          color: #566071;
          text-align: right;
        }
      `}</style>
    </div>
  );
}

function StatusChip({ s }: { s: string }) {
  const colorByStatus: Record<string, string> = {
    queued: "#666",
    running: "#36c",
    succeeded: "#0c5",
    failed: "#a22",
    skipped: "#cc3",
    partial: "#cc3",
  };
  return (
    <span
      style={{
        padding: "1px 6px",
        borderRadius: 3,
        fontSize: 11,
        background: colorByStatus[s] ?? "#444",
        color: "#fff",
      }}
    >
      {s}
    </span>
  );
}

function SafetyCell({
  label,
  ok,
  okText,
  failText,
}: {
  label: string;
  ok: boolean;
  okText: string;
  failText: string;
}) {
  return (
    <div
      style={{
        padding: 10,
        background: "#0e1014",
        border: "1px solid #1f242c",
        borderRadius: 6,
      }}
    >
      <div style={{ color: "#9aa1aa", fontSize: 11 }}>{label}</div>
      <div
        style={{
          marginTop: 4,
          fontWeight: 600,
          color: ok ? "#0c5" : "#fa3",
        }}
      >
        {ok ? okText : failText}
      </div>
    </div>
  );
}

function RunBriefRow({
  run,
}: {
  run: NonNullable<OperationsPanel["discovery"]["last_run"]>;
}) {
  return (
    <div
      style={{
        marginTop: 12,
        padding: 10,
        background: "#0e1014",
        border: "1px solid #1f242c",
        borderRadius: 6,
        fontSize: 12,
        color: "#cfd3d8",
      }}
    >
      Son discovery run: <code>{run.id.slice(0, 8)}…</code> · status{" "}
      <StatusChip s={run.status} /> · {run.device_count} cihaz · trigger:{" "}
      {run.triggered_by || "—"}
      {run.error_code && (
        <span style={{ color: "#fbb", marginLeft: 8 }}>· {run.error_code}</span>
      )}
    </div>
  );
}

function DataInsufficientCard({ d }: { d: OpsDataInsufficient }) {
  return (
    <div
      style={{
        padding: 10,
        background: "#241c0e",
        border: "1px solid #443018",
        borderRadius: 6,
        fontSize: 13,
        color: "#fda",
      }}
    >
      <div style={{ fontWeight: 600 }}>{d.title}</div>
      <div style={{ marginTop: 4, fontSize: 12, color: "#cfd3d8" }}>{d.reason}</div>
      {d.hint && (
        <div style={{ marginTop: 4, fontSize: 12, color: "#9aa1aa" }}>
          → {d.hint}
        </div>
      )}
    </div>
  );
}

function DataSource({ code }: { code: DataCode }) {
  const map: Record<DataCode, { label: string; bg: string; fg: string }> = {
    real: { label: "Gerçek veri", bg: "#0c5", fg: "#fff" },
    missing: { label: "Eksik veri", bg: "#a22", fg: "#fff" },
    skipped: { label: "Skipped", bg: "#cc3", fg: "#000" },
    not_implemented: { label: "Henüz yok", bg: "#666", fg: "#fff" },
    safety_blocked: { label: "Güvenlik kilitli", bg: "#36c", fg: "#fff" },
  };
  const m = map[code];
  return (
    <span
      style={{
        marginLeft: 10,
        padding: "2px 8px",
        borderRadius: 3,
        fontSize: 11,
        background: m.bg,
        color: m.fg,
      }}
    >
      {m.label}
    </span>
  );
}

function formatRelative(iso: string): string {
  try {
    const d = new Date(iso);
    const ms = Date.now() - d.getTime();
    const s = Math.round(ms / 1000);
    if (s < 60) return `${s} sn önce`;
    if (s < 3600) return `${Math.round(s / 60)} dk önce`;
    if (s < 86400) return `${Math.round(s / 3600)} saat önce`;
    return d.toLocaleString("tr-TR");
  } catch {
    return iso;
  }
}
