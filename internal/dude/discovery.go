package dude

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

// RunResult captures everything the orchestrator learned from a
// discovery pass.
type RunResult struct {
	CorrelationID string
	StartedAt     time.Time
	FinishedAt    time.Time
	Success       bool
	ErrorCode     string
	Error         string
	Devices       []DiscoveredDevice
	Stats         DiscoveryStats
	CommandsRun   []string
}

// Run dials the Dude SSH endpoint, runs the read-only discovery
// commands, parses their output and classifies each host.
//
// IMPORTANT (Phase 8 hotfix v8.4.0): the return value is a NAMED
// return so the deferred FinishedAt + Stats.Tally mutations are
// observed by the caller. With a non-named return, `return res`
// copies the result into the caller's slot before the deferred
// closure runs, and Stats stays zero-valued (caused
// discovery_runs.device_count=0 even when 893 devices were upserted).
func Run(ctx context.Context, cfg Config, log *slog.Logger, store wispssh.KnownHostsStore) (res RunResult) {
	res = RunResult{
		CorrelationID: NewCorrelationID(),
		StartedAt:     time.Now().UTC(),
	}
	defer func() {
		res.FinishedAt = time.Now().UTC()
		res.Stats = DiscoveryStats{}
		res.Stats.Tally(res.Devices)
	}()

	if err := cfg.Validate(); err != nil {
		res.Error = "Dude configuration incomplete"
		res.ErrorCode = ErrorCode(err)
		return res
	}

	c := NewClient(cfg, log, store)
	c.SetCorrelationID(res.CorrelationID)
	if err := c.Dial(ctx); err != nil {
		res.Error = SanitizeMessage(err.Error())
		res.ErrorCode = ErrorCode(err)
		return res
	}
	defer c.Close()

	out, err := c.Exec(ctx, "/dude/device/print/detail")
	res.CommandsRun = append(res.CommandsRun, "/dude/device/print/detail")
	if err == nil {
		res.Devices = append(res.Devices, devicesFromDudePrint(out, res.StartedAt)...)
	} else {
		res.Error = SanitizeMessage(err.Error())
		res.ErrorCode = ErrorCode(err)
		neighbors, nerr := c.Exec(ctx, "/ip/neighbor/print/detail")
		res.CommandsRun = append(res.CommandsRun, "/ip/neighbor/print/detail")
		if nerr == nil {
			res.Devices = append(res.Devices, devicesFromNeighborPrint(neighbors, res.StartedAt)...)
			res.Error = ""
			res.ErrorCode = ""
		} else if !errors.Is(nerr, ErrDisallowedCommand) {
			combined := fmt.Sprintf("dude_print=%s; neighbor_print=%s",
				SanitizeMessage(err.Error()), SanitizeMessage(nerr.Error()))
			res.Error = combined
		}
	}

	if id, ierr := c.Exec(ctx, "/system/identity/print"); ierr == nil {
		identity := parseIdentityName(id)
		if identity != "" {
			self := DiscoveredDevice{
				Source:   "mikrotik_dude",
				Name:     identity,
				IP:       cfg.Host,
				Identity: identity,
				Type:     "router",
				LastSeen: res.StartedAt,
				Raw:      map[string]string{"role": "dude_host"},
			}
			Classify(&self)
			res.Devices = append(res.Devices, self)
		}
		res.CommandsRun = append(res.CommandsRun, "/system/identity/print")
	}

	for i := range res.Devices {
		if res.Devices[i].Classification.Category == "" {
			Classify(&res.Devices[i])
		}
	}

	res.Devices = dedupeDevices(res.Devices)

	if res.Error == "" {
		res.Success = true
	}
	return res
}

func devicesFromDudePrint(out string, ts time.Time) []DiscoveredDevice {
	records := ParseDetailPrint(out)
	devs := make([]DiscoveredDevice, 0, len(records))
	for _, r := range records {
		raw := SanitizeAttrs(r)
		d := DiscoveredDevice{
			Source:   "mikrotik_dude",
			Name:     pickFirst(r, "name"),
			IP:       pickFirst(r, "address", "ip-address"),
			MAC:      strings.ToUpper(pickFirst(r, "mac-address", "mac")),
			Model:    pickFirst(r, "model", "board"),
			Type:     pickFirst(r, "type"),
			Status:   pickFirst(r, "status", "state"),
			Identity: pickFirst(r, "identity"),
			Raw:      raw,
			LastSeen: ts,
		}
		Classify(&d)
		devs = append(devs, d)
	}
	return devs
}

func devicesFromNeighborPrint(out string, ts time.Time) []DiscoveredDevice {
	records := ParseDetailPrint(out)
	devs := make([]DiscoveredDevice, 0, len(records))
	for _, r := range records {
		raw := SanitizeAttrs(r)
		d := DiscoveredDevice{
			Source:    "mikrotik_dude",
			Name:      pickFirst(r, "identity", "system-caps"),
			IP:        pickFirst(r, "address4", "address"),
			MAC:       strings.ToUpper(pickFirst(r, "mac-address")),
			Model:     pickFirst(r, "board", "platform"),
			OSVersion: pickFirst(r, "version", "software-id"),
			Identity:  pickFirst(r, "identity"),
			Type:      pickFirst(r, "system-caps"),
			Raw:       raw,
			LastSeen:  ts,
		}
		Classify(&d)
		devs = append(devs, d)
	}
	return devs
}

func pickFirst(r map[string]string, keys ...string) string {
	for _, k := range keys {
		if v, ok := r[k]; ok && v != "" {
			return v
		}
	}
	return ""
}

func dedupeDevices(in []DiscoveredDevice) []DiscoveredDevice {
	seen := map[string]int{}
	var out []DiscoveredDevice
	for _, d := range in {
		key := dedupeKey(d)
		if key == "" {
			out = append(out, d)
			continue
		}
		if i, ok := seen[key]; ok {
			out[i] = mergeDevices(out[i], d)
			continue
		}
		seen[key] = len(out)
		out = append(out, d)
	}
	return out
}

func dedupeKey(d DiscoveredDevice) string {
	switch {
	case d.MAC != "":
		return "mac:" + d.MAC
	case d.IP != "":
		return "ip:" + d.IP
	case d.Name != "":
		return "name:" + strings.ToLower(d.Name)
	}
	return ""
}

func mergeDevices(into, from DiscoveredDevice) DiscoveredDevice {
	if into.Name == "" {
		into.Name = from.Name
	}
	if into.IP == "" {
		into.IP = from.IP
	}
	if into.MAC == "" {
		into.MAC = from.MAC
	}
	if into.Model == "" {
		into.Model = from.Model
	}
	if into.OSVersion == "" {
		into.OSVersion = from.OSVersion
	}
	if from.Classification.Confidence > into.Classification.Confidence {
		into.Classification = from.Classification
	}
	return into
}

// NewCorrelationID generates a short hex token for end-to-end log
// correlation across the SSH client, parser, classifier and repository.
func NewCorrelationID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("dude-%d", time.Now().UnixNano())
	}
	return "dude-" + hex.EncodeToString(b[:])
}
