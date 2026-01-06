# Suggested Commands

## Setup and Installation

```bash
# Install development tools
make install-tools

# Install git pre-commit hooks
make install-hooks

# Verify environment setup
make verify-setup
```

## Code Quality (MUST pass before commit)

```bash
# Format code (gofmt, goimports)
make fmt

# Check code formatting
make fmt-check

# Run all linters (50+ linters, ZERO warnings allowed)
make lint

# Auto-fix linting issues where possible
make lint-fix
```

## Testing

```bash
# Run unit tests only (fast, for development)
make test

# Run tests with coverage report (MUST be ≥80%)
make test-coverage

# Run integration tests (requires Docker)
make test-integration

# Run E2E tests (requires Kubernetes)
make test-e2e

# Run ALL tests (unit + integration + E2E)
make test-all

# Run tests in watch mode
make test-watch

# Run benchmarks
make benchmark
```

## Security

```bash
# Run security scans (gosec + govulncheck)
make security-scan

# Check for committed secrets
make check-secrets
```

## Quality Gates (REQUIRED before PR)

```bash
# Run all pre-commit checks (fmt, lint, security, test)
make pre-commit

# Run complete quality checks (fmt, lint, security, coverage ≥80%)
make quality

# Run full CI pipeline locally
make ci
```

## Build and Run

```bash
# Download dependencies
make deps

# Tidy dependencies
make deps-tidy

# Build binary
make build

# Build for all platforms (Linux, Darwin, amd64, arm64)
make build-all

# Build and run gateway
make run
```

## Docker

```bash
# Build Docker image
make docker-build

# Scan Docker image with Trivy
make docker-scan

# Push Docker image
make docker-push
```

## Documentation

```bash
# Lint Markdown documentation
make lint-docs

# Check documentation links
make check-links

# Verify code examples in docs
make verify-examples
```

## Deployment

```bash
# Deploy to development
make deploy-dev

# Deploy to staging
make deploy-staging

# Deploy to production (requires confirmation)
make deploy-prod
```

## Kubernetes

```bash
# Apply Kubernetes manifests
make k8s-apply

# Delete Kubernetes resources
make k8s-delete

# Tail gateway logs
make k8s-logs

# Describe gateway pods
make k8s-describe
```

## Utilities

```bash
# Generate code (mocks, etc.)
make generate

# Display module dependency graph
make mod-graph

# Show version information
make version

# Show build information
make info

# CPU profiling
make profile-cpu

# Memory profiling
make profile-mem
```

## Development Workflow

```bash
# Daily development cycle:
make lint      # Every few minutes while coding
make test      # After each function/feature
make quality   # Before every commit (MANDATORY)
```

## CRITICAL: Before Every Commit

```bash
# This MUST pass:
make quality

# Ensures:
# ✅ Code formatted
# ✅ Linters pass (zero warnings)
# ✅ Tests pass
# ✅ Coverage ≥80%
# ✅ Security scans pass
```
