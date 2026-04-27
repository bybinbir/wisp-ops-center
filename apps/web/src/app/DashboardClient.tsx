"use client";
import { useEffect, useState } from "react";
import { api, ApiError, CustomerWithIssue } from "@/lib/api";
import { StatCard } from "@/components/StatCard";

type DashboardStats = {
  critical: number;
  warning: number;
  stale: number;
  apWide: number;
  lastCalculatedAt: string | null;
};

export function DashboardClient() {
  const [stats, setStats] = useState<DashboardStats | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    (async () => {
      try {
        const [crit, warn, stale, apw] = await Promise.all([
          api.get<{ data: CustomerWithIssue[] }>(
            "/api/v1/customers-with-issues?severity=critical&limit=500"
          ),
          api.get<{ data: CustomerWithIssue[] }>(
            "/api/v1/customers-with-issues?severity=warning&limit=500"
          ),
          api.get<{ data: CustomerWithIssue[] }>(
            "/api/v1/customers-with-issues?stale=true&limit=500"
          ),
          api.get<{ data: CustomerWithIssue[] }>(
            "/api/v1/customers-with-issues?diagnosis=ap_wide_interference&limit=500"
          ),
        ]);
        const all = [...(crit.data ?? []), ...(warn.data ?? [])];
        const last = all
          .map((r) => new Date(r.calculated_at).getTime())
          .reduce((a, b) => Math.max(a, b), 0);
        setStats({
          critical: (crit.data ?? []).length,
          warning: (warn.data ?? []).length,
          stale: (stale.data ?? []).length,
          apWide: (apw.data ?? []).length,
          lastCalculatedAt: last > 0 ? new Date(last).toISOString() : null,
        });
      } catch (e) {
        const msg =
          e instanceof ApiError && e.status === 503
            ? "Veritabanı bağlı değil. Skor motoru pasif."
            : e instanceof Error
              ? e.message
              : "Bilinmeyen hata";
        setError(msg);
      }
    })();
  }, []);

  return (
    <div>
      {error ? <div className="banner">{error}</div> : null}

      <div className="cards">
        <StatCard
          title="Kritik Müşteri"
          value={stats?.critical ?? "—"}
          meta={
            stats === null
              ? "yükleniyor"
              : "skor < uyarı eşiği · son skorlama satırı"
          }
        />
        <StatCard
          title="Uyarıdaki Müşteri"
          value={stats?.warning ?? "—"}
          meta={
            stats === null ? "yükleniyor" : "uyarı eşiği ile sağlıklı arası"
          }
        />
        <StatCard
          title="AP-Wide Sorun"
          value={stats?.apWide ?? "—"}
          meta="ap_wide_interference tanılı müşteri sayısı"
        />
        <StatCard
          title="Bayat Veri"
          value={stats?.stale ?? "—"}
          meta="is_stale=true · veri tazelik eşiği aşıldı"
        />
        <StatCard
          title="Son Skorlama"
          value={
            stats?.lastCalculatedAt
              ? new Date(stats.lastCalculatedAt).toLocaleString("tr-TR")
              : "—"
          }
          meta="customer_signal_scores en yeni satır"
        />
        <StatCard
          title="Frekans / Parazit"
          value="—"
          meta="öneri motoru (Faz 8)"
        />
      </div>

      <section className="section">
        <h3 className="section-title">Önceliklendirilmiş Olaylar</h3>
        <div className="empty">
          Sorunlu müşteri listesi <a href="/musteriler">/musteriler</a> sayfasında
          gösterilir. Skor üretimi worker veya{" "}
          <code>POST /api/v1/scoring/run</code> ile tetiklenir.
        </div>
      </section>
    </div>
  );
}
