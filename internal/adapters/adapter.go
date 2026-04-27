// Package adapters, vendor-bağımsız cihaz erişim sözleşmelerini
// tanımlar. Her vendor (mikrotik, mimosa) bu paketin alt paketinde
// kendi adapter'ını uygular. Faz 1'de hiçbir adapter cihaza gerçek
// yazma çağrısı yapmaz; arayüz şekli sabittir.
package adapters

import (
	"context"
	"errors"
	"time"
)

// ErrCapabilityDisabled, bir cihazın istenen yetenek için işaretli
// olmadığı durumlarda döner.
var ErrCapabilityDisabled = errors.New("capability disabled")

// ErrWriteBlocked, Faz 1'de tüm yazma yollarında dönen sentinel
// hatadır. Üst katman bu hatayı operasyon ekrana net bir şekilde
// yansıtır.
var ErrWriteBlocked = errors.New("write operations blocked in phase 1")

// HealthSnapshot, cihazdan okunan minimum sağlık verisidir.
type HealthSnapshot struct {
	DeviceID     string
	CollectedAt  time.Time
	Online       bool
	UptimeSec    int64
	CPUPercent   float64
	MemPercent   float64
	TempCelsius  *float64
	FirmwareInfo string
}

// WirelessMetrics, kablosuz arayüzün anlık ölçümleridir.
type WirelessMetrics struct {
	DeviceID        string
	Interface       string
	CollectedAt     time.Time
	FrequencyMHz    int
	ChannelWidthMHz int
	TxPowerDBm      *float64
	NoiseFloor      *float64
	RSSI            *float64
	SNR             *float64
	CCQ             *float64
	TxRateMbps      *float64
	RxRateMbps      *float64
	TxBytes         *int64
	RxBytes         *int64
}

// ClientInfo, bir AP'ye bağlı CPE/istemci bilgisidir.
type ClientInfo struct {
	MAC         string
	IP          string
	Hostname    string
	RSSI        *float64
	SNR         *float64
	TxRateMbps  *float64
	RxRateMbps  *float64
	UptimeSec   int64
	Disconnects int
}

// Adapter, vendor-bağımsız sözleşmeyi tanımlar.
//
// Faz 1: yalnızca okuma metotları çağrılabilir. Yazma metotları
// (örn. ApplyFrequency, BackupConfig) ileri fazlarda eklenecek;
// implementasyonlar şu an ErrWriteBlocked dönmek zorundadır.
type Adapter interface {
	// Vendor, adapter'ın ürettiği üreticiyi döner.
	Vendor() string
	// Probe, kimlik bilgisinin geçerli olduğunu ve adapter'ın cihaza
	// erişebildiğini doğrular. Yazma yapmaz.
	Probe(ctx context.Context) error
	// ReadHealth, cihazın sağlık verisini okur.
	ReadHealth(ctx context.Context) (*HealthSnapshot, error)
	// ReadWirelessMetrics, varsa kablosuz arayüz ölçümlerini okur.
	ReadWirelessMetrics(ctx context.Context) ([]WirelessMetrics, error)
	// ReadClients, varsa istemci listesini okur.
	ReadClients(ctx context.Context) ([]ClientInfo, error)
}

// WriteCapableAdapter, ileri fazlarda eklenecek yazma yeteneklerini
// tanımlar. Faz 1'de hiçbir somut tip bu arayüzü uygulamaz.
type WriteCapableAdapter interface {
	Adapter
	BackupConfig(ctx context.Context) ([]byte, error)
	ApplyFrequency(ctx context.Context, freqMHz int) error
	Rollback(ctx context.Context, backup []byte) error
}
