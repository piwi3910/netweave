# Performance Testing Guide

Comprehensive guide for running performance tests on the netweave O2-IMS Gateway.

## Overview

The netweave gateway performance testing infrastructure includes:

- **k6 Load Tests**: HTTP-level load testing scenarios
- **Go Benchmarks**: Micro-benchmarks for critical code paths
- **SLA Validation**: Automated threshold checking against O-RAN requirements

## Performance SLAs

Per O-RAN O2-IMS specifications:

| Metric | Target |
|--------|--------|
| **API Latency (p95)** | < 100ms |
| **API Latency (p99)** | < 500ms |
| **Auth Latency (p95)** | < 100ms |
| **Token Validation (p95)** | < 50ms |
| **Error Rate** | < 1% |
| **Webhook Delivery** | < 1s end-to-end |

## Prerequisites

### Required Tools

```bash
# Install k6
brew install k6  # macOS
# or
curl https://github.com/grafana/k6/releases/download/v0.48.0/k6-v0.48.0-linux-amd64.tar.gz -L | tar xvz
sudo mv k6-v0.48.0-linux-amd64/k6 /usr/local/bin

# Install Go (for benchmarks)
go version  # Requires Go 1.25.7+

# Verify installation
k6 version
go test -bench=. -benchmem ./tests/performance/benchmarks
```

### Test Environment Setup

```bash
# Start local test environment (Kind cluster + gateway)
make test-e2e-setup

# Or use existing deployment
export BASE_URL=https://o2.netweave.local:8443
```

### Authentication Setup

#### mTLS Authentication

```bash
# Generate test certificates (if not exists)
./scripts/generate-test-certs.sh

# Set certificate paths
export MTLS_CERT=./certs/client.crt
export MTLS_KEY=./certs/client.key
export CA_CERT=./certs/ca.crt
```

#### OAuth2 Authentication

```bash
# Configure OAuth2 credentials
export OAUTH2_TOKEN_URL=https://auth.netweave.local/realms/netweave/protocol/openid-connect/token
export OAUTH2_CLIENT_ID=netweave-gateway
export OAUTH2_CLIENT_SECRET=your-secret-here
export OAUTH2_USERNAME=testuser
export OAUTH2_PASSWORD=testpass
```

## Running k6 Load Tests

### Quick Start

```bash
# Run all performance tests
make test-performance

# Run specific scenario
k6 run tests/performance/k6/scenarios/mtls-auth.js

# Run with custom VUs and duration
k6 run --vus 100 --duration 5m tests/performance/k6/scenarios/oauth2-auth.js

# Generate HTML report
k6 run --out json=results.json tests/performance/k6/scenarios/mixed-workload.js
k6 report results.json --output=report.html
```

### Individual Scenarios

#### 1. mTLS Authentication Load Test

Tests client certificate authentication performance.

```bash
k6 run tests/performance/k6/scenarios/mtls-auth.js
```

**Metrics:**
- Authentication latency (p95, p99)
- Certificate validation performance
- Error rate

**Expected Results:**
- p95 < 100ms
- p99 < 500ms
- Error rate < 1%

#### 2. OAuth2 Token Validation Load Test

Tests OAuth2 bearer token validation performance.

```bash
k6 run tests/performance/k6/scenarios/oauth2-auth.js
```

**Metrics:**
- Token validation latency (p95)
- Keycloak integration performance
- Cache hit rate

**Expected Results:**
- p95 < 50ms
- Error rate < 1%

#### 3. Mixed Workload Test

Tests 50/50 mix of mTLS and OAuth2 authentication.

```bash
k6 run tests/performance/k6/scenarios/mixed-workload.js
```

**Metrics:**
- Combined authentication performance
- Load distribution
- Resource contention

**Expected Results:**
- p95 < 100ms for both auth types
- Even distribution of load

#### 4. Spike Test

Tests sudden load increases (0 → 1000 users in 10s).

```bash
k6 run tests/performance/k6/scenarios/spike-test.js
```

**Metrics:**
- System responsiveness during spike
- Rate limiting effectiveness
- Auto-scaling behavior

**Expected Results:**
- Error rate < 5% during spike
- Graceful degradation
- Recovery within 30s

#### 5. Stress Test

Gradually increases load until failure point is found.

```bash
k6 run tests/performance/k6/scenarios/stress-test.js
```

**Metrics:**
- Maximum sustainable load
- Breaking point identification
- Resource exhaustion patterns

**Expected Results:**
- Identify capacity limits
- Document failure modes
- Plan scaling strategies

#### 6. API CRUD Operations Test

Tests full lifecycle operations under load.

```bash
k6 run tests/performance/k6/scenarios/api-crud.js
```

**Metrics:**
- CREATE, READ, UPDATE, DELETE latencies
- Database transaction performance
- Data consistency

**Expected Results:**
- p95 < 100ms for all operations
- p99 < 500ms
- Zero data corruption

### Custom Scenarios

Create custom scenarios by copying and modifying existing tests:

```javascript
import { BASE_URL, API_VERSION, THRESHOLDS } from '../config.js';

export const options = {
  scenarios: {
    custom: {
      executor: 'constant-vus',
      vus: 50,
      duration: '5m',
    },
  },
  thresholds: THRESHOLDS.api,
};

export default function () {
  // Your test logic here
}
```

## Running Go Benchmarks

### Quick Start

```bash
# Run all benchmarks
make test-benchmark

# Run specific benchmark
go test -bench=BenchmarkMTLSAuthentication -benchmem ./tests/performance/benchmarks

# Run with increased iterations
go test -bench=. -benchtime=10s -benchmem ./tests/performance/benchmarks

# Generate CPU profile
go test -bench=. -benchmem -cpuprofile=cpu.prof ./tests/performance/benchmarks
go tool pprof cpu.prof
```

### Available Benchmarks

#### Authentication Benchmarks

```bash
# mTLS authentication
go test -bench=BenchmarkMTLSAuthentication -benchmem ./tests/performance/benchmarks

# OAuth2 token validation
go test -bench=BenchmarkOAuth2TokenValidation -benchmem ./tests/performance/benchmarks

# RBAC authorization
go test -bench=BenchmarkAuthorizationCheck -benchmem ./tests/performance/benchmarks

# Concurrent authentication
go test -bench=BenchmarkConcurrentAuthentication -benchmem ./tests/performance/benchmarks
```

#### Handler Benchmarks

```bash
# List resource pools
go test -bench=BenchmarkListResourcePools -benchmem ./tests/performance/benchmarks

# Get single resource pool
go test -bench=BenchmarkGetResourcePool -benchmem ./tests/performance/benchmarks

# List resources with filters
go test -bench=BenchmarkListResourcesWithFilter -benchmem ./tests/performance/benchmarks

# Concurrent requests
go test -bench=BenchmarkConcurrentRequests -benchmem ./tests/performance/benchmarks
```

#### Cache Benchmarks

```bash
# Redis SET operations
go test -bench=BenchmarkRedisSet -benchmem ./tests/performance/benchmarks

# Redis GET operations
go test -bench=BenchmarkRedisGet -benchmem ./tests/performance/benchmarks

# Concurrent Redis operations
go test -bench=BenchmarkConcurrentRedisOps -benchmem ./tests/performance/benchmarks
```

### Interpreting Benchmark Results

```
BenchmarkMTLSAuthentication-8    5000    250000 ns/op    1024 B/op    15 allocs/op
```

- **5000**: Number of iterations
- **250000 ns/op**: 250 microseconds per operation
- **1024 B/op**: 1KB allocated per operation
- **15 allocs/op**: 15 memory allocations per operation

**Performance Targets:**
- Authentication: < 1ms per operation
- Handler operations: < 10ms per operation
- Redis operations: < 100μs per operation
- Memory allocations: Minimize allocations in hot paths

## Performance Profiling

### CPU Profiling

```bash
# Generate CPU profile from benchmarks
go test -bench=. -cpuprofile=cpu.prof ./tests/performance/benchmarks

# Analyze CPU profile
go tool pprof cpu.prof
(pprof) top10
(pprof) list <function_name>
(pprof) web  # Generate visual graph (requires graphviz)
```

### Memory Profiling

```bash
# Generate memory profile
go test -bench=. -memprofile=mem.prof ./tests/performance/benchmarks

# Analyze memory profile
go tool pprof mem.prof
(pprof) top10
(pprof) list <function_name>
```

### Live Profiling

```bash
# Start gateway with pprof endpoint
ENABLE_PPROF=true ./build/netweave

# CPU profile (30 seconds)
curl http://localhost:6060/debug/pprof/profile?seconds=30 > cpu.prof

# Heap profile
curl http://localhost:6060/debug/pprof/heap > heap.prof

# Goroutine profile
curl http://localhost:6060/debug/pprof/goroutine > goroutine.prof

# Analyze
go tool pprof cpu.prof
```

## Continuous Performance Testing

### CI/CD Integration

```yaml
# .github/workflows/performance.yml
name: Performance Tests

on:
  pull_request:
    branches: [main]
  schedule:
    - cron: '0 2 * * *'  # Daily at 2 AM

jobs:
  performance:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.25.7'

      - name: Setup test environment
        run: make test-e2e-setup

      - name: Run k6 load tests
        run: make test-performance

      - name: Run Go benchmarks
        run: make test-benchmark

      - name: Upload results
        uses: actions/upload-artifact@v4
        with:
          name: performance-results
          path: |
            results.json
            benchmark-results.txt
```

### Performance Regression Detection

```bash
# Baseline benchmarks (before changes)
go test -bench=. -benchmem ./tests/performance/benchmarks > baseline.txt

# Make code changes...

# New benchmarks (after changes)
go test -bench=. -benchmem ./tests/performance/benchmarks > new.txt

# Compare results
benchstat baseline.txt new.txt
```

## Troubleshooting

### Common Issues

#### k6 Connection Errors

```bash
# Check gateway is running
curl -k https://o2.netweave.local:8443/health

# Verify certificates
openssl s_client -connect o2.netweave.local:8443 -cert $MTLS_CERT -key $MTLS_KEY

# Check DNS resolution
ping o2.netweave.local
```

#### High Error Rates

1. Check gateway logs: `kubectl logs -f -l app=netweave-gateway`
2. Verify resource limits: `kubectl describe pod <pod-name>`
3. Check Redis connectivity: `redis-cli -h <redis-host> ping`
4. Review rate limiting: Check for 429 responses

#### Poor Performance

1. **CPU bound**: Increase pod replicas
2. **Memory bound**: Increase memory limits
3. **Database bound**: Optimize queries, add indexes
4. **Network bound**: Check network latency, use closer regions

### Performance Debugging

```bash
# Enable debug logging
export LOG_LEVEL=debug

# Trace slow requests
export ENABLE_TRACING=true

# Monitor resource usage
kubectl top pods -l app=netweave-gateway

# Check Redis performance
redis-cli --latency-history -h <redis-host>
```

## Best Practices

1. **Isolate Tests**: Run performance tests in dedicated environments
2. **Warm-up Period**: Include warm-up iterations before measurements
3. **Realistic Data**: Use production-like data sizes and patterns
4. **Monitor Resources**: Track CPU, memory, network during tests
5. **Baseline Comparison**: Always compare against known baselines
6. **Document Results**: Keep history of performance test results
7. **Automate**: Integrate into CI/CD for continuous monitoring

## References

- [k6 Documentation](https://k6.io/docs/)
- [Go Benchmarking](https://golang.org/pkg/testing/#hdr-Benchmarks)
- [O-RAN O2-IMS Specification](https://specifications.o-ran.org/)
- [Performance Tuning Guide](./performance-tuning-guide.md)
