# Skorlama Modeli

`internal/scoring` paketinin uyguladığı deterministik kural setidir. ML yoktur; eşikler dokümandadır ve testle korunur.

## Müşteri Kablosuz Sağlık Skoru (0–100)

100 = sağlıklı. 0 = ölçülmedi/erişilemedi.

### Eşik Tablosu

| Metrik | Eşik | Skor düşüşü | Sebep notu |
|---|---|---|---|
| RSSI | ≤ −85 dBm | −40 | "RSSI çok zayıf" |
| RSSI | ≤ −78 dBm | −25 | "RSSI zayıf" |
| RSSI | ≤ −72 dBm | −10 | "RSSI sınırda" |
| SNR  | < 15 dB  | −35 | "SNR kritik" |
| SNR  | < 22 dB  | −15 | "SNR düşük" |
| SNR  | < 28 dB  | −5  | "SNR sınırda" |
| CCQ  | < 60     | −10 | "CCQ düşük" |
| Disconnect (24sa) | ≥ 5 | −10 | "Kopukluk yüksek" |
| 7g sinyal trendi  | < −0.5 dB/gün | −5 | "Düşüş trendi" |
| AP geneli kötüleşme | > 0.5 | −10 | "AP genelinde" |

Sonuç [0,100] aralığına `clamp` edilir.

### Tanı Sınıflandırması

Skor + öncelikli sinyallerden tanı seçilir (çakışma durumunda yukarıdan aşağıya öncelik):

1. **AP geneli parazit** — `APWideDegradation > 0.6`
2. **PTP hat kötüleşmesi** — `LinkCapacityRatio > 0.85`
3. **Zayıf müşteri sinyali** — RSSI ≤ −85
4. **Olası CPE alignment sorunu** — SNR < 15
5. **Frekans/kanal riski** — SNR < 22
6. **Sağlıklı** — başka anomali yok
7. **Veri yetersiz** — RSSI ve SNR ikisi de yok

### Önerilen Aksiyon Eşlemesi

| Tanı | Aksiyon |
|---|---|
| Sağlıklı | Aksiyon gerekmiyor |
| Zayıf müşteri sinyali | Saha ekibi: CPE konum/yön kontrolü |
| Olası CPE alignment | CPE alignment ölçümü |
| AP geneli parazit | AP'ye scan, gerekirse kanal değişikliği önerisi |
| PTP hat kötüleşmesi | Hat performans incelemesi, kapasite/parazit |
| Frekans/kanal riski | Kanal/parazit analizi, frekans önerisi |
| Cihaz çevrimdışı | Cihaz erişimini kontrol et |
| Veri yetersiz | Daha fazla telemetri topla |

## Gerekçeler (`reasons[]`)

Skor düşüşüne yol açan her eşik insan-okur bir cümle olarak `reasons` listesine eklenir. UI bunları "Tanı" sütununun altında gösterir.

## Test Politikası

`internal/scoring/scoring_test.go` Faz 1'de en az şu vakaları kapsar:

- Sağlıklı yüksek RSSI/SNR → skor ≥ 90, tanı `healthy`
- Boş girdi → tanı `data_insufficient`
- RSSI ≤ −85 + SNR ≈ 18 → skor < 80, tanı `healthy` değil
- AP geneli kötüleşme baskın → tanı `ap_wide_interference`

## Faz 2+ Genişletmeleri (yapılmadı)

- Eşikleri DB'den okuma (`thresholds` tablosu).
- Tower bazlı normalizasyon (kuru/sis sezonu).
- Vendor bazlı CCQ normalizasyonu (Mimosa CCQ döndürmüyor).
