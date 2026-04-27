package telemetry

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// DeviceCredentialBinding is a row of device_credentials enriched with
// the joined credential_profile name.
type DeviceCredentialBinding struct {
	DeviceID    string    `json:"device_id"`
	ProfileID   string    `json:"credential_profile_id"`
	ProfileName string    `json:"profile_name,omitempty"`
	AuthType    string    `json:"auth_type,omitempty"`
	Transport   string    `json:"transport"`
	Purpose     string    `json:"purpose"`
	Priority    int       `json:"priority"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
}

// ListBindings returns the credential bindings for one device.
func (r *Repository) ListBindings(ctx context.Context, deviceID string) ([]DeviceCredentialBinding, error) {
	rows, err := r.P.Query(ctx, `
SELECT dc.device_id, dc.profile_id, cp.name, cp.auth_type,
       dc.transport, dc.purpose, dc.priority, dc.enabled, dc.created_at
  FROM device_credentials dc
  JOIN credential_profiles cp ON cp.id = dc.profile_id
 WHERE dc.device_id = $1
 ORDER BY dc.priority, cp.name`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]DeviceCredentialBinding, 0)
	for rows.Next() {
		var b DeviceCredentialBinding
		if err := rows.Scan(&b.DeviceID, &b.ProfileID, &b.ProfileName, &b.AuthType,
			&b.Transport, &b.Purpose, &b.Priority, &b.Enabled, &b.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// UpsertBinding writes (device_id, profile_id, transport) tuple with
// a chosen purpose+priority. Existing row is updated.
func (r *Repository) UpsertBinding(ctx context.Context, b DeviceCredentialBinding) error {
	if b.DeviceID == "" || b.ProfileID == "" || b.Transport == "" {
		return errors.New("device_id, profile_id, transport required")
	}
	if b.Purpose == "" {
		b.Purpose = "primary"
	}
	if b.Priority == 0 {
		b.Priority = 100
	}
	_, err := r.P.Exec(ctx, `
INSERT INTO device_credentials (device_id, profile_id, transport, purpose, priority, enabled)
VALUES ($1,$2,$3,$4,$5,$6)
ON CONFLICT (device_id, transport) DO UPDATE SET
    profile_id = EXCLUDED.profile_id,
    purpose    = EXCLUDED.purpose,
    priority   = EXCLUDED.priority,
    enabled    = EXCLUDED.enabled`,
		b.DeviceID, b.ProfileID, b.Transport, b.Purpose, b.Priority, b.Enabled,
	)
	return err
}

// DeleteBinding removes a binding by (device_id, profile_id).
func (r *Repository) DeleteBinding(ctx context.Context, deviceID, profileID string) error {
	cmd, err := r.P.Exec(ctx,
		`DELETE FROM device_credentials WHERE device_id = $1 AND profile_id = $2`,
		deviceID, profileID,
	)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
