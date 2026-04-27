# Scheduled Checks Engine (Faz 5)

`internal/scheduler` paketi Faz 5'te gerçek bir zamanlanmış-kontrol motoruna dönüştü. Bu doküman tasarımı, sözleşmeyi ve güvenlik sınırlarını anlatır.

## Sözleşme

- Her scheduled check tek bir `JobType` ile eşlenir; tipler `JobCatalog` üzerinden listelenir. Listede olmayan tip API tarafından reddedilir.
- `controlled_apply` modu Faz 9'a kadar **kabul edilmez**; `ScheduledCheckInput.Validate()` ve DB CHECK constraint'i onu reddeder.
- Yüksek riskli (`risk_level=high`) iş yalnızca `manual_approval` modunda kabul edilir. Yine de Faz 5'te yüksek riskli AP-client testleri (`mikrotik_bandwidth_test`, `limited_throughput`) `JobCatalog.Enabled=false` ile **devre dışı**.
- Cron parser kasıtlı olarak küçük bir alt küme — `M H` (daily), `M H D` (weekly DoW), `M H D` (monthly DoM), `interval` saniye veya `one_time` ISO datetime. Karmaşık ifadeler reddedilir; misconfig riskini düşürür.

## Engine + Worker Loop

`apps/worker/internal/scheduler_loop.go::RunSchedulerLoop` Faz 5'te env-gated:
- `WISP_SCHEDULER_ENABLED=true` ve `WISP_DATABASE_URL` set edilmediği sürece loop başlamaz; manuel çalıştırma (run-now) ve API her durumda mevcuttur.
- Tick'te `scheduled_checks` tablosundan `enabled=true AND next_run_at <= now()` satırlarını çeker, `job_runs` tablosuna `running` olarak kaydeder, registry'den handler'ı çalıştırır, sonucu loglar.
- Concurrency varsayılan 4 (en fazla 16). Per-job timeout varsayılan 60 sn.
- Asynq/Redis Faz 5'te opsiyoneldir; sözleşme `JobType` etrafında soyutlandığı için Faz 6'da Redis Streams takılırsa bu loop kalıcı olarak silinmeden değiştirilir.

## Run-Now

`POST /api/v1/scheduled-checks/{id}/run-now` planlı bir kontrolü `job_runs` tablosuna `queued` olarak yazar ve operatöre id döndürür. Worker tetik döngüsü yoksa run-now sonrasında `job_runs` görülebilir; gerçek dispatch worker tarafında handler çalıştığında olur.

## Job Catalog

`internal/scheduler/jobs.go::JobCatalog` aktif Faz 5'te şu durumdadır:
- Aktif (Enabled=true): `mikrotik_readonly_poll`, `mimosa_readonly_poll`, `tower_health_check`, `customer_signal_check`, `daily_network_check`, `weekly_network_report`, `ap_client_ping_latency`, `ap_client_packet_loss`, `ap_client_jitter`, `ap_client_traceroute`.
- Disabled: `frequency_recommendation_analysis` (Faz 8), `ap_client_limited_throughput` (Faz 5'te kapalı), `mikrotik_bandwidth_test` (Faz 9 manuel onaylı).

## Maintenance Window Etkileşimi

`scheduler.GuardWindow` çağrısı:
- Düşük risk: her zaman geçer.
- Orta risk: geçer ama dışarıda ise warning loglar.
- Yüksek risk: aktif bir bakım penceresi içinde değilse `ErrOutsideMaintenanceWindow` döner.

## Audit

Her CRUD ve run-now çağrısı `audit_logs.scheduled_check.ran` aksiyonu olarak yazılır. Metadata: `event` (create/update/delete/run-now), `job_type`, `risk_level`. Ham secret veya runtime payload yazılmaz.
