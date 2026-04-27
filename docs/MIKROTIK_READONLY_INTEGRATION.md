# MikroTik Read-Only Entegrasyonu (Faz 3)

Bu doküman MikroTik cihazlardan yalnızca okuma yaparak veri toplayan Faz 3 entegrasyonunu anlatır. **Her şey salt-okunurdur.** Yazma içeren tek bir komut bu fazda kabul edilmez.

## Desteklenen Transportlar

Tercih sırası:
1. **RouterOS API-SSL** (TCP 8729) — birincil yol; en zengin veri.
2. **SNMP v2c** (UDP 161) — sağlık/identity için yedek.
3. **SSH** (TCP 22) — yalnızca API/SNMP yetersizse, sadece okuma komutları.

Kod konumu: `internal/adapters/mikrotik/{api_client,snmp_client,ssh_client}.go`.

## Gerekli MikroTik Servisleri ve Portlar

- API-SSL servisi açık: `/ip service enable api-ssl` (yalnız sistem yöneticisi tarafından bir kez yapılır).
- SNMP açık: `/snmp set enabled=yes` ve `/snmp community add` ile community string tanımlanmış olmalı.
- SSH açık (varsayılan).

## Kimlik Bilgisi Gereksinimleri

`credential_profiles` tablosundan profil oluşturup cihaza `device_credentials` üzerinden bağlanır. `auth_type` ile transport şu şekilde eşlenir:

| `auth_type`           | Transport          |
|-----------------------|--------------------|
| `routeros_api_ssl`    | API-SSL            |
| `ssh`                 | SSH                |
| `snmp_v2`, `snmp_v3`  | SNMP (v3 planlandı)|

Şifreler AES-256-GCM ile `WISP_VAULT_KEY` kullanılarak şifrelenir; ham parola hiçbir yerde gözükmez. Audit kayıtlarına yalnızca `secret_set: true` bayrağı düşer.

## Read-Only Komut Allowlist'i

`internal/adapters/mikrotik/allowlist.go` aşağıdaki RouterOS yollarını **tam eşleşme** ile kabul eder:

```
/system/identity/print
/system/resource/print
/system/routerboard/print
/interface/print
/interface/wireless/print
/interface/wireless/registration-table/print
/interface/wifi/print
/interface/wifi/registration-table/print
/interface/wifiwave2/print
/interface/wifiwave2/registration-table/print
/ip/address/print
```

## Yasaklı Komut Segmentleri

Aşağıdaki komut segmentleri herhangi bir yolda görünürse **veto** edilir; allowlistte olsa bile çalışmaz:

```
set, add, remove, enable, disable
scan, frequency-monitor
bandwidth-test
reset, reboot, shutdown, upgrade
import, export
file, tool
```

## Toplanan Normalize Tipler

`MikroTikProbeResult`, `MikroTikSystemInfo`, `MikroTikInterfaceMetric`, `MikroTikWirelessInterfaceMetric`, `MikroTikWirelessClientMetric`, `MikroTikReadOnlySnapshot` (`internal/adapters/mikrotik/types.go`).

## Wireless Paket Ayrımı

Adapter üç paket ailesini sırayla dener:
1. `legacy` — `/interface/wireless/...` (eski 2.4/5 GHz wAP/SXT/LHG kartları).
2. `wifiwave2` — `/interface/wifiwave2/...` (RouterOS 7.13+, Audience/AX modelleri).
3. `wifi` — `/interface/wifi/...` (RouterOS 7 yeni nesil).

Switch/router cihazları wireless erişilemediğinde Probe `wireless_available=false` döner; bu hata değildir.

## Capability Güncelleme Kuralları

Probe başarılı dönerse `device_capabilities` tablosunda yalnızca `canRead*` + `supports*` bayrakları TRUE'ya çekilir. Yazma bayrakları (`canApplyFrequency`, `canBackupConfig`, `canRollback`, `canRunScan`) Faz 3'te asla TRUE olmaz.

## Sorun Giderme

| Belirti | Neden | Çözüm |
|---|---|---|
| `auth` hata kodu | Yanlış kullanıcı/şifre veya API-SSL kapalı | RouterOS'ta `/ip service enable api-ssl`, profilin secret'ı kontrol et |
| `timeout` hata kodu | Firewall, port kapalı veya cihaz aşırı yük altında | `/ip firewall filter` MGMT zincirini kontrol et |
| `unreachable` | Yönlendirme/MAC sorunu | `ping` + `traceroute` ile temel erişimi doğrula |
| `disallowed_command` | Kod tarafı bug — geliştirici izi | `internal/adapters/mikrotik/allowlist_test.go` testlerini yeniden çalıştır |
| Wireless bilgisi yok | Cihaz kablosuz değil veya paket farklı | Probe `wifi_package` alanını kontrol et |

## Lab Test Prosedürü

1. Lab'da bir RouterBOARD (örn. hAP ax² veya wAP) hazırla.
2. `/user add name=wispops-ro group=read password=...` ile read-only kullanıcı oluştur.
3. UI'dan o cihaza `routeros_api_ssl` tipinde bir credential profile bağla.
4. Cihazlar sayfasından **Probe** çalıştır → `wireless_available`, `wifi_package` doğrula.
5. **Read-only Poll** çalıştır → arayüzler ve istemciler dolmalı.
6. `/api/v1/mikrotik/poll-results` ile son çağrı kayıtlarını kontrol et.

## Bilinen Sınırlamalar

- SNMPv3 tam destek Faz 3'te taslak; credential profile USM alanları Faz 4'te eklenir.
- SSH transport üzerinden tam telemetri normalize edilmiyor — yalnızca identity onayı yapılır.
- TLS sertifikası varsayılan olarak doğrulanmaz (self-signed RouterOS sertifikaları yaygın olduğu için). `credential_profile.verify_tls` gelecekte eklenecek.
- Yüksek sayıda istemcili AP'lerde registration-table'ın 2 sn timeout'u aşması mümkün; gerekirse `WISP_MIKROTIK_TIMEOUT_SEC` ile artırılabilir.
