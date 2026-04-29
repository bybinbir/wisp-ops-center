# Phase R2 — Classification Correction Report

**Prompt ID:** WISP-R2-CLASSIFICATION-CORRECTION-001
**Tarih:** 2026-04-29
**Branch (önerilen):** `phase/r2-classification-correction`
**Bağımlılıklar:** main HEAD `ebd54324` (R1 merged), R1.1 gofmt hygiene no-op olarak doğrulandı.

## 1. Yönetici Özeti

R1 ürün gerçekliğini görünür yaptı (Bilinmeyen yüzdesi dashboard'a, "neden Bilinmeyen?" drill-down EvidenceModal'a). R2 ürün gerçekliğini *değiştirmek* için classifier'a ikinci bir katman ekliyor: `weak_name_pattern`. Yalnız Unknown'da çalışır, confidence 50 ile cap'lı, ambiguous match Unknown'da bırakır, strong evidence override eder, her match `device_category_evidence` satırı doğurur. Ek olarak: ops-panel API'si strong/weak/unknown sayım ve hedef-progress döndürür; dashboard'a "Sınıflandırma Kovaları" section'ı ve progress bar eklendi; EvidenceModal weak satırı için ayrı banner ve "sınıflandırma katmanı" badge'i; `/ag-envanteri` confidence kolonu artık 3-renk chip + tooltip.

**Verdict:** **engineering-closed** + **product-validation pending**. Tüm code-level gate'ler yeşil; live 893-cihaz lab dataset'inde Unknown oranının ölçülmesi operatör tarafı.

## 2. Verdict Detayı

| Kriter | Sonuç |
|---|---|
| Tests pass | ✅ networkactions 138, dude 38→**59** (+21), apps/api/internal/http 16, full repo 16 ok / 16 no-test |
| `weak_name_pattern` evidence-backed | ✅ her match `device_category_evidence` satırı, heuristic=`weak_name_pattern`, reason Türkçe + matched token |
| Confidence cap zorlanır | ✅ sabit 45 (cap 50), test ile invariant: `TestWeak_ConfidenceCapEnforced` |
| UI confidence + weak/strong ayrımı gösterir | ✅ `/ag-envanteri` chip + tooltip, EvidenceModal `ConfidenceTierBadge`, dashboard "Sınıflandırma Kovaları" |
| R1 modal weak'i açıklar | ✅ Why bileşeni `weak_name_pattern` evidence row'u varsa amber banner + reason render eder |
| Destructive kod yolu değişmedi | ✅ `frequency_correction` registry stub, `RegisterFrequencyCorrection` çağrısı yok, master switch kapalı |
| Live/snapshot sınıflandırma kanıtı | ⚠️ **eksik** — sandbox'ta DB erişimi yok; operatör smoke gerek |
| Unknown oranı ≥%30 azaldı | ⚠️ **ölçülmedi** — product-validation pending |
| Hedef progress dashboard'da | ✅ `/api/v1/dashboard/operations-panel` `discovery.classification.{strong,weak,unknown,target_*_percentage}` döndürür; UI ProgressBar render eder |

## 3. Before / After Tablosu

### Code-level (kanıtlanmış, sandbox'ta)

| Önce | Sonra |
|---|---|
| `internal/dude/classify.go` 257 satır, tek katman | +`classify_weak.go` 175 satır weak fallback; `classify.go` sonuna 5-satırlık `applyWeakNamePattern` çağrısı |
| `dude` paketinde 38 unit test | **59** (+21 R2 weak testi) |
| `opsClassificationProgress` yok | yeni JSON section: `discovery.classification.{strong,weak,weak_by_pattern,unknown,*_percentage,target_*_percentage}` |
| Dashboard'da yalnız Bilinmeyen kartı | "Sınıflandırma Kovaları" section'ı: Güçlü / Zayıf / Bilinmeyen / weak_name_pattern + ProgressBar (hedef = %70 non-Unknown) |
| EvidenceModal'da tier ayrımı yok | `ConfidenceTierBadge` (Güçlü/Zayıf/Bilinmeyen) + weak match için amber banner |
| `/ag-envanteri` confidence kolonu plain sayı | `ConfidenceCell` 3-renk chip + tooltip (güçlü güven / zayıf güven / düşük güven) |

### Live (lab) — operatör tarafı

| Metric | Önce (Phase 8.1 sonrası, `docs/PHASE_008_1_DISCOVERY_ENRICHMENT.md`) | Sonra | Hedef |
|---|---:|---:|---:|
| Toplam cihaz | 914 | (yeniden discovery sonrası) | — |
| Bilinmeyen sayı | ~910 | **operatör smoke gerek** | ≤ 274 |
| Bilinmeyen yüzde | ~%99.6 | **operatör smoke gerek** | ≤ %30 |
| MAC enriched | 4 | (R2 dokunmaz) | — |
| weak_name_pattern eşleşen cihaz | 0 (R2 öncesi) | **operatör smoke gerek** | — |

## 4. Source-Yield Bulguları

`docs/R2_SOURCE_YIELD_AUDIT.md`'de detay. Özet:

- Mevcut Classify 8 kanıt kaynağından besleniyor (Dude type, name pattern, wireless mode, interface type, platform, identity, model/board, iface name).
- Lab Dude'unda 8 kaynaktan 7'si **boş** geliyor; sadece `name` taşınıyor.
- Mevcut `name_hint` listesi operatörün isim konvansiyonunu tam yakalamıyor — Phase 8.1 sonrası 893'ten 1 cihaz Router olarak sınıflandı.
- R2 fallback bu kanıt boşluğunu **isim token'ından çıkarılan zayıf sınıflandırma** ile dolduruyor; operatöre güçlü/zayıf ayrımı net.

## 5. Ne Değişti — Dosya Bazında

```
NEW   internal/dude/classify_weak.go                       175 satır  (weak heuristic + tokenizer)
NEW   internal/dude/classify_weak_test.go                  270 satır  (21 hermetic test)
MOD   internal/dude/classify.go                            +5 satır  (applyWeakNamePattern çağrısı)

MOD   apps/api/internal/http/handlers_operations_panel.go  +60 satır (opsClassificationProgress + SQL)

MOD   apps/web/src/lib/api.ts                              +13 satır (OpsClassificationProgress tipi)
MOD   apps/web/src/app/DashboardClient.tsx                 +85 satır (Sınıflandırma Kovaları + ProgressBar)
MOD   apps/web/src/app/ag-envanteri/EvidenceModal.tsx      +50 satır (ConfidenceTierBadge + weak banner)
MOD   apps/web/src/app/ag-envanteri/Client.tsx             +50 satır (ConfidenceCell — 3-renk chip + tooltip)

NEW   docs/R2_SOURCE_YIELD_AUDIT.md
NEW   docs/R2_CLASSIFICATION_CORRECTION_REPORT.md          (bu dosya)
MOD   TASK_BOARD.md
MOD   docs/MVP_RESCUE_PLAN.md                              (sequencing tweak)
```

Kapsam dışı bırakılanlar: scheduler, rapor sayfası, frequency_correction wiring, R3.

## 6. Eklenen Testler (21)

`internal/dude/classify_weak_test.go`:

| Test | Doğrulanan davranış |
|---|---|
| `TestWeak_AP_KuleMatchesViaFallback` | "Kule-12-Anamur" → AP, conf=45, evidence row var |
| `TestWeak_AP_OmniMatchesViaFallback` | "Omni-Merkez" → AP, conf=45 |
| `TestWeak_BackhaulLink_MimosaMatchesViaFallback` | "Mimosa-Iskele" → BackhaulLink, conf=45 |
| `TestWeak_BackhaulLink_RocketMatches` | "Rocket-Tepe" → BackhaulLink |
| `TestWeak_Bridge_TrailingBridgeMatches` | "merkez-bridge" → Bridge (sondan token) |
| `TestWeak_CPE_KonutMatchesViaFallback` | "Konut-902" → CPE |
| `TestWeak_CPE_HomeMatchesViaFallback` | "Home-Saglik" → CPE |
| `TestWeak_Router_AggMatchesViaFallback` | "Agg-Pop-Anamur" → Router (agg+pop tek kategori) |
| `TestWeak_Router_RbMatchesViaFallback` | "Rb-Anamur" → Router |
| `TestWeak_Ambiguous_KuleMimosaRemainsUnknown` | "Kule-Mimosa" (AP+Backhaul) → Unknown |
| `TestWeak_Ambiguous_OmniKonutRemainsUnknown` | "Omni-Konut" (AP+CPE) → Unknown |
| `TestWeak_NoMatch_StaysUnknown` | "300-OREN" → Unknown (token yok) |
| `TestWeak_EmptyName_StaysUnknown` | "" → Unknown |
| `TestWeak_StrongEvidenceOverridesWeak_DudeTypeRouter` | type=router + MAC → Router (weak fire etmez) |
| `TestWeak_StrongEvidenceOverridesWeak_WirelessModeAP` | wireless-mode=ap-bridge → AP (weak fire etmez) |
| `TestWeak_ConfidenceCapEnforced` | 7 case loop — hep 45, asla >50 |
| `TestWeak_EvidenceRowExposesHeuristicAndReason` | "kule-iskele" — heuristic="weak_name_pattern", reason'da token + cap açıklaması |
| `TestWeak_TokenBoundary_TapDoesNotMatchAP` | "tap.merkez" — "tap" token'ı "ap"'a eşleşmemeli |
| `TestWeak_TokenWithDigitSuffix_AP1Matches` | "ap1-anamur" — "ap1" token'ı "ap" + digit kuralıyla AP |
| `TestWeak_PrimaryNameHintAlreadyClassifies_WeakStaysOff` | "AP-Sahil-1" mevcut name_hint'le AP, weak fire etmez |
| `TestWeak_LowConfidencePrimaryStillWeakBucket` | "Cpe-12" → CPE confidence < 50 (bucket invariant) |

## 7. Quality Gate Sonuçları (sandbox @ R1 + R2 working tree)

```
$ gofmt -l .                                          → boş        RC=0
$ go vet ./...                                        → boş        RC=0
$ go build ./...                                      → boş        RC=0
$ go test -count=1 -short ./...                       → 16 ok / 16 no-test, 0 fail   RC=0
$ go test -short ./internal/networkactions/... -v | grep -c "^--- PASS"  →  138
$ go test -short ./internal/dude/...           -v | grep -c "^--- PASS"  →   59  (+21)
$ go test -short ./apps/api/internal/http/...  -v | grep -c "^--- PASS"  →   16
$ apps/web && tsc --noEmit                            → boş        RC=0
$ apps/web && npm run build                           → NOT RUN — sandbox 45s wall-clock
```

## 8. Live / Lab Smoke

**Yapılmadı.** Sandbox'ta DB erişimi yok; lab DB `194.15.45.62` arkasında. Operatör tarafı şu adımlar gerekli:

1. R2 patch'i uygula (R2_PATCH bundle'ı; aşağıda).
2. `bash scripts/db_migrate.sh up` — *yeni migration yok*, sadece sanity.
3. API + worker'ı yeniden başlat.
4. `/ag-envanteri` → "Discovery Çalıştır" → tamamlanmasını bekle.
5. `R2_SOURCE_YIELD_AUDIT.md §5`'teki SQL'i koştur:
   - Pre-R2 baseline (eski main'de) ile post-R2 (yeni run) sayılarını karşılaştır.
6. `/` dashboard'da "Sınıflandırma Kovaları" panelinde "Hedef %70 non-Unknown" progress bar'ının %70 üstüne çıkıp çıkmadığını gözlemle.
7. En az 10 yeni-sınıflandırılmış cihaza tıkla (`/ag-envanteri` row name → modal) ve `weak_name_pattern` evidence row'unun göründüğünü doğrula.

Bu adımlar tamamlanıp Unknown oranı %30 altına düşene kadar R2 **product-validation pending**.

## 9. Kalan Blocker'lar

1. **Live lab smoke** — yukarıda. R2 product-closed için zorunlu.
2. **Token listesi tuning** — eğer lab smoke Unknown oranını %30 altına çekemezse, gerçek isim çıktısını gözleyip eksik token'lar için minik bir takip PR'ı (`R2.1` token tuning).
3. **R3 başlatma ön koşulu** — R2 product-closed olmadan R3 (gerçek wireless lab target + okunabilir aksiyon sonuçları) başlamaz; mevcut MVP_RESCUE_PLAN bunu zorluyor.

## 10. Onur / Operatör-Usable Delta

> An operator can now open `/`, see Strong / Weak / Unknown bucket counts and a progress bar against the %70 non-Unknown target; click any device on `/ag-envanteri` and see if it was classified by strong evidence or by weak_name_pattern; and on the table see a 3-color confidence chip (yeşil güçlü / sarı zayıf / kırmızı düşük) with a tooltip explaining the tier.

Hiçbir destructive aksiyon eklenmedi, hiçbir master switch flip edilmedi, hiçbir mevcut Phase 5/6/7/8/9/10 davranışı değişmedi.
