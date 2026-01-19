# Local Development Setup with Vault and Keycloak

This guide helps you set up a local development environment with Vault and Keycloak using Docker.

## Prerequisites

- Docker Desktop installed and running
- `jq` installed (`brew install jq` on macOS)
- `curl` installed
- Ports 5432, 6379, 8080, 8200, 8443 available

## Quick Start

### 1. Start Services

```bash
# Start all services
docker-compose -f docker-compose.dev.yml up -d

# Check services are running
docker-compose -f docker-compose.dev.yml ps
```

### 2. Initialize Vault PKI

```bash
# Run Vault initialization script
./deployments/vault/init-vault.sh
```

This script will:
- Create root and intermediate CAs
- Configure PKI roles for client and server certificates
- Generate test certificates
- Create Vault policies for gateway and Keycloak

### 3. Verify Keycloak

```bash
# Wait for Keycloak to be ready (takes ~60 seconds)
until curl -sf http://localhost:8080/health/ready > /dev/null 2>&1; do
  echo "Waiting for Keycloak..."
  sleep 5
done

echo "Keycloak is ready!"
```

### 4. Access Web UIs

**Keycloak Admin Console:**
- URL: http://localhost:8080
- Username: `admin`
- Password: `admin`
- Realm: `o2ims`

**Vault UI:**
- URL: http://localhost:8200/ui
- Token: `root`

**PostgreSQL:**
- Host: `localhost:5432`
- Database: `keycloak`
- Username: `keycloak`
- Password: `keycloak_dev_password`

**Redis:**
- Host: `localhost:6379`
- Password: `redis_dev_password`

## Test Users

The Keycloak realm includes test users:

| Username | Password | Role | Tenant |
|----------|----------|------|--------|
| admin | admin | platform-admin | platform |
| operator | operator | operator | tenant-test |
| viewer | viewer | viewer | tenant-test |

## Test Certificates

After running the Vault init script, test certificates are available:

```bash
# Client certificate for mTLS testing
/tmp/test-client.crt
/tmp/test-client.key

# Server certificate for gateway
/tmp/test-server.crt
/tmp/test-server.key

# CA chain
/tmp/ca-chain.crt
/tmp/root_ca.crt
```

## Testing Authentication

### Test OAuth2 Token Flow

```bash
# Get access token
TOKEN=$(curl -s -X POST 'http://localhost:8080/realms/o2ims/protocol/openid-connect/token' \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  -d 'grant_type=password' \
  -d 'client_id=o2ims-gateway' \
  -d 'client_secret=o2ims-gateway-secret-change-in-production' \
  -d 'username=operator' \
  -d 'password=operator' \
  | jq -r '.access_token')

echo "Access Token: $TOKEN"

# Decode token to see claims
echo $TOKEN | cut -d. -f2 | base64 -d 2>/dev/null | jq .
```

### Test Certificate Issuance

```bash
# Set Vault address and token
export VAULT_ADDR='http://localhost:8200'
export VAULT_TOKEN='root'

# Issue a new client certificate
vault write -format=json pki_int/issue/o2ims-client \
  common_name="alice.o2ims.local" \
  ttl=720h \
  | jq -r '.data | {certificate, private_key, ca_chain}'
```

### Test mTLS Connection

```bash
# Test mTLS with curl (requires gateway running)
curl --cert /tmp/test-client.crt \
     --key /tmp/test-client.key \
     --cacert /tmp/ca-chain.crt \
     https://localhost:8081/o2ims-infrastructureInventory/v1/api_versions
```

## Keycloak Configuration

### View Realm Configuration

1. Go to http://localhost:8080
2. Login with `admin` / `admin`
3. Select `o2ims` realm from dropdown
4. Explore:
   - **Users**: Pre-configured test users
   - **Clients**: `o2ims-gateway` and `admin-portal`
   - **Roles**: Realm and client roles
   - **Client Scopes**: Token mappers (tenant, roles, cert_subject)

### Create New User

```bash
# Via Keycloak Admin UI
# 1. Go to Users → Add User
# 2. Fill in username, email, first/last name
# 3. Go to Credentials tab → Set Password
# 4. Go to Attributes tab → Add tenantId attribute
# 5. Go to Role Mappings → Assign client roles
```

## Vault PKI Configuration

### View PKI Configuration

```bash
export VAULT_ADDR='http://localhost:8200'
export VAULT_TOKEN='root'

# List PKI mounts
vault secrets list

# Read root CA
vault read pki/cert/ca

# Read intermediate CA
vault read pki_int/cert/ca

# List roles
vault list pki_int/roles

# Read client role configuration
vault read pki_int/roles/o2ims-client
```

### Issue Certificate Manually

```bash
# Issue certificate for specific user
vault write pki_int/issue/o2ims-client \
  common_name="bob@acme.com" \
  alt_names="bob.o2ims.local" \
  ttl=8760h \
  format=pem
```

### Revoke Certificate

```bash
# Get certificate serial number
SERIAL=$(vault write -format=json pki_int/issue/o2ims-client \
  common_name="temp.o2ims.local" ttl=1h \
  | jq -r '.data.serial_number')

# Revoke certificate
vault write pki_int/revoke serial_number=$SERIAL
```

## Troubleshooting

### Services won't start

```bash
# Check logs
docker-compose -f docker-compose.dev.yml logs keycloak
docker-compose -f docker-compose.dev.yml logs vault
docker-compose -f docker-compose.dev.yml logs postgres

# Restart services
docker-compose -f docker-compose.dev.yml restart
```

### Keycloak not importing realm

```bash
# Check if realm file exists
ls -la deployments/keycloak/realm-import/

# Manually import realm
docker exec -it o2ims-keycloak \
  /opt/keycloak/bin/kc.sh import \
  --file /opt/keycloak/data/import/o2ims-realm.json
```

### Vault init script fails

```bash
# Check Vault is running
curl http://localhost:8200/v1/sys/health

# Check Vault logs
docker-compose -f docker-compose.dev.yml logs vault

# Re-run init script
./deployments/vault/init-vault.sh
```

### Reset Everything

```bash
# Stop and remove all containers and volumes
docker-compose -f docker-compose.dev.yml down -v

# Remove generated certificates
rm -f /tmp/test-client.* /tmp/test-server.* /tmp/*ca*.crt /tmp/pki_*

# Start fresh
docker-compose -f docker-compose.dev.yml up -d
./deployments/vault/init-vault.sh
```

## Next Steps

1. **Issue #270**: Create Keycloak integration package in `internal/keycloak/`
2. **Issue #271**: Create Vault integration package in `internal/vault/`
3. **Issue #272**: Update authentication middleware to support both mTLS and OAuth2

## Useful Commands

```bash
# View Keycloak logs
docker-compose -f docker-compose.dev.yml logs -f keycloak

# View Vault logs
docker-compose -f docker-compose.dev.yml logs -f vault

# Access Vault CLI
docker exec -it o2ims-vault vault status

# Access PostgreSQL
docker exec -it o2ims-postgres psql -U keycloak -d keycloak

# Stop all services
docker-compose -f docker-compose.dev.yml down

# Stop and remove volumes (full reset)
docker-compose -f docker-compose.dev.yml down -v
```

## Architecture

```
┌─────────────────────────────────────────────┐
│          Docker Local Environment            │
├─────────────────────────────────────────────┤
│                                             │
│  PostgreSQL :5432                           │
│  └─ Keycloak database                       │
│                                             │
│  Keycloak :8080, :8443                      │
│  ├─ Admin UI                                │
│  ├─ Realm: o2ims                            │
│  ├─ Users: admin, operator, viewer          │
│  └─ Clients: o2ims-gateway, admin-portal    │
│                                             │
│  Vault :8200                                │
│  ├─ PKI Root CA (pki/)                      │
│  ├─ PKI Intermediate CA (pki_int/)          │
│  ├─ Roles: o2ims-client, o2ims-server       │
│  └─ Policies: gateway, keycloak             │
│                                             │
│  Redis :6379                                │
│  └─ Existing auth data (for migration)      │
│                                             │
└─────────────────────────────────────────────┘
```

## Security Notes

⚠️ **This is a development environment only!**

- Default credentials are used (change in production)
- TLS is disabled for Keycloak (enable in production)
- Vault is in dev mode (use production mode with proper storage)
- No network policies or firewalls (add in production)
- Secrets are hardcoded (use secret management in production)
