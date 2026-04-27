# AP-to-Client Test Runtime (Faz 5)

Bu doküman Faz 5'te aktif edilen güvenli AP→Client testlerinin **çalışma yolu** sözleşmesini anlatır.

## Çalışma Yeri

**Bu testler wisp-ops-center sunucusundan çalıştırılır.** Hedef IP'ye doğru `ping` / `traceroute` komutu ile sunucu→hedef erişilebilirlik ölçülür.

UI'da operatör netliği için "AP→Client" denilse de gerçek AP-üzerinde komut yürütme Faz 5'in dışında kaldı. AP-side execution güvenli allowlistli bir model gerektirir; Faz 9 controlled-apply altyapısı oturduğunda yeniden değerlendirilir.

## Aktif Test Tipleri

`internal/apclienttest`:

| `TestType` | Açıklama | Çıktı |
|---|---|---|
| `ping_latency` | ICMP ping serisi | latency_min/avg/max, jitter (mdev), loss% |
| `packet_loss` | Aynı veri yolu, loss%'a odak | loss% |
| `jitter` | Aynı veri yolu, jitter'a odak | jitter (mdev) |
| `traceroute` | Bounded hop sayımı | hop_count |

## Disabled Test Tipleri

| Tip | Sebep |
|---|---|
| `limited_throughput` | Müşteri trafiğini etkileyebilir; Faz 5'te kapalı |
| `mikrotik_bandwidth_test` | Yüksek riskli; manuel onay + bakım penceresi + audit + rollback (Faz 9) gerekir |

## Sınırlar (Hard Bounds)

- `count`: 1..20 (default 5)
- `timeout` per packet: 50ms..5s (default 1500ms)
- `max_duration` (toplam): 1s..60s (default 30s)
- `target_ip`: net.ParseIP başarılı olmalı; aksi halde `ErrInvalidTarget`

Bunlardan biri ihlal edilirse `Validate()` ilgili sentinel hatayla durur ve runner sonucu `Status="blocked"` + sınıflandırılmış `error_code` ile döndürür.

## Diagnosis Kategorileri

`classifyPing()` çıktısı:
- `healthy` — düşük loss + makul gecikme + stabil jitter.
- `high_latency` — avg >= 100 ms.
- `packet_loss` — loss% >= 5.
- `unstable_jitter` — mdev >= 30 ms.
- `unreachable` — loss% = 100.
- `route_issue` — traceroute hop_count = 0.
- `data_insufficient` — hesap yapılacak yeterli ölçüm yok.

## Storage

`ap_client_test_results` tablosuna her run bir satır yazar:
- `test_type`, `target_ip`, `latency_min/avg/max_ms`, `packet_loss_percent`, `jitter_ms`, `hop_count`, `diagnosis`, `risk_level`, `status` (success/partial/failed/blocked), `error_code`, `error_message` (sanitized), `created_at`.

CHECK constraint: `test_type` enum'u + `status` enum'u DB seviyesinde zorlanır.

## Yapılmaz

- Cihazda config değişikliği.
- AP'de RouterOS/SNMP komutu.
- Müşteriye ücret veya hizmet etkileyen aksiyon.
- Sınırsız trafik testi.
