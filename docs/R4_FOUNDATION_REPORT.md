# Faz R4-1 — Operator-First POP/IP Topology Temeli

**Tarih:** 2026-04-29
**main HEAD (R4-1 öncesi):** `053434a`
**Branch:** `phase/r4-foundation`
**PR başlığı:** `Phase R4-1: Operator-first POP/IP foundation (no probes)`
**Kapsam:** kod + migration + testler. Cihaz probe motoru R4-3'te, YAML import R4-2'de.

## Karar zinciri (özet)

WISP-R4-DUDE-TO-POP-OPS-FINISH prompt'u bir mimari kararla başladı: Dude'un binary DB'sini Go'da parse etmek yerine **operatörün YAML POP/IP envanterini birinci kaynak yap, Dude'u sadece secondary enrichment olarak tut**. Bu kararla:

- Dude binary parser kritik path'ten çıktı (R4'e bağlanmaz; gerekirse ayrı epic)
- Operatörün topology bilgisi sahteden değil, otantik bir kaynaktan (kendisinden) gelir
- Manuel mapping vs probe sonucu çelişkisi `mapping_conflict` state'iyle dürüstçe yüzeye çıkar — sessiz overwrite yapılmaz

R4 sıralı PR'lara bölündü. R4-1 (bu PR) **temel chassis**: schema, credential vault, redaction, allowlist, concurrency, scheduler iskelet.

## Bu PR'da neler var (12 dosya)

### Migration (1 dosya, 13 yeni tablo + ALTER)

`migrations/000014_phase_r4_dude_pop_ops.sql`

| Tablo | Amaç |
|---|---|
| `pop_topology_imports` | YAML import audit (kim/ne zaman/redacted yaml/sayım/conflict) |
| `pop_groups` | POP grupları (operator-defined veya auto-resolved) |
| `pop_device_membership` | Cihaz → POP eşleşmesi + resolution_rule + reason |
| `device_probe_runs` | Her read-only probe koşusunun lifecycle kaydı |
| `device_raw_snapshots` | Redacted ham probe yanıtı (forensic) |
| `device_interfaces` | Normalize edilmiş interface listesi |
| `device_wireless_interfaces` | Frekans / SSID / mode / TX power / noise floor |
| `device_wireless_clients` | Registration table (signal / CCQ / rate / uptime) |
| `device_bridge_ports` | Bridge port detayı |
| `device_neighbors` | `/ip neighbor` çıktısı |
| `device_link_metrics` | Mimosa link telemetrisi (RSSI/SNR/throughput) |
| `weak_client_findings` | CPE/customer verimsizlik raporu (risk_score + reasons) |
| `frequency_plan_runs` | Dry-run frekans planı (SQL constraint: `mutation = false`) |
| `network_devices.r4_*` | Yeni sınıflandırma kolonları + `mapping_conflict` state |

İdempotent + transactional. CREATE/ALTER only, no DROP. Master switch (Phase 10A-C) etkilenmez.

### `internal/credentials/profile_env.go` + test

Per-class env loader. Operatör onayıyla kabul edilen şema:

```env
WISP_MIKROTIK_ROUTER_USERNAME=
WISP_MIKROTIK_ROUTER_PASSWORD=
WISP_MIKROTIK_AP_USERNAME=
WISP_MIKROTIK_AP_PASSWORD=
WISP_MIMOSA_A_USERNAME=
WISP_MIMOSA_A_PASSWORD=
WISP_MIMOSA_B_USERNAME=
WISP_MIMOSA_B_PASSWORD=
WISP_SNMP_V2_COMMUNITY=
WISP_SNMP_V3_USERNAME=
WISP_SNMP_V3_AUTH_PASSWORD=
WISP_SNMP_V3_PRIV_PASSWORD=
```

Sözleşmeler:

- Yarım yapılandırma (sadece USERNAME veya sadece PASSWORD) `error` döner — probe katmanı 928 sahte `credential_failed` üretmeden önce operatör uyarılır
- Hata mesajları **env değerlerini asla** echo'lamaz; sadece env anahtar isimleri görünür
- `Sanitize()` log/audit/HTTP sınırından geçmeden önce secret'ları `***` ile değiştirir
- 7 hermetic test, "no secret leak" testi dahil

### `internal/devicectl/redact.go` + test

Hassas alan redaction'ı için merkezi modül.

- `RedactText(string)` — düz metin probe çıktısındaki `key=value` kalıplarını maskeler (RouterOS CLI, SSH transcript)
- `RedactStructured(any)` — map/slice/struct içindeki hassas alan değerlerini reflect tabanlı maskeler
- `RedactJSONBytes([]byte)` — JSON byte slice round-trip; parse hata verirse text fallback
- `RedactionVersion = "v1"` — `device_raw_snapshots.redaction_version` kolonu bu değeri tutar
- Hassas alan deseni: `password|secret|token|api-key|auth-key|private-key|wpa-psk|pre-shared-key|ppp-secret|wireless-password|community|bearer|credentials|session-(id|key|token)|cert-(private|priv)`
- 7 hermetic test, JSON ve text path'leri dahil

### `internal/devicectl/allowlist.go` + test

Read-only enforcement (whitelist + mutation guard çift kilit).

| Allowlist | Komut sayısı |
|---|---:|
| `MikrotikReadOnlyCommands` (RouterOS API + SSH normalize) | 27 |
| `MimosaReadOnlyEndpoints` | 9 |
| `SNMPReadOnlyOIDPrefixes` | 6 |
| `MutationTokens` (veto listesi) | 22 |
| `LogPrintAllowedFilters` (`/log print where ...`) | 4 |

API'lar: `EnsureMikrotikCommand(cmd)`, `EnsureMimosaEndpoint(path)`, `EnsureSNMPOID(oid)`. Mutation token whitelist içinde **hiçbir zaman** olmamalı invariant'ı bir testle korunuyor.

11 hermetic test:

- Normalize SSH ↔ API formats
- Whitelist allowed cases (13 örnek)
- Mutation veto cases (12 örnek: set/reboot/upgrade/import/export/user-set/cert-export/...)
- `/log print` filter constraint
- Mimosa endpoint allowlist + denylist
- SNMP OID prefix matching
- "no mutation token in whitelist" invariant

### `internal/devicectl/concurrency.go` + test

Probe katmanı için concurrency limiter.

- `DefaultMaxConcurrentProbes = 10` (operatör onayı)
- `HardConcurrencyCeiling = 20` (config ne derse desin geçilemeyen tavan)
- `Acquire(ctx)` context iptaline saygı; iptal halinde token kaybı yok
- `Release` re-entrant defensive (çoklu çağrı sessizce yutulur)
- `InFlight()` / `PeakInFlight()` gözlem metrikleri
- 7 hermetic test, paralel 50 worker stress dahil

### `internal/devicectl/scheduler.go` + test

Probe scheduler iskelet.

- `ScanMode`: `light` | `deep` | `on_demand`
- `ScanRequest` validation: `light/deep` hedef gerektirmez; `on_demand` PopCode veya DeviceIDs gerektirir; ikisi birden çelişkidir
- `Probe` arayüzü (R4-3'te `mikrotikProbe`, R4-4'te `mimosaProbe` ile dolacak)
- `ProbeOutcome` dürüst status enum'u: `succeeded | partial | timeout | unreachable | credential_failed | protocol_error | parser_error | blocked_by_allowlist | unknown`
- `ProbeOutcome.Sanitize()` error message'taki secret kalıntılarını redact eder
- `DefaultScanWindow()`: light 20dk, deep 22:00→06:00 (operatör config ile override)
- 7 hermetic test

## Yapılmayan, sonraki PR'lara bağlı olan işler (transparan kaydet)

| Slice | İçerik |
|---|---|
| **R4-2** | YAML envanter şeması + parser + DB import + audit log + mapping_conflict detector |
| **R4-3** | MikroTik probe motoru (RouterOS API → SSH → SNMP fallback chain) + per-class profile binding |
| **R4-4** | Mimosa probe motoru (HTTP/API → SSH → SNMP fallback) |
| **R4-5** | Classifier (multi-source priority) + POP resolver (manual → metadata → name → subnet → unknown_pop) + persistence |
| **R4-6** | API: `/api/inventory/summary`, `/api/pops`, `/api/pops/:id`, `/api/pops/:id/analyze`, `/api/pops/:id/frequency-plan` (dry-run), `/api/discovery/run-readonly` |
| **R4-7** | Web UI: POP dashboard + detail tabs (AP/Mimosa/CPE/Frequency Plan) + evidence modal R4 enriched + read-only badge + live read-only smoke + final rapor |

## Kalite kapısı (R4-1 — bu turda yeşil)

| Gate | Durum |
|---|---|
| `gofmt -l internal/credentials internal/devicectl` | clean |
| `go vet ./...` | clean |
| `go build ./...` | clean |
| `go test -count=1 -short ./...` | **17/17 paket OK, 0 FAIL** |
| `go test -count=1 -short ./internal/credentials/...` | OK (8 test) |
| `go test -count=1 -short ./internal/devicectl/...` | OK (32 test) |
| Mutation token in whitelist invariant | green |
| No secret leak in error messages | green |
| Concurrency cap respected under 50-worker stress | green |

Frontend dokunulmadı, `tsc --noEmit` etkilenmiyor.

## Güvenlik invariant'ları (R4-1 sözleşmesi)

- Hiçbir destructive runtime path açılmadı
- Master switch (Phase 10A-C) ve `network_action_runs` constraint'leri etkilenmedi
- Yeni `frequency_plan_runs.mutation` kolonunda `CHECK (mutation = false)` SQL constraint
- `device_raw_snapshots.payload_redacted` JSONB; redaction version v1
- Credential profilleri RAM içinde, `Sanitize*()` her sınır geçişinde maskeleyici
- Read-only allowlist 22 mutation token'ı veto eder; whitelist içinde hiçbir mutation token yoktur (invariant testi)

## Operatöre next steps

1. **R4_PATCH bundle'ı uygula:** `cd F:\WispOps\R4_PATCH; .\APPLY_R4_1.ps1`
2. **Migration smoke:** `psql -d wispops -f migrations/000014_phase_r4_dude_pop_ops.sql` veya app migrate
3. **Env vars:** `WISP_MIKROTIK_ROUTER_*`, `WISP_MIKROTIK_AP_*`, `WISP_MIMOSA_A_*`, `WISP_MIMOSA_B_*`, opsiyonel `WISP_SNMP_*` set et — secret'ları `.env` veya secret store'a koy, kodda hiçbir şey yok
4. **R4-2 hazırla:** YAML POP envanteri (ZIRVE, KOTEKLER, KADILAR, ...) — operatörden tek bir liste alınca import path açılır, probe motorları konuşacak gerçek hedefler bulur

## Verdict

**R4-1 product-ready foundation. PR temiz, tests yeşil, kod prompt sözleşmesini koruyor.** Ürün-uçtan-uca kapatma R4-2..R4-7 ile gelecek; bu PR onların üzerine kuracağı zemin.
