package apclienttest

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"time"
)

// Runner executes one bounded test against a target IP. The runner
// shells out to system `ping` / `traceroute` only — no device-side
// command execution.
type Runner struct {
	// PingExec lets tests inject a fake `ping` command. Production
	// uses the host binary when nil.
	PingExec  func(ctx context.Context, target string, count int, timeout time.Duration) (string, error)
	TraceExec func(ctx context.Context, target string, maxHops int) (string, error)
}

// Default executors use the host binaries.
func defaultPing(ctx context.Context, target string, count int, timeout time.Duration) (string, error) {
	args := []string{"-c", strconv.Itoa(count), "-W", strconv.Itoa(int(timeout.Seconds())), target}
	cmd := exec.CommandContext(ctx, "ping", args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func defaultTraceroute(ctx context.Context, target string, maxHops int) (string, error) {
	cmd := exec.CommandContext(ctx, "traceroute", "-n", "-q", "1", "-m", strconv.Itoa(maxHops), target)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// Run executes the request and returns a normalized TestResult. Errors
// are sanitized into ErrorCode/ErrorMessage; a transport failure is
// reported as Diagnosis=unreachable rather than a Go error.
func (r *Runner) Run(ctx context.Context, req TestRequest) TestResult {
	res := TestResult{
		Type:             req.Type,
		APDeviceID:       req.APDeviceID,
		CustomerID:       req.CustomerID,
		CustomerDeviceID: req.CustomerDeviceID,
		TargetIP:         req.TargetIP,
		StartedAt:        time.Now().UTC(),
		Status:           "success",
		RiskLevel:        req.RiskLevel,
	}
	if err := req.Validate(); err != nil {
		res.Status = "blocked"
		res.Diagnosis = DiagDataInsufficient
		res.ErrorCode = errCode(err)
		res.ErrorMessage = err.Error()
		res.FinishedAt = time.Now().UTC()
		res.DurationMS = res.FinishedAt.Sub(res.StartedAt).Milliseconds()
		return res
	}

	pingExec := r.PingExec
	if pingExec == nil {
		pingExec = defaultPing
	}
	traceExec := r.TraceExec
	if traceExec == nil {
		traceExec = defaultTraceroute
	}

	bounded, cancel := context.WithTimeout(ctx, req.MaxDuration)
	defer cancel()

	switch req.Type {
	case TypePingLatency, TypePacketLoss, TypeJitter:
		out, err := pingExec(bounded, req.TargetIP, req.Count, req.Timeout)
		loss, mn, avg, mx, jit, _ := ParsePing(out)
		res.LatencyMinMs = mn
		res.LatencyAvgMs = avg
		res.LatencyMaxMs = mx
		l := loss
		res.PacketLossPercent = &l
		res.JitterMs = jit
		res.Diagnosis = classifyPing(loss, avg, jit)
		if err != nil {
			// non-zero exit. ping returns 1 on 100% loss; treat as unreachable.
			if loss >= 100 {
				res.Status = "success"
				res.Diagnosis = DiagUnreachable
			} else {
				res.Status = "partial"
			}
		}
	case TypeTraceroute:
		out, err := traceExec(bounded, req.TargetIP, 30)
		if err != nil {
			res.Status = "partial"
			res.ErrorCode = "trace_partial"
			res.ErrorMessage = sanitize(err.Error())
		}
		hops := ParseTraceroute(out)
		res.HopCount = &hops
		res.Diagnosis = DiagHealthy
		if hops == 0 {
			res.Diagnosis = DiagRouteIssue
			res.Status = "partial"
		}
	default:
		res.Status = "blocked"
		res.Diagnosis = DiagDataInsufficient
		res.ErrorCode = "test_unknown"
	}

	res.FinishedAt = time.Now().UTC()
	res.DurationMS = res.FinishedAt.Sub(res.StartedAt).Milliseconds()
	return res
}

func classifyPing(loss float64, avg, jit *float64) Diagnosis {
	if loss >= 100 {
		return DiagUnreachable
	}
	if loss >= 5 {
		return DiagPacketLoss
	}
	if jit != nil && *jit >= 30 {
		return DiagUnstableJitter
	}
	if avg != nil && *avg >= 100 {
		return DiagHighLatency
	}
	if avg == nil {
		return DiagDataInsufficient
	}
	return DiagHealthy
}

func errCode(err error) string {
	switch err {
	case ErrTestDisabled:
		return "test_disabled"
	case ErrTestUnknown:
		return "test_unknown"
	case ErrInvalidTarget:
		return "invalid_target"
	case ErrCountOutOfRange:
		return "count_out_of_range"
	case ErrTimeoutOutOfRange:
		return "timeout_out_of_range"
	case ErrDurationOutOfRange:
		return "duration_out_of_range"
	}
	return "unknown"
}

// sanitize is a tiny defensive helper. A future iteration could share
// the mikrotik/mimosa sanitizer, but the test runner output is much
// simpler so we keep this self-contained.
func sanitize(msg string) string {
	if len(msg) > 240 {
		msg = msg[:240] + "..."
	}
	return fmt.Sprintf("%s", msg)
}
