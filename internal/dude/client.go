package dude

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	wispssh "github.com/wisp-ops-center/wisp-ops-center/internal/adapters/ssh"
	"golang.org/x/crypto/ssh"
)

// Config configures the SSH client used by Discover.
type Config struct {
	Host               string
	Port               int
	Username           string
	Password           string
	Timeout            time.Duration
	HostKeyPolicy      string
	HostKeyFingerprint string
}

// Validate returns ErrNotConfigured when required fields are missing.
func (c Config) Validate() error {
	if c.Host == "" || c.Username == "" || c.Password == "" {
		return ErrNotConfigured
	}
	return nil
}

func (c Config) addr() string {
	port := c.Port
	if port == 0 {
		port = 22
	}
	return net.JoinHostPort(c.Host, fmt.Sprintf("%d", port))
}

// Client wraps a single SSH session against the Dude device.
type Client struct {
	cfg     Config
	conn    *ssh.Client
	timeout time.Duration

	knownHosts wispssh.KnownHostsStore
	log        *slog.Logger

	correlationID string
}

func NewClient(cfg Config, log *slog.Logger, store wispssh.KnownHostsStore) *Client {
	t := cfg.Timeout
	if t <= 0 {
		t = 10 * time.Second
	}
	return &Client{cfg: cfg, timeout: t, knownHosts: store, log: log}
}

func (c *Client) SetCorrelationID(id string) { c.correlationID = id }

// Dial opens the SSH connection and applies the configured host
// key policy.
func (c *Client) Dial(ctx context.Context) error {
	if err := c.cfg.Validate(); err != nil {
		return err
	}

	policy := wispssh.Policy(c.cfg.HostKeyPolicy)
	if policy == "" {
		policy = wispssh.PolicyTOFU
	}
	store := c.knownHosts
	if store == nil {
		store = wispssh.NewMemoryStore()
	}

	cfg := &ssh.ClientConfig{
		User:    c.cfg.Username,
		Auth:    []ssh.AuthMethod{ssh.Password(c.cfg.Password)},
		Timeout: c.timeout,
	}

	switch policy {
	case wispssh.PolicyInsecureIgnore:
		cfg.HostKeyCallback = ssh.InsecureIgnoreHostKey()
	case wispssh.PolicyTOFU, wispssh.PolicyPinned:
		cfg.HostKeyCallback = func(hostname string, _ net.Addr, key ssh.PublicKey) error {
			fp := sshFingerprint(key)
			return wispssh.EnforcePolicy(policy, c.cfg.Host, fp,
				c.cfg.HostKeyFingerprint, store)
		}
	default:
		return ErrHostKey
	}

	c.logInfo("dude_dial_begin", "host", c.cfg.Host, "policy", string(policy))

	dialer := &net.Dialer{Timeout: c.timeout}
	rawConn, err := dialer.DialContext(ctx, "tcp", c.cfg.addr())
	if err != nil {
		c.logWarn("dude_dial_failed", "err", SanitizeMessage(err.Error()))
		return ClassifyError(err)
	}
	sshConn, chans, reqs, err := ssh.NewClientConn(rawConn, c.cfg.addr(), cfg)
	if err != nil {
		_ = rawConn.Close()
		c.logWarn("dude_handshake_failed", "err", SanitizeMessage(err.Error()))
		return ClassifyError(err)
	}
	c.conn = ssh.NewClient(sshConn, chans, reqs)
	c.logInfo("dude_dial_ok", "host", c.cfg.Host)
	return nil
}

func (c *Client) Close() {
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
}

// Exec runs an allowlisted RouterOS CLI command and returns its
// combined stdout.
func (c *Client) Exec(ctx context.Context, cmd string) (string, error) {
	if err := EnsureAllowed(cmd); err != nil {
		return "", err
	}
	if c.conn == nil {
		return "", ErrUnreachable
	}

	sess, err := c.conn.NewSession()
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
	case <-time.After(c.timeout):
		return "", ErrTimeout
	case r := <-ch:
		if r.err != nil {
			return "", ClassifyError(r.err)
		}
		return string(r.out), nil
	}
}

// TestResult is the JSON-friendly outcome of TestConnection.
type TestResult struct {
	Reachable  bool      `json:"reachable"`
	DurationMS int64     `json:"duration_ms"`
	Identity   string    `json:"identity,omitempty"`
	Error      string    `json:"error,omitempty"`
	ErrorCode  string    `json:"error_code,omitempty"`
	StartedAt  time.Time `json:"started_at"`
	Host       string    `json:"host"`
}

// TestConnection performs a one-shot reachability + auth probe.
func TestConnection(ctx context.Context, cfg Config, log *slog.Logger, store wispssh.KnownHostsStore) TestResult {
	res := TestResult{StartedAt: time.Now().UTC(), Host: cfg.Host}
	defer func() { res.DurationMS = time.Since(res.StartedAt).Milliseconds() }()

	if err := cfg.Validate(); err != nil {
		res.Error = "Configuration incomplete"
		res.ErrorCode = ErrorCode(err)
		return res
	}

	c := NewClient(cfg, log, store)
	if err := c.Dial(ctx); err != nil {
		res.Error = SanitizeMessage(err.Error())
		res.ErrorCode = ErrorCode(err)
		return res
	}
	defer c.Close()

	out, err := c.Exec(ctx, "/system/identity/print")
	if err != nil {
		res.Error = SanitizeMessage(err.Error())
		res.ErrorCode = ErrorCode(err)
		return res
	}
	res.Reachable = true
	res.Identity = parseIdentityName(out)
	return res
}

func parseIdentityName(out string) string {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(line), "name:") {
			return strings.TrimSpace(line[5:])
		}
	}
	return ""
}

func sshFingerprint(key ssh.PublicKey) string {
	sum := sha256.Sum256(key.Marshal())
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:])
}

func (c *Client) logInfo(msg string, kv ...any) { c.logAt(slog.LevelInfo, msg, kv...) }
func (c *Client) logWarn(msg string, kv ...any) { c.logAt(slog.LevelWarn, msg, kv...) }
func (c *Client) logAt(lvl slog.Level, msg string, kv ...any) {
	if c.log == nil {
		return
	}
	if c.correlationID != "" {
		kv = append(kv, "correlation_id", c.correlationID)
	}
	c.log.Log(context.Background(), lvl, msg, kv...)
}
