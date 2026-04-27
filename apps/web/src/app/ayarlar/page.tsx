import { CredentialsClient } from "./Client";
import { ScoringThresholdsClient } from "./ScoringThresholds";
import { PageHeader } from "@/components/PageHeader";

export default function SettingsPage() {
  return (
    <div>
      <PageHeader
        title="Ayarlar"
        subtitle="Kimlik bilgileri, skor eşikleri ve bildirim ayarları."
      />
      <CredentialsClient />
      <ScoringThresholdsClient />
    </div>
  );
}
