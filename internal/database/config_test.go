package database

import (
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostgresConfig_ConnectionString_DefaultsToVerifyFull(t *testing.T) {
	c := &PostgresConfig{Host: "db.example.com", Database: "netweave", User: "app"}
	dsn := c.ConnectionString("secret")
	u, err := url.Parse(dsn)
	require.NoError(t, err)
	assert.Equal(t, "verify-full", u.Query().Get("sslmode"))
	assert.Equal(t, "5432", u.Port())
	assert.Equal(t, "db.example.com", u.Hostname())
	assert.Equal(t, "/netweave", u.Path)
}

func TestPostgresConfig_ConnectionString_EncodesReservedPasswordChars(t *testing.T) {
	tests := []struct {
		name     string
		password string
	}{
		{"at sign", "p@ssword"},
		{"slash", "pa/ssword"},
		{"question mark", "pa?ssword"},
		{"hash", "pa#ssword"},
		{"percent", "pa%ssword"},
		{"colon", "pa:ssword"},
		{"space", "pa ssword"},
		{"ampersand injection", "pa&sslmode=disable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &PostgresConfig{Host: "h", Database: "d", User: "u", SSLMode: SSLModeRequire}
			dsn := c.ConnectionString(tt.password)

			u, err := url.Parse(dsn)
			require.NoError(t, err, "dsn must remain parseable: %s", dsn)

			gotPW, hasPW := u.User.Password()
			require.True(t, hasPW)
			assert.Equal(t, tt.password, gotPW, "password must round-trip through the URL")

			// The query must still be exactly sslmode=require; no param injection.
			assert.Equal(t, SSLModeRequire, u.Query().Get("sslmode"))
			assert.Equal(t, 1, len(u.Query()))
		})
	}
}

func TestPostgresConfig_ConnectionString_IncludesRootCert(t *testing.T) {
	c := &PostgresConfig{
		Host:        "h",
		Database:    "d",
		User:        "u",
		SSLMode:     SSLModeVerifyFull,
		SSLRootCert: "/etc/ssl/db-ca.pem",
	}
	dsn := c.ConnectionString("pw")
	u, err := url.Parse(dsn)
	require.NoError(t, err)
	assert.Equal(t, "/etc/ssl/db-ca.pem", u.Query().Get("sslrootcert"))
}

func TestPostgresConfig_ConnectionString_IPv6Host(t *testing.T) {
	c := &PostgresConfig{Host: "::1", Database: "d", User: "u", Port: 6543}
	dsn := c.ConnectionString("pw")
	// JoinHostPort brackets IPv6; url.Parse must still succeed.
	assert.True(t, strings.Contains(dsn, "[::1]:6543"), dsn)
	_, err := url.Parse(dsn)
	require.NoError(t, err)
}

func TestPostgresConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *PostgresConfig
		wantErr string
	}{
		{"missing host", &PostgresConfig{Database: "d", User: "u", PasswordEnvVar: "E"}, "host"},
		{"missing database", &PostgresConfig{Host: "h", User: "u", PasswordEnvVar: "E"}, "database"},
		{"missing user", &PostgresConfig{Host: "h", Database: "d", PasswordEnvVar: "E"}, "user"},
		{"missing env var", &PostgresConfig{Host: "h", Database: "d", User: "u"}, "password_env_var"},
		{"ok", &PostgresConfig{Host: "h", Database: "d", User: "u", PasswordEnvVar: "E"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestPostgresConfig_ValidateForProduction_RejectsInsecureSSL(t *testing.T) {
	base := &PostgresConfig{Host: "h", Database: "d", User: "u", PasswordEnvVar: "E"}
	for _, mode := range []string{SSLModeDisable, SSLModePrefer} {
		t.Run(mode, func(t *testing.T) {
			c := *base
			c.SSLMode = mode
			err := c.ValidateForProduction()
			require.Error(t, err)
			assert.True(t, errors.Is(err, ErrInsecureSSLModeInProduction))
		})
	}
}

func TestPostgresConfig_ValidateForProduction_AcceptsSecureSSL(t *testing.T) {
	base := &PostgresConfig{Host: "h", Database: "d", User: "u", PasswordEnvVar: "E"}
	for _, mode := range []string{SSLModeRequire, SSLModeVerifyCA, SSLModeVerifyFull} {
		t.Run(mode, func(t *testing.T) {
			c := *base
			c.SSLMode = mode
			assert.NoError(t, c.ValidateForProduction())
		})
	}
}
