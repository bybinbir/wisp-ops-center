# Mimosa Read-Only Entegrasyonu (Faz 4)

Bu doküman Mimosa kablosuz cihazlardan **yalnızca SNMP üzerinden okuma** yapan Faz 4 entegrasyonunu anlatır. Yazma yolu hiçbir şekilde mevcut değildir.

## Desteklenen Transport

- **SNMPv2c** (UDP 161) — birincil yol; Mimosa firmware'larında en yaygın çalışan kanal.
- **SNMPv3** (USM) — credential profile şeması Faz 4'te eklendi. Adapter `gosnmp` kütüphanesi MD5/SHA/SHA256 + DES/AES/AES192/AES256 kombinasyonlarını destekler. Lib sınırına takılırsa `mimosa.ErrSNMPv3Misconfigured` döner ve UI bunu net gösterir.
- **Vendor API** — Faz 4'te **kapalı**. `Config.Transport = vendor-api` çağrısı `ErrTransportUnsupported` döner.

## SNMPv2 Kimlik Bilgisi Gereksinimleri

| Alan | Açıklama |
|---|---|
| `auth_type` | `snmp_v2` veya `mimosa_snmp` |
| `secret_ciphertext` | community string (AES-GCM şifreli) |
| `port` | varsayılan 161 |

## SNMPv3 Kimlik Bilgisi Gereksinimleri

`credential_profiles` tablosu Faz 4'te şu alanlarla genişletildi:

| Alan | Açıklama |
|---|---|
| `snmpv3_username` | USM kullanıcı adı |
| `snmpv3_security_level` | `noAuthNoPriv` / `authNoPriv` / `authPriv` |
| `snmpv3_auth_protocol` | `MD5` / `SHA` / `SHA256` |
| `snmpv3_auth_secret_ciphertext` | auth passphrase, AES-GCM şifreli |
| `snmpv3_priv_protocol` | `DES` / `AES` / `AES192` / `AES256` |
| `snmpv3_priv_secret_ciphertext` | priv passphrase, AES-GCM şifreli |

UI yalnızca `snmpv3_auth_set` / `snmpv3_priv_set` boolean rozetlerini gösterir; ham passphrase **hiçbir koşulda** gözükmez.

## Standard SNMP OID'leri (Faz 4'te kullanılan)

| Veri | OID |
|---|---|
| sysDescr | 1.3.6.1.2.1.1.1.0 |
| sysObjectID | 1.3.6.1.2.1.1.2.0 |
| sysUpTime | 1.3.6.1.2.1.1.3.0 |
| sysName | 1.3.6.1.2.1.1.5.0 |
| ifTable / ifXTable | 1.3.6.1.2.1.2.2 / 1.3.6.1.2.1.31.1.1 |

## Mimosa / Vendor OID Stratejisi

**Faz 4'te vendor MIB OID'leri kullanılmaz.** `internal/adapters/mimosa/oids.go` içinde `VendorMIBPlaceholder = "unverified"` sabiti tanımlıdır. Probe ve Poll bu durumu görünce vendor OID çağrısı **yapmaz** ve sonucu `partial=true, vendor_mib_status="unverified"` ile işaretler.

Bu kasıtlıdır:
- Mimosa MIB'leri firmware ailesine göre değişir; doğrulanmamış OID'lere telemetri tabanlı karar bağlamak güvenli değil.
- Vendor MIB Faz 5'te bir lab cihazıyla doğrulandıktan sonra `oids.go` dosyasına isimli sabitler olarak eklenir; probe/poll koşullu olarak çağrı açılır; capability matrisi `canReadFrequency` / `canReadClients` / `canReadWirelessMetrics` bayraklarını ayrı ayrı kanıtlar.

## Toplanan Normalize Tipler

`internal/adapters/mimosa/types.go`:

- `MimosaProbeResult` — sysName/sysDescr/uptime/model/firmware/vendor_mib_status/partial.
- `MimosaSystemInfo` — sysDescr'dan model+firmware tahmini.
- `MimosaInterfaceMetric` — IF-MIB satırı.
- `MimosaRadioMetric`, `MimosaLinkMetric`, `MimosaClientMetric` — vendor MIB doğrulanana kadar boş döner.
- `MimosaReadOnlySnapshot` — Poll sonucu.

## Capability Kuralları (Faz 4)

Probe başarılı → yalnızca `supportsSNMP=true, canReadHealth=true`. Diğerleri vendor MIB doğrulanana kadar **TRUE'ya çekilmez**:
- `canReadWirelessMetrics`, `canReadClients`, `canReadFrequency`, `canRecommendFrequency` → false.
- Yazma bayrakları (`canApplyFrequency`, `canBackupConfig`, `canRollback`, `canRunScan`) → **kalıcı olarak false**.
- `supportsVendorAPI` → false.
- `requiresManualApply` → true.

## Kasıtlı Olarak Devre Dışı

- Frekans değişikliği.
- Konfigürasyon yedeği / geri yükleme.
- Scan tetikleme.
- AP-to-client aktif testler (`docs/AP_CLIENT_TEST_ENGINE.md`).

## Lab Test Prosedürü

1. Lab'da SNMP açık bir Mimosa cihaz ayarla (B5c, B11, A5c gibi).
2. `/snmp/community/add name=ro security=public ...` veya SNMPv3 USM kullanıcısı oluştur.
3. UI'dan o cihaza `snmp_v2` ya da `snmp_v3` tipinde bir credential profile bağla.
4. Cihazlar listesinden **Probe** çalıştır → `system_name`, `system_descr`, `model`, `firmware` dolmuş olmalı; `vendor_mib_status="unverified"`.
5. **Read-only Poll** çalıştır → `interface_metrics` tablosu doldurulmalı; `mimosa_wireless_*` tabloları Faz 5'te dolacak.
6. `/api/v1/mimosa/poll-results` ile son çağrı kayıtlarını kontrol et.

## Sorun Giderme

| Belirti | Neden | Çözüm |
|---|---|---|
| `auth` hata kodu | Yanlış community veya yanlış SNMPv3 USM | Profil tipini ve secret_set rozetini kontrol et |
| `timeout` | Firewall/UDP 161 kapalı | Cihaz tarafında SNMP servisini ve ACL'i kontrol et |
| `unreachable` | Cihaz erişilemiyor | `ping` + `snmpwalk -v2c -c <community> <ip> 1.3.6.1.2.1.1` ile test et |
| `snmpv3_misconfigured` | Username/auth/priv eksik veya `gosnmp` lib yetersiz | UI'da SNMPv3 alanlarını yeniden gir; SHA256/AES256 destek için Go modülünü güncelle |
| `partial` | Beklenen — vendor MIB doğrulanmadı | Faz 5'te kapsam genişler |

## Bilinen Sınırlamalar

- Faz 4'te yalnızca standart MIB veriyi okur; radio/link/client metrikleri henüz dolu gelmiyor.
- SNMPv3 USM için `gosnmp` kütüphanesinin SHA256+AES256 derlemesinin tüm Go modül sürümlerinde tam çalıştığı garanti değildir; hata durumda runtime `snmpv3_misconfigured` kodu döner.
- Vendor API yazma desteği Faz 9'da bile **otomatik olarak açılmaz**; her model+firmware doğrulamasıyla manuel olarak değerlendirilir.

## Güvenlik Garantileri

- Hiçbir noktada yazma komutu yok.
- Hata mesajları `mimosa.SanitizeError` ile maskeleniyor; ham community/auth/priv passphrase loga/audit'e/cevaba düşmez.
- Capability güncellemesi `OR`'lı upsert ile yapılır; daha önce kanıtlanmış bayrak silinmez ama yazma bayrakları **eklenmez**.
