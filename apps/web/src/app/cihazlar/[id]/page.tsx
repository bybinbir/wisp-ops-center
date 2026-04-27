import { DeviceDetailClient } from "./Client";
import { PageHeader } from "@/components/PageHeader";

export default function DeviceDetailPage({
  params
}: {
  params: { id: string };
}) {
  return (
    <div>
      <PageHeader
        title="Cihaz Detayı"
        subtitle="Salt-okuma probe + poll, son sağlık, arayüzler, kablosuz istemciler ve poll geçmişi."
      />
      <DeviceDetailClient deviceId={params.id} />
    </div>
  );
}
