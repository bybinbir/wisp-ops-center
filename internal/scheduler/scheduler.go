// Package scheduler, zamanlanmış kontrolleri ve iş kuyruğu sözleşmesini
// tanımlar. Faz 1'de yalnızca tip ve sözleşme tanımları yer alır;
// gerçek Asynq/Redis bağlantısı Faz 5'te eklenecektir.
package scheduler

import "time"

// JobType, zamanlanabilir iş türlerini tanımlar.
type JobType string

const (
	JobDailyNetworkCheck            JobType = "daily_network_check"
	JobWeeklyNetworkReport          JobType = "weekly_network_report"
	JobTowerHealthCheck             JobType = "tower_health_check"
	JobCustomerSignalCheck          JobType = "customer_signal_check"
	JobMikroTikReadOnlyPoll         JobType = "mikrotik_readonly_poll"
	JobMimosaReadOnlyPoll           JobType = "mimosa_readonly_poll"
	JobFrequencyRecommendationAnaly JobType = "frequency_recommendation_analysis"
)

// AllJobTypes, kayıt için tek noktadan iş listesi.
func AllJobTypes() []JobType {
	return []JobType{
		JobDailyNetworkCheck,
		JobWeeklyNetworkReport,
		JobTowerHealthCheck,
		JobCustomerSignalCheck,
		JobMikroTikReadOnlyPoll,
		JobMimosaReadOnlyPoll,
		JobFrequencyRecommendationAnaly,
	}
}

// Cadence, zamanlama tekrarlama biçimi.
type Cadence string

const (
	CadenceOnce        Cadence = "once"
	CadenceDaily       Cadence = "daily"
	CadenceWeekly      Cadence = "weekly"
	CadenceMonthly     Cadence = "monthly"
	CadenceMaintenance Cadence = "maintenance_window"
)

// ActionMode, kontrolün ne kadar otonom çalıştığını belirler.
type ActionMode string

const (
	// ModeReportOnly: sadece okuma + rapor.
	ModeReportOnly ActionMode = "report_only"
	// ModeRecommendOnly: rapor + öneri üret, uygulama yok.
	ModeRecommendOnly ActionMode = "recommend_only"
	// ModeManualApproval: öneriyi UI'da operatöre sun, onay bekle.
	ModeManualApproval ActionMode = "manual_approval"
	// ModeControlledApply: bakım penceresinde kontrollü uygula.
	// Faz 1'de DEVRE DIŞIDIR.
	ModeControlledApply ActionMode = "controlled_apply"
)

// Scope, kontrolün hedef kapsamıdır.
type Scope struct {
	SiteIDs   []string
	TowerIDs  []string
	DeviceIDs []string
	LinkIDs   []string
}

// ScheduledCheck, kullanıcı tarafından tanımlanmış zamanlı bir
// kontroldür.
type ScheduledCheck struct {
	ID        string
	Name      string
	JobType   JobType
	Cadence   Cadence
	Mode      ActionMode
	Scope     Scope
	NextRunAt *time.Time
	LastRunAt *time.Time
	Enabled   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// JobRun, tek bir iş yürütmesinin özetidir.
type JobRun struct {
	ID         string
	CheckID    string
	JobType    JobType
	StartedAt  time.Time
	FinishedAt *time.Time
	Status     RunStatus
	ErrorText  string
	Summary    map[string]any
}

// RunStatus, iş yürütme durumu.
type RunStatus string

const (
	RunPending RunStatus = "pending"
	RunRunning RunStatus = "running"
	RunSuccess RunStatus = "success"
	RunFailed  RunStatus = "failed"
	RunBlocked RunStatus = "blocked"
)
