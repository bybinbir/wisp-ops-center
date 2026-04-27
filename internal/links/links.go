// Package links, PTP/PTMP hat alan modelini tanımlar.
package links

import "time"

// Topology, hattın topolojisini tanımlar.
type Topology string

const (
	TopologyPTP  Topology = "ptp"
	TopologyPTMP Topology = "ptmp"
)

// Risk, hat için hesaplanan risk seviyesidir.
type Risk string

const (
	RiskHealthy  Risk = "healthy"
	RiskWatch    Risk = "watch"
	RiskWarning  Risk = "warning"
	RiskCritical Risk = "critical"
)

// Link, iki cihaz arasındaki kablosuz bağlantıdır (PTP) veya bir
// master + birden çok slave (PTMP).
type Link struct {
	ID               string
	Name             string
	Topology         Topology
	MasterDeviceID   string
	SlaveDeviceIDs   []string
	FrequencyMHz     int
	ChannelWidthMHz  int
	LastSignalDBm    *float64
	LastSNRDB        *float64
	LastCapacityMbps *float64
	Risk             Risk
	LastCheckedAt    *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}
