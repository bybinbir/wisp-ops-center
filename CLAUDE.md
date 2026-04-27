CLAUDE.md PROMPT

F:\WispOps\wisp-ops-center\ projesi için mevcut repo durumunu okuyarak kısa, net ve uygulanabilir bir CLAUDE.md dosyası oluştur.

Bağlam:
- Proje adı: wisp-ops-center
- Amaç: MikroTik + Mimosa tabanlı WISP ağları için operasyon karar platformu.
- Ana soru: “Bugün ağda ne bozuk, kime müdahale etmeliyim, hangi link riskli?”
- Faz 1-5 tamamlandı.
- Aktif faz: Faz 6 — Customer Signal Scoring + Problem Customer Detection.
- Kalan fazlar: Faz 7 Reports + Work Orders, Faz 8 Frequency Recommendations, Faz 9 Controlled Apply + Rollback, Faz 10 Production Hardening.

CLAUDE.md kısa olmalı. Roman yazma. Maksimum 1-2 ekranlık net proje anayasası olsun.

İçerik başlıkları:
1. Project Identity
2. Current State
3. Active Phase
4. Non-Negotiable Safety Rules
5. Tech Stack
6. Git Workflow
7. Quality Gates
8. Phase 6 Focus
9. Final Report Format

Zorunlu kurallar:
- Soru sorma, onay isteme.
- Repo’yu baştan kurma; mevcut dosyaları oku ve devam et.
- Main branch üzerinde çalışma.
- Her fazda ayrı branch kullan.
- Fake telemetry, fake score, fake success yasak.
- Cihaz config write, frequency apply, scan activation, bandwidth-test, Mimosa write yasak.
- Telemetri yoksa data_insufficient dön.
- Secret’ları log/API/audit/docs/commit içine yazma.
- Her mutating action audit edilmeli.
- Test/build geçmeden tamamlandı deme.

Faz 6 odağı:
- customer wireless health score
- AP health score
- tower risk score
- sorunlu müşteri tespiti
- AP-wide degradation
- CPE alignment diagnosis
- work-order candidates
- scoring thresholds
- Sorunlu Müşteriler ekranının gerçek veriyle çalışması
- SSH host key enforcement ve RouterOS TLS verify runtime kapanışı

Kalite komutları:
- gofmt -l .
- go vet ./...
- go test ./...
- go build ./...
- npx tsc --noEmit
- npm run build

Teslim:
Sadece CLAUDE.md içeriğini üret.