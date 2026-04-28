package networkactions

import (
	"context"
	"errors"
	"testing"
)

// TestPhase10D_RegistryDefault_HasFrequencyCorrectionStub asserts the
// pre-condition Phase 10D's runtime rewire depends on: NewRegistry()
// must populate a stub for KindFrequencyCorrection so the runner
// reaches Execute (and gets ErrActionNotImplemented) instead of
// falling into the registry-miss defensive branch. If a future
// refactor stops registering destructive Kinds by default, this test
// fires before the runtime rewire silently downgrades to registry_miss.
func TestPhase10D_RegistryDefault_HasFrequencyCorrectionStub(t *testing.T) {
	r := NewRegistry()
	got := r.Get(KindFrequencyCorrection)
	if got == nil {
		t.Fatalf("registry returned nil for KindFrequencyCorrection — Phase 10D runtime would fall into registry_miss")
	}
	if got.Kind() != KindFrequencyCorrection {
		t.Errorf("stub Kind = %q, want %q", got.Kind(), KindFrequencyCorrection)
	}
}

// TestPhase10D_StubExecute_ReturnsNotImplemented asserts the contract
// the Phase 10D runtime rewire relies on: calling Execute on a
// destructive Kind's registered stub MUST return
// ErrActionNotImplemented. The runner uses errors.Is to dispatch
// into the execute_not_implemented audit + finalize branch; if the
// stub ever stops returning that sentinel, Phase 10D falls into the
// invariant-violated branch and operators must review.
func TestPhase10D_StubExecute_ReturnsNotImplemented(t *testing.T) {
	r := NewRegistry()
	action := r.Get(KindFrequencyCorrection)
	if action == nil {
		t.Fatal("registry has no KindFrequencyCorrection — pre-condition for Phase 10D failed")
	}
	res, err := action.Execute(context.Background(), Request{
		Kind:          KindFrequencyCorrection,
		DeviceID:      "device-uuid-1",
		CorrelationID: "corr-1",
		DryRun:        false, // Phase 10D happy-path = live request
		Confirm:       true,
		Actor:         "alice",
		Reason:        "phase 10d: lifecycle smoke",
	})
	if !errors.Is(err, ErrActionNotImplemented) {
		t.Errorf("err = %v, want ErrActionNotImplemented", err)
	}
	if res.Success {
		t.Errorf("stub MUST report Success=false; got %+v", res)
	}
	if res.ErrorCode != "not_implemented" {
		t.Errorf("res.ErrorCode = %q, want %q", res.ErrorCode, "not_implemented")
	}
}

// TestPhase10D_StubExecute_PreservesRequestShape asserts the stub
// echoes Request fields the Phase 10D handler will surface in audit
// metadata. The runner doesn't rely on Result fields directly — it
// uses the error to branch — but downstream consumers reading the
// run row see Result.DryRun / Result.CorrelationID, so silently
// dropping them would be a silent data loss bug.
func TestPhase10D_StubExecute_PreservesRequestShape(t *testing.T) {
	r := NewRegistry()
	action := r.Get(KindFrequencyCorrection)
	if action == nil {
		t.Fatal("registry miss")
	}
	res, _ := action.Execute(context.Background(), Request{
		Kind:          KindFrequencyCorrection,
		DeviceID:      "device-2",
		CorrelationID: "corr-2",
		DryRun:        false,
		Confirm:       true,
	})
	if res.DryRun != false {
		t.Errorf("stub must echo DryRun=false; got %v", res.DryRun)
	}
	if res.CorrelationID != "corr-2" {
		t.Errorf("stub must echo CorrelationID; got %q", res.CorrelationID)
	}
	if res.DeviceID != "device-2" {
		t.Errorf("stub must echo DeviceID; got %q", res.DeviceID)
	}
}

// TestPhase10D_AllDestructiveKinds_StubReturnsNotImplemented walks
// every Kind that reports IsDestructive() == true and asserts the
// registry stub honors the same NotImplemented contract. If a future
// patch adds a destructive Kind without a stub, Phase 10D's runtime
// would silently fall into registry_miss for that Kind; this test
// catches the gap before the patch lands.
func TestPhase10D_AllDestructiveKinds_StubReturnsNotImplemented(t *testing.T) {
	allKinds := []Kind{
		KindFrequencyCheck, KindFrequencyCorrection,
		KindAPClientTest, KindLinkSignalTest,
		KindBridgeHealthCheck, KindMaintenanceWindow,
	}
	r := NewRegistry()
	destructiveSeen := 0
	for _, k := range allKinds {
		if !k.IsDestructive() {
			continue
		}
		destructiveSeen++
		action := r.Get(k)
		if action == nil {
			t.Errorf("destructive Kind %q has no registered action — Phase 10D runner falls into registry_miss", k)
			continue
		}
		_, err := action.Execute(context.Background(), Request{
			Kind:    k,
			Confirm: true,
		})
		if !errors.Is(err, ErrActionNotImplemented) {
			t.Errorf("destructive Kind %q: stub Execute returned %v, want ErrActionNotImplemented", k, err)
		}
	}
	if destructiveSeen == 0 {
		t.Fatalf("test self-check failed: no destructive Kinds in walked list — IsDestructive coverage drifted")
	}
}

// TestPhase10D_ErrorSentinelStable pins the literal text of
// ErrActionNotImplemented. The Phase 10D runtime uses errors.Is so
// rewording the message is technically safe; the test exists as a
// Tripwire — if someone ever flips the sentinel to a fresh error
// instance, errors.Is breaks and Phase 10D falls into the
// invariant-violated branch unexpectedly.
func TestPhase10D_ErrorSentinelStable(t *testing.T) {
	if ErrActionNotImplemented == nil {
		t.Fatal("ErrActionNotImplemented is nil")
	}
	want := "networkactions: action not implemented in phase 8"
	if ErrActionNotImplemented.Error() != want {
		t.Errorf("ErrActionNotImplemented.Error() = %q, want %q", ErrActionNotImplemented.Error(), want)
	}
	wrapped := errFunc()
	if !errors.Is(wrapped, ErrActionNotImplemented) {
		t.Errorf("errors.Is fails to recognize wrapped sentinel — Phase 10D branch dispatch breaks")
	}
}

func errFunc() error {
	return errors.Join(errors.New("wrapper context"), ErrActionNotImplemented)
}
