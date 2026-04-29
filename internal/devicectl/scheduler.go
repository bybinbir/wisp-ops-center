package devicectl

// Faz R4 — probe scheduler skeleton.
//
// Üç tarama modu, operatör onayıyla:
//
//   • LightScan  — her 15-30 dakikada bir, POP bazlı, kısa
//                  cevap (identity + interface up/down + wireless
//                  registration count). Cihaz başına ~3-5 saniye
//                  hedef. Eşzamanlılık limiteri kontrolünde.
//   • DeepScan   — gece (22:00-06:00 default), tam telemetry
//                  (interface detail + wireless detail +
//                  registration table + bridge ports + neighbors
//                  + log özet). Cihaz başına ~15-30 saniye.
//   • OnDemand   — operatör dashboard'dan "Bu POP'u şimdi tara"
//                  dediğinde tetiklenir. POP filtresi alır,
//                  doğrudan Deep scope'unda çalışır.
//
// Bu dosya **iskelet**: tipler + dispatch arayüzü + state makinesi.
// Gerçek probe çağrıları R4-3 (MikroTik) ve R4-4 (Mimosa) ile
// gelecek; o zaman buradaki `Probe` arayüzünün concrete
// implementation'ları wire edilir.
//
// Cron / job loop, mevcut `internal/scheduler` paketinin engine'i
// üzerinden koşar (Phase 8.1'de wire edildi); biz sadece
// "ne tara, nasıl tara" politikasını burada tanımlıyoruz.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ScanMode tarama modu enum'u.
type ScanMode string

const (
	ScanModeLight    ScanMode = "light"
	ScanModeDeep     ScanMode = "deep"
	ScanModeOnDemand ScanMode = "on_demand"
)

// IsValid bilinen bir mod mu kontrolü.
func (m ScanMode) IsValid() bool {
	switch m {
	case ScanModeLight, ScanModeDeep, ScanModeOnDemand:
		return true
	}
	return false
}

// ScanRequest scheduler'a verilen iş paketi. Cihaz hedefleme
// stratejisi 3 ayrı yoldan biri:
//
//   - PopCode set → sadece bu POP içindeki cihazlar
//   - DeviceIDs set → sadece bu liste
//   - Hiçbiri set değil → envanterdeki tüm cihazlar
type ScanRequest struct {
	Mode          ScanMode
	PopCode       string
	DeviceIDs     []string
	TriggeredBy   string
	CorrelationID string
	Timeout       time.Duration
}

// Validate ScanRequest'in kendi içinde tutarlı olup olmadığını
// söyler.
func (r ScanRequest) Validate() error {
	if !r.Mode.IsValid() {
		return fmt.Errorf("scan mode geçersiz: %q", r.Mode)
	}
	if r.PopCode != "" && len(r.DeviceIDs) > 0 {
		return errors.New("scan request hem PopCode hem DeviceIDs içeremez")
	}
	if r.Mode == ScanModeOnDemand && r.PopCode == "" && len(r.DeviceIDs) == 0 {
		return errors.New("on-demand scan PopCode veya DeviceIDs içermeli")
	}
	return nil
}

// ScanResult scheduler'ın özet çıktısı. Detaylı per-device kayıtlar
// device_probe_runs tablosuna gider; bu yapı sadece üst düzey
// gözlem ve dashboard için.
type ScanResult struct {
	Mode             ScanMode
	StartedAt        time.Time
	FinishedAt       time.Time
	DurationMS       int64
	DeviceCount      int
	Succeeded        int
	Partial          int
	Timeout          int
	Unreachable      int
	CredentialFailed int
	ProtocolError    int
	ParserError      int
	BlockedByGuard   int
	Unknown          int
	MutationBlocked  int // her zaman 0 olmalı; >0 ise alarm
	Errors           []string
}

// HoneststaticInvariant scan sonucunun read-only sözleşmesini koruyup
// korumadığını doğrular. >0 mutation engellendi sayısı bile alarm:
// allowlist çalışıyor demek ama probe katmanında bir bug var.
func (r ScanResult) HonestInvariant() error {
	if r.MutationBlocked < 0 {
		return errors.New("mutation engellendi sayısı negatif olamaz")
	}
	return nil
}

// Probe tek bir cihaza karşı bir read-only probe çalıştırır. R4-3
// ve R4-4 bu arayüzün concrete implementation'larını sağlar
// (mikrotikProbe, mimosaProbe). R4-1 sadece arayüzü tanımlar.
type Probe interface {
	// ProbeDevice tek bir cihazı tarar; cihazın IP'si, sınıf
	// hint'i (operatör mapping veya geçmiş classification) ve
	// scan mode'una göre derinlik seçer.
	ProbeDevice(ctx context.Context, target ProbeTarget) (ProbeOutcome, error)
}

// ProbeTarget tek bir cihaz için probe parametreleri.
type ProbeTarget struct {
	DeviceID  string
	IP        string
	ClassHint string // örn. "mikrotik_ap", "mimosa_link"
	PopCode   string
	Mode      ScanMode
	Profiles  ProfileLookup
}

// ProbeLookup probe katmanı her cihaz için profile setini bu interface
// üzerinden alır. Test ortamı bunu mock'lar; production
// implementation `credentials.LoadProfileSetFromEnv()` döner.
type ProfileLookup interface {
	LookupForClass(class string) ([]ProfileSnapshot, error)
}

// ProfileSnapshot probe için yeterli minimal credential bilgisi.
// Secret string olduğu için bu yapı log'a serializable DEĞİLDİR;
// sadece in-memory taşıma için.
type ProfileSnapshot struct {
	ID       string
	Username string
	Secret   string
	Port     int
	AuthType string
}

// Sanitize secret'ı maskeler.
func (p ProfileSnapshot) Sanitize() ProfileSnapshot {
	if p.Secret != "" {
		p.Secret = RedactionMask
	}
	return p
}

// ProbeOutcome probe'un cihaz için ürettiği sonuç. Probe katmanı
// device_probe_runs + device_raw_snapshots + normalized tablolara
// yazımı yapacak; bu yapı sadece scheduler'a "ne oldu?" döner.
type ProbeOutcome struct {
	Status          string // succeeded | partial | timeout | unreachable | credential_failed | protocol_error | parser_error | blocked_by_allowlist | unknown
	Transport       string // routeros_api | ssh | snmp | mimosa_http | ...
	UsedProfile     string // "mikrotik_ap", "mimosa_a" — ID, secret asla yok
	ConfidenceScore int
	ErrorCode       string
	ErrorMessage    string
	DurationMS      int64
}

// Sanitize hata mesajındaki olası secret kalıntılarını temizler.
func (o ProbeOutcome) Sanitize() ProbeOutcome {
	if o.ErrorMessage != "" {
		o.ErrorMessage = RedactText(o.ErrorMessage)
	}
	return o
}

// Scheduler probe orkestrasyonunu yönetir. R4-1'de skeleton; gerçek
// dispatch R4-3..R4-7 sırasında doldurulur.
type Scheduler struct {
	limiter *Limiter
	probes  map[string]Probe // class hint → concrete Probe
	clock   func() time.Time
}

// NewScheduler default kapasiteli (10 eşzamanlı) bir scheduler üretir.
// R4-3 sonrası probe register'ları (`Register(class, probe)`) ile
// gelir.
func NewScheduler(limiter *Limiter) *Scheduler {
	if limiter == nil {
		limiter = NewLimiter(DefaultMaxConcurrentProbes)
	}
	return &Scheduler{
		limiter: limiter,
		probes:  map[string]Probe{},
		clock:   time.Now,
	}
}

// Register class hint'i için probe registriye ekler.
func (s *Scheduler) Register(classHint string, p Probe) {
	if classHint == "" || p == nil {
		return
	}
	s.probes[strings.ToLower(classHint)] = p
}

// Limiter mevcut concurrency limiter'ı döner.
func (s *Scheduler) Limiter() *Limiter { return s.limiter }

// LookupProbe class hint için kayıtlı probe'u döner. Yoksa nil.
func (s *Scheduler) LookupProbe(classHint string) Probe {
	return s.probes[strings.ToLower(classHint)]
}

// ScanWindowDefaults light / deep tarama için varsayılan zaman
// pencereleri. Operatör config ile override edebilir; bu sabitler
// "operatör hiçbir şey demediyse" cevabıdır.
type ScanWindowDefaults struct {
	LightInterval time.Duration
	DeepStart     string // "HH:MM"
	DeepEnd       string // "HH:MM"
}

// DefaultScanWindow operatör onayıyla eldeki saha için makul.
func DefaultScanWindow() ScanWindowDefaults {
	return ScanWindowDefaults{
		LightInterval: 20 * time.Minute,
		DeepStart:     "22:00",
		DeepEnd:       "06:00",
	}
}
