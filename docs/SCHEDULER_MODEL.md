# Scheduler Modeli

`internal/scheduler` paketi zamanlanmış kontrolün sözleşmesini tanımlar. Faz 1+2'de gerçek kuyruk YOKTUR; tip ve durum makinesi sabittir.

## İş Tipleri

| `JobType` | Açıklama |
|---|---|
| `daily_network_check` | Tüm ağı kapsar, gece çalışır |
| `weekly_network_report` | Haftalık özet |
| `tower_health_check` | Tek bir kule için derin kontrol |
| `customer_signal_check` | Belirli müşteri kümesi için skor güncellemesi |
| `mikrotik_readonly_poll` | Sadece okuma — RouterOS (Faz 3) |
| `mimosa_readonly_poll` | Sadece okuma — Mimosa (Faz 4) |
| `frequency_recommendation_analysis` | Öneri motoru (Faz 8) |
| `ap_client_test_run` | AP→Client güvenli test (Faz 5; düşük riskli ile başlar) |

## Cadence Seçenekleri

- `once` — tek seferlik
- `daily` — her gün
- `weekly` — haftada bir
- `monthly` — ayda bir
- `maintenance_window` — yalnızca bakım penceresinde (Faz 9)

## Aksiyon Modları

| Mod | Davranış | Faz 1+2 |
|---|---|---|
| `report_only` | Yalnızca okuma + rapor | aktif |
| `recommend_only` | Rapor + öneri | aktif |
| `manual_approval` | Öneri operatöre, onay beklenir | aktif (uygulama yok) |
| `controlled_apply` | Bakım penceresinde uygula | **DB seviyesinde reddedilir** |

## Scope (JSON)

```json
{
  "site_ids":   ["..."],
  "tower_ids":  ["..."],
  "device_ids": ["..."],
  "link_ids":   ["..."]
}
```

Boş alan "tümü" anlamına gelir.

## Durum Makinesi (job_runs)

```
pending → running ─┬─→ success
                   ├─→ failed
                   └─→ blocked   (capability/safety reddi)
```

`blocked` ile `failed` ayrımı önemlidir: `blocked` "yapmadık çünkü güvenli değil"; `failed` "denedik ama hata aldık".

## AP-to-Client Test Entegrasyonu

Faz 5'te `ap_client_test_run` iş tipi şu kuralları uygular:

- Profil `enabled=false` ise iş `blocked`.
- `risk_level='high' AND requires_manual_approval=true` ise yalnızca onay verilmiş `ap_client_test_runs` satırı için handler çalışır.
- Bakım penceresi dışındaki yüksek riskli profil tetiklemeleri `blocked`.
- Sonuçlar `ap_client_test_results` tablosuna düşer; cihaz konfigürasyonu değişmez.

## Kuyruk Implementasyonu (Faz 5)

İlk implementasyon Asynq ile başlayacak. Worker registry zaten arayüz tabanlı (`internal/scheduler.JobType` + `apps/worker/internal.Registry`); değişim yalnızca runtime adapter ekler.

## Bakım Penceresi (Faz 9)

`maintenance_windows` tablosu Faz 9'da eklenecek. Worker run-time'da pencere içinde olunduğunu doğrular; pencere dışı tetiklenmiş yüksek riskli iş `blocked` ile durdurulur.
