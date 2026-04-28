package networkactions

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRegistry_StubReturnsNotImplemented(t *testing.T) {
	r := NewRegistry()
	a := r.Get(KindFrequencyCheck)
	if a == nil {
		t.Fatalf("expected stub registered for %s", KindFrequencyCheck)
	}
	res, err := a.Execute(context.Background(), Request{
		Kind:     KindFrequencyCheck,
		DeviceID: "dev-1",
	})
	if !errors.Is(err, ErrActionNotImplemented) {
		t.Errorf("want ErrActionNotImplemented, got %v", err)
	}
	if res.Success {
		t.Errorf("stub should not report success")
	}
	if res.ErrorCode != "not_implemented" {
		t.Errorf("got error code %q", res.ErrorCode)
	}
}

func TestRegistry_LocksPerDevice(t *testing.T) {
	r := NewRegistry()
	if err := r.AcquireLock("dev-A"); err != nil {
		t.Fatalf("first lock should succeed: %v", err)
	}
	if err := r.AcquireLock("dev-A"); !errors.Is(err, ErrActionLocked) {
		t.Errorf("expected ErrActionLocked, got %v", err)
	}
	r.ReleaseLock("dev-A")
	if err := r.AcquireLock("dev-A"); err != nil {
		t.Errorf("after release, lock should reacquire: %v", err)
	}
}

func TestKind_IsDestructive(t *testing.T) {
	if !KindFrequencyCorrection.IsDestructive() {
		t.Errorf("frequency_correction must be destructive")
	}
	for _, k := range []Kind{KindFrequencyCheck, KindAPClientTest,
		KindLinkSignalTest, KindBridgeHealthCheck, KindMaintenanceWindow} {
		if k.IsDestructive() {
			t.Errorf("%s should NOT be destructive", k)
		}
	}
}

func TestRateLimiter_Burst(t *testing.T) {
	rl := NewRateLimiter(3, time.Second)
	for i := 0; i < 3; i++ {
		if !rl.Allow() {
			t.Fatalf("burst[%d] denied", i)
		}
	}
	if rl.Allow() {
		t.Errorf("4th call should be denied")
	}
}
