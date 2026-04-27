# Bakım Pencereleri (Faz 5)

`maintenance_windows` tablosu Faz 5'te kuruldu. Yüksek riskli işler aktif pencere dışında engellenir; orta riskli işler uyarı üretir; düşük riskli işler hiç etkilenmez.

## Şema (`migrations/000005_*`)

```
id           UUID
name         TEXT
scope_type   TEXT  CHECK ('all_network','site','tower','device','customer_group','customer','link')
scope_id     TEXT  NULLABLE
starts_at    TIMESTAMPTZ
ends_at      TIMESTAMPTZ
timezone     TEXT  DEFAULT 'UTC'
recurrence   TEXT  CHECK ('','daily','weekly','monthly')
enabled      BOOL  DEFAULT TRUE
notes        TEXT
```

## Recurrence Yorumlaması

`scheduler.MaintenanceWindow.IsActive(at)`:
- `""` — tek seferlik [start, end).
- `daily` — her gün start/end saat aralığı.
- `weekly` — start/end haftanın aynı gününde.
- `monthly` — start/end ayın aynı gününde.

## API

| Method | Path | Açıklama |
|---|---|---|
| GET | `/api/v1/maintenance-windows` | Liste |
| POST | `/api/v1/maintenance-windows` | Yeni pencere |
| DELETE | `/api/v1/maintenance-windows/{id}` | Kaldır |

## Risk Enforcement

- Düşük: pencere kontrolü yok.
- Orta: pencere dışında uyarı (log + opsiyonel banner).
- Yüksek: aktif pencere içinde değilse `ErrOutsideMaintenanceWindow`. Faz 5'te kataloğdaki yüksek-riskli işler zaten disabled.
