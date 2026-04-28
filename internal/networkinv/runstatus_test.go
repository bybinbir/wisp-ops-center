package networkinv

import "testing"

// Phase 8 hotfix v8.4.0 invariant: status and error_code must agree.
//
// These tests pin the ComputeRunStatus state machine. The
// runDudeDiscoveryAsync handler relies on this exact mapping:
// any non-empty error_code marks the run non-succeeded.

func TestComputeRunStatus_SucceededRequiresEmptyErrorCode(t *testing.T) {
	cases := []struct {
		name      string
		success   bool
		errorCode string
		devCount  int
		want      string
	}{
		{"success_clean", true, "", 5, "succeeded"},
		{"success_with_error_code_demoted", true, "persist_failed", 5, "partial"},
		{"failure_with_devices", false, "persist_failed", 5, "partial"},
		{"failure_no_devices", false, "unreachable", 0, "failed"},
		{"panic_recovered_never_partial", false, "panic_recovered", 12, "failed"},
		{"empty_run", true, "", 0, "succeeded"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ComputeRunStatus(tc.success, tc.errorCode, tc.devCount)
			if got != tc.want {
				t.Errorf("ComputeRunStatus(success=%v err=%q devs=%d) = %q, want %q",
					tc.success, tc.errorCode, tc.devCount, got, tc.want)
			}
		})
	}
}

func TestComputeRunStatus_RunCannotSucceedWithErrorCode(t *testing.T) {
	got := ComputeRunStatus(true, "persist_failed", 100)
	if got == "succeeded" {
		t.Fatalf("invariant violated: status=succeeded but error_code present")
	}
}

func TestComputeRunStatus_PersistFailedWithoutDevicesMarkedFailed(t *testing.T) {
	got := ComputeRunStatus(false, "persist_failed", 0)
	if got != "failed" {
		t.Fatalf("expected failed when persist failed and no devices, got %q", got)
	}
}

func TestComputeRunStatus_PersistFailedWithDevicesMarkedPartial(t *testing.T) {
	// If even one device made it through before transaction abort, we
	// keep the row as partial so operators see we got partway.
	got := ComputeRunStatus(false, "persist_failed", 12)
	if got != "partial" {
		t.Fatalf("expected partial when persist failed but devices observed, got %q", got)
	}
}
