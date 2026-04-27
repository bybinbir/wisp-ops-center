# Veri Modeli

`migrations/` altındaki SQL dosyaları sıralı uygulanır. Bu doküman tabloların **niye** öyle olduğunu anlatır.

## Migration Sırası

1. `000001_initial_schema.sql` — Faz 1 baseline (17 tablo, koruyucu CHECK).
2. `000002_phase2_inventory_hardening.sql` — Faz 2:
   - `devices` envanter alanları (`os_version`, `firmware_version`, `status`, `tags`, `notes`, `deleted_at`).
   - sites/towers/customers/links indeksleri.
   - `credential_profiles.auth_type` + `port` + CHECK enum'u.
   - `audit_logs(actor, at DESC)` indeksi.
   - `wispops_app` rolünden UPDATE/DELETE `REVOKE` (rol mevcutsa).
   - Üç yeni tablo: `ap_client_test_profiles`, `ap_client_test_runs`, `ap_client_test_results`.

## Tasarım İlkeleri

1. **Hiçbir tabloda ham parola yok.** `credential_profiles.secret_ciphertext` BYTEA, AES-GCM ile şifrelenir.
2. **Telemetri zaman serisidir.** `(entity_id, collected_at DESC)` indeksi.
3. **Audit append-only.** `wispops_app` rolünden UPDATE/DELETE `REVOKE` ile alınır.
4. **Faz koruması DB'den geçer.** `scheduled_checks.phase1_no_apply` Faz 1+2'de `controlled_apply` modunu reddeder; `ap_client_test_profiles.ap_client_high_risk_requires_approval` yüksek riskli onaysız testi reddeder.

## Tablo Özetleri

### Envanter
- `sites` — POP / lokasyon.
- `towers` — Site altında fiziksel kule.
- `devices` — Vendor + rol + ip + status + tags + soft delete (`deleted_at`).
- `device_capabilities` — 1:1 cihaz başına yetenek bayrakları.
- `links` / `link_slaves` — PTP/PTMP master + slave eşlemesi.
- `customers` / `customer_devices` — Aboneler ve onların CPE/AP eşlemesi.

### Kimlik
- `credential_profiles` — `auth_type ∈ {routeros_api_ssl, ssh, snmp_v2, snmp_v3, mimosa_snmp, vendor_api}`, AES-GCM şifreli secret, `secret_key_id` (anahtar fingerprint).
- `device_credentials` — Profil ↔ cihaz eşlemesi (transport bazında).

### Telemetri (Faz 3+ doldurur)
- `telemetry_snapshots` — CPU/mem/temp/uptime.
- `wireless_metrics` — RSSI/SNR/CCQ/Tx-Rx.
- `health_scores` — Skor motoru çıktısı.

### Operasyon
- `scheduled_checks` — Tanımlı kontrol; `phase1_no_apply` CHECK aktif.
- `job_runs` — Her yürütmenin özeti.
- `frequency_recommendations` — Öneri durum makinesi.
- `reports`, `work_orders`.
- `audit_logs` — Append-only.

### AP-to-Client Test (Faz 5+ aktif olur)
- `ap_client_test_profiles` — Test tipi + risk + süre/hız limiti + manuel onay zorunluluğu.
- `ap_client_test_runs` — Her tetikleme; status `planned/running/done/failed/blocked/cancelled`.
- `ap_client_test_results` — Müşteri başına ölçüm (latency_ms, packet_loss_percent, jitter_ms, throughput_mbps, diagnosis, risk_level).

## İndeksleme Notları

Yüksek hacim beklenen tablolarda:
- `telemetry_snapshots(device_id, collected_at DESC)`
- `wireless_metrics(device_id, collected_at DESC)`
- `health_scores(customer_id, computed_at DESC)`
- `audit_logs(at DESC)` ve `audit_logs(actor, at DESC)`

İlk yıl sonunda partition (TimescaleDB veya range partition) değerlendirilir.

## Migration Politikası

- Yeni dosyalar `000003_*.sql` şeklinde sıralı.
- Migration runner her dosya için SHA-256 checksum saklar; değişen geçmiş dosya `MigrationDriftError` ile durdurulur.
- Geri alma scripti (`down`) Faz 3'ten itibaren her değişiklikte yazılır.

## Soft Delete

`devices.deleted_at` ile soft delete uygulanır. Bu sayede `audit_logs` kayıtları tutulurken cihaz görünmez olur. Listelemeler `WHERE deleted_at IS NULL` kullanır. Hard delete operasyonel olarak yasaktır (audit zincirini bozar).
