package mimosa

import (
	"errors"
	"strings"
)

// Sentinel errors. Hata sınıfları Mikrotik adapter ile aynı
// kategorileri kullanır; üst katman (devicectl) tek bir
// classifyErrCode mantığı yazabilsin diye.
var (
	ErrNotImplemented       = errors.New("mimosa: feature not implemented")
	ErrCredentialMissing    = errors.New("mimosa: credential_profile_missing")
	ErrVaultNotConfigured   = errors.New("mimosa: vault_not_configured")
	ErrTransportUnsupported = errors.New("mimosa: transport_unsupported")
	ErrTimeout              = errors.New("mimosa: timeout")
	ErrAuth                 = errors.New("mimosa: authentication_failed")
	ErrUnreachable          = errors.New("mimosa: device_unreachable")
	ErrParse                = errors.New("mimosa: response_parse_failed")
	ErrSNMPv3Misconfigured  = errors.New("mimosa: snmpv3_misconfigured")
)

// SanitizeError parolaları/communityleri/secret'ları maskeler.
// Mikrotik adapter ile aynı kuralları uygular ki üst katmanın tek bir
// "vendor-bağımsız sanitizer" politikası yeterli olsun.
func SanitizeError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	low := strings.ToLower(msg)
	for _, k := range []string{"password", "secret", "passwd", "community", "token", "bearer", "authpassword", "privpassword"} {
		if i := strings.Index(low, k); i >= 0 {
			msg = msg[:i] + "[redacted]"
			break
		}
	}
	const maxLen = 240
	if len(msg) > maxLen {
		msg = msg[:maxLen] + "..."
	}
	return msg
}

// ClassifyError mirrors the MikroTik adapter mapping.
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
	case strings.Contains(low, "auth") || strings.Contains(low, "permission") || strings.Contains(low, "wrong digest") || strings.Contains(low, "unauthorized"):
		return ErrAuth
	case strings.Contains(low, "parse") || strings.Contains(low, "decode"):
		return ErrParse
	default:
		return ErrUnreachable
	}
}
