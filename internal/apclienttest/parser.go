package apclienttest

import (
	"errors"
	"math"
	"regexp"
	"strconv"
)

// Phase 5 ping/packet_loss/jitter all flow through one path: we run
// the host `ping` command (when available) and parse its summary
// output. The parser is intentionally minimal and only consumes
// numbers it recognizes; missing fields fall back to nil.

var (
	rePingTimeMs   = regexp.MustCompile(`time[=<]([0-9]+\.?[0-9]*)\s*ms`)
	rePingLossLine = regexp.MustCompile(`([0-9]+\.?[0-9]*)% packet loss`)
	rePingRttLine  = regexp.MustCompile(`= ([0-9]+\.?[0-9]*)/([0-9]+\.?[0-9]*)/([0-9]+\.?[0-9]*)/([0-9]+\.?[0-9]*) ms`)
)

// ParsePing returns latency stats + loss + jitter from `ping` output.
// Output format expected: BSD/GNU ping summary block.
func ParsePing(out string) (lossPct float64, minMs, avgMs, maxMs, jitterMs *float64, err error) {
	if out == "" {
		return 0, nil, nil, nil, nil, errors.New("empty ping output")
	}
	if m := rePingLossLine.FindStringSubmatch(out); len(m) == 2 {
		if v, perr := strconv.ParseFloat(m[1], 64); perr == nil {
			lossPct = v
		}
	}
	// Some ping implementations (busybox) report rtt min/avg/max only.
	if m := rePingRttLine.FindStringSubmatch(out); len(m) == 5 {
		minV, _ := strconv.ParseFloat(m[1], 64)
		avgV, _ := strconv.ParseFloat(m[2], 64)
		maxV, _ := strconv.ParseFloat(m[3], 64)
		mdevV, _ := strconv.ParseFloat(m[4], 64)
		minMs = &minV
		avgMs = &avgV
		maxMs = &maxV
		jitterMs = &mdevV
		return
	}
	// Fallback: walk individual time= entries to compute min/avg/max + stddev.
	matches := rePingTimeMs.FindAllStringSubmatch(out, -1)
	if len(matches) == 0 {
		return lossPct, nil, nil, nil, nil, nil
	}
	samples := make([]float64, 0, len(matches))
	for _, m := range matches {
		if v, perr := strconv.ParseFloat(m[1], 64); perr == nil {
			samples = append(samples, v)
		}
	}
	if len(samples) == 0 {
		return lossPct, nil, nil, nil, nil, nil
	}
	mn, mx := samples[0], samples[0]
	var sum float64
	for _, s := range samples {
		if s < mn {
			mn = s
		}
		if s > mx {
			mx = s
		}
		sum += s
	}
	avg := sum / float64(len(samples))
	var sd float64
	for _, s := range samples {
		sd += (s - avg) * (s - avg)
	}
	sd = math.Sqrt(sd / float64(len(samples)))
	minMs = &mn
	avgMs = &avg
	maxMs = &mx
	jitterMs = &sd
	return
}

var reTraceHop = regexp.MustCompile(`^\s*([0-9]+)\s+`)

// ParseTraceroute returns the hop count from a traceroute output.
func ParseTraceroute(out string) int {
	if out == "" {
		return 0
	}
	hops := 0
	for _, line := range splitLines(out) {
		if m := reTraceHop.FindStringSubmatch(line); len(m) == 2 {
			if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
				if n > hops {
					hops = n
				}
			}
		}
	}
	return hops
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}
