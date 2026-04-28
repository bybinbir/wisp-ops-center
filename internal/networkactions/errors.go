package networkactions

import (
	"errors"
	"strings"
)

// Sentinels surfaced by the action runner. Shape lets the API layer
// translate these to stable error_code strings without exposing
// transport details.
var (
	ErrNotConfigured     = errors.New("networkactions: not_configured")
	ErrUnreachable       = errors.New("networkactions: device_unreachable")
	ErrTimeout           = errors.New("networkactions: timeout")
	ErrAuth              = errors.New("networkactions: authentication_failed")
	ErrHostKey           = errors.New("networkactions: host_key_policy_violation")
	ErrDisallowedCommand = errors.New("networkactions: disallowed_command")
	ErrParse             = errors.New("networkactions: response_parse_failed")
	ErrSkipped           = errors.New("networkactions: skipped")
)

// ClassifyError maps a transport error string into one of the
// sentinels. Conservative: anything we don't recognize collapses to
// ErrUnreachable so callers never get a free-form message in error_code.
func ClassifyError(err error) error {
	if err == nil {
		return nil
	}
	low := strings.ToLower(err.Error())
	switch {
	case strings.Contains(low, "timeout") || strings.Contains(low, "deadline exceeded"):
		return ErrTimeout
	case strings.Contains(low, "unable to authenticate") ||
		strings.Contains(low, "permission denied") ||
		strings.Contains(low, "authentication failed") ||
		strings.Contains(low, "auth fail"):
		return ErrAuth
	case strings.Contains(low, "host key") || strings.Contains(low, "fingerprint"):
		return ErrHostKey
	case strings.Contains(low, "no route") ||
		strings.Contains(low, "refused") ||
		strings.Contains(low, "unreachable") ||
		strings.Contains(low, "no such host"):
		return ErrUnreachable
	default:
		return ErrUnreachable
	}
}

// ErrorCode returns a short, stable label suitable for the API and
// audit log. NEVER returns the raw error string.
func ErrorCode(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrNotConfigured):
		return "not_configured"
	case errors.Is(err, ErrCredentialNotFound):
		return "credential_not_found"
	case errors.Is(err, ErrInvalidTargetHost):
		return "invalid_target_host"
	case errors.Is(err, ErrTimeout):
		return "timeout"
	case errors.Is(err, ErrAuth):
		return "auth_failed"
	case errors.Is(err, ErrHostKey):
		return "host_key_violation"
	case errors.Is(err, ErrUnreachable):
		return "unreachable"
	case errors.Is(err, ErrDisallowedCommand):
		return "disallowed_command"
	case errors.Is(err, ErrParse):
		return "parse_failed"
	case errors.Is(err, ErrSkipped):
		return "skipped"
	default:
		return "unknown"
	}
}
