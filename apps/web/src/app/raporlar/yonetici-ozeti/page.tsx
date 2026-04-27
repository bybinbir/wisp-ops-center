import { PageHeader } from "@/components/PageHeader";
import { ExecutiveSummaryClient } from "./Client";

export default function ExecutiveSummaryPage() {
  return (
    <div>
      <PageHeader
        title="Yönetici Özeti"
        subtitle="Severity dağılımı, en riskli AP/kuleler, açık iş emirleri ve trend."
      />
      <ExecutiveSummaryClient />
    </div>
  );
}
