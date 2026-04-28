package networkactions

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	wispssh "github.com/wisp-ops-center/wisp-ops-center/internal/adapters/ssh"
)

// FrequencyCheckAction is the Phase 9 read-only action: open an SSH
// session against a RouterOS target, run an allowlisted set of
// /interface/wireless | /interface/wifi | /interface/wifiwave2
// print/detail commands, and produce a structured snapshot of the
// device's current radio configuration + per-interface client stats.
//
// IMPORTANT contract:
//   - NEVER sends a mutation command. EnsureCommandAllowed gates
//     every Exec; tests prove this.
//   - When a device has no wireless interfaces (e.g. a CCR core
//     router, a switch), the action returns SUCCEEDED with the
//     skipped flag set + a non-empty SkippedReason. No fake data.
//   - Returns a Result whose Confidence reflects the strength of the
//     evidence collected, not just "did the SSH connection succeed".
type FrequencyCheckAction struct {
	Log        *slog.Logger
	KnownHosts wispssh.KnownHostsStore
	// Target supplies the SSH credentials + host key policy. The
	// runner injects this from the live config; tests inject a stub
	// SSHTarget that points at a test server.
	Target SSHTarget
	// Optional override of the SSH session factory, useful for
	// hermetic tests that don't actually dial.
	NewSession func(SSHTarget, *slog.Logger, wispssh.KnownHostsStore) Dialer
}

// Dialer is the minimal interface this action needs from the SSH
// transport. SSHSession satisfies it; tests provide a fake.
type Dialer interface {
	Dial(ctx context.Context) error
	Exec(ctx context.Context, cmd string) (string, error)
	Close()
	SetCorrelationID(id string)
}

// Kind returns the action type.
func (a *FrequencyCheckAction) Kind() Kind { return KindFrequencyCheck }

// Execute runs the read-only frequency check pipeline. The returned
// Result carries the structured findings under .Message/.Result and
// signals success via Result.Success. Errors that mean "the device
// does not have wireless data" do NOT collapse into Result.Success
// = false — they produce a "skipped" outcome.
//
// Status semantics:
//
//	succeeded  → wireless data observed AND classified
//	skipped    → device probed OK but no wireless menu / interfaces
//	failed     → SSH dial / auth / timeout / parse / disallowed_command
func (a *FrequencyCheckAction) Execute(ctx context.Context, req Request) (Result, error) {
	res := Result{
		Kind:          a.Kind(),
		DeviceID:      req.DeviceID,
		CorrelationID: req.CorrelationID,
		StartedAt:     time.Now().UTC(),
		DryRun:        req.DryRun,
	}
	defer func() { res.FinishedAt = time.Now().UTC() }()

	if a.Target.Host == "" {
		res.ErrorCode = "not_configured"
		res.Message = "frequency_check: target host is empty"
		return res, ErrNotConfigured
	}
	if err := a.Target.Validate(); err != nil {
		res.ErrorCode = ErrorCode(err)
		res.Message = "frequency_check: target credentials missing"
		return res, err
	}

	dialer := a.dialerFor(a.Target)
	if req.CorrelationID != "" {
		dialer.SetCorrelationID(req.CorrelationID)
	}
	if err := dialer.Dial(ctx); err != nil {
		res.ErrorCode = ErrorCode(err)
		res.Message = SanitizeMessage(err.Error())
		return res, err
	}
	defer dialer.Close()

	out, _, err := runReadOnlyCmd(ctx, asSession(dialer), "/system/identity/print")
	if err != nil {
		res.ErrorCode = ErrorCode(err)
		res.Message = SanitizeMessage(err.Error())
		return res, err
	}
	identity := parseIdentityName(out)

	freqRes := FrequencyCheckResult{DeviceIdentity: identity}
	commands := []SourceCommand{}

	// Phase 9 v1 attempts three menus. Each tries print/detail; an
	// unsupported menu yields a skipped_unsupported entry, NEVER a
	// run-level failure.
	menus := []struct {
		label    string
		cmd      string
		regCmd   string
		convert  func(string) []WirelessSnapshot
		menuName string
	}{
		{"wireless_legacy", "/interface/wireless/print/detail",
			"/interface/wireless/registration-table/print/detail",
			parseWirelessLegacy, "wireless"},
		{"wifi", "/interface/wifi/print/detail",
			"/interface/wifi/registration-table/print/detail",
			parseWifi, "wifi"},
		{"wifiwave2", "/interface/wifiwave2/print/detail",
			"/interface/wifiwave2/registration-table/print/detail",
			parseWifiwave2, "wifiwave2"},
	}

	var observed []WirelessSnapshot
	chosenMenu := "none"
	for _, m := range menus {
		out, sc, err := runReadOnlyCmd(ctx, asSession(dialer), m.cmd)
		commands = append(commands, sc)
		if err != nil {
			// Per-source failure does not fail the run unless it's
			// a disallowed_command, which is structural.
			if errors.Is(err, ErrDisallowedCommand) {
				res.ErrorCode = ErrorCode(err)
				res.Message = "frequency_check: command blocked"
				return res, err
			}
			continue
		}
		if sc.Status == "skipped_unsupported" || strings.TrimSpace(out) == "" {
			continue
		}
		snaps := m.convert(out)
		if len(snaps) == 0 {
			continue
		}
		observed = append(observed, snaps...)
		chosenMenu = m.menuName

		// Pull registration data for the same menu — best-effort.
		regOut, regSC, regErr := runReadOnlyCmd(ctx, asSession(dialer), m.regCmd)
		commands = append(commands, regSC)
		if regErr == nil && regSC.Status != "skipped_unsupported" && strings.TrimSpace(regOut) != "" {
			regs := summarizeRegistration(regOut)
			for i := range observed {
				if r, ok := regs[observed[i].InterfaceName]; ok {
					observed[i].ClientCount = r.count
					observed[i].AvgSignal = r.avgSignal
					observed[i].WorstSignal = r.worstSignal
					observed[i].AvgCCQ = r.avgCCQ
					observed[i].TxRateMbps = r.avgTxMbps
					observed[i].RxRateMbps = r.avgRxMbps
					observed[i].RegistrationOK = true
				}
			}
		}
		break
	}

	freqRes.MenuSource = chosenMenu
	freqRes.Interfaces = observed
	freqRes.Warnings, freqRes.Evidence = analyzeWirelessSnapshots(observed)

	// No wireless interfaces of any kind → succeeded but skipped.
	if len(observed) == 0 {
		freqRes.Skipped = true
		freqRes.SkippedReason = "no_wireless_menu_or_no_interface"
		res.Success = true
		res.Message = "no wireless data on this device"
	} else {
		res.Success = true
		res.Message = fmt.Sprintf("frequency_check observed %d wireless interface(s) via %s",
			len(observed), chosenMenu)
	}

	res.Result = map[string]any{
		"frequency_check": freqRes,
		"commands":        commands,
	}
	return res, nil
}

// analyzeWirelessSnapshots produces the per-run warnings + evidence
// list the audit/UI surfaces. Examples:
//
//   - "AP-Sahil-1 has 0 connected clients"      (warning)
//   - "Frequency 5180 MHz, channel-width 20MHz" (evidence)
//
// Confidence and any future scoring decision build on top of this.
func analyzeWirelessSnapshots(snaps []WirelessSnapshot) (warnings, evidence []string) {
	for _, s := range snaps {
		if s.Disabled != nil && *s.Disabled {
			warnings = append(warnings, fmt.Sprintf("%s is disabled", s.InterfaceName))
		}
		if s.Running != nil && !*s.Running {
			warnings = append(warnings, fmt.Sprintf("%s is configured but not running", s.InterfaceName))
		}
		if s.RegistrationOK && s.ClientCount == 0 {
			warnings = append(warnings, fmt.Sprintf("%s has 0 connected clients", s.InterfaceName))
		}
		if s.AvgSignal != nil && *s.AvgSignal < -80 {
			warnings = append(warnings,
				fmt.Sprintf("%s avg signal %d dBm is poor", s.InterfaceName, *s.AvgSignal))
		}
		if s.WorstSignal != nil && *s.WorstSignal < -88 {
			warnings = append(warnings,
				fmt.Sprintf("%s worst client signal %d dBm is critical", s.InterfaceName, *s.WorstSignal))
		}
		if s.AvgCCQ != nil && *s.AvgCCQ < 50 {
			warnings = append(warnings,
				fmt.Sprintf("%s avg CCQ %d%% is below 50%%", s.InterfaceName, *s.AvgCCQ))
		}
		if s.Frequency != "" {
			evidence = append(evidence, fmt.Sprintf("%s frequency=%s width=%s mode=%s",
				s.InterfaceName, s.Frequency, s.ChannelWidth, s.Mode))
		}
	}
	return warnings, evidence
}

// dialerFor returns either the configured factory or the default
// SSHSession-backed dialer.
func (a *FrequencyCheckAction) dialerFor(target SSHTarget) Dialer {
	if a.NewSession != nil {
		return a.NewSession(target, a.Log, a.KnownHosts)
	}
	return NewSSHSession(target, a.Log, a.KnownHosts)
}

// asSession is a helper for runReadOnlyCmd which is typed for
// *SSHSession but accepts any Dialer for tests.
func asSession(d Dialer) *sessionShim {
	return &sessionShim{d: d}
}

// sessionShim adapts Dialer to the small surface runReadOnlyCmd
// uses. Without this, runReadOnlyCmd is forced to depend on the
// concrete *SSHSession type.
type sessionShim struct{ d Dialer }

func (s *sessionShim) Exec(ctx context.Context, cmd string) (string, error) {
	return s.d.Exec(ctx, cmd)
}

// parseIdentityName extracts "name: <value>" from /system/identity/print.
func parseIdentityName(out string) string {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(line), "name:") {
			return strings.TrimSpace(line[5:])
		}
	}
	return ""
}

// NewCorrelationID issues a short correlation token used to thread
// log entries from API → action runner → SSH session → DB.
func NewCorrelationID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("act-%d", time.Now().UnixNano())
	}
	return "act-" + hex.EncodeToString(b[:])
}
