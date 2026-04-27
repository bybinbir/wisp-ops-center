package http

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wisp-ops-center/wisp-ops-center/internal/audit"
	"github.com/wisp-ops-center/wisp-ops-center/internal/workorders"
)

// workOrdersAvailable, dependency'lerin hazır olduğunu doğrular.
func (s *Server) workOrdersAvailable(w http.ResponseWriter) bool {
	if s.workOrders == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "database_not_configured",
			"hint":  "WISP_DATABASE_URL ayarlayıp migration uygulayın",
		})
		return false
	}
	return true
}

// =====================================================================
// /api/v1/work-orders : GET (list) / POST (-)
// /api/v1/work-orders/{id}                : GET / PATCH
// /api/v1/work-orders/{id}/events         : GET / POST
// /api/v1/work-orders/{id}/assign         : POST
// /api/v1/work-orders/{id}/resolve        : POST
// /api/v1/work-orders/{id}/cancel         : POST
// =====================================================================

func (s *Server) handleWorkOrdersCollection(w http.ResponseWriter, r *http.Request) {
	if !s.workOrdersAvailable(w) {
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	q := r.URL.Query()
	f := workorders.ListFilter{
		Status:     q.Get("status"),
		Priority:   q.Get("priority"),
		Severity:   q.Get("severity"),
		TowerID:    q.Get("tower_id"),
		APDeviceID: q.Get("ap_device_id"),
		CustomerID: q.Get("customer_id"),
		AssignedTo: q.Get("assigned_to"),
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			f.Limit = n
		}
	}
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			f.Offset = n
		}
	}
	if v := q.Get("date_from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.DateFrom = &t
		}
	}
	if v := q.Get("date_to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.DateTo = &t
		}
	}
	rows, total, err := s.workOrders.List(r.Context(), f)
	if err != nil {
		s.log.Warn("wo_list_failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data":   rows,
		"total":  total,
		"limit":  f.Limit,
		"offset": f.Offset,
	})
}

// handleWorkOrderItem, /api/v1/work-orders/{id}* dispatch.
func (s *Server) handleWorkOrderItem(w http.ResponseWriter, r *http.Request) {
	if !s.workOrdersAvailable(w) {
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/work-orders/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	id := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}
	switch action {
	case "":
		s.handleWorkOrderDetail(w, r, id)
	case "events":
		s.handleWorkOrderEvents(w, r, id)
	case "assign":
		s.handleWorkOrderAssign(w, r, id)
	case "resolve":
		s.handleWorkOrderResolve(w, r, id)
	case "cancel":
		s.handleWorkOrderCancel(w, r, id)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
	}
}

func (s *Server) handleWorkOrderDetail(w http.ResponseWriter, r *http.Request, id string) {
	switch r.Method {
	case http.MethodGet:
		wo, err := s.workOrders.Get(r.Context(), id)
		if err != nil {
			if errors.Is(err, workorders.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		events, _ := s.workOrders.ListEvents(r.Context(), id)
		writeJSON(w, http.StatusOK, map[string]any{
			"data":   wo,
			"events": events,
		})
	case http.MethodPatch:
		var body struct {
			Status      *string    `json:"status,omitempty"`
			Priority    *string    `json:"priority,omitempty"`
			AssignedTo  *string    `json:"assigned_to,omitempty"`
			Title       *string    `json:"title,omitempty"`
			Description *string    `json:"description,omitempty"`
			ETAAt       *time.Time `json:"eta_at,omitempty"`
			ClearETA    bool       `json:"clear_eta,omitempty"`
			Note        *string    `json:"note,omitempty"`
		}
		if err := readJSON(r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
			return
		}
		wo, err := s.workOrders.Patch(r.Context(), id, workorders.PatchInput{
			Status:      body.Status,
			Priority:    body.Priority,
			AssignedTo:  body.AssignedTo,
			Title:       body.Title,
			Description: body.Description,
			ETAAt:       body.ETAAt,
			ClearETA:    body.ClearETA,
			Note:        body.Note,
			Actor:       actor(r),
		})
		if err != nil {
			if errors.Is(err, workorders.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
				return
			}
			if errors.Is(err, workorders.ErrInvalidTransition) {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
					"error": "invalid_status_transition",
					"hint":  err.Error(),
				})
				return
			}
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.audit(r.Context(), audit.Entry{
			Actor:   actor(r),
			Action:  "work_order.updated",
			Subject: "work_order:" + id,
			Outcome: audit.OutcomeSuccess,
			Metadata: map[string]any{
				"status":   wo.Status,
				"priority": wo.Priority,
			},
		})
		writeJSON(w, http.StatusOK, map[string]any{"data": wo})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
	}
}

func (s *Server) handleWorkOrderEvents(w http.ResponseWriter, r *http.Request, id string) {
	switch r.Method {
	case http.MethodGet:
		events, err := s.workOrders.ListEvents(r.Context(), id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": events})
	case http.MethodPost:
		var body struct {
			EventType string `json:"event_type,omitempty"`
			Note      string `json:"note,omitempty"`
		}
		if err := readJSON(r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
			return
		}
		ev, err := s.workOrders.AppendEvent(r.Context(), id, body.EventType, body.Note, actor(r))
		if err != nil {
			s.log.Warn("wo_event_failed", "err", err)
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.audit(r.Context(), audit.Entry{
			Actor:   actor(r),
			Action:  "work_order.event_appended",
			Subject: "work_order:" + id,
			Outcome: audit.OutcomeSuccess,
			Metadata: map[string]any{
				"event_id":   ev.ID,
				"event_type": ev.EventType,
			},
		})
		writeJSON(w, http.StatusCreated, map[string]any{"data": ev})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
	}
}

func (s *Server) handleWorkOrderAssign(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	var body struct {
		AssignedTo string `json:"assigned_to"`
		Note       string `json:"note,omitempty"`
		AutoStart  bool   `json:"auto_start,omitempty"`
	}
	if err := readJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	wo, err := s.workOrders.Assign(r.Context(), id, body.AssignedTo, body.Note, actor(r))
	if err != nil {
		if errors.Is(err, workorders.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	// AutoStart: open → assigned otomatik geçişi (kolay UX). Sadece açık ise.
	if body.AutoStart && body.AssignedTo != "" && wo.Status == "open" {
		st := "assigned"
		if patched, err := s.workOrders.Patch(r.Context(), id, workorders.PatchInput{
			Status: &st,
			Actor:  actor(r),
		}); err == nil && patched != nil {
			wo = patched
		}
	}

	s.audit(r.Context(), audit.Entry{
		Actor:   actor(r),
		Action:  "work_order.assigned",
		Subject: "work_order:" + id,
		Outcome: audit.OutcomeSuccess,
		Metadata: map[string]any{
			"assigned_to": body.AssignedTo,
		},
	})
	writeJSON(w, http.StatusOK, map[string]any{"data": wo})
}

func (s *Server) handleWorkOrderResolve(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	var body struct {
		Note string `json:"note,omitempty"`
	}
	if err := readJSON(r, &body); err != nil && !errors.Is(err, io.EOF) {
		// Boş gövde kabul edilebilir; ama unknown_field dönmemeli.
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	wo, err := s.workOrders.Resolve(r.Context(), id, body.Note, actor(r))
	if err != nil {
		if errors.Is(err, workorders.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		if errors.Is(err, workorders.ErrInvalidTransition) {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
				"error": "invalid_status_transition",
				"hint":  err.Error(),
			})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.audit(r.Context(), audit.Entry{
		Actor:   actor(r),
		Action:  "work_order.resolved",
		Subject: "work_order:" + id,
		Outcome: audit.OutcomeSuccess,
		Reason:  body.Note,
	})
	writeJSON(w, http.StatusOK, map[string]any{"data": wo})
}

func (s *Server) handleWorkOrderCancel(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	var body struct {
		Note string `json:"note,omitempty"`
	}
	if err := readJSON(r, &body); err != nil && !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	wo, err := s.workOrders.Cancel(r.Context(), id, body.Note, actor(r))
	if err != nil {
		if errors.Is(err, workorders.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		if errors.Is(err, workorders.ErrInvalidTransition) {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
				"error": "invalid_status_transition",
				"hint":  err.Error(),
			})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.audit(r.Context(), audit.Entry{
		Actor:   actor(r),
		Action:  "work_order.cancelled",
		Subject: "work_order:" + id,
		Outcome: audit.OutcomeSuccess,
		Reason:  body.Note,
	})
	writeJSON(w, http.StatusOK, map[string]any{"data": wo})
}

// =====================================================================
// /api/v1/work-order-candidates/{id}/promote : POST
// =====================================================================

func (s *Server) handleWorkOrderCandidatePromote(w http.ResponseWriter, r *http.Request, id string) {
	if !s.workOrdersAvailable(w) {
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	var body struct {
		Title       string     `json:"title,omitempty"`
		Description string     `json:"description,omitempty"`
		Priority    string     `json:"priority,omitempty"`
		AssignedTo  *string    `json:"assigned_to,omitempty"`
		ETAAt       *time.Time `json:"eta_at,omitempty"`
	}
	if err := readJSON(r, &body); err != nil && !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	priority := workorders.Priority(strings.ToLower(body.Priority))
	if !workorders.IsValidPriority(string(priority)) {
		priority = workorders.PriorityMedium
	}
	out, err := s.workOrders.PromoteCandidate(r.Context(), workorders.PromoteInput{
		CandidateID: id,
		Title:       body.Title,
		Description: body.Description,
		Priority:    priority,
		AssignedTo:  body.AssignedTo,
		ETAAt:       body.ETAAt,
		Actor:       actor(r),
	})
	if err != nil {
		if errors.Is(err, workorders.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		if errors.Is(err, workorders.ErrCandidateNotPromotable) {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
				"error": "candidate_not_promotable",
				"hint":  err.Error(),
			})
			return
		}
		s.log.Warn("wo_promote_failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}

	// Audit — duplicate ise yine kaydet (operatöre net bildirim için).
	action := "work_order.created"
	if out.Duplicate {
		action = "work_order.promoted"
	}
	s.audit(r.Context(), audit.Entry{
		Actor:   actor(r),
		Action:  audit.Action(action),
		Subject: "work_order_candidate:" + id,
		Outcome: audit.OutcomeSuccess,
		Metadata: map[string]any{
			"work_order_id":    out.WorkOrder.ID,
			"duplicate":        out.Duplicate,
			"diagnosis":        out.WorkOrder.Diagnosis,
			"severity":         out.WorkOrder.Severity,
			"priority":         out.WorkOrder.Priority,
			"source_candidate": id,
		},
	})
	if !out.Duplicate {
		s.audit(r.Context(), audit.Entry{
			Actor:   actor(r),
			Action:  "work_order.promoted",
			Subject: "work_order:" + out.WorkOrder.ID,
			Outcome: audit.OutcomeSuccess,
			Metadata: map[string]any{
				"source_candidate": id,
			},
		})
	}

	status := http.StatusCreated
	if out.Duplicate {
		status = http.StatusOK
	}
	writeJSON(w, status, map[string]any{
		"data":      out.WorkOrder,
		"duplicate": out.Duplicate,
	})
}
