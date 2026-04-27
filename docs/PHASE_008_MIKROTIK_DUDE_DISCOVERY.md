# Faz 8 — MikroTik Dude SSH Discovery + Network Inventory

**Durum:** read-only discovery üretime hazır; controlled action framework iskelet.
**Bağımlı olduğu faz:** Faz 7 (work orders + reports).
**Branch:** `phase/008-mikrotik-dude-discovery`
**Migration:** `migrations/000008_mikrotik_dude_discovery.sql`

## 1. Amaç

MikroTik Dude / RouterOS cihazına SSH üzerinden bağlanıp ağdaki AP, link, bridge ve müşteri/CPE cihazlarını otomatik tarayıp envanteri WISP Ops Center dashboard'una akıtmak. Bu faz **yalnızca okuma** yapar; cihaza yazma, frekans değiştirme, reboot ve benzeri işlemler bu fazın kapsamı dışındadır.

## 2. Mimari

```
.env                ──► internal/config (DudeConfig)
                                     │
                                     ▼
apps/api  ──► internal/dude  ──► RouterOS via SSH
              │ client.go
              │ allowlist.go (read-only komut listesi)
              │ parser.go    (RouterOS print detail)
              │ classify.go  (heuristics → Category + Confidence)
              │ discovery.go (orchestrator)
              │
              └─► internal/networkinv (Repository)
                      │  network_devices, discovery_runs,
                      │  device_category_evidence, network_links
                      ▼
                  PostgreSQL  ─► /api/v1/network/* ─► /ag-envanteri (web)

internal/networkactions: action framework iskeleti (Faz 8: stub).
```

### Paketler

| Paket | Sorumluluk |
|---|---|
| `internal/config` | `DudeConfig` env mapping. Şifre asla repoda. |
| `internal/dude` | SSH client, allowlist, parser, classifier, discovery orchestrator. |
| `internal/networkinv` | `network_devices` + `discovery_runs` + `device_category_evidence` repository. Upsert + filter. |
| `internal/networkactions` | Sonraki fazlarda gelecek aksiyonlar için iskelet. Faz 8'de hiçbiri çalıştırılmaz. |
| `apps/api/internal/http/handlers_network.go` | 5 yeni endpoint + async discovery worker. |
| `apps/web/src/app/ag-envanteri/` | Dashboard sayfası. |

## 3. Env Setup

### Zorunlu değişkenler (`.env`)

```
MIKROTIK_DUDE_HOST=194.15.45.62
MIKROTIK_DUDE_PORT=22
MIKROTIK_DUDE_USERNAME=bariss
MIKROTIK_DUDE_PASSWORD=<runtime'da doldur, ASLA commit etme>
MIKROTIK_DUDE_TIMEOUT_MS=10000
MIKROTIK_DUDE_HOST_KEY_POLICY=trust_on_first_use
MIKROTIK_DUDE_HOST_KEY_FINGERPRINT=
```

`MIKROTIK_DUDE_PASSWORD` boş bırakılırsa `Configured() = false` döner, API `412 not_configured` ile cevap verir; SSH bağlantısı denenmez.

### Host-key policy

- `insecure_ignore`: TLS gibi davranmaz, fingerprint doğrulamaz. Sadece geliştirme.
- `trust_on_first_use` (varsayılan): İlk bağlantıda fingerprint Postgres `ssh_known_hosts` tablosuna yazılır; sonraki bağlantılarda eşleşme zorunlu.
- `pinned`: `MIKROTIK_DUDE_HOST_KEY_FINGERPRINT` (`SHA256:...`) zorunlu; eşleşmeyen fingerprint = `ErrHostKey`.

Politika `internal/adapters/ssh.EnforcePolicy` üzerinden Faz 6 ile aynı kod yolunu kullanır.

## 4. Güvenlik kuralları (Faz 8'de uygulanan)

1. **Şifre repoda asla yok.** `.env.example` `MIKROTIK_DUDE_PASSWORD=` boş bırakır. `.gitignore` `.env`, `.env.local`, `.secrets`, `*.pem`, `*.key` korur.
2. **Loglarda host gösterilir, şifre asla.** `dude.SanitizeMessage` `password=`, `secret=`, `token=` öncesini `[redacted]` ile keser ve `>320` byte'ı kapar.
3. **Allowlist enforcement.** `dude.EnsureAllowed` 18 read-only RouterOS komutu dışında her şeyi `ErrDisallowedCommand` ile reddeder. Test (`TestAllowlist_NoDestructiveCommands`) her allowlist girişinin `print|detail` ile bittiğini doğrular.
4. **Raw metadata sanitize.** `dude.SanitizeAttrs` `password|passwd|secret|community|key|token|bearer|auth` içeren her anahtarı `[redacted]` ile değiştirir; `network_devices.raw_metadata` JSONB bu temiz veriyi taşır.
5. **Correlation id.** Her discovery run kendi `dude-<hex>` id'sini üretir; SSH log satırları, audit log ve `discovery_runs.correlation_id` aynı id'yi paylaşır.
6. **Audit log.** Üç action: `network.dude.test_connection`, `network.dude.run.start`, `network.dude.run.finish`. Outcome + Subject + Metadata.

## 5. Discovery akışı

### Tetik

- Manuel: `POST /api/v1/network/discovery/mikrotik-dude/run` → 202 + `run_id`.
- Tek-anda-tek-koşu kuralı: `dudeRunMu` + `dudeRunActive` flag. Çakışan çağrılar `409 discovery_already_running`.

### Adımlar

1. `discovery_runs` satırı `running` durumda oluşturulur.
2. Goroutine içinde:
   1. SSH dial (`dude.Client.Dial`) — host-key policy uygulanır.
   2. `/dude/device/print/detail` çalıştırılır (primary).
   3. Hata olursa `/ip/neighbor/print/detail` fallback (secondary).
   4. `/system/identity/print` ile self-host kaydı eklenir.
   5. Her cihaz `Classify()` ile kategori + confidence + evidence alır.
   6. `dedupeDevices` MAC > IP > Name önceliğiyle birleştirir.
3. `Repository.UpsertDevices` cihazları persistleştirir; her run için `device_category_evidence` yenilenir.
4. `discovery_runs` satırı `succeeded | partial | failed` ile finalize edilir; tüm sayaçlar yazılır.

### Tolerans

- `dude_print` başarısız + `neighbor_print` başarılı → `partial` değil `succeeded`. (Çünkü kullanılabilir cihaz listesi alındı.)
- Persist hatası → `error_code=persist_failed`, parsed cihazlar yine de korunur.

## 6. Desteklenen kategoriler

| Category | Açıklama |
|---|---|
| `AP` | Access Point. Dude `type=ap`, `wireless-mode=ap-bridge`, `wAP/cAP/hAP` model ipucu, isim öneki `AP-`. |
| `BackhaulLink` | PtP / kule arası link. İsim önekleri `PTP-`, `LINK-`, `BH-`, `RB921` gibi modeller. |
| `Bridge` | Bridge interface dominant cihaz. İsim öneki `BR-`, interface-type=bridge. |
| `CPE` | Müşteri / abone cihazı. Dude `type=cpe`, `wireless-mode=station`, model `SXTsq`/`LDF`/`LHG`/`Groove`. |
| `Router` | Çekirdek/edge router. Dude `type=router`, model `CCR`, isim öneki `RTR-`/`CORE-`/`EDGE-`. |
| `Switch` | Switch. Dude `type=switch`, model `CSS`/`CRS`, isim öneki `SW-`. |
| `Unknown` | Hiçbir heuristic eşleşmedi. Confidence 0–5. |

Heuristic kararları `device_category_evidence` tablosuna her run sonrası yeniden yazılır; UI ileride "neden bu kategori?" gösterimini destekleyebilir.

## 7. API uçları

| Method + Path | Açıklama |
|---|---|
| `POST /api/v1/network/discovery/mikrotik-dude/test-connection` | Hızlı reachability + auth probe. `200` + `{reachable, identity, duration_ms}` veya hata kodu. |
| `POST /api/v1/network/discovery/mikrotik-dude/run` | Async discovery başlatır. `202` + `{run_id, correlation_id, status:"running"}`. Çakışmada `409`. |
| `GET /api/v1/network/discovery/runs` | Son 50 koşu (newest first). |
| `GET /api/v1/network/devices?category=&status=&unknown=&low_confidence=` | Filtreli liste + `summary` bloğu (cards için). |
| `GET /api/v1/network/devices/{id}` | Tek cihaz. |

Tümü `Authorization: Bearer <WISP_API_TOKEN>` middleware'inden geçer.

## 8. Veri modeli (özet)

- `discovery_runs` — run metadata + sayaçlar + commands_run.
- `network_devices` — envanter; partial unique index'ler MAC > (host,name) > name ile duplicate korur.
- `network_links` — backhaul/PtP iskelet (Faz 8 boş).
- `device_category_evidence` — heuristic+weight+reason per device per run.
- `network_automation_jobs` — discovery zamanlama tablosu (CHECK `discovery`); destructive aksiyonlar için ayrılmış.

## 9. Bilinen limitler

1. **Live test denenmedi.** Şifre repo'da olmadığı için bu PR'da gerçek MikroTik bağlantı testi yoktur. `.env`'i doldurup `POST /test-connection` ile manuel deneyin.
2. **PostgreSQL'e karşı end-to-end smoke yapılmadı.** Migration ve repository unit test fixture'larıyla doğrulandı; lab DB'de tekrar doğrulanması önerilir.
3. **Network links boş.** Bu fazda sadece tablo var; PtP eşlerini link kaydı olarak yaratmak Faz 9 işidir.
4. **Action framework stub.** `internal/networkactions.Action.Execute` hep `ErrActionNotImplemented` döner. Frequency check/correction, AP-client test, link signal test gibi aksiyonlar sonraki fazlarda implement edilecek.
5. **In-process rate limiter ve lock.** Çoklu API replica için Redis-tabanlı limiter (Faz 9+).

## 10. Sonraki faz: AP/link testleri ve frekans kontrol aksiyonları

- `KindFrequencyCheck` + `KindLinkSignalTest`: read-only telemetri (Faz 9 read-side).
- `KindFrequencyCorrection`: dry-run + confirmation + maintenance window + per-device lock + audit + rollback metadata. (Faz 9 write-side.)
- AP-client test motoru: mevcut `internal/apclienttest`'in Dude envanterindeki `Category=AP` cihazlarına otomatik koşturulması.
- Discovery zamanlama: `network_automation_jobs.cron_expr` üzerinden scheduler entegrasyonu.

## 11. Test kapsamı

- `internal/dude/parser_test.go`: detail-print, quoted spaces, flag prefix, comment lines.
- `internal/dude/classify_test.go`: dude_type, name prefix, wireless-mode, no-signal=Unknown, conflict resolution, confidence cap.
- `internal/dude/sanitize_test.go`: secret-like key redaction, input non-mutation, message stripping, length cap.
- `internal/dude/allowlist_test.go`: kabul + ret + path-segmented "no destructive" guard.
- `internal/dude/discovery_test.go`: 3 cihazlık tipik discovery, MAC dedupe, keyless no-merge, Stats.Tally.
- `internal/networkactions/framework_test.go`: stub Execute = ErrActionNotImplemented; per-device lock; rate limiter burst.
- `internal/config/config_dude_test.go`: env mapping + Configured() true/false + password leak yok.

`gofmt -l` temiz, `go vet ./...` temiz, `go test ./...` Faz 8 paketlerinde yeşil, `next build` yeşil.

## 12. Çalıştırma

```bash
# 1. .env'e MIKROTIK_DUDE_PASSWORD ekle (asla commit etme).
# 2. Migrasyonu uygula (transactional + idempotent).
./scripts/db_migrate.sh

# 3. API + worker'ı başlat (mevcut komutlar).
./scripts/dev_run_api.sh
./scripts/dev_run_worker.sh

# 4. Web dashboard'unu başlat:
cd apps/web && npm run dev
# Tarayıcıda → http://localhost:3000/ag-envanteri
# "Bağlantıyı Test Et" → "Discovery Çalıştır".
```
