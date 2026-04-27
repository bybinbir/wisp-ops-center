package http

import (
	"errors"
	"net/http"

	"github.com/wisp-ops-center/wisp-ops-center/internal/audit"
	"github.com/wisp-ops-center/wisp-ops-center/internal/inventory"
)

func (s *Server) handleSites(w http.ResponseWriter, r *http.Request) {
	if !s.requireDB(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		list, err := s.inv.ListSites(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": list})
	case http.MethodPost:
		var in struct {
			Name      string   `json:"name"`
			Code      string   `json:"code,omitempty"`
			Region    string   `json:"region,omitempty"`
			Address   string   `json:"address,omitempty"`
			Latitude  *float64 `json:"latitude,omitempty"`
			Longitude *float64 `json:"longitude,omitempty"`
			Notes     string   `json:"notes,omitempty"`
		}
		if err := readJSON(r, &in); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
			return
		}
		site, err := s.inv.CreateSite(r.Context(), inventory.CreateSiteInput{
			Name: in.Name, Code: in.Code, Region: in.Region, Address: in.Address,
			Latitude: in.Latitude, Longitude: in.Longitude, Notes: in.Notes,
		})
		var ve *inventory.ErrValidation
		if errors.As(err, &ve) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "validation", "detail": ve.Msg})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		s.audit(r.Context(), audit.Entry{
			Actor: actor(r), Action: audit.ActionSiteCreated, Subject: "site:" + site.ID,
		})
		writeJSON(w, http.StatusCreated, map[string]any{"data": site})
	default:
		w.Header().Set("Allow", "GET, POST")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
	}
}

func (s *Server) handleTowers(w http.ResponseWriter, r *http.Request) {
	if !s.requireDB(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		list, err := s.inv.ListTowers(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": list})
	case http.MethodPost:
		var in struct {
			SiteID    *string  `json:"site_id,omitempty"`
			Name      string   `json:"name"`
			Code      string   `json:"code,omitempty"`
			HeightM   *float64 `json:"height_m,omitempty"`
			Latitude  *float64 `json:"latitude,omitempty"`
			Longitude *float64 `json:"longitude,omitempty"`
			Notes     string   `json:"notes,omitempty"`
		}
		if err := readJSON(r, &in); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
			return
		}
		t, err := s.inv.CreateTower(r.Context(), inventory.CreateTowerInput{
			SiteID: in.SiteID, Name: in.Name, Code: in.Code, HeightM: in.HeightM,
			Latitude: in.Latitude, Longitude: in.Longitude, Notes: in.Notes,
		})
		var ve *inventory.ErrValidation
		if errors.As(err, &ve) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "validation", "detail": ve.Msg})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		s.audit(r.Context(), audit.Entry{
			Actor: actor(r), Action: audit.ActionTowerCreated, Subject: "tower:" + t.ID,
		})
		writeJSON(w, http.StatusCreated, map[string]any{"data": t})
	default:
		w.Header().Set("Allow", "GET, POST")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
	}
}

func (s *Server) handleLinks(w http.ResponseWriter, r *http.Request) {
	if !s.requireDB(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		list, err := s.inv.ListLinks(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": list})
	case http.MethodPost:
		var in struct {
			Name            string `json:"name"`
			Topology        string `json:"topology"`
			MasterDeviceID  string `json:"master_device_id"`
			FrequencyMHz    *int   `json:"frequency_mhz,omitempty"`
			ChannelWidthMHz *int   `json:"channel_width_mhz,omitempty"`
		}
		if err := readJSON(r, &in); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
			return
		}
		l, err := s.inv.CreateLink(r.Context(), inventory.CreateLinkInput{
			Name: in.Name, Topology: in.Topology, MasterDeviceID: in.MasterDeviceID,
			FrequencyMHz: in.FrequencyMHz, ChannelWidthMHz: in.ChannelWidthMHz,
		})
		var ve *inventory.ErrValidation
		if errors.As(err, &ve) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "validation", "detail": ve.Msg})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		s.audit(r.Context(), audit.Entry{
			Actor: actor(r), Action: audit.ActionLinkCreated, Subject: "link:" + l.ID,
		})
		writeJSON(w, http.StatusCreated, map[string]any{"data": l})
	default:
		w.Header().Set("Allow", "GET, POST")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
	}
}

func (s *Server) handleCustomers(w http.ResponseWriter, r *http.Request) {
	if !s.requireDB(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		list, err := s.inv.ListCustomers(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": list})
	case http.MethodPost:
		var in struct {
			ExternalCode   string  `json:"external_code,omitempty"`
			FullName       string  `json:"full_name"`
			Phone          string  `json:"phone,omitempty"`
			Address        string  `json:"address,omitempty"`
			SiteID         *string `json:"site_id,omitempty"`
			TowerID        *string `json:"tower_id,omitempty"`
			ContractedMbps *int    `json:"contracted_mbps,omitempty"`
		}
		if err := readJSON(r, &in); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
			return
		}
		c, err := s.inv.CreateCustomer(r.Context(), inventory.CreateCustomerInput{
			ExternalCode: in.ExternalCode, FullName: in.FullName, Phone: in.Phone, Address: in.Address,
			SiteID: in.SiteID, TowerID: in.TowerID, ContractedMbps: in.ContractedMbps,
		})
		var ve *inventory.ErrValidation
		if errors.As(err, &ve) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "validation", "detail": ve.Msg})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		s.audit(r.Context(), audit.Entry{
			Actor: actor(r), Action: audit.ActionCustomerCreated, Subject: "customer:" + c.ID,
		})
		writeJSON(w, http.StatusCreated, map[string]any{"data": c})
	default:
		w.Header().Set("Allow", "GET, POST")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
	}
}
