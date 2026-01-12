# Troubleshooting Guide

Comprehensive guide for diagnosing and resolving common issues with the netweave O2-IMS Gateway.

## Quick Diagnostic Commands

### Gateway Status

```bash
# Check pod status
kubectl get pods -n o2ims-system -l app.kubernetes.io/name=netweave

# Check pod logs (last 50 lines)
kubectl logs -n o2ims-system -l app.kubernetes.io/name=netweave --tail=50

# Stream logs in real-time
kubectl logs -n o2ims-system -l app.kubernetes.io/name=netweave -f

# Check pod events
kubectl describe pod -n o2ims-system <pod-name>

# Check resource usage
kubectl top pods -n o2ims-system -l app.kubernetes.io/name=netweave
```

### Redis Status

```bash
# Check Redis connectivity
kubectl exec -n o2ims-system redis-node-0 -- redis-cli PING

# Check replication status
kubectl exec -n o2ims-system redis-node-0 -- redis-cli INFO replication

# Check Sentinel status
kubectl exec -n o2ims-system redis-sentinel-0 -- \
  redis-cli -p 26379 SENTINEL master mymaster

# Check Redis logs
kubectl logs -n o2ims-system redis-node-0 -c redis
```

### API Health

```bash
# Health check endpoint
kubectl port-forward -n o2ims-system svc/netweave-gateway 8080:8080 &
curl -k https://localhost:8080/healthz

# Readiness check
curl -k https://localhost:8080/readyz

# Metrics endpoint
curl http://localhost:8080/metrics | grep o2ims_
```

## Common Issues

### Pod Crashes and Restarts

#### Symptom: Pods Constantly Restarting

**Check restart count:**
```bash
kubectl get pods -n o2ims-system -l app.kubernetes.io/name=netweave \
  -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.status.containerStatuses[0].restartCount}{"\n"}{end}'
```

**Diagnosis:**

1. **Check pod status:**
   ```bash
   kubectl describe pod -n o2ims-system <pod-name>
   ```

2. **Check logs from previous container:**
   ```bash
   kubectl logs -n o2ims-system <pod-name> --previous
   ```

3. **Look for common error patterns:**
   - `panic:` - Go panic, check stack trace
   - `fatal error:` - Fatal error, check error message
   - `OOMKilled` - Out of memory, increase memory limits
   - `Segmentation fault` - Memory corruption, check for nil pointer dereferences

**Common Causes:**

**Cause 1: Out of Memory (OOMKilled)**

```bash
# Check memory usage
kubectl top pod -n o2ims-system <pod-name>

# Check OOM events
kubectl get events -n o2ims-system --field-selector involvedObject.name=<pod-name> \
  | grep OOMKilled
```

**Resolution:**
```bash
# Increase memory limits
helm upgrade netweave ./helm/netweave \
  --namespace o2ims-system \
  --set resources.limits.memory=2Gi \
  --reuse-values
```

**Cause 2: Liveness Probe Failure**

```bash
# Check liveness probe config
kubectl get pod -n o2ims-system <pod-name> -o yaml | grep -A 5 livenessProbe

# Test liveness endpoint manually
kubectl exec -n o2ims-system <pod-name> -- \
  wget -qO- http://localhost:8080/healthz
```

**Resolution:**
```yaml
# Adjust probe timings in values.yaml
livenessProbe:
  httpGet:
    path: /healthz
    port: 8080
    scheme: HTTP
  initialDelaySeconds: 30  # Increase if slow startup
  periodSeconds: 10
  timeoutSeconds: 5
  failureThreshold: 3      # Increase for transient failures
```

**Cause 3: Panic in Application Code**

```bash
# Check logs for panic
kubectl logs -n o2ims-system <pod-name> | grep "panic:"
```

**Resolution:**
- Check stack trace in logs
- Verify Go version compatibility
- Check for nil pointer dereferences
- Review recent code changes

**Cause 4: Redis Connection Failure on Startup**

```bash
# Check Redis connectivity
kubectl exec -n o2ims-system <pod-name> -- \
  redis-cli -h redis-node-0 PING

# Check network policies
kubectl get networkpolicies -n o2ims-system
```

**Resolution:**
```bash
# Verify Redis is running
kubectl get pods -n o2ims-system -l app.kubernetes.io/name=redis

# Check Redis password secret
kubectl get secret redis-password -n o2ims-system -o jsonpath='{.data.password}' | base64 -d

# Test connectivity
kubectl run -n o2ims-system redis-test --rm -it --image=redis:7.4-alpine -- \
  redis-cli -h redis-node-0 -a <password> PING
```

#### Symptom: CrashLoopBackOff

**Check crash loop timing:**
```bash
kubectl get pod -n o2ims-system <pod-name> \
  -o jsonpath='{.status.containerStatuses[0].state.waiting.message}'
```

**Diagnosis:**
```bash
# Check logs from all restarts
kubectl logs -n o2ims-system <pod-name> --previous --timestamps

# Check pod events
kubectl describe pod -n o2ims-system <pod-name> | grep -A 10 Events
```

**Common Causes:**
1. **Configuration error** - Check ConfigMap and Secret mounts
2. **Missing dependency** - Check Redis, Kubernetes API connectivity
3. **Port conflict** - Check if port 8080 is already in use
4. **Certificate error** - Check TLS configuration

**Resolution:**
```bash
# Verify ConfigMap
kubectl get configmap netweave-config -n o2ims-system -o yaml

# Verify Secrets
kubectl get secrets -n o2ims-system | grep netweave

# Check mounted volumes
kubectl describe pod -n o2ims-system <pod-name> | grep -A 5 Mounts

# Test configuration manually
kubectl run -n o2ims-system debug --rm -it --image=busybox -- sh
# Inside pod: wget -O- http://netweave-gateway:8080/healthz
```

### Redis Connection Issues

#### Symptom: "Connection Refused" Errors

**Check logs:**
```bash
kubectl logs -n o2ims-system -l app.kubernetes.io/name=netweave | grep "connection refused"
```

**Diagnosis:**

1. **Verify Redis is running:**
   ```bash
   kubectl get pods -n o2ims-system -l app.kubernetes.io/name=redis
   kubectl logs -n o2ims-system redis-node-0 -c redis --tail=50
   ```

2. **Test Redis connectivity:**
   ```bash
   kubectl exec -n o2ims-system <gateway-pod> -- \
     redis-cli -h redis-node-0 PING
   ```

3. **Check Redis Service:**
   ```bash
   kubectl get svc -n o2ims-system redis-headless
   kubectl describe svc -n o2ims-system redis-headless
   ```

**Resolution:**

**If Redis pods not running:**
```bash
# Check Redis deployment
kubectl get statefulset -n o2ims-system redis-node

# Scale up if scaled down
kubectl scale statefulset redis-node -n o2ims-system --replicas=3

# Check PVC status
kubectl get pvc -n o2ims-system
```

**If Redis running but unreachable:**
```bash
# Check network policies
kubectl get networkpolicy -n o2ims-system

# Create network policy to allow gateway -> Redis
cat <<EOF | kubectl apply -f -
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-gateway-to-redis
  namespace: o2ims-system
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/name: redis
  ingress:
    - from:
        - podSelector:
            matchLabels:
              app.kubernetes.io/name: netweave
      ports:
        - protocol: TCP
          port: 6379
        - protocol: TCP
          port: 26379
EOF
```

#### Symptom: Redis Sentinel Not Responding

**Check Sentinel status:**
```bash
kubectl exec -n o2ims-system redis-sentinel-0 -- \
  redis-cli -p 26379 INFO sentinel

kubectl exec -n o2ims-system redis-sentinel-0 -- \
  redis-cli -p 26379 SENTINEL masters
```

**Diagnosis:**

1. **Check Sentinel configuration:**
   ```bash
   kubectl exec -n o2ims-system redis-sentinel-0 -- \
     redis-cli -p 26379 SENTINEL get-master-addr-by-name mymaster
   ```

2. **Check quorum:**
   ```bash
   kubectl exec -n o2ims-system redis-sentinel-0 -- \
     redis-cli -p 26379 SENTINEL ckquorum mymaster
   ```

**Resolution:**

**If master not elected:**
```bash
# Force failover
kubectl exec -n o2ims-system redis-sentinel-0 -- \
  redis-cli -p 26379 SENTINEL failover mymaster

# Verify new master
kubectl exec -n o2ims-system redis-sentinel-0 -- \
  redis-cli -p 26379 SENTINEL get-master-addr-by-name mymaster
```

**If quorum lost:**
```bash
# Check Sentinel replicas
kubectl get pods -n o2ims-system -l app.kubernetes.io/component=sentinel

# Scale Sentinels if needed (must be 3 or 5)
kubectl scale statefulset redis-sentinel -n o2ims-system --replicas=3

# Wait for Sentinels to sync
sleep 60

# Verify quorum
kubectl exec -n o2ims-system redis-sentinel-0 -- \
  redis-cli -p 26379 SENTINEL ckquorum mymaster
```

### Kubernetes API Errors

#### Symptom: "Unauthorized" or "Forbidden" Errors

**Check logs:**
```bash
kubectl logs -n o2ims-system -l app.kubernetes.io/name=netweave | \
  grep -i "unauthorized\|forbidden"
```

**Diagnosis:**

1. **Check ServiceAccount:**
   ```bash
   kubectl get sa -n o2ims-system netweave-gateway
   kubectl describe sa -n o2ims-system netweave-gateway
   ```

2. **Check RBAC:**
   ```bash
   kubectl get clusterrole netweave-gateway
   kubectl get clusterrolebinding netweave-gateway
   kubectl describe clusterrole netweave-gateway
   ```

3. **Test permissions:**
   ```bash
   kubectl auth can-i get deployments --as=system:serviceaccount:o2ims-system:netweave-gateway
   kubectl auth can-i list nodes --as=system:serviceaccount:o2ims-system:netweave-gateway
   ```

**Resolution:**

```bash
# Apply correct RBAC
cat <<EOF | kubectl apply -f -
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: netweave-gateway
rules:
  - apiGroups: [""]
    resources: ["nodes", "pods", "services", "namespaces"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["apps"]
    resources: ["deployments", "statefulsets", "daemonsets"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["batch"]
    resources: ["jobs", "cronjobs"]
    verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: netweave-gateway
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: netweave-gateway
subjects:
  - kind: ServiceAccount
    name: netweave-gateway
    namespace: o2ims-system
EOF

# Restart pods to pick up new permissions
kubectl rollout restart deployment/netweave-gateway -n o2ims-system
```

#### Symptom: Kubernetes API Timeout

**Check logs:**
```bash
kubectl logs -n o2ims-system -l app.kubernetes.io/name=netweave | grep "context deadline exceeded"
```

**Diagnosis:**

1. **Check API server health:**
   ```bash
   kubectl get --raw /healthz
   kubectl get componentstatuses
   ```

2. **Check network latency:**
   ```bash
   kubectl exec -n o2ims-system <gateway-pod> -- \
     wget -qO- --timeout=5 https://kubernetes.default.svc.cluster.local/healthz
   ```

**Resolution:**

```bash
# Increase timeout in gateway configuration
kubectl edit configmap netweave-config -n o2ims-system

# Add/update:
# kubernetes:
#   timeout: 30s
#   qps: 50
#   burst: 100

# Restart gateway
kubectl rollout restart deployment/netweave-gateway -n o2ims-system
```

### Performance Issues

#### Symptom: High API Latency (p99 > 500ms)

**Check metrics:**
```bash
kubectl exec -n o2ims-system <gateway-pod> -- \
  wget -qO- http://localhost:8080/metrics | \
  grep o2ims_http_request_duration_seconds
```

**Diagnosis:**

1. **Check backend latency:**
   ```bash
   kubectl exec -n o2ims-system <gateway-pod> -- \
     wget -qO- http://localhost:8080/metrics | \
     grep o2ims_adapter_backend_latency_seconds
   ```

2. **Check cache hit ratio:**
   ```bash
   kubectl exec -n o2ims-system <gateway-pod> -- \
     wget -qO- http://localhost:8080/metrics | \
     grep o2ims_adapter_cache
   ```

3. **Check resource usage:**
   ```bash
   kubectl top pod -n o2ims-system -l app.kubernetes.io/name=netweave
   ```

**Common Causes:**

**Cause 1: Low Cache Hit Ratio**

```bash
# Check current cache hit ratio
kubectl exec -n o2ims-system <gateway-pod> -- \
  wget -qO- http://localhost:8080/metrics | \
  awk '/o2ims_adapter_cache_hits_total/ || /o2ims_adapter_cache_misses_total/'
```

**Resolution:**
```bash
# Increase cache TTL
kubectl edit configmap netweave-config -n o2ims-system

# Update:
# cache:
#   ttl:
#     resources: 60s        # Increase from 30s
#     resource_pools: 600s  # Increase from 300s

# Restart pods
kubectl rollout restart deployment/netweave-gateway -n o2ims-system
```

**Cause 2: Backend API Slowness**

```bash
# Check backend latency
kubectl exec -n o2ims-system <gateway-pod> -- \
  wget -qO- http://localhost:8080/metrics | \
  grep backend_latency | grep quantile
```

**Resolution:**
- Contact backend provider (K8s, AWS, GCP)
- Implement pagination for large result sets
- Scale backend infrastructure
- Add backend-specific caching

**Cause 3: CPU Throttling**

```bash
# Check CPU usage
kubectl top pod -n o2ims-system -l app.kubernetes.io/name=netweave

# Check CPU throttling
kubectl exec -n o2ims-system <gateway-pod> -- \
  cat /sys/fs/cgroup/cpu/cpu.stat | grep throttled
```

**Resolution:**
```bash
# Increase CPU limits
helm upgrade netweave ./helm/netweave \
  --namespace o2ims-system \
  --set resources.limits.cpu=2000m \
  --reuse-values
```

#### Symptom: High Memory Usage

**Check memory:**
```bash
kubectl top pod -n o2ims-system -l app.kubernetes.io/name=netweave

kubectl exec -n o2ims-system <gateway-pod> -- \
  wget -qO- http://localhost:8080/debug/pprof/heap > heap.out
```

**Diagnosis:**

1. **Check for memory leaks:**
   ```bash
   # Compare memory over time
   for i in {1..10}; do
     kubectl top pod -n o2ims-system <gateway-pod>
     sleep 60
   done
   ```

2. **Check goroutine count:**
   ```bash
   kubectl exec -n o2ims-system <gateway-pod> -- \
     wget -qO- http://localhost:8080/debug/pprof/goroutine?debug=1 | grep goroutine
   ```

**Resolution:**

**If memory leak suspected:**
```bash
# Enable pprof for detailed profiling
kubectl port-forward -n o2ims-system <gateway-pod> 6060:6060 &
go tool pprof http://localhost:6060/debug/pprof/heap

# Analyze in pprof:
# (pprof) top10
# (pprof) list <function-name>
```

**If just high usage:**
```bash
# Increase memory limits
helm upgrade netweave ./helm/netweave \
  --namespace o2ims-system \
  --set resources.limits.memory=2Gi \
  --reuse-values

# Reduce cache size
kubectl edit configmap netweave-config -n o2ims-system
# Update cache max_entries
```

### Certificate Problems

#### Symptom: TLS Handshake Failures

**Check logs:**
```bash
kubectl logs -n o2ims-system -l app.kubernetes.io/name=netweave | \
  grep -i "tls\|certificate\|handshake"
```

**Diagnosis:**

1. **Check certificate status:**
   ```bash
   kubectl get certificate -n o2ims-system
   kubectl describe certificate netweave-tls -n o2ims-system
   ```

2. **Check certificate contents:**
   ```bash
   kubectl get secret netweave-tls -n o2ims-system -o jsonpath='{.data.tls\.crt}' | \
     base64 -d | openssl x509 -text -noout
   ```

3. **Check certificate expiry:**
   ```bash
   kubectl get secret netweave-tls -n o2ims-system -o jsonpath='{.data.tls\.crt}' | \
     base64 -d | openssl x509 -enddate -noout
   ```

**Resolution:**

**If certificate expired:**
```bash
# Force renewal
kubectl delete certificate netweave-tls -n o2ims-system
kubectl apply -f - <<EOF
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: netweave-tls
  namespace: o2ims-system
spec:
  secretName: netweave-tls
  issuerRef:
    name: ca-issuer
    kind: ClusterIssuer
  dnsNames:
    - netweave-gateway.o2ims-system.svc.cluster.local
    - o2ims.example.com
  duration: 2160h  # 90 days
  renewBefore: 720h  # 30 days
EOF

# Wait for certificate to be ready
kubectl wait --for=condition=ready certificate/netweave-tls -n o2ims-system --timeout=300s

# Restart gateway pods
kubectl rollout restart deployment/netweave-gateway -n o2ims-system
```

**If cert-manager issues:**
```bash
# Check cert-manager logs
kubectl logs -n cert-manager -l app=cert-manager

# Check certificate request
kubectl get certificaterequest -n o2ims-system

# Check issuer
kubectl get clusterissuer ca-issuer -o yaml
```

#### Symptom: Client Certificate Validation Failures

**Check logs:**
```bash
kubectl logs -n o2ims-system -l app.kubernetes.io/name=netweave | \
  grep "client certificate"
```

**Diagnosis:**

1. **Check CA bundle:**
   ```bash
   kubectl get secret ca-bundle -n o2ims-system -o jsonpath='{.data.ca\.crt}' | \
     base64 -d | openssl x509 -text -noout
   ```

2. **Test client certificate:**
   ```bash
   openssl verify -CAfile ca.crt client.crt
   ```

**Resolution:**

```bash
# Update CA bundle
kubectl create secret generic ca-bundle \
  --namespace o2ims-system \
  --from-file=ca.crt=updated-ca-bundle.crt \
  --dry-run=client -o yaml | kubectl apply -f -

# Restart gateway
kubectl rollout restart deployment/netweave-gateway -n o2ims-system
```

### Network Issues

#### Symptom: Webhook Delivery Failures

**Check logs:**
```bash
kubectl logs -n o2ims-system -l app.kubernetes.io/name=netweave | \
  grep "webhook\|notification"
```

**Check metrics:**
```bash
kubectl exec -n o2ims-system <gateway-pod> -- \
  wget -qO- http://localhost:8080/metrics | \
  grep o2ims_webhook_delivery
```

**Diagnosis:**

1. **Test webhook endpoint:**
   ```bash
   kubectl exec -n o2ims-system <gateway-pod> -- \
     wget --timeout=5 -O- https://smo.example.com/notify
   ```

2. **Check network policies:**
   ```bash
   kubectl get networkpolicy -n o2ims-system
   kubectl describe networkpolicy -n o2ims-system
   ```

**Resolution:**

**If network policy blocking:**
```bash
cat <<EOF | kubectl apply -f -
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-gateway-egress
  namespace: o2ims-system
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/name: netweave
  policyTypes:
    - Egress
  egress:
    - to:
        - namespaceSelector: {}
      ports:
        - protocol: TCP
          port: 443
    - to:
        - podSelector:
            matchLabels:
              app.kubernetes.io/name: redis
      ports:
        - protocol: TCP
          port: 6379
        - protocol: TCP
          port: 26379
    - to:  # DNS
        - namespaceSelector:
            matchLabels:
              name: kube-system
        - podSelector:
            matchLabels:
              k8s-app: kube-dns
      ports:
        - protocol: UDP
          port: 53
EOF
```

**If SMO unreachable:**
- Verify SMO endpoint DNS resolution
- Check firewall rules
- Verify SMO is accepting connections
- Check webhook retry configuration

#### Symptom: Ingress Not Working

**Check Ingress:**
```bash
kubectl get ingress -n o2ims-system
kubectl describe ingress netweave-gateway -n o2ims-system
```

**Diagnosis:**

1. **Check Ingress Controller:**
   ```bash
   kubectl get pods -n ingress-nginx
   kubectl logs -n ingress-nginx -l app.kubernetes.io/component=controller
   ```

2. **Test Service directly:**
   ```bash
   kubectl port-forward -n o2ims-system svc/netweave-gateway 8080:8080 &
   curl -k https://localhost:8080/healthz
   ```

**Resolution:**

```bash
# Verify Ingress configuration
kubectl get ingress netweave-gateway -n o2ims-system -o yaml

# Check for proper annotations
# nginx.ingress.kubernetes.io/ssl-redirect: "true"
# nginx.ingress.kubernetes.io/backend-protocol: "HTTPS"

# Update if needed
kubectl annotate ingress netweave-gateway -n o2ims-system \
  nginx.ingress.kubernetes.io/backend-protocol=HTTPS --overwrite
```

## Debugging Tools

### Enable Debug Logging

```bash
kubectl edit configmap netweave-config -n o2ims-system

# Add/update:
# logging:
#   level: debug
#   format: json

kubectl rollout restart deployment/netweave-gateway -n o2ims-system
```

### Use Debug Pod

```bash
# Run debug pod with network tools
kubectl run -n o2ims-system debug --rm -it \
  --image=nicolaka/netshoot -- /bin/bash

# Inside debug pod:
# Test DNS: nslookup netweave-gateway
# Test connectivity: curl -k https://netweave-gateway:8080/healthz
# Test Redis: redis-cli -h redis-node-0 PING
```

### Port Forwarding for Local Testing

```bash
# Forward gateway port
kubectl port-forward -n o2ims-system svc/netweave-gateway 8080:8080 &

# Forward metrics port
kubectl port-forward -n o2ims-system svc/netweave-gateway 8081:8080 &

# Forward Redis port
kubectl port-forward -n o2ims-system svc/redis-headless 6379:6379 &

# Test locally
curl -k https://localhost:8080/o2ims-infrastructureInventory/v1/api_versions
curl http://localhost:8081/metrics
redis-cli -h localhost PING
```

### Packet Capture

```bash
# Capture traffic on gateway pod
kubectl exec -n o2ims-system <gateway-pod> -- \
  tcpdump -i any -w /tmp/capture.pcap port 8080

# Copy capture file
kubectl cp o2ims-system/<gateway-pod>:/tmp/capture.pcap ./capture.pcap

# Analyze with Wireshark
wireshark capture.pcap
```

## Getting Help

### Log Collection for Support

```bash
# Collect all relevant logs
mkdir -p debug-logs
kubectl logs -n o2ims-system -l app.kubernetes.io/name=netweave > debug-logs/gateway-logs.txt
kubectl logs -n o2ims-system redis-node-0 > debug-logs/redis-logs.txt
kubectl get all -n o2ims-system -o yaml > debug-logs/resources.yaml
kubectl describe pods -n o2ims-system > debug-logs/pod-descriptions.txt
kubectl get events -n o2ims-system --sort-by='.lastTimestamp' > debug-logs/events.txt

# Create tarball
tar czf debug-logs-$(date +%Y%m%d-%H%M%S).tar.gz debug-logs/
```

### Support Channels

- **GitHub Issues**: [https://github.com/piwi3910/netweave/issues](https://github.com/piwi3910/netweave/issues)
- **Internal Support**: Contact on-call engineer
- **Escalation**: See [Operations README](README.md) for escalation path

## Related Documentation

- [Operations Overview](README.md)
- [Deployment Guide](deployment.md)
- [Monitoring Guide](monitoring.md)
- [Runbooks](runbooks.md)
