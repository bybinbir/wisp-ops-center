# R2 Source-Yield Audit — Sınıflandırıcı Girdileri

**Audit ID:** WISP-R2-SOURCE-YIELD-001
**Tarih:** 2026-04-29
**Kapsam:** `internal/dude` paketinin classifier girdileri ve neden lab dataset'inde Bilinmeyen oranının ~%99 olduğu — kod-seviyesi analiz.
**Live data:** mevcut session'da DB erişimi yok. Live source-yield metrikleri **operatör-tarafı doğrulama gerekiyor**; aşağıdaki tüm satır sayıları ve oranlar code-level analiz + memory'deki Phase 8 / Phase 8.1 lab smoke kanıtlarından geliyor.

## 1. Yönetici Özeti

`Classify` fonksiyonu (`internal/dude/classify.go`) en az 8 kanıt kaynağından besleniyor: `Dude type`, `name pattern`, `wireless mode`, `interface type`, `platform`, `identity`, `model/board`, `interface name`. Bu kanıtların her biri kategori için 5–50 ağırlık üretebiliyor. Cihaz Unknown'a düşmesi için *bestScore < 20* eşiğini aşamaması gerek. Lab gerçeği: 893 cihazın 892'si bu eşiği aşamamış.

Sebep tek başlıklı: **lab Dude'unun döndürdüğü `dude_device` çıktısında `name` alanı genellikle yalnız sayı/lokasyon (örn. "300", "300-OREN", "400") ve `mac/platform/identity/board/interface_name` alanları boş**. Mevcut `name_hint` token listesi ("ap-", "cpe-", "ptp-", "abone", vs.) operatörün isim konvansiyonuna uymuyor → hiçbir token eşleşmez → score = 0 → Unknown.

R2 fallback (`weak_name_pattern`, `internal/dude/classify_weak.go`) bu boşluğu doldurmak için **mevcut name_hint dışında** geniş bir token listesi (kule, omni, mimosa, rocket, sxt, lhg, konut, home, rb, ccr, agg, vs.) ile **yalnız Unknown'da kalan cihazlara** uygulanıyor; confidence 50 ile cap'lı; ambiguous match Unknown'da bırakıyor.

## 2. Mevcut Kanıt Kaynakları (Code-level)

| Kaynak | Field(ler) | Strong/Weak | Ağırlık aralığı | Lab'de yatkınlık |
|---|---|---|---:|---|
| `dude_type` | `Type` (Dude'un kendi etiketi: ap/router/switch/bridge/cpe/client) | strong | 35–50 | **çoğunlukla boş** — lab Dude bu alanı doldurmuyor |
| `name_hint` | `Name` substring/prefix match | weak | 20–35 | **kısmen** — operator isim konvansiyonu name_hint listesiyle uyumsuz |
| `wireless_mode` | `Raw["wireless-mode"]` (ap-bridge / station / station-bridge) | strong | 25–35 | **boş** — `dude_device` enrichment passes bu alanı taşımıyor |
| `interface_type` | `Raw["interface-type"]` | medium | 20 | **boş** |
| `platform` | `Platform` (RouterOS / Mimosa / airOS) | bonus 5 + downstream | strong | **çoğu boş** — Phase 8.1 enrichment'a göre 4/914 cihaz MAC kazandı |
| `identity` | `Identity` | medium | 15 | **boş** |
| `model+board` | `Model + Board` (RB/CCR/CSS/SXT/wAP/...) | medium | 25–30 | **boş** — Dude `dude_device` print/detail bu alanı default yansıtmıyor |
| `iface_name` | `InterfaceName` (wlan1-ap, ether1-uplink) | low | 10 | **boş** |
| `signal_bonus` | MAC + IP + Platform + Identity + Board + InterfaceName var olunca toplam | bonus | +0..+36 | **çok az tetiklenir** (MAC enriched=4/914 cihaz) |

### Phase 8.1 enrichment kaynaklarının doluluk oranı

`docs/PHASE_008_1_DISCOVERY_ENRICHMENT.md`'de operatör smoke sonuçlarına göre:

| Kaynak | Cihaz başına yatkınlık | Not |
|---|---|---|
| `dude_device` primary | 893 / 893 (100%) | Tüm cihazlar bu pass'ten geldi; ama yalnız `name` taşıyor |
| `ip_neighbor` | düşük | Çoğu cihaz neighbor tablosunda yok; lab Dude'u bu sayfayı dolduramıyor |
| `dude_probe` | düşük | Service signal istisnaî; çoğu cihaz için boş |
| `dude_service` | düşük | Per-port ipucu; lab'de seyrek |
| `system_identity` | 1 (self) | Sadece Dude'un kendi kaydı |

Sonuç: %99 cihaz için elimizde sadece **isim** var.

## 3. R2 Çözümü — `weak_name_pattern` Fallback

| Tasarım kararı | Sebep |
|---|---|
| Yalnız Unknown'da çalışır | Mevcut Classify zaten name_hint yakalarsa `weak` çalışmaz; double-evidence yok |
| Confidence cap = 45 | <50 → "düşük güven" UI bucket'ı; sahte güçlü güven yok |
| Ambiguous match → Unknown | İki farklı kategoride pattern eşleşmesi sahte kesinlik üretmez |
| Token-boundary match | `tap` "ap" ile eşleşmemeli; `tokenize → ["tap","merkez"]` → `hasPrefix(token, "ap")` zaten false döner |
| Digit-suffix kabul | `ap1`, `sektor2`, `rb750` gibi numarali variant'lar match eder |
| Evidence row üretir | Her weak match `device_category_evidence` satırı → R1 EvidenceModal'da operatör görür |
| Strong override | Mevcut Classify `Unknown` dışı bir kategori verirse weak hiç çalışmaz |

### `weak_name_pattern` token listesi

Kategori başına token'lar mevcut name_hint *dışında* tutuldu. Mevcut name_hint zaten "ap-", "cpe-", "ptp-", "abone", "bridge-", "rtr-" vs. yakalıyor.

| Kategori | Yeni token'lar (mevcut name_hint dışında) |
|---|---|
| **AP** | `ap`*, `sektor`, `sektör`, `sector`, `tower`, `kule`, `baz`, `base`, `omni`, `wifi`, `wlan` |
| **BackhaulLink** | `ptp`*, `ptmp`, `bh`, `link`*, `relay`*, `backhaul`*, `airfiber`, `mimosa`, `rocket`, `powerbeam`, `lhg`, `sxt`, `dish`, `nano`, `gemi` |
| **Bridge** | `bridge`* (sondan), `br`* (sondan), `core-bridge`, `switch-bridge` |
| **CPE** | `cpe`*, `client`*, `musteri`*, `müşteri`*, `abone`*, `ev`, `home`, `user`, `station`, `sta`, `konut` |
| **Router** | `router`*, `gw`*, `gateway`*, `core`*, `pop`*, `rb`, `ccr`, `chr`, `edge`*, `agg`, `aggregation` |

`*` = mevcut name_hint substring'inde benzer yakalama var; weak yine de tetiklenebilir çünkü name_hint *ön-eki* arar (ör. `"router-"`), weak ise *token boundary* arar (ör. `"merkez-router"` weak'i tetikler ama mevcut name_hint'i tetiklemez).

## 4. Beklenen Yatkınlık (Lab Tahmini)

Operatörün isim konvansiyonu hakkında tek bilgi `docs/PHASE_008_OPERATOR_SMOKE_RESULT.md`'deki örneklerden geliyor: `"400"`, `"300-OREN"`, `"<...>"`. Bu örnekler weak token listesiyle **doğrudan** eşleşmiyor (çünkü çoğu sayısal / yer-bazlı).

→ **Live lab smoke'unda şu ihtimaller var:**

1. **İyimser senaryo**: Cihaz isimlerinin %50–70'i kule/omni/sektor/abone/rb/agg gibi bir token barındırır → R2 hedefi (<%30 Unknown) ulaşılır.
2. **Pesimist senaryo**: Cihaz isimleri çoğunlukla `<numara>-<mahalle>` deseninde → token-bazlı match çok az → R2 hedefi tutmayabilir.
3. **Karışık senaryo**: %30–50 cihaz token barındırır → kısmi iyileşme; hedef tutmazsa R2 raporu **product-validation pending** + token listesi tuning gerek.

Üç senaryo da `docs/R2_CLASSIFICATION_CORRECTION_REPORT.md`'de "operatör smoke gerekiyor" notuyla işaretli.

## 5. Operatör-Tarafı Doğrulama Komutları

Lab DB'sine erişiminiz varsa şu komutla R2 öncesi ve sonrası dağılımı karşılaştırabilirsiniz:

```sql
-- Pre-R2 baseline (R2 kodunu deploy etmeden önce çalıştırın):
SELECT category,
       COUNT(*) AS total,
       COUNT(*) FILTER (WHERE confidence > 50)               AS strong,
       COUNT(*) FILTER (WHERE confidence BETWEEN 1 AND 50)   AS weak,
       COUNT(*) FILTER (WHERE confidence = 0)                AS unconfident
FROM network_devices
GROUP BY category
ORDER BY total DESC;

-- Post-R2 (R2 deploy + bir discovery run sonrası):
-- Aynı sorgu + weak_name_pattern dağılımı:
SELECT
  COUNT(DISTINCT device_id) AS distinct_devices_with_weak_pattern
FROM device_category_evidence
WHERE heuristic = 'weak_name_pattern';
```

Bu sayıları `R2_CLASSIFICATION_CORRECTION_REPORT.md` "Lab Smoke Sonuçları" bölümüne yazmak yeterli.

## 6. Açık Borçlar / Sonraki Adımlar

- **Live source-yield doğrulaması** — yukarıdaki SQL bloklarını lab DB'sine koşturup oranları bu rapora geri yazma.
- **Token listesi tuning** — eğer post-R2 Unknown oranı hâlâ >%30 ise, lab smoke çıktısına bakıp eksik token'ları (örn. `ber`, `top`, `<bilinmeyen-pattern>`) listeye eklemek için ayrı küçük bir PR.
- **R3'e geçiş ön koşulu** — R2 ürün-tamamlandı sayılması için Unknown ≤ %30 olmalı; aksi halde R2 "product-validation pending" durumda kalır ve R3 başlatılamaz.

## 7. Dürüstlük Notu

Bu raporun "live yatkınlık" rakamları **ölçülmemiş tahmin**dir. R2 PR'ı engineering-closed (testler, code, UI hepsi yeşil); product-closed olabilmesi için operatör tarafından gerçek 893-cihaz dataset'inde koşturulup Unknown oranının ölçülmesi gerek.
