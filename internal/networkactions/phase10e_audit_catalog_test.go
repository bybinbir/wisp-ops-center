package networkactions

import "testing"

// TestDestructiveAuditCatalog_Phase10E pins the catalog size at 20
// (Phase 10A: 7 + Phase 10C: 3 + Phase 10D: 2 + Phase 10E: 8) and
// asserts every Phase 10E execute-path event is present. Together
// with the Phase 10C and 10D catalog tests (which now lower-bound
// the size), this is the regression guard for the destructive
// audit surface.
func TestDestructiveAuditCatalog_Phase10E(t *testing.T) {
	got := DestructiveAuditCatalog()
	if len(got) != 20 {
		t.Errorf("catalog size = %d, want 20 (10A: 7 + 10C: 3 + 10D: 2 + 10E: 8)", len(got))
	}
	must := map[DestructiveAuditAction]bool{
		AuditActionExecuteStarted:            false,
		AuditActionExecuteWriteSucceeded:     false,
		AuditActionExecuteWriteFailed:        false,
		AuditActionExecuteVerified:           false,
		AuditActionExecuteVerificationFailed: false,
		AuditActionExecuteRollbackStarted:    false,
		AuditActionExecuteRollbackSucceeded:  false,
		AuditActionExecuteRollbackFailed:     false,
	}
	for _, a := range got {
		if _, ok := must[a]; ok {
			must[a] = true
		}
	}
	for k, seen := range must {
		if !seen {
			t.Errorf("audit catalog missing Phase 10E event %q", k)
		}
	}
}

// TestPhase10EAuditNamesStable pins the literal strings of all 8
// Phase 10E events. Renaming any of them breaks downstream log
// alerts; the test exists to catch a refactor that quietly renames
// e.g. execute_verified to execute_verify_ok.
func TestPhase10EAuditNamesStable(t *testing.T) {
	want := map[DestructiveAuditAction]string{
		AuditActionExecuteStarted:            "network_action.execute_started",
		AuditActionExecuteWriteSucceeded:     "network_action.execute_write_succeeded",
		AuditActionExecuteWriteFailed:        "network_action.execute_write_failed",
		AuditActionExecuteVerified:           "network_action.execute_verified",
		AuditActionExecuteVerificationFailed: "network_action.execute_verification_failed",
		AuditActionExecuteRollbackStarted:    "network_action.execute_rollback_started",
		AuditActionExecuteRollbackSucceeded:  "network_action.execute_rollback_succeeded",
		AuditActionExecuteRollbackFailed:     "network_action.execute_rollback_failed",
	}
	for k, w := range want {
		if string(k) != w {
			t.Errorf("event %q drifted: got %q", w, string(k))
		}
	}
}

// TestPhase10ECatalogOrdering asserts the eight Phase 10E entries
// appear AFTER all Phase 10A/10C/10D entries. Append-only ordering
// is a downstream invariant for snapshot tests.
func TestPhase10ECatalogOrdering(t *testing.T) {
	got := DestructiveAuditCatalog()
	indexOf := func(target DestructiveAuditAction) int {
		for i, a := range got {
			if a == target {
				return i
			}
		}
		return -1
	}
	phase10dTail := indexOf(AuditActionExecuteNotImplemented)
	if phase10dTail < 0 {
		t.Fatalf("Phase 10D tail (execute_not_implemented) missing from catalog")
	}
	for _, ev := range []DestructiveAuditAction{
		AuditActionExecuteStarted,
		AuditActionExecuteWriteSucceeded,
		AuditActionExecuteWriteFailed,
		AuditActionExecuteVerified,
		AuditActionExecuteVerificationFailed,
		AuditActionExecuteRollbackStarted,
		AuditActionExecuteRollbackSucceeded,
		AuditActionExecuteRollbackFailed,
	} {
		i := indexOf(ev)
		if i < 0 {
			t.Errorf("%q missing from catalog", ev)
			continue
		}
		if i <= phase10dTail {
			t.Errorf("%q at index %d, want index > %d (after Phase 10D tail)", ev, i, phase10dTail)
		}
	}
}
