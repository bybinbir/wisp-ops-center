package networkactions

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	wispssh "github.com/wisp-ops-center/wisp-ops-center/internal/adapters/ssh"
)

// APClientTestAction is the Phase 9 v2 read-only action that
// inspects AP-side client health (signal/ccq/rate distribution) by
// reading the wireless registration table only. NEVER writes.
//
// Contract:
//   - dry_run is unconditional.
//   - allowlist guard rejects every mutation token before any
//     command reaches SSH.
//   - if no wireless menu is supported OR the registration table
//     is empty, the action returns succeeded with skipped=true and
//     a non-empty SkippedReason. No fake client data.
//   - per-client MACs are masked to a 5-octet prefix in the result
//     jsonb (see maskMAC) so customer devices stay non-resolvable
//     in audit logs and the API.
type APClientTestAction struct {
	Log        *slog.Logger
	KnownHosts wispssh.KnownHostsStore
	Target     SSHTarget
	NewSession func(SSHTarget, *slog.Logger, wispssh.KnownHostsStore) Dialer

	// WeakSignalThresholdDBm controls "weak client" classification.
	// Defaults to -80 dBm when zero.
	WeakSignalThresholdDBm int
	// LowCCQThresholdPct controls "low ccq client" classification.
	// Defaults to 50% when zero.
	LowCCQThresholdPct int
}

// Kind returns the action type.
func (a *APClientTestAction) Kind() Kind { return KindAPClientTest }

// Execute runs the read-only AP client probe pipeline.
func (a *APClientTestAction) Execute(ctx context.Context, req Request) (Result, error) {
	res := Result{
		Kind:          a.Kind(),
		DeviceID:      req.DeviceID,
		CorrelationID: req.CorrelationID,
		StartedAt:     time.Now().UTC(),
		DryRun:        true, // Phase 9 v2: dry_run always true for read-only
	}
	defer func() { res.FinishedAt = time.Now().UTC() }()

	if err := a.Target.Validate(); err != nil {
		res.ErrorCode = ErrorCode(err)
		res.Message = "ap_client_test: target credentials missing"
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

	idOut, _, err := runReadOnlyCmd(ctx, asSession(dialer), "/system/identity/print")
	if err != nil {
		res.ErrorCode = ErrorCode(err)
		res.Message = SanitizeMessage(err.Error())
		return res, err
	}
	identity := parseIdentityName(idOut)

	apRes := APClientTestResult{DeviceIdentity: identity}
	commands := []SourceCommand{}

	type menu struct {
		label    string
		ifaceCmd string
		regCmd   string
		convert  func(string) []WirelessSnapshot
		menuName string
	}
	menus := []menu{
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

	var observedIfaces []WirelessSnapshot
	var allClients []ClientStat
	chosenMenu := "none"

	for _, m := range menus {
		ifaceOut, sc, err := runReadOnlyCmd(ctx, asSession(dialer), m.ifaceCmd)
		commands = append(commands, sc)
		if err != nil {
			if errors.Is(err, ErrDisallowedCommand) {
				res.ErrorCode = ErrorCode(err)
				res.Message = "ap_client_test: command blocked"
				return res, err
			}
			continue
		}
		if sc.Status == "skipped_unsupported" || strings.TrimSpace(ifaceOut) == "" {
			continue
		}
		snaps := m.convert(ifaceOut)
		if len(snaps) == 0 {
			continue
		}
		observedIfaces = snaps
		chosenMenu = m.menuName

		regOut, regSC, regErr := runReadOnlyCmd(ctx, asSession(dialer), m.regCmd)
		commands = append(commands, regSC)
		if regErr == nil && regSC.Status != "skipped_unsupported" && strings.TrimSpace(regOut) != "" {
			allClients = extractClients(regOut)
		}
		break
	}

	apRes.MenuSource = chosenMenu
	apRes.Interfaces = observedIfaces
	apRes.ClientCount = len(allClients)

	weakThr := a.WeakSignalThresholdDBm
	if weakThr == 0 {
		weakThr = -80
	}
	lowCCQ := a.LowCCQThresholdPct
	if lowCCQ == 0 {
		lowCCQ = 50
	}

	var sumSig, nSig, sumCCQ, nCCQ int
	var worst *int
	for _, c := range allClients {
		if c.Signal != nil {
			sumSig += *c.Signal
			nSig++
			if worst == nil || *c.Signal < *worst {
				v := *c.Signal
				worst = &v
			}
		}
		if c.CCQ != nil {
			sumCCQ += *c.CCQ
			nCCQ++
		}
		if c.Signal != nil && *c.Signal < weakThr {
			cc := c
			cc.Reason = fmt.Sprintf("signal %d dBm < %d", *c.Signal, weakThr)
			apRes.WeakClients = append(apRes.WeakClients, cc)
		}
		if c.CCQ != nil && *c.CCQ < lowCCQ {
			cc := c
			cc.Reason = fmt.Sprintf("ccq %d%% < %d%%", *c.CCQ, lowCCQ)
			apRes.LowCCQClients = append(apRes.LowCCQClients, cc)
		}
	}
	if nSig > 0 {
		v := sumSig / nSig
		apRes.AvgSignal = &v
	}
	if worst != nil {
		apRes.WorstSignal = worst
	}
	if nCCQ > 0 {
		v := sumCCQ / nCCQ
		apRes.AvgCCQ = &v
	}

	apRes.Warnings, apRes.Evidence = analyzeAPClient(apRes)

	if apRes.MenuSource == "none" || apRes.ClientCount == 0 {
		apRes.Skipped = true
		if apRes.MenuSource == "none" {
			apRes.SkippedReason = "no_wireless_menu"
		} else {
			apRes.SkippedReason = "no_registered_clients"
		}
		res.Success = true
		res.Message = "ap_client_test: no client data observed"
	} else {
		res.Success = true
		res.Message = fmt.Sprintf("ap_client_test: %d client(s) observed via %s",
			apRes.ClientCount, apRes.MenuSource)
	}

	res.Result = map[string]any{
		"ap_client_test": apRes,
		"commands":       commands,
	}
	return res, nil
}

func analyzeAPClient(r APClientTestResult) (warnings, evidence []string) {
	if r.AvgSignal != nil {
		evidence = append(evidence, fmt.Sprintf("avg signal %d dBm across %d client(s)",
			*r.AvgSignal, r.ClientCount))
	}
	if len(r.WeakClients) > 0 {
		warnings = append(warnings,
			fmt.Sprintf("%d client(s) below weak-signal threshold", len(r.WeakClients)))
	}
	if len(r.LowCCQClients) > 0 {
		warnings = append(warnings,
			fmt.Sprintf("%d client(s) below low-CCQ threshold", len(r.LowCCQClients)))
	}
	if r.WorstSignal != nil && *r.WorstSignal < -88 {
		warnings = append(warnings, fmt.Sprintf("worst client signal %d dBm is critical", *r.WorstSignal))
	}
	return warnings, evidence
}

func (a *APClientTestAction) dialerFor(target SSHTarget) Dialer {
	if a.NewSession != nil {
		return a.NewSession(target, a.Log, a.KnownHosts)
	}
	return NewSSHSession(target, a.Log, a.KnownHosts)
}
