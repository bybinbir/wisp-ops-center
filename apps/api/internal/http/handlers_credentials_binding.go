package http

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/wisp-ops-center/wisp-ops-center/internal/audit"
	"github.com/wisp-ops-center/wisp-ops-center/internal/telemetry"
)

// handleDeviceCredentials handles GET / PUT for
// /api/v1/devices/{id}/credentials.
func (s *Server) handleDeviceCredentials(w http.ResponseWriter, r *http.Request) {
	if !s.telemetryAvailable(w) {
		return
	}
	deviceID := pathSegment(r.URL.Path, "/api/v1/devices/", "/credentials")
	if deviceID == "" {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		list, err := s.tel.ListBindings(r.Context(), deviceID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": list})
	case http.MethodPut:
		var in struct {
			ProfileID string `json:"credential_profile_id"`
			Transport string `json:"transport"`
			Purpose   string `json:"purpose,omitempty"`
			Priority  int    `json:"priority,omitempty"`
			Enabled   *bool  `json:"enabled,omitempty"`
		}
		if err := readJSON(r, &in); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
			return
		}
		enabled := true
		if in.Enabled != nil {
			enabled = *in.Enabled
		}
		b := telemetry.DeviceCredentialBinding{
			DeviceID:  deviceID,
			ProfileID: in.ProfileID,
			Transport: in.Transport,
			Purpose:   in.Purpose,
			Priority:  in.Priority,
			Enabled:   enabled,
		}
		if err := s.tel.UpsertBinding(r.Context(), b); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.audit(r.Context(), audit.Entry{
			Actor: actor(r), Action: audit.ActionDeviceUpdated,
			Subject: "device:" + deviceID,
			Metadata: map[string]any{
				"binding": "upsert", "transport": in.Transport, "purpose": b.Purpose,
			},
		})
		writeJSON(w, http.StatusOK, map[string]any{"data": b})
	default:
		w.Header().Set("Allow", "GET, PUT")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
	}
}

// handleDeviceCredentialDelete handles DELETE
// /api/v1/devices/{deviceID}/credentials/{profileID}.
func (s *Server) handleDeviceCredentialDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		w.Header().Set("Allow", "DELETE")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	if !s.telemetryAvailable(w) {
		return
	}
	deviceID, profileID := splitDeviceCredsPath(r.URL.Path)
	if deviceID == "" || profileID == "" {
		http.NotFound(w, r)
		return
	}
	err := s.tel.DeleteBinding(r.Context(), deviceID, profileID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	s.audit(r.Context(), audit.Entry{
		Actor: actor(r), Action: audit.ActionDeviceUpdated,
		Subject: "device:" + deviceID,
		Metadata: map[string]any{
			"binding": "delete", "credential_profile_id": profileID,
		},
	})
	w.WriteHeader(http.StatusNoContent)
}

// splitDeviceCredsPath parses /api/v1/devices/{id}/credentials/{profileID}.
func splitDeviceCredsPath(p string) (string, string) {
	const prefix = "/api/v1/devices/"
	if len(p) <= len(prefix) {
		return "", ""
	}
	rest := p[len(prefix):]
	// rest = "{id}/credentials/{profileID}"
	idx := -1
	for i := 0; i < len(rest); i++ {
		if rest[i] == '/' {
			idx = i
			break
		}
	}
	if idx < 0 {
		return "", ""
	}
	deviceID := rest[:idx]
	rest = rest[idx+1:]
	const seg = "credentials/"
	if len(rest) <= len(seg) || rest[:len(seg)] != seg {
		return "", ""
	}
	profileID := rest[len(seg):]
	return deviceID, profileID
}
