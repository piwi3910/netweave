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

// TestLoad tests the Load function with various scenarios.
func TestLoad(t *testing.T) {
	tests := []struct {
		name       string
		configYAML string
		envVars    map[string]string
		wantErr    bool
		validate   func(*testing.T, *config.Config)
	}{
		{
			name: "valid minimal config",
			configYAML: `
server:
  port: 8080
redis:
  addresses:
    - localhost:6379
`,
			wantErr: false,
			validate: func(t *testing.T, cfg *config.Config) {
				t.Helper()
				assert.Equal(t, 8080, cfg.Server.Port)
				assert.Equal(t, "0.0.0.0", cfg.Server.Host)
				assert.Equal(t, []string{"localhost:6379"}, cfg.Redis.Addresses)
			},
		},
		{
			name: "complete config with all options",
			configYAML: `
server:
  host: 127.0.0.1
  port: 9090
  read_timeout: 60s
  write_timeout: 60s
  gin_mode: debug
redis:
  mode: sentinel
  addresses:
    - sentinel1:26379
    - sentinel2:26379
  master_name: mymaster
  password: secret
  db: 1
  pool_size: 20
kubernetes:
  config_path: /home/user/.kube/config
  context: production
  namespace: default
  qps: 100.0
  burst: 200
tls:
  enabled: false
observability:
  logging:
    level: debug
    format: console
  metrics:
    enabled: true
    path: /prometheus
  tracing:
    enabled: false
security:
  enable_cors: true
  rate_limit_enabled: true
  rate_limit:
    tenant:
      requests_per_second: 1000
      burst_size: 2000
`,
			wantErr: false,
			validate: func(t *testing.T, cfg *config.Config) {
				t.Helper()
				assert.Equal(t, "127.0.0.1", cfg.Server.Host)
				assert.Equal(t, 9090, cfg.Server.Port)
				assert.Equal(t, 60*time.Second, cfg.Server.ReadTimeout)
				assert.Equal(t, "debug", cfg.Server.GinMode)

				assert.Equal(t, "sentinel", cfg.Redis.Mode)
				assert.Equal(t, "mymaster", cfg.Redis.MasterName)
				assert.Equal(t, "secret", cfg.Redis.Password)
				assert.Equal(t, 1, cfg.Redis.DB)
				assert.Equal(t, 20, cfg.Redis.PoolSize)

				assert.Equal(t, "/home/user/.kube/config", cfg.Kubernetes.ConfigPath)
				assert.Equal(t, "production", cfg.Kubernetes.Context)
				assert.Equal(t, float32(100.0), cfg.Kubernetes.QPS)

				assert.Equal(t, "debug", cfg.Observability.Logging.Level)
				assert.Equal(t, "console", cfg.Observability.Logging.Format)
				assert.True(t, cfg.Observability.Metrics.Enabled)
				assert.Equal(t, "/prometheus", cfg.Observability.Metrics.Path)

				assert.True(t, cfg.Security.EnableCORS)
				assert.Equal(t, 1000, cfg.Security.RateLimit.PerTenant.RequestsPerSecond)
			},
		},
		{
			name: "environment variable override",
			configYAML: `
server:
  port: 8080
redis:
  addresses:
    - localhost:6379
`,
			envVars: map[string]string{
				"NETWEAVE_SERVER_PORT":                                    "9999",
				"NETWEAVE_OBSERVABILITY_LOGGING_LEVEL":                    "debug",
				"NETWEAVE_REDIS_MODE":                                     "cluster",
				"NETWEAVE_SECURITY_RATE_LIMIT_TENANT_REQUESTS_PER_SECOND": "500",
			},
			wantErr: false,
			validate: func(t *testing.T, cfg *config.Config) {
				t.Helper()
				assert.Equal(t, 9999, cfg.Server.Port)
				assert.Equal(t, "debug", cfg.Observability.Logging.Level)
				assert.Equal(t, "cluster", cfg.Redis.Mode)
				assert.Equal(t, 500, cfg.Security.RateLimit.PerTenant.RequestsPerSecond)
			},
		},
		{
			name: "invalid yaml",
			configYAML: `
server:
  port: not_a_number
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary config file
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.yaml")
			err := os.WriteFile(configPath, []byte(tt.configYAML), 0600)
			require.NoError(t, err)

			// Set environment variables
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			// Load configuration
			cfg, err := config.Load(configPath)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, cfg)

			if tt.validate != nil {
				tt.validate(t, cfg)
			}
		})
	}
}

// TestLoadWithoutConfigFile tests loading with environment variables only.
func TestLoadWithoutConfigFile(t *testing.T) {
	// Set minimum required environment variables
	t.Setenv("NETWEAVE_SERVER_PORT", "8080")
	t.Setenv("NETWEAVE_REDIS_ADDRESSES", "redis:6379")

	cfg, err := config.Load("/nonexistent/config.yaml")

	// Should not error even if file doesn't exist (env vars provide values)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Verify defaults are applied
	assert.Equal(t, "0.0.0.0", cfg.Server.Host)
	assert.Equal(t, 8080, cfg.Server.Port)
}

// TestValidate tests the Validate function with various configurations.
func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *config.Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: &config.Config{
				Server: config.ServerConfig{
					Port:    8080,
					GinMode: "release",
				},
				Redis: config.RedisConfig{
					Mode:      "standalone",
					Addresses: []string{"localhost:6379"},
					DB:        0,
				},
				Observability: config.ObservabilityConfig{
					Logging: config.LoggingConfig{
						Level:  "info",
						Format: "json",
					},
					Metrics: config.MetricsConfig{
						Enabled: true,
						Path:    "/metrics",
						Port:    0,
					},
					Tracing: config.TracingConfig{
						Enabled:      false,
						SamplingRate: 0.1,
					},
				},
				Security: config.SecurityConfig{
					RateLimitEnabled: true,
					RateLimit: config.RateLimitConfig{
						PerTenant: config.TenantRateLimitConfig{
							RequestsPerSecond: 100,
							BurstSize:         200,
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid server port - too low",
			config: &config.Config{
				Server: config.ServerConfig{
					Port:    0,
					GinMode: "release",
				},
				Redis: config.RedisConfig{
					Mode:      "standalone",
					Addresses: []string{"localhost:6379"},
				},
			},
			wantErr: true,
			errMsg:  "invalid server port",
		},
		{
			name: "invalid server port - too high",
			config: &config.Config{
				Server: config.ServerConfig{
					Port:    70000,
					GinMode: "release",
				},
				Redis: config.RedisConfig{
					Mode:      "standalone",
					Addresses: []string{"localhost:6379"},
				},
			},
			wantErr: true,
			errMsg:  "invalid server port",
		},
		{
			name: "invalid gin mode",
			config: &config.Config{
				Server: config.ServerConfig{
					Port:    8080,
					GinMode: "invalid",
				},
				Redis: config.RedisConfig{
					Mode:      "standalone",
					Addresses: []string{"localhost:6379"},
				},
			},
			wantErr: true,
			errMsg:  "invalid gin_mode",
		},
		{
			name: "invalid redis mode",
			config: &config.Config{
				Server: config.ServerConfig{
					Port:    8080,
					GinMode: "release",
				},
				Redis: config.RedisConfig{
					Mode:      "invalid",
					Addresses: []string{"localhost:6379"},
				},
			},
			wantErr: true,
			errMsg:  "invalid redis mode",
		},
		{
			name: "empty redis addresses",
			config: &config.Config{
				Server: config.ServerConfig{
					Port:    8080,
					GinMode: "release",
				},
				Redis: config.RedisConfig{
					Mode:      "standalone",
					Addresses: []string{},
				},
			},
			wantErr: true,
			errMsg:  "redis addresses cannot be empty",
		},
		{
			name: "sentinel mode without master name",
			config: &config.Config{
				Server: config.ServerConfig{
					Port:    8080,
					GinMode: "release",
				},
				Redis: config.RedisConfig{
					Mode:       "sentinel",
					Addresses:  []string{"sentinel:26379"},
					MasterName: "",
				},
			},
			wantErr: true,
			errMsg:  "master_name is required for sentinel mode",
		},
		{
			name: "invalid redis db",
			config: &config.Config{
				Server: config.ServerConfig{
					Port:    8080,
					GinMode: "release",
				},
				Redis: config.RedisConfig{
					Mode:      "standalone",
					Addresses: []string{"localhost:6379"},
					DB:        20,
				},
			},
			wantErr: true,
			errMsg:  "invalid redis db",
		},
		{
			name: "invalid logging level",
			config: &config.Config{
				Server: config.ServerConfig{
					Port:    8080,
					GinMode: "release",
				},
				Redis: config.RedisConfig{
					Mode:      "standalone",
					Addresses: []string{"localhost:6379"},
				},
				Observability: config.ObservabilityConfig{
					Logging: config.LoggingConfig{
						Level:  "invalid",
						Format: "json",
					},
				},
			},
			wantErr: true,
			errMsg:  "invalid logging level",
		},
		{
			name: "invalid logging format",
			config: &config.Config{
				Server: config.ServerConfig{
					Port:    8080,
					GinMode: "release",
				},
				Redis: config.RedisConfig{
					Mode:      "standalone",
					Addresses: []string{"localhost:6379"},
				},
				Observability: config.ObservabilityConfig{
					Logging: config.LoggingConfig{
						Level:  "info",
						Format: "xml",
					},
				},
			},
			wantErr: true,
			errMsg:  "invalid logging format",
		},
		{
			name: "invalid metrics port",
			config: &config.Config{
				Server: config.ServerConfig{
					Port:    8080,
					GinMode: "release",
				},
				Redis: config.RedisConfig{
					Mode:      "standalone",
					Addresses: []string{"localhost:6379"},
				},
				Observability: config.ObservabilityConfig{
					Logging: config.LoggingConfig{
						Level:  "info",
						Format: "json",
					},
					Metrics: config.MetricsConfig{
						Enabled: true,
						Path:    "/metrics",
						Port:    70000,
					},
				},
			},
			wantErr: true,
			errMsg:  "invalid metrics port",
		},
		{
			name: "tracing enabled without endpoint",
			config: &config.Config{
				Server: config.ServerConfig{
					Port:    8080,
					GinMode: "release",
				},
				Redis: config.RedisConfig{
					Mode:      "standalone",
					Addresses: []string{"localhost:6379"},
				},
				Observability: config.ObservabilityConfig{
					Logging: config.LoggingConfig{
						Level:  "info",
						Format: "json",
					},
					Tracing: config.TracingConfig{
						Enabled:  true,
						Provider: "otlp",
						Endpoint: "",
					},
				},
			},
			wantErr: true,
			errMsg:  "tracing endpoint is required",
		},
		{
			name: "invalid tracing sampling rate",
			config: &config.Config{
				Server: config.ServerConfig{
					Port:    8080,
					GinMode: "release",
				},
				Redis: config.RedisConfig{
					Mode:      "standalone",
					Addresses: []string{"localhost:6379"},
				},
				Observability: config.ObservabilityConfig{
					Logging: config.LoggingConfig{
						Level:  "info",
						Format: "json",
					},
					Tracing: config.TracingConfig{
						Enabled:      true,
						Provider:     "otlp",
						Endpoint:     "http://localhost:4318",
						SamplingRate: 1.5,
					},
				},
			},
			wantErr: true,
			errMsg:  "invalid tracing sampling_rate",
		},
		{
			name: "invalid rate limit requests per second",
			config: &config.Config{
				Server: config.ServerConfig{
					Port:    8080,
					GinMode: "release",
				},
				Redis: config.RedisConfig{
					Mode:      "standalone",
					Addresses: []string{"localhost:6379"},
				},
				Observability: config.ObservabilityConfig{
					Logging: config.LoggingConfig{
						Level:  "info",
						Format: "json",
					},
				},
				Security: config.SecurityConfig{
					RateLimitEnabled: true,
					RateLimit: config.RateLimitConfig{
						PerTenant: config.TenantRateLimitConfig{
							RequestsPerSecond: -1,
							BurstSize:         100,
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "invalid tenant requests_per_second",
		},
		{
			name: "invalid rate limit burst size",
			config: &config.Config{
				Server: config.ServerConfig{
					Port:    8080,
					GinMode: "release",
				},
				Redis: config.RedisConfig{
					Mode:      "standalone",
					Addresses: []string{"localhost:6379"},
				},
				Observability: config.ObservabilityConfig{
					Logging: config.LoggingConfig{
						Level:  "info",
						Format: "json",
					},
				},
				Security: config.SecurityConfig{
					RateLimitEnabled: true,
					RateLimit: config.RateLimitConfig{
						PerTenant: config.TenantRateLimitConfig{
							RequestsPerSecond: 100,
							BurstSize:         -1,
						},
					},
				},
			},
			wantErr: true,
			errMsg:  "invalid tenant burst_size",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestValidateTLSConfig tests TLS-specific validation.
func TestValidateTLSConfig(t *testing.T) {
	// Create temporary TLS files for testing
	tmpDir := t.TempDir()
	certFile := filepath.Join(tmpDir, "cert.pem")
	keyFile := filepath.Join(tmpDir, "key.pem")
	caFile := filepath.Join(tmpDir, "ca.pem")

	// Create dummy files
	require.NoError(t, os.WriteFile(certFile, []byte("cert"), 0600))
	require.NoError(t, os.WriteFile(keyFile, []byte("key"), 0600))
	require.NoError(t, os.WriteFile(caFile, []byte("ca"), 0600))

	tests := []struct {
		name    string
		config  *config.Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid TLS config",
			config: &config.Config{
				Server: config.ServerConfig{Port: 8080, GinMode: "release"},
				Redis:  config.RedisConfig{Mode: "standalone", Addresses: []string{"localhost:6379"}},
				TLS: config.TLSConfig{
					Enabled:    true,
					CertFile:   certFile,
					KeyFile:    keyFile,
					ClientAuth: "none",
					MinVersion: "1.3",
				},
				Observability: config.ObservabilityConfig{
					Logging: config.LoggingConfig{Level: "info", Format: "json"},
				},
			},
			wantErr: false,
		},
		{
			name: "TLS enabled without cert file",
			config: &config.Config{
				Server: config.ServerConfig{Port: 8080, GinMode: "release"},
				Redis:  config.RedisConfig{Mode: "standalone", Addresses: []string{"localhost:6379"}},
				TLS: config.TLSConfig{
					Enabled:    true,
					KeyFile:    keyFile,
					MinVersion: "1.3",
				},
			},
			wantErr: true,
			errMsg:  "cert_file is required",
		},
		{
			name: "TLS enabled without key file",
			config: &config.Config{
				Server: config.ServerConfig{Port: 8080, GinMode: "release"},
				Redis:  config.RedisConfig{Mode: "standalone", Addresses: []string{"localhost:6379"}},
				TLS: config.TLSConfig{
					Enabled:    true,
					CertFile:   certFile,
					MinVersion: "1.3",
				},
			},
			wantErr: true,
			errMsg:  "key_file is required",
		},
		{
			name: "cert file does not exist",
			config: &config.Config{
				Server: config.ServerConfig{Port: 8080, GinMode: "release"},
				Redis:  config.RedisConfig{Mode: "standalone", Addresses: []string{"localhost:6379"}},
				TLS: config.TLSConfig{
					Enabled:    true,
					CertFile:   "/nonexistent/cert.pem",
					KeyFile:    keyFile,
					MinVersion: "1.3",
				},
			},
			wantErr: true,
			errMsg:  "cert_file does not exist",
		},
		{
			name: "key file does not exist",
			config: &config.Config{
				Server: config.ServerConfig{Port: 8080, GinMode: "release"},
				Redis:  config.RedisConfig{Mode: "standalone", Addresses: []string{"localhost:6379"}},
				TLS: config.TLSConfig{
					Enabled:    true,
					CertFile:   certFile,
					KeyFile:    "/nonexistent/key.pem",
					MinVersion: "1.3",
				},
			},
			wantErr: true,
			errMsg:  "key_file does not exist",
		},
		{
			name: "invalid client auth mode",
			config: &config.Config{
				Server: config.ServerConfig{Port: 8080, GinMode: "release"},
				Redis:  config.RedisConfig{Mode: "standalone", Addresses: []string{"localhost:6379"}},
				TLS: config.TLSConfig{
					Enabled:    true,
					CertFile:   certFile,
					KeyFile:    keyFile,
					ClientAuth: "invalid",
					MinVersion: "1.3",
				},
			},
			wantErr: true,
			errMsg:  "invalid tls client_auth",
		},
		{
			name: "client auth enabled without CA file",
			config: &config.Config{
				Server: config.ServerConfig{Port: 8080, GinMode: "release"},
				Redis:  config.RedisConfig{Mode: "standalone", Addresses: []string{"localhost:6379"}},
				TLS: config.TLSConfig{
					Enabled:    true,
					CertFile:   certFile,
					KeyFile:    keyFile,
					ClientAuth: "require-and-verify",
					MinVersion: "1.3",
				},
			},
			wantErr: true,
			errMsg:  "ca_file is required",
		},
		{
			name: "CA file does not exist",
			config: &config.Config{
				Server: config.ServerConfig{Port: 8080, GinMode: "release"},
				Redis:  config.RedisConfig{Mode: "standalone", Addresses: []string{"localhost:6379"}},
				TLS: config.TLSConfig{
					Enabled:    true,
					CertFile:   certFile,
					KeyFile:    keyFile,
					CAFile:     "/nonexistent/ca.pem",
					ClientAuth: "require-and-verify",
					MinVersion: "1.3",
				},
			},
			wantErr: true,
			errMsg:  "ca_file does not exist",
		},
		{
			name: "invalid min TLS version",
			config: &config.Config{
				Server: config.ServerConfig{Port: 8080, GinMode: "release"},
				Redis:  config.RedisConfig{Mode: "standalone", Addresses: []string{"localhost:6379"}},
				TLS: config.TLSConfig{
					Enabled:    true,
					CertFile:   certFile,
					KeyFile:    keyFile,
					ClientAuth: "none",
					MinVersion: "1.1",
				},
			},
			wantErr: true,
			errMsg:  "invalid tls min_version",
		},
		{
			name: "valid mTLS config",
			config: &config.Config{
				Server: config.ServerConfig{Port: 8080, GinMode: "release"},
				Redis:  config.RedisConfig{Mode: "standalone", Addresses: []string{"localhost:6379"}},
				TLS: config.TLSConfig{
					Enabled:    true,
					CertFile:   certFile,
					KeyFile:    keyFile,
					CAFile:     caFile,
					ClientAuth: "require-and-verify",
					MinVersion: "1.3",
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
			err := tt.config.Validate()

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestSetDefaults verifies that default values are set correctly.
func TestSetDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Create minimal config file
	minimalConfig := `
redis:
  addresses:
    - localhost:6379
`
	err := os.WriteFile(configPath, []byte(minimalConfig), 0600)
	require.NoError(t, err)

	cfg, err := config.Load(configPath)
	require.NoError(t, err)

	// Verify defaults
	assert.Equal(t, "0.0.0.0", cfg.Server.Host)
	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, 30*time.Second, cfg.Server.ReadTimeout)
	assert.Equal(t, "release", cfg.Server.GinMode)

	assert.Equal(t, "standalone", cfg.Redis.Mode)
	assert.Equal(t, 0, cfg.Redis.DB)
	assert.Equal(t, 10, cfg.Redis.PoolSize)
	assert.Equal(t, 5, cfg.Redis.MinIdleConns)

	assert.Equal(t, float32(50.0), cfg.Kubernetes.QPS)
	assert.Equal(t, 100, cfg.Kubernetes.Burst)
	assert.True(t, cfg.Kubernetes.EnableWatch)

	assert.False(t, cfg.TLS.Enabled)
	assert.Equal(t, "1.3", cfg.TLS.MinVersion)

	assert.Equal(t, "info", cfg.Observability.Logging.Level)
	assert.Equal(t, "json", cfg.Observability.Logging.Format)
	assert.True(t, cfg.Observability.Metrics.Enabled)
	assert.Equal(t, "/metrics", cfg.Observability.Metrics.Path)

	assert.True(t, cfg.Security.RateLimitEnabled)
	assert.Equal(t, 1000, cfg.Security.RateLimit.PerTenant.RequestsPerSecond)
	assert.Equal(t, 2000, cfg.Security.RateLimit.PerTenant.BurstSize)
}
