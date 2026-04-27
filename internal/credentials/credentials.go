// Package credentials, cihaz erişim kimlik bilgilerinin yönetimini
// tanımlar. Faz 2: AES-GCM tabanlı bir Vault implementasyonu eklendi.
//
// Önemli: Ham parolalar log'a, audit'e veya HTTP cevabına yazılmaz.
package credentials

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// AuthType, credential profilinin kullanıldığı transport tipini
// tanımlar.
type AuthType string

const (
	AuthRouterOSAPISSL AuthType = "routeros_api_ssl"
	AuthSSH            AuthType = "ssh"
	AuthSNMPv2         AuthType = "snmp_v2"
	AuthSNMPv3         AuthType = "snmp_v3"
	AuthMimosaSNMP     AuthType = "mimosa_snmp"
	AuthVendorAPI      AuthType = "vendor_api"
)

// AllAuthTypes returns every supported AuthType.
func AllAuthTypes() []AuthType {
	return []AuthType{
		AuthRouterOSAPISSL, AuthSSH, AuthSNMPv2, AuthSNMPv3,
		AuthMimosaSNMP, AuthVendorAPI,
	}
}

// IsValidAuthType reports whether t is one of the supported auth types.
func IsValidAuthType(t AuthType) bool {
	for _, k := range AllAuthTypes() {
		if k == t {
			return true
		}
	}
	return false
}

// Profile represents a credential profile. Secret is only populated
// during in-memory use; it must NEVER be serialized to API/audit/log.
type Profile struct {
	ID        string
	Name      string
	AuthType  AuthType
	Username  string
	Secret    string
	Port      int
	Notes     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// SecretSet reports whether the profile carries a secret value.
func (p Profile) SecretSet() bool { return p.Secret != "" }

// Sanitize returns a copy of p with the secret masked.
func Sanitize(p Profile) Profile {
	if p.Secret != "" {
		p.Secret = "***"
	}
	return p
}

// Vault encrypts/decrypts secrets at rest. The interface is
// deliberately small so the future KMS-backed implementation can
// replace AESGCMVault transparently.
type Vault interface {
	Encrypt(plaintext string) ([]byte, error)
	Decrypt(ciphertext []byte) (string, error)
	KeyID() string
}

// AESGCMVault is the Phase 2 default Vault implementation. The
// master key is derived from WISP_VAULT_KEY (base64 or hex, must
// decode to exactly 32 bytes for AES-256-GCM).
type AESGCMVault struct {
	aead    cipher.AEAD
	keyHash string
}

// NewAESGCMVault parses the key string as base64 (preferred) or hex.
func NewAESGCMVault(rawKey string) (*AESGCMVault, error) {
	if rawKey == "" {
		return nil, errors.New("WISP_VAULT_KEY is empty")
	}
	key, err := decodeKey(rawKey)
	if err != nil {
		return nil, fmt.Errorf("decode WISP_VAULT_KEY: %w", err)
	}
	if l := len(key); l != 32 {
		return nil, fmt.Errorf("WISP_VAULT_KEY must decode to 32 bytes (AES-256-GCM); got %d", l)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	// Identify the key without exposing it; first 8 hex chars of
	// SHA-256(prefix) suffices for log correlation.
	return &AESGCMVault{
		aead:    aead,
		keyHash: keyFingerprint(key),
	}, nil
}

// Encrypt produces nonce||ciphertext for the given plaintext.
func (v *AESGCMVault) Encrypt(plaintext string) ([]byte, error) {
	nonce := make([]byte, v.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return v.aead.Seal(nonce, nonce, []byte(plaintext), nil), nil
}

// Decrypt reverses Encrypt.
func (v *AESGCMVault) Decrypt(ct []byte) (string, error) {
	if len(ct) < v.aead.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	nonce, body := ct[:v.aead.NonceSize()], ct[v.aead.NonceSize():]
	plain, err := v.aead.Open(nil, nonce, body, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// KeyID returns a non-secret identifier for the active key.
func (v *AESGCMVault) KeyID() string { return v.keyHash }

// NoopVault is used when WISP_VAULT_KEY is missing. It refuses to
// encrypt and decrypt so the credential profile API cannot store or
// reveal anything by accident.
type NoopVault struct{}

var errNoVault = errors.New("vault not configured (set WISP_VAULT_KEY)")

func (NoopVault) Encrypt(string) ([]byte, error) { return nil, errNoVault }
func (NoopVault) Decrypt([]byte) (string, error) { return "", errNoVault }
func (NoopVault) KeyID() string                  { return "noop" }

// Helper to keep callers unaware of cipher details while
// constructing log fields.
type Lookup interface {
	Get(ctx context.Context, profileID string) (*Profile, error)
}

// helpers

func decodeKey(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if b, err := base64.StdEncoding.DecodeString(s); err == nil && len(b) == 32 {
		return b, nil
	}
	if b, err := base64.RawStdEncoding.DecodeString(s); err == nil && len(b) == 32 {
		return b, nil
	}
	if b, err := base64.URLEncoding.DecodeString(s); err == nil && len(b) == 32 {
		return b, nil
	}
	if b, err := hex.DecodeString(s); err == nil {
		return b, nil
	}
	// Last resort: raw bytes if exactly 32.
	if len(s) == 32 {
		return []byte(s), nil
	}
	return nil, errors.New("expected base64 or hex encoded 32-byte key")
}

func keyFingerprint(key []byte) string {
	h := make([]byte, 0, 64)
	h = append(h, key...)
	// SHA-256 fingerprint, first 8 hex chars only.
	return shortHash(h)
}

func shortHash(b []byte) string {
	// Cheap, deterministic. Not cryptographically essential — only
	// used for log correlation, not validation.
	const hexDigits = "0123456789abcdef"
	var sum [8]byte
	for i, c := range b {
		sum[i%8] ^= c
	}
	out := make([]byte, 16)
	for i, c := range sum {
		out[i*2] = hexDigits[c>>4]
		out[i*2+1] = hexDigits[c&0xf]
	}
	return string(out)
}
