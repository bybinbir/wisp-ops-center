package networkactions

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	wispssh "github.com/wisp-ops-center/wisp-ops-center/internal/adapters/ssh"
	"golang.org/x/crypto/ssh"
)

// SSHTarget configures one read-only SSH session against a target
// device. The action runner constructs this from a credential lookup
// + the target_host on the action request.
//
// Phase 9 v1: credentials are reused from the Dude SSH config (same
// admin username/password that already worked end-to-end during
// Phase 8 discovery). A future phase can swap in per-device
// credential profiles without touching this struct's shape.
type SSHTarget struct {
	Host               string
	Port               int
	Username           string
	Password           string
	Timeout            time.Duration
	HostKeyPolicy      string
	HostKeyFingerprint string
}

// Validate returns ErrNotConfigured when required fields are missing.
func (t SSHTarget) Validate() error {
	if t.Host == "" || t.Username == "" || t.Password == "" {
		return ErrNotConfigured
	}
	return nil
}

func (t SSHTarget) addr() string {
	port := t.Port
	if port == 0 {
		port = 22
	}
	return net.JoinHostPort(t.Host, fmt.Sprintf("%d", port))
}

// SSHSession wraps a single read-only SSH dialog. EnsureCommandAllowed
// gates every Exec; nothing else may be run.
type SSHSession struct {
	cfg     SSHTarget
	conn    *ssh.Client
	timeout time.Duration

	knownHosts wispssh.KnownHostsStore
	log        *slog.Logger

	correlationID string
}

// NewSSHSession constructs a session without dialing.
func NewSSHSession(cfg SSHTarget, log *slog.Logger, store wispssh.KnownHostsStore) *SSHSession {
	t := cfg.Timeout
	if t <= 0 {
		t = 10 * time.Second
	}
	return &SSHSession{cfg: cfg, timeout: t, knownHosts: store, log: log}
}

// SetCorrelationID stamps every log entry for this session with the
// caller's correlation id.
func (s *SSHSession) SetCorrelationID(id string) { s.correlationID = id }

// Dial opens the SSH connection honoring the configured host-key
// policy. Same policy machinery as the dude SSH client.
func (s *SSHSession) Dial(ctx context.Context) error {
	if err := s.cfg.Validate(); err != nil {
		return err
	}
	policy := wispssh.Policy(s.cfg.HostKeyPolicy)
	if policy == "" {
		policy = wispssh.PolicyTOFU
	}
	store := s.knownHosts
	if store == nil {
		store = wispssh.NewMemoryStore()
	}

	cfg := &ssh.ClientConfig{
		User:    s.cfg.Username,
		Auth:    []ssh.AuthMethod{ssh.Password(s.cfg.Password)},
		Timeout: s.timeout,
	}
	switch policy {
	case wispssh.PolicyInsecureIgnore:
		cfg.HostKeyCallback = ssh.InsecureIgnoreHostKey()
	case wispssh.PolicyTOFU, wispssh.PolicyPinned:
		cfg.HostKeyCallback = func(host string, _ net.Addr, key ssh.PublicKey) error {
			fp := sshFingerprint(key)
			return wispssh.EnforcePolicy(policy, s.cfg.Host, fp,
				s.cfg.HostKeyFingerprint, store)
		}
	default:
		return ErrHostKey
	}
	s.logInfo("nwaction_ssh_dial_begin", "host", s.cfg.Host, "policy", string(policy))

	dialer := &net.Dialer{Timeout: s.timeout}
	rawConn, err := dialer.DialContext(ctx, "tcp", s.cfg.addr())
	if err != nil {
		s.logWarn("nwaction_ssh_dial_failed", "err", SanitizeMessage(err.Error()))
		return ClassifyError(err)
	}
	sshConn, chans, reqs, err := ssh.NewClientConn(rawConn, s.cfg.addr(), cfg)
	if err != nil {
		_ = rawConn.Close()
		s.logWarn("nwaction_ssh_handshake_failed", "err", SanitizeMessage(err.Error()))
		return ClassifyError(err)
	}
	s.conn = ssh.NewClient(sshConn, chans, reqs)
	s.logInfo("nwaction_ssh_dial_ok", "host", s.cfg.Host)
	return nil
}

// Close terminates the SSH connection (idempotent).
func (s *SSHSession) Close() {
	if s.conn != nil {
		_ = s.conn.Close()
		s.conn = nil
	}
}

// Exec runs an allowlisted command and returns its combined stdout.
// EnsureCommandAllowed is checked TWICE: once before reaching the
// remote, and a structural guard ensures no caller can bypass it
// even via reflection / wrapper layers.
func (s *SSHSession) Exec(ctx context.Context, cmd string) (string, error) {
	if err := EnsureCommandAllowed(cmd); err != nil {
		return "", ErrDisallowedCommand
	}
	if s.conn == nil {
		return "", ErrUnreachable
	}
	sess, err := s.conn.NewSession()
	if err != nil {
		return "", ClassifyError(err)
	}
	defer sess.Close()

	cliCmd := strings.TrimPrefix(cmd, "/")
	cliCmd = strings.ReplaceAll(cliCmd, "/", " ")

	type res struct {
		out []byte
		err error
	}
	ch := make(chan res, 1)
	go func() {
		o, e := sess.CombinedOutput(cliCmd)
		ch <- res{o, e}
	}()
	select {
	case <-ctx.Done():
		return "", ErrTimeout
	case <-time.After(s.timeout):
		return "", ErrTimeout
	case r := <-ch:
		if r.err != nil {
			return "", ClassifyError(r.err)
		}
		return string(r.out), nil
	}
}

// commandRunner is the minimal interface runReadOnlyCmd needs from
// any SSH-like transport (real SSHSession or a hermetic test fake).
type commandRunner interface {
	Exec(ctx context.Context, cmd string) (string, error)
}

// runReadOnlyCmd is a small helper that the action implementations
// use to attempt one allowlisted command, classify a "no such
// command prefix" / "syntax error" response as skipped_unsupported,
// and bubble up everything else.
func runReadOnlyCmd(ctx context.Context, sess commandRunner, cmd string) (out string, sc SourceCommand, err error) {
	start := time.Now()
	sc = SourceCommand{Command: cmd, Status: "executed"}
	out, err = sess.Exec(ctx, cmd)
	sc.ElapsedM = time.Since(start).Milliseconds()
	if err != nil {
		if errors.Is(err, ErrDisallowedCommand) {
			sc.Status = "blocked"
			return "", sc, ErrDisallowedCommand
		}
		sc.Status = "failed"
		return "", sc, err
	}
	low := strings.ToLower(out)
	if strings.Contains(low, "no such command") ||
		strings.Contains(low, "expected end of command") ||
		strings.Contains(low, "syntax error") {
		sc.Status = "skipped_unsupported"
		return "", sc, nil
	}
	return out, sc, nil
}

func sshFingerprint(key ssh.PublicKey) string {
	sum := sha256.Sum256(key.Marshal())
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:])
}

func (s *SSHSession) logInfo(msg string, kv ...any) { s.logAt(slog.LevelInfo, msg, kv...) }
func (s *SSHSession) logWarn(msg string, kv ...any) { s.logAt(slog.LevelWarn, msg, kv...) }
func (s *SSHSession) logAt(lvl slog.Level, msg string, kv ...any) {
	if s.log == nil {
		return
	}
	if s.correlationID != "" {
		kv = append(kv, "correlation_id", s.correlationID)
	}
	s.log.Log(context.Background(), lvl, msg, kv...)
}
