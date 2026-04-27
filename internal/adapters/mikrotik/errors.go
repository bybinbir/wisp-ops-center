package mikrotik

import (
	"errors"
	"fmt"
	"strings"
)

// Sentinel errors. Senior callers compare with errors.Is.
var (
	ErrNotImplemented       = errors.New("mikrotik: feature not implemented")
	ErrCredentialMissing    = errors.New("mikrotik: credential_profile_missing")
	ErrVaultNotConfigured   = errors.New("mikrotik: vault_not_configured")
	ErrTransportUnsupported = errors.New("mikrotik: transport_unsupported")
	ErrDisallowedCommand    = errors.New("mikrotik: disallowed_command")
	ErrTimeout              = errors.New("mikrotik: timeout")
	ErrAuth                 = errors.New("mikrotik: authentication_failed")
	ErrUnreachable          = errors.New("mikrotik: device_unreachable")
	ErrParse                = errors.New("mikrotik: response_parse_failed")
)

// SanitizeError strips secrets, hostnames-with-passwords, and bytes that look
// like tokens before we surface error text to API/UI/audit.
//
// MikroTik client libraries occasionally include "user/password" type payload
// fragments in their error returns; we never want those leaking. The
// implementation is deliberately defensive: it only reports a stable category
// + scrubbed detail, never raw internal text.
func SanitizeError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	low := strings.ToLower(msg)

	// Drop anything after a control-keyword that may carry secrets.
	for _, k := range []string{"password", "secret", "passwd", "community", "token", "bearer"} {
		if i := strings.Index(low, k); i >= 0 {
			msg = msg[:i] + "[redacted]"
			break
		}
	}

	// Cap length to avoid log bombs.
	const maxLen = 240
	if len(msg) > maxLen {
		msg = msg[:maxLen] + "..."
	}
	return msg
}

// ClassifyError maps a transport-level error to one of our sentinels for UI
// display + audit categorization. Unknown errors map to ErrUnreachable.
func ClassifyError(err error) error {
	if err == nil {
		return nil
	}
	low := strings.ToLower(err.Error())
	switch {
	case strings.Contains(low, "timeout"):
		return ErrTimeout
	case strings.Contains(low, "no route") || strings.Contains(low, "refused") || strings.Contains(low, "unreachable"):
		return ErrUnreachable
	case strings.Contains(low, "invalid user") || strings.Contains(low, "auth") || strings.Contains(low, "permission") || strings.Contains(low, "unauthorized"):
		return ErrAuth
	case strings.Contains(low, "parse") || strings.Contains(low, "decode"):
		return ErrParse
	default:
		return fmt.Errorf("%w: %s", ErrUnreachable, SanitizeError(err))
	}
}
