package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/piwi3910/netweave/internal/config"
)

// TestKeycloakConfig_GetClientSecret tests the GetClientSecret method.
func TestKeycloakConfig_GetClientSecret(t *testing.T) {
	tests := []struct {
		name        string
		cfg         config.KeycloakConfig
		envVars     map[string]string
		wantSecret  string
		wantErr     bool
		errContains string
	}{
		{
			name: "secret from env var",
			cfg: config.KeycloakConfig{
				ClientSecretEnvVar: "TEST_KC_SECRET",
			},
			envVars:    map[string]string{"TEST_KC_SECRET": "env-secret"},
			wantSecret: "env-secret",
			wantErr:    false,
		},
		{
			name: "env var set but empty returns error",
			cfg: config.KeycloakConfig{
				ClientSecretEnvVar: "TEST_KC_SECRET_EMPTY",
			},
			envVars:     map[string]string{"TEST_KC_SECRET_EMPTY": ""},
			wantErr:     true,
			errContains: "environment variable TEST_KC_SECRET_EMPTY is not set",
		},
		{
			name: "env var not set returns error",
			cfg: config.KeycloakConfig{
				ClientSecretEnvVar: "NONEXISTENT_KC_SECRET",
			},
			wantErr:     true,
			errContains: "environment variable NONEXISTENT_KC_SECRET is not set",
		},
		{
			name: "direct secret when no env var configured",
			cfg: config.KeycloakConfig{
				ClientSecret: "direct-secret",
			},
			wantSecret: "direct-secret",
			wantErr:    false,
		},
		{
			name:       "empty secret when nothing configured",
			cfg:        config.KeycloakConfig{},
			wantSecret: "",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			secret, err := tt.cfg.GetClientSecret()

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantSecret, secret)
			}
		})
	}
}

// TestKeycloakConfig_GetAdminPassword tests the GetAdminPassword method.
func TestKeycloakConfig_GetAdminPassword(t *testing.T) {
	tests := []struct {
		name        string
		cfg         config.KeycloakConfig
		envVars     map[string]string
		wantPwd     string
		wantErr     bool
		errContains string
	}{
		{
			name: "password from env var",
			cfg: config.KeycloakConfig{
				AdminPasswordEnvVar: "TEST_KC_ADMIN_PWD",
			},
			envVars: map[string]string{"TEST_KC_ADMIN_PWD": "env-admin-password"},
			wantPwd: "env-admin-password",
			wantErr: false,
		},
		{
			name: "env var set but empty returns error",
			cfg: config.KeycloakConfig{
				AdminPasswordEnvVar: "TEST_KC_ADMIN_PWD_EMPTY",
			},
			envVars:     map[string]string{"TEST_KC_ADMIN_PWD_EMPTY": ""},
			wantErr:     true,
			errContains: "environment variable TEST_KC_ADMIN_PWD_EMPTY is not set",
		},
		{
			name: "env var not set returns error",
			cfg: config.KeycloakConfig{
				AdminPasswordEnvVar: "NONEXISTENT_KC_ADMIN_PWD",
			},
			wantErr:     true,
			errContains: "environment variable NONEXISTENT_KC_ADMIN_PWD is not set",
		},
		{
			name: "direct password when no env var configured",
			cfg: config.KeycloakConfig{
				AdminPassword: "direct-admin-password",
			},
			wantPwd: "direct-admin-password",
			wantErr: false,
		},
		{
			name:    "empty password when nothing configured",
			cfg:     config.KeycloakConfig{},
			wantPwd: "",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			pwd, err := tt.cfg.GetAdminPassword()

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantPwd, pwd)
			}
		})
	}
}

// TestValidateProductionRules tests production environment validation rules.
func TestValidateProductionRules(t *testing.T) {
	tmpDir := t.TempDir()
	certFile := filepath.Join(tmpDir, "cert.pem")
	keyFile := filepath.Join(tmpDir, "key.pem")
	caFile := filepath.Join(tmpDir, "ca.pem")
	require.NoError(t, os.WriteFile(certFile, []byte("cert"), 0600))
	require.NoError(t, os.WriteFile(keyFile, []byte("key"), 0600))
	require.NoError(t, os.WriteFile(caFile, []byte("ca"), 0600))

	tests := []struct {
		name        string
		cfg         *config.Config
		wantErr     bool
		errContains string
	}{
		{
			name: "production without TLS fails",
			cfg: &config.Config{
				Server: config.ServerConfig{
					Port: 8080, O2Port: 8443, TMFPort: 8444, GraphQLPort: 8445, GinMode: "release",
				},
				Redis:       config.RedisConfig{Mode: "standalone", Addresses: []string{"localhost:6379"}},
				Auth:        config.AuthConfig{Backend: "redis"},
				Environment: "prod",
				TLS:         config.TLSConfig{Enabled: false},
				Observability: config.ObservabilityConfig{
					Logging: config.LoggingConfig{Level: "info", Format: "json"},
				},
				Security: config.SecurityConfig{RateLimitEnabled: true},
			},
			wantErr:     true,
			errContains: "TLS must be enabled in production",
		},
		{
			name: "production without mTLS require-and-verify fails",
			cfg: &config.Config{
				Server: config.ServerConfig{
					Port: 8080, O2Port: 8443, TMFPort: 8444, GraphQLPort: 8445, GinMode: "release",
				},
				Redis:       config.RedisConfig{Mode: "standalone", Addresses: []string{"localhost:6379"}},
				Auth:        config.AuthConfig{Backend: "redis"},
				Environment: "prod",
				TLS: config.TLSConfig{
					Enabled: true, CertFile: certFile, KeyFile: keyFile,
					CAFile: caFile, ClientAuth: "require", MinVersion: "1.3",
				},
				Observability: config.ObservabilityConfig{
					Logging: config.LoggingConfig{Level: "info", Format: "json"},
				},
				Security: config.SecurityConfig{RateLimitEnabled: true},
			},
			wantErr:     true,
			errContains: "mTLS with require-and-verify must be enabled in production",
		},
		{
			name: "production without rate limiting fails",
			cfg: &config.Config{
				Server: config.ServerConfig{
					Port: 8080, O2Port: 8443, TMFPort: 8444, GraphQLPort: 8445, GinMode: "release",
				},
				Redis:       config.RedisConfig{Mode: "standalone", Addresses: []string{"localhost:6379"}},
				Auth:        config.AuthConfig{Backend: "redis"},
				Environment: "prod",
				TLS: config.TLSConfig{
					Enabled: true, CertFile: certFile, KeyFile: keyFile,
					CAFile: caFile, ClientAuth: "require-and-verify", MinVersion: "1.3",
				},
				Observability: config.ObservabilityConfig{
					Logging: config.LoggingConfig{Level: "info", Format: "json"},
				},
				Security: config.SecurityConfig{RateLimitEnabled: false},
			},
			wantErr:     true,
			errContains: "rate limiting must be enabled in production",
		},
		{
			name: "production with development logging fails",
			cfg: &config.Config{
				Server: config.ServerConfig{
					Port: 8080, O2Port: 8443, TMFPort: 8444, GraphQLPort: 8445, GinMode: "release",
				},
				Redis:       config.RedisConfig{Mode: "standalone", Addresses: []string{"localhost:6379"}},
				Auth:        config.AuthConfig{Backend: "redis"},
				Environment: "prod",
				TLS: config.TLSConfig{
					Enabled: true, CertFile: certFile, KeyFile: keyFile,
					CAFile: caFile, ClientAuth: "require-and-verify", MinVersion: "1.3",
				},
				Observability: config.ObservabilityConfig{
					Logging: config.LoggingConfig{Level: "info", Format: "json", Development: true},
				},
				Security: config.SecurityConfig{RateLimitEnabled: true},
			},
			wantErr:     true,
			errContains: "logging development mode must be disabled in production",
		},
		{
			name: "production with debug logging fails",
			cfg: &config.Config{
				Server: config.ServerConfig{
					Port: 8080, O2Port: 8443, TMFPort: 8444, GraphQLPort: 8445, GinMode: "release",
				},
				Redis:       config.RedisConfig{Mode: "standalone", Addresses: []string{"localhost:6379"}},
				Auth:        config.AuthConfig{Backend: "redis"},
				Environment: "prod",
				TLS: config.TLSConfig{
					Enabled: true, CertFile: certFile, KeyFile: keyFile,
					CAFile: caFile, ClientAuth: "require-and-verify", MinVersion: "1.3",
				},
				Observability: config.ObservabilityConfig{
					Logging: config.LoggingConfig{Level: "debug", Format: "json"},
				},
				Security: config.SecurityConfig{RateLimitEnabled: true},
			},
			wantErr:     true,
			errContains: "debug logging level is not recommended for production",
		},
		{
			name: "production with response validation fails",
			cfg: &config.Config{
				Server: config.ServerConfig{
					Port: 8080, O2Port: 8443, TMFPort: 8444, GraphQLPort: 8445, GinMode: "release",
				},
				Redis:       config.RedisConfig{Mode: "standalone", Addresses: []string{"localhost:6379"}},
				Auth:        config.AuthConfig{Backend: "redis"},
				Environment: "prod",
				TLS: config.TLSConfig{
					Enabled: true, CertFile: certFile, KeyFile: keyFile,
					CAFile: caFile, ClientAuth: "require-and-verify", MinVersion: "1.3",
				},
				Observability: config.ObservabilityConfig{
					Logging: config.LoggingConfig{Level: "info", Format: "json"},
				},
				Security:   config.SecurityConfig{RateLimitEnabled: true},
				Validation: config.ValidationConfig{ValidateResponse: true},
			},
			wantErr:     true,
			errContains: "response validation should be disabled in production",
		},
		{
			name: "production CORS enabled without origins fails",
			cfg: &config.Config{
				Server: config.ServerConfig{
					Port: 8080, O2Port: 8443, TMFPort: 8444, GraphQLPort: 8445, GinMode: "release",
				},
				Redis:       config.RedisConfig{Mode: "standalone", Addresses: []string{"localhost:6379"}},
				Auth:        config.AuthConfig{Backend: "redis"},
				Environment: "prod",
				TLS: config.TLSConfig{
					Enabled: true, CertFile: certFile, KeyFile: keyFile,
					CAFile: caFile, ClientAuth: "require-and-verify", MinVersion: "1.3",
				},
				Observability: config.ObservabilityConfig{
					Logging: config.LoggingConfig{Level: "info", Format: "json"},
				},
				Security: config.SecurityConfig{
					RateLimitEnabled: true,
					EnableCORS:       true,
					AllowedOrigins:   []string{},
				},
			},
			wantErr:     true,
			errContains: "CORS enabled in production but allowed_origins is empty",
		},
		{
			name: "valid production config passes",
			cfg: &config.Config{
				Server: config.ServerConfig{
					Port: 8080, O2Port: 8443, TMFPort: 8444, GraphQLPort: 8445, GinMode: "release",
				},
				Redis:       config.RedisConfig{Mode: "standalone", Addresses: []string{"localhost:6379"}},
				Auth:        config.AuthConfig{Backend: "redis"},
				Environment: "prod",
				TLS: config.TLSConfig{
					Enabled: true, CertFile: certFile, KeyFile: keyFile,
					CAFile: caFile, ClientAuth: "require-and-verify", MinVersion: "1.3",
				},
				Observability: config.ObservabilityConfig{
					Logging: config.LoggingConfig{Level: "info", Format: "json"},
				},
				Security: config.SecurityConfig{RateLimitEnabled: true},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestValidateStagingRules tests staging environment validation rules.
func TestValidateStagingRules(t *testing.T) {
	tmpDir := t.TempDir()
	certFile := filepath.Join(tmpDir, "cert.pem")
	keyFile := filepath.Join(tmpDir, "key.pem")
	require.NoError(t, os.WriteFile(certFile, []byte("cert"), 0600))
	require.NoError(t, os.WriteFile(keyFile, []byte("key"), 0600))

	tests := []struct {
		name        string
		cfg         *config.Config
		wantErr     bool
		errContains string
	}{
		{
			name: "staging without TLS fails",
			cfg: &config.Config{
				Server: config.ServerConfig{
					Port: 8080, O2Port: 8443, TMFPort: 8444, GraphQLPort: 8445, GinMode: "release",
				},
				Redis:       config.RedisConfig{Mode: "standalone", Addresses: []string{"localhost:6379"}},
				Auth:        config.AuthConfig{Backend: "redis"},
				Environment: "staging",
				TLS:         config.TLSConfig{Enabled: false},
				Observability: config.ObservabilityConfig{
					Logging: config.LoggingConfig{Level: "info", Format: "json"},
				},
				Security: config.SecurityConfig{RateLimitEnabled: true},
			},
			wantErr:     true,
			errContains: "TLS should be enabled in staging",
		},
		{
			name: "staging without rate limiting fails",
			cfg: &config.Config{
				Server: config.ServerConfig{
					Port: 8080, O2Port: 8443, TMFPort: 8444, GraphQLPort: 8445, GinMode: "release",
				},
				Redis:       config.RedisConfig{Mode: "standalone", Addresses: []string{"localhost:6379"}},
				Auth:        config.AuthConfig{Backend: "redis"},
				Environment: "staging",
				TLS: config.TLSConfig{
					Enabled: true, CertFile: certFile, KeyFile: keyFile,
					ClientAuth: "none", MinVersion: "1.3",
				},
				Observability: config.ObservabilityConfig{
					Logging: config.LoggingConfig{Level: "info", Format: "json"},
				},
				Security: config.SecurityConfig{RateLimitEnabled: false},
			},
			wantErr:     true,
			errContains: "rate limiting should be enabled in staging",
		},
		{
			name: "valid staging config passes",
			cfg: &config.Config{
				Server: config.ServerConfig{
					Port: 8080, O2Port: 8443, TMFPort: 8444, GraphQLPort: 8445, GinMode: "release",
				},
				Redis:       config.RedisConfig{Mode: "standalone", Addresses: []string{"localhost:6379"}},
				Auth:        config.AuthConfig{Backend: "redis"},
				Environment: "staging",
				TLS: config.TLSConfig{
					Enabled: true, CertFile: certFile, KeyFile: keyFile,
					ClientAuth: "none", MinVersion: "1.3",
				},
				Observability: config.ObservabilityConfig{
					Logging: config.LoggingConfig{Level: "info", Format: "json"},
				},
				Security: config.SecurityConfig{RateLimitEnabled: true},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestDetectEnvironment tests environment detection from config path and env vars.
func TestDetectEnvironment(t *testing.T) {
	tests := []struct {
		name         string
		configYAML   string
		envVars      map[string]string
		wantEnv      string
		configSuffix string
	}{
		{
			name: "NETWEAVE_ENV overrides detection",
			configYAML: `
server:
  port: 8080
redis:
  addresses:
    - localhost:6379
`,
			envVars: map[string]string{"NETWEAVE_ENV": "staging"},
			wantEnv: "staging",
		},
		{
			name: "default environment is dev",
			configYAML: `
server:
  port: 8080
redis:
  addresses:
    - localhost:6379
`,
			wantEnv: "dev",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			suffix := "config.yaml"
			if tt.configSuffix != "" {
				suffix = tt.configSuffix
			}
			configPath := filepath.Join(tmpDir, suffix)
			err := os.WriteFile(configPath, []byte(tt.configYAML), 0600)
			require.NoError(t, err)

			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			cfg, err := config.Load(configPath)
			require.NoError(t, err)
			assert.Equal(t, tt.wantEnv, cfg.Environment)
		})
	}
}

// TestValidateGlobalRateLimit tests global rate limit validation.
func TestValidateGlobalRateLimit(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *config.Config
		wantErr     bool
		errContains string
	}{
		{
			name: "negative global requests per second",
			cfg: &config.Config{
				Server: config.ServerConfig{
					Port: 8080, O2Port: 8443, TMFPort: 8444, GraphQLPort: 8445, GinMode: "release",
				},
				Redis: config.RedisConfig{Mode: "standalone", Addresses: []string{"localhost:6379"}},
				Auth:  config.AuthConfig{Backend: "redis"},
				Observability: config.ObservabilityConfig{
					Logging: config.LoggingConfig{Level: "info", Format: "json"},
				},
				Security: config.SecurityConfig{
					RateLimitEnabled: true,
					RateLimit: config.RateLimitConfig{
						Global: config.GlobalRateLimitConfig{
							RequestsPerSecond: -1,
						},
					},
				},
			},
			wantErr:     true,
			errContains: "invalid global requests_per_second",
		},
		{
			name: "negative global max concurrent requests",
			cfg: &config.Config{
				Server: config.ServerConfig{
					Port: 8080, O2Port: 8443, TMFPort: 8444, GraphQLPort: 8445, GinMode: "release",
				},
				Redis: config.RedisConfig{Mode: "standalone", Addresses: []string{"localhost:6379"}},
				Auth:  config.AuthConfig{Backend: "redis"},
				Observability: config.ObservabilityConfig{
					Logging: config.LoggingConfig{Level: "info", Format: "json"},
				},
				Security: config.SecurityConfig{
					RateLimitEnabled: true,
					RateLimit: config.RateLimitConfig{
						Global: config.GlobalRateLimitConfig{
							MaxConcurrentRequests: -1,
						},
					},
				},
			},
			wantErr:     true,
			errContains: "invalid global max_concurrent_requests",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestValidateEndpointRateLimits tests per-endpoint rate limit validation.
func TestValidateEndpointRateLimits(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *config.Config
		wantErr     bool
		errContains string
	}{
		{
			name: "endpoint with empty path",
			cfg: &config.Config{
				Server: config.ServerConfig{
					Port: 8080, O2Port: 8443, TMFPort: 8444, GraphQLPort: 8445, GinMode: "release",
				},
				Redis: config.RedisConfig{Mode: "standalone", Addresses: []string{"localhost:6379"}},
				Auth:  config.AuthConfig{Backend: "redis"},
				Observability: config.ObservabilityConfig{
					Logging: config.LoggingConfig{Level: "info", Format: "json"},
				},
				Security: config.SecurityConfig{
					RateLimitEnabled: true,
					RateLimit: config.RateLimitConfig{
						PerEndpoint: []config.EndpointRateLimitConfig{
							{Path: "", Method: "GET", RequestsPerSecond: 10, BurstSize: 20},
						},
					},
				},
			},
			wantErr:     true,
			errContains: "endpoint[0] path cannot be empty",
		},
		{
			name: "endpoint with empty method",
			cfg: &config.Config{
				Server: config.ServerConfig{
					Port: 8080, O2Port: 8443, TMFPort: 8444, GraphQLPort: 8445, GinMode: "release",
				},
				Redis: config.RedisConfig{Mode: "standalone", Addresses: []string{"localhost:6379"}},
				Auth:  config.AuthConfig{Backend: "redis"},
				Observability: config.ObservabilityConfig{
					Logging: config.LoggingConfig{Level: "info", Format: "json"},
				},
				Security: config.SecurityConfig{
					RateLimitEnabled: true,
					RateLimit: config.RateLimitConfig{
						PerEndpoint: []config.EndpointRateLimitConfig{
							{Path: "/api/v1", Method: "", RequestsPerSecond: 10, BurstSize: 20},
						},
					},
				},
			},
			wantErr:     true,
			errContains: "endpoint[0] method cannot be empty",
		},
		{
			name: "endpoint with negative requests per second",
			cfg: &config.Config{
				Server: config.ServerConfig{
					Port: 8080, O2Port: 8443, TMFPort: 8444, GraphQLPort: 8445, GinMode: "release",
				},
				Redis: config.RedisConfig{Mode: "standalone", Addresses: []string{"localhost:6379"}},
				Auth:  config.AuthConfig{Backend: "redis"},
				Observability: config.ObservabilityConfig{
					Logging: config.LoggingConfig{Level: "info", Format: "json"},
				},
				Security: config.SecurityConfig{
					RateLimitEnabled: true,
					RateLimit: config.RateLimitConfig{
						PerEndpoint: []config.EndpointRateLimitConfig{
							{Path: "/api/v1", Method: "GET", RequestsPerSecond: -1, BurstSize: 20},
						},
					},
				},
			},
			wantErr:     true,
			errContains: "endpoint[0] requests_per_second",
		},
		{
			name: "endpoint with negative burst size",
			cfg: &config.Config{
				Server: config.ServerConfig{
					Port: 8080, O2Port: 8443, TMFPort: 8444, GraphQLPort: 8445, GinMode: "release",
				},
				Redis: config.RedisConfig{Mode: "standalone", Addresses: []string{"localhost:6379"}},
				Auth:  config.AuthConfig{Backend: "redis"},
				Observability: config.ObservabilityConfig{
					Logging: config.LoggingConfig{Level: "info", Format: "json"},
				},
				Security: config.SecurityConfig{
					RateLimitEnabled: true,
					RateLimit: config.RateLimitConfig{
						PerEndpoint: []config.EndpointRateLimitConfig{
							{Path: "/api/v1", Method: "GET", RequestsPerSecond: 10, BurstSize: -1},
						},
					},
				},
			},
			wantErr:     true,
			errContains: "endpoint[0] burst_size",
		},
		{
			name: "valid endpoint rate limit config",
			cfg: &config.Config{
				Server: config.ServerConfig{
					Port: 8080, O2Port: 8443, TMFPort: 8444, GraphQLPort: 8445, GinMode: "release",
				},
				Redis: config.RedisConfig{Mode: "standalone", Addresses: []string{"localhost:6379"}},
				Auth:  config.AuthConfig{Backend: "redis"},
				Observability: config.ObservabilityConfig{
					Logging: config.LoggingConfig{Level: "info", Format: "json"},
				},
				Security: config.SecurityConfig{
					RateLimitEnabled: true,
					RateLimit: config.RateLimitConfig{
						PerEndpoint: []config.EndpointRateLimitConfig{
							{Path: "/api/v1", Method: "GET", RequestsPerSecond: 10, BurstSize: 20},
						},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestValidateStorageMode tests storage mode validation.
func TestValidateStorageMode(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *config.Config
		wantErr     bool
		errContains string
	}{
		{
			name: "invalid storage mode",
			cfg: &config.Config{
				Server: config.ServerConfig{
					Port: 8080, O2Port: 8443, TMFPort: 8444, GraphQLPort: 8445, GinMode: "release",
				},
				Redis:       config.RedisConfig{Mode: "standalone", Addresses: []string{"localhost:6379"}},
				Auth:        config.AuthConfig{Backend: "redis"},
				StorageMode: "invalid",
				Observability: config.ObservabilityConfig{
					Logging: config.LoggingConfig{Level: "info", Format: "json"},
				},
			},
			wantErr:     true,
			errContains: "storage_mode",
		},
		{
			name: "redis storage mode valid",
			cfg: &config.Config{
				Server: config.ServerConfig{
					Port: 8080, O2Port: 8443, TMFPort: 8444, GraphQLPort: 8445, GinMode: "release",
				},
				Redis:       config.RedisConfig{Mode: "standalone", Addresses: []string{"localhost:6379"}},
				Auth:        config.AuthConfig{Backend: "redis"},
				StorageMode: "redis",
				Observability: config.ObservabilityConfig{
					Logging: config.LoggingConfig{Level: "info", Format: "json"},
				},
			},
			wantErr: false,
		},
		{
			name: "empty storage mode defaults to redis",
			cfg: &config.Config{
				Server: config.ServerConfig{
					Port: 8080, O2Port: 8443, TMFPort: 8444, GraphQLPort: 8445, GinMode: "release",
				},
				Redis:       config.RedisConfig{Mode: "standalone", Addresses: []string{"localhost:6379"}},
				Auth:        config.AuthConfig{Backend: "redis"},
				StorageMode: "",
				Observability: config.ObservabilityConfig{
					Logging: config.LoggingConfig{Level: "info", Format: "json"},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestValidateAuth tests auth backend validation.
func TestValidateAuth(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *config.Config
		envVars     map[string]string
		wantErr     bool
		errContains string
	}{
		{
			name: "invalid auth backend",
			cfg: &config.Config{
				Server: config.ServerConfig{
					Port: 8080, O2Port: 8443, TMFPort: 8444, GraphQLPort: 8445, GinMode: "release",
				},
				Redis: config.RedisConfig{Mode: "standalone", Addresses: []string{"localhost:6379"}},
				Auth:  config.AuthConfig{Backend: "invalid"},
				Observability: config.ObservabilityConfig{
					Logging: config.LoggingConfig{Level: "info", Format: "json"},
				},
			},
			wantErr:     true,
			errContains: "auth.backend",
		},
		{
			name: "postgres auth backend valid",
			cfg: &config.Config{
				Server: config.ServerConfig{
					Port: 8080, O2Port: 8443, TMFPort: 8444, GraphQLPort: 8445, GinMode: "release",
				},
				Redis: config.RedisConfig{Mode: "standalone", Addresses: []string{"localhost:6379"}},
				Auth:  config.AuthConfig{Backend: "postgres"},
				Observability: config.ObservabilityConfig{
					Logging: config.LoggingConfig{Level: "info", Format: "json"},
				},
			},
			wantErr: false,
		},
		{
			name: "keycloak backend missing base_url",
			cfg: &config.Config{
				Server: config.ServerConfig{
					Port: 8080, O2Port: 8443, TMFPort: 8444, GraphQLPort: 8445, GinMode: "release",
				},
				Redis: config.RedisConfig{Mode: "standalone", Addresses: []string{"localhost:6379"}},
				Auth: config.AuthConfig{
					Backend: "keycloak",
					Keycloak: config.KeycloakConfig{
						Realm:    "myrealm",
						ClientID: "myclient",
					},
				},
				Observability: config.ObservabilityConfig{
					Logging: config.LoggingConfig{Level: "info", Format: "json"},
				},
			},
			wantErr:     true,
			errContains: "auth.keycloak.base_url is required",
		},
		{
			name: "keycloak backend missing realm",
			cfg: &config.Config{
				Server: config.ServerConfig{
					Port: 8080, O2Port: 8443, TMFPort: 8444, GraphQLPort: 8445, GinMode: "release",
				},
				Redis: config.RedisConfig{Mode: "standalone", Addresses: []string{"localhost:6379"}},
				Auth: config.AuthConfig{
					Backend: "keycloak",
					Keycloak: config.KeycloakConfig{
						BaseURL:  "http://localhost:8090",
						ClientID: "myclient",
					},
				},
				Observability: config.ObservabilityConfig{
					Logging: config.LoggingConfig{Level: "info", Format: "json"},
				},
			},
			wantErr:     true,
			errContains: "auth.keycloak.realm is required",
		},
		{
			name: "keycloak backend missing client_id",
			cfg: &config.Config{
				Server: config.ServerConfig{
					Port: 8080, O2Port: 8443, TMFPort: 8444, GraphQLPort: 8445, GinMode: "release",
				},
				Redis: config.RedisConfig{Mode: "standalone", Addresses: []string{"localhost:6379"}},
				Auth: config.AuthConfig{
					Backend: "keycloak",
					Keycloak: config.KeycloakConfig{
						BaseURL: "http://localhost:8090",
						Realm:   "myrealm",
					},
				},
				Observability: config.ObservabilityConfig{
					Logging: config.LoggingConfig{Level: "info", Format: "json"},
				},
			},
			wantErr:     true,
			errContains: "auth.keycloak.client_id is required",
		},
		{
			name: "keycloak backend missing admin_username",
			cfg: &config.Config{
				Server: config.ServerConfig{
					Port: 8080, O2Port: 8443, TMFPort: 8444, GraphQLPort: 8445, GinMode: "release",
				},
				Redis: config.RedisConfig{Mode: "standalone", Addresses: []string{"localhost:6379"}},
				Auth: config.AuthConfig{
					Backend: "keycloak",
					Keycloak: config.KeycloakConfig{
						BaseURL:      "http://localhost:8090",
						Realm:        "myrealm",
						ClientID:     "myclient",
						ClientSecret: "mysecret",
					},
				},
				Observability: config.ObservabilityConfig{
					Logging: config.LoggingConfig{Level: "info", Format: "json"},
				},
			},
			wantErr:     true,
			errContains: "auth.keycloak.admin_username is required",
		},
		{
			name: "keycloak backend with invalid timeout",
			cfg: &config.Config{
				Server: config.ServerConfig{
					Port: 8080, O2Port: 8443, TMFPort: 8444, GraphQLPort: 8445, GinMode: "release",
				},
				Redis: config.RedisConfig{Mode: "standalone", Addresses: []string{"localhost:6379"}},
				Auth: config.AuthConfig{
					Backend: "keycloak",
					Keycloak: config.KeycloakConfig{
						BaseURL:       "http://localhost:8090",
						Realm:         "myrealm",
						ClientID:      "myclient",
						ClientSecret:  "mysecret",
						AdminUsername: "admin",
						AdminPassword: "adminpass",
						Timeout:       0,
					},
				},
				Observability: config.ObservabilityConfig{
					Logging: config.LoggingConfig{Level: "info", Format: "json"},
				},
			},
			wantErr:     true,
			errContains: "auth.keycloak.timeout",
		},
		{
			name: "valid keycloak backend config",
			cfg: &config.Config{
				Server: config.ServerConfig{
					Port: 8080, O2Port: 8443, TMFPort: 8444, GraphQLPort: 8445, GinMode: "release",
				},
				Redis: config.RedisConfig{Mode: "standalone", Addresses: []string{"localhost:6379"}},
				Auth: config.AuthConfig{
					Backend: "keycloak",
					Keycloak: config.KeycloakConfig{
						BaseURL:       "http://localhost:8090",
						Realm:         "myrealm",
						ClientID:      "myclient",
						ClientSecret:  "mysecret",
						AdminUsername: "admin",
						AdminPassword: "adminpass",
						Timeout:       30 * time.Second,
					},
				},
				Observability: config.ObservabilityConfig{
					Logging: config.LoggingConfig{Level: "info", Format: "json"},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			err := tt.cfg.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestValidateInvalidTracingProvider tests invalid tracing provider validation.
func TestValidateInvalidTracingProvider(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 8080, O2Port: 8443, TMFPort: 8444, GraphQLPort: 8445, GinMode: "release",
		},
		Redis: config.RedisConfig{Mode: "standalone", Addresses: []string{"localhost:6379"}},
		Auth:  config.AuthConfig{Backend: "redis"},
		Observability: config.ObservabilityConfig{
			Logging: config.LoggingConfig{Level: "info", Format: "json"},
			Tracing: config.TracingConfig{
				Enabled:  true,
				Provider: "invalid",
				Endpoint: "http://localhost:4318",
			},
		},
	}

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid tracing provider")
}

// TestValidateMetricsEmptyPath tests metrics with empty path validation.
func TestValidateMetricsEmptyPath(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 8080, O2Port: 8443, TMFPort: 8444, GraphQLPort: 8445, GinMode: "release",
		},
		Redis: config.RedisConfig{Mode: "standalone", Addresses: []string{"localhost:6379"}},
		Auth:  config.AuthConfig{Backend: "redis"},
		Observability: config.ObservabilityConfig{
			Logging: config.LoggingConfig{Level: "info", Format: "json"},
			Metrics: config.MetricsConfig{
				Enabled: true,
				Path:    "",
			},
		},
	}

	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "metrics path cannot be empty")
}

// TestValidateRateLimitDisabled tests that validation passes when rate limit is disabled.
func TestValidateRateLimitDisabled(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 8080, O2Port: 8443, TMFPort: 8444, GraphQLPort: 8445, GinMode: "release",
		},
		Redis: config.RedisConfig{Mode: "standalone", Addresses: []string{"localhost:6379"}},
		Auth:  config.AuthConfig{Backend: "redis"},
		Observability: config.ObservabilityConfig{
			Logging: config.LoggingConfig{Level: "info", Format: "json"},
		},
		Security: config.SecurityConfig{
			RateLimitEnabled: false,
		},
	}

	err := cfg.Validate()
	require.NoError(t, err)
}

// TestResolveConfigPath_NETWEAVE_CONFIG tests config resolution via environment variable.
func TestResolveConfigPath_NETWEAVE_CONFIG(t *testing.T) {
	tmpDir := t.TempDir()
	envConfigPath := filepath.Join(tmpDir, "custom-config.yaml")
	err := os.WriteFile(envConfigPath, []byte(`
server:
  port: 9090
redis:
  addresses:
    - localhost:6379
`), 0600)
	require.NoError(t, err)

	t.Setenv("NETWEAVE_CONFIG", envConfigPath)

	cfg, err := config.Load("")
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, 9090, cfg.Server.Port)
}

// TestResolveConfigPath_DirectoryTraversal tests that directory traversal is rejected.
func TestResolveConfigPath_DirectoryTraversal(t *testing.T) {
	t.Setenv("NETWEAVE_CONFIG", "../../../etc/passwd.yaml")

	// Should fall back to defaults since the path is invalid
	cfg, err := config.Load("")
	require.NoError(t, err)
	require.NotNil(t, cfg)
}

// TestResolveConfigPath_InvalidExtension tests that non-YAML files are rejected.
func TestResolveConfigPath_InvalidExtension(t *testing.T) {
	t.Setenv("NETWEAVE_CONFIG", "/tmp/config.txt")

	// Should fall back to defaults since the extension is invalid
	cfg, err := config.Load("")
	require.NoError(t, err)
	require.NotNil(t, cfg)
}

// TestValidateEnvironmentRulesUnknown tests unknown environment defaults to development.
func TestValidateEnvironmentRulesUnknown(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 8080, O2Port: 8443, TMFPort: 8444, GraphQLPort: 8445, GinMode: "release",
		},
		Redis:       config.RedisConfig{Mode: "standalone", Addresses: []string{"localhost:6379"}},
		Auth:        config.AuthConfig{Backend: "redis"},
		Environment: "unknown-env",
		Observability: config.ObservabilityConfig{
			Logging: config.LoggingConfig{Level: "info", Format: "json"},
		},
	}

	err := cfg.Validate()
	require.NoError(t, err)
}

// TestLoadWithAbsolutePathYAML tests loading with absolute yaml path.
func TestLoadWithAbsolutePathYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")
	err := os.WriteFile(configPath, []byte(`
server:
  port: 7070
redis:
  addresses:
    - localhost:6379
`), 0600)
	require.NoError(t, err)

	cfg, err := config.Load(configPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, 7070, cfg.Server.Port)
}

// TestValidateNoDuplicatePorts tests that duplicate server port configurations are rejected.
func TestValidateNoDuplicatePorts(t *testing.T) {
	tests := []struct {
		name        string
		port        int
		o2Port      int
		tmfPort     int
		graphqlPort int
		wantErr     bool
		errContains string
	}{
		{
			name:        "all unique ports passes port validation",
			port:        8080,
			o2Port:      8443,
			tmfPort:     8444,
			graphqlPort: 8445,
			wantErr:     false,
		},
		{
			name:        "admin and o2 duplicate",
			port:        8080,
			o2Port:      8080,
			tmfPort:     8444,
			graphqlPort: 8445,
			wantErr:     true,
			errContains: "duplicate server port 8080",
		},
		{
			name:        "tmf and graphql duplicate",
			port:        8080,
			o2Port:      8443,
			tmfPort:     8444,
			graphqlPort: 8444,
			wantErr:     true,
			errContains: "duplicate server port 8444",
		},
		{
			name:        "all same port",
			port:        9090,
			o2Port:      9090,
			tmfPort:     9090,
			graphqlPort: 9090,
			wantErr:     true,
			errContains: "duplicate server port 9090",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Server: config.ServerConfig{
					Port:        tt.port,
					O2Port:      tt.o2Port,
					TMFPort:     tt.tmfPort,
					GraphQLPort: tt.graphqlPort,
					GinMode:     "release",
				},
				Redis: config.RedisConfig{
					Mode:      "standalone",
					Addresses: []string{"localhost:6379"},
				},
			}

			err := cfg.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				// Validate() may fail on other config sections (logging, etc.)
				// but should NOT fail on duplicate port validation.
				if err != nil {
					assert.NotContains(t, err.Error(), "duplicate server port")
				}
			}
		})
	}
}
