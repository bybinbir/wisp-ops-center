package ssh

import (
	"errors"
	"strings"
	"sync"
)

// Policy enumerates the SSH host key trust policies supported by the
// credential profile schema added in Phase 4.
type Policy string

const (
	PolicyInsecureIgnore Policy = "insecure_ignore"
	PolicyTOFU           Policy = "trust_on_first_use"
	PolicyPinned         Policy = "pinned"
)

// Errors used by the host key store.
var (
	ErrFingerprintMismatch = errors.New("ssh: host key fingerprint mismatch")
	ErrPinnedMissing       = errors.New("ssh: pinned policy requires fingerprint")
	ErrUnknownPolicy       = errors.New("ssh: unknown host key policy")
)

// KnownHostsStore is a tiny abstraction over the persisted
// ssh_known_hosts state. It is intentionally interface-shaped so a
// Postgres-backed implementation can replace the in-memory one used in
// tests without touching the policy logic in EnforcePolicy.
type KnownHostsStore interface {
	// Get returns the stored fingerprint for the host (and a boolean
	// for "we have one"). Empty string + false means first contact.
	Get(host string) (string, bool, error)
	// Put records a fingerprint as known.
	Put(host, fingerprint string) error
}

// MemoryStore is an in-memory KnownHostsStore for tests + early use.
type MemoryStore struct {
	mu sync.Mutex
	m  map[string]string
}

// NewMemoryStore returns an empty in-memory store.
func NewMemoryStore() *MemoryStore { return &MemoryStore{m: map[string]string{}} }

// Get implements KnownHostsStore.
func (s *MemoryStore) Get(host string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[host]
	return v, ok, nil
}

// Put implements KnownHostsStore.
func (s *MemoryStore) Put(host, fingerprint string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[host] = fingerprint
	return nil
}

// EnforcePolicy applies the configured policy to a freshly observed
// fingerprint. The pinned fingerprint comes from the credential
// profile (`ssh_host_key_fingerprint`); the store lives at
// ssh_known_hosts. EnforcePolicy NEVER returns the raw fingerprint in
// its error text.
func EnforcePolicy(policy Policy, host, observed, pinned string, store KnownHostsStore) error {
	host = strings.TrimSpace(host)
	observed = strings.TrimSpace(observed)
	pinned = strings.TrimSpace(pinned)

	switch policy {
	case PolicyInsecureIgnore, "":
		return nil
	case PolicyPinned:
		if pinned == "" {
			return ErrPinnedMissing
		}
		if !strings.EqualFold(pinned, observed) {
			return ErrFingerprintMismatch
		}
		return nil
	case PolicyTOFU:
		if store == nil {
			return errors.New("ssh: tofu policy requires store")
		}
		known, ok, err := store.Get(host)
		if err != nil {
			return err
		}
		if !ok {
			// First contact: trust + persist.
			return store.Put(host, observed)
		}
		if !strings.EqualFold(known, observed) {
			return ErrFingerprintMismatch
		}
		return nil
	}
	return ErrUnknownPolicy
}
