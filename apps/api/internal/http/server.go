// Package http hosts the wisp-ops-center API HTTP server, route groups
// and middleware.
package http

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/wisp-ops-center/wisp-ops-center/internal/adapters/mikrotik"
	"github.com/wisp-ops-center/wisp-ops-center/internal/audit"
	"github.com/wisp-ops-center/wisp-ops-center/internal/config"
	"github.com/wisp-ops-center/wisp-ops-center/internal/credentials"
	"github.com/wisp-ops-center/wisp-ops-center/internal/database"
	"github.com/wisp-ops-center/wisp-ops-center/internal/devicectl"
	"github.com/wisp-ops-center/wisp-ops-center/internal/inventory"
	"github.com/wisp-ops-center/wisp-ops-center/internal/networkactions"
	"github.com/wisp-ops-center/wisp-ops-center/internal/networkinv"
	"github.com/wisp-ops-center/wisp-ops-center/internal/reports"
	"github.com/wisp-ops-center/wisp-ops-center/internal/scheduler"
	"github.com/wisp-ops-center/wisp-ops-center/internal/scoring"
	"github.com/wisp-ops-center/wisp-ops-center/internal/telemetry"
	"github.com/wisp-ops-center/wisp-ops-center/internal/workorders"
	"sync"
)

type Server struct {
	*http.Server
	cfg        *config.Config
	db         *database.Pool
	log        *slog.Logger
	inv        *inventory.Repository
	creds      *credentials.Repository
	tel        *telemetry.Repository
	devCtl     *devicectl.Service
	sched      *scheduler.Repository
	scoring    *scoring.Repository
	hydrate    *scoring.Hydrator
	auditor    audit.Sink
	workOrders *workorders.Repository
	reports    *reports.Repository

	// Phase 8 — MikroTik Dude discovery + network inventory
	netInv         *networkinv.Repository
	netActions     *networkactions.Registry
	dudeRunMu      sync.Mutex // serialize discovery runs (one at a time)
	dudeRunActive  bool       // true while a run is in flight
}

// New returns a configured HTTP server.
func New(cfg *config.Config, db *database.Pool, log *slog.Logger) *Server {
	s := &Server{cfg: cfg, db: db, log: log}

	if db != nil && db.P != nil {
		s.inv = inventory.NewRepository(db.P)
		s.auditor = audit.NewPostgresSink(db.P)
		s.tel = telemetry.NewRepository(db.P)

		var vault credentials.Vault
		if cfg.Vault.Key != "" {
			v, err := credentials.NewAESGCMVault(cfg.Vault.Key)
			if err != nil {
				log.Warn("vault_key_invalid_using_noop", "err", err)
				vault = credentials.NoopVault{}
			} else {
				log.Info("vault_ready", "key_id", v.KeyID())
				vault = v
			}
		} else {
			log.Warn("vault_not_configured", "hint", "WISP_VAULT_KEY missing; credential profile API will refuse secrets")
			vault = credentials.NoopVault{}
		}
		s.creds = credentials.NewRepository(db.P, vault)
		s.devCtl = devicectl.NewService(db.P, vault, s.tel, s.auditor)
		s.sched = scheduler.NewRepository(db.P)
		s.scoring = scoring.NewRepository(db.P)
		s.hydrate = scoring.NewHydrator(db.P)
		// Faz 7: gerçek iş emri ve rapor repolari.
		s.workOrders = workorders.NewRepository(db.P)
		s.reports = reports.NewRepository(db.P)
		// Faz 6: SSH TOFU/Pinned politikası için Postgres-backed
		// known_hosts store global olarak set edilir.
		mikrotik.SetGlobalKnownHostsStore(&scheduler.SSHKnownHostsStore{P: db.P})
		// Faz 8: ağ envanteri repository + action framework iskeleti.
		s.netInv = networkinv.NewRepository(db.P)
	} else {
		s.auditor = audit.NewMemorySink()
	}
	// Action registry her durumda hazır (DB olmasa bile UI'a 503 döneriz).
	s.netActions = networkactions.NewRegistry()

	mux := http.NewServeMux()
	s.routes(mux)

	httpSrv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           s.middleware(mux),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	s.Server = httpSrv
	return s
}

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")

		if r.URL.Path != "/api/v1/health" && r.URL.Path != "/" && s.cfg.Auth.APIToken != "" {
			if r.Header.Get("Authorization") != "Bearer "+s.cfg.Auth.APIToken {
				writeJSON(w, http.StatusUnauthorized, map[string]string{
					"error": "unauthorized",
					"hint":  "Authorization: Bearer <token> bekleniyor",
				})
				return
			}
		}

		next.ServeHTTP(w, r)

		s.log.Info("http_request",
			"method", r.Method,
			"path", r.URL.Path,
			"remote", r.RemoteAddr,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func readJSON(r *http.Request, dst any) error {
	if r.Body == nil {
		return errors.New("empty body")
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func (s *Server) audit(ctx context.Context, e audit.Entry) {
	if s.auditor == nil {
		return
	}
	if err := s.auditor.Write(ctx, e); err != nil {
		s.log.Warn("audit_write_failed", "err", err, "action", e.Action)
	}
}

func (s *Server) requireDB(w http.ResponseWriter) bool {
	if s.db == nil || s.db.P == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "database_not_configured",
			"hint":  "set WISP_DATABASE_URL and run migrations",
		})
		return false
	}
	return true
}

func actor(r *http.Request) string {
	if r != nil && r.Header.Get("X-Actor") != "" {
		return r.Header.Get("X-Actor")
	}
	return "system"
}

func stub(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"endpoint":  name,
			"status":    "skeleton",
			"phase":     3,
			"data":      []any{},
			"message":   "Bu uç henüz iskelet; ileri fazlarda etkinleşecek.",
			"writeable": false,
		})
	}
}

// pathSegment returns the segment of url.Path between a known prefix and
// suffix. Returns "" if the path doesn't match.
func pathSegment(p, prefix, suffix string) string {
	if !strings.HasPrefix(p, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(p, prefix)
	if !strings.HasSuffix(rest, suffix) {
		return ""
	}
	return strings.TrimSuffix(rest, suffix)
}
