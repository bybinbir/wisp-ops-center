package http

import (
	"errors"
	"net/http"

	"github.com/wisp-ops-center/wisp-ops-center/internal/audit"
	"github.com/wisp-ops-center/wisp-ops-center/internal/credentials"
)

func (s *Server) handleCredCollection(w http.ResponseWriter, r *http.Request) {
	if !s.requireDB(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		list, err := s.creds.List(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": list})
	case http.MethodPost:
		var in struct {
			Name     string `json:"name"`
			AuthType string `json:"auth_type"`
			Username string `json:"username,omitempty"`
			Secret   string `json:"secret,omitempty"`
			Port     *int   `json:"port,omitempty"`
			Notes    string `json:"notes,omitempty"`

			SNMPv3Username        string `json:"snmpv3_username,omitempty"`
			SNMPv3SecurityLevel   string `json:"snmpv3_security_level,omitempty"`
			SNMPv3AuthProtocol    string `json:"snmpv3_auth_protocol,omitempty"`
			SNMPv3AuthSecret      string `json:"snmpv3_auth_secret,omitempty"`
			SNMPv3PrivProtocol    string `json:"snmpv3_priv_protocol,omitempty"`
			SNMPv3PrivSecret      string `json:"snmpv3_priv_secret,omitempty"`
			VerifyTLS             bool   `json:"verify_tls,omitempty"`
			ServerNameOverride    string `json:"server_name_override,omitempty"`
			CACertificatePEM      string `json:"ca_certificate_pem,omitempty"`
			SSHHostKeyPolicy      string `json:"ssh_host_key_policy,omitempty"`
			SSHHostKeyFingerprint string `json:"ssh_host_key_fingerprint,omitempty"`
		}
		if err := readJSON(r, &in); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
			return
		}
		v, err := s.creds.Create(r.Context(), credentials.CreateInput{
			Name: in.Name, AuthType: credentials.AuthType(in.AuthType),
			Username: in.Username, Secret: in.Secret, Port: in.Port, Notes: in.Notes,
			SNMPv3Username: in.SNMPv3Username, SNMPv3SecurityLevel: in.SNMPv3SecurityLevel,
			SNMPv3AuthProtocol: in.SNMPv3AuthProtocol, SNMPv3AuthSecret: in.SNMPv3AuthSecret,
			SNMPv3PrivProtocol: in.SNMPv3PrivProtocol, SNMPv3PrivSecret: in.SNMPv3PrivSecret,
			VerifyTLS: in.VerifyTLS, ServerNameOverride: in.ServerNameOverride,
			CACertificatePEM:      in.CACertificatePEM,
			SSHHostKeyPolicy:      in.SSHHostKeyPolicy,
			SSHHostKeyFingerprint: in.SSHHostKeyFingerprint,
		})
		var ve *credentials.ErrValidation
		if errors.As(err, &ve) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "validation", "detail": ve.Msg})
			return
		}
		if err != nil {
			s.log.Error("cred_create_failed", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		s.audit(r.Context(), audit.Entry{
			Actor: actor(r), Action: audit.ActionCredProfileCreated,
			Subject: "credential_profile:" + v.ID,
			Metadata: map[string]any{
				"name": v.Name, "auth_type": v.AuthType,
				"secret_set":      v.SecretSet,
				"snmpv3_auth_set": v.SNMPv3AuthSet,
				"snmpv3_priv_set": v.SNMPv3PrivSet,
				"verify_tls":      v.VerifyTLS,
			},
		})
		writeJSON(w, http.StatusCreated, map[string]any{"data": v})
	default:
		w.Header().Set("Allow", "GET, POST")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
	}
}

func (s *Server) handleCredItem(w http.ResponseWriter, r *http.Request) {
	if !s.requireDB(w) {
		return
	}
	id := pathID(r.URL.Path, "/api/v1/credential-profiles/")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		v, err := s.creds.Get(r.Context(), id)
		if errors.Is(err, credentials.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": v})
	case http.MethodPatch:
		var in struct {
			Name     *string `json:"name,omitempty"`
			AuthType *string `json:"auth_type,omitempty"`
			Username *string `json:"username,omitempty"`
			Secret   *string `json:"secret,omitempty"`
			Port     *int    `json:"port,omitempty"`
			Notes    *string `json:"notes,omitempty"`

			SNMPv3Username        *string `json:"snmpv3_username,omitempty"`
			SNMPv3SecurityLevel   *string `json:"snmpv3_security_level,omitempty"`
			SNMPv3AuthProtocol    *string `json:"snmpv3_auth_protocol,omitempty"`
			SNMPv3AuthSecret      *string `json:"snmpv3_auth_secret,omitempty"`
			SNMPv3PrivProtocol    *string `json:"snmpv3_priv_protocol,omitempty"`
			SNMPv3PrivSecret      *string `json:"snmpv3_priv_secret,omitempty"`
			VerifyTLS             *bool   `json:"verify_tls,omitempty"`
			ServerNameOverride    *string `json:"server_name_override,omitempty"`
			CACertificatePEM      *string `json:"ca_certificate_pem,omitempty"`
			SSHHostKeyPolicy      *string `json:"ssh_host_key_policy,omitempty"`
			SSHHostKeyFingerprint *string `json:"ssh_host_key_fingerprint,omitempty"`
		}
		if err := readJSON(r, &in); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
			return
		}
		var atPtr *credentials.AuthType
		if in.AuthType != nil {
			a := credentials.AuthType(*in.AuthType)
			atPtr = &a
		}
		v, err := s.creds.Update(r.Context(), id, credentials.UpdateInput{
			Name: in.Name, AuthType: atPtr, Username: in.Username,
			Secret: in.Secret, Port: in.Port, Notes: in.Notes,
			SNMPv3Username: in.SNMPv3Username, SNMPv3SecurityLevel: in.SNMPv3SecurityLevel,
			SNMPv3AuthProtocol: in.SNMPv3AuthProtocol, SNMPv3AuthSecret: in.SNMPv3AuthSecret,
			SNMPv3PrivProtocol: in.SNMPv3PrivProtocol, SNMPv3PrivSecret: in.SNMPv3PrivSecret,
			VerifyTLS: in.VerifyTLS, ServerNameOverride: in.ServerNameOverride,
			CACertificatePEM:      in.CACertificatePEM,
			SSHHostKeyPolicy:      in.SSHHostKeyPolicy,
			SSHHostKeyFingerprint: in.SSHHostKeyFingerprint,
		})
		var ve *credentials.ErrValidation
		if errors.As(err, &ve) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "validation", "detail": ve.Msg})
			return
		}
		if errors.Is(err, credentials.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		if err != nil {
			s.log.Error("cred_update_failed", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		s.audit(r.Context(), audit.Entry{
			Actor: actor(r), Action: audit.ActionCredProfileUpdated,
			Subject: "credential_profile:" + id,
			Metadata: map[string]any{
				"secret_rotated":     in.Secret != nil,
				"snmpv3_auth_rotate": in.SNMPv3AuthSecret != nil,
				"snmpv3_priv_rotate": in.SNMPv3PrivSecret != nil,
			},
		})
		writeJSON(w, http.StatusOK, map[string]any{"data": v})
	case http.MethodDelete:
		err := s.creds.Delete(r.Context(), id)
		if errors.Is(err, credentials.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		s.audit(r.Context(), audit.Entry{
			Actor: actor(r), Action: audit.ActionCredProfileDeleted,
			Subject: "credential_profile:" + id,
		})
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Allow", "GET, PATCH, DELETE")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
	}
}

func (s *Server) handleAuditLogs(w http.ResponseWriter, r *http.Request) {
	if !s.requireDB(w) {
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	rows, err := s.db.P.Query(r.Context(),
		`SELECT id, at, actor, action, COALESCE(subject,''), outcome, COALESCE(reason,''), metadata
FROM audit_logs ORDER BY at DESC LIMIT 200`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	defer rows.Close()

	type row struct {
		ID       int64                  `json:"id"`
		At       string                 `json:"at"`
		Actor    string                 `json:"actor"`
		Action   string                 `json:"action"`
		Subject  string                 `json:"subject"`
		Outcome  string                 `json:"outcome"`
		Reason   string                 `json:"reason,omitempty"`
		Metadata map[string]interface{} `json:"metadata"`
	}
	out := make([]row, 0)
	for rows.Next() {
		var r1 row
		var atTS interface{}
		if err := rows.Scan(&r1.ID, &atTS, &r1.Actor, &r1.Action, &r1.Subject, &r1.Outcome, &r1.Reason, &r1.Metadata); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
			return
		}
		if s, ok := atTS.(interface{ String() string }); ok {
			r1.At = s.String()
		}
		out = append(out, r1)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}
