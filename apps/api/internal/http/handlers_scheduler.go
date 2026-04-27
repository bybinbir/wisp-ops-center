package http

import (
	"errors"
	"net/http"

	"github.com/wisp-ops-center/wisp-ops-center/internal/audit"
	"github.com/wisp-ops-center/wisp-ops-center/internal/scheduler"
)

func (s *Server) schedulerOK(w http.ResponseWriter) bool {
	if !s.requireDB(w) {
		return false
	}
	if s.sched == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "scheduler_unavailable"})
		return false
	}
	return true
}

// /api/v1/scheduled-checks
func (s *Server) handleScheduledChecks(w http.ResponseWriter, r *http.Request) {
	if !s.schedulerOK(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		list, err := s.sched.ListChecks(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": list})
	case http.MethodPost:
		in, err := readScheduleInput(r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body", "detail": err.Error()})
			return
		}
		out, err := s.sched.CreateCheck(r.Context(), in)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.audit(r.Context(), audit.Entry{
			Actor: actor(r), Action: audit.ActionScheduledCheckRan,
			Subject:  "scheduled_check:" + out.ID,
			Metadata: map[string]any{"event": "create", "job_type": out.JobType, "risk_level": out.RiskLevel},
		})
		writeJSON(w, http.StatusCreated, map[string]any{"data": out})
	default:
		w.Header().Set("Allow", "GET, POST")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
	}
}

// /api/v1/scheduled-checks/{id} and /run-now
func (s *Server) handleScheduledCheckItem(w http.ResponseWriter, r *http.Request) {
	if !s.schedulerOK(w) {
		return
	}
	if pathSegment(r.URL.Path, "/api/v1/scheduled-checks/", "/run-now") != "" {
		s.handleScheduledCheckRunNow(w, r)
		return
	}
	id := pathID(r.URL.Path, "/api/v1/scheduled-checks/")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		out, err := s.sched.GetCheck(r.Context(), id)
		if errors.Is(err, scheduler.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": out})
	case http.MethodPatch:
		in, err := readScheduleInput(r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body", "detail": err.Error()})
			return
		}
		out, err := s.sched.UpdateCheck(r.Context(), id, in)
		if errors.Is(err, scheduler.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		s.audit(r.Context(), audit.Entry{
			Actor: actor(r), Action: audit.ActionScheduledCheckRan,
			Subject:  "scheduled_check:" + id,
			Metadata: map[string]any{"event": "update"},
		})
		writeJSON(w, http.StatusOK, map[string]any{"data": out})
	case http.MethodDelete:
		err := s.sched.DeleteCheck(r.Context(), id)
		if errors.Is(err, scheduler.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		s.audit(r.Context(), audit.Entry{
			Actor: actor(r), Action: audit.ActionScheduledCheckRan,
			Subject:  "scheduled_check:" + id,
			Metadata: map[string]any{"event": "delete"},
		})
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Allow", "GET, PATCH, DELETE")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
	}
}

func (s *Server) handleScheduledCheckRunNow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	id := pathSegment(r.URL.Path, "/api/v1/scheduled-checks/", "/run-now")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	check, err := s.sched.GetCheck(r.Context(), id)
	if errors.Is(err, scheduler.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	// Phase 5 manual run-now: record a synthetic queued job_runs row
	// and tell the operator that worker subprocess will pick it up.
	row := scheduler.JobRunRow{
		JobType:   check.JobType,
		ScopeType: check.ScopeType,
		ScopeID:   check.ScopeID,
		Status:    "queued",
		Summary:   map[string]any{"trigger": "run-now", "by": actor(r)},
	}
	row.CheckID = &check.ID
	jobID, err := s.sched.RecordJobRun(r.Context(), row)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	s.audit(r.Context(), audit.Entry{
		Actor: actor(r), Action: audit.ActionScheduledCheckRan,
		Subject:  "scheduled_check:" + id,
		Metadata: map[string]any{"event": "run-now", "job_run_id": jobID},
	})
	writeJSON(w, http.StatusAccepted, map[string]any{
		"data": map[string]string{"job_run_id": jobID, "status": "queued"},
	})
}

// /api/v1/job-runs
func (s *Server) handleJobRuns(w http.ResponseWriter, r *http.Request) {
	if !s.schedulerOK(w) {
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	rows, err := s.sched.ListJobRuns(r.Context(), 200)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rows})
}

// /api/v1/maintenance-windows
func (s *Server) handleMaintenanceWindows(w http.ResponseWriter, r *http.Request) {
	if !s.schedulerOK(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		list, err := s.sched.ListWindows(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": list})
	case http.MethodPost:
		var in scheduler.MaintenanceRow
		if err := readJSON(r, &in); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
			return
		}
		if in.ScopeType == "" {
			in.ScopeType = "all_network"
		}
		if !in.EndsAt.After(in.StartsAt) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ends_at must be after starts_at"})
			return
		}
		out, err := s.sched.CreateWindow(r.Context(), in)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"data": out})
	default:
		w.Header().Set("Allow", "GET, POST")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
	}
}

func (s *Server) handleMaintenanceWindowItem(w http.ResponseWriter, r *http.Request) {
	if !s.schedulerOK(w) {
		return
	}
	id := pathID(r.URL.Path, "/api/v1/maintenance-windows/")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodDelete {
		err := s.sched.DeleteWindow(r.Context(), id)
		if errors.Is(err, scheduler.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Allow", "DELETE")
	writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
}
