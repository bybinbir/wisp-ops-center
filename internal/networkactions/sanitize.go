package networkactions

import "strings"

// secretLikeKeys lists attribute keys whose values may carry secret
// material. SanitizeAttrs replaces matching values with "[redacted]"
// before they leave this package (audit metadata, API response, DB
// raw fields).
var secretLikeKeys = []string{
	"password", "passwd", "secret", "community",
	"key", "token", "bearer", "auth",
	"private", "fingerprint",
}

// SanitizeAttrs returns a copy of attrs with secret-like values
// redacted. Original input is not mutated.
func SanitizeAttrs(attrs map[string]string) map[string]string {
	if attrs == nil {
		return nil
	}
	out := make(map[string]string, len(attrs))
	for k, v := range attrs {
		lk := strings.ToLower(k)
		hit := false
		for _, s := range secretLikeKeys {
			if strings.Contains(lk, s) {
				hit = true
				break
			}
		}
		if hit {
			out[k] = "[redacted]"
		} else {
			out[k] = v
		}
	}
	return out
}

// SanitizeMessage strips obvious secret prefixes from a free-form
// string (e.g. an SSH error containing "password=hunter2 not valid")
// before it reaches the audit log or API response.
func SanitizeMessage(msg string) string {
	low := strings.ToLower(msg)
	for _, k := range []string{
		"password=", "passwd=", "secret=", "token=", "key=",
	} {
		if i := strings.Index(low, k); i >= 0 {
			return msg[:i] + "[redacted]"
		}
	}
	const maxLen = 320
	if len(msg) > maxLen {
		return msg[:maxLen] + "..."
	}
	return msg
}

// SanitizeResultMap returns a copy of the result map with any
// secret-like sub-keys redacted. Used by the action runner before
// writing into network_action_runs.result.
func SanitizeResultMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		lk := strings.ToLower(k)
		hit := false
		for _, s := range secretLikeKeys {
			if strings.Contains(lk, s) {
				hit = true
				break
			}
		}
		switch tv := v.(type) {
		case string:
			if hit {
				out[k] = "[redacted]"
			} else {
				out[k] = tv
			}
		case map[string]any:
			out[k] = SanitizeResultMap(tv)
		case []any:
			out[k] = sanitizeSlice(tv)
		default:
			out[k] = v
		}
	}
	return out
}

func sanitizeSlice(in []any) []any {
	out := make([]any, len(in))
	for i, v := range in {
		switch tv := v.(type) {
		case map[string]any:
			out[i] = SanitizeResultMap(tv)
		case []any:
			out[i] = sanitizeSlice(tv)
		default:
			out[i] = v
		}
	}
	return out
}
