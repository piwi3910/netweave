# Configuration Reference

Complete reference for all configuration options in the O2-IMS Gateway.

## Table of Contents

- [Configuration File Structure](#configuration-file-structure)
- [Server](#server)
- [Redis](#redis)
- [Kubernetes](#kubernetes)
- [TLS](#tls)
- [Observability](#observability)
- [Security](#security)
- [Validation](#validation)
- [Multi-Tenancy](#multi-tenancy)
- [Cache](#cache)
- [Environment Variables](#environment-variables)

## Configuration File Structure

```yaml
# config.yaml - Complete configuration template
server:
  # Server configuration
redis:
  # Redis storage configuration
kubernetes:
  # Kubernetes adapter configuration
tls:
  # TLS/mTLS security configuration
observability:
  # Logging, metrics, tracing
security:
  # CORS, rate limiting
validation:
  # Request/response validation
multi_tenancy:
  # Multi-tenancy and RBAC
cache:
  # Caching strategy (planned)
```

## Server

HTTP server configuration.

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 120s
  shutdown_timeout: 30s
  max_header_bytes: 1048576
  gin_mode: release
```

| Field | Type | Default | Description | Validation |
|-------|------|---------|-------------|------------|
| `host` | string | `"0.0.0.0"` | Server bind address | Valid IP or hostname |
| `port` | int | `8080` | HTTP(S) port | 1-65535 |
| `read_timeout` | duration | `30s` | Request read timeout | > 0 |
| `write_timeout` | duration | `30s` | Response write timeout | > 0 |
| `idle_timeout` | duration | `120s` | Keep-alive idle timeout | > 0 |
| `shutdown_timeout` | duration | `30s` | Graceful shutdown timeout | > 0 |
| `max_header_bytes` | int | `1048576` | Max header size (bytes) | > 0 |
| `gin_mode` | string | `"release"` | Gin framework mode | `debug`, `release`, `test` |

**Environment Variables:**
```bash
NETWEAVE_SERVER_HOST
NETWEAVE_SERVER_PORT
NETWEAVE_SERVER_READ_TIMEOUT
NETWEAVE_SERVER_WRITE_TIMEOUT
NETWEAVE_SERVER_IDLE_TIMEOUT
NETWEAVE_SERVER_SHUTDOWN_TIMEOUT
NETWEAVE_SERVER_MAX_HEADER_BYTES
NETWEAVE_SERVER_GIN_MODE
```

## Redis

Redis storage configuration for subscriptions, authentication, caching, and pub/sub.

```yaml
redis:
  mode: standalone
  addresses:
    - localhost:6379
  master_name: mymaster
  password_env_var: REDIS_PASSWORD
  password_file: /run/secrets/redis-password
  sentinel_password_env_var: SENTINEL_PASSWORD
  sentinel_password_file: /run/secrets/sentinel-password
  db: 0
  pool_size: 10
  min_idle_conns: 2
  max_retries: 3
  dial_timeout: 5s
  read_timeout: 3s
  write_timeout: 3s
  pool_timeout: 4s
  idle_timeout: 5m
  max_conn_age: 0
  enable_tls: false
  tls_insecure_skip_verify: false
```

| Field | Type | Default | Description | Validation |
|-------|------|---------|-------------|------------|
| `mode` | string | `"standalone"` | Redis mode | `standalone`, `sentinel`, `cluster` |
| `addresses` | []string | `["localhost:6379"]` | Redis/Sentinel addresses | Valid `host:port` |
| `master_name` | string | `""` | Sentinel master name | Required for sentinel mode |
| `password_env_var` | string | `""` | Env var with Redis password | |
| `password_file` | string | `""` | File with Redis password | |
| `password` | string | `""` | **DEPRECATED** Direct password | Not recommended |
| `sentinel_password_env_var` | string | `""` | Env var with Sentinel password | |
| `sentinel_password_file` | string | `""` | File with Sentinel password | |
| `sentinel_password` | string | `""` | **DEPRECATED** Direct password | Not recommended |
| `db` | int | `0` | Database number | 0-15 |
| `pool_size` | int | `10` | Max connections | > 0 |
| `min_idle_conns` | int | `0` | Min idle connections | >= 0 |
| `max_retries` | int | `3` | Max retry attempts | >= 0 |
| `dial_timeout` | duration | `5s` | Connection timeout | > 0 |
| `read_timeout` | duration | `3s` | Socket read timeout | > 0 |
| `write_timeout` | duration | `3s` | Socket write timeout | > 0 |
| `pool_timeout` | duration | `4s` | Pool wait timeout | > 0 |
| `idle_timeout` | duration | `5m` | Idle connection timeout | > 0 |
| `max_conn_age` | duration | `0` | Max connection age (0=unlimited) | >= 0 |
| `enable_tls` | bool | `false` | Enable TLS for Redis | |
| `tls_insecure_skip_verify` | bool | `false` | Skip TLS verification | |

**Environment Variables:**
```bash
NETWEAVE_REDIS_MODE
NETWEAVE_REDIS_ADDRESSES  # Comma-separated
NETWEAVE_REDIS_MASTER_NAME
NETWEAVE_REDIS_PASSWORD_ENV_VAR
NETWEAVE_REDIS_PASSWORD_FILE
NETWEAVE_REDIS_SENTINEL_PASSWORD_ENV_VAR
NETWEAVE_REDIS_SENTINEL_PASSWORD_FILE
NETWEAVE_REDIS_DB
NETWEAVE_REDIS_POOL_SIZE
NETWEAVE_REDIS_MIN_IDLE_CONNS
NETWEAVE_REDIS_MAX_RETRIES
NETWEAVE_REDIS_DIAL_TIMEOUT
NETWEAVE_REDIS_READ_TIMEOUT
NETWEAVE_REDIS_WRITE_TIMEOUT
NETWEAVE_REDIS_POOL_TIMEOUT
NETWEAVE_REDIS_IDLE_TIMEOUT
NETWEAVE_REDIS_MAX_CONN_AGE
NETWEAVE_REDIS_ENABLE_TLS
NETWEAVE_REDIS_TLS_INSECURE_SKIP_VERIFY
```

## Kubernetes

Kubernetes adapter configuration.

```yaml
kubernetes:
  config_path: ""
  context: ""
  namespace: ""
  qps: 50.0
  burst: 100
  timeout: 30s
  enable_watch: true
  watch_resync: 10m
```

| Field | Type | Default | Description | Validation |
|-------|------|---------|-------------|------------|
| `config_path` | string | `""` | Path to kubeconfig | Valid file path or empty |
| `context` | string | `""` | Kubeconfig context | |
| `namespace` | string | `""` | Default namespace | Valid K8s name or empty |
| `qps` | float | `50.0` | API queries per second | > 0 |
| `burst` | int | `100` | API burst limit | > 0 |
| `timeout` | duration | `30s` | API request timeout | > 0 |
| `enable_watch` | bool | `true` | Enable watch | |
| `watch_resync` | duration | `10m` | Watch resync period | > 0 |

**Environment Variables:**
```bash
NETWEAVE_KUBERNETES_CONFIG_PATH
NETWEAVE_KUBERNETES_CONTEXT
NETWEAVE_KUBERNETES_NAMESPACE
NETWEAVE_KUBERNETES_QPS
NETWEAVE_KUBERNETES_BURST
NETWEAVE_KUBERNETES_TIMEOUT
NETWEAVE_KUBERNETES_ENABLE_WATCH
NETWEAVE_KUBERNETES_WATCH_RESYNC
```

## TLS

TLS and mTLS configuration.

```yaml
tls:
  enabled: false
  cert_file: /etc/certs/tls.crt
  key_file: /etc/certs/tls.key
  ca_file: /etc/certs/ca.crt
  client_auth: none
  min_version: "1.3"
  cipher_suites: []
```

| Field | Type | Default | Description | Validation |
|-------|------|---------|-------------|------------|
| `enabled` | bool | `false` | Enable TLS | **Required in production** |
| `cert_file` | string | `""` | Server certificate path | Valid file path if TLS enabled |
| `key_file` | string | `""` | Server key path | Valid file path if TLS enabled |
| `ca_file` | string | `""` | CA certificate path | Valid file path for mTLS |
| `client_auth` | string | `"none"` | Client auth mode | `none`, `request`, `require`, `verify`, `require-and-verify` |
| `min_version` | string | `"1.3"` | Minimum TLS version | `1.2`, `1.3` |
| `cipher_suites` | []string | `[]` | TLS cipher suites | Valid cipher names or empty |

**Client Auth Modes:**
- `none`: No client certificates
- `request`: Request but don't require
- `require`: Require but don't verify
- `verify`: Verify if provided
- `require-and-verify`: Require and verify (production)

**Environment Variables:**
```bash
NETWEAVE_TLS_ENABLED
NETWEAVE_TLS_CERT_FILE
NETWEAVE_TLS_KEY_FILE
NETWEAVE_TLS_CA_FILE
NETWEAVE_TLS_CLIENT_AUTH
NETWEAVE_TLS_MIN_VERSION
NETWEAVE_TLS_CIPHER_SUITES  # Comma-separated
```

## Observability

Logging, metrics, and tracing configuration.

### Logging

```yaml
observability:
  logging:
    level: info
    format: json
    output_paths:
      - stdout
    error_output_paths:
      - stderr
    enable_caller: true
    enable_stacktrace: false
    development: false
```

| Field | Type | Default | Description | Validation |
|-------|------|---------|-------------|------------|
| `level` | string | `"info"` | Log level | `debug`, `info`, `warn`, `error`, `fatal` |
| `format` | string | `"json"` | Log format | `json`, `console` |
| `output_paths` | []string | `["stdout"]` | Log output destinations | Valid paths |
| `error_output_paths` | []string | `["stderr"]` | Error log destinations | Valid paths |
| `enable_caller` | bool | `true` | Include caller info | |
| `enable_stacktrace` | bool | `false` | Include stacktraces | |
| `development` | bool | `false` | Development mode | |

**Environment Variables:**
```bash
NETWEAVE_OBSERVABILITY_LOGGING_LEVEL
NETWEAVE_OBSERVABILITY_LOGGING_FORMAT
NETWEAVE_OBSERVABILITY_LOGGING_ENABLE_CALLER
NETWEAVE_OBSERVABILITY_LOGGING_ENABLE_STACKTRACE
NETWEAVE_OBSERVABILITY_LOGGING_DEVELOPMENT
```

### Metrics

```yaml
observability:
  metrics:
    enabled: true
    path: /metrics
    port: 0
    namespace: netweave
    subsystem: gateway
    enable_go_metrics: true
    enable_process_metrics: true
```

| Field | Type | Default | Description | Validation |
|-------|------|---------|-------------|------------|
| `enabled` | bool | `true` | Enable metrics | |
| `path` | string | `"/metrics"` | Metrics endpoint path | Valid HTTP path |
| `port` | int | `0` | Metrics port (0=main port) | 0 or 1-65535 |
| `namespace` | string | `"netweave"` | Prometheus namespace | |
| `subsystem` | string | `"gateway"` | Prometheus subsystem | |
| `enable_go_metrics` | bool | `true` | Go runtime metrics | |
| `enable_process_metrics` | bool | `true` | Process metrics | |

**Environment Variables:**
```bash
NETWEAVE_OBSERVABILITY_METRICS_ENABLED
NETWEAVE_OBSERVABILITY_METRICS_PATH
NETWEAVE_OBSERVABILITY_METRICS_PORT
NETWEAVE_OBSERVABILITY_METRICS_NAMESPACE
NETWEAVE_OBSERVABILITY_METRICS_SUBSYSTEM
```

### Tracing

```yaml
observability:
  tracing:
    enabled: false
    provider: otlp
    endpoint: http://jaeger:4318
    service_name: netweave-gateway
    sampling_rate: 0.1
    enable_batching: true
    batch_timeout: 5s
```

| Field | Type | Default | Description | Validation |
|-------|------|---------|-------------|------------|
| `enabled` | bool | `false` | Enable tracing | |
| `provider` | string | `"otlp"` | Tracing provider | `jaeger`, `zipkin`, `otlp` |
| `endpoint` | string | `""` | Tracing endpoint URL | Valid URL if enabled |
| `service_name` | string | `"netweave-gateway"` | Service name | |
| `sampling_rate` | float | `0.1` | Sampling rate (0.0-1.0) | 0.0-1.0 |
| `enable_batching` | bool | `true` | Batch spans | |
| `batch_timeout` | duration | `5s` | Batch timeout | > 0 |

**Environment Variables:**
```bash
NETWEAVE_OBSERVABILITY_TRACING_ENABLED
NETWEAVE_OBSERVABILITY_TRACING_PROVIDER
NETWEAVE_OBSERVABILITY_TRACING_ENDPOINT
NETWEAVE_OBSERVABILITY_TRACING_SERVICE_NAME
NETWEAVE_OBSERVABILITY_TRACING_SAMPLING_RATE
NETWEAVE_OBSERVABILITY_TRACING_ENABLE_BATCHING
NETWEAVE_OBSERVABILITY_TRACING_BATCH_TIMEOUT
```

## Security

CORS and rate limiting configuration.

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
  exposed_headers:
    - X-Request-ID
  allow_credentials: false
  max_age: 3600
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
  allow_insecure_callbacks: false
```

### CORS Fields

| Field | Type | Default | Description | Validation |
|-------|------|---------|-------------|------------|
| `enable_cors` | bool | `false` | Enable CORS | Disable in production |
| `allowed_origins` | []string | `["*"]` | Allowed origins | Valid URLs or `*` |
| `allowed_methods` | []string | `[...]` | Allowed methods | Valid HTTP methods |
| `allowed_headers` | []string | `[...]` | Allowed headers | Valid header names |
| `exposed_headers` | []string | `[]` | Exposed headers | Valid header names |
| `allow_credentials` | bool | `false` | Allow credentials | |
| `max_age` | int | `3600` | Preflight cache (seconds) | >= 0 |

### Rate Limit Fields

| Field | Type | Default | Description | Validation |
|-------|------|---------|-------------|------------|
| `rate_limit_enabled` | bool | `true` | Enable rate limiting | **Required in production** |
| `tenant.requests_per_second` | int | `100` | Per-tenant RPS | > 0 |
| `tenant.burst_size` | int | `200` | Per-tenant burst | > 0 |
| `global.requests_per_second` | int | `1000` | Global RPS | > 0 |
| `global.max_concurrent_requests` | int | `500` | Max concurrent | > 0 |
| `endpoints[].path` | string | | Endpoint path | Valid HTTP path |
| `endpoints[].method` | string | | HTTP method | Valid method |
| `endpoints[].requests_per_second` | int | | Endpoint RPS | > 0 |
| `endpoints[].burst_size` | int | | Endpoint burst | > 0 |
| `allow_insecure_callbacks` | bool | `false` | Allow HTTP callbacks | **Must be false in prod** |

**Environment Variables:**
```bash
NETWEAVE_SECURITY_ENABLE_CORS
NETWEAVE_SECURITY_ALLOWED_ORIGINS  # Comma-separated
NETWEAVE_SECURITY_RATE_LIMIT_ENABLED
NETWEAVE_SECURITY_RATE_LIMIT_TENANT_REQUESTS_PER_SECOND
NETWEAVE_SECURITY_RATE_LIMIT_TENANT_BURST_SIZE
NETWEAVE_SECURITY_RATE_LIMIT_GLOBAL_REQUESTS_PER_SECOND
NETWEAVE_SECURITY_RATE_LIMIT_GLOBAL_MAX_CONCURRENT_REQUESTS
NETWEAVE_SECURITY_ALLOW_INSECURE_CALLBACKS
```

## Validation

Request and response validation configuration.

```yaml
validation:
  enabled: true
  validate_response: false
  spec_path: ""
  max_body_size: 1048576
```

| Field | Type | Default | Description | Validation |
|-------|------|---------|-------------|------------|
| `enabled` | bool | `true` | Enable validation | |
| `validate_response` | bool | `false` | Validate responses | Enable in dev only |
| `spec_path` | string | `""` | Custom OpenAPI spec path | Valid file path or empty |
| `max_body_size` | int | `1048576` | Max request body (bytes) | > 0 |

**Environment Variables:**
```bash
NETWEAVE_VALIDATION_ENABLED
NETWEAVE_VALIDATION_VALIDATE_RESPONSE
NETWEAVE_VALIDATION_SPEC_PATH
NETWEAVE_VALIDATION_MAX_BODY_SIZE
```

## Multi-Tenancy

Multi-tenancy and RBAC configuration.

```yaml
multi_tenancy:
  enabled: false
  require_mtls: true
  initialize_default_roles: true
  audit_log_retention_days: 30
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

| Field | Type | Default | Description | Validation |
|-------|------|---------|-------------|------------|
| `enabled` | bool | `false` | Enable multi-tenancy | |
| `require_mtls` | bool | `true` | Require mTLS for auth | |
| `initialize_default_roles` | bool | `true` | Create default roles | |
| `audit_log_retention_days` | int | `30` | Audit log retention | > 0 |
| `skip_auth_paths` | []string | `[]` | Paths to skip auth | Valid HTTP paths |
| `default_tenant_quota.*` | | | Default quotas | |

### Default Tenant Quota Fields

| Field | Type | Default | Description | Validation |
|-------|------|---------|-------------|------------|
| `max_subscriptions` | int | `100` | Max subscriptions | > 0 |
| `max_resource_pools` | int | `50` | Max resource pools | > 0 |
| `max_deployments` | int | `200` | Max deployments | > 0 |
| `max_users` | int | `20` | Max users | > 0 |
| `max_requests_per_minute` | int | `1000` | Rate limit | > 0 |

**Environment Variables:**
```bash
NETWEAVE_MULTI_TENANCY_ENABLED
NETWEAVE_MULTI_TENANCY_REQUIRE_MTLS
NETWEAVE_MULTI_TENANCY_INITIALIZE_DEFAULT_ROLES
NETWEAVE_MULTI_TENANCY_AUDIT_LOG_RETENTION_DAYS
NETWEAVE_MULTI_TENANCY_SKIP_AUTH_PATHS  # Comma-separated
NETWEAVE_MULTI_TENANCY_DEFAULT_TENANT_QUOTA_MAX_SUBSCRIPTIONS
NETWEAVE_MULTI_TENANCY_DEFAULT_TENANT_QUOTA_MAX_RESOURCE_POOLS
NETWEAVE_MULTI_TENANCY_DEFAULT_TENANT_QUOTA_MAX_DEPLOYMENTS
NETWEAVE_MULTI_TENANCY_DEFAULT_TENANT_QUOTA_MAX_USERS
NETWEAVE_MULTI_TENANCY_DEFAULT_TENANT_QUOTA_MAX_REQUESTS_PER_MINUTE
```

## Cache

*Planned feature - not yet fully implemented*

Caching strategy configuration.

```yaml
cache:
  enabled: true
  ttl:
    resource_pools: 60s
    resource_types: 300s
    resources: 30s
    deployment_managers: 60s
  invalidation:
    on_create: true
    on_update: true
    on_delete: true
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `true` | Enable caching |
| `ttl.resource_pools` | duration | `60s` | Resource pool cache TTL |
| `ttl.resource_types` | duration | `300s` | Resource type cache TTL |
| `ttl.resources` | duration | `30s` | Resource cache TTL |
| `ttl.deployment_managers` | duration | `60s` | DM cache TTL |
| `invalidation.on_create` | bool | `true` | Invalidate on create |
| `invalidation.on_update` | bool | `true` | Invalidate on update |
| `invalidation.on_delete` | bool | `true` | Invalidate on delete |

## Environment Variables

### Naming Convention

All environment variables use the `NETWEAVE_` prefix and follow this pattern:
- Prefix: `NETWEAVE_`
- Nested keys: Separate with underscores (`_`)
- Arrays: Comma-separated values
- Booleans: `true` or `false` (case-insensitive)
- Durations: Go duration format (`30s`, `5m`, `1h`)

### Examples

```bash
# Simple value
NETWEAVE_SERVER_PORT=8443

# Nested value
NETWEAVE_OBSERVABILITY_LOGGING_LEVEL=info

# Array (comma-separated)
NETWEAVE_REDIS_ADDRESSES="redis1:6379,redis2:6379,redis3:6379"
NETWEAVE_SECURITY_ALLOWED_ORIGINS="https://smo.example.com,https://dashboard.example.com"

# Boolean
NETWEAVE_TLS_ENABLED=true
NETWEAVE_MULTI_TENANCY_ENABLED=false

# Duration
NETWEAVE_SERVER_READ_TIMEOUT=30s
NETWEAVE_KUBERNETES_TIMEOUT=1m
```

### Complete Environment Variable List

**Server:**
```bash
NETWEAVE_SERVER_HOST
NETWEAVE_SERVER_PORT
NETWEAVE_SERVER_READ_TIMEOUT
NETWEAVE_SERVER_WRITE_TIMEOUT
NETWEAVE_SERVER_IDLE_TIMEOUT
NETWEAVE_SERVER_SHUTDOWN_TIMEOUT
NETWEAVE_SERVER_MAX_HEADER_BYTES
NETWEAVE_SERVER_GIN_MODE
```

**Redis:**
```bash
NETWEAVE_REDIS_MODE
NETWEAVE_REDIS_ADDRESSES
NETWEAVE_REDIS_MASTER_NAME
NETWEAVE_REDIS_PASSWORD_ENV_VAR
NETWEAVE_REDIS_PASSWORD_FILE
NETWEAVE_REDIS_SENTINEL_PASSWORD_ENV_VAR
NETWEAVE_REDIS_SENTINEL_PASSWORD_FILE
NETWEAVE_REDIS_DB
NETWEAVE_REDIS_POOL_SIZE
NETWEAVE_REDIS_MIN_IDLE_CONNS
NETWEAVE_REDIS_MAX_RETRIES
NETWEAVE_REDIS_DIAL_TIMEOUT
NETWEAVE_REDIS_READ_TIMEOUT
NETWEAVE_REDIS_WRITE_TIMEOUT
NETWEAVE_REDIS_POOL_TIMEOUT
NETWEAVE_REDIS_IDLE_TIMEOUT
NETWEAVE_REDIS_MAX_CONN_AGE
NETWEAVE_REDIS_ENABLE_TLS
NETWEAVE_REDIS_TLS_INSECURE_SKIP_VERIFY
```

**Kubernetes:**
```bash
NETWEAVE_KUBERNETES_CONFIG_PATH
NETWEAVE_KUBERNETES_CONTEXT
NETWEAVE_KUBERNETES_NAMESPACE
NETWEAVE_KUBERNETES_QPS
NETWEAVE_KUBERNETES_BURST
NETWEAVE_KUBERNETES_TIMEOUT
NETWEAVE_KUBERNETES_ENABLE_WATCH
NETWEAVE_KUBERNETES_WATCH_RESYNC
```

**TLS:**
```bash
NETWEAVE_TLS_ENABLED
NETWEAVE_TLS_CERT_FILE
NETWEAVE_TLS_KEY_FILE
NETWEAVE_TLS_CA_FILE
NETWEAVE_TLS_CLIENT_AUTH
NETWEAVE_TLS_MIN_VERSION
NETWEAVE_TLS_CIPHER_SUITES
```

**Observability:**
```bash
NETWEAVE_OBSERVABILITY_LOGGING_LEVEL
NETWEAVE_OBSERVABILITY_LOGGING_FORMAT
NETWEAVE_OBSERVABILITY_LOGGING_ENABLE_CALLER
NETWEAVE_OBSERVABILITY_LOGGING_ENABLE_STACKTRACE
NETWEAVE_OBSERVABILITY_LOGGING_DEVELOPMENT
NETWEAVE_OBSERVABILITY_METRICS_ENABLED
NETWEAVE_OBSERVABILITY_METRICS_PATH
NETWEAVE_OBSERVABILITY_METRICS_PORT
NETWEAVE_OBSERVABILITY_METRICS_NAMESPACE
NETWEAVE_OBSERVABILITY_METRICS_SUBSYSTEM
NETWEAVE_OBSERVABILITY_TRACING_ENABLED
NETWEAVE_OBSERVABILITY_TRACING_PROVIDER
NETWEAVE_OBSERVABILITY_TRACING_ENDPOINT
NETWEAVE_OBSERVABILITY_TRACING_SERVICE_NAME
NETWEAVE_OBSERVABILITY_TRACING_SAMPLING_RATE
NETWEAVE_OBSERVABILITY_TRACING_ENABLE_BATCHING
NETWEAVE_OBSERVABILITY_TRACING_BATCH_TIMEOUT
```

**Security:**
```bash
NETWEAVE_SECURITY_ENABLE_CORS
NETWEAVE_SECURITY_ALLOWED_ORIGINS
NETWEAVE_SECURITY_RATE_LIMIT_ENABLED
NETWEAVE_SECURITY_RATE_LIMIT_TENANT_REQUESTS_PER_SECOND
NETWEAVE_SECURITY_RATE_LIMIT_TENANT_BURST_SIZE
NETWEAVE_SECURITY_RATE_LIMIT_GLOBAL_REQUESTS_PER_SECOND
NETWEAVE_SECURITY_RATE_LIMIT_GLOBAL_MAX_CONCURRENT_REQUESTS
NETWEAVE_SECURITY_ALLOW_INSECURE_CALLBACKS
```

**Validation:**
```bash
NETWEAVE_VALIDATION_ENABLED
NETWEAVE_VALIDATION_VALIDATE_RESPONSE
NETWEAVE_VALIDATION_SPEC_PATH
NETWEAVE_VALIDATION_MAX_BODY_SIZE
```

**Multi-Tenancy:**
```bash
NETWEAVE_MULTI_TENANCY_ENABLED
NETWEAVE_MULTI_TENANCY_REQUIRE_MTLS
NETWEAVE_MULTI_TENANCY_INITIALIZE_DEFAULT_ROLES
NETWEAVE_MULTI_TENANCY_AUDIT_LOG_RETENTION_DAYS
NETWEAVE_MULTI_TENANCY_SKIP_AUTH_PATHS
NETWEAVE_MULTI_TENANCY_DEFAULT_TENANT_QUOTA_MAX_SUBSCRIPTIONS
NETWEAVE_MULTI_TENANCY_DEFAULT_TENANT_QUOTA_MAX_RESOURCE_POOLS
NETWEAVE_MULTI_TENANCY_DEFAULT_TENANT_QUOTA_MAX_DEPLOYMENTS
NETWEAVE_MULTI_TENANCY_DEFAULT_TENANT_QUOTA_MAX_USERS
NETWEAVE_MULTI_TENANCY_DEFAULT_TENANT_QUOTA_MAX_REQUESTS_PER_MINUTE
```

## See Also

- [Configuration Overview](README.md)
- [Configuration Basics](basics.md)
- [Environment Configuration](environments.md)
- [Security Configuration](security.md)
- [Adapter Configuration](adapters.md)
