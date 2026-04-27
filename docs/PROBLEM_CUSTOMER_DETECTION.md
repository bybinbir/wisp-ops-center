# Problem Customer Detection (Faz 6)

> "Bugün ağda kime müdahale etmeliyim?" sorusunun veri sürücüsü cevabı.
> Bu doküman **Sorunlu Müşteriler** listesinin nasıl türetildiğini ve
> hangi API uçlarının onu beslediğini anlatır.

## Akış

```
telemetry & ap_client_test_results
        │
        ▼
internal/scoring/Hydrator.HydrateCustomer
        │  → Inputs (RSSI/SNR/CCQ/loss/lat/jitter, AP peer-set, trend)
        ▼
internal/scoring/Engine.ScoreCustomer
        │  → Result (score, severity, diagnosis, action, reasons, is_stale)
        ▼
Repository.SaveCustomerScore   ── customer_signal_scores satırı
                              ── customers.last_signal_* cache update
```

Sorunlu müşteri listesi `customer_signal_scores`'tan **müşteri başına en
güncel** satır seçilerek oluşur (`SELECT … ORDER BY calculated_at DESC LIMIT 1`).

## API Uçları

### `GET /api/v1/customers-with-issues`

Sorunlu Müşteriler tablosunu besler. Filtreler:

| Query | Açıklama |
|---|---|
| `severity`     | `critical` veya `warning`. Boşsa ikisi de döner. |
| `diagnosis`    | 12 tanıdan biri ile filtreler. |
| `tower_id`     | Tek kule altındaki müşteriler. |
| `ap_device_id` | Tek AP altındaki müşteriler. |
| `stale=true`   | Yalnız `is_stale=true` kayıtlar. |
| `limit`/`offset` | Sayfalama (1..500, default 100). |

Yanıt sıralaması:

1. `severity` (`critical` → `warning`)
2. `score` artan
3. `calculated_at` azalan

### `POST /api/v1/scoring/run`

Toplu skor üretimi. Body:

```json
{ "customer_ids": ["…"], "all_customers": false, "max_customers": 200 }
```

`all_customers=true` ise `customers.status='active'` üzerinde sınırlı
(`max_customers`, default 200) skor üretilir. Yanıtta `processed` ve
`errors` listesi döner.

### `POST /api/v1/customers/{id}/calculate-score`

Tek müşteri için hydrate + skor + persist. Yanıt:

```json
{ "data": { "id": "uuid", "score": 71, "severity": "warning",
  "diagnosis": "weak_customer_signal", "recommended_action": "check_cpe_alignment",
  "reasons": ["…"], "calculated_at": "…" } }
```

### `GET /api/v1/customers/{id}/signal-score`

En son skor satırı. Hiç skor yoksa `404 not_found`.

### `GET /api/v1/customers/{id}/signal-history?limit=50`

Geçmiş skor satırları (en yeniden eskiye).

### `GET /api/v1/devices/{id}/ap-health`

AP cihaz seviyesi `ap_health_scores` son satırı.

### `GET /api/v1/towers/{id}/risk-score`

Kule risk `tower_risk_scores` son satırı.

## Sınıflandırma Kuralları

Detay için `docs/CUSTOMER_SIGNAL_SCORING.md`. Özet:

- Tek müşteri sinyali kötü, AP peer'leri sağlıklıysa →
  **possible_cpe_alignment_issue** veya **weak_customer_signal**.
- AP altındaki critical müşteri oranı `ap_degradation_customer_ratio_critical`
  üstüne çıkmışsa → **ap_wide_interference**, AP'nin kendisi suçlu.
- PtP kapasite oranı ≥ 0.85 → **ptp_link_degradation**.

## "Veri yetersiz" davranışı

Aşağıdaki durumların hiçbirinde fake skor üretilmez:

- RSSI/SNR/test alanları tümüyle nil → `severity=unknown`,
  `diagnosis=data_insufficient`.
- En son veri `stale_data_minutes`'dan eski → `severity=warning`,
  `diagnosis=stale_data` (skor hesaplaması yine yapılır ama `is_stale=true`
  bayrağı eklenir).

UI: `data_insufficient` durumu Sorunlu Müşteriler listesine düşmez (default
filtre `severity IN ('warning','critical')`); ayrı kart olarak Dashboard'da
sayılır.

## Frontend

- `apps/web/src/app/musteriler/page.tsx` — Sorunlu Müşteriler tablosu
- `apps/web/src/app/musteriler/[id]/page.tsx` — müşteri detay
- `apps/web/src/app/page.tsx` — Operasyon Paneli kartları

UI tüm değerleri API'den okur, hiçbir simülasyon yoktur. API 503
döndürürse banner gösterir, fake satır basmaz.
