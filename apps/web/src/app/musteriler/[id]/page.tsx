import { PageHeader } from "@/components/PageHeader";
import { CustomerDetailClient } from "./Client";

type Params = Promise<{ id: string }>;

export default async function CustomerDetailPage({
  params,
}: {
  params: Params;
}) {
  const { id } = await params;
  return (
    <div>
      <PageHeader
        title="Müşteri Detayı"
        subtitle="Skor geçmişi, evidence, AP-client testleri ve iş emri adayları."
      />
      <CustomerDetailClient customerId={id} />
    </div>
  );
}
