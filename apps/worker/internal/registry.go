// Package workerinternal, worker prosesi için yerel destek tipleridir.
package workerinternal

import (
	"context"
	"sync"

	"github.com/wisp-ops-center/wisp-ops-center/internal/logger"
	"github.com/wisp-ops-center/wisp-ops-center/internal/scheduler"
)

// Handler, tek bir iş tipini yürüten fonksiyondur.
type Handler func(ctx context.Context, payload []byte) error

// Registry, iş tipi -> handler eşlemesini tutar.
type Registry struct {
	mu       sync.RWMutex
	handlers map[scheduler.JobType]Handler
}

// NewRegistry, boş bir registry üretir.
func NewRegistry() *Registry {
	return &Registry{handlers: map[scheduler.JobType]Handler{}}
}

// Register, bir iş tipi için handler atar.
func (r *Registry) Register(t scheduler.JobType, h Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[t] = h
}

// Lookup, kayıtlı handler'ı döner.
func (r *Registry) Lookup(t scheduler.JobType) (Handler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	h, ok := r.handlers[t]
	return h, ok
}

// Len, kayıtlı handler sayısını döner.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.handlers)
}

// SkeletonHandler, Faz 1 için varsayılan handler'dır.
func SkeletonHandler(t scheduler.JobType, log *logger.Logger) Handler {
	return func(ctx context.Context, payload []byte) error {
		log.Info("job_handle_skeleton",
			"job_type", string(t),
			"payload_bytes", len(payload),
			"note", "Faz 1 iskelet handler'i, gercek I/O yok",
		)
		return nil
	}
}
