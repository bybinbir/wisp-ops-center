import Link from "next/link";
import { PageHeader } from "@/components/PageHeader";

export default function ReportsPage() {
  return (
    <div>
      <PageHeader
        title="Raporlar"
        subtitle="Operasyon, müşteri sağlığı ve iş emri raporları."
      />

      <section className="section">
        <h3 className="section-title">Yönetici Özeti</h3>
        <p style={{ color: "var(--text-dim)" }}>
          Severity dağılımı, en riskli AP/kuleler, açık iş emirleri ve son 7/30
          gün trendi.{" "}
          <Link href="/raporlar/yonetici-ozeti" style={{ color: "var(--accent)" }}>
            Yönetici özetini aç →
          </Link>
        </p>
      </section>

      <section className="section">
        <h3 className="section-title">CSV İndirmeleri</h3>
        <ul>
          <li>
            <a
              href="/api/v1/reports/problem-customers.csv"
              target="_blank"
              rel="noreferrer"
            >
              Sorunlu Müşteriler (CSV)
            </a>{" "}
            <span style={{ color: "var(--text-dim)", fontSize: 12 }}>
              — son skor satırı bazlı, severity warning/critical
            </span>
          </li>
          <li>
            <a
              href="/api/v1/reports/ap-health.csv"
              target="_blank"
              rel="noreferrer"
            >
              AP Sağlığı (CSV)
            </a>{" "}
            <span style={{ color: "var(--text-dim)", fontSize: 12 }}>
              — degradation_ratio + AP-wide interference
            </span>
          </li>
          <li>
            <a
              href="/api/v1/reports/tower-risk.csv"
              target="_blank"
              rel="noreferrer"
            >
              Kule Risk Skoru (CSV)
            </a>
          </li>
          <li>
            <a
              href="/api/v1/reports/work-orders.csv"
              target="_blank"
              rel="noreferrer"
            >
              İş Emirleri (CSV)
            </a>
          </li>
        </ul>
      </section>

      <section className="section">
        <h3 className="section-title">Yazdırılabilir / PDF</h3>
        <ul>
          <li>
            <a
              href="/api/v1/reports/executive-summary.pdf"
              target="_blank"
              rel="noreferrer"
            >
              Yönetici Özeti (HTML — tarayıcıdan PDF olarak kaydedilir)
            </a>
          </li>
          <li>
            <a
              href="/api/v1/reports/work-orders.pdf"
              target="_blank"
              rel="noreferrer"
            >
              İş Emirleri Raporu (HTML — tarayıcıdan PDF olarak kaydedilir)
            </a>
          </li>
        </ul>
        <p style={{ color: "var(--text-dim)", fontSize: 12 }}>
          Faz 7 — Server-side PDF rendering açık teknik borçtur. Tarayıcı
          "Yazdır → PDF olarak kaydet" akışı kurumsal görünümde A4 çıktı verir.
        </p>
      </section>
    </div>
  );
}
