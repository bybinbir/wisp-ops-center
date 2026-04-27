# Customer Signal Scoring (Faz 6)

> Bu doküman, müşteri kalite skorunun **nasıl** hesaplandığını ve **hangi
> kuralların** kararı verdiğini anlatır. Skor motoru deterministik kural
> tabanlıdır; **ML kullanılmaz**, **rastgelelik yoktur**, **fake skor
> üretilmez**.

## Motor: `internal/scoring`

Dosya yapısı:

| Dosya | Sorumluluk |
|---|---|
| `types.go`         | `Inputs`, `Result`, `Severity`, `Diagnosis`, `Action` tipleri |
| `thresholds.go`    | Varsayılan eşikler + DB override + key/range doğrulama |
| `engine.go`        | `Engine.ScoreCustomer` ana skor üretici |
| `diagnosis.go`     | 12 tanı kategorisinin sıralı karar ağacı |
| `actions.go`       | Tanı → önerilen aksiyon eşlemesi (10 aksiyon) |
| `ap_degradation.go`| AP/Link/Tower agregasyonu, peer-group analizi |
| `trend.go`         | 7 günlük sinyal trend regresyonu |
| `hydrator.go`      | Müşteri bağlamı → Inputs (telemetri + AP-client testleri) |
| `repository.go`    | pgx tabanlı persistence + Sorunlu Müşteriler sorgusu |

## Skor Aralığı

- `score`: `0..100`, **yüksek = iyi**.
- `severity`:
  - `score >= severity_healthy_at` (varsayılan **80**) → `healthy`
  - `score >= severity_warning_at`  (varsayılan **50**) → `warning`
  - aksi → `critical`
  - `Inputs` tamamen boşsa → `unknown` + `data_insufficient`

## Penalty Tablosu (varsayılan)

Skor 100'den başlar, ilgili koşullar tutarsa puan düşer; alt sınır 0'a clamp.

| Koşul | Critical penalty | Warning penalty |
|---|---|---|
| RSSI ≤ critical_dbm (-80)            | -40 | — |
| RSSI ≤ warning_dbm (-70)             | —   | -18 |
| SNR < critical_db (15)               | -30 | — |
| SNR < warning_db (25)                | —   | -12 |
| CCQ < critical_pct (50)              | -12 | — |
| CCQ < warning_pct (75)               | —   | -5 |
| Packet loss ≥ critical (5%)          | -25 | — |
| Packet loss ≥ warning (2%)           | —   | -10 |
| Latency ≥ critical (100 ms)          | -18 | — |
| Latency ≥ warning (50 ms)            | —   | -8 |
| Jitter ≥ critical (30 ms)            | -12 | — |
| Jitter ≥ warning (15 ms)             | —   | -5 |
| Disconnects/24h ≥ 5                  | -8  | — |
| 7d sinyal trendi ≤ -0.5 dB/gün       | -5  | — |
| AP-wide degraded ratio ≥ 0.40        | -12 | — |
| AP-wide degraded ratio ≥ 0.25        | —   | -5 |
| Link kapasite oranı ≥ 0.85           | -6  | — |

Her koşulun gerekçesi `Result.Reasons` listesine eklenir. UI bunu açıklama
olarak göstermek için kullanır.

## Tanı (Diagnosis) Karar Ağacı

`Thresholds.Classify(in, now)` aşağıdaki sırayla değerlendirir; **ilk eşleşen**
kategori atanır:

1. `data_insufficient` — RSSI/SNR/test verisi tamamen yoksa.
2. `stale_data` — En son örnek/test `stale_data_minutes`'dan eski.
3. `device_offline` — Son test başarısız + telemetri yok.
4. `ap_wide_interference` — Aynı AP altındaki kritik müşteri oranı
   `ap_degradation_customer_ratio_critical` üstü.
5. `ptp_link_degradation` — Link kapasite oranı ≥ 0.85.
6. `weak_customer_signal` — RSSI ≤ critical (ardından warning).
7. `possible_cpe_alignment_issue` — SNR < critical.
8. `frequency_channel_risk` — SNR < warning (ama critical değil).
9. `high_latency` / `packet_loss` / `unstable_jitter` — test
   sonuçları sırasıyla critical eşik üstü.
10. Aksi halde `healthy`.

## Önerilen Aksiyon (Action)

`ActionFor(diagnosis, severity)` 10 kategoriden birini seçer:

| Tanı | Severity | Aksiyon |
|---|---|---|
| healthy                       | —          | no_action |
| weak_customer_signal          | critical   | schedule_field_visit |
| weak_customer_signal          | warning    | check_cpe_alignment |
| possible_cpe_alignment_issue  | —          | check_cpe_alignment |
| ap_wide_interference          | critical   | escalate_network_ops |
| ap_wide_interference          | warning    | check_ap_interference |
| ptp_link_degradation          | —          | check_ptp_backhaul |
| frequency_channel_risk        | —          | review_frequency_plan |
| high_latency                  | critical   | check_ptp_backhaul |
| high_latency                  | warning    | monitor |
| packet_loss                   | critical   | schedule_field_visit |
| packet_loss                   | warning    | check_customer_cable |
| unstable_jitter               | —          | monitor |
| device_offline                | —          | verify_power_or_ethernet |
| stale_data                    | —          | monitor |
| data_insufficient             | —          | monitor |

## Threshold Override

Veritabanı tablosu `scoring_thresholds (key, value, description, updated_at,
updated_by)`. Migration 000006 varsayılanları seed eder. API:

- `GET  /api/v1/scoring-thresholds`
- `PATCH /api/v1/scoring-thresholds` (body: `{"updates": {"key": value, ...}, "by": "user"}`)

Validasyon: bilinmeyen anahtar `unknown_key` ile reddedilir; bilinen anahtar
için değer `[min,max]` aralığı dışındaysa `out_of_range` ile reddedilir.
Aralıklar `internal/scoring/thresholds.go::thresholdSpecs` içinde tanımlıdır.
Her başarılı PATCH `audit_logs`'a `scoring_threshold.updated` eylemiyle yazılır.

## Persistans

Bir hesaplama her zaman **yeni bir satır** ekler — geçmiş analizi için
korunur.

- `customer_signal_scores` — müşteri başına skor satırı + cache `customers.last_signal_*`
- `ap_health_scores` — AP cihaz başına özet skor
- `tower_risk_scores` — kule başına risk skoru
- `work_order_candidates` — sadece warning/critical skorlar üretebilir

## Güvenlik Sınırları

- Skor motoru cihaza **YAZMAZ**, sadece tablo okur.
- Skor değerleri yalnız persisted telemetriden gelir; eksikse
  `data_insufficient`.
- `last_signal_*` cache alanları ileride yetki kontrolü için ayrılmış olabilir.
- Audit eylemleri:
  - `scoring_threshold.updated`
  - `work_order_candidate.created`
- Hassas alan üretilmez; eşik anahtarları sayısal/etiket türünden ibarettir.
