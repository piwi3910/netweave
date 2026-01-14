# Frequently Asked Questions (FAQ)

Common questions about netweave O2 Gateway implementation, deployment, and operation.

## Table of Contents

- [General Questions](#general-questions)
- [Installation & Setup](#installation--setup)
- [Architecture & Design](#architecture--design)
- [Performance & Scaling](#performance--scaling)
- [Security](#security)
- [Operations](#operations)
- [Development](#development)
- [O-RAN Compliance](#o-ran-compliance)
- [Troubleshooting](#troubleshooting)

---

## General Questions

### What is netweave?

netweave is a production-grade O-RAN O2 Gateway implementing O2-IMS, O2-DMS, and O2-SMO APIs for telecom-grade cloud-native infrastructure management. It provides a unified interface to manage infrastructure resources across multiple cloud and on-premise environments.

### What is O2-IMS?

O2-IMS (O-RAN Infrastructure Management Services) is a standard API defined by the O-RAN Alliance for managing RAN infrastructure resources. It provides standardized interfaces for resource pools, resources, resource types, deployment managers, and subscriptions.

### What is O2-DMS?

O2-DMS (O-RAN Deployment Management Services) is a standard API for managing the lifecycle of Cloud Native Functions (CNFs) and Virtual Network Functions (VNFs). It handles package management, deployment orchestration, scaling, rollback, and operational tasks.

### What is O2-SMO?

O2-SMO (O-RAN Service Management & Orchestration) is the integration API enabling SMO systems (like ONAP and OSM) to orchestrate services across O-Cloud infrastructure.

### Is this production-ready?

**O2-IMS**: ✅ Production-ready with 95% O-RAN v3.0.0 spec compliance
**O2-DMS**: ✅ Production-ready with 95% O-RAN v3.0.0 spec compliance
**O2-SMO**: ✅ Operational with 90% spec compliance

All core adapters are functional and can be used in production with proper error handling, logging, and metrics.

### What backends are supported?

**Production-Ready:**
- Kubernetes (100% complete, ≥80% test coverage)

**Functional (70% complete):**
- AWS, Azure, GCP
- OpenStack, VMware vSphere
- DTIAS

**DMS Adapters (85-90% complete):**
- Helm 3, ArgoCD, Flux CD
- Kustomize, Crossplane
- ONAP-LCM, OSM-LCM

### How does netweave differ from other O2-IMS implementations?

- **Multi-backend support**: Pluggable adapter architecture for diverse infrastructure
- **Production-grade**: mTLS, distributed rate limiting, comprehensive observability
- **Cloud-native**: Stateless gateway pods, Redis-based state, Kubernetes-native
- **Extensible**: 25+ adapters across IMS, DMS, and SMO categories

---

## Installation & Setup

### What are the minimum requirements?

**Runtime:**
- Go 1.25.0+ (required by k8s.io/client-go v0.35.0)
- Kubernetes 1.28+ cluster
- Redis 7.4+ (Sentinel for HA)
- 2 CPU cores, 4GB RAM minimum per gateway pod

**Development:**
- Docker (for container builds)
- make, git
- golangci-lint, gosec (for quality checks)

### Can I run this without Kubernetes?

Yes! The Kubernetes adapter is one of many backends. You can use:
- Cloud provider adapters (AWS, Azure, GCP)
- Private cloud adapters (OpenStack, VMware)
- Custom adapters for your infrastructure

However, the gateway itself runs as Kubernetes pods. For non-Kubernetes deployments, you would need to run the gateway binary directly with appropriate configuration.

### How do I get started quickly?

See [Quick Start Guide](../getting-started/quickstart.md) for a 5-minute setup, or [Installation Guide](../getting-started/installation.md) for detailed production deployment.

**Quick start:**
```bash
# Deploy with Helm
helm repo add netweave https://piwi3910.github.io/netweave
helm install netweave netweave/netweave

# Verify
kubectl get pods -n netweave
```

### Do I need TLS in development?

No, TLS is optional in development mode (`NETWEAVE_ENV=dev`). However, TLS is **required** for production deployments with mTLS client authentication enabled.

### How do I configure the gateway?

Configuration via:
1. **Environment variables** (e.g., `NETWEAVE_REDIS_ADDR`)
2. **ConfigMap** (for Kubernetes deployments)
3. **Helm values** (for Helm chart deployments)

See [Configuration Guide](../configuration/README.md) for complete reference.

---

## Architecture & Design

### How does the adapter pattern work?

Backend adapters implement the O2-IMS interface and translate API calls to provider-specific APIs:

```
O2-IMS Request → Router → Adapter → Backend API → Backend
                                ↓
                         Transform Response
                                ↓
                         O2-IMS Response
```

Each adapter handles:
- Resource discovery and transformation
- Backend-specific authentication
- Error handling and retries
- Health checking

See [Adapter Architecture](../adapters/README.md) for details.

### Can I use multiple backends simultaneously?

Yes! The unified plugin registry supports:
- **Multiple adapters per category** (IMS, DMS, SMO)
- **Priority-based selection** with criteria matching
- **Intelligent routing** based on resource type, labels, or location
- **Aggregation mode** for querying multiple backends

See [Routing Configuration](../configuration/adapters.md) for details.

### How are subscriptions handled?

The subscription system uses Kubernetes informers to watch resource changes:

1. **Subscription Controller** watches Subscription CRDs
2. **Event Generator** creates events from resource changes
3. **Event Filter** matches events to subscriptions
4. **Event Queue** buffers events for delivery
5. **Notifier** delivers webhooks with retry logic

See [Subscription Architecture](../api/o2ims/subscriptions.md) for details.

### What happens if Redis fails?

**With Redis Sentinel (Recommended):**
- Automatic failover to replica (~1-2 seconds)
- Subscriptions persist through failover
- Gateway continues operating with brief interruption

**Without Sentinel:**
- Subscriptions lost until Redis recovers
- Gateway cannot track state or deliver events
- Requires manual intervention

See [High Availability Guide](../architecture/high-availability.md).

### How does caching work?

Redis-based caching with:
- **TTL-based expiration** (configurable per resource type)
- **Cache invalidation** on resource changes
- **Write-through caching** for consistency
- **Cache warming** on startup

Target: >90% cache hit ratio for read operations.

---

## Performance & Scaling

### How many requests per second can it handle?

**Typical Performance:**
- 1000+ req/s per replica with caching enabled
- p95 latency < 100ms
- p99 latency < 500ms

Performance depends on:
- Backend response time
- Cache hit ratio
- Request complexity
- Resource count

See [Performance Tuning Guide](../operations/performance.md).

### How do I scale the gateway?

**Horizontal Scaling:**
```bash
# Increase replicas
kubectl scale deployment netweave-gateway --replicas=5

# Or via Helm
helm upgrade netweave netweave/netweave --set replicaCount=5
```

**Vertical Scaling:**
- Increase CPU/memory per pod
- Recommended: 2 CPU cores, 2GB RAM per replica

**Redis Scaling:**
- Use Redis Sentinel with 3+ replicas
- Enable Redis Cluster for >100k subscriptions

### What are the resource requirements?

**Per Gateway Replica:**
- **Minimum**: 1 CPU core, 512MB RAM
- **Recommended**: 2 CPU cores, 2GB RAM
- **High Load**: 4 CPU cores, 4GB RAM

**Redis:**
- **Minimum**: 1 CPU core, 2GB RAM
- **Recommended**: 2 CPU cores, 4GB RAM (with Sentinel)

### How do I optimize performance?

1. **Enable caching** with appropriate TTLs
2. **Increase replicas** for horizontal scaling
3. **Use Redis Sentinel** for HA
4. **Tune rate limits** to prevent overload
5. **Monitor metrics** and adjust based on traffic

See [Performance Tuning Guide](../operations/performance.md).

---

## Security

### How is authentication handled?

**Production:**
- mTLS (Mutual TLS) with client certificate authentication
- Client identity extracted from certificate CN
- TLS 1.3 with strong cipher suites

**Development:**
- Optional mTLS
- Bearer token authentication (testing only)

See [Security Configuration](../configuration/security.md).

### Is multi-tenancy supported?

**Current Status:** 🔄 Planned for v2.0 (Q3 2026)

Multi-tenancy and RBAC are not yet implemented. Current capabilities:
- Single-tenant operation
- mTLS authentication
- Per-endpoint rate limiting
- Audit logging

See [Roadmap](../../README.md#roadmap) for planned features.

### How are secrets managed?

**Kubernetes Deployments:**
- Kubernetes Secrets for credentials
- cert-manager for TLS certificate management
- Optional: External Secrets Operator

**Standalone Deployments:**
- Environment variables (not recommended for production)
- File-based secrets with restricted permissions
- External secret managers (Vault, AWS Secrets Manager)

See [Secrets Management](../security/secrets.md).

### Is rate limiting supported?

Yes! Distributed rate limiting with:
- **Per-endpoint limits** (e.g., 100 req/min for subscriptions)
- **Global limits** (e.g., 10000 req/min total)
- **Standard HTTP headers** (X-RateLimit-Limit, X-RateLimit-Remaining)
- **Graceful degradation** if Redis unavailable

See [Rate Limiting Configuration](../configuration/security.md#rate-limiting).

### How do I enable webhook HMAC signatures?

Configure webhook signing:
```yaml
webhooks:
  signing:
    enabled: true
    algorithm: hmac-sha256
    secretKey: ${WEBHOOK_SECRET}
```

Subscribers verify signatures using:
- Header: `X-Webhook-Signature`
- Format: `sha256=<hex-digest>`

See [Webhook Security Guide](../webhook-security.md).

---

## Operations

### How do I monitor the gateway?

**Prometheus Metrics:**
- HTTP request metrics (latency, status codes)
- Subscription metrics (active, delivered, failed)
- Cache metrics (hit ratio, size)
- Adapter metrics (health, latency)

**Grafana Dashboards:**
- Gateway overview dashboard
- Subscription monitoring dashboard
- Performance dashboard

**Distributed Tracing:**
- Jaeger integration via OpenTelemetry
- Request tracing across adapters

See [Monitoring Guide](../operations/monitoring.md).

### What logs are available?

**Structured JSON Logs:**
```json
{
  "level": "info",
  "ts": "2026-01-14T14:00:00Z",
  "msg": "subscription created",
  "subscriptionID": "sub-123",
  "callback": "https://smo.example.com/notify"
}
```

**Log Levels:**
- `debug`: Detailed debugging information
- `info`: General operational information (default)
- `warn`: Warning conditions
- `error`: Error conditions

Configure via `NETWEAVE_LOG_LEVEL` environment variable.

### How do I troubleshoot issues?

**Common Issues:**
1. **Gateway not starting** → Check Redis connectivity, TLS certificates
2. **Webhooks not delivered** → Check callback URL, HMAC signature
3. **High latency** → Check cache hit ratio, backend performance
4. **Subscription events missing** → Check informer configuration

See [Troubleshooting Guide](../operations/troubleshooting.md) for detailed diagnostics.

### How do I backup data?

**What to Backup:**
- Redis data (subscriptions, cache)
- TLS certificates and keys
- Configuration files
- Custom adapter configurations

**Backup Methods:**
- Redis AOF/RDB snapshots
- Kubernetes Secret/ConfigMap exports
- Velero for complete cluster backups

See [Backup & Recovery Guide](../operations/backup-recovery.md).

### How do I upgrade the gateway?

**Rolling Upgrade:**
```bash
# Update Helm chart
helm upgrade netweave netweave/netweave --version 1.5.0

# Or update image tag
kubectl set image deployment/netweave-gateway \
  gateway=piwi3910/netweave:v1.5.0
```

**Upgrade Process:**
1. Review release notes for breaking changes
2. Backup current state
3. Perform rolling upgrade
4. Verify new version health
5. Monitor for issues

See [Upgrade Guide](../operations/upgrades.md).

---

## Development

### How can I contribute?

We welcome contributions! See [Contributing Guide](../../CONTRIBUTING.md) for:
- Development environment setup
- Code quality standards
- Testing requirements
- Pull request process

### How do I create a custom adapter?

**Steps:**
1. Implement the `adapter.Adapter` interface
2. Register with the adapter registry
3. Add configuration support
4. Write comprehensive tests (≥80% coverage)
5. Add documentation

See [Backend Plugins Guide](../adapters/README.md) for detailed adapter development.

### What is the code quality standard?

**Zero Tolerance:**
- ✅ All linters must pass (`make lint`)
- ✅ Test coverage ≥80% (`make test-coverage`)
- ✅ No security vulnerabilities (`make security-scan`)
- ✅ Documentation updated for changes

**Never Allowed:**
- ❌ `//nolint` directives
- ❌ Hardcoded secrets
- ❌ Unchecked errors
- ❌ Code changes without tests

See [CLAUDE.md](../../CLAUDE.md) for complete standards.

### Where can I ask questions?

- **GitHub Discussions**: Community Q&A
- **GitHub Issues**: Bug reports, feature requests
- **Documentation**: Comprehensive guides at `docs/`
- **Source Code**: Inline comments and examples

---

## O-RAN Compliance

### What O-RAN spec versions are supported?

- **O2-IMS v3.0.0**: 95% compliant
- **O2-DMS v3.0.0**: 95% compliant
- **O2-SMO v3.0.0**: 90% compliant

See [Implementation Status](../IMPLEMENTATION_STATUS.md) for detailed compliance tracking.

### Are all O2-IMS endpoints implemented?

**Fully Implemented:**
- ✅ GET /resourcePools, /resourcePools/:id
- ✅ GET /resources, /resources/:id
- ✅ GET /resourceTypes, /resourceTypes/:id
- ✅ GET /deploymentManagers, /deploymentManagers/:id
- ✅ GET /subscriptions, /subscriptions/:id
- ✅ POST /subscriptions
- ✅ DELETE /subscriptions/:id

**Not Exposed via API (Adapter-level only):**
- ⚠️ POST /resourcePools, /resources (create operations)
- ⚠️ PATCH /resourcePools, /resources (update operations)
- ⚠️ DELETE /resourcePools, /resources (delete operations)

See [API Coverage](../../README.md#o2-ims-api-coverage) for complete status.

### How is compliance validated?

**Automated Testing:**
- OpenAPI 3.0 spec validation
- Compliance checker runs in CI
- Integration tests against real backends
- E2E tests for critical workflows

**Manual Testing:**
- Interoperability testing with SMO systems
- O-RAN Alliance test suites (when available)

See [Compliance Documentation](../compliance/README.md).

### What's missing from full O-RAN compliance?

**O2-IMS (5%):**
- Some advanced filtering options
- Batch operations
- Complete sorting support

**O2-DMS (5%):**
- Some adapter-specific lifecycle hooks
- Advanced orchestration workflows

**O2-SMO (10%):**
- Some policy management features
- Advanced workflow orchestration

See [Implementation Status](../IMPLEMENTATION_STATUS.md) for details.

---

## Troubleshooting

### Gateway pods are CrashLooping

**Common Causes:**
1. Redis not accessible → Check `NETWEAVE_REDIS_ADDR`
2. TLS certificate issues → Verify cert-manager or manual certs
3. Backend API unreachable → Check adapter configuration
4. Out of memory → Increase pod memory limits

**Diagnostics:**
```bash
# Check pod logs
kubectl logs -n netweave deployment/netweave-gateway

# Check pod events
kubectl describe pod -n netweave <pod-name>
```

### Subscriptions not receiving events

**Checklist:**
1. ✅ Subscription created successfully?
2. ✅ Callback URL accessible from gateway?
3. ✅ HMAC signature verification correct?
4. ✅ Informer watching correct resources?
5. ✅ Event filter matching correctly?

**Debug:**
```bash
# Check subscription status
kubectl get subscriptions -n netweave

# Check notifier metrics
curl http://gateway:8080/metrics | grep notifier
```

### High API latency

**Potential Causes:**
1. Low cache hit ratio → Increase TTLs, warm cache
2. Backend API slow → Check adapter metrics
3. Too many concurrent requests → Increase replicas
4. Redis slow → Check Redis metrics, add replicas

**Optimization:**
```bash
# Check cache metrics
curl http://gateway:8080/metrics | grep cache

# Check adapter latency
curl http://gateway:8080/metrics | grep adapter_request_duration
```

### Redis connection failures

**Troubleshooting:**
1. Check Redis is running: `kubectl get pods -n redis`
2. Verify network connectivity: `kubectl exec -it <gateway-pod> -- nc -zv redis 6379`
3. Check credentials: Verify `NETWEAVE_REDIS_PASSWORD`
4. Check TLS config: If Redis uses TLS, ensure `NETWEAVE_REDIS_TLS_ENABLED=true`

### Rate limiting errors (429 responses)

**Understanding Rate Limits:**
- Per-endpoint limits protect specific operations
- Global limits protect overall gateway capacity
- Headers show current limits: `X-RateLimit-Limit`, `X-RateLimit-Remaining`

**Solutions:**
1. Increase rate limits in configuration
2. Add more gateway replicas
3. Implement client-side backoff
4. Use exponential retry with jitter

---

## See Also

- **[Getting Started](../getting-started/README.md)** - Installation and setup
- **[Architecture](../architecture/README.md)** - System design and components
- **[API Reference](../api/README.md)** - Complete API documentation
- **[Operations](../operations/README.md)** - Deployment and monitoring
- **[Security](../security/README.md)** - Security architecture and hardening
- **[Troubleshooting](../operations/troubleshooting.md)** - Detailed diagnostics
- **[Glossary](glossary.md)** - Technical terms and acronyms
