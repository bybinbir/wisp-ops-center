package mikrotik

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	wispssh "github.com/wisp-ops-center/wisp-ops-center/internal/adapters/ssh"
	"golang.org/x/crypto/ssh"
)

// SSHClient runs a tiny set of read-only RouterOS CLI commands as a
// fallback when the API-SSL transport is unavailable.
//
// Important: the SSH path executes EXACT command strings only. The strings
// must already be allowlisted by EnsureAllowed before reaching Exec.
type SSHClient struct {
	cfg     Config
	secret  string
	conn    *ssh.Client
	timeout time.Duration

	// Faz 6: TOFU/Pinned politikası için Postgres-backed store. nil ise
	// store olarak in-memory fallback kullanılır (testler için yeterli).
	knownHosts wispssh.KnownHostsStore
}

// globalKnownHostsStore, NewSSHClient'ın varsayılan olarak kullanacağı
// store. Service init aşamasında SetGlobalKnownHostsStore ile ayarlanır.
var globalKnownHostsStore wispssh.KnownHostsStore

// SetGlobalKnownHostsStore, NewSSHClient çağrılarının varsayılan store'unu
// ayarlar. Service tek process'te tek instance olduğu için global state
// kabul edilebilir; per-call override gerekirse SetKnownHostsStore kullanın.
func SetGlobalKnownHostsStore(s wispssh.KnownHostsStore) {
	globalKnownHostsStore = s
}

// NewSSHClient wires up the SSH client without dialing.
func NewSSHClient(cfg Config, secret string) *SSHClient {
	t := 8 * time.Second
	if cfg.TimeoutSec > 0 {
		t = time.Duration(cfg.TimeoutSec) * time.Second
	}
	return &SSHClient{cfg: cfg, secret: secret, timeout: t, knownHosts: globalKnownHostsStore}
}

// SetKnownHostsStore, TOFU/Pinned policy uygulamak için store enjekte eder.
// Çağrılmazsa in-memory fallback kullanılır (üretimde Postgres-backed olmalı).
func (c *SSHClient) SetKnownHostsStore(s wispssh.KnownHostsStore) {
	c.knownHosts = s
}

// Dial establishes the SSH connection. Faz 6'dan itibaren
// SSHHostKeyPolicy alanı dikkate alınır; "insecure_ignore" varsayılan,
// diğer politikalarda EnforcePolicy çağrılır.
func (c *SSHClient) Dial(ctx context.Context) error {
	if c.cfg.Host == "" {
		return ErrUnreachable
	}
	port := c.cfg.Port
	if port == 0 {
		port = 22
	}
	addr := net.JoinHostPort(c.cfg.Host, fmt.Sprintf("%d", port))

	policy := wispssh.Policy(c.cfg.SSHHostKeyPolicy)
	if policy == "" {
		policy = wispssh.PolicyInsecureIgnore
	}
	store := c.knownHosts
	if store == nil {
		store = wispssh.NewMemoryStore()
	}

	cfg := &ssh.ClientConfig{
		User:    c.cfg.Username,
		Auth:    []ssh.AuthMethod{ssh.Password(c.secret)},
		Timeout: c.timeout,
	}

	switch policy {
	case wispssh.PolicyInsecureIgnore:
		cfg.HostKeyCallback = ssh.InsecureIgnoreHostKey()
	case wispssh.PolicyTOFU, wispssh.PolicyPinned:
		// Custom callback fingerprint hesaplar ve EnforcePolicy uygular
		cfg.HostKeyCallback = func(host string, _ net.Addr, key ssh.PublicKey) error {
			fp := sshFingerprint(key)
			if err := wispssh.EnforcePolicy(policy, c.cfg.Host, fp,
				c.cfg.SSHHostKeyFingerprint, store); err != nil {
				return err
			}
			return nil
		}
	default:
		return wispssh.ErrUnknownPolicy
	}

	conn, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return ClassifyError(err)
	}
	c.conn = conn
	return nil
}

// sshFingerprint, ssh.PublicKey'den SHA256 base64 fingerprint üretir.
func sshFingerprint(key ssh.PublicKey) string {
	sum := sha256.Sum256(key.Marshal())
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:])
}

// Close terminates the SSH session.
func (c *SSHClient) Close() {
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
}

// Exec runs an allowlisted command and returns its stdout. The command
// must already be present in the allowlist; otherwise ErrDisallowedCommand.
func (c *SSHClient) Exec(ctx context.Context, cmd string) (string, error) {
	if err := EnsureAllowed(cmd); err != nil {
		return "", err
	}
	if c.conn == nil {
		return "", errors.New("ssh: not connected")
	}
	sess, err := c.conn.NewSession()
	if err != nil {
		return "", ClassifyError(err)
	}
	defer sess.Close()

	// MikroTik SSH expects RouterOS CLI commands. We accept the API-style
	// path "/system/identity/print" and translate it to CLI form:
	// "system identity print"
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
	case <-time.After(c.timeout):
		return "", ErrTimeout
	case r := <-ch:
		if r.err != nil {
			return "", ClassifyError(r.err)
		}
		return string(r.out), nil
	}
}
