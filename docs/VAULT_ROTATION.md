# WISP_VAULT_KEY Anahtar Rotasyonu

`WISP_VAULT_KEY` MikroTik/Mimosa kimlik bilgilerinin AES-256-GCM ile şifrelenmesinde kullanılan **kök anahtardır**. Anahtar kaybedilirse şifreli credential profile sırlarına bir daha erişilemez.

## Şu Anki Faz 2/3 Modeli

- `internal/credentials/credentials.go::AESGCMVault` 32 baytlık (base64 ya da hex) anahtarı kabul eder.
- Şifreleme nonce + ciphertext bytea olarak `credential_profiles.secret_ciphertext` alanına yazılır.
- Anahtar parmak izi (`secret_key_id`) log korelasyonu için tutulur; ham anahtar asla saklanmaz.

## Neden Yedek Şart?

Anahtar kaybı = bütün cihaz kimliklerinin yeniden girilmesi. Üretimde anahtar şu yerlerde **bağımsız** olarak yedeklenmelidir:
1. Şifreli bir parola yöneticisi (1Password / Bitwarden iş hesabı).
2. Cloud KMS (AWS KMS, GCP KMS) içinde manuel "wrap" edilmiş kopya.
3. Hardware token (YubiHSM2 veya benzeri) — uzun vadeli.

## Anahtar Üretimi

```
openssl rand -base64 32
```

Üretilen anahtar yalnızca `.env` veya systemd `EnvironmentFile=` üzerinden okunur. Repo'ya commit edilmesi YASAK.

## Rotasyon Riski

Tek anahtar değiştirildiğinde **tüm** mevcut `secret_ciphertext` çözülemez hale gelir. Operasyonel olarak:

1. Hizmeti durdurmadan rotasyon mümkün değildir.
2. Tek bir cihaz için bile re-encrypt yapmak için eski anahtar gereklidir.

## İki Anahtar Re-encryption Planı (Faz 4'te uygulanacak)

- `WISP_VAULT_KEY` (active) + `WISP_VAULT_KEY_NEXT` (rotation target) iki ayrı env değişkeni.
- Rotation komutu (`wisp-ops-api -rotate-vault`):
  1. Tüm `credential_profiles` satırlarını oku.
  2. Aktif anahtarla **decrypt**.
  3. Yeni anahtarla **encrypt**, `secret_key_id`'yi yeni parmak izine güncelle.
  4. Tek transaction içinde commit.
  5. `WISP_VAULT_KEY` env'i yeni değerle değiştirilir, `WISP_VAULT_KEY_NEXT` boşaltılır.
- Rotation süreci audit_logs'a `vault.rotation` aksiyonu olarak düşer.

## Acil Kurtarma Notları

- Eski anahtar yedeği yoksa tek seçenek: tüm credential profile sırlarını teker teker yeniden gir.
- Rotasyon başarısız olursa **eski anahtar** ile hâlâ decrypt edilebilen profiller bozulmaz; yarı-rotated tek transaction içinde rollback yapılmalıdır.
- Yedeği parola yöneticisinde tutulurken **erişim logu** günlük gözden geçirilir; anahtar parmak izi (kid) production logları ile çapraz kontrol edilir.

## Bu Doküman Asla İçermez

- Gerçek bir `WISP_VAULT_KEY` değeri.
- Üretim cihaz parolası.
- Müşteri MAC/IP eşlemeleri.

Bu kuralın ihlali güvenlik incelemesi için PR'ı reddetme sebebidir.
