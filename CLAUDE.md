# wisp-ops-center

Bu wispops projesinin ana kod deposu. Stack: Go (backend) + Next.js / TypeScript (web). Detaylı bağlam: `..\.claude_memory\projects\wispops.md`.

## Üst Workspace Talimatları

Workspace kökündeki talimatlar geçerli: `..\CLAUDE.md`. Hafıza dosyaları `..\.claude_memory\` altında. **Oturum başında onları oku.**

## Hızlı Yapı

- `apps/api/` — REST API
- `apps/worker/` — arka plan işçisi
- `apps/web/` — Next.js arayüz
- `internal/` — paylaşılan paketler (adapters, devices, links, scoring, vb.)
- `migrations/` — PostgreSQL şema migration'ları
- `infra/` — nginx, systemd, prometheus konfigürasyonları
- `docs/` — mimari dokümanlar

## Geliştirme Komutları

- `scripts/dev_run_api.sh` — API'yi yerel çalıştır
- `scripts/dev_run_worker.sh` — worker'ı çalıştır
- `scripts/dev_run_web.sh` — web'i çalıştır
- `scripts/db_migrate.sh` — migration'ları uygula
