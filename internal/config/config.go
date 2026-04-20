// Package config provides configuration management for the O2-IMS Gateway.
// It loads configuration from YAML files and environment variables using Viper,
// with support for hot-reloading and validation.
package config

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/piwi3910/netweave/internal/database"
	"github.com/spf13/viper"
)

// TLS client authentication modes.
const (
	tlsClientAuthNone             = "none"
	tlsClientAuthRequest          = "request"
	tlsClientAuthRequire          = "require"
	tlsClientAuthVerify           = "verify"
	tlsClientAuthRequireAndVerify = "require-and-verify"
)

// Environment names for configuration.
const (
	EnvDevelopment = "dev"
	EnvStaging     = "staging"
	EnvProduction  = "prod"

	// DefaultConfigPath is the default configuration file path.
	DefaultConfigPath = "config/config.yaml"
)

// Config represents the complete configuration for the O2-IMS Gateway.
// It includes server settings, Redis configuration, Kubernetes client config,
// TLS/mTLS settings, and observability options.
//
// Configuration can be loaded from:
//   - YAML file (config/config.yaml)
//   - Environment variables (prefixed with NETWEAVE_)
//   - Command-line flags (if integrated with cobra)
//
// Example:
//
//	cfg, err := config.Load("config/config.yaml")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	if err := cfg.Validate(); err != nil {
//	    log.Fatal(err)
//	}
type Config struct {
	Server          ServerConfig            `mapstructure:"server"`
	Redis           RedisConfig             `mapstructure:"redis"`
	Kubernetes      KubernetesConfig        `mapstructure:"kubernetes"`
	TLS             TLSConfig               `mapstructure:"tls"`
	Observability   ObservabilityConfig     `mapstructure:"observability"`
	Security        SecurityConfig          `mapstructure:"security"`
	Validation      ValidationConfig        `mapstructure:"validation"`
	MultiTenancy    MultiTenancyConfig      `mapstructure:"multi_tenancy"`
	OAuth2          OAuth2Config            `mapstructure:"oauth2"`
	Auth            AuthConfig              `mapstructure:"auth"`
	Postgres        database.PostgresConfig `mapstructure:"postgres"`
	StorageMode     string                  `mapstructure:"storage_mode"`
	FrontendPlugins FrontendPlugins         `mapstructure:"frontend_plugins"`
	CertLifecycle   CertLifecycleConfig     `mapstructure:"cert_lifecycle"`

	// Environment stores the detected environment (dev/staging/prod)
	// This field is set automatically during Load() and used for validation
	Environment string `mapstructure:"-"`
}

// FrontendPlugins configures which protocol API frontends are enabled.
// All plugins are disabled by default and must be explicitly enabled by platform admins.
type FrontendPlugins struct {
	O2IMS   FrontendPluginConfig `mapstructure:"o2ims"`
	O2DMS   FrontendPluginConfig `mapstructure:"o2dms"`
	O2SMO   FrontendPluginConfig `mapstructure:"o2smo"`
	TMForum FrontendPluginConfig `mapstructure:"tmforum"`
	GraphQL FrontendPluginConfig `mapstructure:"graphql"`
}

// FrontendPluginConfig contains configuration for a single frontend plugin.
type FrontendPluginConfig struct {
	// Enabled controls whether this frontend plugin is active.
	// Default: false (all plugins disabled by default).
	Enabled bool `mapstructure:"enabled"`
}

// CertLifecycleConfig configures automated certificate lifecycle management.
type CertLifecycleConfig struct {
	// Enabled activates certificate lifecycle automation.
	Enabled bool `mapstructure:"enabled"`

	// ScanInterval is how often to scan for expiring certificates.
	ScanInterval time.Duration `mapstructure:"scan_interval"`

	// RenewalWindow is how far before expiry to trigger renewal.
	RenewalWindow time.Duration `mapstructure:"renewal_window"`

	// MaxRenewalRetries is the maximum renewal attempts before giving up.
	MaxRenewalRetries int `mapstructure:"max_renewal_retries"`

	// RenewalRetryInterval is the base interval between renewal retries.
	RenewalRetryInterval time.Duration `mapstructure:"renewal_retry_interval"`

	// WebhookURL is the target URL for lifecycle event notifications.
	WebhookURL string `mapstructure:"webhook_url"`

	// WebhookHMACSecretEnvVar is the env var containing the HMAC secret.
	WebhookHMACSecretEnvVar string `mapstructure:"webhook_hmac_secret_env_var"`

	// VaultPKIPath is the Vault PKI mount path.
	VaultPKIPath string `mapstructure:"vault_pki_path"`

	// KeycloakSync enables Keycloak user attribute synchronization.
	KeycloakSync bool `mapstructure:"keycloak_sync"`
}

// AuthConfig contains authentication backend configuration.
type AuthConfig struct {
	// Backend specifies the authentication storage backend: "redis" or "keycloak".
	Backend string `mapstructure:"backend"`

	// Keycloak contains Keycloak-specific configuration (when backend is "keycloak").
	Keycloak KeycloakConfig `mapstructure:"keycloak"`
}

// KeycloakConfig contains Keycloak authentication backend configuration.
type KeycloakConfig struct {
	// BaseURL is the Keycloak server base URL (e.g., "http://localhost:8090").
	BaseURL string `mapstructure:"base_url"`

	// Realm is the Keycloak realm name.
	Realm string `mapstructure:"realm"`

	// ClientID is the OAuth2 client ID for the gateway.
	ClientID string `mapstructure:"client_id"`

	// ClientSecret is the OAuth2 client secret.
	ClientSecret string `mapstructure:"client_secret"`

	// ClientSecretEnvVar specifies the environment variable containing the client secret.
	ClientSecretEnvVar string `mapstructure:"client_secret_env_var"`

	// AdminUsername is the Keycloak admin username (for admin API access).
	AdminUsername string `mapstructure:"admin_username"`

	// AdminPassword is the Keycloak admin password.
	AdminPassword string `mapstructure:"admin_password"`

	// AdminPasswordEnvVar specifies the environment variable containing the admin password.
	AdminPasswordEnvVar string `mapstructure:"admin_password_env_var"`

	// Timeout is the HTTP client timeout duration.
	Timeout time.Duration `mapstructure:"timeout"`
}

// GetClientSecret retrieves the Keycloak client secret from the configured source.
func (k *KeycloakConfig) GetClientSecret() (string, error) {
	if k.ClientSecretEnvVar != "" {
		if secret := os.Getenv(k.ClientSecretEnvVar); secret != "" {
			return secret, nil
		}
		return "", fmt.Errorf("environment variable %s is not set", k.ClientSecretEnvVar)
	}
	return k.ClientSecret, nil
}

// GetAdminPassword retrieves the Keycloak admin password from the configured source.
func (k *KeycloakConfig) GetAdminPassword() (string, error) {
	if k.AdminPasswordEnvVar != "" {
		if pwd := os.Getenv(k.AdminPasswordEnvVar); pwd != "" {
			return pwd, nil
		}
		return "", fmt.Errorf("environment variable %s is not set", k.AdminPasswordEnvVar)
	}
	return k.AdminPassword, nil
}

// OAuth2Config contains OAuth2/OIDC authentication configuration.
type OAuth2Config struct {
	// Enabled enables OAuth2/OIDC authentication.
	Enabled bool `mapstructure:"enabled"`

	// Priority indicates whether OAuth2 takes priority over mTLS when both are present.
	Priority bool `mapstructure:"priority"`

	// KeycloakBaseURL is the Keycloak server base URL.
	KeycloakBaseURL string `mapstructure:"keycloak_base_url"`

	// KeycloakRealm is the Keycloak realm name.
	KeycloakRealm string `mapstructure:"keycloak_realm"`

	// KeycloakClientID is the OIDC client ID.
	KeycloakClientID string `mapstructure:"keycloak_client_id"`

	// KeycloakSecret is the OIDC client secret.
	KeycloakSecret string `mapstructure:"keycloak_secret"`

	// KeycloakSecretEnvVar specifies the environment variable containing the client secret.
	KeycloakSecretEnvVar string `mapstructure:"keycloak_secret_env_var"`

	// AutoProvisionUsers enables automatic user creation from OAuth2 token claims.
	AutoProvisionUsers bool `mapstructure:"auto_provision_users"`

	// DefaultRole is the role ID to assign to auto-provisioned users.
	DefaultRole string `mapstructure:"default_role"`

	// GroupRoleMapping maps Keycloak groups to role IDs.
	GroupRoleMapping map[string]string `mapstructure:"group_role_mapping"`

	// RequireTenantClaim requires the tenant_id claim to be present in tokens.
	RequireTenantClaim bool `mapstructure:"require_tenant_claim"`
}

// MultiTenancyConfig contains multi-tenancy and RBAC configuration.
type MultiTenancyConfig struct {
	// Enabled enables multi-tenancy and RBAC enforcement.
	Enabled bool `mapstructure:"enabled"`

	// RequireMTLS requires mTLS client certificates for authentication.
	RequireMTLS bool `mapstructure:"require_mtls"`

	// InitializeDefaultRoles creates default system roles on startup.
	InitializeDefaultRoles bool `mapstructure:"initialize_default_roles"`

	// DefaultTenantQuota sets default quotas for new tenants.
	DefaultTenantQuota DefaultQuotaConfig `mapstructure:"default_tenant_quota"`

	// AuditLogRetentionDays sets how long audit logs are retained.
	AuditLogRetentionDays int `mapstructure:"audit_log_retention_days"`

	// SkipAuthPaths is a list of paths that skip authentication.
	SkipAuthPaths []string `mapstructure:"skip_auth_paths"`
}

// DefaultQuotaConfig contains default quota values for new tenants.
type DefaultQuotaConfig struct {
	MaxSubscriptions     int `mapstructure:"max_subscriptions"`
	MaxResourcePools     int `mapstructure:"max_resource_pools"`
	MaxDeployments       int `mapstructure:"max_deployments"`
	MaxUsers             int `mapstructure:"max_users"`
	MaxRequestsPerMinute int `mapstructure:"max_requests_per_minute"`
}

// ServerConfig contains HTTP server configuration.
type ServerConfig struct {
	// Host is the network interface to bind to (e.g., "0.0.0.0", "localhost")
	Host string `mapstructure:"host"`

	// Port is the admin HTTP server port (default: 8080)
	Port int `mapstructure:"port"`

	// O2Port is the O2-IMS mTLS port for O2-IMS, O2-DMS, O2-SMO endpoints (default: 8443)
	O2Port int `mapstructure:"o2_port"`

	// TMFPort is the TMForum API port (default: 8444)
	TMFPort int `mapstructure:"tmf_port"`

	// GraphQLPort is the GraphQL API port (default: 8445)
	GraphQLPort int `mapstructure:"graphql_port"`

	// ReadTimeout is the maximum duration for reading the entire request
	ReadTimeout time.Duration `mapstructure:"read_timeout"`

	// WriteTimeout is the maximum duration before timing out writes of the response
	WriteTimeout time.Duration `mapstructure:"write_timeout"`

	// IdleTimeout is the maximum duration to wait for the next request when keep-alives are enabled
	IdleTimeout time.Duration `mapstructure:"idle_timeout"`

	// ShutdownTimeout is the maximum duration to wait for graceful shutdown
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`

	// MaxHeaderBytes is the maximum size of request headers
	MaxHeaderBytes int `mapstructure:"max_header_bytes"`

	// GinMode sets the Gin framework mode ("debug", "release", "test")
	GinMode string `mapstructure:"gin_mode"`
}

// RedisConfig contains Redis client and cluster configuration.
type RedisConfig struct {
	// Mode specifies Redis deployment mode: "standalone", "sentinel", "cluster"
	Mode string `mapstructure:"mode"`

	// Addresses contains Redis server addresses
	// For standalone: ["localhost:6379"]
	// For sentinel: ["sentinel1:26379", "sentinel2:26379"]
	// For cluster: ["node1:6379", "node2:6379", ...]
	Addresses []string `mapstructure:"addresses"`

	// MasterName is required for Sentinel mode (e.g., "mymaster")
	MasterName string `mapstructure:"master_name"`

	// Password for Redis authentication (optional, DEPRECATED: use PasswordEnvVar or PasswordFile)
	// WARNING: Storing passwords in config files is insecure. Use environment variables or secret files instead.
	Password string `mapstructure:"password"`

	// PasswordEnvVar specifies the environment variable name containing the Redis password
	// Example: "REDIS_PASSWORD"
	// This takes priority over PasswordFile if both are set
	PasswordEnvVar string `mapstructure:"password_env_var"`

	// PasswordFile specifies the path to a file containing the Redis password
	// Example: "/run/secrets/redis-password" (Kubernetes Secret mount)
	// The file should contain only the password, whitespace will be trimmed
	PasswordFile string `mapstructure:"password_file"`

	// SentinelPassword for Sentinel authentication (optional, DEPRECATED: use SentinelPasswordEnvVar
	// or SentinelPasswordFile)
	// WARNING: Storing passwords in config files is insecure. Use environment variables or secret files instead.
	// Best practice: Use different passwords for Sentinel and Redis.
	SentinelPassword string `mapstructure:"sentinel_password"`

	// SentinelPasswordEnvVar specifies the environment variable name containing the Sentinel password
	// Example: "SENTINEL_PASSWORD"
	// This takes priority over SentinelPasswordFile if both are set
	SentinelPasswordEnvVar string `mapstructure:"sentinel_password_env_var"`

	// SentinelPasswordFile specifies the path to a file containing the Sentinel password
	// Example: "/run/secrets/sentinel-password" (Kubernetes Secret mount)
	// The file should contain only the password, whitespace will be trimmed
	SentinelPasswordFile string `mapstructure:"sentinel_password_file"`

	// DB is the Redis database number (0-15, only for standalone/sentinel)
	DB int `mapstructure:"db"`

	// PoolSize is the maximum number of socket connections
	PoolSize int `mapstructure:"pool_size"`

	// MinIdleConns is the minimum number of idle connections
	MinIdleConns int `mapstructure:"min_idle_conns"`

	// MaxRetries is the maximum number of retries before giving up
	MaxRetries int `mapstructure:"max_retries"`

	// DialTimeout is the timeout for establishing new connections
	DialTimeout time.Duration `mapstructure:"dial_timeout"`

	// ReadTimeout is the timeout for socket reads
	ReadTimeout time.Duration `mapstructure:"read_timeout"`

	// WriteTimeout is the timeout for socket writes
	WriteTimeout time.Duration `mapstructure:"write_timeout"`

	// PoolTimeout is the timeout when all connections are busy
	PoolTimeout time.Duration `mapstructure:"pool_timeout"`

	// IdleTimeout is the amount of time after which client closes idle connections
	IdleTimeout time.Duration `mapstructure:"idle_timeout"`

	// EnableTLS enables TLS for Redis connections
	EnableTLS bool `mapstructure:"enable_tls"`

	// TLSInsecureSkipVerify skips TLS certificate verification (use only for testing)
	TLSInsecureSkipVerify bool `mapstructure:"tls_insecure_skip_verify"`
}

// GetPassword retrieves the Redis password from the configured source.
// Priority order:
//  1. Environment variable (if PasswordEnvVar is set)
//  2. File (if PasswordFile is set)
//  3. Direct password (DEPRECATED, for backward compatibility)
//
// Returns empty string if no password is configured (Redis without authentication).
func (c *RedisConfig) GetPassword() (string, error) {
	// Priority 1: Environment variable
	if c.PasswordEnvVar != "" {
		if pwd := os.Getenv(c.PasswordEnvVar); pwd != "" {
			return pwd, nil
		}
		// Env var specified but not set - this is an error
		return "", fmt.Errorf("environment variable %s is not set", c.PasswordEnvVar)
	}

	// Priority 2: Password file
	if c.PasswordFile != "" {
		data, err := os.ReadFile(c.PasswordFile)
		if err != nil {
			return "", fmt.Errorf("failed to read password file %s: %w", c.PasswordFile, err)
		}
		return strings.TrimSpace(string(data)), nil
	}

	// Priority 3: Direct password (DEPRECATED)
	return c.Password, nil
}

// IsUsingDeprecatedPassword returns true if the deprecated direct Password field
// is being used instead of the recommended PasswordEnvVar or PasswordFile.
// This method avoids direct access to sensitive password data for deprecation checks.
func (c *RedisConfig) IsUsingDeprecatedPassword() bool {
	return c.Password != "" && c.PasswordEnvVar == "" && c.PasswordFile == ""
}

// IsUsingDeprecatedSentinelPassword returns true if the deprecated direct SentinelPassword
// field is being used instead of the recommended SentinelPasswordEnvVar or SentinelPasswordFile.
// This method avoids direct access to sensitive password data for deprecation checks.
func (c *RedisConfig) IsUsingDeprecatedSentinelPassword() bool {
	return c.SentinelPassword != "" && c.SentinelPasswordEnvVar == "" && c.SentinelPasswordFile == ""
}

// GetSentinelPassword retrieves the Sentinel password from the configured source.
// Priority order:
//  1. Environment variable (if SentinelPasswordEnvVar is set)
//  2. File (if SentinelPasswordFile is set)
//  3. Direct password (DEPRECATED, for backward compatibility)
//
// Returns empty string if no password is configured (Sentinel without authentication).
func (c *RedisConfig) GetSentinelPassword() (string, error) {
	// Priority 1: Environment variable
	if c.SentinelPasswordEnvVar != "" {
		if pwd := os.Getenv(c.SentinelPasswordEnvVar); pwd != "" {
			return pwd, nil
		}
		// Env var specified but not set - this is an error
		return "", fmt.Errorf("environment variable %s is not set", c.SentinelPasswordEnvVar)
	}

	// Priority 2: Password file
	if c.SentinelPasswordFile != "" {
		data, err := os.ReadFile(c.SentinelPasswordFile)
		if err != nil {
			return "", fmt.Errorf("failed to read sentinel password file %s: %w", c.SentinelPasswordFile, err)
		}
		return strings.TrimSpace(string(data)), nil
	}

	// Priority 3: Direct password (DEPRECATED)
	return c.SentinelPassword, nil
}

// KubernetesConfig contains Kubernetes client configuration.
type KubernetesConfig struct {
	// ConfigPath is the path to kubeconfig file
	// Leave empty to use in-cluster config when running in a pod
	ConfigPath string `mapstructure:"config_path"`

	// Context is the kubeconfig context to use (optional)
	Context string `mapstructure:"context"`

	// Namespace is the default namespace for operations
	// Leave empty to use all namespaces
	Namespace string `mapstructure:"namespace"`

	// QPS is the maximum queries per second to the Kubernetes API
	QPS float32 `mapstructure:"qps"`

	// Burst is the maximum burst for throttle
	Burst int `mapstructure:"burst"`

	// Timeout is the timeout for Kubernetes API requests
	Timeout time.Duration `mapstructure:"timeout"`

	// EnableWatch enables Kubernetes watch for real-time updates
	EnableWatch bool `mapstructure:"enable_watch"`

	// WatchResync is the resync period for watch cache
	WatchResync time.Duration `mapstructure:"watch_resync"`
}

// TLSConfig contains TLS/mTLS configuration.
type TLSConfig struct {
	// Enabled enables TLS for the HTTP server
	Enabled bool `mapstructure:"enabled"`

	// CertFile is the path to the TLS certificate file
	CertFile string `mapstructure:"cert_file"`

	// KeyFile is the path to the TLS private key file
	KeyFile string `mapstructure:"key_file"`

	// CAFile is the path to the CA certificate file for client verification
	CAFile string `mapstructure:"ca_file"`

	// ClientAuth specifies the client authentication mode
	// Options: "none", "request", "require", "verify", "require-and-verify"
	ClientAuth string `mapstructure:"client_auth"`

	// MinVersion is the minimum TLS version ("1.2", "1.3")
	MinVersion string `mapstructure:"min_version"`

	// CipherSuites is a list of enabled cipher suites (optional)
	CipherSuites []string `mapstructure:"cipher_suites"`
}

// ObservabilityConfig contains logging, metrics, and tracing configuration.
type ObservabilityConfig struct {
	Logging LoggingConfig `mapstructure:"logging"`
	Metrics MetricsConfig `mapstructure:"metrics"`
	Tracing TracingConfig `mapstructure:"tracing"`
}

// LoggingConfig contains structured logging configuration.
type LoggingConfig struct {
	// Level sets the log level ("debug", "info", "warn", "error", "fatal")
	Level string `mapstructure:"level"`

	// Format sets the log format ("json", "console")
	Format string `mapstructure:"format"`

	// OutputPaths is a list of output destinations (e.g., ["stdout", "/var/log/app.log"])
	OutputPaths []string `mapstructure:"output_paths"`

	// ErrorOutputPaths is a list of error output destinations
	ErrorOutputPaths []string `mapstructure:"error_output_paths"`

	// EnableCaller adds caller information to log entries
	EnableCaller bool `mapstructure:"enable_caller"`

	// EnableStacktrace adds stacktrace on errors
	EnableStacktrace bool `mapstructure:"enable_stacktrace"`

	// Development enables development mode (more verbose, console format)
	Development bool `mapstructure:"development"`
}

// MetricsConfig contains Prometheus metrics configuration.
type MetricsConfig struct {
	// Enabled enables Prometheus metrics collection
	Enabled bool `mapstructure:"enabled"`

	// Path is the HTTP path for metrics endpoint (default: "/metrics")
	Path string `mapstructure:"path"`

	// Port is the port for metrics server (0 = use main server port)
	Port int `mapstructure:"port"`

	// Namespace is the Prometheus metrics namespace
	Namespace string `mapstructure:"namespace"`

	// Subsystem is the Prometheus metrics subsystem
	Subsystem string `mapstructure:"subsystem"`

	// EnableGoMetrics enables Go runtime metrics
	EnableGoMetrics bool `mapstructure:"enable_go_metrics"`

	// EnableProcessMetrics enables process metrics
	EnableProcessMetrics bool `mapstructure:"enable_process_metrics"`
}

// TracingConfig contains distributed tracing configuration.
type TracingConfig struct {
	// Enabled enables distributed tracing
	Enabled bool `mapstructure:"enabled"`

	// Provider specifies the tracing provider ("jaeger", "zipkin", "otlp")
	Provider string `mapstructure:"provider"`

	// Endpoint is the tracing collector endpoint
	Endpoint string `mapstructure:"endpoint"`

	// ServiceName is the service name for tracing
	ServiceName string `mapstructure:"service_name"`

	// SamplingRate is the sampling rate (0.0 to 1.0)
	SamplingRate float64 `mapstructure:"sampling_rate"`

	// EnableBatching enables batch span export
	EnableBatching bool `mapstructure:"enable_batching"`

	// BatchTimeout is the timeout for batch export
	BatchTimeout time.Duration `mapstructure:"batch_timeout"`
}

// SecurityConfig contains security-related configuration.
type SecurityConfig struct {
	// EnableCORS enables CORS support
	EnableCORS bool `mapstructure:"enable_cors"`

	// AllowedOrigins is a list of allowed CORS origins
	AllowedOrigins []string `mapstructure:"allowed_origins"`

	// AllowedMethods is a list of allowed HTTP methods
	AllowedMethods []string `mapstructure:"allowed_methods"`

	// AllowedHeaders is a list of allowed HTTP headers
	AllowedHeaders []string `mapstructure:"allowed_headers"`

	// RateLimitEnabled enables rate limiting
	RateLimitEnabled bool `mapstructure:"rate_limit_enabled"`

	// RateLimit contains comprehensive rate limiting configuration
	RateLimit RateLimitConfig `mapstructure:"rate_limit"`

	// AllowInsecureCallbacks allows HTTP (non-HTTPS) webhook callbacks
	// This should ONLY be enabled in development/testing environments
	// Production deployments MUST enforce HTTPS for webhook callbacks
	AllowInsecureCallbacks bool `mapstructure:"allow_insecure_callbacks"`

	// DisableSSRFProtection disables SSRF protection for webhook callbacks
	// This allows callbacks to localhost and private IP addresses
	// WARNING: This should ONLY be enabled in development/testing environments
	// Production deployments MUST keep SSRF protection enabled to prevent attacks
	DisableSSRFProtection bool `mapstructure:"disable_ssrf_protection"`

	// SecurityHeaders contains configuration for security headers middleware
	SecurityHeaders SecurityHeadersConfig `mapstructure:"security_headers"`
}

// SecurityHeadersConfig contains configuration for HTTP security headers.
type SecurityHeadersConfig struct {
	// Enabled controls whether security headers are added (default: true)
	Enabled bool `mapstructure:"enabled"`

	// HSTSMaxAge is the max-age for Strict-Transport-Security header in seconds
	// Default: 31536000 (1 year)
	HSTSMaxAge int `mapstructure:"hsts_max_age"`

	// HSTSIncludeSubDomains includes subdomains in HSTS
	HSTSIncludeSubDomains bool `mapstructure:"hsts_include_subdomains"`

	// HSTSPreload enables HSTS preload
	HSTSPreload bool `mapstructure:"hsts_preload"`

	// ContentSecurityPolicy is the Content-Security-Policy header value
	// Default: "default-src 'none'; frame-ancestors 'none'"
	ContentSecurityPolicy string `mapstructure:"content_security_policy"`

	// FrameOptions is the X-Frame-Options header value
	// Default: "DENY"
	FrameOptions string `mapstructure:"frame_options"`

	// ReferrerPolicy is the Referrer-Policy header value
	// Default: "strict-origin-when-cross-origin"
	ReferrerPolicy string `mapstructure:"referrer_policy"`
}

// RateLimitConfig contains comprehensive rate limiting configuration.
type RateLimitConfig struct {
	// PerTenant configures per-tenant rate limits
	PerTenant TenantRateLimitConfig `mapstructure:"tenant"`

	// PerEndpoint configures per-endpoint rate limits
	PerEndpoint []EndpointRateLimitConfig `mapstructure:"endpoints"`

	// Global configures global rate limits
	Global GlobalRateLimitConfig `mapstructure:"global"`

	// PerResource configures per-resource-type rate limits
	PerResource ResourceRateLimitConfig `mapstructure:"resource"`

	// FailMode controls the default fail-open/fail-closed behavior when the
	// rate-limiter backend (Redis) is unavailable. Valid values:
	//   - "open"   : allow the request (previous behavior; only for read APIs)
	//   - "closed" : deny with 503 Service Unavailable
	//
	// When empty, the limiter picks per-method defaults: fail-closed for
	// POST/PUT/PATCH/DELETE, fail-open for reads. Production configs that
	// hard-code "open" for all endpoints are rejected by the validator.
	FailMode string `mapstructure:"fail_mode"`
}

// TenantRateLimitConfig configures per-tenant rate limits.
type TenantRateLimitConfig struct {
	RequestsPerSecond int `mapstructure:"requests_per_second"`
	BurstSize         int `mapstructure:"burst_size"`
}

// EndpointRateLimitConfig configures rate limits for specific endpoints.
type EndpointRateLimitConfig struct {
	Path              string `mapstructure:"path"`
	Method            string `mapstructure:"method"`
	RequestsPerSecond int    `mapstructure:"requests_per_second"`
	BurstSize         int    `mapstructure:"burst_size"`
	// FailMode overrides RateLimitConfig.FailMode for this endpoint. Valid
	// values: "open", "closed" or empty to inherit.
	FailMode string `mapstructure:"fail_mode"`
}

// GlobalRateLimitConfig configures global rate limits.
type GlobalRateLimitConfig struct {
	RequestsPerSecond     int `mapstructure:"requests_per_second"`
	MaxConcurrentRequests int `mapstructure:"max_concurrent_requests"`
}

// ResourceRateLimitConfig contains resource-type-specific rate limiting configuration.
type ResourceRateLimitConfig struct {
	// Enabled controls whether resource rate limiting is active
	Enabled bool `mapstructure:"enabled"`

	// DeploymentManagers configures limits for deployment manager operations
	DeploymentManagers ResourceTypeLimitConfig `mapstructure:"deployment_managers"`

	// ResourcePools configures limits for resource pool operations
	ResourcePools ResourceTypeLimitConfig `mapstructure:"resource_pools"`

	// Resources configures limits for individual resource operations
	Resources ResourceTypeLimitConfig `mapstructure:"resources"`

	// ResourceTypes configures limits for resource type operations
	ResourceTypes ResourceTypeLimitConfig `mapstructure:"resource_types"`

	// Subscriptions configures limits for subscription operations
	Subscriptions SubscriptionRateLimitConfig `mapstructure:"subscriptions"`
}

// ResourceTypeLimitConfig defines rate limits for a resource type.
type ResourceTypeLimitConfig struct {
	// ReadsPerMinute limits read operations per minute
	ReadsPerMinute int `mapstructure:"reads_per_minute"`

	// WritesPerMinute limits write operations per minute
	WritesPerMinute int `mapstructure:"writes_per_minute"`

	// ListPageSizeMax limits the maximum page size for list operations
	ListPageSizeMax int `mapstructure:"list_page_size_max"`
}

// SubscriptionRateLimitConfig defines rate limits specific to subscriptions.
type SubscriptionRateLimitConfig struct {
	// CreatesPerHour limits subscription creations per hour
	CreatesPerHour int `mapstructure:"creates_per_hour"`

	// MaxActive limits the maximum number of active subscriptions per tenant
	MaxActive int `mapstructure:"max_active"`

	// ReadsPerMinute limits read operations per minute
	ReadsPerMinute int `mapstructure:"reads_per_minute"`
}

// ValidationConfig contains OpenAPI request/response validation configuration.
type ValidationConfig struct {
	// Enabled enables OpenAPI request validation
	Enabled bool `mapstructure:"enabled"`

	// ValidateResponse enables OpenAPI response validation (use only in development/testing)
	ValidateResponse bool `mapstructure:"validate_response"`

	// SpecPath is the path to a custom OpenAPI specification file
	// If empty, the embedded spec will be used
	SpecPath string `mapstructure:"spec_path"`

	// MaxBodySize is the maximum allowed request body size in bytes
	// Requests exceeding this limit are rejected with 413 Payload Too Large
	// Default: 1048576 (1MB)
	MaxBodySize int64 `mapstructure:"max_body_size"`
}

// Load loads configuration from the specified file path and environment variables.
// Environment variables override file values and should be prefixed with NETWEAVE_
// (e.g., NETWEAVE_SERVER_PORT=8080).
//
// Configuration file resolution order:
//  1. Explicit configPath parameter (if provided)
//  2. NETWEAVE_CONFIG environment variable
//  3. Environment-specific config via NETWEAVE_ENV (dev/staging/prod)
//  4. Default: config/config.yaml
//
// Returns an error if the configuration file cannot be read or parsed.
//
// Example:
//
//	cfg, err := config.Load("config/config.yaml")
//	if err != nil {
//	    return fmt.Errorf("failed to load config: %w", err)
//	}
func Load(configPath string) (*Config, error) {
	v := viper.New()

	// Set configuration file with environment detection
	configPath = resolveConfigPath(configPath)
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		// Default configuration file locations
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath("./config")
		v.AddConfigPath(".")
		v.AddConfigPath("/etc/netweave")
	}

	// Enable environment variable overrides
	v.SetEnvPrefix("NETWEAVE")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Set defaults
	setDefaults(v)

	// Read configuration file
	if err := v.ReadInConfig(); err != nil {
		// Config file is optional if all values come from env vars
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if !errors.As(err, &configFileNotFoundError) && !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	// Unmarshal configuration
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Set environment from NETWEAVE_ENV or detect from config path
	cfg.Environment = detectEnvironment(configPath)

	return &cfg, nil
}

// resolveConfigPath determines the configuration file path to use.
// Priority order:
//  1. Explicit configPath parameter (if non-empty)
//  2. NETWEAVE_CONFIG environment variable
//  3. Environment-specific config via NETWEAVE_ENV (dev/staging/prod)
//  4. Empty string (use viper defaults)
//
// Security: All paths are validated to prevent directory traversal and arbitrary file inclusion.
func resolveConfigPath(configPath string) string {
	// Priority 1: Explicit path provided
	if configPath != "" && configPath != DefaultConfigPath {
		if err := validateConfigPath(configPath); err != nil {
			log.Printf("SECURITY: rejected config path %q: %v (falling back to defaults)", configPath, err)
			return ""
		}
		return configPath
	}

	// Priority 2: NETWEAVE_CONFIG environment variable
	if envConfig := os.Getenv("NETWEAVE_CONFIG"); envConfig != "" {
		if err := validateConfigPath(envConfig); err != nil {
			log.Printf("SECURITY: rejected NETWEAVE_CONFIG env var %q: %v (falling back to defaults)", envConfig, err)
			return ""
		}
		return envConfig
	}

	// Priority 3: Environment-specific config via NETWEAVE_ENV
	if env := os.Getenv("NETWEAVE_ENV"); env != "" {
		envConfigPath := fmt.Sprintf("config/config.%s.yaml", env)
		// Check if environment-specific config exists
		if _, err := os.Stat(envConfigPath); err == nil {
			return envConfigPath
		}
	}

	// Priority 4: Use default or empty (viper will search default paths)
	return ""
}

// validateConfigPath validates that a configuration file path is safe to load.
// It prevents directory traversal attacks and arbitrary file inclusion.
// Security checks:
//   - Must have .yaml or .yml extension (prevents loading arbitrary files)
//   - Path must not contain directory traversal (../../../etc/passwd)
//   - For production safety, relative paths should be in ./config/ or current dir
//
// Security logging for rejected paths is handled by the caller (resolveConfigPath).
func validateConfigPath(path string) error {
	if path == "" {
		return fmt.Errorf("config path is empty")
	}

	// Validate file extension
	if err := validateConfigExtension(path); err != nil {
		return err
	}

	// Check for directory traversal
	if err := validateNoTraversal(path); err != nil {
		return err
	}

	// Clean and validate normalized path
	cleanPath := filepath.Clean(path)
	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("config path contains directory traversal after normalization")
	}

	// Validate absolute paths
	if filepath.IsAbs(cleanPath) {
		return validateAbsolutePath(cleanPath)
	}

	// Relative paths are allowed
	return nil
}

// validateConfigExtension ensures the config file has a valid extension.
func validateConfigExtension(path string) error {
	ext := filepath.Ext(path)
	if ext != ".yaml" && ext != ".yml" {
		return fmt.Errorf("config file must have .yaml or .yml extension")
	}
	return nil
}

// validateNoTraversal checks for directory traversal attempts.
func validateNoTraversal(path string) error {
	if strings.Contains(path, "..") {
		return fmt.Errorf("config path contains directory traversal")
	}
	return nil
}

// validateAbsolutePath validates absolute paths and checks for symlinks.
func validateAbsolutePath(cleanPath string) error {
	// For absolute paths, check if it exists and validate if it's a symlink
	info, err := os.Lstat(cleanPath)
	if err != nil {
		// Path doesn't exist yet - this is valid for config files that will be created
		// We've already validated the extension, so just return
		if os.IsNotExist(err) {
			return nil
		}
		// Other errors (like permission denied) should be reported
		return fmt.Errorf("cannot access config path: %w", err)
	}

	// If it's a symlink, validate the target
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := filepath.EvalSymlinks(cleanPath)
		if err != nil {
			return fmt.Errorf("cannot resolve symlink: %w", err)
		}
		// Ensure symlink target also has valid extension
		return validateConfigExtension(target)
	}

	return nil
}

// detectEnvironment determines the environment from the config path or NETWEAVE_ENV.
func detectEnvironment(configPath string) string {
	// Check NETWEAVE_ENV first
	if env := os.Getenv("NETWEAVE_ENV"); env != "" {
		return env
	}

	// Detect from config path
	switch {
	case strings.Contains(configPath, ".dev."):
		return EnvDevelopment
	case strings.Contains(configPath, ".staging."):
		return EnvStaging
	case strings.Contains(configPath, ".prod."):
		return EnvProduction
	default:
		return EnvDevelopment // Default to dev
	}
}

// setDefaults sets default values for all configuration options.
func setDefaults(v *viper.Viper) {
	// Server defaults
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.o2_port", 8443)
	v.SetDefault("server.tmf_port", 8444)
	v.SetDefault("server.graphql_port", 8445)
	v.SetDefault("server.read_timeout", "30s")
	v.SetDefault("server.write_timeout", "30s")
	v.SetDefault("server.idle_timeout", "120s")
	v.SetDefault("server.shutdown_timeout", "30s")
	v.SetDefault("server.max_header_bytes", 1048576) // 1MB
	v.SetDefault("server.gin_mode", "release")

	// Redis defaults
	v.SetDefault("redis.mode", "standalone")
	v.SetDefault("redis.addresses", []string{"localhost:6379"})
	v.SetDefault("redis.db", 0)
	v.SetDefault("redis.pool_size", 10)
	v.SetDefault("redis.min_idle_conns", 5)
	v.SetDefault("redis.max_retries", 3)
	v.SetDefault("redis.dial_timeout", "5s")
	v.SetDefault("redis.read_timeout", "3s")
	v.SetDefault("redis.write_timeout", "3s")
	v.SetDefault("redis.pool_timeout", "4s")
	v.SetDefault("redis.idle_timeout", "5m")
	v.SetDefault("redis.enable_tls", false)
	v.SetDefault("redis.tls_insecure_skip_verify", false)

	// Kubernetes defaults
	v.SetDefault("kubernetes.config_path", "") // Use in-cluster config
	v.SetDefault("kubernetes.qps", 50.0)
	v.SetDefault("kubernetes.burst", 100)
	v.SetDefault("kubernetes.timeout", "30s")
	v.SetDefault("kubernetes.enable_watch", true)
	v.SetDefault("kubernetes.watch_resync", "10m")

	// TLS defaults
	v.SetDefault("tls.enabled", false)
	v.SetDefault("tls.client_auth", "none")
	v.SetDefault("tls.min_version", "1.3")

	// Logging defaults
	v.SetDefault("observability.logging.level", "info")
	v.SetDefault("observability.logging.format", "json")
	v.SetDefault("observability.logging.output_paths", []string{"stdout"})
	v.SetDefault("observability.logging.error_output_paths", []string{"stderr"})
	v.SetDefault("observability.logging.enable_caller", true)
	v.SetDefault("observability.logging.enable_stacktrace", false)
	v.SetDefault("observability.logging.development", false)

	// Metrics defaults
	v.SetDefault("observability.metrics.enabled", true)
	v.SetDefault("observability.metrics.path", "/metrics")
	v.SetDefault("observability.metrics.port", 0) // Use main server port
	v.SetDefault("observability.metrics.namespace", "netweave")
	v.SetDefault("observability.metrics.subsystem", "gateway")
	v.SetDefault("observability.metrics.enable_go_metrics", true)
	v.SetDefault("observability.metrics.enable_process_metrics", true)

	// Tracing defaults
	v.SetDefault("observability.tracing.enabled", false)
	v.SetDefault("observability.tracing.provider", "otlp")
	v.SetDefault("observability.tracing.service_name", "netweave-gateway")
	v.SetDefault("observability.tracing.sampling_rate", 0.1)
	v.SetDefault("observability.tracing.enable_batching", true)
	v.SetDefault("observability.tracing.batch_timeout", "5s")

	// Security defaults
	v.SetDefault("security.enable_cors", false)
	v.SetDefault("security.allowed_methods", []string{"GET", "POST", "PUT", "PATCH", "DELETE"})
	v.SetDefault("security.rate_limit_enabled", true)
	v.SetDefault("security.rate_limit.tenant.requests_per_second", 1000)
	v.SetDefault("security.rate_limit.tenant.burst_size", 2000)
	v.SetDefault("security.rate_limit.global.requests_per_second", 10000)
	v.SetDefault("security.rate_limit.global.max_concurrent_requests", 1000)
	v.SetDefault("security.allow_insecure_callbacks", false)

	// Validation defaults
	v.SetDefault("validation.enabled", true)
	v.SetDefault("validation.validate_response", false)
	v.SetDefault("validation.spec_path", "")
	v.SetDefault("validation.max_body_size", 1048576) // 1MB default

	// Multi-tenancy defaults
	v.SetDefault("multi_tenancy.enabled", false)
	v.SetDefault("multi_tenancy.require_mtls", true)
	v.SetDefault("multi_tenancy.initialize_default_roles", true)
	v.SetDefault("multi_tenancy.audit_log_retention_days", 30)
	v.SetDefault("multi_tenancy.skip_auth_paths", []string{
		"/health", "/healthz", "/ready", "/readyz", "/metrics", "/", "/o2ims",
	})
	v.SetDefault("multi_tenancy.default_tenant_quota.max_subscriptions", 100)
	v.SetDefault("multi_tenancy.default_tenant_quota.max_resource_pools", 50)
	v.SetDefault("multi_tenancy.default_tenant_quota.max_deployments", 200)
	v.SetDefault("multi_tenancy.default_tenant_quota.max_users", 20)
	v.SetDefault("multi_tenancy.default_tenant_quota.max_requests_per_minute", 1000)

	// Auth backend defaults
	v.SetDefault("auth.backend", "redis")
	v.SetDefault("auth.keycloak.timeout", "30s")

	// Storage mode defaults
	v.SetDefault("storage_mode", "redis")

	// Frontend plugins defaults (all disabled by default)
	v.SetDefault("frontend_plugins.o2ims.enabled", false)
	v.SetDefault("frontend_plugins.o2dms.enabled", false)
	v.SetDefault("frontend_plugins.o2smo.enabled", false)
	v.SetDefault("frontend_plugins.tmforum.enabled", false)
	v.SetDefault("frontend_plugins.graphql.enabled", false)

	// Certificate lifecycle defaults
	v.SetDefault("cert_lifecycle.enabled", false)
	v.SetDefault("cert_lifecycle.scan_interval", "5m")
	v.SetDefault("cert_lifecycle.renewal_window", "720h")
	v.SetDefault("cert_lifecycle.max_renewal_retries", 3)
	v.SetDefault("cert_lifecycle.renewal_retry_interval", "1h")
	v.SetDefault("cert_lifecycle.vault_pki_path", "pki_int")

	// PostgreSQL defaults
	v.SetDefault("postgres.host", "localhost")
	v.SetDefault("postgres.port", 5432)
	v.SetDefault("postgres.database", "netweave")
	v.SetDefault("postgres.user", "netweave")
	v.SetDefault("postgres.password_env_var", "POSTGRES_PASSWORD")
	v.SetDefault("postgres.ssl_mode", "prefer")
	v.SetDefault("postgres.max_conns", 25)
	v.SetDefault("postgres.min_conns", 5)
}

// Validate validates the configuration and returns an error if any values are invalid.
// This should be called after Load() to ensure the configuration is valid before use.
// It enforces environment-specific requirements for production and staging environments.
//
// Example:
//
//	cfg, err := config.Load("config/config.yaml")
//	if err != nil {
//	    return err
//	}
//	if err := cfg.Validate(); err != nil {
//	    return fmt.Errorf("invalid configuration: %w", err)
//	}
func (c *Config) Validate() error {
	if err := c.validateServer(); err != nil {
		return err
	}

	if err := c.validateRedis(); err != nil {
		return err
	}

	if err := c.validateTLS(); err != nil {
		return err
	}

	if err := c.validateObservability(); err != nil {
		return err
	}

	if err := c.validateSecurity(); err != nil {
		return err
	}

	if err := c.validateStorageMode(); err != nil {
		return err
	}

	if err := c.validateAuth(); err != nil {
		return err
	}

	if err := c.validateEnvironmentRules(); err != nil {
		return err
	}

	return nil
}

// validateEnvironmentRules enforces environment-specific configuration requirements.
func (c *Config) validateEnvironmentRules() error {
	switch c.Environment {
	case EnvProduction:
		return c.validateProductionRules()
	case EnvStaging:
		return c.validateStagingRules()
	case EnvDevelopment:
		// Development has no strict requirements
		return nil
	default:
		// Unknown environment, treat as development
		return nil
	}
}

// validateProductionRules enforces strict requirements for production.
func (c *Config) validateProductionRules() error {
	// TLS/mTLS must be enabled
	if !c.TLS.Enabled {
		return fmt.Errorf("TLS must be enabled in production (environment: %s)", c.Environment)
	}
	if c.TLS.ClientAuth != tlsClientAuthRequireAndVerify {
		return fmt.Errorf(
			"mTLS with require-and-verify must be enabled in production (got: %s)",
			c.TLS.ClientAuth,
		)
	}

	// Rate limiting must be enabled
	if !c.Security.RateLimitEnabled {
		return fmt.Errorf("rate limiting must be enabled in production")
	}

	// Logging must not be in development mode
	if c.Observability.Logging.Development {
		return fmt.Errorf("logging development mode must be disabled in production")
	}
	if c.Observability.Logging.Level == "debug" {
		return fmt.Errorf("debug logging level is not recommended for production")
	}

	// Response validation should be disabled for performance
	if c.Validation.ValidateResponse {
		return fmt.Errorf("response validation should be disabled in production for performance")
	}

	// CORS should typically be disabled or restricted
	if c.Security.EnableCORS && len(c.Security.AllowedOrigins) == 0 {
		return fmt.Errorf("CORS enabled in production but allowed_origins is empty")
	}

	// SSRF protection must not be disabled in production
	if c.Security.DisableSSRFProtection {
		return fmt.Errorf("SSRF protection must not be disabled in production")
	}

	// Insecure (HTTP) webhook callbacks must not be allowed in production
	if c.Security.AllowInsecureCallbacks {
		return fmt.Errorf("insecure (HTTP) webhook callbacks must not be allowed in production")
	}

	// Rate limiter fail-open is unsafe for write endpoints in production.
	// Reject any configuration that declares fail_mode: open on a write method.
	if err := c.validateRateLimitFailMode(); err != nil {
		return err
	}

	return nil
}

// validateRateLimitFailMode enforces issue #479: fail_mode "open" is never
// acceptable on write endpoints (POST/PUT/PATCH/DELETE) in production.
// A top-level rate_limit.fail_mode=open is also rejected because it would
// apply to every write path.
func (c *Config) validateRateLimitFailMode() error {
	switch c.Security.RateLimit.FailMode {
	case "", "open", "closed":
	default:
		return fmt.Errorf(
			"invalid rate_limit.fail_mode %q (must be 'open', 'closed', or empty)",
			c.Security.RateLimit.FailMode,
		)
	}

	if c.Security.RateLimit.FailMode == "open" {
		return fmt.Errorf(
			"rate_limit.fail_mode=open is not permitted in production; use 'closed' for writes",
		)
	}

	for _, ep := range c.Security.RateLimit.PerEndpoint {
		switch ep.FailMode {
		case "", "open", "closed":
		default:
			return fmt.Errorf(
				"invalid rate_limit.endpoints[%s %s].fail_mode %q (must be 'open', 'closed', or empty)",
				ep.Method, ep.Path, ep.FailMode,
			)
		}
		if ep.FailMode != "open" {
			continue
		}
		method := strings.ToUpper(ep.Method)
		if method == http.MethodPost || method == http.MethodPut ||
			method == http.MethodPatch || method == http.MethodDelete {
			return fmt.Errorf(
				"rate_limit.endpoints[%s %s].fail_mode=open is not permitted in production for write methods",
				ep.Method, ep.Path,
			)
		}
	}

	return nil
}

// validateStagingRules enforces requirements for staging.
func (c *Config) validateStagingRules() error {
	// TLS should be enabled
	if !c.TLS.Enabled {
		return fmt.Errorf("TLS should be enabled in staging (environment: %s)", c.Environment)
	}

	// Rate limiting should be enabled
	if !c.Security.RateLimitEnabled {
		return fmt.Errorf("rate limiting should be enabled in staging")
	}

	return nil
}

// validateServer validates the server configuration.
func (c *Config) validateServer() error {
	ports := map[string]int{
		"server.port":         c.Server.Port,
		"server.o2_port":      c.Server.O2Port,
		"server.tmf_port":     c.Server.TMFPort,
		"server.graphql_port": c.Server.GraphQLPort,
	}

	for name, port := range ports {
		if port < 1 || port > 65535 {
			return fmt.Errorf("invalid %s: %d (must be 1-65535)", name, port)
		}
	}

	// Check for duplicate ports
	if err := c.validateNoDuplicatePorts(ports); err != nil {
		return err
	}

	if c.Server.GinMode != "debug" && c.Server.GinMode != "release" && c.Server.GinMode != "test" {
		return fmt.Errorf("invalid gin_mode: %s (must be debug, release, or test)", c.Server.GinMode)
	}

	return nil
}

// validateNoDuplicatePorts ensures all server ports are unique.
func (c *Config) validateNoDuplicatePorts(ports map[string]int) error {
	seen := make(map[int]string)
	for name, port := range ports {
		if existing, ok := seen[port]; ok {
			return fmt.Errorf("duplicate server port %d used by both %s and %s", port, existing, name)
		}
		seen[port] = name
	}
	return nil
}

// validateRedis validates the Redis configuration.
func (c *Config) validateRedis() error {
	if c.Redis.Mode != "standalone" && c.Redis.Mode != "sentinel" && c.Redis.Mode != "cluster" {
		return fmt.Errorf("invalid redis mode: %s (must be standalone, sentinel, or cluster)", c.Redis.Mode)
	}

	if len(c.Redis.Addresses) == 0 {
		return fmt.Errorf("redis addresses cannot be empty")
	}

	if c.Redis.Mode == "sentinel" && c.Redis.MasterName == "" {
		return fmt.Errorf("redis master_name is required for sentinel mode")
	}

	if c.Redis.DB < 0 || c.Redis.DB > 15 {
		return fmt.Errorf("invalid redis db: %d (must be 0-15)", c.Redis.DB)
	}

	return nil
}

// validateTLS validates the TLS configuration.
func (c *Config) validateTLS() error {
	if !c.TLS.Enabled {
		return nil
	}

	if err := c.validateTLSFiles(); err != nil {
		return err
	}

	if err := c.validateTLSClientAuth(); err != nil {
		return err
	}

	if c.TLS.MinVersion != "1.2" && c.TLS.MinVersion != "1.3" {
		return fmt.Errorf("invalid tls min_version: %s (must be 1.2 or 1.3)", c.TLS.MinVersion)
	}

	return nil
}

// validateTLSFiles validates TLS certificate and key files.
func (c *Config) validateTLSFiles() error {
	if c.TLS.CertFile == "" {
		return fmt.Errorf("tls cert_file is required when TLS is enabled")
	}

	if c.TLS.KeyFile == "" {
		return fmt.Errorf("tls key_file is required when TLS is enabled")
	}

	if _, err := os.Stat(c.TLS.CertFile); os.IsNotExist(err) {
		return fmt.Errorf("tls cert_file does not exist: %s", c.TLS.CertFile)
	}

	if _, err := os.Stat(c.TLS.KeyFile); os.IsNotExist(err) {
		return fmt.Errorf("tls key_file does not exist: %s", c.TLS.KeyFile)
	}

	return nil
}

// validateTLSClientAuth validates TLS client authentication settings.
func (c *Config) validateTLSClientAuth() error {
	validModes := map[string]bool{
		tlsClientAuthNone:             true,
		tlsClientAuthRequest:          true,
		tlsClientAuthRequire:          true,
		tlsClientAuthVerify:           true,
		tlsClientAuthRequireAndVerify: true,
	}

	if !validModes[c.TLS.ClientAuth] {
		return fmt.Errorf("invalid tls client_auth: %s", c.TLS.ClientAuth)
	}

	if c.TLS.ClientAuth == tlsClientAuthNone {
		return nil
	}

	if c.TLS.CAFile == "" {
		return fmt.Errorf("tls ca_file is required when client authentication is enabled")
	}

	if _, err := os.Stat(c.TLS.CAFile); os.IsNotExist(err) {
		return fmt.Errorf("tls ca_file does not exist: %s", c.TLS.CAFile)
	}

	return nil
}

// validateObservability validates the observability configuration.
func (c *Config) validateObservability() error {
	if err := c.validateLogging(); err != nil {
		return err
	}

	if err := c.validateMetrics(); err != nil {
		return err
	}

	if err := c.validateTracing(); err != nil {
		return err
	}

	return nil
}

// validateLogging validates the logging configuration.
func (c *Config) validateLogging() error {
	validLogLevels := map[string]bool{
		"debug": true, "info": true, "warn": true, "error": true, "fatal": true,
	}
	if !validLogLevels[c.Observability.Logging.Level] {
		return fmt.Errorf("invalid logging level: %s", c.Observability.Logging.Level)
	}

	if c.Observability.Logging.Format != "json" && c.Observability.Logging.Format != "console" {
		return fmt.Errorf("invalid logging format: %s (must be json or console)", c.Observability.Logging.Format)
	}

	return nil
}

// validateMetrics validates the metrics configuration.
func (c *Config) validateMetrics() error {
	if !c.Observability.Metrics.Enabled {
		return nil
	}

	if c.Observability.Metrics.Path == "" {
		return fmt.Errorf("metrics path cannot be empty when metrics are enabled")
	}

	if c.Observability.Metrics.Port < 0 || c.Observability.Metrics.Port > 65535 {
		return fmt.Errorf("invalid metrics port: %d", c.Observability.Metrics.Port)
	}

	return nil
}

// validateTracing validates the tracing configuration.
func (c *Config) validateTracing() error {
	if !c.Observability.Tracing.Enabled {
		return nil
	}

	validProviders := map[string]bool{"jaeger": true, "zipkin": true, "otlp": true}
	if !validProviders[c.Observability.Tracing.Provider] {
		return fmt.Errorf("invalid tracing provider: %s", c.Observability.Tracing.Provider)
	}

	if c.Observability.Tracing.Endpoint == "" {
		return fmt.Errorf("tracing endpoint is required when tracing is enabled")
	}

	if c.Observability.Tracing.SamplingRate < 0.0 || c.Observability.Tracing.SamplingRate > 1.0 {
		return fmt.Errorf("invalid tracing sampling_rate: %f (must be 0.0-1.0)", c.Observability.Tracing.SamplingRate)
	}

	return nil
}

// validateSecurity validates the security configuration.
func (c *Config) validateSecurity() error {
	if !c.Security.RateLimitEnabled {
		return nil
	}

	if err := c.validateTenantRateLimit(); err != nil {
		return err
	}
	if err := c.validateGlobalRateLimit(); err != nil {
		return err
	}
	return c.validateEndpointRateLimits()
}

// validateTenantRateLimit validates per-tenant rate limit configuration.
func (c *Config) validateTenantRateLimit() error {
	if c.Security.RateLimit.PerTenant.RequestsPerSecond < 0 {
		return fmt.Errorf(
			"invalid tenant requests_per_second: %d (must be >= 0)",
			c.Security.RateLimit.PerTenant.RequestsPerSecond,
		)
	}
	if c.Security.RateLimit.PerTenant.BurstSize < 0 {
		return fmt.Errorf(
			"invalid tenant burst_size: %d (must be >= 0)",
			c.Security.RateLimit.PerTenant.BurstSize,
		)
	}
	return nil
}

// validateGlobalRateLimit validates global rate limit configuration.
func (c *Config) validateGlobalRateLimit() error {
	if c.Security.RateLimit.Global.RequestsPerSecond < 0 {
		return fmt.Errorf(
			"invalid global requests_per_second: %d (must be >= 0)",
			c.Security.RateLimit.Global.RequestsPerSecond,
		)
	}
	if c.Security.RateLimit.Global.MaxConcurrentRequests < 0 {
		return fmt.Errorf(
			"invalid global max_concurrent_requests: %d (must be >= 0)",
			c.Security.RateLimit.Global.MaxConcurrentRequests,
		)
	}
	return nil
}

// validateEndpointRateLimits validates per-endpoint rate limit configuration.
func (c *Config) validateEndpointRateLimits() error {
	for i, ep := range c.Security.RateLimit.PerEndpoint {
		if ep.Path == "" {
			return fmt.Errorf("endpoint[%d] path cannot be empty", i)
		}
		if ep.Method == "" {
			return fmt.Errorf("endpoint[%d] method cannot be empty", i)
		}
		if ep.RequestsPerSecond < 0 {
			return fmt.Errorf("endpoint[%d] requests_per_second: %d (must be >= 0)", i, ep.RequestsPerSecond)
		}
		if ep.BurstSize < 0 {
			return fmt.Errorf("endpoint[%d] burst_size: %d (must be >= 0)", i, ep.BurstSize)
		}
	}
	return nil
}

// validateAuth validates auth backend configuration.
// validateStorageMode validates the storage mode and PostgreSQL configuration.
func (c *Config) validateStorageMode() error {
	// Default to "redis" for backward compatibility with configs that don't set storage_mode.
	if c.StorageMode == "" {
		c.StorageMode = "redis"
	}

	validModes := map[string]bool{"redis": true, "postgres": true, "dual": true}
	if !validModes[c.StorageMode] {
		return fmt.Errorf("storage_mode: '%s' (must be 'redis', 'postgres', or 'dual')", c.StorageMode)
	}

	if c.StorageMode == "postgres" || c.StorageMode == "dual" {
		if err := c.Postgres.Validate(); err != nil {
			return fmt.Errorf("postgres config: %w", err)
		}
	}

	return nil
}

func (c *Config) validateAuth() error {
	// Validate backend selection
	if c.Auth.Backend != "redis" && c.Auth.Backend != "keycloak" && c.Auth.Backend != "postgres" {
		return fmt.Errorf("auth.backend: '%s' (must be 'redis', 'postgres', or 'keycloak')", c.Auth.Backend)
	}

	// Validate Keycloak config if backend is keycloak
	if c.Auth.Backend == "keycloak" {
		return c.validateKeycloakConfig()
	}

	return nil
}

// validateKeycloakConfig validates Keycloak-specific configuration.
func (c *Config) validateKeycloakConfig() error {
	if c.Auth.Keycloak.BaseURL == "" {
		return fmt.Errorf("auth.keycloak.base_url is required when backend is keycloak")
	}
	if c.Auth.Keycloak.Realm == "" {
		return fmt.Errorf("auth.keycloak.realm is required when backend is keycloak")
	}
	if c.Auth.Keycloak.ClientID == "" {
		return fmt.Errorf("auth.keycloak.client_id is required when backend is keycloak")
	}

	// Validate client secret (from config or env var)
	if _, err := c.Auth.Keycloak.GetClientSecret(); err != nil {
		return fmt.Errorf("auth.keycloak client secret: %w", err)
	}

	if c.Auth.Keycloak.AdminUsername == "" {
		return fmt.Errorf("auth.keycloak.admin_username is required when backend is keycloak")
	}

	// Validate admin password (from config or env var)
	if _, err := c.Auth.Keycloak.GetAdminPassword(); err != nil {
		return fmt.Errorf("auth.keycloak admin password: %w", err)
	}

	if c.Auth.Keycloak.Timeout <= 0 {
		return fmt.Errorf("auth.keycloak.timeout: %s (must be > 0)", c.Auth.Keycloak.Timeout)
	}

	return nil
}
