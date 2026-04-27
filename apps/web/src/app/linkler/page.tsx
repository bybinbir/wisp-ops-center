import { LinksClient } from "./Client";
import { PageHeader } from "@/components/PageHeader";

export default function LinksPage() {
  return (
    <div>
      <PageHeader
        title="Linkler"
        subtitle="PTP / PTMP hat envanteri. Sinyal/SNR/kapasite metrikleri Faz 3+ ile gelecek."
      />
      <LinksClient />
    </div>
  );
}
