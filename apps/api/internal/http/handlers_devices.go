package http

import (
	"errors"
	"net/http"

	"github.com/wisp-ops-center/wisp-ops-center/internal/audit"
	"github.com/wisp-ops-center/wisp-ops-center/internal/inventory"
)

func (s *Server) handleDevicesCollection(w http.ResponseWriter, r *http.Request) {
	if !s.requireDB(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		list, err := s.inv.ListDevices(r.Context())
		if err != nil {
			s.log.Error("devices_list_failed", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": list})
	case http.MethodPost:
		var in struct {
			Name            string   `json:"name"`
			Vendor          string   `json:"vendor"`
			Role            string   `json:"role"`
			IPAddress       string   `json:"ip_address"`
			SiteID          *string  `json:"site_id,omitempty"`
			TowerID         *string  `json:"tower_id,omitempty"`
			Model           string   `json:"model,omitempty"`
			OSVersion       string   `json:"os_version,omitempty"`
			FirmwareVersion string   `json:"firmware_version,omitempty"`
			Status          string   `json:"status,omitempty"`
			Tags            []string `json:"tags,omitempty"`
			Notes           string   `json:"notes,omitempty"`
		}
		if err := readJSON(r, &in); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body", "detail": err.Error()})
			return
		}
		d, err := s.inv.CreateDevice(r.Context(), inventory.CreateDeviceInput{
			Name: in.Name, Vendor: in.Vendor, Role: in.Role, IPAddress: in.IPAddress,
			SiteID: in.SiteID, TowerID: in.TowerID, Model: in.Model,
			OSVersion: in.OSVersion, FirmwareVersion: in.FirmwareVersion,
			Status: in.Status, Tags: in.Tags, Notes: in.Notes,
		})
		var ve *inventory.ErrValidation
		if errors.As(err, &ve) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "validation", "detail": ve.Msg})
			return
		}
		if err != nil {
			s.log.Error("devices_create_failed", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		s.audit(r.Context(), audit.Entry{
			Actor: actor(r), Action: audit.ActionDeviceCreated,
			Subject:  "device:" + d.ID,
			Metadata: map[string]any{"vendor": d.Vendor, "role": d.Role, "name": d.Name},
		})
		writeJSON(w, http.StatusCreated, map[string]any{"data": d})
	default:
		w.Header().Set("Allow", "GET, POST")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
	}
}

func (s *Server) handleDevicesItem(w http.ResponseWriter, r *http.Request) {
	if !s.requireDB(w) {
		return
	}
	id := pathID(r.URL.Path, "/api/v1/devices/")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		d, err := s.inv.GetDevice(r.Context(), id)
		if errors.Is(err, inventory.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		if err != nil {
			s.log.Error("device_get_failed", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": d})
	case http.MethodPatch:
		var in struct {
			Name            *string   `json:"name,omitempty"`
			Vendor          *string   `json:"vendor,omitempty"`
			Role            *string   `json:"role,omitempty"`
			IPAddress       *string   `json:"ip_address,omitempty"`
			SiteID          *string   `json:"site_id,omitempty"`
			TowerID         *string   `json:"tower_id,omitempty"`
			Model           *string   `json:"model,omitempty"`
			OSVersion       *string   `json:"os_version,omitempty"`
			FirmwareVersion *string   `json:"firmware_version,omitempty"`
			Status          *string   `json:"status,omitempty"`
			Tags            *[]string `json:"tags,omitempty"`
			Notes           *string   `json:"notes,omitempty"`
		}
		if err := readJSON(r, &in); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body", "detail": err.Error()})
			return
		}
		d, err := s.inv.UpdateDevice(r.Context(), id, inventory.UpdateDeviceInput{
			Name: in.Name, Vendor: in.Vendor, Role: in.Role, IPAddress: in.IPAddress,
			SiteID: in.SiteID, TowerID: in.TowerID, Model: in.Model,
			OSVersion: in.OSVersion, FirmwareVersion: in.FirmwareVersion,
			Status: in.Status, Tags: in.Tags, Notes: in.Notes,
		})
		var ve *inventory.ErrValidation
		if errors.As(err, &ve) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "validation", "detail": ve.Msg})
			return
		}
		if errors.Is(err, inventory.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		if err != nil {
			s.log.Error("device_update_failed", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		s.audit(r.Context(), audit.Entry{
			Actor: actor(r), Action: audit.ActionDeviceUpdated,
			Subject: "device:" + id,
		})
		writeJSON(w, http.StatusOK, map[string]any{"data": d})
	case http.MethodDelete:
		err := s.inv.DeleteDevice(r.Context(), id)
		if errors.Is(err, inventory.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		if err != nil {
			s.log.Error("device_delete_failed", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		s.audit(r.Context(), audit.Entry{
			Actor: actor(r), Action: audit.ActionDeviceDeleted,
			Subject:  "device:" + id,
			Metadata: map[string]any{"soft_delete": true},
		})
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Allow", "GET, PATCH, DELETE")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
	}
}
