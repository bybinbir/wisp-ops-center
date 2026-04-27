import { PageHeader } from "@/components/PageHeader";
import { WorkOrderDetailClient } from "./Client";

export default async function WorkOrderDetailPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return (
    <div>
      <PageHeader title="İş Emri Detayı" subtitle={"ID: " + id} />
      <WorkOrderDetailClient id={id} />
    </div>
  );
}
