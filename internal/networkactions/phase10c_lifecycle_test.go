package networkactions

import (
	"testing"
)

// TestDestructiveAuditCatalog_Phase10C — Phase 10C added 3 events
// to the catalog. Pin the count + the new names so a future edit
// cannot quietly drop any of them.
func TestDestructiveAuditCatalog_Phase10C(t *testing.T) {
	got := DestructiveAuditCatalog()
	if len(got) != 10 {
		t.Errorf("catalog size = %d, want 10 (Phase 10A: 7 + Phase 10C: 3)", len(got))
	}
	must := map[DestructiveAuditAction]bool{
		AuditActionConfirmed:                false,
		AuditActionGateFail:                 false,
		AuditActionDryRunCompleted:          false,
		AuditActionLiveStartBlocked:         false,
		AuditActionToggleFlipped:            false,
		AuditActionRBACDenied:               false,
		AuditActionMaintenanceWindowDenied:  false,
		AuditActionIdempotencyReused:        false,
		AuditActionRollbackMetadataRecorded: false,
		AuditActionDestructiveDenied:        false,
	}
	for _, a := range got {
		if _, ok := must[a]; ok {
			must[a] = true
		}
	}
	for k, seen := range must {
		if !seen {
			t.Errorf("audit catalog missing %q", k)
		}
	}
}

// TestPhase10CAuditNamesStable — pin the literal strings of the
// three new lifecycle events. Renaming any of these is a breaking
// change for log consumers.
func TestPhase10CAuditNamesStable(t *testing.T) {
	want := map[DestructiveAuditAction]string{
		AuditActionIdempotencyReused:        "network_action.idempotency_reused",
		AuditActionRollbackMetadataRecorded: "network_action.rollback_metadata_recorded",
		AuditActionDestructiveDenied:        "network_action.destructive_denied",
	}
	for k, want := range want {
		if string(k) != want {
			t.Errorf("event %q drifted: got %q", want, string(k))
		}
	}
}

// TestDestructiveErrorCode_StableLabels_Phase10C — Phase 10A pinned
// the destructive error_codes; Phase 10C reuses them. Keep this
// guard so the API contract does not silently drift.
func TestDestructiveErrorCode_StableLabels_Phase10C(t *testing.T) {
	cases := map[error]string{
		ErrDestructiveDisabled:      "destructive_disabled",
		ErrIntentNotConfirmed:       "intent_not_confirmed",
		ErrMaintenanceWindowMissing: "maintenance_window_missing",
		ErrMaintenanceWindowClosed:  "maintenance_window_closed",
		ErrRollbackNoteMissing:      "rollback_note_missing",
		ErrRBACDenied:               "rbac_denied",
		ErrIdempotencyKeyMissing:    "idempotency_key_missing",
	}
	for err, want := range cases {
		if got := DestructiveErrorCode(err); got != want {
			t.Errorf("error %v code = %q want %q", err, got, want)
		}
	}
}

// TestDestructiveCreateRunInput_Validation — the repository helper
// rejects missing idempotency_key, intent or rollback_note BEFORE
// touching the DB. The test exercises the validation without a
// live pool by passing a nil receiver wrapped repository.
//
// Note: a nil pool would panic on QueryRow; we only assert the
// pre-DB validation order via direct error compare.
func TestDestructiveCreateRunInput_RejectsMissingFields(t *testing.T) {
	r := &Repository{P: nil}
	cases := []struct {
		name string
		in   DestructiveCreateRunInput
	}{
		{"missing idem", DestructiveCreateRunInput{
			ActionType: KindFrequencyCorrection, Intent: "x", RollbackNote: "y",
		}},
		{"missing intent", DestructiveCreateRunInput{
			ActionType: KindFrequencyCorrection, IdempotencyKey: "k", RollbackNote: "y",
		}},
		{"missing rollback", DestructiveCreateRunInput{
			ActionType: KindFrequencyCorrection, IdempotencyKey: "k", Intent: "x",
		}},
	}
	for _, c := range cases {
		_, err := r.CreateDestructiveRun(nil, c.in)
		if err == nil {
			t.Errorf("%s: expected validation error, got nil", c.name)
		}
	}
}
