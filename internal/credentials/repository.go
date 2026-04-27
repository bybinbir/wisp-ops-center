package credentials

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned by Repository when a profile is missing.
var ErrNotFound = errors.New("credential profile not found")

// ErrValidation indicates an invalid request payload.
type ErrValidation struct{ Msg string }

func (e *ErrValidation) Error() string { return "validation: " + e.Msg }

// Repository persists credential profiles. Secrets are encrypted at
// rest using the configured Vault.
type Repository struct {
	P     *pgxpool.Pool
	Vault Vault
}

func NewRepository(p *pgxpool.Pool, v Vault) *Repository { return &Repository{P: p, Vault: v} }

// View is the API-safe representation of a credential profile.
type View struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	AuthType  string    `json:"auth_type"`
	Username  string    `json:"username,omitempty"`
	Port      *int      `json:"port,omitempty"`
	SecretSet bool      `json:"secret_set"`
	Notes     string    `json:"notes,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	// Phase 4 additions — never include actual ciphertext or plaintext.
	SNMPv3Username        string `json:"snmpv3_username,omitempty"`
	SNMPv3SecurityLevel   string `json:"snmpv3_security_level,omitempty"`
	SNMPv3AuthProtocol    string `json:"snmpv3_auth_protocol,omitempty"`
	SNMPv3AuthSet         bool   `json:"snmpv3_auth_set"`
	SNMPv3PrivProtocol    string `json:"snmpv3_priv_protocol,omitempty"`
	SNMPv3PrivSet         bool   `json:"snmpv3_priv_set"`
	VerifyTLS             bool   `json:"verify_tls"`
	ServerNameOverride    string `json:"server_name_override,omitempty"`
	CACertificateSet      bool   `json:"ca_certificate_set"`
	SSHHostKeyPolicy      string `json:"ssh_host_key_policy,omitempty"`
	SSHHostKeyFingerprint string `json:"ssh_host_key_fingerprint,omitempty"`
}

const viewCols = `id, name, auth_type, COALESCE(username,''), port, secret_ciphertext,
COALESCE(notes,''), created_at, COALESCE(rotated_at, created_at) AS updated_at,
COALESCE(snmpv3_username,''), COALESCE(snmpv3_security_level,''),
COALESCE(snmpv3_auth_protocol,''), snmpv3_auth_secret_ciphertext,
COALESCE(snmpv3_priv_protocol,''), snmpv3_priv_secret_ciphertext,
COALESCE(verify_tls,FALSE), COALESCE(server_name_override,''),
COALESCE(ssh_host_key_policy,'insecure_ignore'), COALESCE(ssh_host_key_fingerprint,''),
COALESCE(ca_certificate_pem,'')`

func scanView(row pgx.Row) (*View, error) {
	var v View
	var port *int
	var secret []byte
	var snmpAuthSecret, snmpPrivSecret []byte
	var caPEM string
	if err := row.Scan(
		&v.ID, &v.Name, &v.AuthType, &v.Username, &port, &secret, &v.Notes, &v.CreatedAt, &v.UpdatedAt,
		&v.SNMPv3Username, &v.SNMPv3SecurityLevel, &v.SNMPv3AuthProtocol, &snmpAuthSecret,
		&v.SNMPv3PrivProtocol, &snmpPrivSecret, &v.VerifyTLS, &v.ServerNameOverride,
		&v.SSHHostKeyPolicy, &v.SSHHostKeyFingerprint, &caPEM,
	); err != nil {
		return nil, err
	}
	v.Port = port
	v.SecretSet = len(secret) > 0
	v.SNMPv3AuthSet = len(snmpAuthSecret) > 0
	v.SNMPv3PrivSet = len(snmpPrivSecret) > 0
	v.CACertificateSet = caPEM != ""
	return &v, nil
}

// List returns all credential profiles.
func (r *Repository) List(ctx context.Context) ([]View, error) {
	rows, err := r.P.Query(ctx, `SELECT `+viewCols+` FROM credential_profiles ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]View, 0)
	for rows.Next() {
		v, err := scanView(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

// Get returns a single profile (no secret).
func (r *Repository) Get(ctx context.Context, id string) (*View, error) {
	row := r.P.QueryRow(ctx, `SELECT `+viewCols+` FROM credential_profiles WHERE id = $1`, id)
	v, err := scanView(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return v, err
}

// CreateInput holds a new credential profile request.
type CreateInput struct {
	Name     string
	AuthType AuthType
	Username string
	Secret   string
	Port     *int
	Notes    string

	// Phase 4 transport hardening + SNMPv3
	SNMPv3Username        string
	SNMPv3SecurityLevel   string
	SNMPv3AuthProtocol    string
	SNMPv3AuthSecret      string
	SNMPv3PrivProtocol    string
	SNMPv3PrivSecret      string
	VerifyTLS             bool
	ServerNameOverride    string
	CACertificatePEM      string
	SSHHostKeyPolicy      string
	SSHHostKeyFingerprint string
}

// Create stores a new profile, encrypting any provided secret(s).
func (r *Repository) Create(ctx context.Context, in CreateInput) (*View, error) {
	if strings.TrimSpace(in.Name) == "" {
		return nil, &ErrValidation{Msg: "name is required"}
	}
	if !IsValidAuthType(in.AuthType) {
		return nil, &ErrValidation{Msg: "auth_type invalid"}
	}
	if in.SSHHostKeyPolicy == "" {
		in.SSHHostKeyPolicy = "insecure_ignore"
	}

	var ct []byte
	if in.Secret != "" {
		var err error
		ct, err = r.Vault.Encrypt(in.Secret)
		if err != nil {
			return nil, err
		}
	}
	var authCT, privCT []byte
	if in.SNMPv3AuthSecret != "" {
		var err error
		authCT, err = r.Vault.Encrypt(in.SNMPv3AuthSecret)
		if err != nil {
			return nil, err
		}
	}
	if in.SNMPv3PrivSecret != "" {
		var err error
		privCT, err = r.Vault.Encrypt(in.SNMPv3PrivSecret)
		if err != nil {
			return nil, err
		}
	}
	row := r.P.QueryRow(ctx, `
INSERT INTO credential_profiles(
  name, auth_type, username, port, secret_ciphertext, secret_key_id, notes,
  snmpv3_username, snmpv3_security_level, snmpv3_auth_protocol,
  snmpv3_auth_secret_ciphertext, snmpv3_priv_protocol, snmpv3_priv_secret_ciphertext,
  snmpv3_secret_key_id, verify_tls, server_name_override,
  ssh_host_key_policy, ssh_host_key_fingerprint, ca_certificate_pem
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)
RETURNING `+viewCols,
		in.Name, string(in.AuthType), nilIfEmpty(in.Username), in.Port, ct,
		stringPtr(r.Vault.KeyID()), nilIfEmpty(in.Notes),
		nilIfEmpty(in.SNMPv3Username), nilIfEmpty(in.SNMPv3SecurityLevel),
		nilIfEmpty(in.SNMPv3AuthProtocol), authCT,
		nilIfEmpty(in.SNMPv3PrivProtocol), privCT,
		stringPtr(r.Vault.KeyID()), in.VerifyTLS, nilIfEmpty(in.ServerNameOverride),
		in.SSHHostKeyPolicy, nilIfEmpty(in.SSHHostKeyFingerprint),
		nilIfEmpty(in.CACertificatePEM),
	)
	return scanView(row)
}

// UpdateInput allows partial updates including secret rotation.
type UpdateInput struct {
	Name     *string
	AuthType *AuthType
	Username *string
	Secret   *string
	Port     *int
	Notes    *string

	SNMPv3Username        *string
	SNMPv3SecurityLevel   *string
	SNMPv3AuthProtocol    *string
	SNMPv3AuthSecret      *string
	SNMPv3PrivProtocol    *string
	SNMPv3PrivSecret      *string
	VerifyTLS             *bool
	ServerNameOverride    *string
	CACertificatePEM      *string
	SSHHostKeyPolicy      *string
	SSHHostKeyFingerprint *string
}

func (r *Repository) Update(ctx context.Context, id string, in UpdateInput) (*View, error) {
	if in.AuthType != nil && !IsValidAuthType(*in.AuthType) {
		return nil, &ErrValidation{Msg: "auth_type invalid"}
	}
	q := `UPDATE credential_profiles SET rotated_at = now()`
	args := []any{}
	add := func(col string, val any) {
		args = append(args, val)
		q += ", " + col + " = $" + itoa(len(args))
	}
	if in.Name != nil {
		add("name", *in.Name)
	}
	if in.AuthType != nil {
		add("auth_type", string(*in.AuthType))
	}
	if in.Username != nil {
		add("username", *in.Username)
	}
	if in.Port != nil {
		add("port", *in.Port)
	}
	if in.Notes != nil {
		add("notes", *in.Notes)
	}
	if in.Secret != nil {
		if *in.Secret == "" {
			add("secret_ciphertext", nil)
			add("secret_key_id", nil)
		} else {
			ct, err := r.Vault.Encrypt(*in.Secret)
			if err != nil {
				return nil, err
			}
			add("secret_ciphertext", ct)
			add("secret_key_id", r.Vault.KeyID())
		}
	}
	if in.SNMPv3Username != nil {
		add("snmpv3_username", *in.SNMPv3Username)
	}
	if in.SNMPv3SecurityLevel != nil {
		add("snmpv3_security_level", *in.SNMPv3SecurityLevel)
	}
	if in.SNMPv3AuthProtocol != nil {
		add("snmpv3_auth_protocol", *in.SNMPv3AuthProtocol)
	}
	if in.SNMPv3AuthSecret != nil {
		if *in.SNMPv3AuthSecret == "" {
			add("snmpv3_auth_secret_ciphertext", nil)
		} else {
			ct, err := r.Vault.Encrypt(*in.SNMPv3AuthSecret)
			if err != nil {
				return nil, err
			}
			add("snmpv3_auth_secret_ciphertext", ct)
		}
	}
	if in.SNMPv3PrivProtocol != nil {
		add("snmpv3_priv_protocol", *in.SNMPv3PrivProtocol)
	}
	if in.SNMPv3PrivSecret != nil {
		if *in.SNMPv3PrivSecret == "" {
			add("snmpv3_priv_secret_ciphertext", nil)
		} else {
			ct, err := r.Vault.Encrypt(*in.SNMPv3PrivSecret)
			if err != nil {
				return nil, err
			}
			add("snmpv3_priv_secret_ciphertext", ct)
		}
	}
	if in.VerifyTLS != nil {
		add("verify_tls", *in.VerifyTLS)
	}
	if in.ServerNameOverride != nil {
		add("server_name_override", *in.ServerNameOverride)
	}
	if in.CACertificatePEM != nil {
		if *in.CACertificatePEM == "" {
			add("ca_certificate_pem", nil)
		} else {
			add("ca_certificate_pem", *in.CACertificatePEM)
		}
	}
	if in.SSHHostKeyPolicy != nil {
		add("ssh_host_key_policy", *in.SSHHostKeyPolicy)
	}
	if in.SSHHostKeyFingerprint != nil {
		add("ssh_host_key_fingerprint", *in.SSHHostKeyFingerprint)
	}
	args = append(args, id)
	q += " WHERE id = $" + itoa(len(args)) + " RETURNING " + viewCols
	v, err := scanView(r.P.QueryRow(ctx, q, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return v, err
}

func (r *Repository) Delete(ctx context.Context, id string) error {
	cmd, err := r.P.Exec(ctx, `DELETE FROM credential_profiles WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// helpers

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func stringPtr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	a := n / 10
	b := n % 10
	return string(rune('0'+a)) + string(rune('0'+b))
}
