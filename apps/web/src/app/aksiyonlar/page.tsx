import { NetworkActionsClient } from "./Client";
import { PageHeader } from "@/components/PageHeader";

export default function NetworkActionsPage() {
  return (
    <div>
      <PageHeader
        title="Ağ Aksiyonları"
        subtitle="Faz 9 · read-only network actions. Frekans Kontrol şu anda kullanılabilir; mutasyon (frequency apply) yok."
      />
      <NetworkActionsClient />
    </div>
  );
}
