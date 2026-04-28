package http

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wisp-ops-center/wisp-ops-center/internal/networkactions"
)

// TestPrincipalFromRequest_Defaults — empty headers yield the
// "anonymous" actor with no roles; this is the baseline a fresh
// Phase 10B endpoint sees, which the RBAC layer MUST refuse.
func TestPrincipalFromRequest_Defaults(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/v1/network/actions/preflight", nil)
	p := principalFromRequest(r)
	if p.Actor != "anonymous" {
		t.Errorf("default actor=%q want anonymous", p.Actor)
	}
	if len(p.Roles) != 0 {
		t.Errorf("default roles should be empty, got %v", p.Roles)
	}
}

// TestPrincipalFromRequest_HeadersParsed — operator session is
// modeled with X-Actor + X-Roles. Multiple roles are comma-separated
// and trimmed.
func TestPrincipalFromRequest_HeadersParsed(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Actor", "alice")
	r.Header.Set("X-Roles", "net_admin , net_ops")
	p := principalFromRequest(r)
	if p.Actor != "alice" {
		t.Errorf("actor=%q want alice", p.Actor)
	}
	if len(p.Roles) != 2 || p.Roles[0] != "net_admin" || p.Roles[1] != "net_ops" {
		t.Errorf("roles=%v want [net_admin net_ops]", p.Roles)
	}
}

// TestBlockingReasonsFromState — the preflight blocking_reasons
// list reflects every failure path so an operator can see why
// destructive execution would be denied. Exact strings are part of
// the API contract.
func TestBlockingReasonsFromState(t *testing.T) {
	cases := []struct {
		name        string
		enabled     bool
		toggleErr   error
		windows     []networkactions.MaintenanceRecord
		windowsErr  error
		mustContain []string
	}{
		{"all-bad", false, errors.New("x"), nil, errors.New("y"),
			[]string{"toggle_store_error", "destructive_disabled", "window_store_error", "no_active_maintenance_window"}},
		{"only-disabled", false, nil, []networkactions.MaintenanceRecord{{}}, nil,
			[]string{"destructive_disabled"}},
		{"only-no-window", true, nil, nil, nil,
			[]string{"no_active_maintenance_window"}},
		{"happy-with-window", true, nil, []networkactions.MaintenanceRecord{{}}, nil,
			[]string{}},
	}
	for _, c := range cases {
		got := blockingReasonsFromState(c.enabled, c.toggleErr, c.windows, c.windowsErr)
		joined := strings.Join(got, ",")
		for _, must := range c.mustContain {
			if !strings.Contains(joined, must) {
				t.Errorf("%s: missing %q in %v", c.name, must, got)
			}
		}
		if c.name == "happy-with-window" && len(got) != 0 {
			t.Errorf("happy: expected empty, got %v", got)
		}
	}
}
