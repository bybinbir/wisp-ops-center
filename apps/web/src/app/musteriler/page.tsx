import { PageHeader } from "@/components/PageHeader";
import { ProblemCustomersClient } from "./Client";

export default function CustomersPage() {
  return (
    <div>
      <PageHeader
        title="Sorunlu Müşteriler"
        subtitle="Skor motoru kötü sinyal, kopukluk veya CPE alignment sorunu olan müşterileri buraya düşürür. Telemetri yoksa skor üretilmez (data_insufficient)."
      />
      <ProblemCustomersClient />
    </div>
  );
}
