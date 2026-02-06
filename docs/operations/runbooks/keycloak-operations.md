# Keycloak Operations Runbook

Operational procedures for managing Keycloak in the netweave deployment.

## Table of Contents

1. [Common Operations](#common-operations)
2. [User Management](#user-management)
3. [Realm Configuration](#realm-configuration)
4. [Client Management](#client-management)
5. [Troubleshooting](#troubleshooting)
6. [Backup and Restore](#backup-and-restore)
7. [Scaling Procedures](#scaling-procedures)
8. [Emergency Procedures](#emergency-procedures)

---

## Common Operations

### Access Keycloak Admin Console

**Via Port Forward:**
```bash
# Port-forward to Keycloak service
kubectl port-forward -n keycloak-system svc/keycloak 8080:80

# Open browser to http://localhost:8080
# Default path: /admin

# Get admin password
ADMIN_PASSWORD=$(kubectl get secret -n keycloak-system keycloak \
  -o jsonpath='{.data.admin-password}' | base64 -d)

echo "Admin username: admin"
echo "Admin password: ${ADMIN_PASSWORD}"
```

**Via Ingress (if enabled):**
```bash
# Access at: https://keycloak.example.com/admin
```

### Using kcadm CLI

The kcadm CLI tool provides command-line access to Keycloak admin API:

**Authenticate:**
```bash
kubectl exec -n keycloak-system keycloak-0 -- \
  /opt/keycloak/bin/kcadm.sh config credentials \
  --server http://localhost:8080 \
  --realm master \
  --user admin \
  --password "${ADMIN_PASSWORD}"
```

**Common Commands:**
```bash
# List all realms
kubectl exec -n keycloak-system keycloak-0 -- \
  /opt/keycloak/bin/kcadm.sh get realms

# List users in realm
kubectl exec -n keycloak-system keycloak-0 -- \
  /opt/keycloak/bin/kcadm.sh get users -r netweave

# List groups
kubectl exec -n keycloak-system keycloak-0 -- \
  /opt/keycloak/bin/kcadm.sh get groups -r netweave

# List clients
kubectl exec -n keycloak-system keycloak-0 -- \
  /opt/keycloak/bin/kcadm.sh get clients -r netweave
```

### Check Keycloak Health

```bash
# Health endpoint
kubectl exec -n keycloak-system keycloak-0 -- \
  curl -sf http://localhost:8080/health

# Readiness endpoint
kubectl exec -n keycloak-system keycloak-0 -- \
  curl -sf http://localhost:8080/health/ready

# Liveness endpoint
kubectl exec -n keycloak-system keycloak-0 -- \
  curl -sf http://localhost:8080/health/live

# Database connectivity
kubectl exec -n keycloak-system keycloak-0 -- \
  curl -sf http://localhost:8080/health | jq '.checks[] | select(.name == "Database connections health check")'
```

**Expected Response:**
```json
{
  "status": "UP",
  "checks": [
    {
      "name": "Keycloak database connections health check",
      "status": "UP"
    }
  ]
}
```

---

## User Management

### Create User

**Via kcadm CLI:**
```bash
kubectl exec -n keycloak-system keycloak-0 -- \
  /opt/keycloak/bin/kcadm.sh create users -r netweave \
  -s username=new-user \
  -s enabled=true \
  -s email=new-user@example.com \
  -s firstName=New \
  -s lastName=User
```

**Set User Password:**
```bash
# Get user ID
USER_ID=$(kubectl exec -n keycloak-system keycloak-0 -- \
  /opt/keycloak/bin/kcadm.sh get users -r netweave -q username=new-user \
  | jq -r '.[0].id')

# Set password (temporary)
kubectl exec -n keycloak-system keycloak-0 -- \
  /opt/keycloak/bin/kcadm.sh set-password -r netweave \
  --userid ${USER_ID} \
  --new-password 'TempPassword123!' \
  --temporary

# Set password (permanent)
kubectl exec -n keycloak-system keycloak-0 -- \
  /opt/keycloak/bin/kcadm.sh set-password -r netweave \
  --userid ${USER_ID} \
  --new-password 'SecurePassword123!'
```

### List Users

```bash
# List all users
kubectl exec -n keycloak-system keycloak-0 -- \
  /opt/keycloak/bin/kcadm.sh get users -r netweave

# List users with filters
kubectl exec -n keycloak-system keycloak-0 -- \
  /opt/keycloak/bin/kcadm.sh get users -r netweave \
  -q email=*@example.com \
  -q enabled=true

# Get specific user
kubectl exec -n keycloak-system keycloak-0 -- \
  /opt/keycloak/bin/kcadm.sh get users -r netweave \
  -q username=specific-user
```

### Update User

```bash
# Get user ID
USER_ID=$(kubectl exec -n keycloak-system keycloak-0 -- \
  /opt/keycloak/bin/kcadm.sh get users -r netweave -q username=user-to-update \
  | jq -r '.[0].id')

# Update user attributes
kubectl exec -n keycloak-system keycloak-0 -- \
  /opt/keycloak/bin/kcadm.sh update users/${USER_ID} -r netweave \
  -s email=updated-email@example.com \
  -s enabled=true

# Add user attribute (for certificate serial, tenant ID, etc.)
kubectl exec -n keycloak-system keycloak-0 -- \
  /opt/keycloak/bin/kcadm.sh update users/${USER_ID} -r netweave \
  -s 'attributes.certificate_serial=["7c:3e:f8:a1:2b:4d"]' \
  -s 'attributes.tenant_id=["tenant-123"]'
```

### Delete User

```bash
# Get user ID
USER_ID=$(kubectl exec -n keycloak-system keycloak-0 -- \
  /opt/keycloak/bin/kcadm.sh get users -r netweave -q username=user-to-delete \
  | jq -r '.[0].id')

# Delete user
kubectl exec -n keycloak-system keycloak-0 -- \
  /opt/keycloak/bin/kcadm.sh delete users/${USER_ID} -r netweave
```

### Assign User to Group

```bash
# Get user ID
USER_ID=$(kubectl exec -n keycloak-system keycloak-0 -- \
  /opt/keycloak/bin/kcadm.sh get users -r netweave -q username=user-name \
  | jq -r '.[0].id')

# Get group ID
GROUP_ID=$(kubectl exec -n keycloak-system keycloak-0 -- \
  /opt/keycloak/bin/kcadm.sh get groups -r netweave -q name=tenant-admins \
  | jq -r '.[0].id')

# Add user to group
kubectl exec -n keycloak-system keycloak-0 -- \
  /opt/keycloak/bin/kcadm.sh update users/${USER_ID}/groups/${GROUP_ID} -r netweave \
  -s realm=netweave -s userId=${USER_ID} -s groupId=${GROUP_ID} -n
```

### Remove User from Group

```bash
# Get user ID and group ID (same as above)

# Remove user from group
kubectl exec -n keycloak-system keycloak-0 -- \
  /opt/keycloak/bin/kcadm.sh delete users/${USER_ID}/groups/${GROUP_ID} -r netweave
```

---

## Realm Configuration

### Export Realm Configuration

```bash
# Export full realm configuration
kubectl exec -n keycloak-system keycloak-0 -- \
  /opt/keycloak/bin/kc.sh export \
  --dir /tmp \
  --realm netweave \
  --users realm_file

# Copy exported file
kubectl cp keycloak-system/keycloak-0:/tmp/netweave-realm.json \
  ./backups/netweave-realm-$(date +%Y%m%d).json
```

### Import Realm Configuration

```bash
# Copy realm file to pod
kubectl cp ./realm-config/netweave-realm.json \
  keycloak-system/keycloak-0:/tmp/netweave-realm.json

# Import realm
kubectl exec -n keycloak-system keycloak-0 -- \
  /opt/keycloak/bin/kc.sh import \
  --file /tmp/netweave-realm.json
```

### Update Realm Settings

```bash
# Enable user registration
kubectl exec -n keycloak-system keycloak-0 -- \
  /opt/keycloak/bin/kcadm.sh update realms/netweave \
  -s registrationAllowed=true

# Set token lifespans
kubectl exec -n keycloak-system keycloak-0 -- \
  /opt/keycloak/bin/kcadm.sh update realms/netweave \
  -s accessTokenLifespan=300 \
  -s ssoSessionIdleTimeout=1800 \
  -s ssoSessionMaxLifespan=36000

# Configure password policy
kubectl exec -n keycloak-system keycloak-0 -- \
  /opt/keycloak/bin/kcadm.sh update realms/netweave \
  -s 'passwordPolicy="length(12) and digits(1) and lowerCase(1) and upperCase(1) and specialChars(1)"'
```

### View Realm Information

```bash
# Get full realm configuration
kubectl exec -n keycloak-system keycloak-0 -- \
  /opt/keycloak/bin/kcadm.sh get realms/netweave

# Get specific realm settings
kubectl exec -n keycloak-system keycloak-0 -- \
  /opt/keycloak/bin/kcadm.sh get realms/netweave \
  | jq '{
      realm: .realm,
      enabled: .enabled,
      accessTokenLifespan: .accessTokenLifespan,
      ssoSessionMaxLifespan: .ssoSessionMaxLifespan
    }'
```

---

## Client Management

### List Clients

```bash
# List all clients in realm
kubectl exec -n keycloak-system keycloak-0 -- \
  /opt/keycloak/bin/kcadm.sh get clients -r netweave

# Get specific client (netweave-gateway)
kubectl exec -n keycloak-system keycloak-0 -- \
  /opt/keycloak/bin/kcadm.sh get clients -r netweave \
  -q clientId=netweave-gateway
```

### Get Client Secret

```bash
# Get client ID (not the same as clientId)
CLIENT_ID=$(kubectl exec -n keycloak-system keycloak-0 -- \
  /opt/keycloak/bin/kcadm.sh get clients -r netweave \
  -q clientId=netweave-gateway \
  | jq -r '.[0].id')

# Get client secret
kubectl exec -n keycloak-system keycloak-0 -- \
  /opt/keycloak/bin/kcadm.sh get clients/${CLIENT_ID}/client-secret -r netweave \
  | jq -r '.value'
```

### Regenerate Client Secret

```bash
# Get client ID
CLIENT_ID=$(kubectl exec -n keycloak-system keycloak-0 -- \
  /opt/keycloak/bin/kcadm.sh get clients -r netweave \
  -q clientId=netweave-gateway \
  | jq -r '.[0].id')

# Generate new secret
NEW_SECRET=$(kubectl exec -n keycloak-system keycloak-0 -- \
  /opt/keycloak/bin/kcadm.sh create clients/${CLIENT_ID}/client-secret -r netweave \
  | jq -r '.value')

echo "New client secret: ${NEW_SECRET}"

# Update gateway secret
kubectl create secret generic netweave-oauth2-config \
  --from-literal=keycloak-client-secret="${NEW_SECRET}" \
  -n netweave --dry-run=client -o yaml | kubectl apply -f -

# Restart gateway pods to pick up new secret
kubectl rollout restart deployment/netweave-gateway -n netweave
```

### Update Client Configuration

```bash
# Get client ID
CLIENT_ID=$(kubectl exec -n keycloak-system keycloak-0 -- \
  /opt/keycloak/bin/kcadm.sh get clients -r netweave \
  -q clientId=netweave-gateway \
  | jq -r '.[0].id')

# Update redirect URIs
kubectl exec -n keycloak-system keycloak-0 -- \
  /opt/keycloak/bin/kcadm.sh update clients/${CLIENT_ID} -r netweave \
  -s 'redirectUris=["https://o2ims.example.com/*"]'

# Enable service account
kubectl exec -n keycloak-system keycloak-0 -- \
  /opt/keycloak/bin/kcadm.sh update clients/${CLIENT_ID} -r netweave \
  -s serviceAccountsEnabled=true

# Set access type to confidential
kubectl exec -n keycloak-system keycloak-0 -- \
  /opt/keycloak/bin/kcadm.sh update clients/${CLIENT_ID} -r netweave \
  -s publicClient=false
```

---

## Troubleshooting

### Issue: Keycloak Pod Not Starting

**Symptoms:**
- Pod in CrashLoopBackOff or Error state
- Container keeps restarting

**Diagnosis:**
```bash
# Check pod status
kubectl get pods -n keycloak-system -l app.kubernetes.io/name=keycloak

# Check pod events
kubectl describe pod -n keycloak-system keycloak-0

# Check logs
kubectl logs -n keycloak-system keycloak-0 --previous
```

**Common Causes & Solutions:**

1. **Database Connection Failure**
   ```bash
   # Check PostgreSQL health
   kubectl get pods -n keycloak-system -l app.kubernetes.io/name=postgresql

   # Test database connection
   kubectl exec -n keycloak-system postgresql-0 -- \
     psql -U keycloak -d keycloak -c "SELECT 1;"

   # Check Keycloak database configuration
   kubectl get secret -n keycloak-system keycloak -o yaml | grep database
   ```

2. **Insufficient Resources**
   ```bash
   # Check resource limits
   kubectl describe pod -n keycloak-system keycloak-0 | grep -A 10 "Limits:"

   # Check node resources
   kubectl top nodes

   # Increase resource limits
   helm upgrade keycloak bitnami/keycloak \
     -n keycloak-system \
     --reuse-values \
     --set resources.limits.memory=2Gi \
     --set resources.limits.cpu=2000m
   ```

3. **Configuration Error**
   ```bash
   # Check environment variables
   kubectl get deployment -n keycloak-system keycloak -o yaml | grep -A 20 "env:"

   # Verify secret values
   kubectl get secret -n keycloak-system keycloak -o jsonpath='{.data}' | \
     jq 'to_entries | .[] | {key: .key, value: (.value | @base64d)}'
   ```

### Issue: Unable to Log In to Admin Console

**Symptoms:**
- Invalid username or password error
- Account locked

**Diagnosis:**
```bash
# Verify admin password
kubectl get secret -n keycloak-system keycloak \
  -o jsonpath='{.data.admin-password}' | base64 -d

# Check admin user status
kubectl exec -n keycloak-system keycloak-0 -- \
  /opt/keycloak/bin/kcadm.sh get users -r master -q username=admin
```

**Solutions:**

1. **Reset Admin Password**
   ```bash
   # Set new password
   NEW_PASSWORD=$(openssl rand -base64 32)

   # Update Keycloak secret
   kubectl patch secret keycloak -n keycloak-system \
     --type='json' -p='[{"op": "replace", "path": "/data/admin-password", "value": "'$(echo -n "${NEW_PASSWORD}" | base64)'"}]'

   # Restart Keycloak pods
   kubectl rollout restart deployment/keycloak -n keycloak-system

   echo "New admin password: ${NEW_PASSWORD}"
   ```

2. **Unlock Admin Account**
   ```bash
   # This requires direct database access
   kubectl exec -n keycloak-system postgresql-0 -- \
     psql -U keycloak -d keycloak -c \
     "UPDATE user_entity SET enabled = true WHERE username = 'admin' AND realm_id = 'master';"
   ```

### Issue: Token Validation Failures

**Symptoms:**
- Gateway logs show "invalid token" errors
- OAuth2 authentication failing

**Diagnosis:**
```bash
# Check Keycloak realm endpoint
curl -sf http://keycloak.keycloak-system.svc.cluster.local:80/realms/netweave

# Verify issuer URL matches gateway configuration
kubectl get configmap netweave-config -n netweave -o yaml | grep issuerURL

# Test token generation
kubectl exec -n keycloak-system keycloak-0 -- bash -c '
  CLIENT_SECRET=$(cat /opt/keycloak/secrets/client-secret)
  curl -s -X POST http://localhost:8080/realms/netweave/protocol/openid-connect/token \
    -d "client_id=netweave-gateway" \
    -d "client_secret=${CLIENT_SECRET}" \
    -d "username=test-user" \
    -d "password=test-password" \
    -d "grant_type=password" | jq
'
```

**Solutions:**

1. **Verify Client Configuration**
   ```bash
   # Check client exists and is enabled
   CLIENT_ID=$(kubectl exec -n keycloak-system keycloak-0 -- \
     /opt/keycloak/bin/kcadm.sh get clients -r netweave \
     -q clientId=netweave-gateway \
     | jq -r '.[0].id')

   kubectl exec -n keycloak-system keycloak-0 -- \
     /opt/keycloak/bin/kcadm.sh get clients/${CLIENT_ID} -r netweave \
     | jq '{enabled: .enabled, serviceAccountsEnabled: .serviceAccountsEnabled}'
   ```

2. **Check Clock Skew**
   ```bash
   # Check time on gateway pods
   kubectl exec -n netweave netweave-gateway-0 -- date -u

   # Check time on Keycloak pods
   kubectl exec -n keycloak-system keycloak-0 -- date -u

   # Time difference should be < 5 minutes
   ```

### Issue: High Memory Usage

**Symptoms:**
- Keycloak pods OOMKilled
- Slow response times
- Pod evicted due to resource pressure

**Diagnosis:**
```bash
# Check current memory usage
kubectl top pod -n keycloak-system -l app.kubernetes.io/name=keycloak

# Check memory limits
kubectl get deployment -n keycloak-system keycloak \
  -o jsonpath='{.spec.template.spec.containers[0].resources}'

# Check JVM heap usage
kubectl exec -n keycloak-system keycloak-0 -- \
  jcmd 1 VM.native_memory summary
```

**Solutions:**

1. **Increase Memory Limits**
   ```bash
   helm upgrade keycloak bitnami/keycloak \
     -n keycloak-system \
     --reuse-values \
     --set resources.requests.memory=2Gi \
     --set resources.limits.memory=4Gi
   ```

2. **Tune JVM Settings**
   ```bash
   # Set JVM options via environment variable
   helm upgrade keycloak bitnami/keycloak \
     -n keycloak-system \
     --reuse-values \
     --set extraEnvVars[0].name=JAVA_OPTS \
     --set extraEnvVars[0].value="-Xms1g -Xmx2g -XX:MaxMetaspaceSize=512m"
   ```

3. **Scale Horizontally**
   ```bash
   # Increase replica count
   helm upgrade keycloak bitnami/keycloak \
     -n keycloak-system \
     --reuse-values \
     --set replicaCount=3
   ```

---

## Backup and Restore

### Backup Procedures

#### Database Backup (Recommended)

```bash
# Create PostgreSQL backup
kubectl exec -n keycloak-system postgresql-0 -- \
  pg_dump -U keycloak -d keycloak -Fc > backups/keycloak-db-$(date +%Y%m%d-%H%M%S).dump

# Verify backup
ls -lh backups/keycloak-db-*.dump
```

#### Realm Export Backup

```bash
# Export realm configuration
kubectl exec -n keycloak-system keycloak-0 -- \
  /opt/keycloak/bin/kc.sh export \
  --dir /tmp \
  --realm netweave \
  --users realm_file

# Copy to local
kubectl cp keycloak-system/keycloak-0:/tmp/netweave-realm.json \
  backups/netweave-realm-$(date +%Y%m%d-%H%M%S).json
```

#### Full Backup Script

```bash
#!/bin/bash
# backup-keycloak.sh

set -e

BACKUP_DIR="backups/keycloak-$(date +%Y%m%d-%H%M%S)"
mkdir -p "${BACKUP_DIR}"

echo "Creating Keycloak backup..."

# 1. Database backup
echo "Backing up PostgreSQL database..."
kubectl exec -n keycloak-system postgresql-0 -- \
  pg_dump -U keycloak -d keycloak -Fc > "${BACKUP_DIR}/keycloak-db.dump"

# 2. Realm export
echo "Exporting Keycloak realm..."
kubectl exec -n keycloak-system keycloak-0 -- \
  /opt/keycloak/bin/kc.sh export \
  --dir /tmp \
  --realm netweave \
  --users realm_file

kubectl cp keycloak-system/keycloak-0:/tmp/netweave-realm.json \
  "${BACKUP_DIR}/netweave-realm.json"

# 3. Helm values
echo "Backing up Helm configuration..."
helm get values keycloak -n keycloak-system > "${BACKUP_DIR}/helm-values.yaml"

# 4. Secrets (encrypted)
echo "Backing up secrets..."
kubectl get secret -n keycloak-system keycloak -o yaml > "${BACKUP_DIR}/keycloak-secret.yaml"

# 5. Compress backup
echo "Compressing backup..."
tar czf "${BACKUP_DIR}.tar.gz" "${BACKUP_DIR}"
rm -rf "${BACKUP_DIR}"

echo "Backup complete: ${BACKUP_DIR}.tar.gz"
```

### Restore Procedures

#### Database Restore

```bash
# Stop Keycloak
kubectl scale deployment keycloak -n keycloak-system --replicas=0

# Restore database
kubectl cp backups/keycloak-db.dump keycloak-system/postgresql-0:/tmp/restore.dump

kubectl exec -n keycloak-system postgresql-0 -- \
  pg_restore -U keycloak -d keycloak -c /tmp/restore.dump

# Start Keycloak
kubectl scale deployment keycloak -n keycloak-system --replicas=2

# Verify
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=keycloak \
  -n keycloak-system --timeout=300s
```

#### Realm Import Restore

```bash
# Copy realm file
kubectl cp backups/netweave-realm.json keycloak-system/keycloak-0:/tmp/restore-realm.json

# Import realm
kubectl exec -n keycloak-system keycloak-0 -- \
  /opt/keycloak/bin/kc.sh import --file /tmp/restore-realm.json

# Restart Keycloak
kubectl rollout restart deployment/keycloak -n keycloak-system
```

---

## Scaling Procedures

### Horizontal Scaling

**Scale Up:**
```bash
# Increase replicas
helm upgrade keycloak bitnami/keycloak \
  -n keycloak-system \
  --reuse-values \
  --set replicaCount=3

# Verify new pods
kubectl get pods -n keycloak-system -l app.kubernetes.io/name=keycloak -w
```

**Scale Down:**
```bash
# Decrease replicas (minimum 2 for HA)
helm upgrade keycloak bitnami/keycloak \
  -n keycloak-system \
  --reuse-values \
  --set replicaCount=2

# Wait for termination
kubectl get pods -n keycloak-system -w
```

### Vertical Scaling

```bash
# Increase resources
helm upgrade keycloak bitnami/keycloak \
  -n keycloak-system \
  --reuse-values \
  --set resources.requests.cpu=2000m \
  --set resources.requests.memory=2Gi \
  --set resources.limits.cpu=4000m \
  --set resources.limits.memory=4Gi

# Monitor rollout
kubectl rollout status deployment/keycloak -n keycloak-system
```

---

## Emergency Procedures

### Complete Outage Recovery

**Scenario**: All Keycloak pods down

```bash
#!/bin/bash
# emergency-recovery.sh

echo "=== Keycloak Emergency Recovery ==="

# 1. Check PostgreSQL
echo "Checking PostgreSQL..."
kubectl get pods -n keycloak-system -l app.kubernetes.io/name=postgresql

PG_READY=$(kubectl get pods -n keycloak-system -l app.kubernetes.io/name=postgresql \
  -o jsonpath='{.items[*].status.conditions[?(@.type=="Ready")].status}' | grep -o "True" | wc -l)

if [ "${PG_READY}" -eq 0 ]; then
  echo "ERROR: PostgreSQL not ready. Fix PostgreSQL first."
  exit 1
fi

# 2. Check for Keycloak pods
echo "Checking Keycloak pods..."
kubectl get pods -n keycloak-system -l app.kubernetes.io/name=keycloak

# 3. Force delete stuck pods
echo "Deleting stuck pods..."
kubectl delete pod -n keycloak-system -l app.kubernetes.io/name=keycloak --force --grace-period=0

# 4. Scale down and up
echo "Restarting Keycloak..."
kubectl scale deployment keycloak -n keycloak-system --replicas=0
sleep 10
kubectl scale deployment keycloak -n keycloak-system --replicas=2

# 5. Wait for ready
echo "Waiting for Keycloak to be ready..."
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=keycloak \
  -n keycloak-system --timeout=600s

# 6. Verify health
echo "Verifying health..."
kubectl port-forward -n keycloak-system svc/keycloak 8080:80 > /dev/null 2>&1 &
PF_PID=$!
sleep 5

HEALTH=$(curl -s http://localhost:8080/health | jq -r '.status')
kill $PF_PID

if [ "${HEALTH}" == "UP" ]; then
  echo "✓ Keycloak recovery successful"
else
  echo "✗ Keycloak recovery failed"
  exit 1
fi
```

### Rollback to Previous Version

```bash
# View Helm history
helm history keycloak -n keycloak-system

# Rollback to previous version
helm rollback keycloak -n keycloak-system

# Or rollback to specific version
helm rollback keycloak 3 -n keycloak-system

# Verify rollback
kubectl rollout status deployment/keycloak -n keycloak-system
```

---

## Related Documentation

- [Deployment Plan](../deployment-plan.md) - Migration deployment procedures
- [Rollback Plan](../rollback-plan.md) - Rollback procedures
- [Vault Operations](vault-operations.md) - Vault management
- [OAuth2 Migration Guide](../../security/oauth2-migration-guide.md) - Client migration

---

**Document Version**: 1.0
**Last Updated**: 2026-02-06
**Maintained By**: NetWeave Operations Team
