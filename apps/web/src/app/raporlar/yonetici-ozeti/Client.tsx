"use client";
import { useEffect, useState } from "react";
import {
  api,
  ApiError,
  ExecutiveSummary,
  DIAGNOSIS_LABELS,
  Diagnosis,
  Severity,
} from "@/lib/api";
import { StatCard } from "@/components/StatCard";
import { Button, Toolbar } from "@/components/Toolbar";

const SEVERITY_BADGE: Record<Severity, { label: string; bg: string }> = {
  critical: { label: "KRİTİK", bg: "#7d1d1d" },
  warning: { label: "UYARI", bg: "#7a5a00" },
  healthy: { label: "Sağlıklı", bg: "#1c4f1c" },
  unknown: { label: "Bilinmiyor", bg: "#404040" },
};

export function ExecutiveSummaryClient() {
  const [es, setEs] = useState<ExecutiveSummary | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    (async () => {
      try {
        const r = await api.get<{ data: ExecutiveSummary }>(
          "/api/v1/reports/executive-summary"
        );
        setEs(r.data);
      } catch (e) {
        const msg =
          e instanceof ApiError && e.status === 503
            ? "Veritabanı bağlı değil. Yönetici özeti üretilemiyor."
            : e instanceof Error
              ? e.message
              : "Bilinmeyen hata";
        setError(msg);
      }
    })();
  }, []);

  if (error) return <div className="banner">{error}</div>;
  if (!es) return <p style={{ color: "var(--text-dim)" }}>Yükleniyor…</p>;

  return (
    <div>
      <Toolbar>
        <a
          href="/api/v1/reports/executive-summary.pdf"
          target="_blank"
          rel="noreferrer"
        >
          <Button>Yazdırılabilir / PDF</Button>
        </a>
        <span style={{ color: "var(--text-dim)", fontSize: 12 }}>
          Üretildi: {new Date(es.generated_at).toLocaleString("tr-TR")} · Dönem:{" "}
          {new Date(es.period_start).toLocaleDateString("tr-TR")} →{" "}
          {new Date(es.period_end).toLocaleDateString("tr-TR")}
        </span>
      </Toolbar>

      <div className="cards">
        <StatCard
          title="Toplam Aktif Müşteri"
          value={es.total_customers}
          meta="customers.status='active'"
        />
        <StatCard title="Kritik Müşteri" value={es.critical_customers} />
        <StatCard title="Uyarıdaki Müşteri" value={es.warning_customers} />
        <StatCard title="Bayat Veri" value={es.stale_customers} />
        <StatCard
          title="AP-Wide Etkilenen"
          value={es.ap_wide_interference_customers}
          meta="diagnosis=ap_wide_interference"
        />
        <StatCard title="Açık İş Emri" value={es.open_work_orders} />
        <StatCard
          title="Urgent / High"
          value={es.urgent_or_high_priority_work_orders}
        />
        <StatCard title="ETA Geçen" value={es.overdue_eta_work_orders} />
        <StatCard
          title="Bugün Oluşturulan"
          value={es.created_today_work_orders}
        />
      </div>

      <section className="section">
        <h3 className="section-title">En Riskli 10 AP</h3>
        {es.top10_risky_aps.length === 0 ? (
          <div className="empty">AP skoru henüz üretilmemiş.</div>
        ) : (
          <table>
            <thead>
              <tr>
                <th>AP</th>
                <th>Kule</th>
                <th>Skor</th>
                <th>Severity</th>
                <th>Müşteri</th>
                <th>Kritik</th>
                <th>Uyarı</th>
                <th>AP-Wide</th>
              </tr>
            </thead>
            <tbody>
              {es.top10_risky_aps.map((a) => {
                const sev = SEVERITY_BADGE[a.severity] ?? SEVERITY_BADGE.unknown;
                return (
                  <tr key={a.ap_device_id}>
                    <td>{a.ap_device_name || a.ap_device_id.slice(0, 8) + "…"}</td>
                    <td>{a.tower_name ?? "—"}</td>
                    <td style={{ fontWeight: 700 }}>{a.ap_score}</td>
                    <td>
                      <span
                        style={{
                          background: sev.bg,
                          color: "#fff",
                          padding: "2px 6px",
                          borderRadius: 4,
                          fontSize: 11,
                          fontWeight: 700,
                        }}
                      >
                        {sev.label}
                      </span>
                    </td>
                    <td>{a.total_customers}</td>
                    <td>{a.critical_customers}</td>
                    <td>{a.warning_customers}</td>
                    <td>{a.is_ap_wide_interference ? "Evet" : "—"}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </section>

      <section className="section">
        <h3 className="section-title">En Riskli 10 Kule</h3>
        {es.top10_risky_towers.length === 0 ? (
          <div className="empty">Kule risk skoru yok.</div>
        ) : (
          <table>
            <thead>
              <tr>
                <th>Kule</th>
                <th>Risk Skoru</th>
                <th>Severity</th>
                <th>Hesaplandı</th>
              </tr>
            </thead>
            <tbody>
              {es.top10_risky_towers.map((t) => {
                const sev = SEVERITY_BADGE[t.severity] ?? SEVERITY_BADGE.unknown;
                return (
                  <tr key={t.tower_id}>
                    <td>{t.tower_name || t.tower_id.slice(0, 8) + "…"}</td>
                    <td style={{ fontWeight: 700 }}>{t.risk_score}</td>
                    <td>
                      <span
                        style={{
                          background: sev.bg,
                          color: "#fff",
                          padding: "2px 6px",
                          borderRadius: 4,
                          fontSize: 11,
                          fontWeight: 700,
                        }}
                      >
                        {sev.label}
                      </span>
                    </td>
                    <td>{new Date(t.calculated_at).toLocaleString("tr-TR")}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </section>

      <section className="section">
        <h3 className="section-title">En Çok Tekrar Eden Tanılar</h3>
        {es.top10_diagnoses.length === 0 ? (
          <div className="empty">Tanı verisi yok.</div>
        ) : (
          <table>
            <thead>
              <tr>
                <th>Tanı</th>
                <th>Müşteri Sayısı</th>
              </tr>
            </thead>
            <tbody>
              {es.top10_diagnoses.map((d) => (
                <tr key={d.diagnosis}>
                  <td>{DIAGNOSIS_LABELS[d.diagnosis as Diagnosis] ?? d.diagnosis}</td>
                  <td>{d.count}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>

      <section className="section">
        <h3 className="section-title">Son 7 Gün Trendi</h3>
        {es.trend_7d.length === 0 ? (
          <div className="empty">Henüz skor üretilmemiş.</div>
        ) : (
          <table>
            <thead>
              <tr>
                <th>Gün</th>
                <th>Kritik</th>
                <th>Uyarı</th>
                <th>Sağlıklı</th>
              </tr>
            </thead>
            <tbody>
              {es.trend_7d.map((b) => (
                <tr key={b.day}>
                  <td>{new Date(b.day).toLocaleDateString("tr-TR")}</td>
                  <td>{b.critical}</td>
                  <td>{b.warning}</td>
                  <td>{b.healthy}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
    </div>
  );
}
