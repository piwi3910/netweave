# Operational Runbooks

Incident response procedures and operational playbooks for the netweave O2-IMS Gateway.

## Using This Guide

**During an Incident:**
1. Identify severity level (P1-P4)
2. Follow incident response procedure
3. Locate specific runbook for the issue
4. Execute steps systematically
5. Document actions taken
6. Update runbook with lessons learned

**Incident Severity Levels:**

| Level | Response Time | Description | Examples |
|-------|--------------|-------------|----------|
| **P1** | < 15 min | Service down, data loss | All pods down, Redis data lost |
| **P2** | < 1 hour | Degraded service | High error rate, latency spikes |
| **P3** | < 4 hours | Minor issues | Single pod down, cert expiring soon |
| **P4** | Next business day | Maintenance, questions | Config optimization, documentation |

## Incident Response Procedure

### Step 1: Initial Response (First 5 Minutes)

```bash
# Acknowledge alert
# In PagerDuty, Slack, or monitoring system

# Join incident channel
# #incident-<timestamp> in Slack

# Quick assessment
kubectl get pods -n o2ims-system
kubectl get events -n o2ims-system --sort-by='.lastTimestamp' | tail -20

# Check monitoring dashboards
# Open Grafana: http://grafana.example.com/d/o2ims-gateway

# Initial status update
# Post in incident channel:
# "P<X> incident acknowledged. Investigating..."
```

### Step 2: Triage (5-15 Minutes)

```bash
# Identify affected components
kubectl get pods -n o2ims-system -o wide
kubectl top pods -n o2ims-system

# Check recent changes
kubectl rollout history deployment/netweave-gateway -n o2ims-system | tail -5
helm history netweave -n o2ims-system | tail -5

# Check error logs
kubectl logs -n o2ims-system -l app.kubernetes.io/name=netweave --tail=100 | grep ERROR

# Update severity if needed
# Post in incident channel:
# "Issue identified: <description>. Executing Runbook <NAME>"
```

### Step 3: Execute Runbook (15-60 Minutes)

```bash
# Follow specific runbook below
# Document all actions taken
# Update incident channel every 15 minutes

# Example update:
# "15:30 - Executed rollback, monitoring recovery"
# "15:45 - Service restored, error rate dropping"
# "16:00 - Incident resolved, entering monitoring period"
```

### Step 4: Resolution and Monitoring (1-24 Hours)

```bash
# Verify service restored
curl -k https://o2ims.example.com/healthz
kubectl logs -n o2ims-system -l app.kubernetes.io/name=netweave --tail=50

# Monitor metrics for stability
# Watch for 30 minutes minimum

# Post-incident tasks
# - Create incident report
# - Schedule postmortem
# - Create follow-up tasks
# - Update runbook if needed
```

## Runbook Index

| Runbook | Severity | Common Alerts | Response Time |
|---------|----------|---------------|---------------|
| [RB-001: All Pods Down](#rb-001-all-pods-down) | P1 | GatewayDown | < 15 min |
| [RB-002: Redis Primary Down](#rb-002-redis-primary-down) | P1 | RedisPrimaryDown | < 15 min |
| [RB-003: High Error Rate](#rb-003-high-error-rate) | P2 | HighErrorRate | < 1 hour |
| [RB-004: High Latency](#rb-004-high-latency) | P2 | HighLatency | < 1 hour |
| [RB-005: Certificate Expiring](#rb-005-certificate-expiring) | P3 | CertExpiringSoon | < 4 hours |
| [RB-006: Pod CrashLoopBackOff](#rb-006-pod-crashloopbackoff) | P2 | PodCrashLoop | < 1 hour |
| [RB-007: Webhook Delivery Failures](#rb-007-webhook-delivery-failures) | P2 | WebhookFailures | < 1 hour |
| [RB-008: Cache Hit Ratio Low](#rb-008-cache-hit-ratio-low) | P3 | LowCacheHitRatio | < 4 hours |
| [RB-009: Kubernetes API Errors](#rb-009-kubernetes-api-errors) | P2 | K8sAPIErrors | < 1 hour |
| [RB-010: Subscription Data Loss](#rb-010-subscription-data-loss) | P1 | DataLoss | < 15 min |

---

## RB-001: All Pods Down

**Severity:** P1 - Critical
**Response Time:** < 15 minutes
**Impact:** Complete service outage

### Symptoms

- All gateway pods in CrashLoopBackOff or Pending state
- Health checks failing
- API completely unavailable
- Alert: `GatewayDown`

### Investigation

```bash
# Check pod status
kubectl get pods -n o2ims-system -l app.kubernetes.io/name=netweave

# Check recent events
kubectl get events -n o2ims-system --sort-by='.lastTimestamp' | tail -20

# Check logs from failed pods
kubectl logs -n o2ims-system -l app.kubernetes.io/name=netweave --all-containers --previous

# Check deployment status
kubectl describe deployment netweave-gateway -n o2ims-system

# Check for resource constraints
kubectl describe nodes | grep -A 5 "Allocated resources"
```

### Resolution

#### Scenario A: Recent Bad Deployment

```bash
# Rollback to previous version
helm rollback netweave -n o2ims-system

# Monitor rollback
kubectl rollout status deployment/netweave-gateway -n o2ims-system -w

# Verify recovery
kubectl get pods -n o2ims-system -l app.kubernetes.io/name=netweave
curl -k https://o2ims.example.com/healthz

# Expected recovery time: 2-3 minutes
```

#### Scenario B: Resource Exhaustion

```bash
# Check available resources
kubectl describe nodes | grep -A 10 "Allocated resources"

# If no resources available, scale down non-critical workloads
kubectl scale deployment <non-critical-app> --replicas=0

# Or add nodes to cluster (cloud provider specific)
# AWS:
eksctl scale nodegroup --cluster=<cluster> --nodes=5 --nodes-min=3 --nodes-max=10 <nodegroup>

# Wait for new nodes
kubectl get nodes -w

# Pods should auto-schedule to new nodes
kubectl get pods -n o2ims-system -w
```

#### Scenario C: Configuration Error

```bash
# Check ConfigMap for errors
kubectl get configmap netweave-config -n o2ims-system -o yaml

# Compare with last known good configuration
kubectl rollout history deployment/netweave-gateway -n o2ims-system
kubectl get configmap netweave-config -n o2ims-system --revision=<previous>

# Restore good configuration
kubectl apply -f backup-configmap.yaml

# Restart pods
kubectl rollout restart deployment/netweave-gateway -n o2ims-system
```

#### Scenario D: Redis Unavailable

```bash
# Check Redis status
kubectl get pods -n o2ims-system -l app.kubernetes.io/name=redis

# If Redis down, see RB-002

# If Redis healthy but unreachable, check network
kubectl exec -n o2ims-system netweave-gateway-0 -- \
  redis-cli -h redis-node-0 PING

# Check NetworkPolicies
kubectl get networkpolicy -n o2ims-system
kubectl describe networkpolicy -n o2ims-system
```

### Verification

```bash
# All pods running
kubectl get pods -n o2ims-system -l app.kubernetes.io/name=netweave
# Expected: 3/3 pods Running, 1/1 Ready

# Health check passing
curl -k https://o2ims.example.com/healthz
# Expected: HTTP 200 OK

# API responding
curl -k https://o2ims.example.com/o2ims-infrastructureInventory/v1/api_versions
# Expected: JSON response with API versions

# Metrics available
kubectl exec -n o2ims-system netweave-gateway-0 -- \
  wget -qO- http://localhost:8080/metrics | grep o2ims_up
# Expected: o2ims_up 1
```

### Post-Incident

- [ ] Create incident report
- [ ] Identify root cause
- [ ] Create preventive actions
- [ ] Update monitoring thresholds
- [ ] Schedule postmortem meeting

---

## RB-002: Redis Primary Down

**Severity:** P1 - Critical
**Response Time:** < 15 minutes
**Impact:** Service unavailable, no new subscriptions

### Symptoms

- Gateway pods can't connect to Redis
- Subscriptions not being created/updated
- Alert: `RedisPrimaryDown`

### Investigation

```bash
# Check Redis pod status
kubectl get pods -n o2ims-system -l app.kubernetes.io/name=redis

# Check Sentinel status
kubectl exec -n o2ims-system redis-sentinel-0 -- \
  redis-cli -p 26379 SENTINEL masters

# Check replication status
kubectl exec -n o2ims-system redis-node-0 -- redis-cli INFO replication

# Check logs
kubectl logs -n o2ims-system redis-node-0 -c redis --tail=50
```

### Resolution

#### Automatic Failover (Sentinel Enabled)

```bash
# Verify Sentinel is triggering failover
kubectl logs -n o2ims-system -l app.kubernetes.io/component=sentinel --tail=50 | grep failover

# Monitor failover progress
watch kubectl exec -n o2ims-system redis-sentinel-0 -- \
  redis-cli -p 26379 SENTINEL get-master-addr-by-name mymaster

# Failover should complete in 30-60 seconds

# Verify new master elected
kubectl exec -n o2ims-system redis-sentinel-0 -- \
  redis-cli -p 26379 SENTINEL masters | grep -A 20 mymaster

# Gateway should auto-reconnect
kubectl logs -n o2ims-system -l app.kubernetes.io/name=netweave | grep "redis.*connected"
```

#### Manual Failover (If Automatic Fails)

```bash
# Identify healthy replica
kubectl get pods -n o2ims-system -l app.kubernetes.io/component=replica

# Force failover to specific replica
kubectl exec -n o2ims-system redis-sentinel-0 -- \
  redis-cli -p 26379 SENTINEL failover mymaster

# Monitor failover
kubectl exec -n o2ims-system redis-sentinel-0 -- \
  redis-cli -p 26379 SENTINEL get-master-addr-by-name mymaster

# Restart gateway pods if not auto-reconnecting
kubectl rollout restart deployment/netweave-gateway -n o2ims-system
```

#### Redis Pod Not Starting

```bash
# Check pod events
kubectl describe pod -n o2ims-system redis-node-0

# Check PVC status
kubectl get pvc -n o2ims-system | grep redis

# If PVC bound but pod failing, check data corruption
kubectl exec -n o2ims-system redis-node-0 -- redis-check-rdb /data/dump.rdb
kubectl exec -n o2ims-system redis-node-0 -- redis-check-aof /data/appendonly.aof

# If corrupted, restore from backup (see RB-010)
```

### Verification

```bash
# Redis primary elected
kubectl exec -n o2ims-system redis-sentinel-0 -- \
  redis-cli -p 26379 SENTINEL get-master-addr-by-name mymaster
# Expected: IP and port of new master

# Replicas connected
kubectl exec -n o2ims-system redis-node-0 -- redis-cli INFO replication
# Expected: connected_slaves:2

# Gateway connected
kubectl logs -n o2ims-system -l app.kubernetes.io/name=netweave | grep "redis.*connected"
# Expected: Recent log entries showing successful connection

# Subscriptions working
curl -k -X POST https://o2ims.example.com/o2ims-infrastructureInventory/v1/subscriptions \
  -H "Content-Type: application/json" \
  -d '{"callback":"https://test.com/notify","consumerSubscriptionId":"rb-002-test"}'
# Expected: HTTP 201 Created
```

### Post-Incident

- [ ] Investigate Redis crash cause
- [ ] Check Redis resource limits
- [ ] Review Redis logs for warnings
- [ ] Consider increasing Redis replicas
- [ ] Test failover procedures

---

## RB-003: High Error Rate

**Severity:** P2 - High
**Response Time:** < 1 hour
**Impact:** Degraded service quality

### Symptoms

- Error rate > 5%
- 5xx responses increasing
- Alert: `HighErrorRate`

### Investigation

```bash
# Check current error rate
kubectl exec -n o2ims-system netweave-gateway-0 -- \
  wget -qO- http://localhost:8080/metrics | \
  grep o2ims_http_requests_total | grep status=\"5

# Check error types in logs
kubectl logs -n o2ims-system -l app.kubernetes.io/name=netweave --tail=200 | \
  grep ERROR | cut -d' ' -f5- | sort | uniq -c | sort -rn

# Check backend health
kubectl logs -n o2ims-system -l app.kubernetes.io/name=netweave | \
  grep -i "backend.*error\|k8s.*error\|redis.*error"

# Check resource usage
kubectl top pods -n o2ims-system -l app.kubernetes.io/name=netweave
```

### Resolution

#### Scenario A: Backend API Errors

```bash
# Identify failing backend
kubectl logs -n o2ims-system -l app.kubernetes.io/name=netweave | \
  grep ERROR | grep -o "backend=[^ ]*" | sort | uniq -c

# Test backend connectivity
kubectl exec -n o2ims-system netweave-gateway-0 -- \
  wget --timeout=5 -O- https://kubernetes.default.svc.cluster.local/version

# If Kubernetes API slow/failing:
# Check API server health
kubectl get --raw /healthz

# If AWS/GCP API failing:
# Check cloud provider status page
# Implement retry with exponential backoff
# Consider circuit breaker pattern
```

#### Scenario B: Rate Limiting

```bash
# Check if rate limit hit
kubectl logs -n o2ims-system -l app.kubernetes.io/name=netweave | \
  grep -i "rate limit\|429\|too many requests"

# Identify high-volume clients
kubectl logs -n o2ims-system -l app.kubernetes.io/name=netweave | \
  grep "429" | cut -d' ' -f8 | sort | uniq -c | sort -rn

# Contact high-volume clients
# Or increase rate limits
kubectl edit configmap netweave-config -n o2ims-system
# Update:
# rate_limiting:
#   requests_per_second: 100  # Increase from 50
```

#### Scenario C: Resource Exhaustion

```bash
# Check CPU/memory usage
kubectl top pods -n o2ims-system -l app.kubernetes.io/name=netweave

# If approaching limits, scale up
kubectl scale deployment netweave-gateway -n o2ims-system --replicas=5

# Or increase limits
helm upgrade netweave ./helm/netweave \
  --namespace o2ims-system \
  --set resources.limits.cpu=2000m \
  --set resources.limits.memory=2Gi \
  --reuse-values
```

#### Scenario D: Bug in Recent Deployment

```bash
# Check deployment history
helm history netweave -n o2ims-system

# If errors started after recent deployment
# Rollback immediately
helm rollback netweave -n o2ims-system

# Monitor error rate after rollback
watch 'kubectl exec -n o2ims-system netweave-gateway-0 -- \
  wget -qO- http://localhost:8080/metrics | \
  grep o2ims_http_requests_total | grep status=\"5\"'
```

### Verification

```bash
# Error rate < 1%
kubectl exec -n o2ims-system netweave-gateway-0 -- \
  wget -qO- http://localhost:8080/metrics | \
  awk '/o2ims_http_requests_total.*status="[45][0-9][0-9]"/{errors+=$2} \
       /o2ims_http_requests_total/{total+=$2} \
       END{print "Error rate: " (errors/total)*100 "%"}'
# Expected: Error rate < 1%

# No errors in recent logs
kubectl logs -n o2ims-system -l app.kubernetes.io/name=netweave --since=5m | \
  grep ERROR | wc -l
# Expected: 0 or very low number

# API responding normally
for i in {1..10}; do
  curl -s -o /dev/null -w "%{http_code}\n" \
    -k https://o2ims.example.com/o2ims-infrastructureInventory/v1/api_versions
done
# Expected: All 200 responses
```

### Post-Incident

- [ ] Identify error root cause
- [ ] Create monitoring alert for specific error type
- [ ] Implement additional error handling
- [ ] Update error rate thresholds
- [ ] Create preventive measures

---

## RB-004: High Latency

**Severity:** P2 - High
**Response Time:** < 1 hour
**Impact:** Slow response times

### Symptoms

- p99 latency > 500ms
- p95 latency > 100ms
- Alert: `HighLatency`

### Investigation

```bash
# Check current latency
kubectl exec -n o2ims-system netweave-gateway-0 -- \
  wget -qO- http://localhost:8080/metrics | \
  grep o2ims_http_request_duration_seconds | grep quantile

# Check backend latency
kubectl exec -n o2ims-system netweave-gateway-0 -- \
  wget -qO- http://localhost:8080/metrics | \
  grep o2ims_adapter_backend_latency_seconds | grep quantile

# Check cache hit ratio
kubectl exec -n o2ims-system netweave-gateway-0 -- \
  wget -qO- http://localhost:8080/metrics | \
  awk '/o2ims_adapter_cache_hits_total/{hits=$2} \
       /o2ims_adapter_cache_misses_total/{misses=$2} \
       END{print "Cache hit ratio: " (hits/(hits+misses))*100 "%"}'

# Check CPU usage
kubectl top pods -n o2ims-system -l app.kubernetes.io/name=netweave
```

### Resolution

#### Scenario A: Backend API Slow

```bash
# Identify slow backends
kubectl exec -n o2ims-system netweave-gateway-0 -- \
  wget -qO- http://localhost:8080/metrics | \
  grep o2ims_adapter_backend_latency_seconds | grep quantile | sort -k2 -rn

# Test backend directly
time kubectl get nodes
time kubectl get deployments --all-namespaces

# If Kubernetes API slow:
# Check API server load
kubectl top pods -n kube-system -l component=kube-apiserver

# Contact platform team
# Consider implementing timeout and circuit breaker
kubectl edit configmap netweave-config -n o2ims-system
# Update:
# kubernetes:
#   timeout: 10s
#   circuit_breaker:
#     enabled: true
#     threshold: 5
```

#### Scenario B: Low Cache Hit Ratio

```bash
# Check current hit ratio
kubectl exec -n o2ims-system netweave-gateway-0 -- \
  wget -qO- http://localhost:8080/metrics | \
  awk '/o2ims_adapter_cache_hits_total/{hits+=$2} \
       /o2ims_adapter_cache_misses_total/{misses+=$2} \
       END{print "Hit ratio: " (hits/(hits+misses))*100 "%"}'

# If < 90%, increase cache TTL
kubectl edit configmap netweave-config -n o2ims-system
# Update:
# cache:
#   ttl:
#     resources: 60s         # Increase from 30s
#     resource_pools: 600s   # Increase from 300s

# Restart pods to apply
kubectl rollout restart deployment/netweave-gateway -n o2ims-system
```

#### Scenario C: CPU Throttling

```bash
# Check CPU throttling
for pod in $(kubectl get pods -n o2ims-system -l app.kubernetes.io/name=netweave -o name); do
  echo "$pod:"
  kubectl exec -n o2ims-system ${pod#pod/} -- \
    cat /sys/fs/cgroup/cpu/cpu.stat | grep throttled_time
done

# If throttled, increase CPU limits
helm upgrade netweave ./helm/netweave \
  --namespace o2ims-system \
  --set resources.limits.cpu=2000m \
  --reuse-values
```

#### Scenario D: High Request Volume

```bash
# Check request rate
kubectl exec -n o2ims-system netweave-gateway-0 -- \
  wget -qO- http://localhost:8080/metrics | \
  grep o2ims_http_requests_total

# If very high, scale horizontally
kubectl scale deployment netweave-gateway -n o2ims-system --replicas=5

# Or enable autoscaling
helm upgrade netweave ./helm/netweave \
  --namespace o2ims-system \
  --set autoscaling.enabled=true \
  --set autoscaling.minReplicas=3 \
  --set autoscaling.maxReplicas=10 \
  --reuse-values
```

### Verification

```bash
# p95 latency < 100ms
kubectl exec -n o2ims-system netweave-gateway-0 -- \
  wget -qO- http://localhost:8080/metrics | \
  grep 'o2ims_http_request_duration_seconds.*quantile="0.95"'
# Expected: < 0.1 (100ms)

# p99 latency < 500ms
kubectl exec -n o2ims-system netweave-gateway-0 -- \
  wget -qO- http://localhost:8080/metrics | \
  grep 'o2ims_http_request_duration_seconds.*quantile="0.99"'
# Expected: < 0.5 (500ms)

# Cache hit ratio > 90%
kubectl exec -n o2ims-system netweave-gateway-0 -- \
  wget -qO- http://localhost:8080/metrics | \
  awk '/o2ims_adapter_cache_hits_total/{hits+=$2} \
       /o2ims_adapter_cache_misses_total/{misses+=$2} \
       END{print "Hit ratio: " (hits/(hits+misses))*100 "%"}'
# Expected: > 90%
```

### Post-Incident

- [ ] Analyze latency trends
- [ ] Optimize slow queries
- [ ] Consider caching strategy
- [ ] Review backend SLAs
- [ ] Implement performance testing

---

## RB-005: Certificate Expiring

**Severity:** P3 - Medium
**Response Time:** < 4 hours
**Impact:** Service will fail when cert expires

### Symptoms

- Certificate expiring within 14 days
- Alert: `CertificateExpiringSoon`

### Investigation

```bash
# Check certificate expiry
kubectl get certificate -n o2ims-system netweave-tls -o yaml | grep notAfter

# Check cert-manager status
kubectl get certificaterequest -n o2ims-system
kubectl logs -n cert-manager -l app=cert-manager --tail=50

# Check issuer status
kubectl get clusterissuer ca-issuer -o yaml
```

### Resolution

#### Automatic Renewal (cert-manager)

```bash
# Trigger manual renewal
cmctl renew netweave-tls -n o2ims-system

# Monitor renewal
kubectl describe certificate netweave-tls -n o2ims-system

# Wait for renewal to complete
kubectl wait --for=condition=ready certificate/netweave-tls \
  -n o2ims-system --timeout=300s

# Verify new certificate
kubectl get secret netweave-tls -n o2ims-system \
  -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -enddate -noout
```

#### Manual Certificate Rotation

```bash
# Generate new certificate
openssl req -x509 -newkey rsa:4096 -keyout server.key -out server.crt \
  -days 365 -nodes \
  -subj "/CN=o2ims.example.com"

# Update secret
kubectl create secret tls netweave-tls \
  --cert=server.crt \
  --key=server.key \
  -n o2ims-system \
  --dry-run=client -o yaml | kubectl apply -f -

# Restart pods to pick up new cert
kubectl rollout restart deployment/netweave-gateway -n o2ims-system
```

### Verification

```bash
# Certificate valid for > 30 days
kubectl get secret netweave-tls -n o2ims-system \
  -o jsonpath='{.data.tls\.crt}' | base64 -d | \
  openssl x509 -enddate -noout | cut -d= -f2 | \
  xargs -I {} date -d {} +%s | \
  awk -v now=$(date +%s) '{print "Days remaining: " ($1-now)/86400}'
# Expected: > 30 days

# cert-manager monitoring
kubectl get certificate netweave-tls -n o2ims-system
# Expected: READY True

# Service still working with new cert
curl -vk https://o2ims.example.com/healthz 2>&1 | grep "expire date"
# Expected: Future date > 30 days
```

### Post-Incident

- [ ] Verify auto-renewal working
- [ ] Update renewal thresholds
- [ ] Document certificate process
- [ ] Schedule regular cert audits

---

## RB-006: Pod CrashLoopBackOff

**Severity:** P2 - High
**Response Time:** < 1 hour
**Impact:** Reduced capacity

### Symptoms

- Pod repeatedly crashing
- Status: CrashLoopBackOff
- Alert: `PodCrashLoop`

### Investigation

```bash
# Check pod status and restart count
kubectl get pods -n o2ims-system -l app.kubernetes.io/name=netweave \
  -o custom-columns=NAME:.metadata.name,RESTARTS:.status.containerStatuses[0].restartCount,STATUS:.status.phase

# Check logs from current and previous containers
kubectl logs -n o2ims-system <pod-name> --tail=100
kubectl logs -n o2ims-system <pod-name> --previous --tail=100

# Check for OOM kills
kubectl describe pod -n o2ims-system <pod-name> | grep -i oom

# Check pod events
kubectl describe pod -n o2ims-system <pod-name> | grep -A 10 Events
```

### Resolution

See detailed resolution steps in [RB-001](#rb-001-all-pods-down) and [Troubleshooting Guide](troubleshooting.md#pod-crashes-and-restarts).

---

## RB-007: Webhook Delivery Failures

**Severity:** P2 - High
**Response Time:** < 1 hour
**Impact:** SMO not receiving notifications

### Investigation

```bash
# Check webhook metrics
kubectl exec -n o2ims-system netweave-gateway-0 -- \
  wget -qO- http://localhost:8080/metrics | \
  grep o2ims_webhook_delivery

# Check webhook errors in logs
kubectl logs -n o2ims-system -l app.kubernetes.io/name=netweave | \
  grep -i webhook | grep -i error

# List affected subscriptions
kubectl exec -n o2ims-system redis-node-0 -- \
  redis-cli KEYS "subscription:*" | \
  while read key; do
    kubectl exec -n o2ims-system redis-node-0 -- redis-cli GET "$key"
  done | jq -r '.callback' | sort | uniq -c
```

### Resolution

```bash
# Test SMO endpoint
for callback in $(kubectl exec -n o2ims-system redis-node-0 -- \
  redis-cli KEYS "subscription:*" | \
  while read key; do
    kubectl exec -n o2ims-system redis-node-0 -- redis-cli GET "$key"
  done | jq -r '.callback' | sort -u); do
  echo "Testing: $callback"
  curl -k -X POST "$callback" \
    -H "Content-Type: application/json" \
    -d '{"test": true}' \
    -w "HTTP %{http_code}\n" \
    -o /dev/null
done

# If SMO unreachable, contact SMO team
# If network policy blocking, update policy
kubectl apply -f - <<EOF
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-webhook-egress
  namespace: o2ims-system
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/name: netweave
  policyTypes:
    - Egress
  egress:
    - ports:
        - protocol: TCP
          port: 443
EOF
```

---

## RB-008: Cache Hit Ratio Low

**Severity:** P3 - Medium
**Response Time:** < 4 hours
**Impact:** Increased backend load, higher latency

### Resolution

See detailed steps in [RB-004 Scenario B](#scenario-b-low-cache-hit-ratio).

---

## RB-009: Kubernetes API Errors

**Severity:** P2 - High
**Response Time:** < 1 hour
**Impact:** Resource data unavailable

### Resolution

See detailed steps in [Troubleshooting Guide](troubleshooting.md#kubernetes-api-errors).

---

## RB-010: Subscription Data Loss

**Severity:** P1 - Critical
**Response Time:** < 15 minutes
**Impact:** All subscriptions lost

### Resolution

See detailed steps in [Backup and Recovery Guide](backup-recovery.md#scenario-5-restore-from-backup-after-data-loss).

---

## On-Call Playbook

### Pre-Shift Checklist

- [ ] Verify access to all systems
- [ ] Review current system health
- [ ] Check for scheduled maintenance
- [ ] Review recent incidents
- [ ] Test alerting channels
- [ ] Bookmark key dashboards

### During Shift

- [ ] Acknowledge alerts within 5 minutes
- [ ] Follow runbooks systematically
- [ ] Document all actions taken
- [ ] Escalate if needed
- [ ] Communicate status updates

### Post-Shift Handoff

- [ ] Document ongoing incidents
- [ ] Share lessons learned
- [ ] Update runbooks if needed
- [ ] Hand off to next engineer

## Escalation Path

| Level | Role | Contact | Response Time |
|-------|------|---------|---------------|
| **L1** | On-call Engineer | PagerDuty | 15 min |
| **L2** | Platform Lead | Slack + Phone | 30 min |
| **L3** | Engineering Manager | Phone | 1 hour |
| **L4** | VP Engineering | Phone | 2 hours |

## Related Documentation

- [Operations Overview](README.md)
- [Monitoring Guide](monitoring.md)
- [Troubleshooting Guide](troubleshooting.md)
- [Backup and Recovery](backup-recovery.md)
