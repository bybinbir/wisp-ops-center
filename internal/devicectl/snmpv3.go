package devicectl

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/wisp-ops-center/wisp-ops-center/internal/adapters/mimosa"
)

// snmpv3USM is the in-memory bundle decrypted from credential_profiles.
type snmpv3USM struct {
	Username   string
	Level      string
	AuthProto  string
	AuthSecret string
	PrivProto  string
	PrivSecret string
}

// loadSNMPv3USM fetches and decrypts SNMPv3 USM fields. Returns
// mimosa.ErrSNMPv3Misconfigured if any required field is empty.
func (s *Service) loadSNMPv3USM(ctx context.Context, profileID string) (*snmpv3USM, error) {
	if s.Vault == nil {
		return nil, mimosa.ErrVaultNotConfigured
	}
	row := s.P.QueryRow(ctx, `
SELECT COALESCE(snmpv3_username,''),
       COALESCE(snmpv3_security_level,''),
       COALESCE(snmpv3_auth_protocol,''),
       snmpv3_auth_secret_ciphertext,
       COALESCE(snmpv3_priv_protocol,''),
       snmpv3_priv_secret_ciphertext
  FROM credential_profiles
 WHERE id = $1`, profileID)

	var u snmpv3USM
	var authCT, privCT []byte
	err := row.Scan(&u.Username, &u.Level, &u.AuthProto, &authCT, &u.PrivProto, &privCT)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, mimosa.ErrCredentialMissing
	}
	if err != nil {
		return nil, err
	}
	if u.Username == "" {
		return nil, mimosa.ErrSNMPv3Misconfigured
	}
	switch u.Level {
	case "noAuthNoPriv":
		// auth/priv may be empty.
	case "authNoPriv":
		if len(authCT) == 0 {
			return nil, mimosa.ErrSNMPv3Misconfigured
		}
		v, derr := s.Vault.Decrypt(authCT)
		if derr != nil {
			return nil, mimosa.ErrSNMPv3Misconfigured
		}
		u.AuthSecret = v
	case "authPriv":
		if len(authCT) == 0 || len(privCT) == 0 {
			return nil, mimosa.ErrSNMPv3Misconfigured
		}
		a, e1 := s.Vault.Decrypt(authCT)
		if e1 != nil {
			return nil, mimosa.ErrSNMPv3Misconfigured
		}
		p, e2 := s.Vault.Decrypt(privCT)
		if e2 != nil {
			return nil, mimosa.ErrSNMPv3Misconfigured
		}
		u.AuthSecret = a
		u.PrivSecret = p
	default:
		return nil, mimosa.ErrSNMPv3Misconfigured
	}
	return &u, nil
}
