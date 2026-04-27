# AP-to-Client Test Engine (Tasarım)

Bu doküman **planlanan** bir modülün tasarımıdır. Faz 2'de yalnızca veri modeli, dokümantasyon ve UI placeholder'ı vardır. Faz 5'e kadar **hiçbir gerçek test** yürütülmez. Faz 5 ve sonrasında bile, aşağıdaki güvenlik kuralları sağlanmadan aktif olamaz.

## Amaç

AP tarafındaki altyapıdan müşteri/CPE cihazlarına yönelik **zamanlı, kontrollü** testler çalıştırarak operasyon ekibinin gözüne çarpmadan kötüye giden bağlantıları yakalamak: kötü sinyal, yüksek gecikme, paket kaybı, jitter, kapasite düşüşü.

Hedef: Sabah müşteri çağrısı gelmeden önce gece çalışan testlerin sonucu masada hazır olsun.

## Desteklenecek Test Tipleri

| Test | Risk | Açıklama |
|---|---|---|
| `ping_latency` | düşük | Tek hedefe ICMP ping; ortalama RTT, jitter ve packet loss raporu |
| `packet_loss` | düşük | Sürekli ping serisi; paket kaybı yüzdesi |
| `jitter` | düşük | RTT varyansı |
| `traceroute` | düşük | Yol görünürlüğü |
| `limited_throughput` | orta | Sınırlı bant genişlikli iperf benzeri ölçüm |
| `mikrotik_bandwidth_test` | yüksek | RouterOS native; **manuel onay zorunlu** |

## Risk Sınıflandırması

- **Düşük risk:** ölçülebilir trafik etkisi yok / minimal.
- **Orta risk:** kısa süreli kapasite kullanımı; saat ve trafik limiti zorunlu.
- **Yüksek risk:** üretim trafiğini etkileyebilir; yalnızca bakım penceresi + manuel onayla.

## Kesin Güvenlik Kuralları

1. **Kontrolsüz bandwidth-test yasaktır.** `mikrotik_bandwidth_test` tipinin `requires_manual_approval=TRUE` olması veritabanı CHECK constraint'i ile zorlanır (`ap_client_high_risk_requires_approval`).
2. **Her aktif test süre limiti taşımalıdır.** `max_duration_seconds` zorunlu, max 600 saniye.
3. **Her aktif test trafik limiti taşımalıdır.** `max_rate_mbps` opsiyonel ama orta+ riskli testler için zorunlu kullanılmalı.
4. **Bakım penceresi kontrolü.** Yüksek riskli testler `maintenance_window` cadence'ı dışında çalışmaz (Faz 9'da kuralı uygulayan worker eklenecek).
5. **Müşteri etki sınıflandırması.** Test tetiklenmeden önce etkilenecek müşteri sayısı tahmin edilir; eşiği aşıyorsa onay istenir.
6. **Audit zorunlu.** Her run `ap_client_test_runs` + `audit_logs` kaydı üretir; `outcome` alanı `success/failure/blocked` değerlerinden biri olur.
7. **Sonuç tarihçesi.** `ap_client_test_results` tablosu run başına detay tutar; geriye dönük analiz için saklanır.
8. **Cihaz konfigürasyonunu değiştirmez.** Bu motor yalnızca okuma/ölçüm yapar; herhangi bir konfig komutu çalıştırırsa Faz 9 controlled-apply akışına devredilir.
9. **Yüksek risk = manuel onay.** UI'dan bir operatör onaylamadan worker yüksek riskli profili çalıştırmaz.
10. **Varsayılan mod report-only.** `enabled=FALSE` varsayılan; aktivasyon bilinçli karardır.

## Veri Modeli

`migrations/000002_phase2_inventory_hardening.sql` üç tablo ekler:

```
ap_client_test_profiles (id, name, test_type, risk_level, max_duration_seconds,
                         max_rate_mbps, requires_manual_approval, enabled,
                         created_at, updated_at)

ap_client_test_runs     (id, profile_id, ap_device_id, scheduled_check_id,
                         status, started_at, finished_at, created_at)

ap_client_test_results  (id, run_id, customer_id, customer_device_id,
                         target_device_id, latency_ms, packet_loss_percent,
                         jitter_ms, throughput_mbps, diagnosis, risk_level,
                         created_at)
```

DB seviyesinde uygulanan kurallar:
- `ap_client_test_profiles.test_type` enum'u sınırlı.
- `risk_level` `low|medium|high`.
- `max_duration_seconds` 0 < d <= 600.
- `risk_level='high' AND requires_manual_approval=FALSE` kombinasyonu reddedilir.

## İlerideki Scheduler Seçenekleri

UI'dan tanımlanacak (Faz 5'te):

- Hangi AP?
- Hangi müşteri grubu? (tüm bağlı CPE'ler / yalnız riskli müşteriler / etiket bazlı)
- Test zamanı (cron veya bakım penceresi referansı)
- Test sıklığı
- Test tipi
- Maksimum süre / trafik
- Mod: rapor / uyarı / iş emri yarat

## İleride Üretilecek Rapor Çıktısı

Faz 7 raporu içine girecek alanlar:

- AP adı
- Test penceresi
- Test edilen müşteri sayısı
- Başarılı test sayısı
- Riskli müşteri sayısı
- Ortalama gecikme
- Ortalama paket kaybı
- Ortalama jitter
- Tanı (skor motoru ile birleştirilmiş)
- Önerilen aksiyon

## Hangi Fazda Gerçek Çalıştırılacak?

| Faz | Durum |
|---|---|
| Faz 2 | Veri modeli + UI placeholder + dokümantasyon. **Çalıştırma yok.** |
| Faz 3 | MikroTik salt-okuma poll'u; AP / CPE telemetri akışı. Test motoru hâlâ kapalı. |
| Faz 4 | Mimosa salt-okuma poll'u. Test motoru hâlâ kapalı. |
| Faz 5 | Scheduler + bakım penceresi modeli + **düşük riskli** ping/packet_loss/jitter testleri. |
| Faz 6 | Skor motoru sonuçlarına entegrasyon. |
| Faz 8/9 | Yüksek riskli testler (manuel onay + bakım penceresi + audit + rollback testi). |
