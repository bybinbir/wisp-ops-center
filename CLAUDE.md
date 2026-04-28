# CLAUDE.md — wisp-ops-center

WISP (Wireless ISP) operasyon karar platformu için proje anayasası. Her oturum başında oku ve uy.

## 1. Project Identity

- **İsim:** wisp-ops-center
- **Hedef:** MikroTik + Mimosa tabanlı WISP ağları için operasyon karar platformu.
- **Ana soru:** "Bugün ağda ne bozuk, kime müdahale etmeliyim, hangi link riskli?"
- **Tek doğru ölçü:** sahada yanlışlıkla atılan tek bir komut, bin satır kod kadar maliyetlidir. Read-only güvenlik kapısı her şeyin üstünde.

## 2. Current State

- ✅ **Tamamlandı (main HEAD: `927c711`):** Faz 1, 2, 3, 4, 5, 6, 7, 8, 8.1, 9, 9 v2, 9 v3, 10A, 10B, 10C.
- 🟢 **Aktif:** Faz 10D — destructive happy-path lifecycle (gate ON + window aktif + RBAC granted; Execute() hâlâ stub).
- ⏳ **Sıradaki:** Faz 10E (gerçek mutation execution: tek action, tek device, full rollback drill), Faz 10F (production hardening: TLS uçtan uca, KMS, alerts, multi-tenant RBAC, SOC2/KVKK).
- **Safety invariants** her fazda korundu: `0 destructive succeeded`, `0 mutation cmd`, `0 secret leak`, `0 raw MAC`, master switch fail-closed (legacy global + provider toggle, iki katman).

## 3. Active Phase: Faz 10D — Destructive Happy-Path Lifecycle

- **Kapsam:** `runDestructiveActionAsync` 8-step pre-gate'i geçer; Execute() çağrısına kadar varır; Execute() **hâlâ** `ErrActionNotImplemented` döner; lifecycle audit eksiksiz emit eder.
- **Senaryo matrisi:** {toggle on/off} × {window aktif/yok} × {RBAC granted/denied} × {idempotency yeni/yeniden}.
- **Yeni audit event'leri:** `network_action.execute_attempted`, `network_action.execute_not_implemented` (gate_pass'ten sonra emit).
- **Cihaza yazılan tek byte yok.** Gerçek mutation Faz 10E'ye saklı.
- **DoD:** master switch açıkken bile `0 destructive succeeded`, `0 mutation cmd`, `0 secret leak`, `0 raw MAC`; lifecycle 5+ event/run; 3× post-merge migration replay temiz.

## 4. Non-Negotiable Safety Rules

1. **Soru sorma, onay isteme.** Karar gerektiğinde en güvenli profesyonel seçimi yap.
2. **Repo'yu baştan kurma;** mevcut dosyaları oku ve devam et.
3. **Main branch üzerinde çalışma.** Her iş kendi branch'inde: `phase/NNN-kebab-case`, `chore/...`, `fix/...`.
4. **Fake telemetry / fake score / fake success yasak.** Veri yoksa `data_insufficient` dön.
5. **Cihaz config write, frequency apply, scan activation, bandwidth-test, Mimosa write** — Faz 10E onayına kadar **hepsi yasak**.
6. **Secret'lar log/API/audit metadata/docs/commit içine yazılmaz.** Audit redaction zorunlu; raw MAC maskelenir (`AA:BB:CC:DD:EE:**`).
7. **Her mutating action audit'lenir** (`work_order.*`, `network_action.*`, `scoring_threshold.updated`, vb.).
8. **Test/build geçmeden "tamamlandı" deme.** Quality gates eksiksiz çalışmalı.
9. **Master switch fail-closed iki katmanlı:** legacy `DestructiveActionEnabled = false` + provider `MemoryToggle.Enabled() = false`. İkisini birden ON yapmak Faz 10E'nin ilk kapısı.

## 5. Tech Stack

- **Backend:** Go 1.21+, pgx (PostgreSQL), chi router, prometheus client. Monorepo: `apps/api/`, `apps/worker/`.
- **Worker:** scheduler engine + JobCatalog (`apps/worker/internal/`).
- **Web:** Next.js 14 + TypeScript + App Router + Türkçe UI (`apps/web/`).
- **DB:** PostgreSQL 14+; migrations `migrations/` altında numaralı, **idempotent + transactional** (BEGIN/COMMIT, IF NOT EXISTS).
- **Adapters:** `internal/adapters/{mikrotik,snmp,ssh,mimosa}` — allowlist + sanitize katmanları zorunlu, mutation token'ları `denyMutationSegments`/`denyMutationTokens` ile reddedilir.
- **Safety katmanı:** `internal/networkactions/` — DestructiveToggle, RBACResolver, MaintenanceProvider, EnsureDestructiveAllowedWithProviders 8-step pre-gate.

## 6. Git Workflow

- **Branch isimlendirme:** `phase/NNN-kebab-isim` (yeni faz), `chore/...` (hijyen), `fix/...` (bug).
- **Conventional commits:** `feat(phase-10d): ...`, `fix(networkactions): ...`, `chore(docs): ...`.
- **PR template:** scope + safety invariants + test count diff + smoke senaryo özeti + DB invariant tablosu.
- **Merge:** squash; main fast-forward; feature branch silinir.
- **Pre-merge:** bağımsız review (kod + smoke + invariantlar), gerekiyorsa pre-merge fix commit'i.
- **Post-merge:** 3× idempotent migration replay + full repo test + `npm run build` + lab smoke regression.

## 7. Quality Gates

"Tamamlandı" demek için **hepsi yeşil** olmalı:

- `gofmt -l .` → boş çıktı.
- `go vet ./...` → clean.
- `go test ./...` → full repo PASS.
- `go build ./...` → clean.
- `cd apps/web && npx tsc --noEmit` → type-clean.
- `cd apps/web && npm run build` → yeşil.
- Yeni migration: `bash scripts/db_migrate.sh up` 3× ardışık idempotent (NOTICE: skipping; exit=0).
- Lab smoke: en az happy-path + 1 fail-closed senaryo + 1 idempotency reuse senaryosu.

## 8. Active Phase Focus (Faz 10D)

- `internal/networkactions/runtime.go` — `runDestructiveActionAsync` Execute branch'i.
- Yeni audit event'leri: `execute_attempted`, `execute_not_implemented`; `DestructiveAuditCatalog`'a ekle.
- Smoke matris: toggle on × window aktif × RBAC granted = gate_pass + execute_attempted + execute_not_implemented + finish(failed/error_code=action_not_implemented).
- Test paketleri: `internal/networkactions/runtime_test.go` happy-path + matrix dekomposizyonu, hedef +12-15 yeni test.
- Migration **gerekmiyor** (Faz 10C'nin 000013'ü yeterli); sadece kod + audit catalog.
- **Bitmiş kabul kriteri:** master switch açıkken bile sahaya tek mutation gitmediğini DB ve audit jsonb'larında kanıtla.

## 9. Final Report Format

Her faz tamamlandığında final raporda **bunlar**:

1. **Branch + commit zinciri** — `git log --oneline main..HEAD`.
2. **PR linki + squash-merge SHA** — main HEAD doğrula.
3. **Test count diff** — önceki faz X → bu faz Y (+Z yeni test).
4. **Migration replay kanıtı** — 3× ardışık `NOTICE skipping`, exit=0.
5. **Smoke senaryo tablosu** — numara + ad + beklenen + gerçekleşen + status.
6. **DB invariant özeti** — dry_run sayacı, `destructive succeeded`, mutation cmd, secret leak (3 kaynak: api log + audit metadata + result jsonb), raw 6-octet MAC.
7. **Audit event count** — her event_type için count, lifecycle bütünlüğü kanıtı.
8. **Açık borçlar / sıradaki adım** — `current.md`'ye yansıt, hardening backlog güncelle.

---

**İlgili hafıza:** `F:\WispOps\.claude_memory\current.md` (aktif iş + son durum), `MEMORY.md` (oturumlar arası indeks).
**Operasyonel kural:** Üzerinde çalışılan iş ne olursa olsun, sahaya bir paket bile yanlış gitmesin. Read-only kuralını her commit'te yeniden kanıtla.
