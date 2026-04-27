import { PageHeader } from "@/components/PageHeader";
import { SkeletonNotice } from "@/components/SkeletonNotice";
import { StatCard } from "@/components/StatCard";

export default function DashboardPage() {
  return (
    <div>
      <PageHeader
        title="Bugün ağda ne bozuk?"
        subtitle="Operasyonun ilk soruları: kime müdahale etmeliyim, hangi link riskli, hangi kule kötüleşiyor."
      />

      <SkeletonNotice />

      <div className="cards">
        <StatCard title="Kritik Linkler" value="—" meta="Faz 4 sonrası" />
        <StatCard
          title="Kötü Sinyalli Müşteriler"
          value="—"
          meta="Faz 6 sonrası"
        />
        <StatCard
          title="Kötüleşen AP'ler"
          value="—"
          meta="trend analizi (Faz 6+)"
        />
        <StatCard
          title="Frekans / Parazit Riski"
          value="—"
          meta="öneri motoru (Faz 8)"
        />
        <StatCard title="Çevrimdışı Cihazlar" value="—" meta="poll (Faz 3+)" />
        <StatCard
          title="Bugünkü Planlı Kontroller"
          value="—"
          meta="zamanlayıcı (Faz 5)"
        />
      </div>

      <section className="section">
        <h3 className="section-title">Önceliklendirilmiş Olaylar</h3>
        <div className="empty">
          Henüz olay yok. Olaylar Faz 5 zamanlayıcı + Faz 6 skor motoru
          devreye alındığında burada listelenir.
        </div>
      </section>
    </div>
  );
}
