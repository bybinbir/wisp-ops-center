// Package devices, MikroTik ve Mimosa cihazlarının ortak alan
// modelini tanımlar. Faz 1 yalnızca tipleri içerir; depolama ve
// senkronizasyon mantığı sonraki fazlarda eklenecektir.
package devices

import "time"

// Vendor, cihaz üreticisini tanımlar.
type Vendor string

const (
	VendorMikroTik Vendor = "mikrotik"
	VendorMimosa   Vendor = "mimosa"
	VendorUnknown  Vendor = "unknown"
)

// Role, cihazın ağdaki işlevini tanımlar.
type Role string

const (
	RoleAP        Role = "ap"
	RoleCPE       Role = "cpe"
	RolePTPMaster Role = "ptp_master"
	RolePTPSlave  Role = "ptp_slave"
	RoleRouter    Role = "router"
	RoleSwitch    Role = "switch"
)

// Device, envanterdeki bir donanım cihazını temsil eder.
type Device struct {
	ID           string
	Name         string
	Vendor       Vendor
	Role         Role
	IP           string
	SiteID       string
	TowerID      string
	Model        string
	Firmware     string
	RouterOS     string
	Capabilities Capabilities
	LastSeenAt   *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Capabilities, bir cihazın yetkinlik bayraklarını tutar.
// Ayrıntılı tanım docs/DEVICE_CAPABILITY_MODEL.md içindedir.
type Capabilities struct {
	CanReadHealth          bool
	CanReadWirelessMetrics bool
	CanReadClients         bool
	CanReadFrequency       bool
	CanRunScan             bool
	CanRecommendFrequency  bool
	CanBackupConfig        bool
	CanApplyFrequency      bool
	CanRollback            bool
	RequiresManualApply    bool
	SupportsSNMP           bool
	SupportsRouterOSAPI    bool
	SupportsSSH            bool
	SupportsVendorAPI      bool
}
