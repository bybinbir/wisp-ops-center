import { TowersClient } from "./Client";
import { PageHeader } from "@/components/PageHeader";

export default function TowersPage() {
  return (
    <div>
      <PageHeader
        title="Kuleler"
        subtitle="Her kule için temel envanter alanları. Sağlık metrikleri Faz 3+ ile gerçek poll'dan gelecek."
      />
      <TowersClient />
    </div>
  );
}
