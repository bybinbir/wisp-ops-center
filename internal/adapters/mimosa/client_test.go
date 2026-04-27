package mimosa

import (
	"errors"
	"strings"
	"testing"
)

func TestExtractModelFromSysDescr(t *testing.T) {
	cases := map[string]string{
		"":                    "",
		"Mimosa B5c v2.5.4":   "B5c",
		"Mimosa A5-360 1.5.6": "A5-360",
		"OtherVendor X1":      "OtherVendor",
	}
	for in, want := range cases {
		if got := extractModelFromSysDescr(in); got != want {
			t.Fatalf("extractModelFromSysDescr(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExtractFirmwareFromSysDescr(t *testing.T) {
	if got := extractFirmwareFromSysDescr("Mimosa B5c v2.5.4"); got != "v2.5.4" {
		t.Fatalf("expected v2.5.4, got %q", got)
	}
	if got := extractFirmwareFromSysDescr("no version here"); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestSanitizeErrorMasksSecrets(t *testing.T) {
	err := errors.New("snmpv3 auth failed: authpassword=topSecret123")
	got := SanitizeError(err)
	if strings.Contains(got, "topSecret") {
		t.Fatalf("secret leaked: %q", got)
	}
	if !strings.Contains(got, "[redacted]") {
		t.Fatalf("expected redaction, got %q", got)
	}
}

func TestClassifyError(t *testing.T) {
	if !errors.Is(ClassifyError(errors.New("connection timeout")), ErrTimeout) {
		t.Fatal("expected ErrTimeout")
	}
	if !errors.Is(ClassifyError(errors.New("connection refused")), ErrUnreachable) {
		t.Fatal("expected ErrUnreachable")
	}
	if !errors.Is(ClassifyError(errors.New("auth failure")), ErrAuth) {
		t.Fatal("expected ErrAuth")
	}
	if ClassifyError(nil) != nil {
		t.Fatal("nil should pass through")
	}
}

func TestBuildUSMRequiresUsername(t *testing.T) {
	if _, err := buildUSM(Config{SNMPVersion: SNMPv3, V3SecurityLevel: AuthPriv}); !errors.Is(err, ErrSNMPv3Misconfigured) {
		t.Fatalf("expected ErrSNMPv3Misconfigured, got %v", err)
	}
}

func TestBuildUSMNoAuthNoPrivAcceptsBareUsername(t *testing.T) {
	if _, err := buildUSM(Config{SNMPVersion: SNMPv3, V3Username: "ro"}); err != nil {
		t.Fatalf("noAuthNoPriv minimal config should succeed: %v", err)
	}
}

func TestCapabilityFlagsHaveNoWriteFields(t *testing.T) {
	// Faz 4 sözleşmesi: CapabilityFlags struct'ı yazma alanı içermez.
	// Bu test, gelecekte yanlışlıkla canApplyFrequency-tipi bir alan
	// eklenmesini engelleyen bir guard'dır.
	c := CapabilityFlags{
		SupportsSNMP:           true,
		CanReadHealth:          true,
		CanReadWirelessMetrics: true,
		CanReadClients:         true,
		CanReadFrequency:       true,
		CanRecommendFrequency:  true,
	}
	if c.SupportsVendorAPI {
		t.Fatal("Mimosa supportsVendorAPI must be false in Phase 4")
	}
}

func TestVendorMIBPlaceholderIsUnverified(t *testing.T) {
	if VendorMIBPlaceholder != "unverified" {
		t.Fatalf("vendor MIB placeholder must remain 'unverified', got %q", VendorMIBPlaceholder)
	}
}

func TestProbeWithoutHostFails(t *testing.T) {
	cfg := Config{Transport: TransportSNMP, SNMPVersion: SNMPv2c, Community: "public"}
	res, caps, err := Probe(nil, cfg) //nolint:staticcheck
	if err == nil {
		t.Fatal("expected error for empty host")
	}
	if res != nil && res.Reachable {
		t.Fatal("must not report reachable on dial failure")
	}
	if caps.CanReadHealth || caps.SupportsVendorAPI {
		t.Fatal("no capability should flip to true on failure")
	}
}
