package workorders

import "testing"

func TestStatusTransitions(t *testing.T) {
	cases := []struct {
		name     string
		from     Status
		to       Status
		expected bool
	}{
		{"open→assigned", StatusOpen, StatusAssigned, true},
		{"open→in_progress", StatusOpen, StatusInProgress, true},
		{"open→cancelled", StatusOpen, StatusCancelled, true},
		{"open→resolved-skip-allowed?", StatusOpen, StatusResolved, false},
		{"assigned→in_progress", StatusAssigned, StatusInProgress, true},
		{"assigned→open(reopen)", StatusAssigned, StatusOpen, true},
		{"assigned→resolved-skip", StatusAssigned, StatusResolved, false},
		{"in_progress→resolved", StatusInProgress, StatusResolved, true},
		{"in_progress→open(reopen)", StatusInProgress, StatusOpen, true},
		{"in_progress→cancelled", StatusInProgress, StatusCancelled, true},
		{"resolved→anything", StatusResolved, StatusOpen, false},
		{"cancelled→anything", StatusCancelled, StatusOpen, false},
		{"same-state-noop", StatusOpen, StatusOpen, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CanTransition(tc.from, tc.to)
			if got != tc.expected {
				t.Errorf("CanTransition(%s,%s) = %v, want %v",
					tc.from, tc.to, got, tc.expected)
			}
		})
	}
}

func TestPriorityFromSeverity(t *testing.T) {
	cases := []struct {
		severity string
		expected Priority
	}{
		{"critical", PriorityHigh},
		{"warning", PriorityMedium},
		{"healthy", PriorityLow},
		{"unknown", PriorityLow},
		{"", PriorityLow},
		{"CRITICAL", PriorityHigh},
	}
	for _, tc := range cases {
		got := PriorityFromSeverity(tc.severity)
		if got != tc.expected {
			t.Errorf("PriorityFromSeverity(%q)=%s want %s", tc.severity, got, tc.expected)
		}
	}
}

func TestIsValidStatusAndPriority(t *testing.T) {
	if !IsValidStatus("open") {
		t.Error("open should be valid")
	}
	if IsValidStatus("done") {
		t.Error("done should be invalid")
	}
	if !IsValidPriority("urgent") {
		t.Error("urgent should be valid")
	}
	if IsValidPriority("super") {
		t.Error("super should be invalid")
	}
}

func TestIsTerminal(t *testing.T) {
	if !IsTerminal(StatusResolved) || !IsTerminal(StatusCancelled) {
		t.Error("resolved/cancelled must be terminal")
	}
	if IsTerminal(StatusOpen) || IsTerminal(StatusInProgress) {
		t.Error("open/in_progress must not be terminal")
	}
}
