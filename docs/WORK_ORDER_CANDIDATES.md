# Work Order Candidates (Faz 6)

> Skor motorunun ürettiği iş emri **adaylarıdır**. Otomatik dispatch yoktur;
> bir operatörün gözden geçirip onaylaması gerekir. Bu kasıtlıdır:
> WISP Ops Center, müşteriye veya cihaza otomatik müdahale etmez.

## Tablo

`work_order_candidates`:

| Kolon | Anlam |
|---|---|
| `id`                  | UUID |
| `customer_id`         | İlgili müşteri (FK customers, ON DELETE SET NULL) |
| `ap_device_id`        | İlgili AP (FK devices) |
| `tower_id`            | İlgili kule (FK towers) |
| `source_score_id`     | Aday'ı doğuran `customer_signal_scores` satırı |
| `diagnosis`           | 12 tanı kategorisinden biri |
| `recommended_action`  | 10 aksiyon kategorisinden biri |
| `severity`            | `warning` / `critical` (yalnızca bu ikisi) |
| `reasons`             | jsonb — skor motorunun gerekçe satırları |
| `status`              | `open` / `dismissed` / `promoted` |
| `notes`               | Operatör notu (opsiyonel) |
| `promoted_work_order_id` | Faz 7'de gerçek iş emrine bağlanır |
| `created_at` / `updated_at` | timestamptz |

## API Uçları

### `POST /api/v1/customers/{id}/create-work-order-from-score`

Müşterinin **en son** skorundan aday üretir.

Kurallar:

- Skor severity'si `warning` veya `critical` değilse `422
  score_severity_not_actionable` döner.
- Aynı müşteri+tanı için status='open' aday halihazırda varsa **yeni satır
  oluşturulmaz**, mevcut id `200 OK` ile `duplicate=true` bayrağıyla döner.
  Bu davranış `internal/scoring/repository.go::CreateWorkOrderCandidate`
  içinde tanımlıdır.
- Tüm sonuçlar audit'e `work_order_candidate.created` eylemiyle yazılır
  (`duplicate` bayrağı metadata'da görünür).

Yanıt:

```json
{ "data": {
    "id": "uuid",
    "customer_id": "uuid",
    "diagnosis": "weak_customer_signal",
    "recommended_action": "check_cpe_alignment",
    "severity": "warning",
    "status": "open",
    "duplicate": false
} }
```

### `GET /api/v1/work-order-candidates?status=open|dismissed|promoted`

Default `status=open`. En yeni 100 aday severity (critical→warning) ve
`created_at DESC` sırasına göre listelenir.

### `PATCH /api/v1/work-order-candidates/{id}`

```json
{ "status": "dismissed", "notes": "Müşteri arandı, geçici sorun" }
```

`status` yalnız `open` / `dismissed` / `promoted` olabilir. Bilinmeyen
status `400` döner. `not_found` `ErrNotFound` ile maplenir.

## Faz 7 Yol Haritası

`promoted_work_order_id` Faz 7'de **gerçek iş emirleri** tablosuna referans
verir. Bu fazda yalnız aday vardır; otomatik dispatch yok, atama yok,
müşteriye iletişim gönderimi yok.

## Audit ve Güvenlik

| Olay | Audit eylemi |
|---|---|
| Aday oluşturuldu / duplicate bulundu | `work_order_candidate.created` |
| Status değişti                       | (Faz 7 — şu an handler düz update yapıyor) |

- Aday oluşturma istemcisi `Authorization: Bearer <WISP_API_TOKEN>` ile
  kimlik doğrulamalıdır.
- Duplicate guard, sıkışmış sahada aynı tanı için spam aday üretmeyi engeller.
- Hassas ham telemetri değerleri (`reasons` jsonb içine) yazılmadan önce
  skor motoru tarafından operatör-okuyabilir cümlelere çevrilir.
