package devicectl

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/wisp-ops-center/wisp-ops-center/internal/adapters/mimosa"
	"github.com/wisp-ops-center/wisp-ops-center/internal/audit"
	"github.com/wisp-ops-center/wisp-ops-center/internal/telemetry"
)

// ProbeMimosa executes a Mimosa SNMP read-only probe.
func (s *Service) ProbeMimosa(ctx context.Context, deviceID, actor string) (*mimosa.MimosaProbeResult, error) {
	dev, err := s.loadDevice(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(dev.vendor, "mimosa") {
		return nil, ErrVendorUnsupported
	}
	cred, err := s.loadCredential(ctx, dev.id)
	if err != nil {
		return nil, err
	}
	cfg, err := mimosaCfg(dev, cred, s)
	if err != nil {
		return nil, err
	}

	start := time.Now().UTC()
	result, caps, perr := mimosa.Probe(ctx, *cfg)
	fin := time.Now().UTC()

	status := telemetry.StatusSuccess
	errMsg := ""
	if perr != nil {
		status = telemetry.StatusFailed
		errMsg = mimosa.SanitizeError(perr)
	} else if result.Partial {
		status = telemetry.StatusPartial
	}

	pollID, _ := s.Tel.RecordPollResult(ctx, telemetry.PollResultInput{
		DeviceID:     dev.id,
		Vendor:       dev.vendor,
		Operation:    telemetry.OpProbe,
		Transport:    string(cfg.Transport),
		Status:       status,
		StartedAt:    start,
		FinishedAt:   fin,
		ErrorCode:    classifyMimosaErrCode(perr),
		ErrorMessage: errMsg,
		Summary: map[string]any{
			"reachable":         result.Reachable,
			"system_name":       result.SystemName,
			"vendor_mib_status": result.VendorMIBStatus,
			"partial":           result.Partial,
			"model":             result.Model,
			"firmware":          result.Firmware,
		},
	})
	_ = pollID

	if perr == nil {
		_ = s.Tel.CapabilitiesUpsertMimosa(ctx, dev.id, caps)
	}
	if s.Audit != nil {
		_ = s.Audit.Write(ctx, audit.Entry{
			Actor:   actor,
			Action:  audit.ActionScheduledCheckRan,
			Subject: "device:" + dev.id,
			Outcome: outcomeFor(status),
			Reason:  errMsg,
			Metadata: map[string]any{
				"operation":         "probe",
				"transport":         string(cfg.Transport),
				"vendor":            "mimosa",
				"vendor_mib_status": result.VendorMIBStatus,
			},
		})
	}
	if perr != nil {
		return result, perr
	}
	return result, nil
}

// PollMimosa runs a read-only SNMP poll and persists the snapshot.
func (s *Service) PollMimosa(ctx context.Context, deviceID, actor string) (*mimosa.MimosaReadOnlySnapshot, error) {
	dev, err := s.loadDevice(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(dev.vendor, "mimosa") {
		return nil, ErrVendorUnsupported
	}
	cred, err := s.loadCredential(ctx, dev.id)
	if err != nil {
		return nil, err
	}
	cfg, err := mimosaCfg(dev, cred, s)
	if err != nil {
		return nil, err
	}

	snap, caps, perr := mimosa.Poll(ctx, *cfg)

	status := telemetry.StatusSuccess
	errMsg := ""
	if perr != nil {
		status = telemetry.StatusFailed
		errMsg = mimosa.SanitizeError(perr)
	} else if snap.Partial || len(snap.Errors) > 0 {
		status = telemetry.StatusPartial
		if len(snap.Errors) > 0 {
			errMsg = "partial: " + strings.Join(snap.Errors, "; ")
		}
	}

	pollID, _ := s.Tel.RecordPollResult(ctx, telemetry.PollResultInput{
		DeviceID:     dev.id,
		Vendor:       dev.vendor,
		Operation:    telemetry.OpPoll,
		Transport:    string(cfg.Transport),
		Status:       status,
		StartedAt:    snap.StartedAt,
		FinishedAt:   snap.FinishedAt,
		ErrorCode:    classifyMimosaErrCode(perr),
		ErrorMessage: errMsg,
		Summary: map[string]any{
			"interfaces":        len(snap.Interfaces),
			"radios":            len(snap.Radios),
			"links":             len(snap.Links),
			"clients":           len(snap.Clients),
			"vendor_mib_status": snap.VendorMIBStatus,
			"partial":           snap.Partial,
		},
	})

	if perr == nil && pollID > 0 {
		if perr2 := s.Tel.PersistMimosaSnapshot(ctx, pollID, snap); perr2 != nil {
			snap.Errors = append(snap.Errors, "persist:"+mimosa.SanitizeError(perr2))
		}
		_ = s.Tel.CapabilitiesUpsertMimosa(ctx, dev.id, caps)
	}

	if s.Audit != nil {
		_ = s.Audit.Write(ctx, audit.Entry{
			Actor:   actor,
			Action:  audit.ActionScheduledCheckRan,
			Subject: "device:" + dev.id,
			Outcome: outcomeFor(status),
			Reason:  errMsg,
			Metadata: map[string]any{
				"operation":         "poll",
				"transport":         string(cfg.Transport),
				"vendor":            "mimosa",
				"clients":           len(snap.Clients),
				"vendor_mib_status": snap.VendorMIBStatus,
			},
		})
	}

	if perr != nil {
		return snap, perr
	}
	return snap, nil
}

func mimosaCfg(dev *deviceLookup, cred *credentialLookup, s *Service) (*mimosa.Config, error) {
	if s.Vault == nil {
		return nil, mimosa.ErrVaultNotConfigured
	}
	cfg := &mimosa.Config{
		DeviceID:   dev.id,
		Host:       dev.ip,
		Transport:  mimosa.TransportSNMP,
		TimeoutSec: 6,
	}
	if cred.port != nil {
		cfg.Port = *cred.port
	}
	switch cred.authType {
	case "snmp_v2", "mimosa_snmp":
		cfg.SNMPVersion = mimosa.SNMPv2c
		secret, err := s.Vault.Decrypt(cred.cipherText)
		if err != nil {
			return nil, mimosa.ErrVaultNotConfigured
		}
		cfg.Community = secret
	case "snmp_v3":
		cfg.SNMPVersion = mimosa.SNMPv3
		// Phase 5: load USM fields from the credential profile and
		// decrypt auth/priv passphrases via the vault.
		usm, err := s.loadSNMPv3USM(context.Background(), cred.profileID)
		if err != nil {
			return cfg, err
		}
		cfg.V3Username = usm.Username
		cfg.V3SecurityLevel = mimosa.SNMPv3SecurityLevel(usm.Level)
		cfg.V3AuthProtocol = mimosa.SNMPv3AuthProtocol(usm.AuthProto)
		cfg.V3AuthSecret = usm.AuthSecret
		cfg.V3PrivProtocol = mimosa.SNMPv3PrivProtocol(usm.PrivProto)
		cfg.V3PrivSecret = usm.PrivSecret
	default:
		return nil, mimosa.ErrTransportUnsupported
	}
	return cfg, nil
}

func classifyMimosaErrCode(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, mimosa.ErrTimeout):
		return "timeout"
	case errors.Is(err, mimosa.ErrAuth):
		return "auth"
	case errors.Is(err, mimosa.ErrUnreachable):
		return "unreachable"
	case errors.Is(err, mimosa.ErrParse):
		return "parse"
	case errors.Is(err, mimosa.ErrCredentialMissing):
		return "credential_missing"
	case errors.Is(err, mimosa.ErrVaultNotConfigured):
		return "vault_not_configured"
	case errors.Is(err, mimosa.ErrSNMPv3Misconfigured):
		return "snmpv3_misconfigured"
	}
	return "unknown"
}
