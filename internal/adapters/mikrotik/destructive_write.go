package mikrotik

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Phase 10E — destructive write surface.
//
// This file is the ONLY place inside the mikrotik adapter that
// constructs and executes a RouterOS command capable of mutating
// device state. The read-only allowlist in allowlist.go is unchanged
// and stays the default; writes go through a SECOND, much smaller
// allowlist defined here.
//
// Layered safety the destructive runtime depends on:
//
//   1. Path allowlist: WriteAllowlistedCommands lists the exact
//      RouterOS paths the destructive runtime may invoke. Phase 10E
//      ships exactly one entry: "/interface/wireless/set". Anything
//      else is rejected before a byte hits the wire.
//
//   2. Required-args check: WriteRequiredArgs forces every call to
//      carry the operator-meaningful identifiers (which interface,
//      what target value). A bare "/interface/wireless/set" with no
//      args is rejected — the runtime cannot accidentally apply a
//      change to the wrong interface or strip a required field.
//
//   3. Argument-key allowlist: WriteAllowedArgs enumerates which
//      key=value pairs may appear in the args map. Anything outside
//      the set (e.g. "ssid=", "passphrase=") is rejected; the
//      destructive runtime cannot smuggle a second mutation through
//      the same path.
//
//   4. Server-side argument formatting: the caller passes a typed
//      args map; this file is the only place that flattens it into a
//      RouterOS CLI string. Operator-supplied free-form text never
//      reaches the device.
//
// Allowlist drift (adding a path / loosening a required arg) is a
// deliberate, security-reviewed change. Tests in
// destructive_write_test.go pin every allowlist entry.

// WriteAllowlistedCommands is the destructive write counterpart to
// AllowlistedCommands in allowlist.go. Phase 10E lists exactly one
// path. Future destructive Kinds extend this list AFTER review.
var WriteAllowlistedCommands = []string{
	"/interface/wireless/set",
}

// WriteRequiredArgs maps an allowlisted write path to the set of
// args the runtime MUST supply. Missing any one of these is a
// rejection.
var WriteRequiredArgs = map[string][]string{
	"/interface/wireless/set": {"number", "frequency"},
}

// WriteAllowedArgs maps an allowlisted write path to the FULL set of
// args the runtime is permitted to send. Anything outside the set
// for that path is rejected. The intent is to prevent the
// destructive runtime from quietly piggybacking a second mutation
// (e.g. SSID change) onto a frequency adjust.
var WriteAllowedArgs = map[string]map[string]struct{}{
	"/interface/wireless/set": {
		"number":    {},
		"frequency": {},
	},
}

// ErrDisallowedWrite is returned for any write request that fails
// the path / required-args / arg-key allowlist checks.
var ErrDisallowedWrite = errors.New("mikrotik: destructive write not allowed")

// IsWriteAllowed returns true when (cmd, args) clears every layer of
// the destructive write allowlist. The check is order-insensitive
// over args; path and arg keys are normalised to lowercase so an
// operator typo cannot defeat the allowlist by case mismatch.
func IsWriteAllowed(cmd string, args map[string]string) bool {
	cmd = strings.TrimSpace(strings.ToLower(cmd))
	if cmd == "" {
		return false
	}
	allowedKeys, ok := WriteAllowedArgs[cmd]
	if !ok {
		return false
	}
	// Normalise the args map to lowercase keys once so both the
	// extra-arg check and the required-args lookup operate on the
	// same canonical form.
	normArgs := make(map[string]string, len(args))
	for k, v := range args {
		normArgs[strings.ToLower(strings.TrimSpace(k))] = v
	}
	for k := range normArgs {
		if _, allowed := allowedKeys[k]; !allowed {
			return false
		}
	}
	required, ok := WriteRequiredArgs[cmd]
	if !ok {
		return false
	}
	for _, r := range required {
		v, present := normArgs[r]
		if !present || strings.TrimSpace(v) == "" {
			return false
		}
	}
	pathAllowed := false
	for _, p := range WriteAllowlistedCommands {
		if cmd == p {
			pathAllowed = true
			break
		}
	}
	return pathAllowed
}

// EnsureWriteAllowed returns ErrDisallowedWrite when the request
// fails the destructive write allowlist.
func EnsureWriteAllowed(cmd string, args map[string]string) error {
	if IsWriteAllowed(cmd, args) {
		return nil
	}
	return ErrDisallowedWrite
}

// FormatWriteCmd flattens (path, args) into the RouterOS CLI form
// SSHClient.ExecWrite ships. Args are sorted by key so the rendered
// command is deterministic — useful for audit fingerprinting + test
// snapshots. Caller MUST have run EnsureWriteAllowed first.
func FormatWriteCmd(cmd string, args map[string]string) string {
	cli := strings.TrimPrefix(cmd, "/")
	cli = strings.ReplaceAll(cli, "/", " ")
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		cli += " " + k + "=" + args[k]
	}
	return cli
}

// ExecWrite executes a destructive write through the SSH session.
// It enforces EnsureWriteAllowed BEFORE opening a session, formats
// the command via FormatWriteCmd, and parses the device reply
// strictly. Returns the device's stdout on success.
//
// Phase 10E only routes /interface/wireless/set through this path.
// Future destructive Kinds add their paths to the WriteAllowlistedCommands
// + WriteRequiredArgs + WriteAllowedArgs trio in this file and reuse
// ExecWrite — there is exactly one place to read to know what the
// destructive runtime can send.
func (c *SSHClient) ExecWrite(ctx context.Context, cmd string, args map[string]string) (string, error) {
	if err := EnsureWriteAllowed(cmd, args); err != nil {
		return "", err
	}
	if c.conn == nil {
		return "", errors.New("ssh: not connected")
	}
	cli := FormatWriteCmd(cmd, args)
	sess, err := c.conn.NewSession()
	if err != nil {
		return "", ClassifyError(err)
	}
	defer sess.Close()
	type res struct {
		out []byte
		err error
	}
	ch := make(chan res, 1)
	go func() {
		o, e := sess.CombinedOutput(cli)
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
		out := strings.TrimSpace(string(r.out))
		// RouterOS replies with empty body on a successful set. A
		// body that contains "failure", "no such item" or "input
		// does not match" is a device-side rejection — bubble it.
		low := strings.ToLower(out)
		if strings.Contains(low, "failure") ||
			strings.Contains(low, "no such item") ||
			strings.Contains(low, "input does not match") ||
			strings.Contains(low, "syntax error") {
			return out, fmt.Errorf("mikrotik: device rejected write: %s", out)
		}
		return out, nil
	}
}
