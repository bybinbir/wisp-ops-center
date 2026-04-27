import { ScheduledChecksClient } from "./Client";
import { PageHeader } from "@/components/PageHeader";

export default function ScheduledChecksPage() {
  return (
    <div>
      <PageHeader
        title="Planlı Kontroller"
        subtitle="Zamanlanmış MikroTik / Mimosa salt-okuma poll'ları + güvenli AP→Client testleri."
      />
      <ScheduledChecksClient />
    </div>
  );
}
