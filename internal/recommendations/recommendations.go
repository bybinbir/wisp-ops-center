// Package recommendations, frekans/kanal değişikliği önerilerinin
// modelini tanımlar. Faz 1'de gerçek hesaplama YOKTUR; sadece
// arayüz, durum makinesi ve veri tipleri sabitlenir.
package recommendations

import "time"

// Status, önerinin yaşam döngüsündeki adımı tanımlar.
type Status string

const (
	StatusDraft      Status = "draft"
	StatusReviewed   Status = "reviewed"
	StatusApproved   Status = "approved"
	StatusApplied    Status = "applied"     // Faz 9
	StatusRolledBack Status = "rolled_back" // Faz 9
	StatusDismissed  Status = "dismissed"
)

// Risk, öneriye eşlik eden risk seviyesidir.
type Risk string

const (
	RiskLow    Risk = "low"
	RiskMedium Risk = "medium"
	RiskHigh   Risk = "high"
)

// Recommendation, tek bir cihaz/hat için frekans önerisi.
type Recommendation struct {
	ID                      string
	DeviceID                string
	LinkID                  string
	CurrentFrequencyMHz     int
	RecommendedFrequencyMHz int
	ChannelWidthMHz         int
	Risk                    Risk
	AffectedCustomerCount   int
	ExpectedImprovement     string
	Reasons                 []string
	Status                  Status
	CreatedAt               time.Time
	UpdatedAt               time.Time
}
