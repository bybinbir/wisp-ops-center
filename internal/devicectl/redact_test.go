package devicectl

import (
	"strings"
	"testing"
)

func TestIsSensitiveKey(t *testing.T) {
	cases := map[string]bool{
		"password":          true,
		"Password":          true,
		"PASSWORD":          true,
		"user-password":     true,
		"wireless-password": true,
		"wpa-psk":           true,
		"wpa_psk":           true,
		"pre-shared-key":    true,
		"ppp-secret":        true,
		"private-key":       true,
		"privkey":           true,
		"snmp-community":    true,
		"community-string":  true,
		"bearer-token":      true,
		"sessionId":         true, // session[-_]?(id|key|token)
		"name":              false,
		"interface":         false,
		"frequency":         false,
		"identity":          false,
		"signal":            false,
	}
	for k, want := range cases {
		got := IsSensitiveKey(k)
		if got != want {
			t.Errorf("IsSensitiveKey(%q) = %v, want %v", k, got, want)
		}
	}
}

func TestRedactText_KeyEqualsValue(t *testing.T) {
	in := `name=ZIRVE_AP password=mySuperSecret port=8728 user-password="another secret"`
	out := RedactText(in)
	if strings.Contains(out, "mySuperSecret") {
		t.Errorf("şifre sızdırıldı: %q", out)
	}
	if strings.Contains(out, "another secret") {
		t.Errorf("user-password sızdırıldı: %q", out)
	}
	if !strings.Contains(out, "name=ZIRVE_AP") {
		t.Errorf("zararsız alan kayboldu: %q", out)
	}
	if !strings.Contains(out, "port=8728") {
		t.Errorf("port maskelenmemeliydi: %q", out)
	}
}

func TestRedactText_RouterOSStyle(t *testing.T) {
	in := `flags: A
name: ether1
ppp-secret: leakybird
wpa-psk: another-leaky-key
`
	out := RedactText(in)
	if strings.Contains(out, "leakybird") {
		t.Errorf("ppp-secret sızdı: %q", out)
	}
	if strings.Contains(out, "another-leaky-key") {
		t.Errorf("wpa-psk sızdı: %q", out)
	}
	if !strings.Contains(out, "name: ether1") {
		t.Errorf("zararsız alan kayboldu: %q", out)
	}
}

func TestRedactStructured_MapStringAny(t *testing.T) {
	in := map[string]any{
		"name":     "ZIRVE_AP",
		"password": "do_not_leak",
		"nested": map[string]any{
			"ppp-secret": "another",
			"signal":     -65,
		},
		"list": []any{
			map[string]any{"token": "xyz", "ssid": "open"},
		},
	}
	out := RedactStructured(in).(map[string]any)
	if out["password"] != RedactionMask {
		t.Errorf("top-level password maskelenmedi: %v", out["password"])
	}
	nested := out["nested"].(map[string]any)
	if nested["ppp-secret"] != RedactionMask {
		t.Errorf("nested ppp-secret maskelenmedi: %v", nested["ppp-secret"])
	}
	if nested["signal"] != -65 {
		t.Errorf("zararsız alan değişti: %v", nested["signal"])
	}
	list := out["list"].([]any)
	first := list[0].(map[string]any)
	if first["token"] != RedactionMask {
		t.Errorf("list içindeki token maskelenmedi: %v", first["token"])
	}
	if first["ssid"] != "open" {
		t.Errorf("ssid değişti: %v", first["ssid"])
	}
}

func TestRedactJSONBytes_RoundTrip(t *testing.T) {
	in := []byte(`{"name":"ap1","password":"leak","clients":[{"mac":"aa","auth-key":"k"}]}`)
	out := RedactJSONBytes(in)
	s := string(out)
	if strings.Contains(s, "leak") {
		t.Errorf("password sızdırıldı: %s", s)
	}
	if strings.Contains(s, `"k"`) && !strings.Contains(s, `"***"`) {
		t.Errorf("auth-key maskelenmedi: %s", s)
	}
	if !strings.Contains(s, "ap1") {
		t.Errorf("zararsız alan kayboldu: %s", s)
	}
}

func TestRedactJSONBytes_BrokenJSONFallsbackToText(t *testing.T) {
	in := []byte(`name=ap1 password=leakily`)
	out := string(RedactJSONBytes(in))
	if strings.Contains(out, "leakily") {
		t.Errorf("broken JSON için text fallback de sızdırdı: %q", out)
	}
}

func TestRedactStructured_StructWithSensitiveField(t *testing.T) {
	type cred struct {
		Username string
		Password string
		Notes    string
	}
	in := cred{Username: "ro", Password: "do_not_leak", Notes: "harmless"}
	out := RedactStructured(in).(cred)
	if out.Password != RedactionMask {
		t.Errorf("Password alanı maskelenmedi: %q", out.Password)
	}
	if out.Username != "ro" || out.Notes != "harmless" {
		t.Errorf("zararsız alanlar bozuldu: %+v", out)
	}
}

func TestRedactStructured_NilSafe(t *testing.T) {
	if got := RedactStructured(nil); got != nil {
		t.Errorf("nil input nil dönmeli, %v geldi", got)
	}
}
