package devicectl

import (
	"context"
	"errors"
	"testing"
)

func TestScanMode_IsValid(t *testing.T) {
	for _, m := range []ScanMode{ScanModeLight, ScanModeDeep, ScanModeOnDemand} {
		if !m.IsValid() {
			t.Errorf("%q geçerli olmalı", m)
		}
	}
	if ScanMode("garbage").IsValid() {
		t.Error("rastgele string geçerli olmamalı")
	}
}

func TestScanRequest_Validate(t *testing.T) {
	cases := []struct {
		name    string
		req     ScanRequest
		wantErr bool
	}{
		{"valid_light_no_target", ScanRequest{Mode: ScanModeLight}, false},
		{"valid_deep_no_target", ScanRequest{Mode: ScanModeDeep}, false},
		{"on_demand_needs_target", ScanRequest{Mode: ScanModeOnDemand}, true},
		{"on_demand_with_pop", ScanRequest{Mode: ScanModeOnDemand, PopCode: "ZIRVE"}, false},
		{"on_demand_with_devices", ScanRequest{Mode: ScanModeOnDemand, DeviceIDs: []string{"d1"}}, false},
		{"both_targets_conflict", ScanRequest{Mode: ScanModeLight, PopCode: "ZIRVE", DeviceIDs: []string{"d1"}}, true},
		{"invalid_mode", ScanRequest{Mode: "garbage"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.req.Validate()
			if tc.wantErr && err == nil {
				t.Error("hata bekleniyordu")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("beklenmedik hata: %v", err)
			}
		})
	}
}

func TestScheduler_Register_LookupCaseInsensitive(t *testing.T) {
	s := NewScheduler(nil)
	probe := stubProbe{}
	s.Register("Mikrotik_AP", probe)

	if got := s.LookupProbe("mikrotik_ap"); got == nil {
		t.Error("case-insensitive lookup başarısız")
	}
	if got := s.LookupProbe("MIKROTIK_AP"); got == nil {
		t.Error("upper-case lookup başarısız")
	}
	if got := s.LookupProbe("unknown_class"); got != nil {
		t.Error("kayıtsız class probe dönmemeli")
	}
}

func TestProbeOutcome_Sanitize_RedactsErrorMessage(t *testing.T) {
	o := ProbeOutcome{
		Status:       "credential_failed",
		ErrorMessage: "auth failed: password=do_not_leak",
	}
	san := o.Sanitize()
	if got := san.ErrorMessage; got == o.ErrorMessage {
		t.Error("error message redact edilmedi")
	}
	if got := san.ErrorMessage; got != "" && contains(got, "do_not_leak") {
		t.Errorf("secret hâlâ görünüyor: %q", got)
	}
}

func TestProfileSnapshot_Sanitize(t *testing.T) {
	p := ProfileSnapshot{ID: "ap", Username: "u", Secret: "s"}
	if san := p.Sanitize(); san.Secret != RedactionMask {
		t.Errorf("secret maskelenmedi: %q", san.Secret)
	}
}

func TestDefaultScanWindow(t *testing.T) {
	w := DefaultScanWindow()
	if w.LightInterval == 0 {
		t.Error("LightInterval set olmalı")
	}
	if w.DeepStart == "" || w.DeepEnd == "" {
		t.Error("DeepStart/DeepEnd boş olmamalı")
	}
}

func TestScanResult_HonestInvariant(t *testing.T) {
	ok := ScanResult{}
	if err := ok.HonestInvariant(); err != nil {
		t.Errorf("boş result invariant'tan geçmeliydi: %v", err)
	}
	bad := ScanResult{MutationBlocked: -1}
	if err := bad.HonestInvariant(); err == nil {
		t.Error("negatif MutationBlocked alarm vermeliydi")
	}
}

// stubProbe — testler için boş Probe implementation'ı.
type stubProbe struct{}

func (stubProbe) ProbeDevice(ctx context.Context, target ProbeTarget) (ProbeOutcome, error) {
	return ProbeOutcome{Status: "succeeded", Transport: "stub"}, errors.New("stub")
}

func contains(s, substr string) bool {
	if substr == "" {
		return true
	}
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
