package dude

import (
	"errors"
	"strings"
)

var (
	ErrNotConfigured     = errors.New("dude: not_configured")
	ErrInvalidCredential = errors.New("dude: invalid_credential")
	ErrUnreachable       = errors.New("dude: device_unreachable")
	ErrTimeout           = errors.New("dude: timeout")
	ErrAuth              = errors.New("dude: authentication_failed")
	ErrHostKey           = errors.New("dude: host_key_policy_violation")
	ErrDisallowedCommand = errors.New("dude: disallowed_command")
	ErrParse             = errors.New("dude: response_parse_failed")
)

// ClassifyError maps a raw transport error into one of the sentinels.
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

// ErrorCode returns a stable short string suitable for the UI and
// for audit logs. NEVER returns the raw error message.
func ErrorCode(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrNotConfigured):
		return "not_configured"
	case errors.Is(err, ErrInvalidCredential):
		return "invalid_credential"
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
	default:
		return "unknown"
	}
}
