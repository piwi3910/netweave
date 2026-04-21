// Package database provides PostgreSQL database connectivity for the netweave gateway.
// It manages connection pooling via pgxpool and handles schema migrations.
package database

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
)

// Supported sslmode values for libpq/pgx.
const (
	SSLModeDisable    = "disable"
	SSLModePrefer     = "prefer"
	SSLModeRequire    = "require"
	SSLModeVerifyCA   = "verify-ca"
	SSLModeVerifyFull = "verify-full"
)

// DefaultSSLMode is the fallback mode used when none is configured.
// verify-full rejects plaintext fallback and validates the server cert against
// the trust store and hostname; this is the correct default for a multi-tenant
// telco control plane storing credentials and audit records.
const DefaultSSLMode = SSLModeVerifyFull

// ErrInsecureSSLModeInProduction is returned when the operator configured
// sslmode=disable or sslmode=prefer while the gateway is running in production.
// Both modes allow the client to fall through to a plaintext connection, which
// is unacceptable for production data.
var ErrInsecureSSLModeInProduction = errors.New(
	"postgres sslmode 'disable' and 'prefer' are not permitted in production; " +
		"use 'require', 'verify-ca', or 'verify-full'",
)

// PostgresConfig holds PostgreSQL connection parameters.
type PostgresConfig struct {
	// Host is the database server hostname.
	Host string `mapstructure:"host"`

	// Port is the database server port.
	Port int `mapstructure:"port"`

	// Database is the database name.
	Database string `mapstructure:"database"`

	// User is the database user.
	User string `mapstructure:"user"`

	// PasswordEnvVar is the environment variable name containing the password.
	PasswordEnvVar string `mapstructure:"password_env_var"`

	// SSLMode is the SSL connection mode
	// (disable, prefer, require, verify-ca, verify-full).
	// Defaults to verify-full.
	SSLMode string `mapstructure:"ssl_mode"`

	// SSLRootCert is an optional path to the PEM-encoded trust anchor used
	// by verify-ca and verify-full. If empty, the system trust store is used.
	SSLRootCert string `mapstructure:"ssl_root_cert"`

	// MaxConns is the maximum number of connections in the pool.
	MaxConns int32 `mapstructure:"max_conns"`

	// MinConns is the minimum number of idle connections in the pool.
	MinConns int32 `mapstructure:"min_conns"`
}

// ConnectionString builds a PostgreSQL DSN from the config fields.
//
// The password is passed separately (it is read from an environment variable
// and never stored on the struct). It is URL-encoded via url.UserPassword so
// passwords containing '@', '/', '?', '#', '%', ':' or other reserved
// characters do not corrupt the DSN or allow parameter injection.
func (c *PostgresConfig) ConnectionString(password string) string {
	sslMode := c.SSLMode
	if sslMode == "" {
		sslMode = DefaultSSLMode
	}
	port := c.Port
	if port == 0 {
		port = 5432
	}
	hostPort := net.JoinHostPort(c.Host, strconv.Itoa(port))

	query := url.Values{}
	query.Set("sslmode", sslMode)
	if c.SSLRootCert != "" {
		query.Set("sslrootcert", c.SSLRootCert)
	}

	u := &url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(c.User, password),
		Host:     hostPort,
		Path:     "/" + c.Database,
		RawQuery: query.Encode(),
	}
	return u.String()
}

// Validate checks that required fields are set.
func (c *PostgresConfig) Validate() error {
	if c.Host == "" {
		return fmt.Errorf("postgres host is required")
	}
	if c.Database == "" {
		return fmt.Errorf("postgres database is required")
	}
	if c.User == "" {
		return fmt.Errorf("postgres user is required")
	}
	if c.PasswordEnvVar == "" {
		return fmt.Errorf("postgres password_env_var is required")
	}
	return nil
}

// ValidateForProduction extends Validate with stricter rules that must hold
// when the gateway runs in production. It rejects insecure sslmode settings.
func (c *PostgresConfig) ValidateForProduction() error {
	if err := c.Validate(); err != nil {
		return err
	}
	switch c.SSLMode {
	case "", SSLModeDisable, SSLModePrefer:
		// Empty falls back to DefaultSSLMode at connect-time, but the
		// operator should make the choice explicit in production config.
		if c.SSLMode == SSLModeDisable || c.SSLMode == SSLModePrefer {
			return ErrInsecureSSLModeInProduction
		}
	}
	return nil
}
