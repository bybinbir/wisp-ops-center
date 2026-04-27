package scheduler

import "errors"

// RiskLevel sınıflandırması — Faz 5 kuralları.
type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

// AllRiskLevels returns the supported risk levels.
func AllRiskLevels() []RiskLevel { return []RiskLevel{RiskLow, RiskMedium, RiskHigh} }

// IsValidRiskLevel reports whether r is a known risk level.
func IsValidRiskLevel(r RiskLevel) bool {
	for _, k := range AllRiskLevels() {
		if k == r {
			return true
		}
	}
	return false
}

// ErrControlledApplyForbidden is returned whenever a caller asks to
// schedule a job in controlled_apply mode. Faz 5 kapalı kalır.
var ErrControlledApplyForbidden = errors.New("scheduler: controlled_apply mode forbidden until Phase 9")

// ErrHighRiskNeedsApproval is returned when a high-risk job is queued
// without manual approval. Faz 5'te düşük riskli AP-client testler
// dışındaki yüksek riskli yollar zaten kapalı, ama sözleşme sabit.
var ErrHighRiskNeedsApproval = errors.New("scheduler: high-risk job requires manual approval")

// ErrOutsideMaintenanceWindow is returned for high-risk jobs scheduled
// outside their assigned maintenance window. Medium-risk jobs only get
// a warning (see WarnMediumRiskOutsideWindow). Low-risk passes.
var ErrOutsideMaintenanceWindow = errors.New("scheduler: outside maintenance window")

// ErrJobTypeUnknown is returned when the job type is not in JobCatalog.
var ErrJobTypeUnknown = errors.New("scheduler: unknown job type")

// ErrJobTypeDisabled is returned for jobs we know about but that are
// hard-locked off in the current phase (e.g. mikrotik_bandwidth_test).
var ErrJobTypeDisabled = errors.New("scheduler: job type disabled in this phase")
