package devicectl

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestNewLimiter_DefaultsToTen(t *testing.T) {
	l := NewLimiter(0)
	if l.Cap() != DefaultMaxConcurrentProbes {
		t.Errorf("kapasite default 10 olmalıydı, %d geldi", l.Cap())
	}
}

func TestNewLimiter_ClampsAboveCeiling(t *testing.T) {
	l := NewLimiter(99)
	if l.Cap() != HardConcurrencyCeiling {
		t.Errorf("üst tavan 20 olmalıydı, %d geldi", l.Cap())
	}
}

func TestLimiter_AcquireRelease_RespectsCap(t *testing.T) {
	const cap = 3
	l := NewLimiter(cap)
	ctx := context.Background()

	releases := []func(){}
	for i := 0; i < cap; i++ {
		rel, err := l.Acquire(ctx)
		if err != nil {
			t.Fatalf("acquire %d hata: %v", i, err)
		}
		releases = append(releases, rel)
	}

	// Bir tane daha denersek bloklanmalı; kısa zaman aşımıyla bekleyip
	// gerçekten bloklandığını doğrulayalım.
	tCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	if _, err := l.Acquire(tCtx); err == nil {
		t.Fatal("kapasite dolu iken acquire blok'lanıp ctx hata ile dönmeliydi")
	}

	// Birini serbest bırakırsak yeni acquire geçmeli.
	releases[0]()
	tCtx2, cancel2 := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel2()
	rel, err := l.Acquire(tCtx2)
	if err != nil {
		t.Fatalf("release sonrası acquire başarısız: %v", err)
	}
	rel()
}

func TestLimiter_PeakInFlightTracksObservedMax(t *testing.T) {
	l := NewLimiter(5)
	ctx := context.Background()

	rels := []func(){}
	for i := 0; i < 4; i++ {
		rel, err := l.Acquire(ctx)
		if err != nil {
			t.Fatalf("acquire hata: %v", err)
		}
		rels = append(rels, rel)
	}
	if got := l.InFlight(); got != 4 {
		t.Errorf("InFlight=%d, want 4", got)
	}
	if got := l.PeakInFlight(); got < 4 {
		t.Errorf("PeakInFlight en az 4 olmalı, %d geldi", got)
	}
	for _, r := range rels {
		r()
	}
	if got := l.InFlight(); got != 0 {
		t.Errorf("release sonrası InFlight=%d, want 0", got)
	}
	if got := l.PeakInFlight(); got < 4 {
		t.Errorf("Peak release sonrası bile düşmemeli, %d geldi", got)
	}
}

func TestLimiter_ContextCancelDoesNotLoseToken(t *testing.T) {
	l := NewLimiter(2)
	ctx := context.Background()

	r1, err := l.Acquire(ctx)
	if err != nil {
		t.Fatalf("ilk acquire hata: %v", err)
	}
	r2, err := l.Acquire(ctx)
	if err != nil {
		t.Fatalf("ikinci acquire hata: %v", err)
	}

	tCtx, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
	defer cancel()
	if _, err := l.Acquire(tCtx); err == nil {
		t.Fatal("kapasite dolu iken acquire ctx hata ile dönmeliydi")
	}

	r1()
	r2()
	r3, err := l.Acquire(ctx)
	if err != nil {
		t.Fatalf("release sonrası acquire başarısız (token kaybı?): %v", err)
	}
	r3()
}

func TestLimiter_ReleaseIdempotent(t *testing.T) {
	l := NewLimiter(1)
	ctx := context.Background()
	rel, err := l.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rel()
	rel() // ikinci release sessizce yutulmalı
	rel = nil

	// Hâlâ acquire edilebilir mi?
	tCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	rel2, err := l.Acquire(tCtx)
	if err != nil {
		t.Fatalf("ikinci release sonrası acquire başarısız: %v", err)
	}
	rel2()
}

func TestLimiter_Concurrent(t *testing.T) {
	const cap = 8
	l := NewLimiter(cap)
	ctx := context.Background()

	var wg sync.WaitGroup
	const workers = 50
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rel, err := l.Acquire(ctx)
			if err != nil {
				t.Errorf("acquire hata: %v", err)
				return
			}
			defer rel()
			time.Sleep(2 * time.Millisecond)
		}()
	}
	wg.Wait()
	if got := l.PeakInFlight(); got > cap {
		t.Errorf("Peak kapasite üstüne çıktı: %d > %d", got, cap)
	}
	if got := l.InFlight(); got != 0 {
		t.Errorf("Tüm worker bittikten sonra InFlight=%d, want 0", got)
	}
}
