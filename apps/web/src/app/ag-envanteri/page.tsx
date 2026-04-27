import { NetworkInventoryClient } from "./Client";
import { PageHeader } from "@/components/PageHeader";

export default function NetworkInventoryPage() {
  return (
    <div>
      <PageHeader
        title="Ağ Envanteri"
        subtitle="MikroTik Dude SSH discovery — read-only. AP, link, bridge, CPE ve bilinmeyen cihazlar otomatik sınıflandırılır."
      />
      <NetworkInventoryClient />
    </div>
  );
}
