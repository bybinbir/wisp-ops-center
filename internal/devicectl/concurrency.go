package devicectl

// Faz R4 — probe katmanı için concurrency / rate limit altyapısı.
//
// Operatör kararı: aynı anda maksimum 10 cihazla başla, gerekirse
// 20'ye çıkar. 928 cihazlık envantere paralel bağlantı atmak hem
// Dude'u (komşu/ARP cache) hem ağ omurgasını hem de cihazların
// kendisini gereksiz zorlar; tek noktadan kontrollü ölçek esas.
//
// Bu paket cihaza bağlanmaz, sadece "kaç tane eşzamanlı probe
// çalışıyor" sayacını tutar ve `Acquire`/`Release` semantiği sunar.
// Her transport (RouterOS API / SSH / SNMP / Mimosa HTTP) aynı
// limitter'dan geçer.
//
// Tasarım notları:
//
//   • Default cap = 10. NewLimiter(0) çağrısı default'a düşer.
//   • Hard ceiling = 20. NewLimiter(50) gibi bir aşırı değer
//     hata değil, sessiz clamp olur — operatör daha sonra config
//     ile değiştirir, bu modül sadece sınırı zorlar.
//   • Acquire context iptaline saygı duyar; iptal halinde
//     `ctx.Err()` döner ve sayaç artmaz.
//   • Release sayacı kaç kez çağrılırsa çağrılsın 0'ın altına
//     inmez (re-entrant defensive).

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

// DefaultMaxConcurrentProbes operatör onayıyla başlangıç eşzamanlılık
// sayısı.
const DefaultMaxConcurrentProbes = 10

// HardConcurrencyCeiling konfig ne derse desin geçilemeyen tavan.
const HardConcurrencyCeiling = 20

// ErrLimiterClosed Limiter Close() çağrıldıktan sonra Acquire
// denenince döner.
var ErrLimiterClosed = errors.New("probe limiter kapatıldı")

// Limiter eşzamanlı probe sayısını sınırlar. Goroutine güvenlidir.
type Limiter struct {
	cap    int
	tokens chan struct{}

	mu     sync.Mutex
	closed bool
	in     atomic.Int64 // şu an aktif probe sayısı (gözlem için)
	max    atomic.Int64 // gözlemlenen pik
}

// NewLimiter yeni bir Limiter üretir. max <= 0 ise default kullanılır;
// max > tavan ise tavana clamp edilir.
func NewLimiter(max int) *Limiter {
	if max <= 0 {
		max = DefaultMaxConcurrentProbes
	}
	if max > HardConcurrencyCeiling {
		max = HardConcurrencyCeiling
	}
	l := &Limiter{
		cap:    max,
		tokens: make(chan struct{}, max),
	}
	for i := 0; i < max; i++ {
		l.tokens <- struct{}{}
	}
	return l
}

// Cap aktif kapasite limitini döner.
func (l *Limiter) Cap() int { return l.cap }

// InFlight şu an aktif probe sayısını döner (gözlem için).
func (l *Limiter) InFlight() int { return int(l.in.Load()) }

// PeakInFlight gözlenen pik aktif probe sayısını döner.
func (l *Limiter) PeakInFlight() int { return int(l.max.Load()) }

// Acquire ya bir token alır ve geri verme fonksiyonu döner, ya da
// context iptal olursa hata döner. Çağıran `defer release()` yapmalı.
func (l *Limiter) Acquire(ctx context.Context) (release func(), err error) {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil, ErrLimiterClosed
	}
	l.mu.Unlock()

	select {
	case <-l.tokens:
		now := l.in.Add(1)
		// Pik gözlemi (race-tolerant; CAS gerek yok, ufak gözlem
		// hatası kabul edilebilir).
		if now > l.max.Load() {
			l.max.Store(now)
		}
		var once sync.Once
		return func() {
			once.Do(func() {
				l.in.Add(-1)
				select {
				case l.tokens <- struct{}{}:
				default:
					// kanal full ise (aşırı release) sessizce yut;
					// re-entrant savunma.
				}
			})
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Close limiteri kapatır; bekleyen Acquire çağrıları context iptaliyle
// dönmelidir. Bu modül kanal kapatmaz çünkü Acquire'ın "iptal halinde
// token kaybetme" garantisi var.
func (l *Limiter) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closed = true
}
