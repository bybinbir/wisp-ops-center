package mikrotik

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/go-routeros/routeros/v3"
)

// ErrInvalidCA, ca_certificate_pem geçerli bir PEM bloku değilse veya
// hiçbir sertifika çözümlenemiyorsa döner. Fail-closed davranır:
// VerifyTLS=true iken bu hata alınırsa bağlantı asla kurulmaz.
var ErrInvalidCA = errors.New("mikrotik: invalid ca_certificate_pem")

// APIClient wraps a RouterOS API-SSL connection. It never executes commands
// outside the read-only allowlist; the public methods enforce this.
type APIClient struct {
	cfg     Config
	secret  string
	conn    *routeros.Client
	timeout time.Duration
}

// NewAPIClient creates the wrapper. Connection is deferred to Dial.
func NewAPIClient(cfg Config, secret string) *APIClient {
	t := 8 * time.Second
	if cfg.TimeoutSec > 0 {
		t = time.Duration(cfg.TimeoutSec) * time.Second
	}
	return &APIClient{cfg: cfg, secret: secret, timeout: t}
}

// Dial establishes the TLS API connection with bounded timeout.
func (c *APIClient) Dial(ctx context.Context) error {
	if c.cfg.Host == "" {
		return ErrUnreachable
	}
	port := c.cfg.Port
	if port == 0 {
		port = 8729
	}
	addr := net.JoinHostPort(c.cfg.Host, fmt.Sprintf("%d", port))

	tlsCfg, err := BuildAPITLSConfig(c.cfg)
	if err != nil {
		return err
	}

	dialer := &net.Dialer{Timeout: c.timeout}
	tlsConn, err := tls.DialWithDialer(dialer, "tcp", addr, tlsCfg)
	if err != nil {
		return ClassifyError(err)
	}
	cl, err := routeros.NewClient(tlsConn)
	if err != nil {
		_ = tlsConn.Close()
		return ClassifyError(err)
	}
	if err := cl.Login(c.cfg.Username, c.secret); err != nil {
		cl.Close()
		return ErrAuth
	}
	c.conn = cl
	return nil
}

// Close releases the underlying connection.
func (c *APIClient) Close() {
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
}

// Run executes one allowlisted RouterOS API path and returns the response
// items as map[string]string per row. Disallowed commands are rejected
// before reaching the wire.
func (c *APIClient) Run(ctx context.Context, cmd string) ([]map[string]string, error) {
	if err := EnsureAllowed(cmd); err != nil {
		return nil, err
	}
	if c.conn == nil {
		return nil, errors.New("api: not connected")
	}

	// Bound the call with a deadline derived from ctx + adapter timeout.
	deadline := time.Now().Add(c.timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}

	type result struct {
		reply *routeros.Reply
		err   error
	}
	ch := make(chan result, 1)
	go func() {
		r, e := c.conn.Run(strings.Split(cmd, " ")...)
		ch <- result{r, e}
	}()

	select {
	case <-ctx.Done():
		return nil, ErrTimeout
	case <-time.After(time.Until(deadline)):
		return nil, ErrTimeout
	case r := <-ch:
		if r.err != nil {
			return nil, ClassifyError(r.err)
		}
		out := make([]map[string]string, 0, len(r.reply.Re))
		for _, sentence := range r.reply.Re {
			row := make(map[string]string, len(sentence.Map))
			for k, v := range sentence.Map {
				row[k] = v
			}
			out = append(out, row)
		}
		return out, nil
	}
}
