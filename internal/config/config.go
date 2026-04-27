// Package config loads runtime configuration from environment
// variables. No secrets are hardcoded.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config groups all settings consumed by API and worker processes.
type Config struct {
	Env      string
	HTTPAddr string

	Database DatabaseConfig
	Redis    RedisConfig
	Auth     AuthConfig
	Vault    VaultConfig
	Log      LogConfig

	MaintenanceMode bool
}

// DatabaseConfig holds PostgreSQL connection parameters.
type DatabaseConfig struct {
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// RedisConfig is reserved for the Asynq-compatible scheduler.
type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

// AuthConfig describes the authentication settings used by the API.
type AuthConfig struct {
	APIToken string
}

// VaultConfig holds the AES-GCM master key for credential profile
// encryption. Phase 2: a 32-byte key is required when the credential
// profile API is exercised. Stored as base64 or hex via env.
type VaultConfig struct {
	Key string
}

// LogConfig controls structured logger behaviour.
type LogConfig struct {
	Level  string
	Format string
}

// Load reads configuration from the environment. Both DATABASE_URL
// and WISP_DATABASE_URL are accepted; WISP_-prefixed names take
// precedence to match the Phase 2 contract.
func Load() (*Config, error) {
	cfg := &Config{
		Env:      firstNonEmpty(os.Getenv("WISP_ENV"), "development"),
		HTTPAddr: firstNonEmpty(os.Getenv("WISP_HTTP_ADDR"), ":8080"),

		Database: DatabaseConfig{
			DSN:             firstNonEmpty(os.Getenv("WISP_DATABASE_URL"), os.Getenv("DATABASE_URL")),
			MaxOpenConns:    envInt("DATABASE_MAX_OPEN_CONNS", 10),
			MaxIdleConns:    envInt("DATABASE_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: envDuration("DATABASE_CONN_MAX_LIFETIME", 30*time.Minute),
		},

		Redis: RedisConfig{
			Addr:     os.Getenv("REDIS_ADDR"),
			Password: os.Getenv("REDIS_PASSWORD"),
			DB:       envInt("REDIS_DB", 0),
		},

		Auth: AuthConfig{
			APIToken: os.Getenv("WISP_API_TOKEN"),
		},

		Vault: VaultConfig{
			Key: os.Getenv("WISP_VAULT_KEY"),
		},

		Log: LogConfig{
			Level:  strings.ToLower(firstNonEmpty(os.Getenv("LOG_LEVEL"), "info")),
			Format: strings.ToLower(firstNonEmpty(os.Getenv("LOG_FORMAT"), "text")),
		},

		MaintenanceMode: envBool("WISP_MAINTENANCE_MODE", false),
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	if c.HTTPAddr == "" {
		return errors.New("WISP_HTTP_ADDR must not be empty")
	}
	switch c.Log.Format {
	case "text", "json":
	default:
		return fmt.Errorf("LOG_FORMAT must be text or json, got %q", c.Log.Format)
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func envBool(key string, def bool) bool {
	v := strings.ToLower(os.Getenv(key))
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}
