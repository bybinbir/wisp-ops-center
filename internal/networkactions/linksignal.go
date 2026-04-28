package networkactions

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	wispssh "github.com/wisp-ops-center/wisp-ops-center/internal/adapters/ssh"
)

// LinkSignalTestAction is the Phase 9 v2 read-only action that
// estimates point-to-point/backhaul link health from the
// registration table + interface state. NEVER initiates a
// bandwidth-test; bandwidth-test commands are forbidden by the
// allowlist (segment denylist) so even a code-level mistake cannot
// fire one.
//
// Heuristics:
//   - The action picks the best candidate "link interface" by
//     scoring the wireless interfaces: a bridge / station /
//     ap-bridge wireless mode AND exactly 1-2 registered peers
//     usually means PtP.
//   - When no wireless menu reports a registered peer, we return
//     succeeded with skipped=true and a non-empty reason. No fake
//     link metrics.
type LinkSignalTestAction struct {
	Log        *slog.Logger
	KnownHosts wispssh.KnownHostsStore
	Target     SSHTarget
	NewSession func(SSHTarget, *slog.Logger, wispssh.KnownHostsStore) Dialer
}

// Kind returns the action type.
func (a *LinkSignalTestAction) Kind() Kind { return KindLinkSignalTest }

// Execute runs the read-only link-signal probe pipeline.
func (a *LinkSignalTestAction) Execute(ctx context.Context, req Request) (Result, error) {
	res := Result{
		Kind:          a.Kind(),
		DeviceID:      req.DeviceID,
		CorrelationID: req.CorrelationID,
		StartedAt:     time.Now().UTC(),
		DryRun:        true,
	}
	defer func() { res.FinishedAt = time.Now().UTC() }()

	if err := a.Target.Validate(); err != nil {
		res.ErrorCode = ErrorCode(err)
		res.Message = "link_signal_test: target credentials missing"
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

	lr := LinkSignalTestResult{
		DeviceIdentity: identity,
		HealthStatus:   "unknown",
	}
	commands := []SourceCommand{}

	type menu struct {
		ifaceCmd string
		regCmd   string
		convert  func(string) []WirelessSnapshot
		menuName string
	}
	menus := []menu{
		{"/interface/wireless/print/detail",
			"/interface/wireless/registration-table/print/detail",
			parseWirelessLegacy, "wireless"},
		{"/interface/wifi/print/detail",
			"/interface/wifi/registration-table/print/detail",
			parseWifi, "wifi"},
		{"/interface/wifiwave2/print/detail",
			"/interface/wifiwave2/registration-table/print/detail",
			parseWifiwave2, "wifiwave2"},
	}

	for _, m := range menus {
		ifaceOut, sc, err := runReadOnlyCmd(ctx, asSession(dialer), m.ifaceCmd)
		commands = append(commands, sc)
		if err != nil {
			if errors.Is(err, ErrDisallowedCommand) {
				res.ErrorCode = ErrorCode(err)
				res.Message = "link_signal_test: command blocked"
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

		regOut, regSC, regErr := runReadOnlyCmd(ctx, asSession(dialer), m.regCmd)
		commands = append(commands, regSC)
		var clients []ClientStat
		if regErr == nil && regSC.Status != "skipped_unsupported" && strings.TrimSpace(regOut) != "" {
			clients = extractClients(regOut)
		}
		// Pick the best link candidate: an interface with 1 or 2
		// registered peers; ties broken by bridge/station mode hint.
		picked := pickLinkCandidate(snaps, clients)
		if picked.iface == "" {
			continue
		}
		lr.MenuSource = m.menuName
		lr.LocalInterface = picked.iface
		lr.LinkDetected = picked.peerCount > 0
		if picked.peer != nil {
			lr.RemoteIdentifier = picked.peer.MACPrefix
			lr.Signal = picked.peer.Signal
			lr.TxRateMbps = picked.peer.TxRateMbps
			lr.RxRateMbps = picked.peer.RxRateMbps
			lr.CCQ = picked.peer.CCQ
		}
		break
	}

	lr.HealthStatus, lr.Warnings, lr.Evidence = analyzeLinkSignal(lr)

	if !lr.LinkDetected {
		lr.Skipped = true
		if lr.MenuSource == "" {
			lr.SkippedReason = "no_wireless_menu"
		} else {
			lr.SkippedReason = "no_registered_peer_on_any_interface"
		}
		res.Success = true
		res.Message = "link_signal_test: no link data observed"
	} else {
		res.Success = true
		res.Message = fmt.Sprintf("link_signal_test: link on %s health=%s",
			lr.LocalInterface, lr.HealthStatus)
	}

	res.Result = map[string]any{
		"link_signal_test": lr,
		"commands":         commands,
	}
	return res, nil
}

// pickLinkCandidate scores each interface by peer count + mode.
// Returns the strongest candidate (best signal among peers).
type linkCandidate struct {
	iface     string
	peer      *ClientStat
	peerCount int
}

func pickLinkCandidate(snaps []WirelessSnapshot, clients []ClientStat) linkCandidate {
	if len(snaps) == 0 {
		return linkCandidate{}
	}
	byIface := make(map[string][]ClientStat, len(snaps))
	for _, c := range clients {
		byIface[c.InterfaceName] = append(byIface[c.InterfaceName], c)
	}
	type scored struct {
		c     linkCandidate
		score int
	}
	var ranked []scored
	for _, s := range snaps {
		peers := byIface[s.InterfaceName]
		if len(peers) == 0 {
			continue
		}
		// Strongest peer per iface.
		sort.SliceStable(peers, func(i, j int) bool {
			si, sj := -1<<31, -1<<31
			if peers[i].Signal != nil {
				si = *peers[i].Signal
			}
			if peers[j].Signal != nil {
				sj = *peers[j].Signal
			}
			return si > sj
		})
		strongest := peers[0]
		score := 0
		// Prefer 1-2 peer interfaces (typical PtP).
		switch len(peers) {
		case 1:
			score += 30
		case 2:
			score += 25
		default:
			score += 5
		}
		if s.Mode != "" {
			low := strings.ToLower(s.Mode)
			if strings.Contains(low, "bridge") || strings.Contains(low, "station") {
				score += 15
			}
		}
		if strongest.Signal != nil {
			// Better signal = stronger candidate.
			score += 100 + *strongest.Signal // signal is negative; -50 → 50
		}
		c := linkCandidate{
			iface:     s.InterfaceName,
			peer:      &strongest,
			peerCount: len(peers),
		}
		ranked = append(ranked, scored{c: c, score: score})
	}
	if len(ranked) == 0 {
		return linkCandidate{}
	}
	sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].score > ranked[j].score })
	return ranked[0].c
}

// analyzeLinkSignal grades the link as healthy/warning/critical
// based on the strongest peer's signal + ccq + rate. unknown when
// no link detected.
func analyzeLinkSignal(r LinkSignalTestResult) (string, []string, []string) {
	if !r.LinkDetected {
		return "unknown", nil, nil
	}
	var warns, evidence []string
	status := "healthy"
	if r.Signal != nil {
		evidence = append(evidence, fmt.Sprintf("peer signal %d dBm", *r.Signal))
		switch {
		case *r.Signal < -85:
			warns = append(warns, fmt.Sprintf("peer signal %d dBm is critical", *r.Signal))
			status = "critical"
		case *r.Signal < -75:
			if status != "critical" {
				status = "warning"
			}
			warns = append(warns, fmt.Sprintf("peer signal %d dBm is below ideal", *r.Signal))
		}
	} else {
		status = "unknown"
		warns = append(warns, "peer signal unknown")
	}
	if r.CCQ != nil {
		evidence = append(evidence, fmt.Sprintf("ccq %d%%", *r.CCQ))
		if *r.CCQ < 50 && status != "critical" {
			status = "warning"
			warns = append(warns, fmt.Sprintf("ccq %d%% below 50%%", *r.CCQ))
		}
	}
	if r.TxRateMbps != nil {
		evidence = append(evidence, fmt.Sprintf("tx %d Mbps", *r.TxRateMbps))
	}
	if r.RxRateMbps != nil {
		evidence = append(evidence, fmt.Sprintf("rx %d Mbps", *r.RxRateMbps))
	}
	return status, warns, evidence
}

func (a *LinkSignalTestAction) dialerFor(target SSHTarget) Dialer {
	if a.NewSession != nil {
		return a.NewSession(target, a.Log, a.KnownHosts)
	}
	return NewSSHSession(target, a.Log, a.KnownHosts)
}
