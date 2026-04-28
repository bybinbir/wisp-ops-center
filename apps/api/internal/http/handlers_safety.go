package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/wisp-ops-center/wisp-ops-center/internal/audit"
	"github.com/wisp-ops-center/wisp-ops-center/internal/networkactions"
)

// Phase 10B — Postgres-backed destructive-safety surface.
//
// Every endpoint in this file is READ-ONLY with respect to the
// network: nothing here ever runs an SSH command. The handlers
// only read/write the Phase 10A/B safety tables
// (network_action_toggle_flips, network_action_maintenance_windows)
// and emit audit events. The destructive master switch stays
// fail-closed; a flip via POST /toggle records the operator's
// intent but does NOT itself execute any network action.

// principalFromRequest extracts a Principal from request headers.
// Phase 10B is conservative: the API server does not yet have a
// session/auth backend, so we read the actor + roles from request
// headers (X-Actor, X-Roles). Production will replace this with a
// proper session lookup; the action layer keeps consuming a
// Principal so the swap is mechanical.
func principalFromRequest(r *http.Request) networkactions.Principal {
	actor := strings.TrimSpace(r.Header.Get("X-Actor"))
	if actor == "" {
		actor = "anonymous"
	}
	rolesHeader := r.Header.Get("X-Roles")
	var roles []string
	if rolesHeader != "" {
		for _, part := range strings.Split(rolesHeader, ",") {
			role := strings.TrimSpace(part)
			if role != "" {
				roles = append(roles, role)
			}
		}
	}
	return networkactions.Principal{Actor: actor, Roles: roles}
}

// requireCapability is the single authorization choke-point used by
// every Phase 10B safety endpoint. Returns true when the principal
// holds `want`. On deny it emits the rbac_denied audit event AND
// writes 403 to the response.
func (s *Server) requireCapability(
	w http.ResponseWriter,
	r *http.Request,
	p networkactions.Principal,
	want networkactions.Capability,
) bool {
	if networkactions.HasCapability(r.Context(), s.actionRBAC, p, want) {
		return true
	}
	s.audit(r.Context(), audit.Entry{
		Actor:   p.Actor,
		Action:  audit.Action(networkactions.AuditActionRBACDenied),
		Outcome: audit.OutcomeFailure,
		Subject: r.URL.Path,
		Metadata: map[string]any{
			"capability": string(want),
			"endpoint":   r.URL.Path,
			"roles":      p.Roles,
		},
	})
	writeJSON(w, http.StatusForbidden, map[string]string{
		"error": "rbac_denied",
		"hint":  "principal lacks required capability",
	})
	return false
}

// =============================================================================
// GET /api/v1/network/actions/preflight
// =============================================================================

// preflightResponse is the API shape returned by the preflight endpoint.
// Field naming is stable so future Phase 10C work can extend it
// without breaking consumers.
type preflightResponse struct {
	DestructiveEnabled bool                               `json:"destructive_enabled"`
	ToggleSource       string                             `json:"toggle_source"`
	LastFlip           *networkactions.FlipReceipt        `json:"last_flip,omitempty"`
	ActiveWindows      []networkactions.MaintenanceRecord `json:"active_maintenance_windows"`
	Checklist          []string                           `json:"pregate_checklist"`
	Capabilities       []networkactions.Capability        `json:"caller_capabilities"`
	BlockingReasons    []string                           `json:"blocking_reasons"`
	Now                time.Time                          `json:"now"`
}

func (s *Server) handleSafetyPreflight(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	p := principalFromRequest(r)
	if !s.requireCapability(w, r, p, networkactions.CapabilityPreflightRead) {
		return
	}
	caps, _ := s.actionRBAC.Capabilities(r.Context(), p)

	now := time.Now().UTC()
	enabled := false
	var toggleErr error
	if s.actionToggle != nil {
		enabled, toggleErr = s.actionToggle.Enabled(r.Context())
	}
	toggleSource := "none"
	if s.actionToggle != nil {
		toggleSource = "store"
	}

	var lastFlip *networkactions.FlipReceipt
	if pg, ok := s.actionToggle.(*networkactions.PgToggleStore); ok && pg != nil {
		lastFlip, _ = pg.LastFlip(r.Context())
	} else if mem, ok := s.actionToggle.(*networkactions.MemoryToggle); ok && mem != nil {
		lastFlip = mem.LastFlip()
	}

	var windows []networkactions.MaintenanceRecord
	var windowsErr error
	if s.actionWindowsProv != nil {
		windows, windowsErr = s.actionWindowsProv.ActiveAt(r.Context(), "", now)
	}

	resp := preflightResponse{
		DestructiveEnabled: enabled && toggleErr == nil,
		ToggleSource:       toggleSource,
		LastFlip:           lastFlip,
		ActiveWindows:      windows,
		Checklist:          networkactions.PreGateChecklist(),
		Capabilities:       caps,
		BlockingReasons:    blockingReasonsFromState(enabled, toggleErr, windows, windowsErr),
		Now:                now,
	}

	s.audit(r.Context(), audit.Entry{
		Actor:   p.Actor,
		Action:  audit.Action("network_action.preflight_checked"),
		Outcome: audit.OutcomeSuccess,
		Subject: r.URL.Path,
		Metadata: map[string]any{
			"destructive_enabled":     resp.DestructiveEnabled,
			"active_windows":          len(windows),
			"toggle_store_error":      toggleErr != nil,
			"window_store_error":      windowsErr != nil,
			"caller_capability_count": len(caps),
		},
	})

	writeJSON(w, http.StatusOK, resp)
}

func blockingReasonsFromState(enabled bool, toggleErr error, wins []networkactions.MaintenanceRecord, winsErr error) []string {
	out := []string{}
	if toggleErr != nil {
		out = append(out, "toggle_store_error")
	}
	if !enabled {
		out = append(out, "destructive_disabled")
	}
	if winsErr != nil {
		out = append(out, "window_store_error")
	}
	if len(wins) == 0 {
		out = append(out, "no_active_maintenance_window")
	}
	return out
}

// =============================================================================
// POST /api/v1/network/actions/toggle
// =============================================================================

type toggleFlipRequest struct {
	Enabled bool   `json:"enabled"`
	Reason  string `json:"reason"`
}

func (s *Server) handleSafetyToggle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	if !s.requireDB(w) {
		return
	}
	if s.actionToggle == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "toggle_store_unavailable"})
		return
	}
	p := principalFromRequest(r)
	if !s.requireCapability(w, r, p, networkactions.CapabilityToggleFlip) {
		return
	}
	var req toggleFlipRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body", "hint": err.Error()})
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "reason_required"})
		return
	}
	receipt, err := s.actionToggle.Flip(r.Context(), req.Enabled, p.Actor, reason)
	if err != nil {
		s.log.Warn("nwaction_toggle_flip_failed", "err", err, "actor", p.Actor)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "toggle_flip_failed"})
		return
	}
	s.audit(r.Context(), audit.Entry{
		Actor:   p.Actor,
		Action:  audit.Action(networkactions.AuditActionToggleFlipped),
		Outcome: audit.OutcomeSuccess,
		Subject: "destructive_master_switch",
		Metadata: map[string]any{
			"enabled":    receipt.Enabled,
			"flipped_at": receipt.FlippedAt,
			// reason is part of the audit record (operator-supplied,
			// safe to persist; SanitizeMessage runs to defend against
			// pasted credentials).
			"reason": networkactions.SanitizeMessage(receipt.Reason),
		},
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":    receipt.Enabled,
		"actor":      receipt.Actor,
		"flipped_at": receipt.FlippedAt,
	})
}

// =============================================================================
// GET / POST /api/v1/network/actions/maintenance-windows
// PATCH /api/v1/network/actions/maintenance-windows/{id}/disable
// =============================================================================

func (s *Server) handleSafetyMaintenanceWindowsDispatch(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/network/actions/maintenance-windows")
	switch {
	case rest == "" || rest == "/":
		s.handleMaintenanceWindowsRoot(w, r)
		return
	case strings.HasSuffix(rest, "/disable"):
		id := strings.TrimSuffix(strings.TrimPrefix(rest, "/"), "/disable")
		s.handleMaintenanceWindowDisable(w, r, id)
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
}

func (s *Server) handleMaintenanceWindowsRoot(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleMaintenanceWindowsList(w, r)
	case http.MethodPost:
		s.handleMaintenanceWindowsCreate(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
	}
}

func (s *Server) handleMaintenanceWindowsList(w http.ResponseWriter, r *http.Request) {
	if !s.requireDB(w) {
		return
	}
	if s.actionWindows == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "maintenance_store_unavailable"})
		return
	}
	p := principalFromRequest(r)
	if !s.requireCapability(w, r, p, networkactions.CapabilityPreflightRead) {
		return
	}
	rows, err := s.actionWindows.List(r.Context())
	if err != nil {
		s.log.Warn("nwaction_window_list_failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rows})
}

type maintenanceCreateRequest struct {
	Title    string    `json:"title"`
	Start    time.Time `json:"start"`
	End      time.Time `json:"end"`
	Scope    []string  `json:"scope"`
	Reason   string    `json:"reason"`
	Notes    string    `json:"notes"`
	Operator string    `json:"operator"`
}

func (s *Server) handleMaintenanceWindowsCreate(w http.ResponseWriter, r *http.Request) {
	if !s.requireDB(w) {
		return
	}
	if s.actionWindows == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "maintenance_store_unavailable"})
		return
	}
	p := principalFromRequest(r)
	if !s.requireCapability(w, r, p, networkactions.CapabilityMaintenanceManage) {
		return
	}
	var req maintenanceCreateRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body", "hint": err.Error()})
		return
	}
	reason := strings.TrimSpace(req.Reason)
	operator := strings.TrimSpace(req.Operator)
	if operator == "" {
		operator = p.Actor
	}
	if reason == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "reason_required"})
		return
	}
	if operator == "" || operator == "anonymous" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "operator_required"})
		return
	}
	notes := strings.TrimSpace(req.Notes)
	if notes == "" {
		// A reason is mandatory; merge it into notes so the audit
		// trail always carries the human-supplied context even if
		// the operator forgot the optional notes field.
		notes = reason
	}
	rec, err := s.actionWindows.Create(r.Context(), networkactions.CreateInput{
		Title:     req.Title,
		Start:     req.Start,
		End:       req.End,
		Scope:     req.Scope,
		CreatedBy: operator,
		Notes:     notes,
	})
	if err != nil {
		if errors.Is(err, networkactions.ErrMaintenanceWindowEmptyTitle) ||
			errors.Is(err, networkactions.ErrMaintenanceWindowInvertedRange) ||
			errors.Is(err, networkactions.ErrMaintenanceWindowDurationTooShort) ||
			errors.Is(err, networkactions.ErrMaintenanceWindowDurationTooLong) {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "invalid_window",
				"hint":  err.Error(),
			})
			s.audit(r.Context(), audit.Entry{
				Actor:   p.Actor,
				Action:  audit.Action(networkactions.AuditActionMaintenanceWindowDenied),
				Outcome: audit.OutcomeFailure,
				Subject: r.URL.Path,
				Metadata: map[string]any{
					"reason": networkactions.SanitizeMessage(err.Error()),
				},
			})
			return
		}
		s.log.Warn("nwaction_window_create_failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	s.audit(r.Context(), audit.Entry{
		Actor:   p.Actor,
		Action:  audit.Action("network_action.maintenance_window_created"),
		Outcome: audit.OutcomeSuccess,
		Subject: rec.ID,
		Metadata: map[string]any{
			"window_id":   rec.ID,
			"title":       rec.Title,
			"start":       rec.Start,
			"end":         rec.End,
			"scope_count": len(rec.Scope),
			"operator":    rec.CreatedBy,
			"reason":      networkactions.SanitizeMessage(reason),
		},
	})
	writeJSON(w, http.StatusCreated, map[string]any{"data": rec})
}

type maintenanceDisableRequest struct {
	Reason string `json:"reason"`
}

func (s *Server) handleMaintenanceWindowDisable(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPatch {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	if !s.requireDB(w) {
		return
	}
	if s.actionWindows == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "maintenance_store_unavailable"})
		return
	}
	if id == "" || !looksLikeUUID(id) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_id"})
		return
	}
	p := principalFromRequest(r)
	if !s.requireCapability(w, r, p, networkactions.CapabilityMaintenanceManage) {
		return
	}
	var req maintenanceDisableRequest
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body", "hint": err.Error()})
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "reason_required"})
		return
	}
	rec, err := s.actionWindows.Disable(r.Context(), networkactions.DisableInput{
		ID:            id,
		DisabledBy:    p.Actor,
		DisableReason: reason,
	})
	if err != nil {
		if errors.Is(err, networkactions.ErrMaintenanceWindowNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		s.log.Warn("nwaction_window_disable_failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	s.audit(r.Context(), audit.Entry{
		Actor:   p.Actor,
		Action:  audit.Action("network_action.maintenance_window_disabled"),
		Outcome: audit.OutcomeSuccess,
		Subject: rec.ID,
		Metadata: map[string]any{
			"window_id": rec.ID,
			"reason":    networkactions.SanitizeMessage(reason),
		},
	})
	writeJSON(w, http.StatusOK, map[string]any{"data": rec})
}

// _ silence unused imports in some build layouts.
var _ = json.Marshal
var _ context.Context
