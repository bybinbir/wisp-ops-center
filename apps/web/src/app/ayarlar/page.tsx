import { CredentialsClient } from "./Client";
import { PageHeader } from "@/components/PageHeader";

export default function SettingsPage() {
  return (
    <div>
      <PageHeader
        title="Ayarlar"
        subtitle="Kimlik bilgileri, eşikler ve bildirim ayarları."
      />
      <CredentialsClient />
    </div>
  );
}
