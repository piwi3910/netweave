# Testing Guidelines

This document outlines testing standards, patterns, and requirements for netweave development.

## Table of Contents

- [Testing Philosophy](#testing-philosophy)
- [Coverage Requirements](#coverage-requirements)
- [Test Types](#test-types)
- [Testing Patterns](#testing-patterns)
- [Running Tests](#running-tests)
- [Writing Tests](#writing-tests)
- [Best Practices](#best-practices)

## Testing Philosophy

**Core Principles:**

1. **Tests are first-class code** - Maintain same quality standards as production code
2. **Test behavior, not implementation** - Focus on what code does, not how
3. **Fast feedback loops** - Unit tests must be fast (<100ms each)
4. **Comprehensive coverage** - ≥80% coverage is mandatory, not optional
5. **No flaky tests** - Tests must be deterministic and reliable

**Test Pyramid:**

```
        /\
       /  \
      / E2E\     ~5%  - Critical user workflows
     /______\
    /        \
   /Integration\ ~15% - API endpoints, DB, Redis
  /____________\
 /              \
/   Unit Tests   \ ~80% - Business logic, error paths
/__________________\
```

## Coverage Requirements

### Mandatory Requirements

| Package Type | Min Coverage | Target Coverage |
|--------------|--------------|-----------------|
| **Business Logic** | 80% | 90%+ |
| **API Handlers** | 80% | 85%+ |
| **Adapters** | 75% | 80%+ |
| **Storage/Cache** | 85% | 90%+ |
| **Utilities** | 90% | 95%+ |

### What Must Be Tested

✅ **ALWAYS test:**
- All public functions and methods
- All error paths and edge cases
- All input validation logic
- All state transitions
- All configuration loading
- All API endpoints

❌ **DO NOT test:**
- Third-party library internals
- Auto-generated code (mocks, proto)
- Main functions (test via E2E)

### Coverage Enforcement

```bash
# Check coverage (fails if < 80%)
make test-coverage

# View HTML coverage report
make test-coverage
open coverage.html

# Check specific package
go test -cover ./internal/adapters/kubernetes

# Detailed coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## Test Types

### 1. Unit Tests

**Purpose:** Test individual functions/methods in isolation

**Characteristics:**
- Fast (<100ms per test)
- No external dependencies (use mocks)
- Test single responsibility
- Comprehensive edge case coverage

**Location:** Same directory as code (`*_test.go`)

**Example:**

```go
func TestSubscriptionStore_Create(t *testing.T) {
    tests := []struct {
        name    string
        sub     *O2Subscription
        wantErr error
    }{
        {
            name: "valid subscription",
            sub: &O2Subscription{
                ID:       "sub-123",
                Callback: "https://smo.example.com/notify",
            },
            wantErr: nil,
        },
        {
            name: "duplicate subscription",
            sub: &O2Subscription{ID: "sub-exists"},
            wantErr: ErrSubscriptionExists,
        },
        {
            name: "nil subscription",
            sub: nil,
            wantErr: ErrInvalidInput,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Use miniredis for Redis operations
            mockRedis := miniredis.RunT(t)
            defer mockRedis.Close()

            store := NewStore(mockRedis.Addr())
            err := store.Create(context.Background(), tt.sub)

            if tt.wantErr != nil {
                require.ErrorIs(t, err, tt.wantErr)
            } else {
                require.NoError(t, err)
            }
        })
    }
}
```

### 2. Integration Tests

**Purpose:** Test component interactions with real dependencies

**Characteristics:**
- Moderate speed (1-5s per test)
- Real dependencies (Redis, K8s API)
- Test end-to-end workflows
- Require Docker/K8s

**Location:** `tests/integration/`

**Example:**

```go
func TestSubscriptionIntegration(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test")
    }

    // Start real Redis container
    ctx := context.Background()
    redisContainer, err := testcontainers.GenericContainer(ctx, 
        testcontainers.GenericContainerRequest{
            ContainerRequest: testcontainers.ContainerRequest{
                Image:        "redis:7.4-alpine",
                ExposedPorts: []string{"6379/tcp"},
                WaitingFor:   wait.ForLog("Ready to accept connections"),
            },
            Started: true,
        })
    require.NoError(t, err)
    defer redisContainer.Terminate(ctx)

    // Test with real dependencies
    host, _ := redisContainer.Host(ctx)
    port, _ := redisContainer.MappedPort(ctx, "6379")
    
    store := NewStore(fmt.Sprintf("%s:%s", host, port.Port()))
    
    // Test actual subscription workflow
    sub := &O2Subscription{
        ID:       uuid.New().String(),
        Callback: "https://smo.example.com/notify",
    }
    
    err = store.Create(ctx, sub)
    require.NoError(t, err)
    
    retrieved, err := store.Get(ctx, sub.ID)
    require.NoError(t, err)
    assert.Equal(t, sub.ID, retrieved.ID)
}
```

### 3. E2E Tests

**Purpose:** Test complete user workflows

**Characteristics:**
- Slow (10-30s per test)
- Full system deployment
- Real HTTP requests
- Critical paths only

**Location:** `tests/e2e/`

**Example:**

```go
func TestE2E_SubscriptionWorkflow(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping E2E test")
    }

    // Deploy full gateway stack
    gateway := deployGatewayStack(t)
    defer gateway.Cleanup()

    client := gateway.Client()

    // 1. Create subscription
    sub := &SubscriptionRequest{
        Callback: "https://webhook.site/test-123",
        Filter: map[string]string{
            "resourceTypeId": "compute-node",
        },
    }
    
    resp, err := client.Post("/o2ims/v1/subscriptions", sub)
    require.NoError(t, err)
    assert.Equal(t, http.StatusCreated, resp.StatusCode)
    
    var created Subscription
    json.NewDecoder(resp.Body).Decode(&created)
    
    // 2. List subscriptions
    resp, err = client.Get("/o2ims/v1/subscriptions")
    require.NoError(t, err)
    assert.Equal(t, http.StatusOK, resp.StatusCode)
    
    // 3. Trigger event (create node)
    node := createTestNode(t, gateway.KubeClient())
    
    // 4. Verify webhook delivery
    assert.Eventually(t, func() bool {
        return webhookReceived(created.ID)
    }, 5*time.Second, 100*time.Millisecond)
    
    // 5. Delete subscription
    resp, err = client.Delete("/o2ims/v1/subscriptions/" + created.ID)
    require.NoError(t, err)
    assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}
```

## Testing Patterns

### Table-Driven Tests

**ALWAYS use table-driven tests for comprehensive coverage:**

```go
func TestResourcePool_Validate(t *testing.T) {
    tests := []struct {
        name    string
        pool    *ResourcePool
        wantErr error
    }{
        {"valid pool", validPool(), nil},
        {"empty name", &ResourcePool{Name: ""}, ErrEmptyName},
        {"invalid location", &ResourcePool{Location: "!@#"}, ErrInvalidLocation},
        {"nil pool", nil, ErrNilInput},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := tt.pool.Validate()
            if tt.wantErr != nil {
                require.ErrorIs(t, err, tt.wantErr)
            } else {
                require.NoError(t, err)
            }
        })
    }
}
```

### Test Fixtures and Helpers

```go
// testdata/fixtures.go
package testdata

func ValidSubscription() *O2Subscription {
    return &O2Subscription{
        ID:       "sub-test-123",
        Callback: "https://smo.example.com/notify",
        Filter: map[string]string{
            "resourceTypeId": "compute-node",
        },
    }
}

func InvalidSubscription() *O2Subscription {
    return &O2Subscription{
        ID:       "", // Invalid: empty ID
        Callback: "not-a-url",
    }
}

// test_helpers.go
func setupTestRedis(t *testing.T) *miniredis.Miniredis {
    t.Helper()
    
    mockRedis := miniredis.RunT(t)
    t.Cleanup(func() {
        mockRedis.Close()
    })
    
    return mockRedis
}

func assertNoError(t *testing.T, err error, msgAndArgs ...interface{}) {
    t.Helper()
    
    if err != nil {
        t.Fatalf("Expected no error but got: %v. %v", err, msgAndArgs)
    }
}
```

### Mocking with gomock

```go
//go:generate mockgen -destination=mocks/mock_adapter.go -package=mocks . Adapter

func TestController_HandleEvent(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    // Create mock
    mockAdapter := mocks.NewMockAdapter(ctrl)
    
    // Set expectations
    mockAdapter.EXPECT().
        GetResource(gomock.Any(), "resource-123").
        Return(&Resource{ID: "resource-123"}, nil)
    
    // Test code that uses mock
    controller := NewController(mockAdapter)
    err := controller.HandleEvent(context.Background(), "resource-123")
    
    require.NoError(t, err)
}
```

### Testing Error Paths

**ALWAYS test error scenarios:**

```go
func TestStore_Create_Errors(t *testing.T) {
    tests := []struct {
        name        string
        setup       func(*miniredis.Miniredis)
        sub         *O2Subscription
        wantErr     error
        errContains string
    }{
        {
            name: "duplicate subscription",
            setup: func(mr *miniredis.Miniredis) {
                // Pre-populate existing subscription
                mr.Set("subscription:sub-123", "existing")
            },
            sub:     &O2Subscription{ID: "sub-123"},
            wantErr: ErrSubscriptionExists,
        },
        {
            name: "redis connection error",
            setup: func(mr *miniredis.Miniredis) {
                mr.Close() // Simulate connection loss
            },
            sub:         &O2Subscription{ID: "sub-456"},
            errContains: "connection refused",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            mockRedis := miniredis.RunT(t)
            defer mockRedis.Close()
            
            if tt.setup != nil {
                tt.setup(mockRedis)
            }
            
            store := NewStore(mockRedis.Addr())
            err := store.Create(context.Background(), tt.sub)
            
            require.Error(t, err)
            if tt.wantErr != nil {
                assert.ErrorIs(t, err, tt.wantErr)
            }
            if tt.errContains != "" {
                assert.Contains(t, err.Error(), tt.errContains)
            }
        })
    }
}
```

## Running Tests

### Quick Reference

```bash
# Unit tests only (fast)
make test

# Unit tests with race detection
make test-race

# Integration tests
make test-integration

# E2E tests
make test-e2e

# All tests
make test-all

# Specific package
go test ./internal/adapters/kubernetes/...

# Specific test
go test -run TestSubscriptionStore_Create ./internal/storage

# Verbose output
go test -v ./...

# Show coverage
go test -cover ./...

# Watch mode (requires entr)
find . -name '*.go' | entr -c make test
```

### Test Flags

```bash
# Skip slow tests
go test -short ./...

# Run with race detector
go test -race ./...

# Run in parallel
go test -parallel 4 ./...

# Timeout per test
go test -timeout 30s ./...

# Verbose with timestamps
go test -v -timeout 30s ./... 2>&1 | ts '[%Y-%m-%d %H:%M:%S]'
```

## Writing Tests

### Test Structure (AAA Pattern)

```go
func TestFunction(t *testing.T) {
    // ARRANGE - Set up test fixtures
    mockRedis := miniredis.RunT(t)
    defer mockRedis.Close()
    store := NewStore(mockRedis.Addr())
    sub := testdata.ValidSubscription()
    
    // ACT - Execute the code under test
    err := store.Create(context.Background(), sub)
    
    // ASSERT - Verify the results
    require.NoError(t, err)
    
    retrieved, err := store.Get(context.Background(), sub.ID)
    require.NoError(t, err)
    assert.Equal(t, sub.ID, retrieved.ID)
}
```

### Assertions

```go
// Use require for critical assertions (stops test on failure)
require.NoError(t, err)
require.NotNil(t, result)
require.Equal(t, expected, actual)

// Use assert for non-critical checks (continues test)
assert.Equal(t, "expected", result.Field)
assert.Contains(t, result.Items, item)
assert.Len(t, result.Items, 5)
```

### Test Naming

```go
// Good test names describe behavior
func TestSubscriptionStore_Create_ReturnsErrorWhenDuplicate(t *testing.T) {}
func TestResourcePool_Validate_AcceptsValidInput(t *testing.T) {}
func TestAdapter_GetResource_ReturnsNotFoundForMissingResource(t *testing.T) {}

// Bad test names
func TestCreate(t *testing.T) {}  // Too vague
func TestCase1(t *testing.T) {}   // Meaningless
func TestBug123(t *testing.T) {}  // No context
```

## Best Practices

### DO ✅

- Use table-driven tests
- Test error paths thoroughly
- Use subtests with t.Run()
- Mock external dependencies
- Clean up resources with defer or t.Cleanup()
- Use testdata directories for fixtures
- Parallelize independent tests with t.Parallel()
- Use miniredis for Redis tests
- Use testcontainers for integration tests

### DON'T ❌

- Test third-party libraries
- Write flaky tests (use timeouts properly)
- Share state between tests
- Use time.Sleep() (use Eventually() instead)
- Ignore test failures
- Skip tests without documentation
- Test implementation details
- Use production credentials in tests

### Performance Testing

```go
func BenchmarkSubscriptionStore_Create(b *testing.B) {
    mockRedis := miniredis.NewMiniRedis()
    defer mockRedis.Close()
    mockRedis.Start()
    
    store := NewStore(mockRedis.Addr())
    sub := testdata.ValidSubscription()
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        sub.ID = fmt.Sprintf("sub-%d", i)
        store.Create(context.Background(), sub)
    }
}
```

Run benchmarks:
```bash
go test -bench=. ./...
go test -bench=BenchmarkSubscriptionStore_Create -benchmem ./internal/storage
```

## Troubleshooting Tests

### Flaky Tests

**Problem:** Tests pass sometimes, fail other times

**Solutions:**
```go
// Use Eventually instead of Sleep
assert.Eventually(t, func() bool {
    return webhookReceived()
}, 5*time.Second, 100*time.Millisecond)

// Use contexts with timeouts
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
result, err := store.Get(ctx, id)

// Ensure proper cleanup
t.Cleanup(func() {
    mockRedis.Close()
    cleanupTestData()
})
```

### Slow Tests

**Problem:** Test suite takes too long

**Solutions:**
- Use t.Parallel() for independent tests
- Mock external dependencies
- Use miniredis instead of real Redis for unit tests
- Tag slow tests with `-short` flag
- Reduce test data size
- Profile tests: `go test -cpuprofile=cpu.prof`

### Test Coverage Gaps

**Problem:** Coverage below 80%

**Solutions:**
```bash
# Find uncovered code
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | grep -v "100.0%"

# View in browser
go tool cover -html=coverage.out
```

## Resources

- **Testing Package:** https://pkg.go.dev/testing
- **Testify:** https://github.com/stretchr/testify
- **Gomock:** https://github.com/uber-go/mock
- **Miniredis:** https://github.com/alicebob/miniredis
- **Testcontainers:** https://github.com/testcontainers/testcontainers-go

---

**Quality is not optional. Write tests that give you confidence to deploy.**
