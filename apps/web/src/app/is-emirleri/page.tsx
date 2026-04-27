import { PageHeader } from "@/components/PageHeader";
import { SkeletonNotice } from "@/components/SkeletonNotice";

export default function WorkOrdersPage() {
  return (
    <div>
      <PageHeader
        title="İş Emirleri"
        subtitle="Saha ekibine atanan işlerin takibi."
      />
      <SkeletonNotice>
        İş emri akışı Faz 7'de tasarlanacak. Şu an yalnızca veritabanı
        tablosu mevcut.
      </SkeletonNotice>

      <table>
        <thead>
          <tr>
            <th>Başlık</th>
            <th>Müşteri / Cihaz</th>
            <th>Öncelik</th>
            <th>Atanan</th>
            <th>Durum</th>
            <th>Oluşturuldu</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <td colSpan={6} className="empty">
              İş emri yok.
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  );
}
