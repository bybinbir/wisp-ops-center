package networkactions

import (
	"testing"
)

// TestDestructiveAuditCatalog_Phase10D pins the catalog size at 12
// after Phase 10D and asserts the two new execute-path events are
// present. Together with TestDestructiveAuditCatalog_Phase10C
// (lower-bound size check + Phase 10A/10C names), this gives us a
// double guard: Phase 10C still loads its 10 names, AND Phase 10D
// adds exactly two more.
func TestDestructiveAuditCatalog_Phase10D(t *testing.T) {
	got := DestructiveAuditCatalog()
	// Phase 10E added eight more events; the size assertion is a
	// lower bound so this stays a Phase 10D regression guard rather
	// than a moving target. Phase 10E's catalog test pins the exact
	// total going forward.
	if len(got) < 12 {
		t.Errorf("catalog size = %d, want >= 12 (Phase 10A: 7 + Phase 10C: 3 + Phase 10D: 2)", len(got))
	}
	must := map[DestructiveAuditAction]bool{
		AuditActionExecuteAttempted:      false,
		AuditActionExecuteNotImplemented: false,
	}
	for _, a := range got {
		if _, ok := must[a]; ok {
			must[a] = true
		}
	}
	for k, seen := range must {
		if !seen {
			t.Errorf("audit catalog missing Phase 10D event %q", k)
		}
	}
}

// TestPhase10DAuditNamesStable pins the literal strings of the two
// Phase 10D events. Renaming either is a breaking change for any
// downstream log consumer that grew an alert on these names.
func TestPhase10DAuditNamesStable(t *testing.T) {
	want := map[DestructiveAuditAction]string{
		AuditActionExecuteAttempted:      "network_action.execute_attempted",
		AuditActionExecuteNotImplemented: "network_action.execute_not_implemented",
	}
	for k, w := range want {
		if string(k) != w {
			t.Errorf("event %q drifted: got %q", w, string(k))
		}
	}
}

// TestPhase10DCatalogOrdering asserts the two Phase 10D entries
// appear AFTER all Phase 10A and Phase 10C entries. The catalog is
// append-only by convention; tests downstream rely on stable order
// when iterating for snapshot comparisons.
func TestPhase10DCatalogOrdering(t *testing.T) {
	got := DestructiveAuditCatalog()
	indexOf := func(target DestructiveAuditAction) int {
		for i, a := range got {
			if a == target {
				return i
			}
		}
		return -1
	}
	phase10cTail := indexOf(AuditActionDestructiveDenied)
	if phase10cTail < 0 {
		t.Fatalf("AuditActionDestructiveDenied not in catalog: %+v", got)
	}
	for _, ev := range []DestructiveAuditAction{
		AuditActionExecuteAttempted,
		AuditActionExecuteNotImplemented,
	} {
		i := indexOf(ev)
		if i < 0 {
			t.Errorf("%q missing from catalog", ev)
			continue
		}
		if i <= phase10cTail {
			t.Errorf("%q at index %d, want index > %d (after Phase 10C tail)", ev, i, phase10cTail)
		}
	}
}
