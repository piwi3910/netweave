# Backup and Recovery Guide

Comprehensive backup strategies and disaster recovery procedures for the netweave O2-IMS Gateway.

## Overview

The gateway's state is stored entirely in Redis. A robust backup and recovery strategy ensures business continuity and data protection.

### RTO/RPO Targets

| Environment | RTO (Recovery Time) | RPO (Data Loss) | Backup Frequency |
|-------------|---------------------|-----------------|------------------|
| **Development** | 4 hours | 24 hours | Daily |
| **Staging** | 1 hour | 4 hours | Every 6 hours |
| **Production** | 15 minutes | 5 minutes | Continuous (AOF) + Hourly snapshots |

**Definitions:**
- **RTO**: Maximum acceptable downtime before service must be restored
- **RPO**: Maximum acceptable data loss measured in time
- **AOF**: Append-Only File - Redis persistence mechanism for write operations

## What Needs Backup

### Redis Data

**Subscription state:**
- Active subscriptions (IDs, callbacks, filters)
- Subscription metadata (creation time, expiry)
- Subscription-to-resource mappings

**Cache data:**
- Resource cache (not critical, can be rebuilt)
- Resource pool cache (not critical)
- Adapter metadata cache (not critical)

**Priority:**
1. **Critical**: Subscription state (must backup)
2. **Non-critical**: Cache data (can be rebuilt on restart)

### Configuration

**Kubernetes manifests:**
- Deployments, Services, ConfigMaps
- Secrets (TLS certificates, Redis passwords)
- RBAC configuration

**Helm releases:**
- Helm values files
- Custom resource definitions

## Backup Strategies

### Strategy 1: Redis RDB Snapshots

**Purpose**: Point-in-time snapshot backups
**RPO**: Based on snapshot frequency (1-6 hours)
**Storage**: S3, GCS, Azure Blob, or PVC

#### Configuration

```yaml
# Redis ConfigMap
apiVersion: v1
kind: ConfigMap
metadata:
  name: redis-config
  namespace: o2ims-system
data:
  redis.conf: |
    # RDB Snapshots
    save 900 1      # After 900 sec (15 min) if at least 1 key changed
    save 300 10     # After 300 sec (5 min) if at least 10 keys changed
    save 60 10000   # After 60 sec if at least 10000 keys changed

    stop-writes-on-bgsave-error yes
    rdbcompression yes
    rdbchecksum yes
    dbfilename dump.rdb
    dir /data
```

#### Manual Snapshot

```bash
# Trigger manual backup
kubectl exec -n o2ims-system redis-node-0 -- redis-cli BGSAVE

# Check backup status
kubectl exec -n o2ims-system redis-node-0 -- redis-cli LASTSAVE

# Copy snapshot file
kubectl exec -n o2ims-system redis-node-0 -- tar czf - /data/dump.rdb | \
  cat > backup-$(date +%Y%m%d-%H%M%S).rdb.tar.gz
```

#### Automated Snapshot Backup

**CronJob for automated backups:**

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: redis-backup
  namespace: o2ims-system
spec:
  schedule: "0 */6 * * *"  # Every 6 hours
  concurrencyPolicy: Forbid
  successfulJobsHistoryLimit: 3
  failedJobsHistoryLimit: 1
  jobTemplate:
    spec:
      template:
        spec:
          serviceAccountName: redis-backup
          containers:
          - name: backup
            image: redis:7.4-alpine
            env:
            - name: AWS_ACCESS_KEY_ID
              valueFrom:
                secretKeyRef:
                  name: s3-credentials
                  key: access-key-id
            - name: AWS_SECRET_ACCESS_KEY
              valueFrom:
                secretKeyRef:
                  name: s3-credentials
                  key: secret-access-key
            - name: S3_BUCKET
              value: "netweave-backups"
            - name: BACKUP_PREFIX
              value: "redis-snapshots"
            command:
            - /bin/sh
            - -c
            - |
              set -e

              # Trigger Redis backup
              redis-cli -h redis-node-0 BGSAVE

              # Wait for backup to complete
              while [ "$(redis-cli -h redis-node-0 INFO persistence | grep rdb_bgsave_in_progress | cut -d: -f2 | tr -d '\r')" = "1" ]; do
                echo "Waiting for backup to complete..."
                sleep 5
              done

              # Copy dump.rdb from Redis pod
              kubectl cp o2ims-system/redis-node-0:/data/dump.rdb /tmp/dump.rdb

              # Create timestamped backup
              TIMESTAMP=$(date +%Y%m%d-%H%M%S)
              BACKUP_FILE="dump-${TIMESTAMP}.rdb"
              cp /tmp/dump.rdb /tmp/${BACKUP_FILE}
              gzip /tmp/${BACKUP_FILE}

              # Upload to S3
              aws s3 cp /tmp/${BACKUP_FILE}.gz \
                s3://${S3_BUCKET}/${BACKUP_PREFIX}/${BACKUP_FILE}.gz \
                --storage-class STANDARD_IA

              echo "Backup completed: ${BACKUP_FILE}.gz"

              # Cleanup old backups (keep last 30 days)
              aws s3 ls s3://${S3_BUCKET}/${BACKUP_PREFIX}/ | \
                awk '{print $4}' | \
                sort -r | \
                tail -n +720 | \
                xargs -I {} aws s3 rm s3://${S3_BUCKET}/${BACKUP_PREFIX}/{}
          restartPolicy: OnFailure
```

**Create ServiceAccount and RBAC:**

```bash
kubectl create serviceaccount redis-backup -n o2ims-system

kubectl create role redis-backup-role -n o2ims-system \
  --verb=get,list \
  --resource=pods,pods/exec

kubectl create rolebinding redis-backup-binding -n o2ims-system \
  --role=redis-backup-role \
  --serviceaccount=o2ims-system:redis-backup
```

### Strategy 2: Redis AOF (Append-Only File)

**Purpose**: Continuous write logging for minimal data loss
**RPO**: 1 second (with `appendfsync everysec`)
**Storage**: PVC with persistent storage

#### Configuration

```yaml
# Redis ConfigMap
apiVersion: v1
kind: ConfigMap
metadata:
  name: redis-config
  namespace: o2ims-system
data:
  redis.conf: |
    # AOF Configuration
    appendonly yes
    appendfilename "appendonly.aof"
    appendfsync everysec  # fsync every second

    # AOF rewrite configuration
    auto-aof-rewrite-percentage 100
    auto-aof-rewrite-min-size 64mb

    aof-load-truncated yes
    aof-use-rdb-preamble yes
```

#### AOF Backup

```bash
# Trigger AOF rewrite (compaction)
kubectl exec -n o2ims-system redis-node-0 -- redis-cli BGREWRITEAOF

# Check rewrite status
kubectl exec -n o2ims-system redis-node-0 -- redis-cli INFO persistence | grep aof_rewrite_in_progress

# Copy AOF file
kubectl exec -n o2ims-system redis-node-0 -- tar czf - /data/appendonly.aof | \
  cat > backup-aof-$(date +%Y%m%d-%H%M%S).tar.gz
```

### Strategy 3: Redis Sentinel Replication

**Purpose**: Real-time replication for HA and zero-RPO DR
**RPO**: Near-zero (replication lag typically < 1 second)
**Storage**: Replicated across multiple Redis instances

#### Multi-Cluster Replication Setup

```mermaid
graph LR
    subgraph "Primary Cluster (US-East)"
        RM[Redis Master]
        RR1[Redis Replica 1]
        RR2[Redis Replica 2]
    end

    subgraph "DR Cluster (US-West)"
        RDR[Redis DR Replica]
    end

    RM -->|Async Replication| RR1
    RM -->|Async Replication| RR2
    RM -.->|Cross-Region Replication| RDR

    style "Primary Cluster (US-East)" fill:#e1f5ff
    style "DR Cluster (US-West)" fill:#fff4e6
```

**Configure DR replica:**

```bash
# In DR cluster, configure Redis as replica of primary
kubectl exec -n o2ims-system redis-dr-0 -- redis-cli \
  REPLICAOF redis-primary.us-east.example.com 6379

# Set read-only on DR replica
kubectl exec -n o2ims-system redis-dr-0 -- redis-cli \
  CONFIG SET replica-read-only yes

# Monitor replication lag
kubectl exec -n o2ims-system redis-dr-0 -- redis-cli INFO replication
```

### Strategy 4: Volume Snapshots

**Purpose**: Filesystem-level backup of Redis data
**RPO**: Based on snapshot frequency
**Storage**: Cloud provider volume snapshots

#### Using VolumeSnapshot

```yaml
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshot
metadata:
  name: redis-snapshot-20260112
  namespace: o2ims-system
spec:
  volumeSnapshotClassName: csi-snapclass
  source:
    persistentVolumeClaimName: redis-data-redis-node-0
```

**Automated snapshots with CronJob:**

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: redis-volume-snapshot
  namespace: o2ims-system
spec:
  schedule: "0 */6 * * *"
  jobTemplate:
    spec:
      template:
        spec:
          serviceAccountName: volume-snapshot
          containers:
          - name: snapshot
            image: bitnami/kubectl:latest
            command:
            - /bin/sh
            - -c
            - |
              TIMESTAMP=$(date +%Y%m%d-%H%M%S)

              cat <<EOF | kubectl apply -f -
              apiVersion: snapshot.storage.k8s.io/v1
              kind: VolumeSnapshot
              metadata:
                name: redis-snapshot-${TIMESTAMP}
                namespace: o2ims-system
              spec:
                volumeSnapshotClassName: csi-snapclass
                source:
                  persistentVolumeClaimName: redis-data-redis-node-0
              EOF

              # Wait for snapshot to be ready
              kubectl wait --for=condition=ready volumesnapshot/redis-snapshot-${TIMESTAMP} \
                -n o2ims-system --timeout=300s

              echo "Snapshot redis-snapshot-${TIMESTAMP} created successfully"

              # Delete snapshots older than 7 days
              kubectl get volumesnapshots -n o2ims-system \
                --sort-by=.metadata.creationTimestamp | \
                head -n -168 | \
                awk '{print $1}' | \
                xargs -r kubectl delete volumesnapshot -n o2ims-system
          restartPolicy: OnFailure
```

## Recovery Procedures

### Scenario 1: Single Pod Failure

**Symptoms:**
- One gateway pod down
- Other pods healthy and serving traffic

**Impact:**
- No service impact (HA maintained)
- Reduced capacity

**Recovery:**

```bash
# Verify pod status
kubectl get pods -n o2ims-system -l app.kubernetes.io/name=netweave

# Check pod logs
kubectl logs -n o2ims-system <failed-pod> --previous

# Delete failed pod (StatefulSet will recreate)
kubectl delete pod <failed-pod> -n o2ims-system

# Monitor recreation
kubectl get pods -n o2ims-system -w

# Verify new pod is healthy
kubectl logs -n o2ims-system <new-pod> --tail=50
curl -k https://<pod-ip>:8080/healthz
```

**RTO**: 2-3 minutes
**RPO**: None (no data loss)

### Scenario 2: Redis Primary Failure

**Symptoms:**
- Redis primary pod down
- Gateway pods unable to connect

**Impact:**
- Service degraded/unavailable
- No new subscriptions accepted
- Existing subscriptions not updated

**Recovery (Automatic with Sentinel):**

```bash
# Verify Sentinel detects failure
kubectl exec -n o2ims-system redis-sentinel-0 -- \
  redis-cli -p 26379 SENTINEL get-master-addr-by-name mymaster

# Monitor automatic failover
kubectl logs -n o2ims-system redis-sentinel-0 -f

# Verify new master elected
kubectl exec -n o2ims-system redis-sentinel-0 -- \
  redis-cli -p 26379 SENTINEL masters

# Check gateway reconnection
kubectl logs -n o2ims-system -l app.kubernetes.io/name=netweave | grep "redis"

# Verify service restored
curl -k https://o2ims.example.com/o2ims-infrastructureInventory/v1/api_versions
```

**RTO**: 1-2 minutes (automatic failover)
**RPO**: < 1 second (replication lag)

### Scenario 3: Redis Data Corruption

**Symptoms:**
- Redis won't start
- Error messages about corrupted AOF/RDB files

**Impact:**
- Service unavailable
- Data recovery required

**Recovery:**

```bash
# Check Redis logs
kubectl logs -n o2ims-system redis-node-0

# If AOF corrupted, try repair
kubectl exec -n o2ims-system redis-node-0 -- redis-check-aof --fix /data/appendonly.aof

# If RDB corrupted, try repair
kubectl exec -n o2ims-system redis-node-0 -- redis-check-rdb /data/dump.rdb

# If repair fails, restore from backup
# 1. Stop Redis
kubectl scale statefulset redis-node -n o2ims-system --replicas=0

# 2. Download latest backup from S3
aws s3 cp s3://netweave-backups/redis-snapshots/dump-latest.rdb.gz /tmp/
gunzip /tmp/dump-latest.rdb.gz

# 3. Copy to Redis PVC
kubectl run -n o2ims-system restore-helper --rm -it \
  --image=redis:7.4-alpine \
  --overrides='
  {
    "spec": {
      "containers": [{
        "name": "restore-helper",
        "image": "redis:7.4-alpine",
        "command": ["sleep", "3600"],
        "volumeMounts": [{
          "name": "data",
          "mountPath": "/data"
        }]
      }],
      "volumes": [{
        "name": "data",
        "persistentVolumeClaim": {
          "claimName": "redis-data-redis-node-0"
        }
      }]
    }
  }'

# In separate terminal, copy backup
kubectl cp /tmp/dump-latest.rdb o2ims-system/restore-helper:/data/dump.rdb

# 4. Restart Redis
kubectl scale statefulset redis-node -n o2ims-system --replicas=3

# 5. Verify data restored
kubectl exec -n o2ims-system redis-node-0 -- redis-cli DBSIZE
kubectl exec -n o2ims-system redis-node-0 -- redis-cli KEYS "subscription:*"
```

**RTO**: 15-30 minutes
**RPO**: Based on backup age (1-6 hours)

### Scenario 4: Complete Cluster Failure

**Symptoms:**
- Entire Kubernetes cluster down
- All pods unreachable

**Impact:**
- Complete service outage
- DR cluster activation required

**Recovery (Failover to DR Cluster):**

```bash
# 1. Verify primary cluster is down
kubectl cluster-info  # Should fail

# 2. Switch context to DR cluster
kubectl config use-context dr-cluster

# 3. Promote DR Redis replica to master
kubectl exec -n o2ims-system redis-dr-0 -- redis-cli REPLICAOF NO ONE

# 4. Verify promotion
kubectl exec -n o2ims-system redis-dr-0 -- redis-cli INFO replication | grep role

# 5. Update DNS to point to DR cluster
# Update o2ims.example.com -> dr-cluster-lb-ip

# 6. Scale up gateway pods in DR cluster
kubectl scale deployment netweave-gateway -n o2ims-system --replicas=3

# 7. Verify service restored
curl -k https://o2ims.example.com/healthz

# 8. Monitor metrics
kubectl port-forward -n o2ims-system svc/netweave-gateway 8080:8080 &
curl http://localhost:8080/metrics
```

**RTO**: 15-30 minutes
**RPO**: < 1 second (if replication active)

### Scenario 5: Restore from Backup After Data Loss

**Scenario**: Accidental deletion of subscriptions or configuration drift

**Recovery:**

```bash
# 1. Identify backup to restore
aws s3 ls s3://netweave-backups/redis-snapshots/ | sort -r | head -10

# 2. Download backup
BACKUP_FILE="dump-20260112-140000.rdb.gz"
aws s3 cp s3://netweave-backups/redis-snapshots/${BACKUP_FILE} /tmp/
gunzip /tmp/${BACKUP_FILE}

# 3. Create restore job
cat <<EOF | kubectl apply -f -
apiVersion: batch/v1
kind: Job
metadata:
  name: redis-restore
  namespace: o2ims-system
spec:
  template:
    spec:
      containers:
      - name: restore
        image: redis:7.4-alpine
        command:
        - /bin/sh
        - -c
        - |
          # Stop writes to Redis
          redis-cli -h redis-node-0 CONFIG SET stop-writes-on-bgsave-error no

          # Copy backup to temp location
          cp /backup/dump.rdb /data/dump-restore.rdb

          # Use DEBUG RELOAD to load backup
          redis-cli -h redis-node-0 DEBUG RELOAD NOSAVE

          echo "Restore completed"
        volumeMounts:
        - name: backup
          mountPath: /backup
        - name: data
          mountPath: /data
      volumes:
      - name: backup
        configMap:
          name: redis-backup-file
      - name: data
        persistentVolumeClaim:
          claimName: redis-data-redis-node-0
      restartPolicy: Never
EOF

# 4. Monitor restore
kubectl logs -n o2ims-system job/redis-restore -f

# 5. Verify data restored
kubectl exec -n o2ims-system redis-node-0 -- redis-cli DBSIZE
kubectl exec -n o2ims-system redis-node-0 -- redis-cli KEYS "subscription:*"

# 6. Re-enable writes
kubectl exec -n o2ims-system redis-node-0 -- redis-cli CONFIG SET stop-writes-on-bgsave-error yes

# 7. Restart gateway pods to clear caches
kubectl rollout restart deployment/netweave-gateway -n o2ims-system
```

**RTO**: 20-40 minutes
**RPO**: Based on backup age

## Backup Verification

### Regular Backup Testing

**Monthly backup restore test:**

```bash
#!/bin/bash
# test-backup-restore.sh

set -e

echo "Starting backup restore test..."

# 1. Create test subscription
TEST_SUB_ID="test-backup-$(date +%s)"
curl -k -X POST https://o2ims.example.com/o2ims-infrastructureInventory/v1/subscriptions \
  -H "Content-Type: application/json" \
  -d "{
    \"callback\": \"https://test.example.com/notify\",
    \"consumerSubscriptionId\": \"${TEST_SUB_ID}\"
  }"

# 2. Verify subscription exists
kubectl exec -n o2ims-system redis-node-0 -- redis-cli GET "subscription:${TEST_SUB_ID}"

# 3. Trigger backup
kubectl exec -n o2ims-system redis-node-0 -- redis-cli BGSAVE

# Wait for backup to complete
sleep 10

# 4. Download backup
kubectl exec -n o2ims-system redis-node-0 -- cat /data/dump.rdb > /tmp/test-backup.rdb

# 5. Restore to test Redis instance
docker run -d --name redis-test -p 6380:6379 redis:7.4-alpine
sleep 5
docker cp /tmp/test-backup.rdb redis-test:/data/dump.rdb
docker restart redis-test
sleep 5

# 6. Verify subscription in restored instance
RESTORED_SUB=$(docker exec redis-test redis-cli GET "subscription:${TEST_SUB_ID}")

if [ -n "$RESTORED_SUB" ]; then
  echo "✓ Backup restore test PASSED"
  echo "  Test subscription found in restored backup"
else
  echo "✗ Backup restore test FAILED"
  echo "  Test subscription NOT found in restored backup"
  exit 1
fi

# 7. Cleanup
docker rm -f redis-test
kubectl exec -n o2ims-system redis-node-0 -- redis-cli DEL "subscription:${TEST_SUB_ID}"

echo "Backup restore test completed successfully"
```

### Backup Monitoring

**Alert on backup failures:**

```yaml
# Prometheus alert
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: redis-backup-alerts
  namespace: o2ims-system
spec:
  groups:
  - name: redis_backup
    interval: 30s
    rules:
    - alert: RedisBackupFailed
      expr: |
        kube_job_status_failed{namespace="o2ims-system",job_name=~"redis-backup.*"} > 0
      for: 5m
      labels:
        severity: critical
      annotations:
        summary: "Redis backup job failed"
        description: "Backup job {{ $labels.job_name }} has failed"

    - alert: RedisBackupOld
      expr: |
        time() - redis_rdb_last_save_timestamp_seconds > 21600
      for: 1h
      labels:
        severity: warning
      annotations:
        summary: "Redis backup is old"
        description: "Last Redis backup is older than 6 hours"
```

## Backup Retention Policy

### Retention Schedule

| Backup Type | Retention Period | Storage Class |
|-------------|------------------|---------------|
| **Hourly snapshots** | 7 days | Standard |
| **Daily snapshots** | 30 days | Standard-IA |
| **Weekly snapshots** | 90 days | Standard-IA |
| **Monthly snapshots** | 1 year | Glacier |
| **Yearly snapshots** | 7 years (compliance) | Glacier Deep Archive |

### Implement Lifecycle Policy

**S3 lifecycle policy:**

```json
{
  "Rules": [
    {
      "Id": "HourlyBackupRetention",
      "Filter": {
        "Prefix": "redis-snapshots/hourly/"
      },
      "Status": "Enabled",
      "Expiration": {
        "Days": 7
      }
    },
    {
      "Id": "DailyBackupRetention",
      "Filter": {
        "Prefix": "redis-snapshots/daily/"
      },
      "Status": "Enabled",
      "Transitions": [
        {
          "Days": 1,
          "StorageClass": "STANDARD_IA"
        }
      ],
      "Expiration": {
        "Days": 30
      }
    },
    {
      "Id": "WeeklyBackupRetention",
      "Filter": {
        "Prefix": "redis-snapshots/weekly/"
      },
      "Status": "Enabled",
      "Transitions": [
        {
          "Days": 7,
          "StorageClass": "GLACIER"
        }
      ],
      "Expiration": {
        "Days": 90
      }
    },
    {
      "Id": "MonthlyBackupRetention",
      "Filter": {
        "Prefix": "redis-snapshots/monthly/"
      },
      "Status": "Enabled",
      "Transitions": [
        {
          "Days": 30,
          "StorageClass": "DEEP_ARCHIVE"
        }
      ],
      "Expiration": {
        "Days": 2555
      }
    }
  ]
}
```

## Disaster Recovery Testing

### Quarterly DR Drill

**DR test checklist:**

- [ ] Notify stakeholders of DR test
- [ ] Document current state (subscription count, metrics)
- [ ] Simulate primary cluster failure
- [ ] Failover to DR cluster
- [ ] Verify service availability
- [ ] Run functional tests
- [ ] Measure RTO and RPO achieved
- [ ] Document lessons learned
- [ ] Failback to primary cluster
- [ ] Verify normal operations restored

**DR test script:**

```bash
#!/bin/bash
# dr-test.sh

echo "=== Disaster Recovery Test ==="
echo "Start time: $(date)"

# 1. Baseline metrics
echo "Recording baseline metrics..."
BASELINE_SUBS=$(kubectl exec -n o2ims-system redis-node-0 -- redis-cli DBSIZE)
echo "  Subscriptions: ${BASELINE_SUBS}"

# 2. Simulate failure
echo "Simulating primary cluster failure..."
START_TIME=$(date +%s)
kubectl scale deployment netweave-gateway -n o2ims-system --replicas=0
kubectl scale statefulset redis-node -n o2ims-system --replicas=0

# 3. Switch to DR
echo "Switching to DR cluster..."
kubectl config use-context dr-cluster

# 4. Promote DR Redis
kubectl exec -n o2ims-system redis-dr-0 -- redis-cli REPLICAOF NO ONE

# 5. Scale up DR gateway
kubectl scale deployment netweave-gateway -n o2ims-system --replicas=3

# 6. Wait for pods ready
kubectl wait --for=condition=ready pod \
  -l app.kubernetes.io/name=netweave \
  -n o2ims-system --timeout=300s

END_TIME=$(date +%s)
RTO=$((END_TIME - START_TIME))

# 7. Verify service
echo "Testing DR service..."
RESPONSE=$(curl -k -s -o /dev/null -w "%{http_code}" \
  https://dr.o2ims.example.com/healthz)

if [ "$RESPONSE" = "200" ]; then
  echo "✓ DR service is healthy"
else
  echo "✗ DR service check failed (HTTP ${RESPONSE})"
  exit 1
fi

# 8. Verify data
DR_SUBS=$(kubectl exec -n o2ims-system redis-dr-0 -- redis-cli DBSIZE)
echo "  DR Subscriptions: ${DR_SUBS}"

DATA_LOSS=$((BASELINE_SUBS - DR_SUBS))
RPO_ESTIMATE=$((DATA_LOSS * 60))  # Rough estimate

# 9. Results
echo ""
echo "=== DR Test Results ==="
echo "  RTO Achieved: ${RTO} seconds (target: 900s)"
echo "  RPO Estimate: ${RPO_ESTIMATE} seconds (target: 300s)"
echo "  Data Loss: ${DATA_LOSS} subscriptions"
echo "End time: $(date)"

if [ ${RTO} -lt 900 ] && [ ${RPO_ESTIMATE} -lt 300 ]; then
  echo "✓ DR test PASSED"
else
  echo "✗ DR test FAILED - RTO/RPO targets not met"
  exit 1
fi
```

## Related Documentation

- [Operations Overview](README.md)
- [Deployment Guide](deployment.md)
- [Monitoring Guide](monitoring.md)
- [Troubleshooting Guide](troubleshooting.md)
