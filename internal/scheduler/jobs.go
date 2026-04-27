package scheduler

// JobCatalogEntry describes a job type, its default risk and whether
// it is currently allowed to run. Faz 5'in tek doğruluk kaynağıdır.
type JobCatalogEntry struct {
	Type        JobType
	Risk        RiskLevel
	Enabled     bool
	Description string
}

// JobCatalog enumerates every job type Phase 5 understands. Adding a
// type without first listing it here is rejected at the API boundary.
var JobCatalog = []JobCatalogEntry{
	// Read-only polls (Phase 3 + 4).
	{JobMikroTikReadOnlyPoll, RiskLow, true, "MikroTik salt-okuma poll (Phase 3)"},
	{JobMimosaReadOnlyPoll, RiskLow, true, "Mimosa salt-okuma poll (Phase 4)"},

	// Coarse-grained operational jobs.
	{JobTowerHealthCheck, RiskLow, true, "Tower-wide health check"},
	{JobCustomerSignalCheck, RiskLow, true, "Customer signal check (Phase 6 — telemetri+test → skor)"},
	{JobDailyNetworkCheck, RiskLow, true, "Daily network sweep"},
	{JobWeeklyNetworkReport, RiskLow, true, "Weekly network report"},
	// Frequency recommendation analysis stays planned-only in Phase 5.
	{JobFrequencyRecommendationAnaly, RiskMedium, false, "Frekans öneri motoru (Phase 8)"},

	// Phase 5 low-risk AP-client tests.
	{"ap_client_ping_latency", RiskLow, true, "AP→Client ping latency (Phase 5)"},
	{"ap_client_packet_loss", RiskLow, true, "AP→Client packet loss (Phase 5)"},
	{"ap_client_jitter", RiskLow, true, "AP→Client jitter (Phase 5)"},
	{"ap_client_traceroute", RiskLow, true, "AP→Client traceroute (Phase 5)"},

	// Disabled / planned high-risk tests.
	{"ap_client_limited_throughput", RiskMedium, false, "Limited throughput (Phase 5'te disabled)"},
	{"mikrotik_bandwidth_test", RiskHigh, false, "MikroTik bandwidth-test (Phase 9'da bile manuel onay)"},
}

// LookupJob returns the catalog entry for t.
func LookupJob(t JobType) (JobCatalogEntry, bool) {
	for _, e := range JobCatalog {
		if e.Type == t {
			return e, true
		}
	}
	return JobCatalogEntry{}, false
}

// EnsureJobAllowed validates that a job type is known and enabled.
func EnsureJobAllowed(t JobType) error {
	e, ok := LookupJob(t)
	if !ok {
		return ErrJobTypeUnknown
	}
	if !e.Enabled {
		return ErrJobTypeDisabled
	}
	return nil
}
