import { DevicesClient } from "./Client";
import { PageHeader } from "@/components/PageHeader";

export default function DevicesPage() {
  return (
    <div>
      <PageHeader
        title="Cihazlar"
        subtitle="MikroTik ve Mimosa envanteri. Vendor + rol + capability rozetleri."
      />
      <DevicesClient />
    </div>
  );
}
