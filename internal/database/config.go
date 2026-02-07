// Package database provides PostgreSQL database connectivity for the netweave gateway.
// It manages connection pooling via pgxpool and handles schema migrations.
package database

import (
	"fmt"
	"net"
	"strconv"
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

	// SSLMode is the SSL connection mode (disable, prefer, require, verify-ca, verify-full).
	SSLMode string `mapstructure:"ssl_mode"`

	// MaxConns is the maximum number of connections in the pool.
	MaxConns int32 `mapstructure:"max_conns"`

	// MinConns is the minimum number of idle connections in the pool.
	MinConns int32 `mapstructure:"min_conns"`
}

// ConnectionString builds a PostgreSQL DSN from the config fields.
// The password is passed separately and not stored in the config struct.
func (c *PostgresConfig) ConnectionString(password string) string {
	sslMode := c.SSLMode
	if sslMode == "" {
		sslMode = "prefer"
	}
	port := c.Port
	if port == 0 {
		port = 5432
	}
	hostPort := net.JoinHostPort(c.Host, strconv.Itoa(port))
	return fmt.Sprintf(
		"postgres://%s:%s@%s/%s?sslmode=%s",
		c.User, password, hostPort, c.Database, sslMode,
	)
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
