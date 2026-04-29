package credentials

import (
	"strings"
	"testing"
)

// envSet ortamı izole test başına set/unset eder.
func envSet(t *testing.T, kv map[string]string) {
	t.Helper()
	for k, v := range kv {
		t.Setenv(k, v)
	}
}

func clearAll(t *testing.T) {
	t.Helper()
	keys := []string{
		EnvMikrotikRouterUsername, EnvMikrotikRouterPassword,
		EnvMikrotikAPUsername, EnvMikrotikAPPassword,
		EnvMimosaAUsername, EnvMimosaAPassword,
		EnvMimosaBUsername, EnvMimosaBPassword,
		EnvSNMPv2Community,
		EnvSNMPv3Username, EnvSNMPv3AuthPassword, EnvSNMPv3PrivPassword,
	}
	for _, k := range keys {
		t.Setenv(k, "")
	}
}

func TestLoadProfileSet_AllEmpty_NoError(t *testing.T) {
	clearAll(t)
	set, err := LoadProfileSetFromEnv()
	if err != nil {
		t.Fatalf("boş env hata vermemeli, ne aldık: %v", err)
	}
	if set.HasMikrotikRouter() || set.HasMikrotikAP() || set.HasMimosa() {
		t.Fatalf("hiçbir profil dolu olmamalı: %+v", set.SanitizedSet())
	}
}

func TestLoadProfileSet_MikrotikRouter_Pair(t *testing.T) {
	clearAll(t)
	envSet(t, map[string]string{
		EnvMikrotikRouterUsername: "ro_router",
		EnvMikrotikRouterPassword: "secret-router-1",
	})
	set, err := LoadProfileSetFromEnv()
	if err != nil {
		t.Fatalf("hata olmamalıydı: %v", err)
	}
	if !set.HasMikrotikRouter() {
		t.Fatal("router profili dolu olmalıydı")
	}
	if set.MikrotikRouter.Username != "ro_router" {
		t.Fatalf("user yanlış: %q", set.MikrotikRouter.Username)
	}
	if set.MikrotikRouter.AuthType != AuthRouterOSAPISSL {
		t.Fatalf("auth type RouterOSAPISSL olmalıydı, %q geldi", set.MikrotikRouter.AuthType)
	}
}

func TestLoadProfileSet_HalfConfigured_Errors(t *testing.T) {
	clearAll(t)
	// Username set, password yok.
	envSet(t, map[string]string{
		EnvMikrotikAPUsername: "ro_ap",
	})
	_, err := LoadProfileSetFromEnv()
	if err == nil {
		t.Fatal("yarım config hata üretmeliydi")
	}
	if !strings.Contains(err.Error(), EnvMikrotikAPUsername) {
		t.Fatalf("hata env adını anmalıydı, %q geldi", err.Error())
	}
}

func TestLoadProfileSet_MimosaBoth_FallbackOrder(t *testing.T) {
	clearAll(t)
	envSet(t, map[string]string{
		EnvMimosaAUsername: "admin",
		EnvMimosaAPassword: "passA",
		EnvMimosaBUsername: "admin",
		EnvMimosaBPassword: "passB",
	})
	set, err := LoadProfileSetFromEnv()
	if err != nil {
		t.Fatalf("hata olmamalıydı: %v", err)
	}
	profs := set.MimosaProfiles()
	if len(profs) != 2 {
		t.Fatalf("2 Mimosa profili bekleniyordu, %d geldi", len(profs))
	}
	if profs[0].ID != "mimosa_a" {
		t.Fatalf("ilk profil A olmalıydı, %q geldi", profs[0].ID)
	}
	if profs[1].ID != "mimosa_b" {
		t.Fatalf("ikinci profil B olmalıydı, %q geldi", profs[1].ID)
	}
}

func TestLoadProfileSet_SNMPv3_HalfConfigured_Errors(t *testing.T) {
	clearAll(t)
	envSet(t, map[string]string{
		EnvSNMPv3Username: "snmpuser",
	})
	_, err := LoadProfileSetFromEnv()
	if err == nil {
		t.Fatal("v3 username set ama auth yok hata vermeliydi")
	}
}

func TestSanitizedSet_MasksAllSecrets(t *testing.T) {
	set := ProfileSet{
		MikrotikAP: &Profile{
			ID: "ap", Username: "u", Secret: "do_not_leak",
			AuthType: AuthRouterOSAPISSL,
		},
		MimosaA: &Profile{
			ID: "ma", Username: "u", Secret: "another_secret",
			AuthType: AuthSSH,
		},
		SNMP: SNMPProfile{
			V2Community:  "public_secret",
			V3Username:   "snmpuser",
			V3AuthSecret: "auth_secret",
			V3PrivSecret: "priv_secret",
		},
	}
	san := set.SanitizedSet()
	if san.MikrotikAP.Secret != "***" {
		t.Errorf("AP secret maskelenmedi: %q", san.MikrotikAP.Secret)
	}
	if san.MimosaA.Secret != "***" {
		t.Errorf("Mimosa secret maskelenmedi: %q", san.MimosaA.Secret)
	}
	if san.SNMP.V2Community != "***" {
		t.Errorf("SNMP v2 community maskelenmedi: %q", san.SNMP.V2Community)
	}
	if san.SNMP.V3AuthSecret != "***" {
		t.Errorf("SNMP v3 auth maskelenmedi: %q", san.SNMP.V3AuthSecret)
	}
	if san.SNMP.V3PrivSecret != "***" {
		t.Errorf("SNMP v3 priv maskelenmedi: %q", san.SNMP.V3PrivSecret)
	}
	// Orijinal değişmedi mi?
	if set.MikrotikAP.Secret != "do_not_leak" {
		t.Error("Sanitize orijinal yapıyı kirletti")
	}
}

func TestLoadProfileSet_ErrorMessageDoesNotLeakSecret(t *testing.T) {
	clearAll(t)
	envSet(t, map[string]string{
		EnvMikrotikRouterPassword: "TOP_SECRET_VALUE_XYZ",
	})
	_, err := LoadProfileSetFromEnv()
	if err == nil {
		t.Fatal("yarım config hata vermeliydi")
	}
	if strings.Contains(err.Error(), "TOP_SECRET_VALUE_XYZ") {
		t.Fatalf("hata mesajı secret içerdi: %q", err.Error())
	}
}
