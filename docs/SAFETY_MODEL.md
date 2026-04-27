# Güvenlik (Safety) Modeli

WISP cihazlarına dokunmak müşteriyi internetsiz bırakabilir. Bu doküman hangi koruma katmanlarının **ne zaman** devrede olduğunu net biçimde tanımlar.

## Faz 1+2+3 — Mutlak Kurallar

1. **Hiçbir cihaza yazma yapılmaz.** MikroTik adapter'ı dahil tüm yazma yolları kapalıdır. Komut allowlist'i segment-bazlı veto ile yazma içeren her komutu reddeder (set/add/remove/scan/bandwidth-test/reset/reboot/upgrade/import/export/file/tool).
2. **Otomatik frekans değişikliği yoktur.**
3. **`scheduled_checks.mode = 'controlled_apply'` veritabanı seviyesinde reddedilir** (`phase1_no_apply` CHECK Faz 1+2+3'te aktif).
4. **Kimlik bilgileri AES-GCM şifreli saklanır.** API cevaplarında yalnızca `secret_set: bool`, audit metadatasında `secret_rotated`/`secret_set` bayrakları, log alanları `credentials.Sanitize` veya `mikrotik.SanitizeError` ile maskelenir.
5. **AP-to-client testleri çalıştırılmaz.** Faz 3'te yalnızca veri modeli + UI placeholder + Cihazlar sayfasında manuel MikroTik salt-okuma poll'u var.
6. **Yüksek riskli AP-client testleri zorunlu manuel onay** (DB CHECK constraint'i).
7. **MikroTik telemetri toplaması read-only'dir.** Adapter `WriteCapableAdapter` arayüzünü hiçbir somut tip uygulamaz. Probe başarılı dönse bile `canApplyFrequency`, `canBackupConfig`, `canRollback`, `canRunScan` bayrakları **TRUE'ya çekilmez**.
8. **Hata mesajları sanitize edilir.** `mikrotik.SanitizeError` parolayı/community'yi/token'ı log/audit/cevap dışına atar.

## İleri Fazlarda Yazma için Asgari Koşullar

Bir cihaza ilk yazma yapılmadan önce **TÜM** aşağıdaki koşullar sağlanmalıdır:

1. Capability check — `device_capabilities.can_apply_frequency = TRUE`
2. Credential izni — `device_credentials.transport` yetkin
3. Yedek/export — `BackupConfig` başarılı, audit_logs.metadata referansı
4. Dry-run etki tahmini
5. Bakım penceresi (`maintenance_windows` içinde)
6. Audit log — `recommendation.apply` önce yazılmış
7. 60 sn recovery doğrulaması
8. Test edilmiş rollback planı

Koşullardan biri sağlanmıyorsa worker `RunBlocked` döner.

## Mimosa Yazma Yasağı

- `can_apply_frequency` Faz 9'da bile **otomatik** TRUE olmaz.
- Her model + firmware kombinasyonu için ayrı doğrulama matrisi gerekir.
- Doğrulanmamış Mimosa modeline yazma denemesi `RunBlocked` ile reddedilir.

## Vault Politikası

- AES-256-GCM, `WISP_VAULT_KEY` zorunlu.
- `secret_ciphertext` BYTEA + `secret_key_id` (anahtar fingerprint).
- Rotation runbook: `docs/VAULT_ROTATION.md`. İki anahtar re-encryption planı Faz 4'te uygulanacak.

## Audit Politikası

- `audit_logs` append-only.
- `wispops_app` rolünden UPDATE/DELETE `REVOKE` (migration 000002).
- Probe ve Poll çağrıları `scheduled_check.ran` aksiyonu olarak audit'e düşer; `outcome` `success/blocked/failure` olur.

## API Auth

- `WISP_API_TOKEN` set edildiyse: `/api/v1/health` ve `/` dışındaki tüm uçlarda `Authorization: Bearer <token>` zorunludur.
- `WISP_API_TOKEN` boşsa: yerel geliştirme için izinli, log uyarısı atılır.

## MikroTik Read-Only Sınırları (Faz 3)

`docs/MIKROTIK_READONLY_INTEGRATION.md` içinde detayı vardır. Özet:
- Allowlist: 11 RouterOS yolu, **tam eşleşme** ile.
- Forbidden segmentler: 16 yazma/risk verbi; bir segment olarak görülürse veto.
- Probe & Poll yalnızca `canRead*` + `supports*` bayraklarını günceller; yazma bayraklarına asla dokunulmaz.
- TLS doğrulaması varsayılan KAPALI (self-signed RouterOS sertifikaları yaygın). Bunu profil bazında açmak Faz 4'te eklenecek.

## Anti-pattern'ler (yapılmaz)

- "Tek tıkla optimize et" düğmesi.
- `if recommendation.risk == "low" { apply() }` kestirmesi.
- Probe başarılı diye `canApply*` TRUE.
- Audit logu olmadan apply.
- Şifrelemesiz parola saklamak.
- Allowlist'te olmayan komutu "tek seferlik" çalıştırmak.
