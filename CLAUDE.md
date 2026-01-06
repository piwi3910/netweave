# O2-IMS Gateway Project - Claude Development Guide

This file provides development guidelines for Claude Code when working on the O2-IMS Gateway project.

## Project Overview

**O2-IMS Gateway** is a production-grade ORAN O2-IMS compliant API gateway that translates O2-IMS requests to Kubernetes API calls, enabling disaggregation of backend infrastructure components.

**Technology Stack:**
- Language: Go 1.23+
- Framework: Gin (HTTP)
- Storage: Redis OSS 7.4+ (Sentinel)
- Container Orchestration: Kubernetes 1.30+
- Service Mesh: Istio 1.23+
- Certificate Management: cert-manager 1.15+

**Architecture:**
- Stateless gateway pods (3+ replicas)
- Redis for subscriptions, caching, and inter-pod communication
- Kubernetes API as source of truth for infrastructure resources
- mTLS everywhere (pod-to-pod, pod-to-external, pod-to-backend)
- Multi-cluster capable via Redis replication

## Code Quality Standards - ZERO TOLERANCE

### 1. Linting - MANDATORY

**All code MUST pass linting without ANY errors or warnings.**

#### Linters Used:
```yaml
golangci-lint:
  - gosec          # Security vulnerabilities
  - revive         # Code style and best practices
  - staticcheck    # Static analysis
  - govet          # Go vet tool
  - errcheck       # Unchecked errors
  - ineffassign    # Ineffective assignments
  - unused         # Unused code
  - gocyclo        # Cyclomatic complexity
  - gofmt          # Code formatting
  - goimports      # Import organization
  - misspell       # Spelling errors
  - godot          # Comment formatting
  - gocritic       # Go best practices
  - prealloc       # Slice preallocation
  - unconvert      # Unnecessary conversions
  - unparam        # Unused function parameters
```

#### Linting Rules - ABSOLUTE REQUIREMENTS:

**❌ NEVER ALLOWED:**
- Disabling linters via `//nolint` comments
- Using `//golangci-lint:disable` directives
- Committing code with linting errors
- Ignoring security warnings from gosec
- Using `interface{}` or `any` without strong justification
- Leaving TODO/FIXME comments without GitHub issues

**✅ ALWAYS REQUIRED:**
- Run `make lint` before every commit
- Fix ALL linting issues (no exceptions)
- If a linting rule is triggered, FIX THE CODE, not the rule
- Document why if a specific pattern is required (rare)
- Use strongly-typed interfaces and structs

#### Running Linters:

```bash
# Run all linters (MUST pass before commit)
make lint

# Auto-fix formatting issues
make fmt

# Security scan only
make security-scan

# All quality checks (lint + test + security)
make quality
```

### 2. Code Security - MANDATORY

**All code MUST be secure by default.**

#### Security Requirements:

**Input Validation:**
- ✅ Validate ALL external inputs (HTTP requests, env vars, config files)
- ✅ Use OpenAPI schema validation for API requests
- ✅ Sanitize inputs before logging (no secrets in logs)
- ✅ Use parameterized queries/commands (prevent injection)

**Authentication & Authorization:**
- ✅ mTLS for all service-to-service communication
- ✅ Validate client certificates in Istio gateway
- ✅ Use Kubernetes RBAC for pod permissions
- ✅ Rotate secrets regularly (cert-manager automation)

**Secrets Management:**
- ✅ NEVER hardcode secrets, passwords, API keys
- ✅ Use Kubernetes Secrets or external secret managers
- ✅ Read secrets from environment variables only
- ✅ Redact secrets in logs and error messages

**Error Handling:**
- ✅ NEVER expose internal errors to external clients
- ✅ Log detailed errors internally, return generic errors externally
- ✅ Use structured logging (no sensitive data in logs)

**Dependencies:**
- ✅ Use `go mod verify` to verify dependencies
- ✅ Run `govulncheck` before releases
- ✅ Keep dependencies updated (monthly security patches)
- ✅ Pin dependency versions in go.mod

#### Security Scanning:

```bash
# Static security analysis
gosec ./...

# Vulnerability scanning
govulncheck ./...

# Container scanning (before deployment)
trivy image o2ims-gateway:latest

# All security checks
make security-scan
```

**Security violations are BLOCKING - code will NOT be merged.**

### 3. Testing Standards - MANDATORY

**All code MUST have comprehensive tests.**

#### Test Coverage Requirements:

- **Unit Tests:** ≥80% coverage for all packages
- **Integration Tests:** All API endpoints
- **E2E Tests:** Critical user flows (subscriptions, resource queries)

#### Test Patterns:

```go
// Table-driven tests (PREFERRED)
func TestSubscriptionStore_Create(t *testing.T) {
    tests := []struct {
        name    string
        sub     *O2Subscription
        wantErr bool
        errType error
    }{
        {
            name: "valid subscription",
            sub: &O2Subscription{
                ID:       "sub-123",
                Callback: "https://smo.example.com/notify",
            },
            wantErr: false,
        },
        {
            name: "duplicate subscription",
            sub: &O2Subscription{
                ID: "sub-123",
            },
            wantErr: true,
            errType: ErrSubscriptionExists,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := store.Create(context.Background(), tt.sub)
            if tt.wantErr {
                require.Error(t, err)
                require.ErrorIs(t, err, tt.errType)
            } else {
                require.NoError(t, err)
            }
        })
    }
}
```

#### Running Tests:

```bash
# Run all tests
make test

# Run tests with coverage
make test-coverage

# Run integration tests
make test-integration

# Run E2E tests
make test-e2e

# Watch mode (development)
make test-watch
```

**Tests MUST pass before commit. No exceptions.**

### 4. Code Style - Go Best Practices

#### Naming Conventions:

- **Packages:** Short, lowercase, no underscores (e.g., `storage`, `cache`, `o2ims`)
- **Files:** Lowercase with underscores (e.g., `subscription_store.go`, `redis_cache.go`)
- **Types:** PascalCase (e.g., `O2Subscription`, `DeploymentManager`)
- **Functions/Methods:** PascalCase for exported, camelCase for unexported
- **Constants:** PascalCase (not SCREAMING_CASE)
- **Interfaces:** `-er` suffix for single-method (e.g., `Storer`, `Cacher`)

#### Code Organization:

```
internal/           # Private application code
  adapter/          # Adapter interface and registry
  adapters/         # Concrete adapter implementations
    k8s/
    mock/
  config/           # Configuration loading
  controller/       # Subscription controller
  o2ims/            # O2-IMS models and handlers
    models/
    handlers/
  server/           # HTTP server setup

pkg/                # Public reusable libraries
  cache/            # Cache abstraction
  storage/          # Storage abstraction
  errors/           # Error types

cmd/                # Entry points
  gateway/          # Main gateway binary
  migrate/          # Migration tools
```

#### Error Handling:

```go
// ✅ GOOD: Wrap errors with context
if err := store.Create(ctx, sub); err != nil {
    return fmt.Errorf("failed to create subscription: %w", err)
}

// ✅ GOOD: Sentinel errors for specific cases
var (
    ErrSubscriptionNotFound = errors.New("subscription not found")
    ErrSubscriptionExists   = errors.New("subscription already exists")
)

// ✅ GOOD: Check error types
if errors.Is(err, ErrSubscriptionNotFound) {
    return http.StatusNotFound
}

// ❌ BAD: Ignoring errors
_ = store.Create(ctx, sub)

// ❌ BAD: Generic error messages
return errors.New("error")
```

#### Context Handling:

```go
// ✅ GOOD: Always pass context as first parameter
func (s *Store) Create(ctx context.Context, sub *O2Subscription) error

// ✅ GOOD: Respect context cancellation
select {
case <-ctx.Done():
    return ctx.Err()
case result := <-ch:
    return result
}

// ❌ BAD: Context as struct field
type Store struct {
    ctx context.Context  // NEVER DO THIS
}
```

#### Concurrency:

```go
// ✅ GOOD: Use sync primitives properly
type Cache struct {
    mu    sync.RWMutex
    items map[string]interface{}
}

func (c *Cache) Get(key string) interface{} {
    c.mu.RLock()
    defer c.mu.RUnlock()
    return c.items[key]
}

// ✅ GOOD: Close channels to signal completion
ch := make(chan result)
defer close(ch)

// ❌ BAD: Shared state without synchronization
var counter int  // Race condition!
go func() { counter++ }()
go func() { counter++ }()
```

### 5. Documentation Requirements

#### Code Documentation:

```go
// ✅ GOOD: Package documentation
// Package storage provides abstractions for subscription storage.
// It supports Redis-backed storage with automatic failover and caching.
package storage

// ✅ GOOD: Type documentation with examples
// O2Subscription represents an O2-IMS subscription.
// Subscribers receive webhook notifications when watched resources change.
//
// Example:
//
//	sub := &O2Subscription{
//	    ID:       "sub-123",
//	    Callback: "https://smo.example.com/notify",
//	    Filter: SubscriptionFilter{
//	        ResourcePoolID: "pool-abc",
//	    },
//	}
type O2Subscription struct { ... }

// ✅ GOOD: Function documentation with parameters and errors
// Create creates a new subscription in the store.
// Returns ErrSubscriptionExists if a subscription with the same ID already exists.
// The context is used for cancellation and timeout control.
func (s *Store) Create(ctx context.Context, sub *O2Subscription) error
```

#### External Documentation:

**Required Documentation Files:**
- `README.md` - Project overview, quickstart, architecture diagram
- `docs/architecture.md` - Detailed architecture and design decisions
- `docs/api-mapping.md` - O2-IMS ↔ Kubernetes mappings
- `docs/deployment.md` - Deployment guide (single and multi-cluster)
- `docs/security.md` - Security model and mTLS configuration
- `docs/operations.md` - Operational runbooks (backup, restore, failover)
- `CONTRIBUTING.md` - How to contribute (PR process, coding standards)

### 6. Git Workflow - MANDATORY

#### Branch Protection:

**Main Branch Rules:**
- ✅ Require pull request reviews (minimum 1 approval)
- ✅ Require status checks to pass (lint, test, security)
- ✅ Require branches to be up to date before merging
- ✅ No direct commits to main (enforce PRs only)
- ✅ Require linear history (squash or rebase merge)
- ✅ Require signed commits (GPG)

#### Commit Standards:

```bash
# ✅ GOOD: Conventional commit format
feat(adapter): add Kubernetes adapter for O2-IMS resource pools
fix(cache): resolve race condition in Redis cache invalidation
docs(api): update O2-IMS OpenAPI specification to v1.2
test(subscription): add integration tests for webhook delivery
refactor(storage): simplify Redis key schema for subscriptions
chore(deps): upgrade Redis client to v9.6.1

# ✅ GOOD: Reference GitHub issues
fix(auth): validate mTLS certificates in ingress gateway

Fixes certificate validation logic that allowed expired
certificates to pass through.

Resolves #42

# ❌ BAD: Vague commit messages
fix bug
update code
wip
```

#### Pre-Commit Hooks:

**Automated Checks (MUST pass before commit):**
1. Run `gofmt` (code formatting)
2. Run `goimports` (import organization)
3. Run `golangci-lint` (all linters)
4. Run `go test ./...` (unit tests)
5. Run `gosec` (security scan)
6. Check for secrets (using gitleaks)
7. Validate commit message format

```bash
# Pre-commit hook installation
make install-hooks

# Manual pre-commit check
make pre-commit
```

#### Pull Request Process:

**PR Requirements:**
1. Create feature branch: `feature/issue-NUM-description`
2. Write code following ALL standards above
3. Write tests (≥80% coverage)
4. Run `make quality` (MUST pass)
5. Update documentation if needed
6. Create PR with template
7. Address review comments
8. Squash commits before merge

**PR Template Must Include:**
- Summary of changes
- Link to GitHub issue (`Resolves #NUM`)
- Testing performed
- Screenshots (for UI changes)
- Breaking changes (if any)
- Checklist:
  - [ ] Tests added/updated
  - [ ] Documentation updated
  - [ ] Linting passed
  - [ ] Security scan passed
  - [ ] No secrets committed

### 7. Performance Guidelines

**Code MUST be performant and scalable.**

#### Performance Requirements:

- API response time: p95 < 100ms, p99 < 500ms
- Subscription webhook delivery: < 1s from event to webhook
- Cache hit ratio: > 90%
- Redis operations: < 5ms (local), < 50ms (cross-cluster)
- Memory usage per pod: < 512MB under normal load

#### Performance Best Practices:

```go
// ✅ GOOD: Preallocate slices when size is known
subs := make([]*O2Subscription, 0, len(subIDs))

// ✅ GOOD: Use sync.Pool for frequently allocated objects
var bufferPool = sync.Pool{
    New: func() interface{} {
        return new(bytes.Buffer)
    },
}

// ✅ GOOD: Use buffered channels appropriately
ch := make(chan result, 100)

// ✅ GOOD: Avoid unnecessary allocations in hot paths
func (s *Store) Get(key string) (*Subscription, error) {
    // Reuse buffer instead of allocating
    buf := bufferPool.Get().(*bytes.Buffer)
    defer bufferPool.Put(buf)
    buf.Reset()
    ...
}

// ❌ BAD: Unnecessary conversions
str := string([]byte(str))  // Wasteful

// ❌ BAD: Unbuffered channels in loops
for _, item := range items {
    ch <- item  // Can block and slow down loop
}
```

#### Profiling:

```bash
# CPU profiling
go test -cpuprofile=cpu.prof -bench=.
go tool pprof cpu.prof

# Memory profiling
go test -memprofile=mem.prof -bench=.
go tool pprof mem.prof

# Live profiling (pprof endpoint)
curl http://localhost:8080/debug/pprof/heap > heap.prof
go tool pprof heap.prof
```

### 8. Observability - MANDATORY

**All code MUST be observable.**

#### Structured Logging:

```go
// ✅ GOOD: Structured logging with context
log.Info("subscription created",
    "subscriptionID", sub.ID,
    "callback", sub.Callback,
    "cluster", clusterID,
)

// ✅ GOOD: Log levels appropriately
log.Debug("cache hit", "key", key)              // Verbose debugging
log.Info("subscription created", "id", id)       // Important events
log.Warn("redis unavailable, using cache", ...)  // Degraded mode
log.Error("failed to send webhook", "error", err) // Errors

// ❌ BAD: Unstructured logging
log.Println("Subscription created:", sub.ID)

// ❌ BAD: Logging sensitive data
log.Info("auth token", "token", token)  // NEVER LOG SECRETS
```

#### Metrics:

```go
// ✅ GOOD: Instrument all critical paths
var (
    subscriptionCreates = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "o2ims_subscription_creates_total",
            Help: "Total number of subscription create requests",
        },
        []string{"status"},
    )

    webhookDeliveryDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "o2ims_webhook_delivery_duration_seconds",
            Help:    "Webhook delivery latency",
            Buckets: prometheus.ExponentialBuckets(0.001, 2, 10),
        },
        []string{"status"},
    )
)

// Instrument function
func (s *Store) Create(ctx context.Context, sub *O2Subscription) error {
    timer := prometheus.NewTimer(webhookDeliveryDuration.WithLabelValues("success"))
    defer timer.ObserveDuration()

    err := s.create(ctx, sub)
    if err != nil {
        subscriptionCreates.WithLabelValues("error").Inc()
        return err
    }

    subscriptionCreates.WithLabelValues("success").Inc()
    return nil
}
```

#### Tracing:

```go
// ✅ GOOD: Add tracing to all major operations
func (s *Store) Create(ctx context.Context, sub *O2Subscription) error {
    ctx, span := otel.Tracer("storage").Start(ctx, "Create")
    defer span.End()

    span.SetAttributes(
        attribute.String("subscription.id", sub.ID),
        attribute.String("subscription.callback", sub.Callback),
    )

    err := s.redis.Set(ctx, ...)
    if err != nil {
        span.RecordError(err)
        span.SetStatus(codes.Error, err.Error())
        return err
    }

    return nil
}
```

## Development Workflow

### Initial Setup:

```bash
# Clone repository
git clone https://github.com/yourorg/o2-gateway.git
cd o2-gateway

# Install development tools
make install-tools

# Install pre-commit hooks
make install-hooks

# Verify setup
make verify-setup
```

### Daily Development:

```bash
# 1. Create feature branch
git checkout -b feature/issue-42-add-subscription-filter

# 2. Write code (following ALL standards above)

# 3. Run quality checks frequently
make lint      # Every few minutes
make test      # After each function/feature
make quality   # Before commit

# 4. Commit (pre-commit hooks run automatically)
git add .
git commit -m "feat(subscription): add resource type filter

Adds ability to filter subscriptions by resource type.
Includes unit and integration tests.

Resolves #42"

# 5. Push and create PR
git push origin feature/issue-42-add-subscription-filter
# Create PR via GitHub UI or gh CLI
```

### Before Creating PR:

```bash
# Run full quality check
make quality

# Update from main
git fetch origin
git rebase origin/main

# Run tests one more time
make test

# Create PR
gh pr create --title "feat(subscription): add resource type filter" \
  --body "Resolves #42"
```

## Claude Code Specific Instructions

### When Writing Code:

1. **ALWAYS run linters** after writing code: `make lint`
2. **NEVER disable linters** - fix the code instead
3. **ALWAYS write tests** alongside implementation
4. **ALWAYS check security** with `gosec` before committing
5. **ALWAYS use structured logging** with appropriate levels
6. **ALWAYS add metrics** for critical operations
7. **ALWAYS document** exported functions and types
8. **ALWAYS handle errors** properly (wrap with context)

### When Reviewing Code:

1. Check linting passes: `make lint`
2. Check tests pass: `make test`
3. Check coverage: `make test-coverage`
4. Check security: `make security-scan`
5. Verify documentation exists
6. Verify observability (logs, metrics, traces)

### Common Violations to Avoid:

❌ Using `//nolint` to suppress linter warnings
❌ Ignoring errors with `_` operator
❌ Hardcoding secrets or credentials
❌ Using `interface{}` or `any` without justification
❌ Missing tests for new code
❌ Exposing internal errors to external clients
❌ Logging sensitive information
❌ Disabling security checks
❌ Committing without running quality checks
❌ Creating PRs without linking to GitHub issues

### Quality Gates (MUST PASS):

Every commit and PR must pass:
1. ✅ `make fmt` - Code formatting
2. ✅ `make lint` - All linters (zero warnings)
3. ✅ `make test` - All tests pass
4. ✅ `make test-coverage` - Coverage ≥80%
5. ✅ `make security-scan` - No security issues
6. ✅ Pre-commit hooks - All automated checks
7. ✅ PR review - Minimum 1 approval
8. ✅ CI pipeline - All checks green

## Makefile Targets

All common operations have Makefile targets:

```bash
make help              # Show all available targets
make install-tools     # Install development tools
make install-hooks     # Install git pre-commit hooks
make fmt               # Format code
make lint              # Run all linters
make test              # Run unit tests
make test-coverage     # Run tests with coverage report
make test-integration  # Run integration tests
make test-e2e          # Run E2E tests
make security-scan     # Run security scanners
make quality           # Run all quality checks (lint+test+security)
make build             # Build binary
make docker-build      # Build Docker image
make deploy-dev        # Deploy to dev environment
make clean             # Clean build artifacts
```

## Resources

- [Effective Go](https://go.dev/doc/effective_go)
- [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md)
- [O-RAN O2 IMS Specification](https://specifications.o-ran.org/)
- [Kubernetes API Conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md)

## Questions?

If anything is unclear or you encounter edge cases not covered here, ask the team before proceeding. When in doubt, prioritize:

1. **Security** - Never compromise security
2. **Quality** - Code must be maintainable
3. **Performance** - Must scale to production loads
4. **Observability** - Must be debuggable in production

**Remember: We build production systems for critical telecom infrastructure. Quality is not optional.**
