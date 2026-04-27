"use client";
import { useEffect, useState } from "react";
import { api, ApiError, ExecutiveSummary } from "@/lib/api";
import { StatCard } from "@/components/StatCard";
import Link from "next/link";

export function DashboardClient() {
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
            ? "Veritabanı bağlı değil. Skor motoru ve raporlar pasif."
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
          value={es?.critical_customers ?? "—"}
          meta="severity = critical (son skor)"
        />
        <StatCard
          title="Uyarıdaki Müşteri"
          value={es?.warning_customers ?? "—"}
          meta="severity = warning"
        />
        <StatCard
          title="AP-Wide Sorun"
          value={es?.ap_wide_interference_customers ?? "—"}
          meta="ap_wide_interference tanılı"
        />
        <StatCard
          title="Bayat Veri"
          value={es?.stale_customers ?? "—"}
          meta="is_stale=true"
        />
        <StatCard
          title="Açık İş Emri"
          value={es?.open_work_orders ?? "—"}
          meta="status ∈ open/assigned/in_progress"
        />
        <StatCard
          title="Urgent / High"
          value={es?.urgent_or_high_priority_work_orders ?? "—"}
          meta="priority urgent veya high"
        />
        <StatCard
          title="ETA Geçenler"
          value={es?.overdue_eta_work_orders ?? "—"}
          meta="eta_at < now ve aktif"
        />
        <StatCard
          title="Bugün Oluşturulan"
          value={es?.created_today_work_orders ?? "—"}
          meta="bu güne ait iş emri"
        />
      </div>

      <section className="section">
        <h3 className="section-title">Hızlı Erişim</h3>
        <ul style={{ lineHeight: 1.8 }}>
          <li>
            <Link href="/musteriler" style={{ color: "var(--accent)" }}>
              Sorunlu Müşteriler
            </Link>{" "}
            — skor sonucu açık problem listesi
          </li>
          <li>
            <Link href="/is-emirleri" style={{ color: "var(--accent)" }}>
              İş Emirleri
            </Link>{" "}
            — saha ekibi atama, ETA, çözüm akışı
          </li>
          <li>
            <Link href="/raporlar/yonetici-ozeti" style={{ color: "var(--accent)" }}>
              Yönetici Özeti
            </Link>{" "}
            — risk haritası ve trend
          </li>
          <li>
            <Link href="/raporlar" style={{ color: "var(--accent)" }}>
              Raporlar
            </Link>{" "}
            — CSV ve yazdırılabilir HTML/PDF
          </li>
        </ul>
      </section>
    </div>
  );
}
