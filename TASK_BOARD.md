# Görev Panosu — wisp-ops-center

Notasyon: ☐ yapılmadı · ▣ kısmen · ☑ tamamlandı.

---

## Faz 1 — Temel ve Ürün İskeleti  ☑
## Faz 2 — Cihaz Envanteri, Credential Profiles, GitHub Workflow  ☑
## Faz 3 — MikroTik Salt-Okuma Entegrasyonu  ☑
## Faz 4 — Mimosa Salt-Okuma + Credential & Transport Hardening  ☑

## Faz 5 — Scheduled Checks Engine + Safe AP→Client Tests + Transport Hardening Completion  ☑

- ☑ Scheduler engine (planner cron alt kümesi, JobCatalog, risk policy)
- ☑ Repository (scheduled_checks/job_runs/maintenance_windows pgx)
- ☑ Worker scheduler loop (env-gated, concurrency, timeout, job_runs)
- ☑ Maintenance windows model + API + risk enforcement
- ☑ Safe AP→Client runner (server-originated ping/loss/jitter/traceroute)
- ☑ Disabled tests: limited_throughput, mikrotik_bandwidth_test
- ☑ SNMPv3 USM secret runtime path (vault decrypt → adapter cfg)
- ☑ SSH host key TOFU foundation (insecure_ignore | tofu | pinned)
- ☑ Migration 000005 (maintenance_windows, scheduler_locks,
  ssh_known_hosts, scheduled_checks alanları, ap_client_test_results
  ek alanları, job_runs ek alanları)
- ☑ HTTP endpoints: scheduled-checks CRUD + run-now, job-runs,
  maintenance-windows CRUD, ap-client-test-runs/run-now,
  ap-client-test-results
- ☑ Frontend: Planlı Kontroller real CRUD, /job-runs, /ap-client-tests,
  sidebar nav genişletildi
- ☑ Dokümantasyon: SCHEDULED_CHECKS_ENGINE.md,
  AP_CLIENT_TESTS_RUNTIME.md, MAINTENANCE_WINDOWS.md,
  SSH_HOST_KEY_POLICY.md, README/TASK_BOARD güncel
- ☑ Birim testleri: scheduler (11), apclienttest (8), ssh hostkey (4)
- ☐ Gerçek lab Postgres + Redis + cihaz ile end-to-end (sandbox kısıtı)

## Faz 6 — Müşteri Sinyal Skorlaması (Telemetri + AP-Client Sonuçları)  ☐
## Faz 7 — Raporlar ve İş Emirleri  ☐
## Faz 8 — Frekans Öneri Motoru  ☐
## Faz 9 — Kontrollü Uygulama ve Rollback  ☐
## Faz 10 — Üretim Sertleştirmesi  ☐

---

## Faz Geçiş Kriteri

1. İlgili dokümantasyon güncel
2. Birim ve/veya entegrasyon testleri yeşil (gerçek DB içerenler için Postgres ile manuel doğrulama)
3. Faz başında belirlenen güvenlik kuralı ihlal edilmemiş
