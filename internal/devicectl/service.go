// Package devicectl orchestrates probe/poll workflows for both
// MikroTik and Mimosa vendors. It loads the device, fetches the
// preferred credential profile, decrypts the secret, dispatches to
// the right adapter, and persists the result.
package devicectl

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wisp-ops-center/wisp-ops-center/internal/adapters/mikrotik"
	"github.com/wisp-ops-center/wisp-ops-center/internal/audit"
	"github.com/wisp-ops-center/wisp-ops-center/internal/credentials"
	"github.com/wisp-ops-center/wisp-ops-center/internal/telemetry"
)

// ErrVendorUnsupported is returned when the vendor wiring is missing.
var ErrVendorUnsupported = errors.New("devicectl: vendor not supported")

// ErrDeviceNotFound is returned when the lookup fails.
var ErrDeviceNotFound = errors.New("devicectl: device not found")

// Service is the use-case level orchestrator.
type Service struct {
	P     *pgxpool.Pool
	Vault credentials.Vault
	Tel   *telemetry.Repository
	Audit audit.Sink
}

// NewService wires the orchestrator.
func NewService(p *pgxpool.Pool, v credentials.Vault, tel *telemetry.Repository, a audit.Sink) *Service {
	return &Service{P: p, Vault: v, Tel: tel, Audit: a}
}

type deviceLookup struct {
	id     string
	name   string
	vendor string
	role   string
	ip     string
}

func (s *Service) loadDevice(ctx context.Context, id string) (*deviceLookup, error) {
	var d deviceLookup
	var ip *string
	err := s.P.QueryRow(ctx, `
SELECT id, name, vendor, role, host(ip)
  FROM devices WHERE id = $1 AND deleted_at IS NULL`, id).
		Scan(&d.id, &d.name, &d.vendor, &d.role, &ip)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrDeviceNotFound
	}
	if err != nil {
		return nil, err
	}
	if ip != nil {
		d.ip = *ip
	}
	return &d, nil
}

type credentialLookup struct {
	authType              string
	username              string
	port                  *int
	cipherText            []byte
	profileID             string
	tlsVerify             bool
	sshHostKeyPolicy      string
	sshHostKeyFingerprint string
}

// loadCredential picks the highest-priority enabled binding. Phase 4:
// the lookup orders by (priority asc, transport preference). MikroTik
// devices prefer api-ssl > ssh > snmp; Mimosa devices prefer
// snmp > vendor-api.
func (s *Service) loadCredential(ctx context.Context, deviceID string) (*credentialLookup, error) {
	row := s.P.QueryRow(ctx, `
SELECT cp.auth_type, COALESCE(cp.username,''), cp.port, cp.secret_ciphertext, cp.id::text,
       COALESCE(cp.tls_verify, FALSE),
       COALESCE(cp.ssh_host_key_policy,'insecure_ignore'),
       COALESCE(cp.ssh_host_key_fingerprint,'')
  FROM device_credentials dc
  JOIN credential_profiles cp ON cp.id = dc.profile_id
 WHERE dc.device_id = $1 AND dc.enabled = TRUE
 ORDER BY dc.priority ASC,
          CASE dc.transport
            WHEN 'api-ssl' THEN 1
            WHEN 'ssh'     THEN 2
            WHEN 'snmp'    THEN 3
            ELSE 4
          END
 LIMIT 1`, deviceID)
	var c credentialLookup
	err := row.Scan(&c.authType, &c.username, &c.port, &c.cipherText, &c.profileID,
		&c.tlsVerify, &c.sshHostKeyPolicy, &c.sshHostKeyFingerprint)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, mikrotik.ErrCredentialMissing
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func transportFor(authType string) (mikrotik.Transport, error) {
	switch authType {
	case "routeros_api_ssl":
		return mikrotik.TransportAPISSL, nil
	case "ssh":
		return mikrotik.TransportSSH, nil
	case "snmp_v2", "snmp_v3":
		return mikrotik.TransportSNMP, nil
	}
	return "", mikrotik.ErrTransportUnsupported
}

// Probe dispatches by vendor.
func (s *Service) Probe(ctx context.Context, deviceID, actor string) (any, error) {
	dev, err := s.loadDevice(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(dev.vendor) {
	case "mikrotik":
		return s.probeMikroTik(ctx, dev, actor)
	case "mimosa":
		return s.ProbeMimosa(ctx, deviceID, actor)
	}
	return nil, fmt.Errorf("%w: vendor=%s", ErrVendorUnsupported, dev.vendor)
}

// Poll dispatches by vendor.
func (s *Service) Poll(ctx context.Context, deviceID, actor string) (any, error) {
	dev, err := s.loadDevice(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(dev.vendor) {
	case "mikrotik":
		return s.pollMikroTik(ctx, dev, actor)
	case "mimosa":
		return s.PollMimosa(ctx, deviceID, actor)
	}
	return nil, fmt.Errorf("%w: vendor=%s", ErrVendorUnsupported, dev.vendor)
}

// probeMikroTik runs the original Phase 3 MikroTik probe path.
func (s *Service) probeMikroTik(ctx context.Context, dev *deviceLookup, actor string) (*mikrotik.MikroTikProbeResult, error) {
	cred, err := s.loadCredential(ctx, dev.id)
	if err != nil {
		return nil, err
	}
	transport, err := transportFor(cred.authType)
	if err != nil {
		return nil, err
	}
	if s.Vault == nil {
		return nil, mikrotik.ErrVaultNotConfigured
	}
	secret, err := s.Vault.Decrypt(cred.cipherText)
	if err != nil {
		return nil, mikrotik.ErrVaultNotConfigured
	}
	cfg := mikrotik.Config{
		DeviceID:              dev.id,
		Host:                  dev.ip,
		Username:              cred.username,
		Transport:             transport,
		SNMPCommunity:         secret,
		TimeoutSec:            8,
		VerifyTLS:             cred.tlsVerify,
		SSHHostKeyPolicy:      cred.sshHostKeyPolicy,
		SSHHostKeyFingerprint: cred.sshHostKeyFingerprint,
	}
	if cred.port != nil {
		cfg.Port = *cred.port
	}

	start := time.Now().UTC()
	result, caps, perr := mikrotik.Probe(ctx, cfg, secret)
	fin := time.Now().UTC()

	status := telemetry.StatusSuccess
	errMsg := ""
	if perr != nil {
		status = telemetry.StatusFailed
		errMsg = mikrotik.SanitizeError(perr)
	}

	pollID, _ := s.Tel.RecordPollResult(ctx, telemetry.PollResultInput{
		DeviceID: dev.id, Vendor: dev.vendor, Operation: telemetry.OpProbe,
		Transport: string(transport), Status: status,
		StartedAt: start, FinishedAt: fin,
		ErrorCode: classifyErrCode(perr), ErrorMessage: errMsg,
		Summary: map[string]any{
			"reachable":          result.Reachable,
			"identity":           result.IdentityName,
			"routeros_version":   result.RouterOSVersion,
			"wireless_available": result.WirelessAvailable,
			"wifi_package":       result.WiFiPackage,
		},
	})
	_ = pollID

	if perr == nil {
		_ = s.Tel.CapabilitiesUpsert(ctx, dev.id, caps)
	}
	if s.Audit != nil {
		_ = s.Audit.Write(ctx, audit.Entry{
			Actor: actor, Action: audit.ActionScheduledCheckRan,
			Subject: "device:" + dev.id,
			Outcome: outcomeFor(status), Reason: errMsg,
			Metadata: map[string]any{
				"operation": "probe", "transport": string(transport), "vendor": dev.vendor,
			},
		})
	}
	if perr != nil {
		return result, perr
	}
	return result, nil
}

// pollMikroTik runs the Phase 3 MikroTik poll path.
func (s *Service) pollMikroTik(ctx context.Context, dev *deviceLookup, actor string) (*mikrotik.MikroTikReadOnlySnapshot, error) {
	cred, err := s.loadCredential(ctx, dev.id)
	if err != nil {
		return nil, err
	}
	transport, err := transportFor(cred.authType)
	if err != nil {
		return nil, err
	}
	if s.Vault == nil {
		return nil, mikrotik.ErrVaultNotConfigured
	}
	secret, err := s.Vault.Decrypt(cred.cipherText)
	if err != nil {
		return nil, mikrotik.ErrVaultNotConfigured
	}
	cfg := mikrotik.Config{
		DeviceID: dev.id, Host: dev.ip, Username: cred.username,
		Transport: transport, SNMPCommunity: secret, TimeoutSec: 10,
		VerifyTLS:             cred.tlsVerify,
		SSHHostKeyPolicy:      cred.sshHostKeyPolicy,
		SSHHostKeyFingerprint: cred.sshHostKeyFingerprint,
	}
	if cred.port != nil {
		cfg.Port = *cred.port
	}

	snap, caps, perr := mikrotik.Poll(ctx, cfg, secret)
	status := telemetry.StatusSuccess
	errMsg := ""
	if perr != nil {
		status = telemetry.StatusFailed
		errMsg = mikrotik.SanitizeError(perr)
	} else if len(snap.Errors) > 0 {
		status = telemetry.StatusPartial
		errMsg = "partial: " + strings.Join(snap.Errors, "; ")
	}

	pollID, _ := s.Tel.RecordPollResult(ctx, telemetry.PollResultInput{
		DeviceID: dev.id, Vendor: dev.vendor, Operation: telemetry.OpPoll,
		Transport: string(transport), Status: status,
		StartedAt: snap.StartedAt, FinishedAt: snap.FinishedAt,
		ErrorCode: classifyErrCode(perr), ErrorMessage: errMsg,
		Summary: map[string]any{
			"interfaces":          len(snap.Interfaces),
			"wireless_interfaces": len(snap.WirelessInterfaces),
			"wireless_clients":    len(snap.WirelessClients),
		},
	})

	if perr == nil && pollID > 0 {
		if perr2 := s.Tel.PersistMikroTikSnapshot(ctx, pollID, snap); perr2 != nil {
			snap.Errors = append(snap.Errors, "persist:"+mikrotik.SanitizeError(perr2))
		}
		_ = s.Tel.CapabilitiesUpsert(ctx, dev.id, caps)
	}
	if s.Audit != nil {
		_ = s.Audit.Write(ctx, audit.Entry{
			Actor: actor, Action: audit.ActionScheduledCheckRan,
			Subject: "device:" + dev.id, Outcome: outcomeFor(status), Reason: errMsg,
			Metadata: map[string]any{
				"operation": "poll", "transport": string(transport), "vendor": dev.vendor,
				"clients": len(snap.WirelessClients),
			},
		})
	}
	if perr != nil {
		return snap, perr
	}
	return snap, nil
}

func classifyErrCode(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, mikrotik.ErrTimeout):
		return "timeout"
	case errors.Is(err, mikrotik.ErrAuth):
		return "auth"
	case errors.Is(err, mikrotik.ErrUnreachable):
		return "unreachable"
	case errors.Is(err, mikrotik.ErrParse):
		return "parse"
	case errors.Is(err, mikrotik.ErrCredentialMissing):
		return "credential_missing"
	case errors.Is(err, mikrotik.ErrVaultNotConfigured):
		return "vault_not_configured"
	case errors.Is(err, mikrotik.ErrDisallowedCommand):
		return "disallowed_command"
	}
	return "unknown"
}

func outcomeFor(s telemetry.PollStatus) audit.Outcome {
	switch s {
	case telemetry.StatusSuccess, telemetry.StatusPartial:
		return audit.OutcomeSuccess
	case telemetry.StatusBlocked:
		return audit.OutcomeBlocked
	default:
		return audit.OutcomeFailure
	}
}
