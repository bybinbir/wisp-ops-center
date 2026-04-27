import { PageHeader } from "@/components/PageHeader";
import { WorkOrdersClient } from "./Client";

export default function WorkOrdersPage() {
  return (
    <div>
      <PageHeader
        title="İş Emirleri"
        subtitle="Saha ekibine atanan gerçek iş emirleri — durum, atama, ETA ve geçmiş."
      />
      <WorkOrdersClient />
    </div>
  );
}
