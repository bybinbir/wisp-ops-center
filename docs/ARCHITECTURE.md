# wisp-ops-center — Mimari

## Sistem Amacı

WISP'in günlük operasyonel kararlarını tek yerden almak için tasarlanmış **operasyon karar platformu**. Genel cihaz yönetim paneli **değildir**.

Ana soru: **"Bugün ağda ne bozuk, kime müdahale etmeliyim, hangi link riskli?"**

## Ürün Felsefesi

- **Operasyon önce gelir.** UI ve API, saha ekibinin sabah ilk yarım saatte hangi sırayla müdahale edeceğine cevap vermek için tasarlandı.
- **Sade, az ama doğru.** Karmaşık topoloji çizimleri, fancy grafikler yok.
- **Önce oku, sonra öner, sonra (çok sonra) uygula.** Yazma yetkileri kasıtlı olarak gecikmeli.
- **Hiçbir cihazda dilekçesiz değişiklik yok.** Yedek + dry-run + bakım penceresi + audit + rollback olmadan üretim cihazına dokunulmaz.
- **Vendor uyumsuzluklarını gizleme.** MikroTik ve Mimosa farklıdır; bu UI'da capability rozetleriyle görünür.

## Modüler Sınırlar

```
apps/
  api/        Go HTTP API (pgxpool, AES-GCM vault, CRUD endpoints).
  worker/     Asynq-uyumlu sözleşmeli worker (Faz 5'te gerçek kuyruk).
  web/        Next.js + TypeScript. Türkçe operasyon UI.

internal/
  audit/             Audit kayıt sözleşmesi + Postgres + memory sink.
  config/            Env → typed config (WISP_DATABASE_URL, WISP_VAULT_KEY,
                     WISP_API_TOKEN dahil).
  credentials/       Vault interface + AES-GCM impl + Profile + Sanitize +
                     Repository (CRUD, secret asla cevapta dönmez).
  database/          pgxpool wrapper + migration runner (schema_migrations,
                     checksum, idempotent, transactional).
  devices/           Cihaz domain modeli + capability flag tipi +
                     capability_matrix.go (vendor+rol → varsayılan bayrak).
  inventory/         Sites/Towers/Devices/Links/Customers repository ve
                     domain tipleri.
  adapters/          Vendor erişim sözleşmesi (Faz 2'de hâlâ stub).
    mikrotik/        RouterOS taslağı.
    mimosa/          Mimosa taslağı.
    snmp/            Ortak SNMP yardımcıları.
    ssh/             Ortak SSH yardımcıları.
  links/             PTP/PTMP hat domain modeli (eski tip; inventory paketine
                     entegre).
  customers/         Müşteri tip sabitleri.
  scheduler/         İş tipi + cadence + mod tanımları.
  scoring/           Müşteri/hat sağlık skor motoru (deterministik).
  reports/           Günlük/haftalık rapor modeli.
  recommendations/   Frekans öneri durum makinesi.
```

`internal/` dışından `apps/` içine bağımlılık yapılmaz. UI ile API arasındaki tek sözleşme `/api/v1/*` HTTP yüzeyidir.

## Backend Mimarisi (Faz 2)

- Go 1.22, jackc/pgx/v5/pgxpool, golang.org/x/crypto/aes-gcm.
- HTTP `net/http` + ince middleware (request log + güvenlik başlıkları + Bearer token).
- Yapılandırma yalnızca env'den; secret hardcode yok.
- Migration runner API binary'sine `--migrate` bayrağı ile gömülü.
- Audit yazımı her CRUD aksiyonunda Postgres sink'e (db yoksa memory sink).
- Capability matrisi Probe gerçek değer yazana kadar varsayılanı sağlar; yazma bayrakları (`canApply*`, `canBackupConfig`, `canRollback`) varsayılan **DAİMA false**.

## Worker Mimarisi

- Faz 1+2: Asynq/Redis bağlantısı yok. İş tipi kaydı + iskelet handler + heartbeat.
- Faz 5: Asynq adapter eklenecek; sözleşme `internal/scheduler` paketinde sabit.
- AP-to-client test handler'ları yalnızca Faz 5'ten sonra register edilir.

## Frontend Mimarisi

- Next.js 14 App Router, TypeScript strict (`noUncheckedIndexedAccess`).
- Türkçe varsayılan, koyu tema.
- `src/lib/api.ts`: typed fetch helper, ApiError, opsiyonel `NEXT_PUBLIC_API_TOKEN` Bearer.
- Form bileşenleri: `Toolbar`, `Field`, `Modal` (basit, bağımsız).
- 503 cevabı (DB yok) UI tarafında "Veritabanı bağlı değil" banner'ı olarak gösterilir.

## MikroTik / Mimosa Entegrasyon Planı

- **Faz 3:** RouterOS API-SSL → SSH → SNMP read-only; capability güncelleme.
- **Faz 4:** Mimosa SNMP-first; vendor API yazma KAPALI.
- Faz 2'de yalnızca arayüz mevcuttur; tüm I/O metotları `errNotImplemented`.

## Scheduler Akışı

1. UI'dan `scheduled_checks` kaydı oluşturulur (cadence + scope + mode).
2. Scheduler (Faz 5+) `next_run_at`'a göre `job_runs` kaydı açar.
3. Worker, ilgili handler'ı çalıştırır.
4. Sonuç `job_runs.summary` JSON'una düşer; öneri/uyarı uygunsa ilgili tabloya yazılır.
5. `controlled_apply` Faz 9'a kadar DB seviyesinde reddedilir.

## AP-to-Client Test Akışı (Plan)

`docs/AP_CLIENT_TEST_ENGINE.md` detaylı tasarımı içerir. Özet:

1. Operatör `ap_client_test_profiles` tanımlar (test_type, risk, süre, hız limiti).
2. Scheduler profil + AP grubunu zamana bağlar.
3. Worker düşük riskli testleri çalıştırır; sonuçlar `ap_client_test_results`'a düşer.
4. Yüksek riskli test → manuel onay + bakım penceresi + audit.
5. Rapor (Faz 7) ve skor motoru (Faz 6) bu sonuçları tüketir.

## Sinyal Skorlama Akışı

`internal/scoring` deterministik kural motoru. Girdi: RSSI, SNR, CCQ, Tx/Rx, disconnect, uptime kararlılığı, AP geneli kötüleşme, hat kapasite, 7g trend. Çıktı: 0–100 + tanı + aksiyon. ML yok.

## Frekans Öneri Akışı

Faz 8'de eklenecek. Faz 9'da yedek + dry-run + bakım penceresi + audit + rollback ile uygulamaya bağlanır.

## Dağıtım Modeli

Tek Linux sunucu (Ubuntu 22.04+), API & worker `systemd`, nginx → 443 TLS, PostgreSQL & Redis aynı host veya yönetilen servis.

## Gözlemlenebilirlik

- Yapılandırılmış log (`log/slog`, JSON üretimde).
- Prometheus `/metrics` Faz 10'da.
- Audit logu UI'da Ayarlar → Audit altında Faz 7'de.
