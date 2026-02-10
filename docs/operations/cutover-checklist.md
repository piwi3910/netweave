# Cutover Checklist: Keycloak + Vault Migration

Operational checklist for deployment day execution and validation.

## Table of Contents

1. [Pre-Cutover Verification](#pre-cutover-verification)
2. [During-Cutover Monitoring](#during-cutover-monitoring)
3. [Post-Cutover Validation](#post-cutover-validation)
4. [Sign-Off Requirements](#sign-off-requirements)

---

## Pre-Cutover Verification

### 24 Hours Before Cutover

#### Communications

- [ ] **Stakeholder Notification**: All stakeholders notified of maintenance window
- [ ] **Status Page Updated**: Maintenance banner posted on status page
- [ ] **Team Coordination**: Deployment team confirmed available
  - [ ] Primary Operator: [NAME]
  - [ ] Secondary Operator: [NAME]
  - [ ] SRE On-Call: [NAME]
  - [ ] Application Owner: [NAME]
- [ ] **Communication Channels**: All channels tested
  - [ ] Slack: #netweave-deployment active
  - [ ] Zoom/Teams: Deployment bridge open
  - [ ] Phone Tree: Escalation contacts verified

#### Infrastructure

- [ ] **Kubernetes Cluster Health**
  ```bash
  kubectl get nodes
  kubectl top nodes
  kubectl get componentstatuses
  ```
  - [ ] All nodes Ready
  - [ ] CPU < 70% across all nodes
  - [ ] Memory < 80% across all nodes
  - [ ] No failing component statuses

- [ ] **Storage Verification**
  ```bash
  kubectl get sc
  kubectl get pv
  kubectl get pvc -A
  ```
  - [ ] Storage class available for dynamic provisioning
  - [ ] No PVCs in Pending state
  - [ ] Storage capacity > 200Gi available

- [ ] **Network Verification**
  ```bash
  kubectl get networkpolicies -A
  kubectl get ingress -A
  ```
  - [ ] Network policies not blocking required traffic
  - [ ] Ingress controller healthy
  - [ ] DNS resolution working

- [ ] **Vault PKI Health**
  ```bash
  kubectl get pods -n vault-system
  kubectl exec -n vault-system vault-0 -- vault status | grep -i sealed
  kubectl exec -n vault-system vault-0 -- vault read pki_int/roles/netweave-mtls
  ```
  - [ ] All Vault pods Running and unsealed
  - [ ] PKI engine configured with netweave-mtls role

#### Backups

- [ ] **Redis Backup Created**
  ```bash
  kubectl exec -n netweave redis-0 -- redis-cli BGSAVE
  kubectl cp netweave/redis-0:/data/dump.rdb ./backups/redis-pre-cutover-$(date +%Y%m%d).rdb
  ```
  - [ ] Backup file size > 0 bytes
  - [ ] Backup compressed and stored offsite
  - [ ] Backup restoration tested in non-prod

- [ ] **Configuration Backup**
  ```bash
  helm get values netweave -n netweave > backups/helm-values-pre-cutover.yaml
  kubectl get secrets -n netweave -o yaml > backups/secrets-pre-cutover.yaml
  kubectl get deployment netweave-gateway -n netweave -o yaml > backups/gateway-pre-cutover.yaml
  ```
  - [ ] All configuration files backed up
  - [ ] Files committed to git (excluding secrets)

- [ ] **Secrets Prepared**
  - [ ] PostgreSQL password generated and stored securely
  - [ ] Keycloak admin password generated and stored securely
  - [ ] Keycloak client secret generated and stored securely
  - [ ] All passwords minimum 32 characters, high entropy
  - [ ] Secrets stored in password manager (1Password/LastPass/Vault)

#### Dependencies

- [ ] **External Dependencies Verified**
  - [ ] Container registry accessible (ghcr.io)
  - [ ] Bitnami Helm repository accessible
  - [ ] External DNS resolving correctly
  - [ ] NTP synchronized (time drift < 1 second)

- [ ] **Deployment Artifacts Ready**
  ```bash
  ls -lh deployments/helm/netweave/
  ls -lh deployments/kubernetes/vault/
  ls -lh deployments/keycloak/realm-import/
  ```
  - [ ] Helm chart present and validated
  - [ ] Vault manifests present
  - [ ] Keycloak realm import file present
  - [ ] No uncommitted changes in deployment files

#### Current Service Baseline

- [ ] **Capture Baseline Metrics**
  ```bash
  # User count
  kubectl exec -n netweave redis-0 -- redis-cli KEYS "user:*" | wc -l > baselines/user-count.txt

  # Tenant count
  kubectl exec -n netweave redis-0 -- redis-cli KEYS "tenant:*" | wc -l > baselines/tenant-count.txt

  # Subscription count
  kubectl exec -n netweave redis-0 -- redis-cli KEYS "subscription:*" | wc -l > baselines/subscription-count.txt

  # Gateway pod count
  kubectl get pods -n netweave -l app.kubernetes.io/component=gateway | grep Running | wc -l > baselines/gateway-pods.txt

  # API response time (p95)
  curl -s http://localhost:9090/metrics | grep http_request_duration_seconds | grep 0.95 > baselines/api-latency-p95.txt
  ```
  - [ ] All baseline files created
  - [ ] Baselines documented in deployment log

---

## During-Cutover Monitoring

### Phase 1: Infrastructure Deployment

#### Start: [TIME]

- [ ] **PostgreSQL Deployment**
  - [ ] Helm install command executed
  - [ ] Pods transitioning to Running state
  - [ ] Primary pod ready
  - [ ] Read replica pod ready (if enabled)
  - [ ] Database connection test successful
  - [ ] Test table creation/deletion successful

**Validation Command:**
```bash
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=postgresql \
  -n keycloak-system --timeout=300s
```

**Status**: ⬜ In Progress | ⬜ Complete | ⬜ Failed

- [ ] **Keycloak Deployment**
  - [ ] Helm install command executed
  - [ ] Pods transitioning to Running state
  - [ ] Pod 1 ready
  - [ ] Pod 2 ready
  - [ ] Health endpoint responding (HTTP 200)
  - [ ] Master realm accessible

**Validation Command:**
```bash
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=keycloak \
  -n keycloak-system --timeout=600s
curl -sf http://localhost:8080/realms/master
```

**Status**: ⬜ In Progress | ⬜ Complete | ⬜ Failed

- [ ] **Keycloak Realm Import**
  - [ ] Realm ConfigMap created
  - [ ] Import job created
  - [ ] Import job completed successfully
  - [ ] Netweave realm accessible
  - [ ] Groups created (platform-admins, tenant-admins, tenant-editors, tenant-viewers)
  - [ ] Client created (netweave-gateway)

**Validation Command:**
```bash
kubectl wait --for=condition=complete job/keycloak-realm-import \
  -n keycloak-system --timeout=300s
curl -sf http://localhost:8080/realms/netweave
```

**Status**: ⬜ In Progress | ⬜ Complete | ⬜ Failed

- [ ] **Vault Deployment**
  - [ ] Namespace created
  - [ ] Kustomize apply completed
  - [ ] All 3 pods Running
  - [ ] Vault initialized (keys stored securely)
  - [ ] All pods unsealed
  - [ ] Raft consensus established

**Validation Command:**
```bash
kubectl get pods -n vault-system
for pod in vault-0 vault-1 vault-2; do
  kubectl exec -n vault-system $pod -- vault status | grep -i sealed
done
```

**Status**: ⬜ In Progress | ⬜ Complete | ⬜ Failed

- [ ] **Vault PKI Initialization**
  - [ ] PKI init job created
  - [ ] Root PKI mounted at pki/
  - [ ] Intermediate PKI mounted at pki_int/
  - [ ] Role created: netweave-mtls
  - [ ] Test certificate issuance successful

**Validation Command:**
```bash
kubectl wait --for=condition=complete job/vault-pki-init \
  -n vault-system --timeout=300s
kubectl exec -n vault-system vault-0 -- vault read pki_int/roles/netweave-mtls
```

**Status**: ⬜ In Progress | ⬜ Complete | ⬜ Failed

#### Phase 1 Final Check

- [ ] **Gateway Still Operational**
  ```bash
  kubectl get pods -n netweave -l app.kubernetes.io/component=gateway
  curl -k https://o2ims.example.com/healthz
  ```
  - [ ] All gateway pods Running
  - [ ] Health check returns 200
  - [ ] mTLS authentication working

**Phase 1 Complete**: ⬜ Yes | ⬜ No - ROLLBACK REQUIRED

**Completion Time**: [TIME]
**Duration**: [MINUTES]

---

### Phase 2: Data Migration

#### Start: [TIME]

- [ ] **Pre-Migration Verification**
  - [ ] Source data count captured
    ```bash
    kubectl exec -n netweave redis-0 -- redis-cli KEYS "user:*" | wc -l
    kubectl exec -n netweave redis-0 -- redis-cli KEYS "tenant:*" | wc -l
    ```
  - [ ] Keycloak ready for import
  - [ ] Redis connection stable

- [ ] **Migration Execution**
  - [ ] Migration job created
  - [ ] Migration job running
  - [ ] Progress logs reviewed (no errors)
  - [ ] Migration job completed
  - [ ] Migration logs saved for audit

**Validation Command:**
```bash
kubectl logs -n netweave job/keycloak-data-migration -f
kubectl wait --for=condition=complete job/keycloak-data-migration \
  -n netweave --timeout=1800s
```

**Status**: ⬜ In Progress | ⬜ Complete | ⬜ Failed

- [ ] **Post-Migration Verification**
  - [ ] User count comparison:
    - Redis: [COUNT]
    - Keycloak: [COUNT]
    - Match: ⬜ Yes | ⬜ No
  - [ ] Group membership verification:
    - Platform admins: [COUNT]
    - Tenant admins: [COUNT]
    - Tenant editors: [COUNT]
    - Tenant viewers: [COUNT]
  - [ ] OAuth subject mappings created in Redis
    ```bash
    kubectl exec -n netweave redis-0 -- redis-cli KEYS "users:oauth:*" | wc -l
    ```
    - Count: [COUNT]
    - Expected: Same as user count

- [ ] **Sample User Verification**
  - [ ] Select 5 random users from Redis
  - [ ] Verify each exists in Keycloak
  - [ ] Verify group membership correct
  - [ ] Verify OAuth subject mapping exists

#### Phase 2 Final Check

- [ ] **Gateway Still Operational**
  ```bash
  curl -k https://o2ims.example.com/o2ims-infrastructureInventory/v1/api_versions \
    --cert client.crt --key client.key --cacert ca.crt
  ```
  - [ ] Health check returns 200
  - [ ] mTLS authentication working
  - [ ] No increase in error rate

**Phase 2 Complete**: ⬜ Yes | ⬜ No - ROLLBACK REQUIRED

**Completion Time**: [TIME]
**Duration**: [MINUTES]

---

### Phase 3: Gateway Update

#### Start: [TIME]

- [ ] **Pre-Update Verification**
  - [ ] Current deployment backed up
    ```bash
    kubectl get deployment netweave-gateway -n netweave -o yaml > backups/gateway-before-phase3.yaml
    ```
  - [ ] OAuth2 secrets created
  - [ ] Gateway configuration reviewed

- [ ] **Helm Upgrade Execution**
  - [ ] Helm upgrade command executed
    ```bash
    helm upgrade netweave deployments/helm/netweave \
      --namespace netweave \
      --values values-dual-auth.yaml \
      --wait --timeout=10m
    ```
  - [ ] Rollout started
  - [ ] Old pods terminating
  - [ ] New pods starting
  - [ ] New pods ready
  - [ ] Rollout complete

**Validation Command:**
```bash
kubectl rollout status deployment/netweave-gateway -n netweave -w
```

**Status**: ⬜ In Progress | ⬜ Complete | ⬜ Failed

- [ ] **Post-Update Verification**
  - [ ] Pod status:
    - Pod 1: ⬜ Ready
    - Pod 2: ⬜ Ready
    - Pod 3: ⬜ Ready
  - [ ] Configuration verification:
    ```bash
    kubectl get configmap netweave-config -n netweave -o yaml | grep oauth2
    ```
    - [ ] oauth2.enabled: true
    - [ ] oauth2.priority: false (mTLS priority)

- [ ] **Authentication Testing**
  - [ ] **Test 1: mTLS Authentication (Existing)**
    ```bash
    curl -k https://o2ims.example.com/o2ims-infrastructureInventory/v1/api_versions \
      --cert client.crt --key client.key --cacert ca.crt
    ```
    - Result: ⬜ 200 OK | ⬜ Failed

  - [ ] **Test 2: OAuth2 Authentication (New)**
    ```bash
    TOKEN=$(curl -s -X POST \
      http://keycloak.keycloak-system.svc.cluster.local:80/realms/netweave/protocol/openid-connect/token \
      -d "client_id=netweave-gateway" \
      -d "client_secret=${KEYCLOAK_CLIENT_SECRET}" \
      -d "username=test-user" \
      -d "password=test-password" \
      -d "grant_type=password" \
      | jq -r '.access_token')

    curl -k https://o2ims.example.com/o2ims-infrastructureInventory/v1/api_versions \
      -H "Authorization: Bearer ${TOKEN}"
    ```
    - Result: ⬜ 200 OK | ⬜ Failed

  - [ ] **Test 3: Priority Verification (Both Present)**
    ```bash
    curl -k https://o2ims.example.com/o2ims-infrastructureInventory/v1/api_versions \
      -H "Authorization: Bearer ${TOKEN}" \
      --cert client.crt --key client.key --cacert ca.crt
    ```
    - Result: ⬜ 200 OK | ⬜ Failed
    - Method Used: ⬜ mTLS | ⬜ OAuth2 (should be mTLS)

- [ ] **Metrics Verification**
  ```bash
  kubectl port-forward -n netweave svc/netweave-gateway 9090:9090 &
  curl -s http://localhost:9090/metrics | grep o2ims_authentication_total
  ```
  - [ ] mtls success count > 0
  - [ ] oauth2 success count > 0
  - [ ] No increase in failure count

- [ ] **Performance Check**
  - [ ] p95 latency: [VALUE]ms (target: < 105ms)
  - [ ] p99 latency: [VALUE]ms (target: < 500ms)
  - [ ] Error rate: [VALUE]% (target: < 0.1%)

#### Phase 3 Final Check

- [ ] **All Authentication Methods Working**
  - [ ] Existing mTLS clients: ⬜ Working
  - [ ] New OAuth2 clients: ⬜ Working
  - [ ] Priority correct (mTLS over OAuth2): ⬜ Verified

**Phase 3 Complete**: ⬜ Yes | ⬜ No - ROLLBACK REQUIRED

**Completion Time**: [TIME]
**Duration**: [MINUTES]

---

## Post-Cutover Validation

### Immediate Validation (0-1 Hour)

- [ ] **Service Availability**
  - [ ] Gateway health check: ⬜ Passing
  - [ ] All gateway pods ready: ⬜ 3/3
  - [ ] No pod restarts: ⬜ Verified
  - [ ] No CrashLoopBackOff: ⬜ Verified

- [ ] **Authentication Success Rate**
  ```bash
  # Monitor for 10 minutes
  watch -n 30 'kubectl port-forward -n netweave svc/netweave-gateway 9090:9090 > /dev/null 2>&1 & \
    sleep 2 && curl -s http://localhost:9090/metrics | grep o2ims_authentication_total && \
    kill %1'
  ```
  - [ ] mTLS success rate: [VALUE]% (target: > 99%)
  - [ ] OAuth2 success rate: [VALUE]% (target: > 99%)
  - [ ] No spike in authentication failures

- [ ] **API Performance**
  - [ ] p95 response time: [VALUE]ms (baseline: [BASELINE]ms, delta: [DELTA]ms)
  - [ ] p99 response time: [VALUE]ms
  - [ ] Acceptable degradation: ⬜ Yes (< 5%) | ⬜ No

- [ ] **Error Monitoring**
  ```bash
  kubectl logs -n netweave -l app.kubernetes.io/component=gateway --since=1h | grep -i "error\|fatal"
  ```
  - [ ] Error count: [COUNT]
  - [ ] All errors triaged: ⬜ Yes | ⬜ Actionable issues identified

- [ ] **Keycloak Health**
  ```bash
  kubectl get pods -n keycloak-system
  curl -sf http://keycloak.keycloak-system.svc.cluster.local:80/health
  ```
  - [ ] All Keycloak pods Running: ⬜ 2/2
  - [ ] Health endpoint: ⬜ UP
  - [ ] Database connections: ⬜ Healthy

- [ ] **Vault Health**
  ```bash
  kubectl exec -n vault-system vault-0 -- vault status
  ```
  - [ ] All Vault pods Running: ⬜ 3/3
  - [ ] All pods unsealed: ⬜ Yes
  - [ ] Raft leader elected: ⬜ Yes

- [ ] **PostgreSQL Health**
  ```bash
  kubectl exec -n keycloak-system postgresql-0 -- \
    psql -U keycloak -d keycloak -c "SELECT COUNT(*) FROM users;"
  ```
  - [ ] Database accessible: ⬜ Yes
  - [ ] User count matches: ⬜ Yes
  - [ ] Replication lag: [VALUE]ms (if replica enabled)

### Extended Validation (1-24 Hours)

- [ ] **Hour 1 Check**
  - Time: [TIME]
  - [ ] No critical alerts
  - [ ] Error rate stable
  - [ ] Performance within SLA

- [ ] **Hour 4 Check**
  - Time: [TIME]
  - [ ] No authentication issues reported
  - [ ] OAuth2 adoption starting: [COUNT] users
  - [ ] Keycloak memory usage: [VALUE]% (target: < 80%)
  - [ ] Vault memory usage: [VALUE]% (target: < 80%)

- [ ] **Hour 12 Check**
  - Time: [TIME]
  - [ ] Gateway stable overnight
  - [ ] No pod restarts
  - [ ] Backup jobs successful
  - [ ] Log volume normal

- [ ] **Hour 24 Check**
  - Time: [TIME]
  - [ ] All SLAs met
  - [ ] No rollback triggers activated
  - [ ] Client feedback: ⬜ Positive | ⬜ Issues identified
  - [ ] Ready to proceed to Phase 4: ⬜ Yes | ⬜ No

### Data Integrity Validation

- [ ] **User Data Verification**
  ```bash
  # Compare Redis and Keycloak user counts
  REDIS_COUNT=$(kubectl exec -n netweave redis-0 -- redis-cli KEYS "user:*" | wc -l)
  KC_COUNT=$(kubectl exec -n keycloak-system keycloak-0 -- \
    /opt/keycloak/bin/kcadm.sh get users -r netweave --fields id | jq length)
  ```
  - Redis users: [COUNT]
  - Keycloak users: [COUNT]
  - Match: ⬜ Yes | ⬜ No - INVESTIGATE

- [ ] **OAuth Mapping Verification**
  ```bash
  MAPPING_COUNT=$(kubectl exec -n netweave redis-0 -- redis-cli KEYS "users:oauth:*" | wc -l)
  ```
  - OAuth mappings: [COUNT]
  - Expected: Same as user count
  - Match: ⬜ Yes | ⬜ No - INVESTIGATE

- [ ] **Subscription Data Integrity**
  ```bash
  kubectl exec -n netweave redis-0 -- redis-cli KEYS "subscription:*" | wc -l
  ```
  - Subscription count: [COUNT]
  - Baseline count: [BASELINE]
  - Difference: [DELTA]
  - Acceptable: ⬜ Yes (no deletions) | ⬜ No - INVESTIGATE

- [ ] **Sample Data Verification**
  - [ ] Select 10 random users from Keycloak
  - [ ] Verify OAuth subject mapping exists for each
  - [ ] Verify tenant association correct
  - [ ] Verify role assignment correct
  - [ ] All 10 verified: ⬜ Yes | ⬜ No

---

## Sign-Off Requirements

### Deployment Team Sign-Off

**Primary Operator**: ______________________ Date: ______ Time: ______

**Verification Completed:**
- [ ] All phases completed successfully
- [ ] All validation checks passed
- [ ] No rollback triggers activated
- [ ] Deployment logs saved
- [ ] Handover to operations complete

**Secondary Operator**: ______________________ Date: ______ Time: ______

**Verification Completed:**
- [ ] Independent verification of all checks
- [ ] Monitoring configured and active
- [ ] Alert channels verified
- [ ] Runbooks accessible

### SRE Sign-Off

**SRE Engineer**: ______________________ Date: ______ Time: ______

**Verification Completed:**
- [ ] Service availability within SLA
- [ ] Performance metrics acceptable
- [ ] Monitoring and alerting operational
- [ ] Incident response procedures ready
- [ ] 24-hour stability confirmed

**Comments:**
```
[SRE notes and observations]
```

### Application Owner Sign-Off

**Application Owner**: ______________________ Date: ______ Time: ______

**Business Verification:**
- [ ] No customer-impacting issues
- [ ] Service functionality verified
- [ ] Client communication completed
- [ ] Stakeholder expectations met
- [ ] Ready to proceed to Phase 4 (Client Migration)

**Comments:**
```
[Business impact assessment and approval to proceed]
```

---

## Deployment Log

### Timeline

| Phase | Start Time | End Time | Duration | Status | Notes |
|-------|-----------|----------|----------|--------|-------|
| Phase 1: Infrastructure | [TIME] | [TIME] | [MIN] | ⬜ Pass / ⬜ Fail | |
| Phase 2: Data Migration | [TIME] | [TIME] | [MIN] | ⬜ Pass / ⬜ Fail | |
| Phase 3: Gateway Update | [TIME] | [TIME] | [MIN] | ⬜ Pass / ⬜ Fail | |

**Total Cutover Duration**: [MINUTES]

### Issues Encountered

| Issue # | Time | Description | Severity | Resolution | Status |
|---------|------|-------------|----------|------------|--------|
| 1 | | | | | ⬜ Open / ⬜ Resolved |
| 2 | | | | | ⬜ Open / ⬜ Resolved |

### Baseline vs. Current Metrics

| Metric | Baseline | Current | Delta | Acceptable? |
|--------|----------|---------|-------|-------------|
| User Count | [COUNT] | [COUNT] | [DELTA] | ⬜ Yes / ⬜ No |
| Tenant Count | [COUNT] | [COUNT] | [DELTA] | ⬜ Yes / ⬜ No |
| Gateway Pods | 3 | [COUNT] | [DELTA] | ⬜ Yes / ⬜ No |
| p95 Latency | [MS] | [MS] | [MS] | ⬜ Yes / ⬜ No |
| Error Rate | [%] | [%] | [%] | ⬜ Yes / ⬜ No |

### Lessons Learned

```
[Document any deviations from the plan, unexpected issues, or improvements for next time]

1.

2.

3.
```

---

## Emergency Contacts

### Deployment Team

- **Primary Operator**: [NAME] - [PHONE] - [EMAIL]
- **Secondary Operator**: [NAME] - [PHONE] - [EMAIL]
- **SRE On-Call**: [NAME] - [PHONE] - [EMAIL]

### Escalation

- **L2 Escalation (SRE Manager)**: [NAME] - [PHONE] - [EMAIL]
- **L3 Escalation (Engineering Manager)**: [NAME] - [PHONE] - [EMAIL]
- **L4 Escalation (CTO)**: [NAME] - [PHONE] - [EMAIL]

### Vendor Support

- **Bitnami Support**: [CONTACT INFO]
- **HashiCorp Support**: [CONTACT INFO]
- **Cloud Provider Support**: [CONTACT INFO]

---

## Related Documentation

- [Deployment Plan](deployment-plan.md) - Comprehensive deployment plan
- [Rollback Plan](rollback-plan.md) - Phase-specific rollback procedures
- [Keycloak Operations](runbooks/keycloak-operations.md) - Keycloak management
- [Vault Operations](runbooks/vault-operations.md) - Vault management

---

**Document Version**: 1.0
**Last Updated**: 2026-02-06
**Maintained By**: NetWeave Operations Team
