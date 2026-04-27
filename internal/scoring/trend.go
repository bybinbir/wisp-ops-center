package scoring

import (
	"sort"
	"time"
)

// SignalSample, trend hesaplaması için tek bir örnek.
type SignalSample struct {
	At      time.Time
	RSSIdBm float64
}

// SignalTrend7d, son 7 günde dB/gün eğimi (negatif = kötüleşme).
// En az 3 örnek gerekir; aksi halde nil döner.
//
// Basit en küçük kareler regresyon: y = a + b*x
// b (eğim) dB/saniye → günlüğe çevir.
func SignalTrend7d(samples []SignalSample, now time.Time) *float64 {
	if len(samples) < 3 {
		return nil
	}
	cutoff := now.Add(-7 * 24 * time.Hour)
	filtered := make([]SignalSample, 0, len(samples))
	for _, s := range samples {
		if s.At.After(cutoff) {
			filtered = append(filtered, s)
		}
	}
	if len(filtered) < 3 {
		return nil
	}
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].At.Before(filtered[j].At)
	})
	// Zamanı saniye olarak alıp en küçük örneği 0 yapalım
	t0 := filtered[0].At
	var sumX, sumY, sumXY, sumX2 float64
	n := float64(len(filtered))
	for _, s := range filtered {
		x := s.At.Sub(t0).Seconds()
		y := s.RSSIdBm
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}
	denom := n*sumX2 - sumX*sumX
	if denom == 0 {
		return nil
	}
	slopePerSecond := (n*sumXY - sumX*sumY) / denom
	slopePerDay := slopePerSecond * 86400
	return &slopePerDay
}
