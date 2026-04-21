package config_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/piwi3910/netweave/internal/config"
	"github.com/piwi3910/netweave/internal/database"
)

// TestConfigAccessors exercises the role-specific accessor methods added
// for H10 (interface-segregation). Each accessor simply returns a backing
// sub-struct; the test verifies the returned value matches the field,
// guarding against field-name drift or wrong field being returned.
func TestConfigAccessors(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Server: config.ServerConfig{
			Host:         "127.0.0.1",
			Port:         8080,
			GinMode:      "test",
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 5 * time.Second,
		},
		TLS: config.TLSConfig{
			Enabled:    true,
			MinVersion: "1.3",
		},
		Security: config.SecurityConfig{
			EnableCORS:       true,
			RateLimitEnabled: true,
		},
		Observability: config.ObservabilityConfig{
			Metrics: config.MetricsConfig{Enabled: true, Path: "/metrics", Namespace: "ns"},
		},
		Validation: config.ValidationConfig{
			Enabled:     true,
			MaxBodySize: 1024,
		},
		MultiTenancy: config.MultiTenancyConfig{RequireMTLS: true},
		FrontendPlugins: config.FrontendPlugins{
			O2IMS: config.FrontendPluginConfig{Enabled: true},
		},
		Auth:          config.AuthConfig{},
		OAuth2:        config.OAuth2Config{},
		Redis:         config.RedisConfig{},
		Kubernetes:    config.KubernetesConfig{},
		Postgres:      database.PostgresConfig{},
		Events:        config.EventsConfig{Enabled: true, WebhookWorkers: 7},
		CertLifecycle: config.CertLifecycleConfig{Enabled: true, MaxRenewalRetries: 3},
		StorageMode:   "postgres",
		Environment:   config.EnvProduction,
	}

	assert.Equal(t, cfg.Server, cfg.ServerCfg())
	assert.Equal(t, cfg.TLS, cfg.TLSCfg())
	assert.Equal(t, cfg.Security, cfg.SecurityCfg())
	assert.Equal(t, cfg.Observability, cfg.ObservabilityCfg())
	assert.Equal(t, cfg.Validation, cfg.ValidationCfg())
	assert.Equal(t, cfg.MultiTenancy, cfg.MultiTenancyCfg())
	assert.Equal(t, cfg.FrontendPlugins, cfg.FrontendPluginsCfg())
	assert.Equal(t, cfg.Auth, cfg.AuthCfg())
	assert.Equal(t, cfg.OAuth2, cfg.OAuth2Cfg())
	assert.Equal(t, cfg.Redis, cfg.RedisCfg())
	assert.Equal(t, cfg.Kubernetes, cfg.KubernetesCfg())
	assert.Equal(t, cfg.Postgres, cfg.PostgresCfg())
	assert.Equal(t, cfg.Events, cfg.EventsCfg())
	assert.Equal(t, cfg.CertLifecycle, cfg.CertLifecycleCfg())
	assert.Equal(t, "postgres", cfg.StorageModeCfg())
	assert.Equal(t, config.EnvProduction, cfg.EnvironmentCfg())
}
