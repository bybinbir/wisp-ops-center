import { PageHeader } from "@/components/PageHeader";
import { DashboardClient } from "./DashboardClient";

export default function DashboardPage() {
  return (
    <div>
      <PageHeader
        title="Bugün ağda ne bozuk?"
        subtitle="Operasyonun ilk soruları: kime müdahale etmeliyim, hangi link riskli, hangi kule kötüleşiyor."
      />
      <DashboardClient />
    </div>
  );
}
