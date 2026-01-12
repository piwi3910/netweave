# Development Guide

Welcome to the netweave development documentation. This guide provides everything you need to contribute to and develop netweave, a production-grade O-RAN O2 Gateway.

## Quick Start for Developers

**New to the project?** Follow these steps:

1. **[Development Setup →](setup.md)** - Configure your local environment (15 minutes)
2. **[Testing Guidelines →](testing.md)** - Understand our testing philosophy and requirements
3. **[Contributing Guide →](contributing.md)** - Learn the contribution workflow
4. **[Architecture Decisions →](decisions.md)** - Understand past design choices

## Overview

netweave is built with production-grade standards:

- **Language:** Go 1.25.0+
- **Framework:** Gin for HTTP
- **Testing:** ≥80% coverage required
- **Quality:** Zero-tolerance linting (no warnings allowed)
- **Security:** Continuous vulnerability scanning
- **Verification:** ALWAYS verify fixes work (CI must pass)

## Development Environment

### Prerequisites

| Tool | Version | Purpose |
|------|---------|---------|
| **Go** | 1.25.0+ | Core language (required by k8s.io/client-go v0.35.0) |
| **Docker** | Latest | Container builds and testing |
| **Kubernetes** | 1.30+ | Primary infrastructure platform |
| **kubectl** | 1.30+ | Kubernetes CLI |
| **Helm** | 3.x+ | Package management |
| **Redis** | 7.4+ | State storage (via Docker or local) |
| **make** | 3.81+ | Build automation |
| **Git** | 2.40+ | Version control with GPG signing |
| **golangci-lint** | 1.56+ | Linting (installed via `make install-tools`) |

### Quick Setup

```bash
# 1. Clone repository
git clone https://github.com/piwi3910/netweave.git
cd netweave

# 2. Install development tools (golangci-lint, gosec, etc.)
make install-tools

# 3. Install git hooks (pre-commit validation)
make install-hooks

# 4. Verify setup
make verify-setup

# 5. Run tests
make test

# 6. Build binary
make build

# 7. Run locally
make run-dev
```

**See [Development Setup](setup.md) for detailed instructions.**

## Development Workflow

### 1. Create GitHub Issue

**MANDATORY: Create an issue BEFORE any code change.**

```bash
gh issue create \
  --title "Add resource type filter to subscriptions" \
  --body "Feature request: Allow filtering subscriptions by resource type..." \
  --label "enhancement"
```

### 2. Create Feature Branch

```bash
# Branch naming: issue-NUM-brief-description
git checkout -b issue-42-add-resource-filter
```

### 3. Develop with Quality Checks

```bash
# Write code + tests

# Format code (run frequently)
make fmt

# Run linters (every few minutes)
make lint

# Run tests (after each feature)
make test

# Run all quality checks (before commit)
make quality
```

### 4. Commit with Standards

```bash
git commit -m "[Feature] Add resource type filter to subscriptions

Enables filtering subscriptions by resourceTypeId for more
targeted event notifications.

- Add filter field to subscription model
- Implement filter logic in event dispatcher
- Add unit and integration tests
- Update API documentation

Resolves #42"
```

**Important:**
- ALWAYS commit as `Pascal Watteel <pascal@watteel.com>`
- NEVER include `Co-Authored-By` lines for Claude or AI assistants
- NEVER mention AI assistance in commit messages

### 5. Verify CI Passes

```bash
# Push changes
git push origin issue-42-add-resource-filter

# Check CI status (MANDATORY - wait for completion)
gh run list --limit 1

# If CI fails, check logs
gh run view <id> --log | grep "FAIL\|ERROR"

# Fix issues and repeat until CI passes
```

### 6. Create Pull Request

```bash
gh pr create \
  --title "[Feature] Add resource type filter to subscriptions" \
  --body "Resolves #42

## Summary
Adds ability to filter subscriptions by resource type.

## Changes
- New filter field in subscription model
- Filter logic in event dispatcher
- Comprehensive tests (unit + integration)

## Testing
- Unit tests: 95% coverage
- Integration tests: All pass
- Manual testing: Verified with K8s cluster

## Documentation
- Updated API docs
- Updated subscription examples"
```

## Code Quality Standards

### Zero-Tolerance Policy

**ALL code MUST pass these checks:**

```bash
make quality
```

This runs:
1. ✅ `make fmt` - Code formatting (no changes)
2. ✅ `make lint` - All linters pass (zero warnings)
3. ✅ `make test` - All tests pass (≥80% coverage)
4. ✅ `make test-integration` - Integration tests pass
5. ✅ `make security-scan` - No vulnerabilities
6. ✅ `make lint-docs` - Documentation formatting

### Linting Rules

**50+ linters enforced:**
- gosec, revive, staticcheck, govet
- errcheck, ineffassign, unused
- gocyclo, gofmt, goimports
- misspell, godot, gocritic
- prealloc, unconvert, unparam

**❌ NEVER ALLOWED:**
- Using `//nolint` directives
- Using `//golangci-lint:disable`
- Ignoring linting errors
- Committing code with warnings

**✅ ALWAYS REQUIRED:**
- Fix the code, not the rule
- Run `make lint` before every commit
- Achieve 100% linting compliance

### Testing Requirements

**Coverage:** ≥80% for all packages

**Test Types:**
- **Unit Tests:** All business logic, error paths, edge cases
- **Integration Tests:** API endpoints with real dependencies
- **E2E Tests:** Critical user workflows

**Libraries:**
```go
import (
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "go.uber.org/mock/gomock"
    "github.com/alicebob/miniredis/v2"
)
```

**See [Testing Guidelines](testing.md) for detailed requirements.**

## Common Development Tasks

### Running the Gateway Locally

```bash
# Development mode (debug logging, no TLS)
make run-dev

# Staging mode (TLS enabled, production-like)
make run-staging

# With custom config
./bin/gateway --config=config/custom.yaml

# With environment variables
export NETWEAVE_SERVER_PORT=9443
export NETWEAVE_LOG_LEVEL=debug
./bin/gateway
```

### Working with Tests

```bash
# Run all unit tests
make test

# Run specific package tests
go test ./internal/adapters/kubernetes/... -v

# Run tests with race detection
go test -race ./...

# Run integration tests (requires Redis + K8s)
make test-integration

# Run E2E tests
make test-e2e

# Generate coverage report
make test-coverage
open coverage.html

# Watch mode for TDD
make test-watch
```

### Working with Linters

```bash
# Run all linters
make lint

# Run specific linter
golangci-lint run --disable-all --enable=gosec

# Auto-fix formatting issues
make fmt

# Check for security issues
make security-scan
```

### Building and Packaging

```bash
# Build binary
make build

# Build for specific platform
GOOS=linux GOARCH=amd64 make build

# Build Docker image
make docker-build

# Build with specific tag
make docker-build TAG=v1.2.3

# Build multi-arch images
make docker-buildx
```

### Debugging

```bash
# Run with debug logging
NETWEAVE_LOG_LEVEL=debug ./bin/gateway

# Enable Delve debugger
dlv debug ./cmd/gateway -- --config=config/config.dev.yaml

# Attach to running process
dlv attach $(pgrep gateway)

# Run with profiling
./bin/gateway --cpuprofile=cpu.prof --memprofile=mem.prof
go tool pprof cpu.prof
```

## Project Structure

```
netweave/
├── api/                    # API specifications
│   └── openapi/           # OpenAPI/Swagger specs
├── cmd/                   # Binary entrypoints
│   └── gateway/          # Main gateway binary
├── internal/              # Private application code
│   ├── adapter/          # Core adapter interface (O2-IMS)
│   ├── adapters/         # O2-IMS backend adapters
│   │   ├── kubernetes/   # Kubernetes adapter (primary)
│   │   ├── openstack/    # OpenStack adapter
│   │   ├── aws/          # AWS adapter
│   │   └── ...           # Other IMS adapters
│   ├── dms/              # O2-DMS (Deployment Management)
│   │   ├── adapter/      # DMS adapter interface
│   │   └── adapters/     # DMS backend adapters
│   │       ├── helm/     # Helm 3 adapter
│   │       ├── argocd/   # ArgoCD adapter
│   │       └── ...       # Other DMS adapters
│   ├── smo/              # O2-SMO (Service Management)
│   │   ├── adapter/      # SMO adapter interface
│   │   └── adapters/     # SMO backend adapters
│   ├── config/           # Configuration management
│   ├── controller/       # Event controllers
│   ├── o2ims/            # O2-IMS models & handlers
│   ├── observability/    # Logging, metrics, tracing
│   └── server/           # HTTP server
├── pkg/                   # Public reusable packages
│   ├── cache/            # Cache abstraction
│   ├── storage/          # Storage abstraction
│   └── errors/           # Error types
├── config/                # Configuration files
│   ├── config.dev.yaml   # Development config
│   ├── config.staging.yaml
│   └── config.prod.yaml
├── deployments/           # Deployment manifests
│   └── kubernetes/       # K8s YAML files
├── helm/                  # Helm charts
│   └── netweave/         # Main chart
├── docs/                  # Documentation
├── tests/                 # Test suites
│   ├── integration/      # Integration tests
│   └── e2e/              # End-to-end tests
├── tools/                 # Development tools
│   └── compliance/       # O-RAN compliance checker
├── Makefile              # Build automation
├── go.mod                # Go module dependencies
├── go.sum                # Dependency checksums
├── CLAUDE.md             # Go-specific dev standards
└── README.md             # Project overview
```

## Technology Stack

### Core Dependencies

```go
// go.mod - Critical versions
go 1.25.0  // Required by k8s.io/client-go v0.35.0

require (
    // Kubernetes client libraries
    k8s.io/client-go v0.35.0
    k8s.io/api v0.35.0
    k8s.io/apimachinery v0.35.0

    // HTTP framework
    github.com/gin-gonic/gin v1.10.0

    // Storage & cache
    github.com/redis/go-redis/v9 v9.7.0

    // Observability
    go.uber.org/zap v1.27.0
    go.opentelemetry.io/otel v1.32.0
    github.com/prometheus/client_golang v1.20.5

    // Testing
    github.com/stretchr/testify v1.10.0
    go.uber.org/mock v0.5.0
)
```

### Why These Versions?

**Go 1.25.0+**
- Required by k8s.io/client-go v0.35.0
- Provides latest security and performance improvements

**k8s.io v0.35.0**
- Resolves yaml.v3 module path conflicts
- Uses structured-merge-diff/v6 (migrated from v4)
- Fixes compatibility issues with Go 1.24+

**Known Issues with Earlier Versions:**
- k8s.io v0.31-v0.34 + Go 1.24: yaml.v3 type mismatch errors
- k8s.io v0.34 + Go 1.23: golang.org/x/net@v0.47.0 requires Go 1.24+

## Key Development Principles

### 1. ALWAYS Verify Fixes Work

**NEVER assume a fix will work. ALWAYS verify before considering complete.**

```bash
# MANDATORY verification checklist:
1. Run affected tests: go test -race ./path/to/package
2. Run full test suite: make test
3. Run linters: make lint
4. Commit and push changes
5. **Wait for CI to complete** (do not move on)
6. Check CI status: gh run list --limit 1
7. If CI fails: gh run view <id> --log | grep "FAIL\|ERROR"
8. Fix ALL failures, repeat from step 1
9. Only mark complete when `gh run list` shows "success"
```

### 2. Complete All Tasks

**No job is too big or too small. When given a task, COMPLETE IT FULLY.**

**NEVER:**
- Complain about complexity or time
- Provide time estimates
- Suggest breaking work into pieces (unless asked)
- Stop before reaching stated goal
- Make excuses about scope or difficulty

**ALWAYS:**
- Execute the full task as requested
- Work systematically through requirements
- Continue until goal is achieved
- Provide progress updates, not excuses
- Ask for clarification if goal is unclear (not if difficult)

### 3. Fix Code, Not Rules

When encountering linting errors:

```go
// ❌ BAD: Disable linter
func complexFunction() error {
    //nolint:cyclop
    // ... 50 lines of nested logic
}

// ✅ GOOD: Refactor code
func complexFunction() error {
    if err := validateInputs(); err != nil {
        return err
    }
    if err := processData(); err != nil {
        return err
    }
    return finalizeResults()
}
```

### 4. Test Everything

Every line of code must have tests:

```go
// ✅ GOOD: Comprehensive tests
func TestResourcePool_Create(t *testing.T) {
    tests := []struct {
        name    string
        pool    *ResourcePool
        wantErr error
    }{
        {"valid pool", validPool(), nil},
        {"nil pool", nil, ErrInvalidInput},
        {"empty name", poolWithEmptyName(), ErrInvalidName},
        {"duplicate pool", existingPool(), ErrAlreadyExists},
    }
    // ... test implementation
}
```

### 5. Document as You Go

```go
// ✅ GOOD: Clear documentation
// SubscriptionStore manages O2-IMS subscriptions with Redis persistence.
// It provides thread-safe CRUD operations and automatic webhook delivery
// for infrastructure change events.
//
// All methods are safe for concurrent use. Context cancellation is
// respected for long-running operations.
type SubscriptionStore interface {
    // Create creates a new subscription.
    // Returns ErrSubscriptionExists if ID already exists.
    Create(ctx context.Context, sub *Subscription) error
}
```

## Code Style Guidelines

### Naming Conventions

```go
// Packages: short, lowercase, no underscores
package storage   // ✅ Good
package storageHelper  // ❌ Bad

// Files: lowercase with underscores
subscription_store.go  // ✅ Good
subscriptionStore.go   // ❌ Bad

// Types: PascalCase
type ResourcePool struct{}  // ✅ Good

// Functions: PascalCase (exported), camelCase (unexported)
func CreatePool() {}     // ✅ Good (exported)
func validatePool() {}   // ✅ Good (unexported)

// Constants: PascalCase (not SCREAMING_CASE)
const MaxRetries = 3     // ✅ Good
const MAX_RETRIES = 3    // ❌ Bad

// Interfaces: -er suffix for single-method
type Storer interface { Store() error }  // ✅ Good
```

### Avoid Variable Shadowing

```go
// ❌ BAD: Shadows imported package
import "github.com/example/adapter"

func TestSomething(t *testing.T) {
    adapter := NewAdapter()  // SHADOWS package!
}

// ✅ GOOD: Use different name or import alias
import dmsadapter "github.com/example/adapter"

func TestSomething(t *testing.T) {
    adp := NewAdapter()
    assert.Contains(t, adp.Capabilities(), dmsadapter.CapabilityX)
}
```

### Error Handling

```go
// ✅ GOOD: Wrap with context
if err := store.Create(ctx, sub); err != nil {
    return fmt.Errorf("failed to create subscription %s: %w", sub.ID, err)
}

// ✅ GOOD: Sentinel errors
var (
    ErrSubscriptionNotFound = errors.New("subscription not found")
    ErrSubscriptionExists   = errors.New("subscription already exists")
)

// ✅ GOOD: Check error types
if errors.Is(err, ErrSubscriptionNotFound) {
    return http.StatusNotFound, nil
}
```

## Makefile Targets

### Quality Checks

```bash
make fmt              # Format code (gofmt, goimports)
make lint             # Run all linters (golangci-lint)
make security-scan    # Security scanning (gosec, govulncheck)
make quality          # Run all quality checks (REQUIRED before PR)
```

### Testing

```bash
make test             # Unit tests
make test-race        # Tests with race detector
make test-coverage    # Coverage report
make test-integration # Integration tests (requires K8s)
make test-e2e         # End-to-end tests
make test-all         # All test suites
make test-watch       # Watch mode for TDD
```

### Building

```bash
make build            # Build binary
make build-all        # Build for all platforms
make docker-build     # Build Docker image
make docker-buildx    # Build multi-arch images
```

### Development

```bash
make run-dev          # Run in development mode
make run-staging      # Run in staging mode
make run-prod         # Run in production mode
make install-tools    # Install development tools
make install-hooks    # Install git hooks
make verify-setup     # Verify development environment
```

### Deployment

```bash
make deploy-dev       # Deploy to dev environment
make deploy-staging   # Deploy to staging
make deploy-prod      # Deploy to production
make helm-package     # Package Helm chart
```

### Dependencies

```bash
make deps-download    # Download dependencies
make deps-upgrade     # Upgrade dependencies
make deps-tidy        # Clean up dependencies
make deps-verify      # Verify dependencies
```

## Getting Help

### Documentation

- **This Guide:** Development overview
- **[Setup Guide](setup.md):** Detailed environment setup
- **[Testing Guide](testing.md):** Testing requirements and patterns
- **[Contributing Guide](contributing.md):** PR process and standards
- **[CLAUDE.md](../../CLAUDE.md):** Go-specific standards and patterns

### Support Channels

- **Questions:** [GitHub Discussions](https://github.com/piwi3910/netweave/discussions)
- **Bugs:** [GitHub Issues](https://github.com/piwi3910/netweave/issues)
- **Security:** security@example.com (private disclosure)

### Resources

- **[Effective Go](https://go.dev/doc/effective_go)**
- **[Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md)**
- **[O-RAN Alliance Specifications](https://specifications.o-ran.org/)**

## Next Steps

1. **[Set up your development environment →](setup.md)**
2. **[Learn our testing philosophy →](testing.md)**
3. **[Read the contribution guide →](contributing.md)**
4. **[Browse architecture decisions →](decisions.md)**

---

**Remember: We build production systems for critical telecom infrastructure. Quality is not optional.**
