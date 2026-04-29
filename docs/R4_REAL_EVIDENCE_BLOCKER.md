# R4 — Real Evidence Classification: BLOCKED BY DUDE SCHEMA

**Tarih:** 2026-04-29
**main HEAD:** `5bc9f6fa82038a3c93891fcfa01e443472b79610`
**Verdict:** **BLOCKED BY DEVICE ACCESS** — operatör veri-tarafı, classifier kodu değil

## Kullanıcı varsayımı vs Dude gerçeği

Operatör "her cihazın IP'si var" dedi; Dude RouterOS'a doğrudan SSH ile sorduk:

| Sorgu | Sonuç |
|---|---|
| `/dude/device print count-only` | 928 cihaz |
| `/dude/device print detail value-list where name="KOTEKLER_AP"` | **tek field: `name: KOTEKLER_AP`** — başka veri yok |
| `/dude/device get [find name="KOTEKLER_AP"] address` | **`input does not match any value of value-name`** — RouterOS reddetti, `address` Dude device şemasında bir field DEĞİL |
| `/dude/device print detail` (tüm cihazlar) | 928 satırın 924'ü sadece `name` taşıyor; 4'ü `/ip/neighbor` enrichment'tan MAC/platform kazanmış |
| `/ip/neighbor print count-only` | **6** (Dude'un kendi L2 broadcast domain'indeki komşular) |
| `/ip/arp print count-only` | **0** |
| `/dude/agent`, `/dude/network-map` | empty / "bad command name" |

## Net teknik gerçek

Dude `/dude/device` tablosunun **şemasında IP alanı yok**. Operatör bu tabloyu **etiket / harita** olarak kullanmış: 928 cihaz = saha topolojisinin Dude haritasındaki nokta. Cihazların gerçek IP/MAC/credentials'ı **Dude'da kayıtlı değil**.

- Phase 8.1 enrichment (`/ip/neighbor`, `/dude/probe`, `/dude/service`) zaten devreye giriyor — yine de 928'den yalnız 4 cihaz MAC kazanıyor çünkü neighbor cache 6 satır.
- Sandbox'tan TCP/22 reachability probe: 1/3 cihaz (sadece Dude'un kendisi).

## R4 enrichment sınırı

R4 prompt'unun istediği RouterOS read-only / SNMP read-only enrichment **gerçek hedefe yok**:

- SSH için → IP gerek; 928'den 3 cihazda IP var, 1'i reachable.
- SNMP için → IP gerek; aynı sınır.
- Dude internal probe/service → Phase 8.1 zaten kullanıyor, daha fazla veri yok.

Yeni Go kodu yazmak **boş döner**: hedefin %99.7'sinde IP olmadığı için yeni RouterOS/SNMP collector çalışacak adres bulamaz.

## Çözüm (operatör-tarafı, kod değil)

Hedefe ulaşmak için **operatörün Dude device kayıtlarını zenginleştirmesi gerek**. Üç pratik yol:

1. **Dude WebFig'de her cihaza IP girmek**: 928 cihaz × manuel ~5 sn = ~75 dk operatör çalışması. R4 enrichment otomatik olarak çalışacak hedefi bulur.
2. **Dude'un Auto-Discovery'sini açmak**: Dude'un kendi keşif moduyla cihazları subnet bazlı taratmak. Operatörün ağ topolojisini bilmem lazım.
3. **Operatörün ayrı NMS'i (LibreNMS / Zabbix / PRTG) üzerinden cihaz envanterini sync**: NMS'te zaten IP+credentials varsa, oradan WispOps'a aktarmak.

## R3'te ulaşılan (değişmedi, son state)

| Metric | Değer |
|---|---:|
| Total devices | 928 |
| Strong | 3 |
| Weak | 162 |
| Unknown | 763 (%82.2) |
| AP | 86 |
| CPE | 62 |
| Link | 1 |
| Bridge | 0 |
| Router | 15 |
| Switch | 1 |

## Verdict

**BLOCKED BY DEVICE ACCESS — Dude device schema-tarafı.** R4 için yeni kod yazılmaz; operatörün Dude'a IP/credentials disiplini eklemesi sınıflandırma tavanını kaldırır. Aksi halde "isim suffix'i" (`_AP`, `_CPE`) en iyi kazanım kanalıdır ve R3'te o kazanımı zaten tahsil ettik.
