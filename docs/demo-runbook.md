# Netweave Demo Runbook

Complete step-by-step guide to deploy Netweave from scratch and demonstrate multi-tenant O-RAN O2-IMS functionality.

## Prerequisites

- Docker Desktop with Kubernetes enabled (3-node cluster)
- `kubectl`, `helm`, `go 1.25+`, `docker` CLI tools installed
- NGINX Ingress Controller deployed with SSL passthrough enabled
- `/etc/hosts` entries configured

### Install NGINX Ingress Controller

```bash
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.12.0/deploy/static/provider/cloud/deploy.yaml
```

Enable SSL passthrough (required for mTLS):

```bash
kubectl -n ingress-nginx patch deployment ingress-nginx-controller --type=json \
  -p='[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--enable-ssl-passthrough"}]'
```

Wait for the controller to be ready:

```bash
kubectl -n ingress-nginx wait --for=condition=available deployment/ingress-nginx-controller --timeout=60s
```

### Configure /etc/hosts

```bash
sudo bash -c 'echo "127.0.0.1 admin.netweave.local api.netweave.local auth.netweave.local" >> /etc/hosts'
```

---

## Phase 1: Build Everything

### 1.1 Build the CLI

```bash
cd /Users/pascal/Documents/git/netweave
go build -o build/netweave-cli ./cmd/cli/
```

### 1.2 Build the Gateway Docker Image

```bash
docker build -t netweave-gateway:local .
```

### 1.3 Build the Admin Portal Docker Image

```bash
docker build -t netweave-admin-portal:local web/admin-portal/
```

### 1.4 Load Images into Kubernetes Nodes

Docker Desktop K8s uses containerd inside the VM nodes. Load images into each worker:

```bash
for NODE in desktop-worker desktop-worker2; do
  docker save netweave-gateway:local | docker exec -i $NODE ctr -n k8s.io images import --all-platforms -
  docker save netweave-admin-portal:local | docker exec -i $NODE ctr -n k8s.io images import --all-platforms -
done
```

---

## Phase 2: Deploy the Stack

The CLI `setup all` command orchestrates the full deployment in 4 phases:
1. Vault deployment + initialization + unseal + PKI engine setup
2. Certificate issuance (server, client, ingress TLS)
3. Helm chart installation (PostgreSQL, Redis, Keycloak, Gateway, Admin Portal)
4. Keycloak bootstrap (realm, user profile, tenant, roles, admin user with password)

### 2.1 Deploy Everything

```bash
build/netweave-cli setup all \
  --chart-path deployments/helm/netweave \
  --values-file deployments/helm/netweave/values-local.yaml \
  -v
```

### 2.2 Verify Deployment

```bash
kubectl get pods -n netweave
```

Expected output: All pods `Running` or `Completed`.

```bash
kubectl get svc -n netweave
```

### 2.3 Verify Health

```bash
curl -ks https://api.netweave.local/healthz \
  --cert ~/.netweave/client.crt \
  --key ~/.netweave/client.key \
  --cacert ~/.netweave/ca.crt | jq .
```

---

## Phase 3: Get Admin Access

### 3.1 Obtain an OAuth2 Token

The admin user was created during bootstrap with:
- Email: `admin@netweave.local`
- Password: `admin`
- Cert CN: `admin.netweave.local`

Get a Bearer token from Keycloak:

```bash
# Get the gateway client secret from K8s
CLIENT_SECRET=$(kubectl get secret -n netweave netweave-keycloak-client \
  -o jsonpath='{.data.client-secret}' | base64 -d)

echo "Gateway client secret: $CLIENT_SECRET"

# Exchange credentials for a token
TOKEN=$(curl -ks https://auth.netweave.local/realms/netweave/protocol/openid-connect/token \
  -d "grant_type=password" \
  -d "client_id=netweave-gateway" \
  -d "client_secret=$CLIENT_SECRET" \
  -d "username=admin@netweave.local" \
  -d "password=admin" | jq -r '.access_token')

echo "Admin token obtained (${#TOKEN} chars)"
```

### 3.2 Verify Admin Access

Test the admin API with mTLS + Bearer token:

```bash
curl -ks https://api.netweave.local/admin/tenant/me \
  --cert ~/.netweave/client.crt \
  --key ~/.netweave/client.key \
  --cacert ~/.netweave/ca.crt \
  -H "Authorization: Bearer $TOKEN" | jq .
```

---

## Phase 4: Enable the O2-IMS Interface

The O2-IMS plugin is disabled by default. Enable it via the admin API:

```bash
curl -ks -X PUT https://api.netweave.local/admin/platform/plugins/o2ims \
  --cert ~/.netweave/client.crt \
  --key ~/.netweave/client.key \
  --cacert ~/.netweave/ca.crt \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"enabled": true}' | jq .
```

Verify it's enabled:

```bash
curl -ks https://api.netweave.local/admin/platform/plugins \
  --cert ~/.netweave/client.crt \
  --key ~/.netweave/client.key \
  --cacert ~/.netweave/ca.crt \
  -H "Authorization: Bearer $TOKEN" | jq .
```

---

## Phase 5: Create Two Mock IMS Backends

### 5.1 Create Backend Alpha

```bash
ALPHA_ID=$(curl -ks -X POST https://api.netweave.local/admin/infrastructure/backends \
  --cert ~/.netweave/client.crt \
  --key ~/.netweave/client.key \
  --cacert ~/.netweave/ca.crt \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Mock IMS Alpha",
    "category": "ims",
    "adapterType": "mock",
    "description": "Simulated O-Cloud infrastructure for Acme Telecom",
    "config": {
      "populate_sample_data": "true",
      "ocloud_id": "ocloud-alpha-001"
    }
  }' | jq -r '.id')

echo "Alpha Backend ID: $ALPHA_ID"
```

### 5.2 Create Backend Beta

```bash
BETA_ID=$(curl -ks -X POST https://api.netweave.local/admin/infrastructure/backends \
  --cert ~/.netweave/client.crt \
  --key ~/.netweave/client.key \
  --cacert ~/.netweave/ca.crt \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Mock IMS Beta",
    "category": "ims",
    "adapterType": "mock",
    "description": "Simulated O-Cloud infrastructure for GlobalNet Corp",
    "config": {
      "populate_sample_data": "true",
      "ocloud_id": "ocloud-beta-002"
    }
  }' | jq -r '.id')

echo "Beta Backend ID: $BETA_ID"
```

### 5.3 Verify Backends

```bash
curl -ks https://api.netweave.local/admin/infrastructure/backends \
  --cert ~/.netweave/client.crt \
  --key ~/.netweave/client.key \
  --cacert ~/.netweave/ca.crt \
  -H "Authorization: Bearer $TOKEN" | jq '.backends[] | {id, name, category, adapterType, status}'
```

---

## Phase 6: Create Two Tenants

### 6.1 Create Acme Telecom

```bash
ACME_ID=$(curl -ks -X POST https://api.netweave.local/admin/platform/tenants \
  --cert ~/.netweave/client.crt \
  --key ~/.netweave/client.key \
  --cacert ~/.netweave/ca.crt \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Acme Telecom",
    "description": "Mobile network operator - US West Coast",
    "contactEmail": "admin@acme-telecom.example.com",
    "quota": {
      "maxSubscriptions": 100,
      "maxResourcePools": 50,
      "maxDeployments": 100,
      "maxUsers": 20,
      "maxRequestsPerMinute": 500
    }
  }' | jq -r '.id')

echo "Acme Tenant ID: $ACME_ID"
```

### 6.2 Create GlobalNet Corp

```bash
GLOBALNET_ID=$(curl -ks -X POST https://api.netweave.local/admin/platform/tenants \
  --cert ~/.netweave/client.crt \
  --key ~/.netweave/client.key \
  --cacert ~/.netweave/ca.crt \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "GlobalNet Corp",
    "description": "Enterprise network provider - EU region",
    "contactEmail": "admin@globalnet.example.com",
    "quota": {
      "maxSubscriptions": 100,
      "maxResourcePools": 50,
      "maxDeployments": 100,
      "maxUsers": 20,
      "maxRequestsPerMinute": 500
    }
  }' | jq -r '.id')

echo "GlobalNet Tenant ID: $GLOBALNET_ID"
```

### 6.3 Verify Tenants

```bash
curl -ks https://api.netweave.local/admin/platform/tenants \
  --cert ~/.netweave/client.crt \
  --key ~/.netweave/client.key \
  --cacert ~/.netweave/ca.crt \
  -H "Authorization: Bearer $TOKEN" | jq '.tenants[] | {id, name, status}'
```

---

## Phase 7: Assign Backends to Tenants

### 7.1 Map Alpha Backend to Acme Telecom

```bash
curl -ks -X POST "https://api.netweave.local/admin/infrastructure/tenants/${ACME_ID}/backend-access" \
  --cert ~/.netweave/client.crt \
  --key ~/.netweave/client.key \
  --cacert ~/.netweave/ca.crt \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"backendId\": \"$ALPHA_ID\",
    \"permissions\": [\"read\", \"subscribe\"]
  }" | jq .
```

### 7.2 Map Beta Backend to GlobalNet Corp

```bash
curl -ks -X POST "https://api.netweave.local/admin/infrastructure/tenants/${GLOBALNET_ID}/backend-access" \
  --cert ~/.netweave/client.crt \
  --key ~/.netweave/client.key \
  --cacert ~/.netweave/ca.crt \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"backendId\": \"$BETA_ID\",
    \"permissions\": [\"read\", \"subscribe\"]
  }" | jq .
```

---

## Phase 8: Create Operator Users

### 8.1 Get the Operator Role ID

```bash
OPERATOR_ROLE_ID=$(curl -ks https://api.netweave.local/admin/tenant/roles \
  --cert ~/.netweave/client.crt \
  --key ~/.netweave/client.key \
  --cacert ~/.netweave/ca.crt \
  -H "Authorization: Bearer $TOKEN" | jq -r '.roles[] | select(.name=="tenant-admin") | .id')

echo "Operator Role ID: $OPERATOR_ROLE_ID"
```

### 8.2 Create Acme Operator User

```bash
curl -ks -X POST "https://api.netweave.local/admin/platform/tenants/${ACME_ID}/users" \
  --cert ~/.netweave/client.crt \
  --key ~/.netweave/client.key \
  --cacert ~/.netweave/ca.crt \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"subject\": \"CN=acme-operator.netweave.local,O=Netweave\",
    \"commonName\": \"acme-operator.netweave.local\",
    \"email\": \"operator@acme-telecom.example.com\",
    \"roleId\": \"$OPERATOR_ROLE_ID\"
  }" | jq .
```

### 8.3 Create GlobalNet Operator User

```bash
curl -ks -X POST "https://api.netweave.local/admin/platform/tenants/${GLOBALNET_ID}/users" \
  --cert ~/.netweave/client.crt \
  --key ~/.netweave/client.key \
  --cacert ~/.netweave/ca.crt \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"subject\": \"CN=globalnet-operator.netweave.local,O=Netweave\",
    \"commonName\": \"globalnet-operator.netweave.local\",
    \"email\": \"operator@globalnet.example.com\",
    \"roleId\": \"$OPERATOR_ROLE_ID\"
  }" | jq .
```

---

## Phase 9: Issue mTLS Certificates for Operators

### 9.1 Issue Acme Operator Certificate

```bash
build/netweave-cli certs issue \
  --cn acme-operator.netweave.local \
  --type client \
  --out ~/.netweave/test-users/acme-operator
```

### 9.2 Issue GlobalNet Operator Certificate

```bash
build/netweave-cli certs issue \
  --cn globalnet-operator.netweave.local \
  --type client \
  --out ~/.netweave/test-users/globalnet-operator
```

### 9.3 Verify Certificates

```bash
build/netweave-cli certs verify --cert ~/.netweave/test-users/acme-operator/client.crt
build/netweave-cli certs verify --cert ~/.netweave/test-users/globalnet-operator/client.crt
```

---

## Phase 10: Demonstrate Tenant Isolation via O-RAN API

This is the key demo: each operator sees only their own O-Cloud data.

### 10.1 Acme Telecom Views Their O-Cloud

**Deployment Managers:**

```bash
echo "=== Acme Telecom - Deployment Managers ==="
curl -ks https://api.netweave.local/o2ims-infrastructureInventory/v1/deploymentManagers \
  --cert ~/.netweave/test-users/acme-operator/client.crt \
  --key ~/.netweave/test-users/acme-operator/client.key \
  --cacert ~/.netweave/ca.crt | jq '.deploymentManagers[] | {deploymentManagerId, name}'
```

**Resource Pools:**

```bash
echo "=== Acme Telecom - Resource Pools ==="
curl -ks https://api.netweave.local/o2ims-infrastructureInventory/v1/resourcePools \
  --cert ~/.netweave/test-users/acme-operator/client.crt \
  --key ~/.netweave/test-users/acme-operator/client.key \
  --cacert ~/.netweave/ca.crt | jq '.resourcePools[] | {resourcePoolId, name, location}'
```

**Resource Types:**

```bash
echo "=== Acme Telecom - Resource Types ==="
curl -ks https://api.netweave.local/o2ims-infrastructureInventory/v1/resourceTypes \
  --cert ~/.netweave/test-users/acme-operator/client.crt \
  --key ~/.netweave/test-users/acme-operator/client.key \
  --cacert ~/.netweave/ca.crt | jq '.resourceTypes[] | {resourceTypeId, name, vendor}'
```

**Resources:**

```bash
echo "=== Acme Telecom - Resources ==="
curl -ks https://api.netweave.local/o2ims-infrastructureInventory/v1/resources \
  --cert ~/.netweave/test-users/acme-operator/client.crt \
  --key ~/.netweave/test-users/acme-operator/client.key \
  --cacert ~/.netweave/ca.crt | jq '{total, resources: [.resources[] | {resourceId, description}]}'
```

### 10.2 GlobalNet Corp Views Their O-Cloud

**Deployment Managers:**

```bash
echo "=== GlobalNet Corp - Deployment Managers ==="
curl -ks https://api.netweave.local/o2ims-infrastructureInventory/v1/deploymentManagers \
  --cert ~/.netweave/test-users/globalnet-operator/client.crt \
  --key ~/.netweave/test-users/globalnet-operator/client.key \
  --cacert ~/.netweave/ca.crt | jq '.deploymentManagers[] | {deploymentManagerId, name}'
```

**Resource Pools:**

```bash
echo "=== GlobalNet Corp - Resource Pools ==="
curl -ks https://api.netweave.local/o2ims-infrastructureInventory/v1/resourcePools \
  --cert ~/.netweave/test-users/globalnet-operator/client.crt \
  --key ~/.netweave/test-users/globalnet-operator/client.key \
  --cacert ~/.netweave/ca.crt | jq '.resourcePools[] | {resourcePoolId, name, location}'
```

**Resource Types:**

```bash
echo "=== GlobalNet Corp - Resource Types ==="
curl -ks https://api.netweave.local/o2ims-infrastructureInventory/v1/resourceTypes \
  --cert ~/.netweave/test-users/globalnet-operator/client.crt \
  --key ~/.netweave/test-users/globalnet-operator/client.key \
  --cacert ~/.netweave/ca.crt | jq '.resourceTypes[] | {resourceTypeId, name, vendor}'
```

**Resources:**

```bash
echo "=== GlobalNet Corp - Resources ==="
curl -ks https://api.netweave.local/o2ims-infrastructureInventory/v1/resources \
  --cert ~/.netweave/test-users/globalnet-operator/client.crt \
  --key ~/.netweave/test-users/globalnet-operator/client.key \
  --cacert ~/.netweave/ca.crt | jq '{total, resources: [.resources[] | {resourceId, description}]}'
```

### 10.3 Prove Cross-Tenant Isolation

Get a resource pool ID from Acme's view, then try to access it with GlobalNet's cert:

```bash
# Get Acme's first resource pool ID
ACME_POOL=$(curl -ks https://api.netweave.local/o2ims-infrastructureInventory/v1/resourcePools \
  --cert ~/.netweave/test-users/acme-operator/client.crt \
  --key ~/.netweave/test-users/acme-operator/client.key \
  --cacert ~/.netweave/ca.crt | jq -r '.resourcePools[0].resourcePoolId')

echo "Acme's pool: $ACME_POOL"

# GlobalNet tries to access Acme's pool - should return empty/not found
echo "=== Cross-tenant access attempt ==="
curl -ks "https://api.netweave.local/o2ims-infrastructureInventory/v1/resourcePools/$ACME_POOL" \
  --cert ~/.netweave/test-users/globalnet-operator/client.crt \
  --key ~/.netweave/test-users/globalnet-operator/client.key \
  --cacert ~/.netweave/ca.crt | jq .
```

The cross-tenant request should return a `404 Not Found` or empty result, proving tenant isolation.

---

## Phase 11: Access the Admin Portal (Web UI)

### 11.1 Import the CA Certificate into Your Browser

The admin portal uses HTTPS with our Vault-issued certificates. Import the CA cert to avoid browser warnings:

```bash
# macOS: Import CA cert into system keychain
sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain ~/.netweave/ca.crt
```

### 11.2 Open the Admin Portal

Navigate to: **https://admin.netweave.local**

Login with:
- Email: `admin@netweave.local`
- Password: `admin`

The portal authenticates via Keycloak OIDC and provides a web UI for managing tenants, users, roles, backends, and monitoring the platform.

---

## Teardown

To completely remove the deployment:

```bash
build/netweave-cli setup teardown --force -v
```

This removes:
- Helm release (all pods, services, configmaps)
- Vault deployment and data
- PersistentVolumeClaims
- Namespace

---

## Quick Reference

### Hostnames

| Service | URL | Auth |
|---------|-----|------|
| Gateway API | `https://api.netweave.local` | mTLS + OAuth2 |
| Admin Portal | `https://admin.netweave.local` | OAuth2/OIDC |
| Keycloak | `https://auth.netweave.local` | Username/Password |

### Default Credentials

| Service | Username | Password |
|---------|----------|----------|
| Admin Portal | `admin@netweave.local` | `admin` |
| Keycloak Admin | `admin` | `admin` |

### Key File Locations

| File | Path |
|------|------|
| CLI binary | `build/netweave-cli` |
| Admin client cert | `~/.netweave/client.crt` |
| Admin client key | `~/.netweave/client.key` |
| CA certificate | `~/.netweave/ca.crt` |
| Vault credentials | `~/.netweave/credentials.json` |
| Acme operator cert | `~/.netweave/test-users/acme-operator/client.crt` |
| GlobalNet operator cert | `~/.netweave/test-users/globalnet-operator/client.crt` |

### CLI Commands

| Command | Description |
|---------|-------------|
| `setup all` | Full stack deployment |
| `setup vault` | Deploy and configure Vault |
| `setup certs` | Issue TLS certificates |
| `setup helm` | Install Helm chart |
| `setup keycloak` | Bootstrap Keycloak |
| `setup teardown --force` | Remove everything |
| `certs issue --cn NAME --type client --out DIR` | Issue a client certificate |
| `certs verify --cert FILE` | Verify a certificate |

### Admin API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/admin/platform/tenants` | GET/POST | List/create tenants |
| `/admin/platform/tenants/:id` | GET/PUT/DELETE | Manage tenant |
| `/admin/platform/tenants/:id/users` | GET/POST | Manage tenant users |
| `/admin/platform/plugins` | GET | List plugins |
| `/admin/platform/plugins/:name` | PUT | Enable/disable plugin |
| `/admin/infrastructure/backends` | GET/POST | List/create backends |
| `/admin/infrastructure/backends/:id` | GET/PUT/DELETE | Manage backend |
| `/admin/infrastructure/tenants/:tid/backend-access` | GET/POST | Manage tenant backend access |
| `/admin/tenant/me` | GET | Current user info |
| `/admin/tenant/roles` | GET | List roles |

### O-RAN O2-IMS API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/o2ims-infrastructureInventory/v1/deploymentManagers` | GET | List deployment managers |
| `/o2ims-infrastructureInventory/v1/resourcePools` | GET | List resource pools |
| `/o2ims-infrastructureInventory/v1/resourceTypes` | GET | List resource types |
| `/o2ims-infrastructureInventory/v1/resources` | GET | List resources |
| `/o2ims-infrastructureInventory/v1/subscriptions` | GET/POST | List/create subscriptions |
| `/o2ims-infrastructureInventory/v1/oCloudInfrastructure` | GET | O-Cloud info |
