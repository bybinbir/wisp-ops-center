# GitHub Workflow

Bu doküman wisp-ops-center'ın faz tabanlı geliştirme akışını GitHub üzerinde nasıl yürüttüğünü tanımlar.

## Repo

Üretim deposu: **https://github.com/bybinbir/wisp-ops-center**

Faz 2 GitHub aksiyonları:
- Repo Chrome MCP ile bybinbir hesabı altında oluşturuldu (Public, README/gitignore/license olmadan — yerel commit'leri kayıpsız aktarmak için).
- `main` branch yerel olarak Faz 1 baseline'ı taşıyor (`5510473`); `phase/002-device-inventory-credentials` branch'i 6 yeni commitle Faz 2'yi taşıyor.
- Sandbox ortamından HTTPS push kimlik bilgisi gerektiriyor. Aşağıdaki **Push Adımları** geliştirme makinesinde tek seferlik olarak uygulanır.

## Branş Yapısı

- `main` — sürümlenebilir, korunur. Doğrudan commit yasaktır.
- `phase/<NNN>-<özet>` — bir faz boyunca aktif olan branş. Örnekler:
  - `phase/001-foundation`
  - `phase/002-device-inventory-credentials`
  - `phase/003-mikrotik-readonly`
- `fix/<konu>` — küçük üretim hata düzeltmeleri.
- `docs/<konu>` — yalnızca dokümantasyon güncellemeleri.

## Pull Request Disiplini

1. Her faz, kendi branşında PR olarak `main`'e açılır.
2. PR başlığı `WISP-OPS-PHASE-<NNN>: <faz adı>`.
3. PR açıklaması Faz Sonuç Raporu'nun ilk üç bölümünü içerir.
4. PR merge edilmeden önce:
   - `go fmt`, `go vet`, `go test`, `go build` yeşil
   - `npm run typecheck`, `npm run build` yeşil
   - Tüm yeni güvenlik kuralları docs/SAFETY_MODEL.md ile uyumlu

## Commit Mesajı Konvansiyonu

[Conventional Commits](https://www.conventionalcommits.org/):
- `chore:`, `feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `perf:`

## Faz 2 Commit Listesi

```
1a72019 feat: add postgres migration runner and pgxpool
2c2ea7d feat: implement device inventory api
4521b0d feat: add credential profile vault foundation
ac2916f chore: ignore editor backup files (.go.NNN)
48bf97e feat: connect inventory ui to api
e50f334 docs: add ap to client test engine design
b9345b2 chore: ignore stray go build outputs (api, worker, *-go-tmp-*)
```

## Push Adımları (Geliştirme Makinesinde Bir Kez)

Sandbox kalıcı git kimliği tutmadığı için Faz 2 commit'leri yerel git data'sında (`/tmp/wispops-git`) kuruldu ve **work-tree** F:\WispOps üzerinden push'a hazır halde duruyor. Push'u kendi geliştirme makinenizden Powershell veya bash'te tek seferlik şu komutlarla tamamlayın:

```bash
cd F:/WispOps/wisp-ops-center

# (Eğer bu klasörde henüz git başlatılmadıysa)
git init -b main
git config user.name "<your-name>"
git config user.email "<your-email>"

# Tüm Faz 1 + Faz 2 değişikliklerini yerel olarak indeksle
git add -A
git commit -m "feat: phase 1 + phase 2 (initial publish)"

# Faz 2 branch'ı hazırlayın
git branch phase/002-device-inventory-credentials

# Remote bağlama + push
git remote add origin https://github.com/bybinbir/wisp-ops-center.git
git push -u origin main
git push -u origin phase/002-device-inventory-credentials

# PR açma (gh CLI varsa)
gh pr create \
  --base main \
  --head phase/002-device-inventory-credentials \
  --title "WISP-OPS-PHASE-002: Device Inventory & Credential Profiles" \
  --body "Faz 2 sonuç raporu için bkz. TASK_BOARD.md"
```

> **Not:** Faz başına temiz commit history korumak istiyorsanız sandbox'tan üretilmiş `/tmp/wispops-git` klasörünü makineye taşıyabilirsiniz. Yukarıdaki tek-commit yaklaşımı GitHub'da temiz bir başlangıç sağlar; faz başına commit ayrımı bir sonraki fazdan itibaren `main`'den branch açılarak doğal olarak korunur.

## Issue Etiketleme

- `phase:002`, `phase:003`, ...
- `area:backend`, `area:frontend`, `area:db`, `area:infra`
- `type:feature`, `type:bug`, `type:doc`
- `priority:low|medium|high`
- `safety:read-only` (yazma yapan değişikliklere `safety:write` etiketi zorunlu)

## Sürümleme

`main` etiketleri:
- v0.1.0 — Faz 1
- v0.2.0 — Faz 2 (bu faz sonu)
- v0.3.0 — Faz 3 (MikroTik salt-okuma)
- ...
- v1.0.0 — Faz 9 (controlled apply)
- v1.1.0 — Faz 10 (üretim sertleştirmesi)

## CI (Faz 10'da Eklenecek)

`.github/workflows/ci.yml`:
1. Setup Go + Node
2. `go vet`, `go test ./...`, `go build ./...`
3. `npm install --no-audit`
4. `npx tsc --noEmit`
5. `npx next build`
6. `gosec`, `govulncheck`
