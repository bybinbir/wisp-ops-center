package workerinternal

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/wisp-ops-center/wisp-ops-center/internal/devicectl"
	"github.com/wisp-ops-center/wisp-ops-center/internal/logger"
)

// MikroTikPollPayload is the JSON shape accepted by the worker handler.
type MikroTikPollPayload struct {
	DeviceIDs   []string `json:"device_ids,omitempty"`
	SiteID      string   `json:"site_id,omitempty"`
	TowerID     string   `json:"tower_id,omitempty"`
	Vendor      string   `json:"vendor,omitempty"`
	MaxParallel int      `json:"max_parallel,omitempty"`
	TimeoutSec  int      `json:"timeout_sec,omitempty"`
}

// MikroTikReadOnlyPollHandler runs the read-only poll worker job. It
// honours per-device timeout and a small concurrency limit so that a
// large fleet does not stampede the API at the same time.
//
// Phase 3: this handler is invoked directly (no Asynq wired yet); the
// signature is compatible with the future kuyruk integration.
func MikroTikReadOnlyPollHandler(svc *devicectl.Service, log *logger.Logger) Handler {
	return func(ctx context.Context, payload []byte) error {
		if svc == nil {
			return errors.New("mikrotik_readonly_poll: devicectl unavailable")
		}
		var p MikroTikPollPayload
		if len(payload) > 0 {
			if err := json.Unmarshal(payload, &p); err != nil {
				return err
			}
		}
		if len(p.DeviceIDs) == 0 {
			return errors.New("mikrotik_readonly_poll: no device_ids in payload")
		}

		max := p.MaxParallel
		if max <= 0 || max > 16 {
			max = 4
		}
		timeout := time.Duration(p.TimeoutSec) * time.Second
		if timeout <= 0 || timeout > 30*time.Second {
			timeout = 12 * time.Second
		}

		sem := make(chan struct{}, max)
		var wg sync.WaitGroup
		for _, id := range p.DeviceIDs {
			id := id
			wg.Add(1)
			sem <- struct{}{}
			go func() {
				defer wg.Done()
				defer func() { <-sem }()
				ctx2, cancel := context.WithTimeout(ctx, timeout)
				defer cancel()
				if _, err := svc.Poll(ctx2, id, "worker:mikrotik_readonly_poll"); err != nil {
					log.Warn("mikrotik_poll_device_failed",
						"device_id", id,
						"err_class", err.Error(), // already sanitized by adapter
					)
				}
			}()
		}
		wg.Wait()
		return nil
	}
}
