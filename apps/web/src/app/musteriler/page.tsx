import { PageHeader } from "@/components/PageHeader";

export default function CustomersPage() {
  return (
    <div>
      <PageHeader
        title="Sorunlu Müşteriler"
        subtitle="Skor motoru kötü sinyal, kopukluk veya CPE alignment sorunu olan müşterileri buraya düşürür."
      />
      <div className="banner">
        <strong>Faz 2 durumu:</strong> Müşteri envanteri{" "}
        <span className="kbd">/api/v1/customers</span> üzerinden CRUD'a hazır.
        Gerçek sinyal skoru Faz 6'da telemetri verisiyle üretilecek; bu sayfa
        skor motoru aktifleşene kadar yalnızca veri tabanı kayıtlarını
        sıralayacaktır.
      </div>
      <table>
        <thead>
          <tr>
            <th>Müşteri</th>
            <th>Bölge / Kule</th>
            <th>AP / Link</th>
            <th>Vendor</th>
            <th>RSSI</th>
            <th>SNR</th>
            <th>Kalite</th>
            <th>Tanı</th>
            <th>Önerilen Aksiyon</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <td colSpan={9} className="empty">
              Skor motoru aktifleşene kadar bu sayfa boş kalacak. Müşterileri{" "}
              <em>Cihazlar / Kuleler</em> ile birlikte ekleyebilirsiniz.
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  );
}
