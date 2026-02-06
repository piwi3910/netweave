# Vault Operations Runbook

Operational procedures for managing HashiCorp Vault in the netweave deployment.

## Table of Contents

1. [Unsealing Procedures](#unsealing-procedures)
2. [PKI Operations](#pki-operations)
3. [Backup and Restore](#backup-and-restore)
4. [Emergency Procedures](#emergency-procedures)
5. [Monitoring and Health](#monitoring-and-health)
6. [Troubleshooting](#troubleshooting)

---

## Unsealing Procedures

### Understanding Vault Sealing

Vault starts in a **sealed** state where it cannot read or write data until it is unsealed using unseal keys. This is a security feature to protect data at rest.

**Seal Status:**
- **Sealed**: Vault cannot access encrypted data (vault is encrypted at rest)
- **Unsealed**: Vault can access encrypted data and serve requests

### Check Seal Status

```bash
# Check all Vault pods
for pod in vault-0 vault-1 vault-2; do
  echo "=== ${pod} ==="
  kubectl exec -n vault-system ${pod} -- vault status | grep -E "Sealed|Initialized"
done
```

**Expected Output (Unsealed):**
```
Initialized     true
Sealed          false
```

### Manual Unseal Procedure

**Prerequisites:**
- Unseal keys available (generated during Vault initialization)
- At least 3 of 5 unseal keys required (threshold)

**Unseal Steps:**

```bash
#!/bin/bash
# unseal-vault.sh

# WARNING: In production, retrieve keys from secure storage (HSM, KMS)
# NEVER store unseal keys in plain text

echo "=== Unsealing Vault Cluster ==="

# Read unseal keys from secure storage
UNSEAL_KEY_1="<key-from-secure-storage>"
UNSEAL_KEY_2="<key-from-secure-storage>"
UNSEAL_KEY_3="<key-from-secure-storage>"

# Unseal each pod
for pod in vault-0 vault-1 vault-2; do
  echo "Unsealing ${pod}..."

  # Apply 3 unseal keys (threshold)
  kubectl exec -n vault-system ${pod} -- vault operator unseal ${UNSEAL_KEY_1}
  kubectl exec -n vault-system ${pod} -- vault operator unseal ${UNSEAL_KEY_2}
  kubectl exec -n vault-system ${pod} -- vault operator unseal ${UNSEAL_KEY_3}

  # Verify unsealed
  SEALED=$(kubectl exec -n vault-system ${pod} -- vault status | grep "Sealed" | awk '{print $2}')
  if [ "${SEALED}" == "false" ]; then
    echo "✓ ${pod} unsealed successfully"
  else
    echo "✗ ${pod} still sealed"
    exit 1
  fi
done

echo "✓ All Vault pods unsealed"
```

### Auto-Unseal with Cloud KMS (Production)

**AWS KMS Auto-Unseal:**

```hcl
# vault.hcl
seal "awskms" {
  region     = "us-east-1"
  kms_key_id = "arn:aws:kms:us-east-1:123456789012:key/abc123..."
}
```

**Azure Key Vault Auto-Unseal:**

```hcl
# vault.hcl
seal "azurekeyvault" {
  tenant_id      = "tenant-uuid"
  client_id      = "client-uuid"
  client_secret  = "client-secret"
  vault_name     = "vault-name"
  key_name       = "key-name"
}
```

**GCP Cloud KMS Auto-Unseal:**

```hcl
# vault.hcl
seal "gcpckms" {
  project     = "my-project"
  region      = "us-east1"
  key_ring    = "vault-keyring"
  crypto_key  = "vault-key"
}
```

### Seal Vault (Emergency Only)

**Use Case**: Security breach, data corruption, emergency maintenance

```bash
# Seal specific pod
kubectl exec -n vault-system vault-0 -- vault operator seal

# Seal all pods
for pod in vault-0 vault-1 vault-2; do
  kubectl exec -n vault-system ${pod} -- vault operator seal
done

# Verify all sealed
kubectl exec -n vault-system vault-0 -- vault status | grep Sealed
# Expected: Sealed  true
```

---

## PKI Operations

### Issue Certificate

**Via Vault CLI:**

```bash
# Authenticate with root token (use carefully)
export VAULT_TOKEN="<root-token-from-secure-storage>"

# Issue certificate
kubectl exec -n vault-system vault-0 -- vault write \
  pki_int/issue/netweave-mtls \
  common_name="user@tenant.example.com" \
  ttl=8760h \
  format=pem

# Save certificate
kubectl exec -n vault-system vault-0 -- vault write -format=json \
  pki_int/issue/netweave-mtls \
  common_name="user@tenant.example.com" \
  ttl=8760h > certificate.json

# Extract components
cat certificate.json | jq -r '.data.certificate' > cert.pem
cat certificate.json | jq -r '.data.private_key' > key.pem
cat certificate.json | jq -r '.data.issuing_ca' > ca.pem
```

**Via Gateway Certificate Manager:**

```bash
# Use the gateway's certificate management API (preferred method)
curl -k -X POST https://o2ims.example.com/api/v1/certificates \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${TOKEN}" \
  -d '{
    "user_id": "user-123",
    "tenant_id": "tenant-abc",
    "common_name": "user-123@tenant-abc.example.com",
    "ttl": "8760h"
  }'
```

### Revoke Certificate

```bash
# Revoke by serial number
kubectl exec -n vault-system vault-0 -- vault write \
  pki_int/revoke \
  serial_number="7c:3e:f8:a1:2b:4d"

# Verify revocation
kubectl exec -n vault-system vault-0 -- vault read \
  pki_int/cert/7c:3e:f8:a1:2b:4d
```

### List Certificates

```bash
# List all certificates
kubectl exec -n vault-system vault-0 -- vault list pki_int/certs

# Get specific certificate details
kubectl exec -n vault-system vault-0 -- vault read \
  pki_int/cert/7c:3e:f8:a1:2b:4d
```

### Rotate Root CA

**WARNING**: This is a critical operation. Test in non-production first.

```bash
#!/bin/bash
# rotate-root-ca.sh

echo "=== Rotating Vault Root CA ==="

# 1. Generate new intermediate CA
kubectl exec -n vault-system vault-0 -- vault write -format=json \
  pki_int/intermediate/generate/internal \
  common_name="NetWeave Intermediate CA" \
  ttl=43800h | jq -r '.data.csr' > intermediate.csr

# 2. Sign with root CA
kubectl exec -n vault-system vault-0 -- vault write -format=json \
  pki/root/sign-intermediate \
  csr=@intermediate.csr \
  format=pem_bundle \
  ttl=43800h | jq -r '.data.certificate' > intermediate.cert.pem

# 3. Import signed certificate
kubectl cp intermediate.cert.pem vault-system/vault-0:/tmp/intermediate.cert.pem

kubectl exec -n vault-system vault-0 -- vault write \
  pki_int/intermediate/set-signed \
  certificate=@/tmp/intermediate.cert.pem

# 4. Verify new CA
kubectl exec -n vault-system vault-0 -- vault read pki_int/cert/ca

echo "✓ Root CA rotation complete"
```

### Update PKI Role

```bash
# Get current role configuration
kubectl exec -n vault-system vault-0 -- vault read \
  pki_int/roles/netweave-mtls

# Update role
kubectl exec -n vault-system vault-0 -- vault write \
  pki_int/roles/netweave-mtls \
  allowed_domains="example.com,tenant.example.com" \
  allow_subdomains=true \
  max_ttl=8760h \
  ttl=8760h \
  generate_lease=true \
  no_store=false
```

### Check Certificate Expiry

```bash
# Get CA certificate expiry
kubectl exec -n vault-system vault-0 -- vault read -format=json \
  pki_int/cert/ca | jq -r '.data.certificate' | \
  openssl x509 -noout -dates

# Expected output:
# notBefore=Jan 20 10:00:00 2026 GMT
# notAfter=Jan 20 10:00:00 2031 GMT
```

---

## Backup and Restore

### Backup Procedures

#### Snapshot Backup (Integrated Storage)

```bash
#!/bin/bash
# backup-vault.sh

set -e

BACKUP_DIR="backups/vault-$(date +%Y%m%d-%H%M%S)"
mkdir -p "${BACKUP_DIR}"

echo "Creating Vault snapshot..."

# Take Raft snapshot
kubectl exec -n vault-system vault-0 -- \
  vault operator raft snapshot save /tmp/vault-snapshot.snap

# Copy snapshot to local
kubectl cp vault-system/vault-0:/tmp/vault-snapshot.snap \
  "${BACKUP_DIR}/vault-snapshot.snap"

# Backup unseal keys (if stored in k8s - NOT RECOMMENDED)
# In production, retrieve from HSM/KMS
kubectl get secret vault-unseal-keys -n vault-system -o yaml > \
  "${BACKUP_DIR}/unseal-keys.yaml"

# Backup Vault configuration
kubectl get configmap vault-config -n vault-system -o yaml > \
  "${BACKUP_DIR}/vault-config.yaml"

# Compress backup
tar czf "${BACKUP_DIR}.tar.gz" "${BACKUP_DIR}"
rm -rf "${BACKUP_DIR}"

echo "Backup complete: ${BACKUP_DIR}.tar.gz"
```

#### PKI Configuration Export

```bash
# Export root CA
kubectl exec -n vault-system vault-0 -- vault read -format=json \
  pki/cert/ca | jq -r '.data.certificate' > backups/root-ca.pem

# Export intermediate CA
kubectl exec -n vault-system vault-0 -- vault read -format=json \
  pki_int/cert/ca | jq -r '.data.certificate' > backups/intermediate-ca.pem

# Export PKI role configuration
kubectl exec -n vault-system vault-0 -- vault read -format=json \
  pki_int/roles/netweave-mtls > backups/pki-role-config.json
```

### Restore Procedures

#### Snapshot Restore

```bash
#!/bin/bash
# restore-vault.sh

set -e

echo "=== Restoring Vault from Snapshot ==="

# Extract backup
BACKUP_FILE="backups/vault-20260206.tar.gz"
RESTORE_DIR=$(mktemp -d)
tar xzf "${BACKUP_FILE}" -C "${RESTORE_DIR}"

# Copy snapshot to Vault pod
kubectl cp "${RESTORE_DIR}/vault-snapshot.snap" \
  vault-system/vault-0:/tmp/vault-snapshot.snap

# Restore snapshot
kubectl exec -n vault-system vault-0 -- \
  vault operator raft snapshot restore /tmp/vault-snapshot.snap

# Restart all Vault pods
kubectl rollout restart statefulset/vault -n vault-system

# Wait for pods to be ready
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=vault \
  -n vault-system --timeout=300s

# Unseal all pods
# (Run unseal procedure - see above)

echo "✓ Vault restore complete"
```

#### PKI Configuration Restore

```bash
# Restore PKI mounts (if needed)
kubectl exec -n vault-system vault-0 -- vault secrets enable pki
kubectl exec -n vault-system vault-0 -- vault secrets enable -path=pki_int pki

# Tune PKI max lease TTL
kubectl exec -n vault-system vault-0 -- vault secrets tune -max-lease-ttl=87600h pki
kubectl exec -n vault-system vault-0 -- vault secrets tune -max-lease-ttl=87600h pki_int

# Import root CA
kubectl cp backups/root-ca.pem vault-system/vault-0:/tmp/root-ca.pem
kubectl exec -n vault-system vault-0 -- vault write \
  pki/config/ca pem_bundle=@/tmp/root-ca.pem

# Import intermediate CA
kubectl cp backups/intermediate-ca.pem vault-system/vault-0:/tmp/intermediate-ca.pem
kubectl exec -n vault-system vault-0 -- vault write \
  pki_int/intermediate/set-signed certificate=@/tmp/intermediate-ca.pem

# Restore PKI role
kubectl cp backups/pki-role-config.json vault-system/vault-0:/tmp/role-config.json
kubectl exec -n vault-system vault-0 -- vault write \
  pki_int/roles/netweave-mtls \
  @/tmp/role-config.json
```

---

## Emergency Procedures

### Vault Cluster Outage Recovery

**Scenario**: All Vault pods down or unreachable

```bash
#!/bin/bash
# emergency-vault-recovery.sh

echo "=== Vault Emergency Recovery ==="

# 1. Check pod status
echo "Checking Vault pods..."
kubectl get pods -n vault-system -l app.kubernetes.io/name=vault

# 2. Check for stuck pods
echo "Checking for stuck pods..."
STUCK_PODS=$(kubectl get pods -n vault-system -l app.kubernetes.io/name=vault \
  -o json | jq -r '.items[] | select(.status.phase != "Running") | .metadata.name')

if [ -n "${STUCK_PODS}" ]; then
  echo "Force deleting stuck pods..."
  for pod in ${STUCK_PODS}; do
    kubectl delete pod -n vault-system ${pod} --force --grace-period=0
  done
fi

# 3. Restart StatefulSet
echo "Restarting Vault StatefulSet..."
kubectl rollout restart statefulset/vault -n vault-system

# 4. Wait for pods
echo "Waiting for pods to be ready..."
sleep 30

# 5. Unseal all pods
echo "Unsealing Vault pods..."
# Run unseal procedure (see above)

# 6. Verify health
echo "Verifying Vault health..."
for pod in vault-0 vault-1 vault-2; do
  STATUS=$(kubectl exec -n vault-system ${pod} -- vault status 2>&1 | grep "Sealed" | awk '{print $2}')
  if [ "${STATUS}" == "false" ]; then
    echo "✓ ${pod} healthy"
  else
    echo "✗ ${pod} not healthy"
  fi
done

echo "✓ Emergency recovery complete"
```

### Raft Cluster Recovery

**Scenario**: Raft consensus lost, cluster unhealthy

```bash
# Check Raft status
kubectl exec -n vault-system vault-0 -- vault operator raft list-peers

# Remove failed node from Raft
kubectl exec -n vault-system vault-0 -- vault operator raft remove-peer vault-2

# Force new leader election
kubectl exec -n vault-system vault-0 -- vault operator step-down

# Verify new leader
kubectl exec -n vault-system vault-0 -- vault operator raft list-peers
```

### Restore from Backup After Data Loss

```bash
#!/bin/bash
# restore-after-data-loss.sh

echo "=== Restoring Vault After Data Loss ==="

# 1. Stop all Vault pods
echo "Stopping Vault..."
kubectl scale statefulset vault -n vault-system --replicas=0

# 2. Clear old data (if corruption detected)
echo "Clearing old data..."
for pod in vault-0 vault-1 vault-2; do
  kubectl exec -n vault-system ${pod} -- rm -rf /vault/data/*
done

# 3. Restore from snapshot (latest backup)
LATEST_BACKUP=$(ls -t backups/vault-*.tar.gz | head -1)
echo "Restoring from backup: ${LATEST_BACKUP}"

# Extract and restore (see restore procedure above)

# 4. Start Vault
echo "Starting Vault..."
kubectl scale statefulset vault -n vault-system --replicas=3

# 5. Wait and unseal
echo "Waiting for pods..."
kubectl wait --for=condition=ready pod vault-0 -n vault-system --timeout=300s

# 6. Unseal all pods
echo "Unsealing..."
# Run unseal procedure

echo "✓ Restore complete"
```

---

## Monitoring and Health

### Health Checks

```bash
# System health
kubectl exec -n vault-system vault-0 -- vault status

# Raft cluster health
kubectl exec -n vault-system vault-0 -- vault operator raft list-peers

# PKI health
kubectl exec -n vault-system vault-0 -- vault read pki_int/cert/ca
```

**Expected Healthy Output:**
```
Key                     Value
---                     -----
Seal Type               shamir
Initialized             true
Sealed                  false
Total Shares            5
Threshold               3
Version                 1.15.4
Build Date              2024-01-26T14:53:40Z
Storage Type            raft
Cluster Name            vault-cluster-abc123
Cluster ID              12345678-1234-1234-1234-123456789012
HA Enabled              true
HA Cluster              https://vault-0.vault-internal:8201
HA Mode                 active
Active Since            2026-02-06T10:00:00.000000Z
Raft Committed Index    1234
Raft Applied Index      1234
```

### Metrics Monitoring

```bash
# Port-forward to Vault metrics endpoint
kubectl port-forward -n vault-system vault-0 8200:8200 &

# Query Prometheus metrics
curl -s http://localhost:8200/v1/sys/metrics?format=prometheus

# Key metrics to monitor:
# - vault_core_unsealed: 1 = unsealed, 0 = sealed
# - vault_raft_leader: 1 = leader, 0 = follower
# - vault_raft_peers: Number of Raft peers
# - vault_pki_tidy_cert_store_current_entry: Certificate count
```

### Prometheus Alerts

```yaml
# vault-alerts.yaml
groups:
  - name: vault
    rules:
      - alert: VaultSealed
        expr: vault_core_unsealed == 0
        for: 1m
        annotations:
          summary: "Vault is sealed"

      - alert: VaultRaftLeaderMissing
        expr: max(vault_raft_leader) == 0
        for: 2m
        annotations:
          summary: "No Raft leader elected"

      - alert: VaultHighCertificateCount
        expr: vault_pki_tidy_cert_store_current_entry > 10000
        for: 10m
        annotations:
          summary: "High certificate count (> 10k)"
```

---

## Troubleshooting

### Issue: Vault Pod Sealed After Restart

**Symptoms:**
- Pod shows "Sealed" status after restart
- Cannot access Vault data

**Diagnosis:**
```bash
kubectl exec -n vault-system vault-0 -- vault status | grep Sealed
# Sealed  true
```

**Solution:**
```bash
# Unseal the pod (see unseal procedure above)
# In production, use auto-unseal with KMS to prevent this
```

### Issue: Cannot Issue Certificates

**Symptoms:**
- Certificate issuance fails with permission error
- PKI role not found

**Diagnosis:**
```bash
# Check PKI mount
kubectl exec -n vault-system vault-0 -- vault secrets list | grep pki

# Check PKI role
kubectl exec -n vault-system vault-0 -- vault read pki_int/roles/netweave-mtls
```

**Solution:**
```bash
# Re-create PKI role if missing
kubectl exec -n vault-system vault-0 -- vault write \
  pki_int/roles/netweave-mtls \
  allowed_domains="example.com" \
  allow_subdomains=true \
  max_ttl=8760h
```

### Issue: Raft Consensus Lost

**Symptoms:**
- Vault pods show no leader
- Read/write operations failing

**Diagnosis:**
```bash
kubectl exec -n vault-system vault-0 -- vault operator raft list-peers

# Check for:
# - No leader elected
# - Missing peers
# - Network partitions
```

**Solution:**
```bash
# Force leader election
kubectl exec -n vault-system vault-0 -- vault operator step-down

# Remove failed peer
kubectl exec -n vault-system vault-0 -- vault operator raft remove-peer vault-2

# Verify recovery
kubectl exec -n vault-system vault-0 -- vault operator raft list-peers
```

### Issue: High Memory Usage

**Symptoms:**
- Vault pods OOMKilled
- Slow response times

**Diagnosis:**
```bash
kubectl top pod -n vault-system -l app.kubernetes.io/name=vault
```

**Solution:**
```bash
# Increase memory limits
kubectl patch statefulset vault -n vault-system --patch '
{
  "spec": {
    "template": {
      "spec": {
        "containers": [
          {
            "name": "vault",
            "resources": {
              "limits": {
                "memory": "2Gi"
              }
            }
          }
        ]
      }
    }
  }
}'

# Restart pods
kubectl rollout restart statefulset/vault -n vault-system
```

---

## Related Documentation

- [Deployment Plan](../deployment-plan.md) - Migration deployment procedures
- [Rollback Plan](../rollback-plan.md) - Rollback procedures
- [Keycloak Operations](keycloak-operations.md) - Keycloak management
- [Security Architecture](../../security/architecture.md) - Security overview

---

**Document Version**: 1.0
**Last Updated**: 2026-02-06
**Maintained By**: NetWeave Operations Team
