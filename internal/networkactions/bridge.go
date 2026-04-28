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

// BridgeHealthCheckAction is the Phase 9 v2 read-only action that
// reports bridge + bridge-port state. NEVER modifies topology.
//
// Allowed sources:
//   - /interface/bridge/print/detail
//   - /interface/bridge/port/print/detail
//   - /interface/print/detail (cross-reference for running state)
//
// Explicitly NOT allowed (out of allowlist):
//   - /interface/bridge/host/print  (would expose attached MACs)
//   - any /set, /add, /remove path
type BridgeHealthCheckAction struct {
	Log        *slog.Logger
	KnownHosts wispssh.KnownHostsStore
	Target     SSHTarget
	NewSession func(SSHTarget, *slog.Logger, wispssh.KnownHostsStore) Dialer
}

// Kind returns the action type.
func (a *BridgeHealthCheckAction) Kind() Kind { return KindBridgeHealthCheck }

// Execute runs the read-only bridge health pipeline.
func (a *BridgeHealthCheckAction) Execute(ctx context.Context, req Request) (Result, error) {
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
		res.Message = "bridge_health_check: target credentials missing"
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

	br := BridgeHealthResult{DeviceIdentity: identity}
	commands := []SourceCommand{}

	// 1. Bridge list.
	bridgeOut, sc1, err := runReadOnlyCmd(ctx, asSession(dialer), "/interface/bridge/print/detail")
	commands = append(commands, sc1)
	if err != nil {
		if errors.Is(err, ErrDisallowedCommand) {
			res.ErrorCode = ErrorCode(err)
			res.Message = "bridge_health_check: command blocked"
			return res, err
		}
	}
	bridges := parseBridgeList(bridgeOut)

	// 2. Bridge port list.
	portsOut, sc2, perr := runReadOnlyCmd(ctx, asSession(dialer), "/interface/bridge/port/print/detail")
	commands = append(commands, sc2)
	if perr != nil && errors.Is(perr, ErrDisallowedCommand) {
		res.ErrorCode = ErrorCode(perr)
		res.Message = "bridge_health_check: command blocked"
		return res, perr
	}
	ports := parseBridgePorts(portsOut)

	// 3. Cross-reference interface running/disabled.
	ifaceOut, sc3, ierr := runReadOnlyCmd(ctx, asSession(dialer), "/interface/print/detail")
	commands = append(commands, sc3)
	ifaceState := map[string]struct {
		running  *bool
		disabled *bool
	}{}
	if ierr == nil && sc3.Status != "skipped_unsupported" && strings.TrimSpace(ifaceOut) != "" {
		records := parseDetailPrint(ifaceOut)
		for _, r := range records {
			name := pickFirst(r, "name")
			if name == "" {
				continue
			}
			s := struct {
				running  *bool
				disabled *bool
			}{}
			if v, ok := r["running"]; ok {
				b := isTrue(v)
				s.running = &b
			}
			if v, ok := r["disabled"]; ok {
				b := isTrue(v)
				s.disabled = &b
			}
			ifaceState[name] = s
		}
	}

	// Annotate ports with running/disabled from /interface/print/detail.
	portsByBridge := map[string]int{}
	for i := range ports {
		st, ok := ifaceState[ports[i].InterfaceName]
		if ok {
			ports[i].Running = st.running
			ports[i].Disabled = st.disabled
		}
		if ports[i].Bridge != "" {
			portsByBridge[ports[i].Bridge]++
		}
	}
	for i := range bridges {
		bridges[i].PortCount = portsByBridge[bridges[i].Name]
	}

	// Tally health.
	for _, p := range ports {
		if p.Disabled != nil && *p.Disabled {
			br.DisabledPorts = append(br.DisabledPorts, p)
		}
		if p.Running != nil && !*p.Running && (p.Disabled == nil || !*p.Disabled) {
			br.DownPorts = append(br.DownPorts, p)
		}
	}
	br.Bridges = bridges
	br.BridgeCount = len(bridges)
	br.BridgePortsCount = len(ports)

	br.Warnings, br.Evidence = analyzeBridge(br)

	if br.BridgeCount == 0 && br.BridgePortsCount == 0 {
		br.Skipped = true
		br.SkippedReason = "no_bridge_configured"
		res.Success = true
		res.Message = "bridge_health_check: no bridge configured on this device"
	} else {
		br.RunningSummary = fmt.Sprintf("bridges=%d ports=%d down=%d disabled=%d",
			br.BridgeCount, br.BridgePortsCount, len(br.DownPorts), len(br.DisabledPorts))
		res.Success = true
		res.Message = "bridge_health_check: " + br.RunningSummary
	}

	res.Result = map[string]any{
		"bridge_health_check": br,
		"commands":            commands,
	}
	return res, nil
}

// analyzeBridge produces warnings + evidence from the tally.
func analyzeBridge(r BridgeHealthResult) (warnings, evidence []string) {
	if r.BridgeCount == 0 && r.BridgePortsCount == 0 {
		return nil, nil
	}
	evidence = append(evidence, fmt.Sprintf("bridges=%d ports=%d", r.BridgeCount, r.BridgePortsCount))
	if len(r.DownPorts) > 0 {
		warnings = append(warnings, fmt.Sprintf("%d bridge port(s) are down (not running)", len(r.DownPorts)))
	}
	if len(r.DisabledPorts) > 0 {
		warnings = append(warnings, fmt.Sprintf("%d bridge port(s) are administratively disabled", len(r.DisabledPorts)))
	}
	for _, b := range r.Bridges {
		if b.Disabled != nil && *b.Disabled {
			warnings = append(warnings, fmt.Sprintf("bridge %q is disabled", b.Name))
		}
	}
	return warnings, evidence
}

func (a *BridgeHealthCheckAction) dialerFor(target SSHTarget) Dialer {
	if a.NewSession != nil {
		return a.NewSession(target, a.Log, a.KnownHosts)
	}
	return NewSSHSession(target, a.Log, a.KnownHosts)
}
