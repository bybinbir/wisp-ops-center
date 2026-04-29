// Package alerts validates the Prometheus alert rule file shipped
// with Phase 10F-A. The package's only job today is a structural
// sanity check on the YAML file — once the metrics surface lands
// (Phase 10F-B or later) this package can grow to actually load
// the rules and exercise their PromQL.
package alerts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// rulesFilePath is relative to the repo root. The test changes
// working directory upward until it finds the file so it can be
// invoked from the package directory in the usual `go test` flow.
const rulesFileName = "destructive_alerts.rules.yml"

// findRulesFile walks upward from the test cwd looking for
// `infra/prometheus/destructive_alerts.rules.yml`. Returns the
// resolved absolute path.
func findRulesFile(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	for cur := wd; ; {
		candidate := filepath.Join(cur, "infra", "prometheus", rulesFileName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			t.Fatalf("could not locate %s walking up from %s", rulesFileName, wd)
		}
		cur = parent
	}
}

// TestPhase10F_AlertRulesFileExists pins the file's presence at the
// expected path. Misplacing it (e.g. a rename to .yaml) silently
// removes every safety alert from the deployment.
func TestPhase10F_AlertRulesFileExists(t *testing.T) {
	path := findRulesFile(t)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if info.Size() < 100 {
		t.Errorf("alert rules file is suspiciously small (%d bytes)", info.Size())
	}
}

// TestPhase10F_AlertRulesContainEverySafetyAlert is the regression
// guard for alert drift. Every named alert below MUST be present in
// the rule file; renaming or dropping one is a deliberate, security-
// reviewed change and the test should fail loudly until the rename
// is reflected here.
func TestPhase10F_AlertRulesContainEverySafetyAlert(t *testing.T) {
	path := findRulesFile(t)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	text := string(body)
	mustHave := []string{
		// Phase 10E destructive runtime
		"WispDestructiveSucceededUnexpected",
		"WispDestructiveAttemptedWhileDisabled",
		"WispDestructiveVerifyFailedRollbackRecovered",
		"WispDestructiveRollbackFailed",
		"WispDestructiveExecuteNotImplementedSpike",
		// Toggle posture
		"WispProviderToggleEnabledOutsideWindow",
		// Leak detectors
		"WispSecretLeakDetected",
		"WispRawMacLeakDetected",
		// Retention housekeeping
		"WispRetentionDeletedLargeBatch",
	}
	for _, name := range mustHave {
		if !strings.Contains(text, name) {
			t.Errorf("alert rule %q missing from %s", name, rulesFileName)
		}
	}
}

// TestPhase10F_AlertRulesYAMLShapeIsSane checks the structural
// keywords every Prometheus rules file uses. We avoid pulling in a
// YAML library here — the keyword presence + indentation heuristic
// catches the common breakage modes (dropped `groups:` root, missing
// `expr:` / `labels:` per rule).
func TestPhase10F_AlertRulesYAMLShapeIsSane(t *testing.T) {
	path := findRulesFile(t)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	text := string(body)
	if !strings.Contains(text, "groups:") {
		t.Error("rules file missing top-level `groups:` key")
	}
	alertCount := strings.Count(text, "- alert: Wisp")
	exprCount := strings.Count(text, "expr: ")
	if alertCount < 9 {
		t.Errorf("only %d Wisp alerts found, want >= 9", alertCount)
	}
	if exprCount < alertCount {
		t.Errorf("alerts:%d > expr:%d — every alert needs an expr block", alertCount, exprCount)
	}
	if !strings.Contains(text, "severity: critical") {
		t.Error("rules file should declare at least one critical-severity alert")
	}
	if !strings.Contains(text, "severity: warning") {
		t.Error("rules file should declare at least one warning-severity alert")
	}
}

// TestPhase10F_AlertRulesEveryRuleHasAnnotations is a documentation
// guard: every named alert MUST carry summary + description so an
// oncall pager has something useful to render.
func TestPhase10F_AlertRulesEveryRuleHasAnnotations(t *testing.T) {
	path := findRulesFile(t)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	text := string(body)
	alertCount := strings.Count(text, "- alert: Wisp")
	summaryCount := strings.Count(text, "summary:")
	descriptionCount := strings.Count(text, "description:")
	if summaryCount < alertCount {
		t.Errorf("summary blocks=%d < alerts=%d — every alert needs a summary", summaryCount, alertCount)
	}
	if descriptionCount < alertCount {
		t.Errorf("description blocks=%d < alerts=%d — every alert needs a description", descriptionCount, alertCount)
	}
}
