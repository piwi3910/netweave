# Linting Rules and Standards

## Zero-Tolerance Linting Policy

**ALL code MUST pass linting without ANY errors or warnings.**

## Enabled Linters (50+ linters)

### Core Linters
- errcheck: Check for unchecked errors
- govet: Go vet (includes shadow checking)
- ineffassign: Detect ineffectual assignments
- staticcheck: Static analysis (includes gosimple)
- unused: Check for unused constants, variables, functions

### Security Linters
- gosec: Security issues (G101-G601)
- forbidigo: Forbid dangerous functions (panic, os.Exit, fmt.Print*, http.DefaultClient)

### Code Quality Linters
- gocyclo: Cyclomatic complexity (max 15)
- gocognit: Cognitive complexity (max 20)
- dupl: Duplicate code detection (threshold 100)
- goconst: Find repeated strings (min 3 occurrences)
- gocritic: Comprehensive Go code checks
- maintidx: Maintainability index

### Style Linters
- revive: Replacement for golint (40+ rules)
- godot: Comments must end with period
- misspell: Fix commonly misspelled words
- whitespace: Check unnecessary whitespace
- unparam: Find unused function parameters
- unconvert: Remove unnecessary type conversions

### Error Handling Linters
- errorlint: Error wrapping best practices
- wrapcheck: Check error wrapping
- errname: Check error naming conventions
- nilerr: Find code returning nil even if error is not nil

### Best Practices Linters
- contextcheck: Check context is passed correctly
- bodyclose: Check HTTP response body is closed
- noctx: Find HTTP requests without context
- ireturn: Accept interfaces, return concrete types
- interfacebloat: Check for bloated interfaces

## Complexity Limits

- Cyclomatic complexity: ≤15
- Cognitive complexity: ≤20
- Function length: Checked by complexity linters
- Nested if statements: ≤4 levels
- Line length: 120 characters

## Forbidden Patterns

- ❌ print*, panic, fmt.Print* (use structured logging)
- ❌ os.Exit (use proper error handling)
- ❌ http.DefaultClient (use configured HTTP client)
- ❌ fmt.Errorf (use errors.New or custom errors with wrapping)
- ❌ //nolint comments without explanation (require-explanation: true)

## Security Checks (gosec)

- G101: Hardcoded credentials
- G102: Bind to all interfaces
- G103: Audit use of unsafe
- G104: Audit errors not checked
- G107: URL provided to HTTP request as taint input
- G201-G202: SQL injection
- G204: Command injection
- G301-G306: Poor file permissions
- G401-G404: Weak crypto
- G402: TLS InsecureSkipVerify
- G501-G505: Blocklisted imports (MD5, DES, RC4, SHA1)
- G601: Implicit memory aliasing

## Import Organization

Enforced by gci and importas linters:

```go
import (
    // Standard library
    "context"
    "fmt"
    
    // External dependencies
    "github.com/gin-gonic/gin"
    "go.uber.org/zap"
    
    // Internal imports
    "github.com/yourorg/netweave/internal/adapter"
)
```

## Required Import Aliases

- k8s.io/api/core/v1 → corev1
- k8s.io/apimachinery/pkg/apis/meta/v1 → metav1
- k8s.io/client-go/kubernetes → kubernetes
- github.com/redis/go-redis/v9 → redis

## Running Linters

```bash
# Run all linters
make lint

# Auto-fix where possible
make lint-fix

# Check formatting only
make fmt-check
```

## CRITICAL RULES

1. **NEVER use //nolint** - Fix code, not rules
2. **NEVER disable linters** - All linters are required
3. **FIX violations immediately** - Don't let them accumulate
4. **Zero warnings policy** - All issues must be resolved
5. **Pre-commit hooks required** - Automated enforcement

## Configuration

Linting configuration is in `.golangci.yml` with:
- timeout: 5m
- All enabled linters must pass
- No issues skipped
- Tests files have relaxed rules for some linters
