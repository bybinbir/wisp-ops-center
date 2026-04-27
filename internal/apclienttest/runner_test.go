package apclienttest

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestValidateBoundsAndAllowlist(t *testing.T) {
	cases := []struct {
		req TestRequest
		err error
	}{
		{TestRequest{Type: TypeMikroTikBandwidthTest, TargetIP: "10.0.0.1"}, ErrTestDisabled},
		{TestRequest{Type: TypeLimitedThroughput, TargetIP: "10.0.0.1"}, ErrTestDisabled},
		{TestRequest{Type: "rocket-launch", TargetIP: "10.0.0.1"}, ErrTestUnknown},
		{TestRequest{Type: TypePingLatency, TargetIP: "not-an-ip"}, ErrInvalidTarget},
		{TestRequest{Type: TypePingLatency, TargetIP: "10.0.0.1", Count: 99}, ErrCountOutOfRange},
		{TestRequest{Type: TypePingLatency, TargetIP: "10.0.0.1", Timeout: time.Hour}, ErrTimeoutOutOfRange},
		{TestRequest{Type: TypePingLatency, TargetIP: "10.0.0.1", MaxDuration: time.Hour}, ErrDurationOutOfRange},
	}
	for _, c := range cases {
		if err := c.req.Validate(); !errors.Is(err, c.err) {
			t.Fatalf("Validate(%v) = %v, want %v", c.req, err, c.err)
		}
	}
	ok := TestRequest{Type: TypePingLatency, TargetIP: "10.0.0.1"}
	if err := ok.Validate(); err != nil {
		t.Fatalf("default values must validate: %v", err)
	}
	if ok.Count != 5 || ok.Timeout == 0 || ok.MaxDuration == 0 || ok.RiskLevel != "low" {
		t.Fatalf("defaults not applied: %+v", ok)
	}
}

func TestParsePingGNUStyleSummary(t *testing.T) {
	out := `PING 10.0.0.1 (10.0.0.1) 56(84) bytes of data.
64 bytes from 10.0.0.1: time=1.20 ms
64 bytes from 10.0.0.1: time=1.30 ms
64 bytes from 10.0.0.1: time=1.40 ms
--- 10.0.0.1 ping statistics ---
3 packets transmitted, 3 received, 0% packet loss, time 2003ms
rtt min/avg/max/mdev = 1.200/1.300/1.400/0.082 ms
`
	loss, mn, avg, mx, jit, err := ParsePing(out)
	if err != nil {
		t.Fatal(err)
	}
	if loss != 0 || mn == nil || avg == nil || mx == nil || jit == nil {
		t.Fatalf("parse failed: %v %v %v %v %v", loss, mn, avg, mx, jit)
	}
	if *avg != 1.300 || *mx != 1.400 {
		t.Fatalf("unexpected avg/max: %v %v", *avg, *mx)
	}
}

func TestParsePingFallbackComputesStats(t *testing.T) {
	out := `64 bytes from 10.0.0.1: time=10 ms
64 bytes from 10.0.0.1: time=14 ms
64 bytes from 10.0.0.1: time=12 ms
2 packets transmitted, 2 received, 0% packet loss
`
	loss, mn, avg, mx, jit, err := ParsePing(out)
	if err != nil {
		t.Fatal(err)
	}
	if loss != 0 || mn == nil || avg == nil || mx == nil || jit == nil {
		t.Fatal("fallback parse failed")
	}
	if *mn != 10 || *mx != 14 {
		t.Fatalf("unexpected min/max: %v %v", *mn, *mx)
	}
}

func TestParseTraceroute(t *testing.T) {
	out := ` 1  10.0.0.1  1.234 ms
 2  192.168.1.1  4.234 ms
 3  *
 4  8.8.8.8  9.0 ms
`
	if hops := ParseTraceroute(out); hops != 4 {
		t.Fatalf("expected 4 hops, got %d", hops)
	}
}

func TestRunBlocksDisabledTypes(t *testing.T) {
	r := &Runner{}
	res := r.Run(context.Background(), TestRequest{
		Type: TypeMikroTikBandwidthTest, TargetIP: "10.0.0.1",
	})
	if res.Status != "blocked" || res.ErrorCode != "test_disabled" {
		t.Fatalf("disabled test must be blocked: %+v", res)
	}
}

func TestRunPingViaInjectedExecutor(t *testing.T) {
	r := &Runner{
		PingExec: func(ctx context.Context, target string, count int, timeout time.Duration) (string, error) {
			return `64 bytes from 10.0.0.1: time=1 ms
64 bytes from 10.0.0.1: time=2 ms
2 packets transmitted, 2 received, 0% packet loss
rtt min/avg/max/mdev = 1.000/1.500/2.000/0.500 ms`, nil
		},
	}
	res := r.Run(context.Background(), TestRequest{
		Type: TypePingLatency, TargetIP: "10.0.0.1",
	})
	if res.Status != "success" {
		t.Fatalf("expected success: %+v", res)
	}
	if res.Diagnosis != DiagHealthy {
		t.Fatalf("expected healthy: %s", res.Diagnosis)
	}
	if res.LatencyAvgMs == nil || *res.LatencyAvgMs != 1.5 {
		t.Fatalf("avg parse failed: %v", res.LatencyAvgMs)
	}
}

func TestRunPing100PctLossUnreachable(t *testing.T) {
	r := &Runner{
		PingExec: func(ctx context.Context, target string, count int, timeout time.Duration) (string, error) {
			return `--- 10.0.0.99 ping statistics ---
5 packets transmitted, 0 received, 100% packet loss, time 4093ms`, errors.New("exit 1")
		},
	}
	res := r.Run(context.Background(), TestRequest{
		Type: TypePingLatency, TargetIP: "10.0.0.99",
	})
	if res.Diagnosis != DiagUnreachable {
		t.Fatalf("expected unreachable: %+v", res)
	}
}

func TestRunDoesNotLeakErrorBytes(t *testing.T) {
	// The runner must never include credentials / device tokens in its
	// ErrorMessage. This test passes a long synthetic error and ensures
	// it gets capped.
	r := &Runner{
		PingExec: func(ctx context.Context, target string, count int, timeout time.Duration) (string, error) {
			return "", errors.New(strings.Repeat("x", 5000))
		},
	}
	res := r.Run(context.Background(), TestRequest{
		Type: TypePingLatency, TargetIP: "10.0.0.1",
	})
	// ErrorMessage may be empty in the partial path; if set, it must be capped.
	if len(res.ErrorMessage) > 250 {
		t.Fatalf("error message not capped: %d chars", len(res.ErrorMessage))
	}
}
