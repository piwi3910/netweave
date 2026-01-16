# Performance Tuning and Optimization Guide

This guide provides comprehensive recommendations for optimizing netweave O2 Gateway performance in production environments.

## Table of Contents

- [Performance Targets](#performance-targets)
- [Configuration Tuning](#configuration-tuning)
- [Caching Strategy](#caching-strategy)
- [Resource Limits](#resource-limits)
- [Scaling Recommendations](#scaling-recommendations)
- [Benchmarking](#benchmarking)
- [Common Bottlenecks](#common-bottlenecks)
- [Production Checklist](#production-checklist)

---

## Performance Targets

### Expected Throughput

**Per Gateway Replica:**
- **Light Load**: 500-1000 req/s (typical production)
- **Medium Load**: 1000-2000 req/s (with caching)
- **Heavy Load**: 2000-5000 req/s (optimized configuration)

**Factors Affecting Throughput:**
- Backend API response time (Kubernetes, cloud providers)
- Cache hit ratio (>90% recommended)
- Request complexity (list vs get operations)
- Concurrent subscriptions and webhook delivery

### Latency Targets

**Gateway Latency (excluding backend):**
- **p50**: < 10ms (cache hit)
- **p95**: < 100ms (including backend calls)
- **p99**: < 500ms

**End-to-End Latency:**
- **p50**: 20-50ms (typical)
- **p95**: 100-200ms
- **p99**: 500ms-1s

**Factors Affecting Latency:**
- Cache hit ratio
- Backend API latency (K8s API: 10-50ms typical)
- Network latency between gateway and backend
- TLS handshake overhead (first request)
- Redis latency (< 1ms local, < 5ms network)

### Resource Usage

**Per Gateway Replica:**
- **CPU**: 0.5-2 cores (depending on load)
- **Memory**: 512MB-2GB (depending on cache size)
- **Network**: 10-100 Mbps (typical)

**Per Redis Instance:**
- **CPU**: 0.5-1 core
- **Memory**: 2-4GB (with Sentinel)
- **Network**: 10-50 Mbps

### Concurrent Requests

- **Max Concurrent**: 1000-5000 per replica
- **Recommended**: Keep below 2000 concurrent for best latency
- **Rate Limiting**: Configure per-endpoint limits to prevent overload

---

## Configuration Tuning

### Redis Connection Pool

Optimize Redis connection pooling for your workload:

```yaml
# config.yaml
redis:
  # Connection pool size
  # Rule of thumb: (concurrent_requests / 100) + 10
  pool_size: 100  # Default: 100

  # Minimum idle connections
  # Keep connections warm for low latency
  min_idle_conns: 10  # Default: 10

  # Connection timeouts
  dial_timeout: 5s    # Default: 5s
  read_timeout: 3s    # Default: 3s
  write_timeout: 3s   # Default: 3s

  # Connection lifetime
  max_conn_age: 0     # 0 = no limit (recommended)
  pool_timeout: 4s    # Wait time for connection from pool
  idle_timeout: 5m    # Close idle connections after 5 minutes
```

**Tuning Recommendations:**
- **Low Traffic** (< 100 req/s): pool_size=50, min_idle=5
- **Medium Traffic** (100-1000 req/s): pool_size=100, min_idle=10
- **High Traffic** (> 1000 req/s): pool_size=200, min_idle=20

### Kubernetes Client

Optimize K8s client for large clusters:

```yaml
# config.yaml
kubernetes:
  # QPS and burst limits for K8s API calls
  qps: 50.0         # Default: 50.0
  burst: 100        # Default: 100

  # Cache size for informer cache
  # Increase for large clusters (10000+ resources)
  cache_size_mb: 256  # Default: 256MB

  # Resync period for informers
  # Longer = less API load, but slower drift detection
  resync_period: 10m  # Default: 10m
```

**Tuning Recommendations:**
- **Small Clusters** (< 1000 resources): qps=50, burst=100
- **Medium Clusters** (1000-10000 resources): qps=100, burst=200
- **Large Clusters** (> 10000 resources): qps=200, burst=400, cache_size_mb=512

### HTTP Server

Optimize HTTP server settings:

```yaml
# config.yaml
server:
  # Timeouts
  read_timeout: 30s      # Default: 30s
  write_timeout: 30s     # Default: 30s
  idle_timeout: 120s     # Default: 120s

  # Max header size
  max_header_bytes: 1048576  # 1MB default

  # Keep-alive
  # Enable for better performance with persistent connections
  keep_alive: true

  # Max concurrent streams (HTTP/2)
  max_concurrent_streams: 250  # Default: 250
```

### TLS Performance

Optimize TLS for better performance:

```yaml
# config.yaml
server:
  tls:
    # Prefer modern, fast cipher suites
    cipher_suites:
      - TLS_AES_128_GCM_SHA256
      - TLS_AES_256_GCM_SHA384
      - TLS_CHACHA20_POLY1305_SHA256

    # Enable session resumption for faster reconnections
    session_cache_size: 10000  # Cache up to 10k sessions
    session_tickets: true      # Enable session tickets

    # Prefer server cipher suite order
    prefer_server_cipher_suites: true
```

**TLS Performance Tips:**
- Use hardware AES acceleration if available
- Enable session resumption to avoid full handshakes
- Use modern cipher suites (AES-GCM is fast)
- Consider using ECDSA certificates (faster than RSA)

---

## Caching Strategy

### Cache TTL Recommendations

Configure appropriate TTLs by resource type:

```yaml
# config.yaml
cache:
  # Resource pools change infrequently
  resource_pools_ttl: 5m    # Default: 5 minutes

  # Resources change more frequently
  resources_ttl: 1m         # Default: 1 minute

  # Resource types are mostly static
  resource_types_ttl: 30m   # Default: 30 minutes

  # Deployment managers rarely change
  deployment_managers_ttl: 10m  # Default: 10 minutes

  # Subscriptions are write-heavy, cache briefly
  subscriptions_ttl: 30s    # Default: 30 seconds
```

**Tuning by Use Case:**
- **Read-Heavy Workloads**: Increase TTLs (5-15 minutes)
- **Write-Heavy Workloads**: Decrease TTLs (30s-1m)
- **Strict Consistency**: Disable caching or use very short TTLs (< 30s)
- **Best Performance**: Use long TTLs with cache invalidation on writes

### Cache Size Limits

Configure Redis memory limits:

```yaml
# redis.conf
maxmemory 2gb
maxmemory-policy allkeys-lru  # Evict least recently used keys

# For production with Sentinel
maxmemory 4gb
```

**Memory Calculation:**
```
Cache Memory = (Avg Resource Size × Num Resources × Cache Hit Ratio) / TTL
```

**Example:**
- 10,000 resources
- 5KB average size
- 90% cache hit ratio
- 5 minute TTL
→ ~450MB cache memory needed

### Cache Warming

Warm cache on startup for better performance:

```yaml
# config.yaml
cache:
  warm_on_startup: true

  # Prefetch these resource types on startup
  warm_resources:
    - resource_pools
    - resource_types
    - deployment_managers
```

**When to Use Cache Warming:**
- High-traffic production deployments
- After gateway restarts or updates
- When cache hit ratio is critical

### Cache Invalidation

Strategies for keeping cache fresh:

1. **Time-based (TTL)**: Simple, eventual consistency
2. **Event-based**: Invalidate on resource changes (via informers)
3. **Write-through**: Update cache on write operations
4. **Manual**: Invalidate via admin API when needed

**Recommended Strategy:**
- Use TTL for baseline freshness
- Add event-based invalidation for critical resources
- Manual invalidation for emergencies

---

## Resource Limits

### Gateway Pod Resources

Configure appropriate resource requests and limits:

```yaml
# Kubernetes Deployment
resources:
  requests:
    cpu: "500m"      # 0.5 CPU core
    memory: "512Mi"
  limits:
    cpu: "2000m"     # 2 CPU cores
    memory: "2Gi"
```

**Sizing Guidelines:**

| Load Level | CPU Request | CPU Limit | Memory Request | Memory Limit |
|------------|-------------|-----------|----------------|--------------|
| **Light** (< 100 req/s) | 250m | 1000m | 256Mi | 1Gi |
| **Medium** (100-500 req/s) | 500m | 2000m | 512Mi | 2Gi |
| **Heavy** (500-1000 req/s) | 1000m | 4000m | 1Gi | 4Gi |
| **Very Heavy** (> 1000 req/s) | 2000m | 8000m | 2Gi | 8Gi |

**Best Practices:**
- Set requests = 50-75% of typical usage
- Set limits = 2-3x requests for burst capacity
- Monitor actual usage and adjust accordingly
- Use Horizontal Pod Autoscaler (HPA) for automatic scaling

### Redis Resources

```yaml
# Redis StatefulSet
resources:
  requests:
    cpu: "500m"
    memory: "2Gi"
  limits:
    cpu: "2000m"
    memory: "4Gi"
```

**Redis Sizing:**
- **Memory**: 2-4GB for typical deployments
- **CPU**: 0.5-1 core (Redis is single-threaded)
- **Storage**: 10-20GB for AOF persistence

### Network Bandwidth

**Gateway:**
- **Minimum**: 10 Mbps
- **Recommended**: 100 Mbps
- **High Traffic**: 1 Gbps

**Redis:**
- **Minimum**: 10 Mbps
- **Recommended**: 100 Mbps (for Sentinel replication)

---

## Scaling Recommendations

### Horizontal Scaling (Add Replicas)

**When to Scale Horizontally:**
- CPU usage consistently > 70%
- Memory usage consistently > 80%
- Request latency increasing (p95 > 200ms)
- Rate limit errors (429 responses)
- Planned traffic increase

**How to Scale:**

```bash
# Scale with kubectl
kubectl scale deployment netweave-gateway --replicas=5

# Scale with Helm
helm upgrade netweave netweave/netweave --set replicaCount=5

# Or enable HPA
kubectl autoscale deployment netweave-gateway \
  --cpu-percent=70 \
  --min=3 \
  --max=10
```

**Replica Count Guidelines:**
| Expected RPS | Replicas | Notes |
|--------------|----------|-------|
| 0-500 | 2-3 | Minimum for HA |
| 500-2000 | 3-5 | Typical production |
| 2000-5000 | 5-10 | High traffic |
| > 5000 | 10+ | Scale Redis too |

### Vertical Scaling (Increase Resources)

**When to Scale Vertically:**
- Single replica reaching resource limits
- Complex operations requiring more CPU/memory
- Cache size needs to increase
- Cannot add more replicas (cost/complexity)

**How to Scale:**

```yaml
# Update deployment
resources:
  requests:
    cpu: "2000m"     # Increased from 500m
    memory: "4Gi"    # Increased from 512Mi
  limits:
    cpu: "4000m"
    memory: "8Gi"
```

**When Vertical Scaling is NOT Enough:**
- Bottleneck is external (backend API)
- Need higher availability
- Need better geographic distribution

### Load Balancer Configuration

Optimize load balancer for best performance:

```yaml
# Kubernetes Service
apiVersion: v1
kind: Service
metadata:
  name: netweave-gateway
  annotations:
    # Session affinity for better caching
    service.beta.kubernetes.io/aws-load-balancer-type: "nlb"
spec:
  type: LoadBalancer
  sessionAffinity: ClientIP  # Sticky sessions
  sessionAffinityConfig:
    clientIP:
      timeoutSeconds: 3600   # 1 hour
  ports:
    - port: 443
      targetPort: 8443
```

**Load Balancing Strategies:**
- **Round Robin**: Simple, works well with stateless operations
- **Least Connections**: Better for long-lived connections
- **IP Hash**: Session affinity for better cache locality

### Redis Scaling

**Vertical Scaling (Single Instance):**
```yaml
resources:
  requests:
    memory: "8Gi"    # Increased for larger cache
    cpu: "2000m"
```

**Horizontal Scaling (Sentinel):**
```yaml
# Redis Sentinel with replicas
master:
  replicas: 1
sentinel:
  replicas: 3      # Minimum 3 for quorum
replica:
  replicas: 2      # 2 read replicas
```

**When to Use Redis Cluster:**
- > 100,000 subscriptions
- > 10GB cache size
- Need horizontal scalability beyond Sentinel

---

## Benchmarking

### Load Testing Tools

**Recommended Tools:**

1. **Apache Bench (ab)**
   ```bash
   ab -n 10000 -c 100 -H "Authorization: Bearer TOKEN" \
     https://gateway:8443/o2ims-infrastructureInventory/v1/resourcePools
   ```

2. **wrk**
   ```bash
   wrk -t12 -c400 -d30s --latency \
     https://gateway:8443/o2ims-infrastructureInventory/v1/resourcePools
   ```

3. **k6** (Recommended for complex scenarios)
   ```javascript
   import http from 'k6/http';

   export let options = {
     vus: 100,
     duration: '5m',
   };

   export default function() {
     http.get('https://gateway:8443/o2ims-infrastructureInventory/v1/resourcePools');
   }
   ```

4. **Grafana k6** (Cloud-based)
   - Best for realistic load patterns
   - Built-in reporting and analysis

### Sample Load Test Scenarios

#### Scenario 1: Read-Heavy Workload

```bash
# 80% reads, 20% writes
# 1000 concurrent users
# 5 minute duration

k6 run --vus 1000 --duration 5m load-test-read-heavy.js
```

```javascript
// load-test-read-heavy.js
import http from 'k6/http';
import { check, sleep } from 'k6';

export default function() {
  // 80% chance of read operation
  if (Math.random() < 0.8) {
    let res = http.get('https://gateway:8443/o2ims-infrastructureInventory/v1/resources');
    check(res, { 'status is 200': (r) => r.status === 200 });
  } else {
    // 20% subscription operations
    http.post('https://gateway:8443/o2ims-infrastructureInventory/v1/subscriptions', JSON.stringify({
      callback: 'https://smo.example.com/notify',
      filter: 'resourceType=Node'
    }));
  }

  sleep(1);
}
```

#### Scenario 2: Subscription-Heavy Workload

```javascript
// load-test-subscriptions.js
import http from 'k6/http';

export default function() {
  // Create subscription
  http.post('https://gateway:8443/o2ims-infrastructureInventory/v1/subscriptions', JSON.stringify({
    callback: `https://smo.example.com/notify/${__VU}`,
    filter: 'resourceType=Node'
  }));

  // List subscriptions
  http.get('https://gateway:8443/o2ims-infrastructureInventory/v1/subscriptions');

  sleep(5);
}
```

### Interpreting Results

**Key Metrics:**

```
Requests      [total, rate, throughput]       10000, 200.00, 195.23
Duration      [total, attack, wait]           51.2s, 50s, 1.2s
Latencies     [min, mean, 50, 90, 95, 99, max]
  10ms, 45ms, 40ms, 80ms, 120ms, 250ms, 500ms
Success       [ratio]                         99.5%
```

**What to Look For:**
- **Throughput**: Should match expected req/s per replica
- **Latency p95**: Should be < 100ms (with caching)
- **Latency p99**: Should be < 500ms
- **Success Rate**: Should be > 99%
- **Error Rate**: Should be < 1%

**Warning Signs:**
- ⚠️ p95 > 200ms: Increase replicas or optimize
- ⚠️ p99 > 1s: Backend bottleneck or resource exhaustion
- ⚠️ Success < 99%: Errors, rate limiting, or crashes
- ⚠️ High CPU/Memory: Need vertical scaling

### Baseline Performance Metrics

Establish baseline before optimization:

| Metric | Development | Production | High Performance |
|--------|-------------|------------|------------------|
| **Throughput** | 100 req/s | 1000 req/s | 5000 req/s |
| **Latency p50** | 50ms | 20ms | 10ms |
| **Latency p95** | 200ms | 100ms | 50ms |
| **Latency p99** | 500ms | 300ms | 150ms |
| **Cache Hit Ratio** | 70% | 90% | 95% |
| **CPU Usage** | 30% | 60% | 80% |
| **Memory Usage** | 50% | 70% | 80% |

---

## Common Bottlenecks

### 1. Backend API Slowness

**Symptoms:**
- High p95/p99 latencies
- Adapter metrics show slow backend response times
- CPU/memory usage is normal

**Solutions:**
- Increase cache TTLs to reduce backend calls
- Optimize backend queries (K8s field selectors, label selectors)
- Add backend API rate limiting
- Consider backend scaling (more K8s API servers)

**K8s API Optimization:**
```yaml
kubernetes:
  # Use field selectors to reduce data transfer
  field_selector: "status.phase=Running"

  # Use label selectors
  label_selector: "app=o2ims"

  # Increase QPS for faster operations
  qps: 100
  burst: 200
```

### 2. Redis Latency

**Symptoms:**
- Cache operations slow (> 10ms)
- High Redis CPU usage
- Network latency between gateway and Redis

**Solutions:**
- Deploy Redis in same datacenter/availability zone
- Use Redis Sentinel for HA
- Increase Redis resources (CPU, memory)
- Monitor Redis slow log: `redis-cli slowlog get 10`

**Redis Tuning:**
```bash
# redis.conf
# Disable persistence for better performance (if acceptable)
save ""
appendonly no

# Or use optimized persistence
save 900 1
save 300 10
appendfsync everysec
```

### 3. CPU-Bound Operations

**Symptoms:**
- High CPU usage (> 80%)
- Increased latency under load
- Throttling in metrics

**Solutions:**
- Add more gateway replicas (horizontal scaling)
- Increase CPU limits (vertical scaling)
- Optimize hot code paths (profiling)
- Enable CPU affinity for better performance

**CPU Profiling:**
```bash
# Enable pprof endpoint
curl http://gateway:6060/debug/pprof/profile?seconds=30 > cpu.prof

# Analyze with go tool
go tool pprof cpu.prof
```

### 4. Memory Constraints

**Symptoms:**
- OOMKilled pod restarts
- High memory usage (> 90%)
- Increased garbage collection

**Solutions:**
- Increase memory limits
- Reduce cache size
- Add more replicas to distribute load
- Memory profiling to find leaks

**Memory Profiling:**
```bash
curl http://gateway:6060/debug/pprof/heap > heap.prof
go tool pprof heap.prof
```

### 5. Network Latency

**Symptoms:**
- Consistent baseline latency (> 50ms)
- Geographic distribution issues
- High inter-pod latency

**Solutions:**
- Deploy gateway close to backends
- Use faster network (10GbE, 100GbE)
- Optimize network policies
- Use service mesh for observability

---

## Production Checklist

### Performance Configuration

- [ ] Resource limits configured (CPU, memory)
- [ ] Redis Sentinel deployed (3+ replicas)
- [ ] Multiple gateway replicas (3+ for HA)
- [ ] HPA configured for automatic scaling
- [ ] Cache TTLs optimized for workload
- [ ] TLS session resumption enabled
- [ ] Connection pools tuned

### Monitoring & Alerting

- [ ] Prometheus metrics collected
- [ ] Grafana dashboards deployed
- [ ] Alerts configured:
  - [ ] High latency (p95 > 200ms)
  - [ ] High error rate (> 1%)
  - [ ] Low cache hit ratio (< 80%)
  - [ ] High CPU/memory usage (> 80%)
  - [ ] Pod crashes/restarts
- [ ] Distributed tracing enabled (Jaeger)

### Load Testing

- [ ] Baseline performance established
- [ ] Load test scenarios documented
- [ ] Peak load tested (2x expected traffic)
- [ ] Sustained load tested (8+ hours)
- [ ] Failure scenarios tested:
  - [ ] Redis failover
  - [ ] Pod crashes
  - [ ] Backend API slowdown
  - [ ] Network partitions

### Disaster Recovery

- [ ] Backup procedures documented
- [ ] Recovery time tested (RTO < 1 hour)
- [ ] Redis backup enabled (AOF or RDB)
- [ ] Configuration backed up
- [ ] Runbooks created for common issues

### Optimization Validation

- [ ] Cache hit ratio > 90%
- [ ] p95 latency < 100ms
- [ ] p99 latency < 500ms
- [ ] Error rate < 1%
- [ ] No OOMKilled pods
- [ ] No CPU throttling
- [ ] Webhook delivery > 99% success

---

## See Also

- **[Configuration Reference](../configuration/reference.md)** - Complete configuration options
- **[Monitoring Guide](monitoring.md)** - Metrics and dashboards
- **[Troubleshooting Guide](troubleshooting.md)** - Common issues and solutions
- **[High Availability Guide](../architecture/high-availability.md)** - HA architecture
- **[Deployment Guide](deployment.md)** - Production deployment
