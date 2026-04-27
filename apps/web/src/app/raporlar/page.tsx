import { PageHeader } from "@/components/PageHeader";
import { SkeletonNotice } from "@/components/SkeletonNotice";

export default function ReportsPage() {
  return (
    <div>
      <PageHeader
        title="Raporlar"
        subtitle="Günlük ve haftalık operasyon raporları."
      />
      <SkeletonNotice>
        Rapor üretimi Faz 7'de eklenecek. PDF/HTML çıktıları o aşamada
        servisten indirilebilir hale gelecek.
      </SkeletonNotice>

      <table>
        <thead>
          <tr>
            <th>Tip</th>
            <th>Dönem</th>
            <th>Özet</th>
            <th>Üretildi</th>
            <th>Dosya</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <td colSpan={5} className="empty">
              Henüz rapor yok.
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  );
}
