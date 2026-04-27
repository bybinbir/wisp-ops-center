# Cihaz Yetenek Modeli

Her cihazın hangi işlemleri **kanıtlanmış olarak** desteklediğini anlatan boolean bayrak setidir. UI buradaki bilgiyi rozet olarak gösterir; backend, yetenek olmayan bir aksiyonu denemeden önce buradan bakar.

## Bayraklar

| Bayrak | Anlam | Faz 2 varsayılan |
|---|---|---|
| `canReadHealth` | CPU/mem/uptime/temperature okunabilir mi | matriks varsayılanı |
| `canReadWirelessMetrics` | RSSI/SNR/CCQ/Tx/Rx okunabilir mi | matriks varsayılanı |
| `canReadClients` | AP'ye bağlı istemci listesi alınabilir mi | matriks varsayılanı |
| `canReadFrequency` | Anlık frekans/kanal okunabilir mi | matriks varsayılanı |
| `canRunScan` | Frekans tarama | **false** (Faz 3'te detection eklenecek) |
| `canRecommendFrequency` | Bu cihaz için öneri motoru çalıştırılabilir mi | matriks varsayılanı |
| `canBackupConfig` | Yapılandırma yedeği alınabilir mi | **false** (Faz 9) |
| `canApplyFrequency` | Frekans/kanal değişikliği | **false** (Faz 9) |
| `canRollback` | Yedeği geri yükleyebilir miyiz | **false** (Faz 9) |
| `requiresManualApply` | Operatör onayı zorunlu mu | **true** |
| `supportsSNMP` | SNMP read-only çalışıyor mu | matriks varsayılanı |
| `supportsRouterOSAPI` | API-SSL erişilebilir mi | matriks varsayılanı |
| `supportsSSH` | SSH erişilebilir mi | matriks varsayılanı |
| `supportsVendorAPI` | Mimosa/Cambium gibi vendor API var mı | **false** |

`internal/devices/capability_matrix.go` içindeki `DefaultCapabilities(vendor, role)` fonksiyonu Probe sonucu DB'ye yazılana kadar tahmini varsayılanı sağlar.

## Vendor Varsayımları

### MikroTik (`capability_matrix.go`)
- `supportsRouterOSAPI = true`, `supportsSSH = true`, `supportsSNMP = true`.
- `canReadHealth`, `canReadFrequency`, `canRecommendFrequency = true`.
- `canReadWirelessMetrics`: AP/CPE/PTP rollerinde true; switch/router'da false.
- `canReadClients`: AP / PTP master rollerinde true.
- Yazma bayrakları: **hard-locked false** (Faz 9).

### Mimosa (`capability_matrix.go`)
- `supportsSNMP = true`. `supportsVendorAPI = false` (model bazında doğrulanana kadar).
- `canReadHealth`, `canReadWirelessMetrics`, `canReadFrequency`, `canRecommendFrequency = true`.
- `canReadClients`: AP/PTP master'da true.
- Yazma bayrakları: **hard-locked false**.

## Probe Sonucu Yazma Kuralı

Probe başarılı dönerse:
- Sadece `canRead*` ve `supports*` bayrakları TRUE'ya çekilir.
- `canApply*`, `canRollback`, `canBackupConfig` Probe ile değişmez. Bunlar yalnızca model+firmware doğrulamasıyla manuel açılır.

Bu kural, "cihazla konuşabiliyoruz" kanıtının "üzerine yazabiliriz" anlamına **gelmemesini** garanti eder.

## Capability Matrix Kullanımı

```go
import "github.com/wisp-ops-center/wisp-ops-center/internal/devices"

caps := devices.DefaultCapabilities(devices.VendorMikroTik, devices.RoleAP)
// caps.SupportsRouterOSAPI == true
// caps.CanApplyFrequency   == false  (Faz 9'a kadar)
```

`DefaultCapabilities` Probe henüz çalıştırılmamış cihazlar için "tahmini görünüm" sağlar; gerçek capability `device_capabilities` tablosunda saklanır.
