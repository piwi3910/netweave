# O2-IMS Gateway Operations Documentation

Comprehensive operational documentation for the netweave O2-IMS Gateway. This documentation covers deployment, monitoring, troubleshooting, maintenance, and incident response procedures.

## Documentation Structure

| Document | Description | Use When |
|----------|-------------|----------|
| **[Deployment](deployment.md)** | Deployment strategies and procedures | Setting up or updating gateway deployments |
| **[Monitoring](monitoring.md)** | Metrics, dashboards, and alerts | Setting up observability infrastructure |
| **[Troubleshooting](troubleshooting.md)** | Common issues and debugging | Investigating problems or incidents |
| **[Backup & Recovery](backup-recovery.md)** | Backup strategies and disaster recovery | Planning DR or recovering from failures |
| **[Upgrades](upgrades.md)** | Version upgrades and rollbacks | Upgrading to new versions |
| **[Runbooks](runbooks.md)** | Incident response procedures | During incidents or on-call rotations |

## Quick Reference

### Prerequisites

**Required Infrastructure:**
- Kubernetes 1.30+ cluster
- Redis 7.4+ (Sentinel mode for HA)
- cert-manager 1.15+ (for TLS)
- Prometheus 2.x+ (for monitoring)
- Grafana 9.x+ (for visualization)

**Access Requirements:**
- Kubernetes cluster admin access
- Redis administrative access
- Network access to backend systems (K8s API, cloud APIs)
- Certificate authority access (for mTLS)

### Common Operations

**Check Gateway Health:**
```bash
kubectl get pods -n o2ims-system -l app=netweave-gateway
kubectl logs -n o2ims-system -l app=netweave-gateway --tail=50
curl -k https://netweave-gateway.o2ims-system.svc.cluster.local/healthz
```

**Check Redis Status:**
```bash
kubectl exec -n o2ims-system redis-node-0 -- redis-cli INFO replication
kubectl exec -n o2ims-system redis-sentinel-0 -- redis-cli -p 26379 SENTINEL master mymaster
```

**View Metrics:**
```bash
kubectl port-forward -n o2ims-system svc/netweave-gateway 8080:8080
curl http://localhost:8080/metrics
```

**Check Subscriptions:**
```bash
kubectl exec -n o2ims-system redis-node-0 -- redis-cli KEYS "subscription:*"
kubectl exec -n o2ims-system redis-node-0 -- redis-cli GET "subscription:sub-123"
```

### Emergency Contacts

**Escalation Path:**
1. **Level 1**: On-call engineer (responds within 15 min)
2. **Level 2**: Platform team lead (responds within 30 min)
3. **Level 3**: Engineering manager (responds within 1 hour)

**External Contacts:**
- Redis Support: [redis-support@example.com](mailto:redis-support@example.com)
- Kubernetes Support: [k8s-support@example.com](mailto:k8s-support@example.com)
- Cloud Provider: [cloud-support@example.com](mailto:cloud-support@example.com)

## SLO Targets

| Metric | Target | Measurement Window |
|--------|--------|-------------------|
| **Availability** | 99.9% (43m downtime/month) | 30 days |
| **API Latency (p95)** | < 100ms | 5 minutes |
| **API Latency (p99)** | < 500ms | 5 minutes |
| **Error Rate** | < 1% | 5 minutes |
| **Cache Hit Ratio** | > 90% | 10 minutes |
| **Webhook Delivery** | < 1s end-to-end | 5 minutes |

## Architecture Overview

```mermaid
graph TB
    subgraph "External"
        SMO[O2 SMO]
        DMS[DMS Systems]
    end

    subgraph "Gateway Layer"
        GW1[Gateway Pod 1]
        GW2[Gateway Pod 2]
        GW3[Gateway Pod 3]
    end

    subgraph "Storage Layer"
        Redis1[Redis Primary]
        Redis2[Redis Replica 1]
        Redis3[Redis Replica 2]
        Sentinel1[Sentinel 1]
        Sentinel2[Sentinel 2]
        Sentinel3[Sentinel 3]
    end

    subgraph "Backend Systems"
        K8s[Kubernetes API]
        AWS[AWS API]
        GCP[GCP API]
        VMware[VMware vCenter]
    end

    subgraph "Observability"
        Prom[Prometheus]
        Graf[Grafana]
        Jaeger[Jaeger]
    end

    SMO -->|mTLS| GW1
    SMO -->|mTLS| GW2
    SMO -->|mTLS| GW3
    DMS -->|mTLS| GW1
    DMS -->|mTLS| GW2

    GW1 --> Redis1
    GW2 --> Redis1
    GW3 --> Redis1

    Redis1 --> Redis2
    Redis1 --> Redis3

    Sentinel1 -.monitor.-> Redis1
    Sentinel2 -.monitor.-> Redis2
    Sentinel3 -.monitor.-> Redis3

    GW1 --> K8s
    GW1 --> AWS
    GW2 --> GCP
    GW3 --> VMware

    GW1 --> Prom
    GW2 --> Prom
    GW3 --> Prom
    Prom --> Graf
    GW1 --> Jaeger

    style External fill:#e1f5ff
    style "Gateway Layer" fill:#fff4e6
    style "Storage Layer" fill:#ffe6f0
    style "Backend Systems" fill:#e8f5e9
    style Observability fill:#f3e5f5
```

## Key Concepts

### Stateless Gateway Design

The gateway is **completely stateless**. All state (subscriptions, cache) is stored in Redis. This enables:
- **Horizontal scaling**: Add/remove pods without coordination
- **Zero-downtime deployments**: Rolling updates with no impact
- **Multi-cluster support**: Share state across clusters via Redis replication

### Subscription Management

Subscriptions are stored in Redis with the following lifecycle:
1. **Creation**: SMO creates subscription via POST /subscriptions
2. **Storage**: Gateway stores in Redis with TTL
3. **Watch**: Gateway watches backend resources for changes
4. **Notification**: Gateway sends webhook on resource changes
5. **Renewal**: SMO renews subscription before expiry
6. **Deletion**: Explicit DELETE or TTL expiry

### Adapter Pattern

The gateway uses adapters to interface with different backend systems:
- **Kubernetes**: Native K8s client-go
- **AWS**: AWS SDK for Go
- **GCP**: GCP Cloud SDK
- **VMware**: govmomi library

Each adapter implements the same interface, enabling consistent operations across all backends.

## Deployment Topologies

### Development

**Single cluster, minimal redundancy:**
- 1 gateway pod
- 1 Redis instance (no Sentinel)
- Basic TLS (self-signed)
- Minimal monitoring

**Use for:**
- Local development
- Feature testing
- CI/CD pipelines

### Staging

**Production-like, single cluster:**
- 3 gateway pods
- Redis Sentinel (3 nodes, 3 sentinels)
- mTLS with cert-manager
- Full observability stack

**Use for:**
- Pre-production testing
- Load testing
- Integration testing

### Production

**Multi-cluster, full redundancy:**
- 3+ gateway pods per cluster
- Redis Sentinel with cross-cluster replication
- mTLS with automated cert rotation
- Full observability with high availability
- Multi-region deployment (optional)

**Use for:**
- Production workloads
- Mission-critical services

## Security Considerations

### Network Security

- **mTLS**: All external communication uses mutual TLS
- **Network Policies**: Restrict pod-to-pod communication
- **Ingress Rules**: Whitelist trusted sources only
- **Egress Rules**: Limit outbound connections to known backends

### Authentication & Authorization

- **Client Certificates**: X.509 certificates for SMO authentication
- **RBAC**: Kubernetes RBAC for service account permissions
- **Secrets Management**: Kubernetes Secrets for sensitive data
- **Audit Logging**: All API calls logged for compliance

### Certificate Management

- **cert-manager**: Automated certificate issuance and renewal
- **CA Hierarchy**: Root CA → Intermediate CA → Leaf certificates
- **Certificate Rotation**: Automated rotation 30 days before expiry
- **Certificate Monitoring**: Alerts 14 days before expiry

## Monitoring & Alerting

### Critical Alerts (P1)

**Immediate response required (< 15 min):**
- Gateway pods down (< 2 healthy replicas)
- Redis primary down (no primary elected)
- API error rate > 10%
- p99 latency > 1s
- Certificate expiring < 7 days

### Warning Alerts (P2)

**Response required within 1 hour:**
- API error rate > 5%
- p99 latency > 500ms
- Cache hit ratio < 90%
- Redis replica down
- Certificate expiring < 14 days

### Informational Alerts (P3)

**Review during business hours:**
- Resource pool count drop > 20%
- No subscription activity for 10+ minutes
- Backend API latency increasing

## Operational Runbooks

### Daily Operations

**Morning Checks (10 min):**
1. Review overnight alerts
2. Check Grafana dashboards
3. Verify all pods healthy
4. Check Redis replication lag
5. Review error logs

**Weekly Maintenance (30 min):**
1. Review capacity metrics
2. Check for pending upgrades
3. Test backup/restore procedures
4. Review security advisories
5. Update runbooks

### Incident Response

**During Incident:**
1. Acknowledge alert
2. Check incident severity (see runbooks.md)
3. Follow appropriate runbook
4. Document actions in incident ticket
5. Communicate status updates

**Post-Incident:**
1. Write incident report
2. Document root cause
3. Create action items
4. Update runbooks
5. Conduct blameless postmortem

## Performance Tuning

### Gateway Configuration

**CPU/Memory:**
- Development: 100m CPU, 128Mi memory
- Staging: 500m CPU, 512Mi memory
- Production: 1000m CPU, 1Gi memory

**Redis Configuration:**
```yaml
maxmemory: 2gb
maxmemory-policy: allkeys-lru
save: "900 1 300 10 60 10000"
appendonly: yes
appendfsync: everysec
```

**Cache TTLs:**
- Resource lists: 30s
- Individual resources: 60s
- Resource pools: 300s
- Capabilities: 3600s

## Compliance & Audit

### Audit Log Requirements

**Logged Information:**
- Client identity (from certificate CN)
- Operation performed (GET, POST, DELETE, etc.)
- Resource accessed
- Timestamp (UTC)
- Response status code
- Request/response bodies (for POST/PUT)

**Retention:**
- Development: 7 days
- Staging: 30 days
- Production: 90 days (365 days for compliance)

**Access:**
- Encrypted at rest
- Access logged and audited
- Restricted to authorized personnel

## Related Documentation

- [Architecture Documentation](../architecture.md)
- [API Mapping Guide](../api-mapping.md)
- [Development Guide](../development.md)
- [Contributing Guide](../CONTRIBUTING.md)

## Getting Help

**Documentation Issues:**
- File issues at [GitHub Issues](https://github.com/piwi3910/netweave/issues)
- Tag with `documentation` label

**Operational Issues:**
- Check [Troubleshooting Guide](troubleshooting.md)
- Search [Runbooks](runbooks.md)
- Contact on-call engineer

**Enhancement Requests:**
- Discuss in GitHub Discussions
- Create enhancement issue
- Tag with `enhancement` label
