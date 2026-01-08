# Configuration Guide

Complete configuration reference for the O2-IMS Gateway with environment-specific settings and best practices.

## Table of Contents

- [Overview](#overview)
- [Environment Detection](#environment-detection)
- [Configuration Files](#configuration-files)
- [Configuration Reference](#configuration-reference)
- [Environment-Specific Settings](#environment-specific-settings)
- [Secrets Management](#secrets-management)
- [Kubernetes Deployment](#kubernetes-deployment)
- [Validation Rules](#validation-rules)
- [Best Practices](#best-practices)
- [Troubleshooting](#troubleshooting)

## Overview

The O2-IMS Gateway uses a hierarchical configuration system that supports:

- **Environment-specific configurations** (dev/staging/prod)
- **Automatic environment detection** via `NETWEAVE_ENV`
- **Environment variable overrides** with `NETWEAVE_` prefix
- **Validation with environment-specific rules**
- **Hot-reloadable configuration** (where supported)

### Configuration Priority

Configuration values are resolved in this order (highest to lowest priority):

1. Environment variables (`NETWEAVE_SERVER_PORT`, `NETWEAVE_REDIS_PASSWORD`, etc.)
2. Explicit config file path (`--config=/path/to/config.yaml`)
3. `NETWEAVE_CONFIG` environment variable
4. Environment-specific config via `NETWEAVE_ENV` (e.g., `config/config.prod.yaml`)
5. Default config file (`config/config.yaml`)

## Environment Detection

The gateway automatically detects the environment using the following methods:

### Method 1: NETWEAVE_ENV Environment Variable

```bash
# Development
export NETWEAVE_ENV=dev
./bin/gateway

# Staging
export NETWEAVE_ENV=staging
./bin/gateway

# Production
export NETWEAVE_ENV=prod
./bin/gateway
```

### Method 2: Config File Path Pattern

```bash
# Auto-detects "dev" environment
./bin/gateway --config=config/config.dev.yaml

# Auto-detects "prod" environment
./bin/gateway --config=config/config.prod.yaml
```

### Method 3: Makefile Targets

```bash
# Development
make run-dev

# Staging
make run-staging

# Production
make run-prod
```

## Configuration Files

### Development (`config/config.dev.yaml`)

**Purpose**: Local development with minimal security

**Key Features**:
- HTTP only (no TLS)
- Debug logging with console format
- Local Redis without authentication
- CORS enabled for frontend development
- No rate limiting
- Response validation enabled for debugging

**Usage**:
```bash
NETWEAVE_ENV=dev ./bin/gateway
# or
make run-dev
```

### Staging (`config/config.staging.yaml`)

**Purpose**: Pre-production testing with full security

**Key Features**:
- TLS/mTLS enabled
- Redis Sentinel with authentication
- Info-level logging with JSON format
- Rate limiting enabled (moderate limits)
- Tracing enabled (50% sampling)
- Multi-tenancy enabled

**Usage**:
```bash
NETWEAVE_ENV=staging ./bin/gateway
# or
make run-staging
```

### Production (`config/config.prod.yaml`)

**Purpose**: Production deployment with maximum security

**Key Features**:
- Strict TLS/mTLS (require-and-verify)
- Redis Sentinel with TLS
- Info-level logging (JSON only)
- High rate limits
- Tracing (10% sampling for efficiency)
- Multi-tenancy with RBAC
- No CORS
- Response validation disabled for performance

**Usage**:
```bash
NETWEAVE_ENV=prod ./bin/gateway
# or
make run-prod
```

## Configuration Reference

### Server Configuration

```yaml
server:
  host: "0.0.0.0"              # Bind address
  port: 8443                    # HTTP(S) port
  read_timeout: 30s             # Request read timeout
  write_timeout: 30s            # Response write timeout
  idle_timeout: 120s            # Keep-alive idle timeout
  shutdown_timeout: 30s         # Graceful shutdown timeout
  max_header_bytes: 1048576     # Max header size (1MB)
  gin_mode: release             # Gin mode: debug, release, test
```

### Redis Configuration

```yaml
redis:
  mode: sentinel                # Mode: standalone, sentinel, cluster
  addresses:                    # Redis/Sentinel addresses
    - redis-sentinel-0:26379
    - redis-sentinel-1:26379
    - redis-sentinel-2:26379
  master_name: mymaster         # Sentinel master name (sentinel mode only)
  password: ${REDIS_PASSWORD}   # Password from env var
  db: 0                         # Database number (0-15)
  pool_size: 50                 # Max connections
  min_idle_conns: 10            # Min idle connections
  max_retries: 5                # Max retry attempts
  dial_timeout: 5s              # Connection timeout
  read_timeout: 3s              # Read timeout
  write_timeout: 3s             # Write timeout
  pool_timeout: 4s              # Pool wait timeout
  idle_timeout: 5m              # Idle connection timeout
  enable_tls: true              # Enable TLS
  tls_insecure_skip_verify: false
```

### Kubernetes Configuration

```yaml
kubernetes:
  config_path: ""               # Path to kubeconfig (empty = in-cluster)
  context: ""                   # Kubeconfig context
  namespace: ""                 # Default namespace (empty = all)
  qps: 100.0                    # API queries per second
  burst: 200                    # API burst limit
  timeout: 30s                  # API request timeout
  enable_watch: true            # Enable watch for real-time updates
  watch_resync: 10m             # Watch cache resync period
```

### TLS/mTLS Configuration

```yaml
tls:
  enabled: true
  cert_file: /etc/certs/tls.crt
  key_file: /etc/certs/tls.key
  ca_file: /etc/certs/ca.crt
  client_auth: require-and-verify  # none, request, require, verify, require-and-verify
  min_version: "1.3"                # TLS 1.2 or 1.3
  cipher_suites: []                 # Empty = Go defaults
```

### Observability Configuration

#### Logging

```yaml
observability:
  logging:
    level: info                   # debug, info, warn, error, fatal
    format: json                  # json, console
    output_paths:
      - stdout
    error_output_paths:
      - stderr
    enable_caller: true           # Add caller information
    enable_stacktrace: false      # Add stacktraces on errors
    development: false            # Development mode (verbose)
```

#### Metrics

```yaml
  metrics:
    enabled: true
    path: /metrics
    port: 0                       # 0 = use main server port
    namespace: netweave
    subsystem: gateway
    enable_go_metrics: true
    enable_process_metrics: true
```

#### Tracing

```yaml
  tracing:
    enabled: true
    provider: otlp                # jaeger, zipkin, otlp
    endpoint: http://jaeger:4318
    service_name: netweave-gateway-prod
    sampling_rate: 0.1            # 0.0 to 1.0
    enable_batching: true
    batch_timeout: 5s
```

### Security Configuration

```yaml
security:
  enable_cors: false
  allowed_origins:
    - https://example.com
  allowed_methods:
    - GET
    - POST
    - PUT
    - PATCH
    - DELETE
  allowed_headers:
    - Content-Type
    - Authorization
    - X-Request-ID
  rate_limit_enabled: true
  rate_limit:
    tenant:
      requests_per_second: 100
      burst_size: 200
    global:
      requests_per_second: 1000
      max_concurrent_requests: 500
    endpoints:
      - path: /o2ims/v1/subscriptions
        method: POST
        requests_per_second: 10
        burst_size: 20
```

### Validation Configuration

```yaml
validation:
  enabled: true
  validate_response: false        # Enable only in dev
  spec_path: ""                   # Custom OpenAPI spec
  max_body_size: 1048576          # 1MB
```

### Multi-Tenancy Configuration

```yaml
multi_tenancy:
  enabled: true
  require_mtls: true
  initialize_default_roles: true
  audit_log_retention_days: 90
  skip_auth_paths:
    - /health
    - /healthz
    - /metrics
  default_tenant_quota:
    max_subscriptions: 100
    max_resource_pools: 50
    max_deployments: 200
    max_users: 20
    max_requests_per_minute: 1000
```

## Environment-Specific Settings

### Development Environment

| Setting | Value | Reason |
|---------|-------|--------|
| TLS | Disabled | Easier local testing |
| Logging Level | `debug` | Verbose debugging |
| Logging Format | `console` | Human-readable |
| Redis Auth | Disabled | Simpler setup |
| Rate Limiting | Disabled | Unrestricted testing |
| CORS | Enabled | Frontend development |
| Response Validation | Enabled | Catch errors early |

### Staging Environment

| Setting | Value | Reason |
|---------|-------|--------|
| TLS | Enabled | Production-like |
| mTLS | Enabled | Security testing |
| Logging Level | `info` | Balanced verbosity |
| Redis | Sentinel | HA testing |
| Rate Limiting | Moderate | Realistic testing |
| Tracing | 50% sampling | Performance testing |
| Multi-Tenancy | Enabled | Feature testing |

### Production Environment

| Setting | Value | Reason |
|---------|-------|--------|
| TLS | Required | Security |
| mTLS | require-and-verify | Strict security |
| Logging Level | `info` | Performance |
| Redis | Sentinel + TLS | High availability |
| Rate Limiting | Enabled | DoS protection |
| Tracing | 10% sampling | Efficiency |
| Response Validation | Disabled | Performance |
| CORS | Disabled | Security |

## Secrets Management

### Environment Variables

**Never hardcode secrets in configuration files.**

```yaml
redis:
  password: ${REDIS_PASSWORD}  # Read from env var
```

### Kubernetes Secrets

```bash
# Create secret
kubectl create secret generic netweave-secrets \
  --from-literal=redis-password="${REDIS_PASSWORD}" \
  --namespace=o2ims-system

# Reference in deployment
env:
  - name: REDIS_PASSWORD
    valueFrom:
      secretKeyRef:
        name: netweave-secrets
        key: redis-password
```

### Secret Redaction

Secrets are automatically redacted from:
- Log messages
- Error messages
- Metrics labels
- Tracing spans

## Kubernetes Deployment

### Using Helm with Environment Values

```bash
# Development
helm install netweave ./helm/netweave \
  --values helm/netweave/values-dev.yaml \
  --namespace o2ims-dev

# Production
helm install netweave ./helm/netweave \
  --values helm/netweave/values-prod.yaml \
  --set image.tag=v1.0.0 \
  --namespace o2ims-prod
```

### ConfigMap Generation

The Helm chart automatically generates ConfigMaps from values:

```yaml
# helm/netweave/values-prod.yaml
environment: prod  # Sets NETWEAVE_ENV=prod

config:
  server:
    port: 8443
    tls:
      enabled: true
```

### Environment Variable Injection

The deployment sets `NETWEAVE_ENV` based on the `environment` value:

```yaml
env:
  - name: NETWEAVE_ENV
    value: "prod"
```

## Validation Rules

### Universal Rules

Applied to all environments:

- Server port must be 1-65535
- Gin mode must be debug/release/test
- Redis mode must be standalone/sentinel/cluster
- Redis DB must be 0-15
- TLS min version must be 1.2 or 1.3
- Log level must be debug/info/warn/error/fatal

### Production-Specific Rules

**Enforced when `NETWEAVE_ENV=prod`:**

- ✅ TLS **must** be enabled
- ✅ mTLS client auth **must** be `require-and-verify`
- ✅ Rate limiting **must** be enabled
- ✅ Logging development mode **must** be disabled
- ✅ Debug logging level **not recommended**
- ✅ Response validation **should** be disabled
- ✅ If CORS enabled, `allowed_origins` **must** be specified

### Staging-Specific Rules

**Enforced when `NETWEAVE_ENV=staging`:**

- ✅ TLS **should** be enabled
- ✅ Rate limiting **should** be enabled

## Best Practices

### Configuration Management

1. **Use environment-specific configs** - Don't modify prod config for dev use
2. **Version control configs** - Track changes to configuration files
3. **Validate before deploy** - Run `make test` to validate configs
4. **Document changes** - Update this file when adding new config options
5. **Review regularly** - Audit configs quarterly for security

### Security

1. **Never commit secrets** - Use env vars or secret managers
2. **Enable mTLS in prod** - Require client certificates
3. **Restrict CORS** - Disable or whitelist specific origins
4. **Enable rate limiting** - Protect against DoS
5. **Use TLS 1.3** - Disable older TLS versions
6. **Rotate credentials** - Change passwords regularly

### Performance

1. **Tune Redis pool size** - Match expected concurrency
2. **Adjust rate limits** - Based on capacity testing
3. **Set appropriate timeouts** - Balance responsiveness vs stability
4. **Disable debug logging in prod** - Use info level
5. **Sample traces wisely** - Lower sampling in high-traffic prod

### Observability

1. **Use JSON logging in prod** - Structured logs for parsing
2. **Enable metrics** - Required for monitoring
3. **Configure tracing** - Essential for troubleshooting
4. **Set appropriate retention** - Balance storage vs compliance
5. **Monitor configuration drift** - Alert on unexpected changes

## Troubleshooting

### Common Issues

#### "TLS must be enabled in production"

**Cause**: Trying to run with `NETWEAVE_ENV=prod` but `tls.enabled: false`

**Solution**:
```yaml
tls:
  enabled: true
  cert_file: /path/to/cert
  key_file: /path/to/key
```

#### "mTLS with require-and-verify must be enabled in production"

**Cause**: Production requires strict mTLS

**Solution**:
```yaml
tls:
  client_auth: require-and-verify
  ca_file: /path/to/ca.crt
```

#### "Config file not found"

**Cause**: Config file doesn't exist or wrong path

**Solution**:
```bash
# Check file exists
ls -la config/config.prod.yaml

# Use explicit path
./bin/gateway --config=/absolute/path/to/config.yaml

# Or set env var
export NETWEAVE_CONFIG=/path/to/config.yaml
```

#### "Redis connection failed"

**Cause**: Wrong Redis address or authentication

**Solution**:
```yaml
redis:
  addresses:
    - correct-redis-host:6379  # Check hostname/port
  password: ${REDIS_PASSWORD}   # Check password
```

#### "Rate limiting must be enabled in production"

**Cause**: Production requires rate limiting

**Solution**:
```yaml
security:
  rate_limit_enabled: true
  rate_limit:
    tenant:
      requests_per_second: 100
```

### Validation Checks

```bash
# Validate development config
NETWEAVE_ENV=dev go run cmd/gateway/main.go --config=config/config.dev.yaml

# Validate production config (will fail if certs don't exist)
NETWEAVE_ENV=prod go run cmd/gateway/main.go --config=config/config.prod.yaml

# Test with environment variable override
NETWEAVE_ENV=prod NETWEAVE_SERVER_PORT=9443 go run cmd/gateway/main.go
```

### Debug Configuration Loading

```bash
# Show which config file is being used
NETWEAVE_ENV=prod ./bin/gateway 2>&1 | grep "config"

# Show all environment variables
env | grep NETWEAVE_

# Validate config without starting server
go run cmd/gateway/main.go --config=config/config.prod.yaml --validate
```

## References

- [Go Configuration with Viper](https://github.com/spf13/viper)
- [Kubernetes ConfigMaps](https://kubernetes.io/docs/concepts/configuration/configmap/)
- [Kubernetes Secrets](https://kubernetes.io/docs/concepts/configuration/secret/)
- [TLS Best Practices](https://wiki.mozilla.org/Security/Server_Side_TLS)
