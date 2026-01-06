// Package config provides configuration management for the O2-IMS Gateway.
// It loads configuration from YAML files and environment variables using Viper,
// with support for hot-reloading and validation.
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

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
	Server        ServerConfig        `mapstructure:"server"`
	Redis         RedisConfig         `mapstructure:"redis"`
	Kubernetes    KubernetesConfig    `mapstructure:"kubernetes"`
	TLS           TLSConfig           `mapstructure:"tls"`
	Observability ObservabilityConfig `mapstructure:"observability"`
	Security      SecurityConfig      `mapstructure:"security"`
	Validation    ValidationConfig    `mapstructure:"validation"`
}

// ServerConfig contains HTTP server configuration.
type ServerConfig struct {
	// Host is the network interface to bind to (e.g., "0.0.0.0", "localhost")
	Host string `mapstructure:"host"`

	// Port is the HTTP server port (default: 8080)
	Port int `mapstructure:"port"`

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

	// Password for Redis authentication (optional)
	Password string `mapstructure:"password"`

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

	// RateLimitRequests is the maximum requests per window
	RateLimitRequests int `mapstructure:"rate_limit_requests"`

	// RateLimitWindow is the rate limit time window
	RateLimitWindow time.Duration `mapstructure:"rate_limit_window"`
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
}

// Load loads configuration from the specified file path and environment variables.
// Environment variables override file values and should be prefixed with NETWEAVE_
// (e.g., NETWEAVE_SERVER_PORT=8080).
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

	// Set configuration file
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

	return &cfg, nil
}

// setDefaults sets default values for all configuration options.
func setDefaults(v *viper.Viper) {
	// Server defaults
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)
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
	v.SetDefault("security.rate_limit_requests", 100)
	v.SetDefault("security.rate_limit_window", "1m")

	// Validation defaults
	v.SetDefault("validation.enabled", true)
	v.SetDefault("validation.validate_response", false)
	v.SetDefault("validation.spec_path", "")
}

// Validate validates the configuration and returns an error if any values are invalid.
// This should be called after Load() to ensure the configuration is valid before use.
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

	return nil
}

// validateServer validates the server configuration.
func (c *Config) validateServer() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d (must be 1-65535)", c.Server.Port)
	}

	if c.Server.GinMode != "debug" && c.Server.GinMode != "release" && c.Server.GinMode != "test" {
		return fmt.Errorf("invalid gin_mode: %s (must be debug, release, or test)", c.Server.GinMode)
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
	if c.Security.RateLimitEnabled {
		if c.Security.RateLimitRequests < 1 {
			return fmt.Errorf("invalid rate_limit_requests: %d (must be > 0)", c.Security.RateLimitRequests)
		}

		if c.Security.RateLimitWindow < time.Second {
			return fmt.Errorf("invalid rate_limit_window: %s (must be >= 1s)", c.Security.RateLimitWindow)
		}
	}

	return nil
}
