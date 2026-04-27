// Package devices içindeki capability_matrix, vendor + rol bazında
// varsayılan yetkinlik bayraklarını sağlar. Probe gerçek değeri
// yazana kadar bu tablo "tahmini varsayılan" olarak kullanılır.
package devices

// DefaultCapabilities, verilen vendor + rol için Faz 2 başlangıç
// bayraklarını döner. Yazma yetenekleri (canApplyFrequency,
// canBackupConfig, canRollback) bu fazda DAİMA false döner.
func DefaultCapabilities(vendor Vendor, role Role) Capabilities {
	c := Capabilities{
		RequiresManualApply: true,
	}

	switch vendor {
	case VendorMikroTik:
		c.SupportsRouterOSAPI = true
		c.SupportsSSH = true
		c.SupportsSNMP = true
		c.CanReadHealth = true
		c.CanReadFrequency = true
		c.CanRecommendFrequency = true
		// Wireless metrics + clients only meaningful for wireless roles.
		switch role {
		case RoleAP, RolePTPMaster:
			c.CanReadWirelessMetrics = true
			c.CanReadClients = true
		case RoleCPE, RolePTPSlave:
			c.CanReadWirelessMetrics = true
		}

	case VendorMimosa:
		c.SupportsSNMP = true
		c.SupportsVendorAPI = false // proven per-model in later phases
		c.CanReadHealth = true
		c.CanReadWirelessMetrics = true
		c.CanReadFrequency = true
		c.CanRecommendFrequency = true
		switch role {
		case RoleAP, RolePTPMaster:
			c.CanReadClients = true
		}
	}

	// Phase 2 hard locks: all destructive paths off.
	c.CanRunScan = false
	c.CanBackupConfig = false
	c.CanApplyFrequency = false
	c.CanRollback = false

	return c
}
