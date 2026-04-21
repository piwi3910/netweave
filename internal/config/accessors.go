// Package config - accessor methods used by role-specific consumer interfaces.
//
// These accessors exist so that consumer packages can declare narrow
// role-specific interfaces (interface-segregation pattern, see H10)
// instead of depending on the full *Config god-struct. Each accessor
// returns the backing sub-struct value by value; Config itself remains
// the single source of truth loaded once from viper in cmd/gateway/main.go.
package config

import "github.com/piwi3910/netweave/internal/database"

// ServerCfg returns the HTTP server settings.
func (c *Config) ServerCfg() ServerConfig { return c.Server }

// TLSCfg returns the TLS/mTLS settings.
func (c *Config) TLSCfg() TLSConfig { return c.TLS }

// SecurityCfg returns the security settings (CORS, rate limiting, headers).
func (c *Config) SecurityCfg() SecurityConfig { return c.Security }

// ObservabilityCfg returns logging, metrics, and tracing settings.
func (c *Config) ObservabilityCfg() ObservabilityConfig { return c.Observability }

// ValidationCfg returns request validation settings.
func (c *Config) ValidationCfg() ValidationConfig { return c.Validation }

// MultiTenancyCfg returns multi-tenancy and mTLS-requirement settings.
func (c *Config) MultiTenancyCfg() MultiTenancyConfig { return c.MultiTenancy }

// FrontendPluginsCfg returns the frontend plugin toggle settings.
func (c *Config) FrontendPluginsCfg() FrontendPlugins { return c.FrontendPlugins }

// AuthCfg returns the auth settings.
func (c *Config) AuthCfg() AuthConfig { return c.Auth }

// OAuth2Cfg returns the OAuth2 settings.
func (c *Config) OAuth2Cfg() OAuth2Config { return c.OAuth2 }

// RedisCfg returns Redis client settings.
func (c *Config) RedisCfg() RedisConfig { return c.Redis }

// KubernetesCfg returns Kubernetes client settings.
func (c *Config) KubernetesCfg() KubernetesConfig { return c.Kubernetes }

// PostgresCfg returns Postgres settings.
func (c *Config) PostgresCfg() database.PostgresConfig { return c.Postgres }

// EventsCfg returns the event-pipeline settings.
func (c *Config) EventsCfg() EventsConfig { return c.Events }

// CertLifecycleCfg returns the certificate-lifecycle settings.
func (c *Config) CertLifecycleCfg() CertLifecycleConfig { return c.CertLifecycle }

// StorageModeCfg returns the configured storage mode.
func (c *Config) StorageModeCfg() string { return c.StorageMode }

// EnvironmentCfg returns the detected environment (dev/staging/prod).
func (c *Config) EnvironmentCfg() string { return c.Environment }
