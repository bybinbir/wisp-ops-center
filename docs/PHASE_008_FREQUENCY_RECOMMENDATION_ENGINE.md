# Phase 8 — Frequency Recommendation Engine (Plan)

**Branch:** `phase/008-frequency-recommendation-engine`
**Önkoşul:** Phase 7 main üstünde merge edildi (`685b23f`), baseline cleanup `95e94b6`. `customer_signal_scores`, `ap_health_scores`, `tower_risk_scores`, `work_orders` artık baseline.
**Statü:** **PLAN ONLY** — bu PR sadece dokümantasyondur. Kod değişikliği yok. Detay implementasyon ayrı PR'larda.

Phase 8 ana hedefi: **read-only sürveyans verisinden frekans riski analiz et + öneri üret. Apply YOK, operator approval YOK, device mutation YOK.**

---

## 1. Hedef ve Kapsam

WISP operasyon merkezi şu an:
- AP/CPE telemetrisi okuyabiliyor (Phase 3-4).
- Müşteri sinyal skoru üretiyor (Phase 6).
- AP health + tower risk skorluyor (Phase 6).
- İş emri yönetiyor (Phase 7).

**Eksik:** Operatör "bu AP'nin frekansını değiştirsem skor düzelir mi?" sorusuna kanıta dayalı yanıt alamıyor. Phase 8 bu boşluğu read-only öneri ile doldurur.

### Phase 8 yapacaklar

1. `frequency_recommendations` tablosu — öneri ve gerekçe.
2. `internal/recommendations/` paket — gerçek implementasyon (Phase 1'de iskelet vardı).
3. AP/client/link read-only survey verisinden frekans risk modeli.
4. **Co-channel** / **adjacent-channel** / **noise-floor** / **SNR** / **CCQ** / **retry rate** / **modulation drop** kompoziti.
5. Öneri formu: "AP X için kanal değişikliği {from → to}, beklenen iyileşme: skor +N puan, gerekçe ...".
6. Confidence score (0..100): kararsız kanıtlarda düşük, net kanıtlarda yüksek.
7. Audit trail zorunlu: her öneri üretimi `recommendation.created` / `recommendation.dismissed` / `recommendation.archived`.
8. UI: `/frekans-onerileri` sayfası gerçek veriyle.

### Phase 8 yapmayacaklar (sınırlar)

- ❌ Apply (AP/CPE/PtP konfigine yansıtma).
- ❌ Operator approval workflow (Phase 9'a ertelendi).
- ❌ Device mutation.
- ❌ Channel scan / spectrum analysis tetikleme (Phase 9'da bakım penceresinde).
- ❌ Bandwidth-test, ping flood, throughput test.
- ❌ Otomatik öneri "auto-apply" — hiçbir koşulda.

Apply kapısı **kontrollü apply** olarak Phase 9'a aittir; o aşamada backup + verification + automatic rollback gibi güvenlik katmanları eklenir.

---

## 2. Veri Modeli

### 2.1 `frequency_recommendations`

```
id                       uuid pk
ap_device_id             uuid → devices(id) ON DELETE CASCADE
tower_id                 uuid → towers(id)  ON DELETE SET NULL
band                     text  CHECK ∈ {'2.4ghz', '5ghz', '6ghz', 'sub_6ghz_ptp'}
current_channel          int        -- şu anki merkez frekans (MHz)
current_channel_width    int        -- 20 / 40 / 80 / 160 MHz
recommended_channel      int        -- önerilen merkez frekans
recommended_width        int
expected_improvement     int        -- AP score'da öngörülen artış (-100..+100)
confidence               int        -- 0..100
reasons                  jsonb      -- ["co_channel_overlap_with_ap_X","high_noise_floor",...]
risk_breakdown           jsonb      -- her risk faktörünün katsayısı
status                   text  CHECK ∈ {'open','dismissed','expired','superseded'}
generated_by             text       -- 'scheduler' | 'manual' | 'recompute'
notes                    text
created_at               timestamptz default now()
updated_at               timestamptz default now()
```

İndeksler:
- `(ap_device_id, status, created_at DESC)`
- `(tower_id, status, created_at DESC)` partial WHERE not null
- `(status, confidence DESC)` — UI önceliklendirme

### 2.2 `frequency_observations` (opsiyonel — survey snapshot)

Modele girdi olan ham gözlemler. Phase 6'daki telemetry tablolarından türetilir; bu yüzden ayrı tablo şart değil. Ama snapshot tutmak rapor + audit için faydalı:

```
id, ap_device_id, observed_at, channel, channel_width, noise_floor_dbm,
tx_retries, modulation_index, neighbor_aps jsonb (id, channel, rssi),
client_count, avg_ccq, ...
```

Kararı kod review aşamasında verilir; şu an plan: **Türetme** — telemetry tablolarını kullan, ayrı snapshot yok. Snapshot ihtiyacı doğarsa eklenir.

### 2.3 `scoring_thresholds` ek anahtarları (öneri)

| Anahtar | Varsayılan | Anlam |
|---|---|---|
| `freq_co_channel_max_neighbors` | 2 | Aynı kanalda kaç komşu AP "kötü" sayılır |
| `freq_adjacent_channel_max_overlap` | 50 | Bitişik kanal örtüşme yüzdesi (rapor amaçlı) |
| `freq_noise_floor_warning_dbm` | -85 | NFR'nin altı uyarı |
| `freq_noise_floor_critical_dbm` | -75 | Kritik |
| `freq_recommendation_confidence_min` | 50 | UI'da varsayılan filtre (>= bu confidence) |
| `freq_recommendation_cooldown_hours` | 12 | Aynı AP için yeniden öneri üretimi cooldown |

Threshold validation `internal/scoring/thresholds.go` içinde aynı pattern.

---

## 3. Risk Modeli

Per-AP frekans riski hesabı, deterministik bir penalty toplamı (Phase 6 skor motorunun benzeri).

### 3.1 Girdi

`internal/recommendations/inputs.go::Inputs`:

```go
type Inputs struct {
    APDeviceID         string
    Band               Band
    CurrentChannel     int
    CurrentWidth       int
    NoiseFloorDbm      *float64
    AvgCCQ             *float64
    AvgTxRetries       *float64
    ModulationDrop     *float64
    NeighborAPs        []NeighborAP    // (id, channel, rssi, vendor)
    CustomerCounts     CustomerCounts  // total, critical, warning
    APHealthScore      *int
}

type NeighborAP struct {
    DeviceID  string
    Channel   int
    RSSI      float64
    Vendor    string
    SameTower bool
}
```

Hydrator (Phase 6 hydrator pattern'i): `internal/recommendations/hydrator.go::HydrateAP(ctx, apID)` — telemetry / wireless_clients / customer_signal_scores / ap_health_scores'tan veri çeker.

### 3.2 Risk faktörleri

Her AP için:

| Faktör | Sinyal | Penalty |
|---|---|---|
| **co_channel_overlap** | `len(neighbors WHERE channel == current_channel) >= max_neighbors` | -20..-40 |
| **adjacent_channel_overlap** | komşu AP center freq farkı < channel_width yarısı | -10..-20 |
| **noise_floor_high** | NFR >= warning eşiği | -10..-20; >= critical eşiği -25..-35 |
| **ccq_low** | avg_ccq < ccq_warning_percent | -10 |
| **tx_retry_high** | avg_tx_retries > 15% | -10 |
| **modulation_drop** | modulation_drop_count_24h > 0 | -5..-15 |
| **ap_health_critical** | ap_health.severity = critical | -10 |

Skor 0..100. Yüksek skor → düşük risk, frekans değişikliği önerilmez. Düşük skor → öneri üretilir.

### 3.3 Öneri seçimi

Açık (open) öneri için:
1. Aynı band'da uygun (regulatory + width-uyumlu) tüm aday kanalları enumerate et.
2. Her aday kanal için aynı modeli simüle et (mevcut neighbor map ile).
3. En iyi `expected_improvement` veren kanalı seç.
4. `confidence = (data quality * sample size * dpenaltystability) * 100`.

**Önemli:** Bu simülasyon **yalnız analitik bir tahmindir** — gerçek RF ortamı non-deterministiktir. UI bu belirsizliği "öngörülen" olarak gösterir.

### 3.4 Cooldown

`freq_recommendation_cooldown_hours` içinde aynı AP için yeni öneri üretilmez. Mevcut açık öneri varsa `superseded` ile değiştirilir.

---

## 4. API

| Method | Path | Açıklama |
|---|---|---|
| GET    | `/api/v1/frequency-recommendations` | Filtre: status, ap_device_id, tower_id, band, min_confidence. |
| GET    | `/api/v1/frequency-recommendations/{id}` | Detay + risk_breakdown. |
| PATCH  | `/api/v1/frequency-recommendations/{id}` | status: dismissed / archived. **Apply YOK.** |
| POST   | `/api/v1/devices/{id}/recompute-frequency-recommendation` | Tek AP için yeniden hesaplat. |
| POST   | `/api/v1/recommendations/run` | Toplu hesaplama (max 200 AP). |

Hiçbir endpoint cihaza yazma yapmaz; tüm kayıtlar `frequency_recommendations` tablosunda kalır.

---

## 5. Scheduler

Yeni job: `frequency_recommendation_analysis` (mevcut `JobFrequencyRecommendationAnaly` Phase 1'den iskelet olarak vardı, **risk=medium, enabled=false**). Phase 8'de **risk=low, enabled=true** olarak güncellenir çünkü read-only.

Davranış:
- Tüm aktif AP'ler için hydrate + skorla.
- Yeni öneriler `frequency_recommendations` tablosuna insert.
- Cihazla iletişim sadece SNMP/RouterOS API read-only — Phase 3-4 adapter'ı.
- Apply yok, mutation yok.

---

## 6. UI

`/frekans-onerileri` sayfası Phase 1'den iskelet. Phase 8'de gerçek API ile beslenir.

Filtre:
- status (default: open)
- band (2.4 / 5 / 6)
- min_confidence (default 50)
- tower_id, ap_device_id

Detay:
- Mevcut kanal vs. önerilen kanal
- Beklenen iyileşme (AP skor delta)
- Confidence
- Risk breakdown (her faktör + katsayı)
- Komşu AP listesi (co-channel ve adjacent-channel)
- "Dismiss" / "Archive" butonu (Apply YOK)

Detail page'de **net bir banner**: "Bu öneri yalnızca analitik bir tahmindir. Apply ileri fazda kontrollü iş akışı ile yapılır."

---

## 7. Audit Trail

Mutating action'lar:
- `recommendation.created` (öneri üretildi)
- `recommendation.dismissed` (operatör reddetti)
- `recommendation.archived` (otomatik veya manuel)
- `recommendation.superseded` (yeni öneri eskinin üzerine)
- `frequency_threshold.updated` (eşik değişimi)

Ham secret yazılmaz; metadata yalnız AP ID, kanal numaraları, confidence, sebep listesi.

---

## 8. Test Stratejisi

Birim testler:
- `internal/recommendations/engine_test.go` — risk faktörü penalty matrisi, confidence formülü, cooldown davranışı.
- `internal/recommendations/repository_test.go` — duplicate guard (cooldown), supersede zinciri.
- `internal/scheduler/engine_test.go` — `JobFrequencyRecommendationAnaly` enabled durumu.

Integration smoke (Postgres lab gerekir):
- 1 AP + 3 komşu AP'lik mock dataset → öneri çıkması beklenen
- Cooldown içinde 2. çağrı superseded yapması
- Dismiss eden öneri tekrar üretilmemesi (cooldown_hours boyunca)
- "Net kanıt yok" datasetinde confidence < 50 olması
- Operatörün dismiss attığı öneri için `recommendation.dismissed` audit kaydı.

UI testleri: `/frekans-onerileri` mock-fallback YOK — Postgres bağlı değilse banner.

---

## 9. Güvenlik Sınırları

Phase 8 planının kabulü için aşağıdaki sınırlar **mutlaktır**:

- ❌ AP/CPE/PtP cihazlarına yazma yok.
- ❌ Frekans değiştirme yok (recommendation kaydı + UI gösterim).
- ❌ Bandwidth-test yok.
- ❌ Müşteri bağlantısını etkileyecek işlem yok.
- ❌ Otomatik apply yok (operatör onayı bile olsa Phase 9 gelene dek yok).
- ❌ Channel scan / survey aktivasyonu yok (cihaza komut göndermek).

İzinli:
- ✅ Telemetri okuma (Phase 3-4 adapter'ları).
- ✅ Risk modeli hesaplama.
- ✅ Öneri kayıtları + audit.
- ✅ UI gösterim.
- ✅ Operatör dismiss / archive.

---

## 10. Phase 9'a Hazırlık

Phase 9'da kontrollü apply geldiğinde:

- `frequency_recommendations.status` yeni değer alabilir: `pending_approval`, `approved`, `applying`, `verified`, `rolled_back`.
- Apply öncesi RouterOS config backup zorunlu.
- Apply maintenance window kısıtı.
- Multi-actor onay zinciri.
- Apply sonrası automatic rollback (link kapanırsa, müşteri sayısı düşerse).

Phase 8 bu state machine'in sadece `open / dismissed / expired / superseded` kısmını implement eder.

---

## 11. Açık Borçlar (Phase 7'den taşınan)

Phase 8 kapsamında Phase 7'nin iki açık borcu da kapatılmaya değer (öncelik düşük ama mantıken aynı release):

1. **Server-side gerçek PDF rendering** — `signintech/gopdf` ile yönetici özeti + iş emri raporları.
2. **Audit retention scheduler job** — `audit_retention_purge` (risk=low, weekly cadence). 90 gün politikası.

Bu ikisi Phase 8 ana hedefini engellemez; ayrı küçük PR'lar olarak sırayla merge edilebilir.

---

## 12. PR Sıralaması (Önerilen)

1. **Bu PR (plan):** `phase/008-frequency-recommendation-engine` → `main`. Sadece bu doküman.
2. PR'ler sırasıyla:
   - `phase/008-migration` — `frequency_recommendations` migration + threshold key seed.
   - `phase/008-engine` — `internal/recommendations/` engine + hydrator + repository.
   - `phase/008-scheduler` — `JobFrequencyRecommendationAnaly` enabled + worker handler.
   - `phase/008-api` — `/frequency-recommendations` API + audit.
   - `phase/008-ui` — `/frekans-onerileri` real implementation.
   - `phase/008-pdf-debt` — server-side PDF (Phase 7 borcu).
   - `phase/008-audit-retention-debt` — audit_retention_purge job (Phase 7 borcu).

Her PR'da: gofmt + vet + test + build + npm build kapıları yeşil olmadan merge yok.

---

## 13. Kabul Kriterleri (Phase 8 tamamlandığında)

- [ ] `frequency_recommendations` tablosu canlı; migration idempotent.
- [ ] `internal/recommendations/` engine + repository test coverage > %70.
- [ ] `/api/v1/frequency-recommendations` filtre, dismiss, archive çalışır.
- [ ] `/frekans-onerileri` UI mock-fallback'siz gerçek veri gösterir.
- [ ] Scheduler job 1 günde tüm aktif AP'leri hesaplar; max süre < 5 dakika / 100 AP.
- [ ] Audit trail: `recommendation.created` / dismissed / superseded.
- [ ] Lab Postgres + lab RouterOS ile end-to-end smoke (Phase 7 + Phase 8 ortak).
- [ ] **Hiçbir kod yolu cihaza yazma yapmıyor** — kanıt: `internal/adapters/mikrotik/allowlist.go` ve `internal/recommendations/` review.
- [ ] Phase 7 açık borçları (PDF + audit retention) ayrı PR'larda kapatıldı.

---

## 14. Test Plan (PR seviyesi)

- [ ] Plan PR (bu): doküman review only, kod değişikliği yok.
- [ ] Migration PR: `WISP_DATABASE_URL` ile lab Postgres üzerinde apply doğrulanır.
- [ ] Engine PR: in-memory test fixture'lar (mock NeighborAP setleri) ile penalty matris testi.
- [ ] Scheduler PR: lab worker boot + 1 AP için run.
- [ ] API PR: curl smoke + audit log doğrulama.
- [ ] UI PR: `/frekans-onerileri` görsel inceleme + filtre testi.
- [ ] Lab smoke result: `docs/PHASE_008_LAB_SMOKE_RESULT.md` (Phase 7'nin formatına benzer).
