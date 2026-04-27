package dude

import "strings"

// secretLikeKeys matches attribute keys that may carry secret
// material. Anything matched here is dropped from logs and from
// stored raw_metadata before persistence.
var secretLikeKeys = []string{
	"password", "passwd", "secret", "community",
	"key", "token", "bearer", "auth",
}

// SanitizeAttrs returns a copy of attrs with any secret-like values
// redacted. The original map is not mutated.
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

// SanitizeMessage strips obvious secret-bearing fragments from a
// human-readable string before it is logged or surfaced through
// the API.
func SanitizeMessage(msg string) string {
	low := strings.ToLower(msg)
	for _, k := range []string{"password=", "passwd=", "secret=", "token="} {
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
