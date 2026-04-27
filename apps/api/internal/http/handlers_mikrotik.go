package http

import (
	"errors"
	"net/http"

	"github.com/wisp-ops-center/wisp-ops-center/internal/adapters/mikrotik"
	"github.com/wisp-ops-center/wisp-ops-center/internal/devicectl"
)

func (s *Server) devicectlOrError(w http.ResponseWriter) bool {
	if s.devCtl == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "devicectl_unavailable",
			"hint":  "set WISP_DATABASE_URL and WISP_VAULT_KEY",
		})
		return false
	}
	return true
}

func (s *Server) telemetryAvailable(w http.ResponseWriter) bool {
	if !s.requireDB(w) {
		return false
	}
	if s.tel == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "telemetry_unavailable"})
		return false
	}
	return true
}

// handleDeviceProbe runs the vendor-aware probe.
func (s *Server) handleDeviceProbe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	if !s.requireDB(w) || !s.devicectlOrError(w) {
		return
	}
	id := pathSegment(r.URL.Path, "/api/v1/devices/", "/probe")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	res, err := s.devCtl.Probe(r.Context(), id, actor(r))
	if errors.Is(err, devicectl.ErrDeviceNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	status := http.StatusOK
	if err != nil {
		status = http.StatusBadGateway
	}
	writeJSON(w, status, map[string]any{
		"data":  res,
		"error": mikrotik.SanitizeError(err),
	})
}

func (s *Server) handleDevicePoll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	if !s.requireDB(w) || !s.devicectlOrError(w) {
		return
	}
	id := pathSegment(r.URL.Path, "/api/v1/devices/", "/poll")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	snap, err := s.devCtl.Poll(r.Context(), id, actor(r))
	if errors.Is(err, devicectl.ErrDeviceNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	status := http.StatusOK
	if err != nil {
		status = http.StatusBadGateway
	}
	writeJSON(w, status, map[string]any{
		"data":  snap,
		"error": mikrotik.SanitizeError(err),
	})
}

func (s *Server) handleDeviceTelemetryLatest(w http.ResponseWriter, r *http.Request) {
	if !s.telemetryAvailable(w) {
		return
	}
	id := pathSegment(r.URL.Path, "/api/v1/devices/", "/telemetry/latest")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	res, err := s.tel.LatestSummary(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": nil})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": res})
}

func (s *Server) handleDeviceWirelessClientsLatest(w http.ResponseWriter, r *http.Request) {
	if !s.telemetryAvailable(w) {
		return
	}
	id := pathSegment(r.URL.Path, "/api/v1/devices/", "/wireless-clients/latest")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	rows, err := s.tel.LatestWirelessClients(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rows})
}

func (s *Server) handleDeviceInterfacesLatest(w http.ResponseWriter, r *http.Request) {
	if !s.telemetryAvailable(w) {
		return
	}
	id := pathSegment(r.URL.Path, "/api/v1/devices/", "/interfaces/latest")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	rows, err := s.tel.LatestInterfaces(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rows})
}

func (s *Server) handleMikrotikPollResults(w http.ResponseWriter, r *http.Request) {
	if !s.telemetryAvailable(w) {
		return
	}
	rows, err := s.tel.RecentResults(r.Context(), 100)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rows})
}

// handleDeviceMimosaLatest returns Mimosa-specific latest poll views
// (clients + links). Composite endpoint to keep the UI simple.
func (s *Server) handleDeviceMimosaLatest(w http.ResponseWriter, r *http.Request) {
	if !s.telemetryAvailable(w) {
		return
	}
	id := pathSegment(r.URL.Path, "/api/v1/devices/", "/mimosa/latest")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	clients, err := s.tel.LatestMimosaClients(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	links, err := s.tel.LatestMimosaLinks(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"clients": clients, "links": links},
	})
}

func (s *Server) handleMimosaPollResults(w http.ResponseWriter, r *http.Request) {
	// We reuse the unified device_poll_results table; the worker tag
	// stores vendor='mimosa'. Filter the recent list.
	if !s.telemetryAvailable(w) {
		return
	}
	rows, err := s.tel.RecentResults(r.Context(), 200)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	out := rows[:0]
	for _, r1 := range rows {
		if r1.Vendor == "mimosa" {
			out = append(out, r1)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}
