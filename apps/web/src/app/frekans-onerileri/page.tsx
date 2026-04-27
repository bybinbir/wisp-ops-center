import { PageHeader } from "@/components/PageHeader";
import { SkeletonNotice } from "@/components/SkeletonNotice";

export default function FrequencyRecommendationsPage() {
  return (
    <div>
      <PageHeader
        title="Frekans Önerileri"
        subtitle="Öneri motorunun ürettiği aday değişiklikler. Hiçbiri otomatik uygulanmaz."
      />
      <SkeletonNotice>
        Öneri motoru Faz 8'de aktif olacak. Uygulama (apply + rollback)
        yalnızca Faz 9'da, capability + bakım penceresi + audit + dry-run
        kontrolleri geçtikten sonra mümkün olacak.
      </SkeletonNotice>

      <table>
        <thead>
          <tr>
            <th>Cihaz / Hat</th>
            <th>Mevcut Frekans</th>
            <th>Önerilen</th>
            <th>Risk</th>
            <th>Etkilenen Müşteri</th>
            <th>Beklenen Fayda</th>
            <th>Durum</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <td colSpan={7} className="empty">
              Henüz öneri yok.
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  );
}
