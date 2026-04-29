package devicectl

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeMikrotikCommand_SSHForm(t *testing.T) {
	path, filter := NormalizeMikrotikCommand("/system identity print")
	if path != "/system/identity/print" {
		t.Errorf("path = %q", path)
	}
	if filter != "" {
		t.Errorf("beklenmedik filter: %q", filter)
	}
}

func TestNormalizeMikrotikCommand_DetailFlag(t *testing.T) {
	path, _ := NormalizeMikrotikCommand("/interface wireless print detail")
	if path != "/interface/wireless/print/detail" {
		t.Errorf("path = %q", path)
	}
}

func TestNormalizeMikrotikCommand_LogPrintWithFilter(t *testing.T) {
	path, filter := NormalizeMikrotikCommand(`/log print where topics~"wireless|error|critical"`)
	if path != "/log/print" {
		t.Errorf("path = %q", path)
	}
	if !strings.Contains(filter, "wireless") {
		t.Errorf("filter = %q", filter)
	}
}

func TestIsMikrotikCommandAllowed_Whitelist(t *testing.T) {
	allowed := []string{
		"/system identity print",
		"/system resource print",
		"/system routerboard print",
		"/system package print",
		"/interface print detail",
		"/interface wireless print detail",
		"/interface wireless registration-table print detail",
		"/interface bridge print detail",
		"/interface bridge port print detail",
		"/ip address print detail",
		"/ip route print detail",
		"/ip neighbor print detail",
		`/log print where topics~"wireless|error|critical"`,
	}
	for _, cmd := range allowed {
		if !IsMikrotikCommandAllowed(cmd) {
			t.Errorf("allowlist olmalı: %q", cmd)
		}
	}
}

func TestIsMikrotikCommandAllowed_MutationVeto(t *testing.T) {
	denied := []string{
		"/interface wireless set frequency=5660",
		"/system reboot",
		"/system reset-configuration",
		"/system upgrade",
		"/ip address add address=10.0.0.1/24",
		"/ip address remove [find]",
		"/interface enable ether1",
		"/interface disable ether1",
		"/file remove startup.rsc",
		"/import config.rsc",
		"/export show-sensitive",
		"/user set password",
		"/certificate print decrypted-private-key",
	}
	for _, cmd := range denied {
		if IsMikrotikCommandAllowed(cmd) {
			t.Errorf("BLOCKED olmalı: %q", cmd)
		}
		if err := EnsureMikrotikCommand(cmd); err == nil {
			t.Errorf("EnsureMikrotikCommand %q hata vermeliydi", cmd)
		} else if !errors.Is(err, ErrMutationCommandBlocked) && !errors.Is(err, ErrCommandNotAllowed) {
			t.Errorf("yanlış hata türü: %v", err)
		}
	}
}

func TestIsMikrotikCommandAllowed_LogFilterEnforced(t *testing.T) {
	// İzinli filter ile geçer:
	if !IsMikrotikCommandAllowed(`/log print where topics~"wireless"`) {
		t.Error("topics~wireless izinli olmalıydı")
	}
	// Random filter reddedilir:
	if IsMikrotikCommandAllowed(`/log print where topics~"firewall"`) {
		t.Error("topics~firewall reddedilmeliydi")
	}
}

func TestIsMimosaEndpointAllowed(t *testing.T) {
	allowed := []string{
		"/api/v1/status",
		"/api/v1/status/wireless",
		"/cgi-bin/status",
	}
	denied := []string{
		"/api/v1/system/reboot",
		"/api/v1/wireless/set",
		"/cgi-bin/set.cgi",
		"/api/v1/firmware/upgrade",
		"/admin",
	}
	for _, p := range allowed {
		if !IsMimosaEndpointAllowed(p) {
			t.Errorf("Mimosa endpoint allowlist'te olmalıydı: %q", p)
		}
	}
	for _, p := range denied {
		if IsMimosaEndpointAllowed(p) {
			t.Errorf("Mimosa endpoint REDDEDİLMELİYDİ: %q", p)
		}
	}
}

func TestIsSNMPOIDAllowed(t *testing.T) {
	allowed := []string{
		"1.3.6.1.2.1.1.1.0",
		"1.3.6.1.2.1.2.2.1.10",
		"1.3.6.1.4.1.14988.1.1.1.4",
		"1.3.6.1.4.1.43356.1",
	}
	denied := []string{
		"1.3.6.1.4.1.9999",
		"",
		"some.random.oid",
	}
	for _, oid := range allowed {
		if !IsSNMPOIDAllowed(oid) {
			t.Errorf("OID izinli olmalıydı: %q", oid)
		}
	}
	for _, oid := range denied {
		if IsSNMPOIDAllowed(oid) {
			t.Errorf("OID reddedilmeliydi: %q", oid)
		}
	}
}

func TestEnsure_ErrorTypes(t *testing.T) {
	if err := EnsureMikrotikCommand("/random/path"); !errors.Is(err, ErrCommandNotAllowed) {
		t.Errorf("ErrCommandNotAllowed bekleniyordu, %v geldi", err)
	}
	if err := EnsureMikrotikCommand("/system reboot"); !errors.Is(err, ErrMutationCommandBlocked) {
		t.Errorf("ErrMutationCommandBlocked bekleniyordu, %v geldi", err)
	}
}

func TestNoMutationCommandLeaksThroughWhitelist(t *testing.T) {
	// Önemli invariant: whitelist'in HİÇBİR girişinin segment'i
	// MutationTokens listesinden olmamalı. Yoksa whitelist ile
	// mutation guard çelişir, savunma açık kalır.
	for _, cmd := range MikrotikReadOnlyCommands {
		for _, seg := range strings.Split(cmd, "/") {
			if seg == "" {
				continue
			}
			for _, mut := range MutationTokens {
				if seg == mut {
					t.Errorf("whitelist içinde mutation token: cmd=%q seg=%q", cmd, seg)
				}
			}
		}
	}
}
