#!/bin/sh
set -e  # Exit on any error
set -u  # Exit on undefined variable
set -o pipefail  # Exit on pipe failures

echo "Initializing Vault..."

# Check if already initialized
if [ -f "/vault/init/initialized" ]; then
  echo "Vault already initialized"
  exit 0
fi

# Initialize Vault
echo "Running vault operator init..."
if ! vault operator init -key-shares=5 -key-threshold=3 -format=json > /vault/init/keys.json; then
  echo "ERROR: Failed to initialize Vault" >&2
  exit 1
fi

# Validate keys file was created
if [ ! -s "/vault/init/keys.json" ]; then
  echo "ERROR: keys.json is empty or not created" >&2
  exit 1
fi

# Extract unseal keys using jq
echo "Extracting unseal keys..."
KEY0=$(jq -r '.unseal_keys_b64[0]' /vault/init/keys.json)
KEY1=$(jq -r '.unseal_keys_b64[1]' /vault/init/keys.json)
KEY2=$(jq -r '.unseal_keys_b64[2]' /vault/init/keys.json)
ROOT_TOKEN=$(jq -r '.root_token' /vault/init/keys.json)

# Validate extraction
if [ -z "$KEY0" ] || [ "$KEY0" = "null" ]; then
  echo "ERROR: Failed to extract unseal key 0" >&2
  exit 1
fi
if [ -z "$KEY1" ] || [ "$KEY1" = "null" ]; then
  echo "ERROR: Failed to extract unseal key 1" >&2
  exit 1
fi
if [ -z "$KEY2" ] || [ "$KEY2" = "null" ]; then
  echo "ERROR: Failed to extract unseal key 2" >&2
  exit 1
fi
if [ -z "$ROOT_TOKEN" ] || [ "$ROOT_TOKEN" = "null" ]; then
  echo "ERROR: Failed to extract root token" >&2
  exit 1
fi

echo "Unsealing Vault..."

# Unseal Vault with validation
if ! vault operator unseal "$KEY0"; then
  echo "ERROR: Failed to apply unseal key 0" >&2
  exit 1
fi

if ! vault operator unseal "$KEY1"; then
  echo "ERROR: Failed to apply unseal key 1" >&2
  exit 1
fi

if ! vault operator unseal "$KEY2"; then
  echo "ERROR: Failed to apply unseal key 2" >&2
  exit 1
fi

# Verify unsealed
if ! vault status | grep -q "Sealed.*false"; then
  echo "ERROR: Vault remains sealed after unseal attempts" >&2
  exit 1
fi

echo "Vault unsealed successfully"

# Login and configure PKI
export VAULT_TOKEN="$ROOT_TOKEN"

echo "Configuring PKI..."

# Enable PKI (idempotent)
if ! vault secrets enable -path=pki pki 2>/dev/null; then
  if ! vault secrets list | grep -q "^pki/"; then
    echo "ERROR: Failed to enable PKI secrets engine" >&2
    exit 1
  fi
  echo "PKI already enabled"
fi

if ! vault secrets tune -max-lease-ttl=87600h pki; then
  echo "ERROR: Failed to tune PKI secrets engine" >&2
  exit 1
fi

# Generate root CA
echo "Generating root CA..."
if ! vault write pki/root/generate/internal \
  common_name="NetWeave Root CA" \
  issuer_name="netweave-root" \
  ttl=87600h > /dev/null; then
  echo "ERROR: Failed to generate root CA" >&2
  exit 1
fi

if ! vault write pki/config/urls \
  issuing_certificates="https://vault:8200/v1/pki/ca" \
  crl_distribution_points="https://vault:8200/v1/pki/crl" > /dev/null; then
  echo "ERROR: Failed to configure PKI URLs" >&2
  exit 1
fi

# Enable intermediate PKI (idempotent)
echo "Configuring intermediate PKI..."
if ! vault secrets enable -path=pki_int pki 2>/dev/null; then
  if ! vault secrets list | grep -q "^pki_int/"; then
    echo "ERROR: Failed to enable intermediate PKI secrets engine" >&2
    exit 1
  fi
  echo "Intermediate PKI already enabled"
fi

if ! vault secrets tune -max-lease-ttl=43800h pki_int; then
  echo "ERROR: Failed to tune intermediate PKI secrets engine" >&2
  exit 1
fi

# Generate intermediate CSR
echo "Generating intermediate CA..."
if ! vault write -field=csr pki_int/intermediate/generate/internal \
  common_name="NetWeave Intermediate CA" \
  issuer_name="netweave-intermediate" > /tmp/pki_intermediate.csr; then
  echo "ERROR: Failed to generate intermediate CSR" >&2
  exit 1
fi

# Validate CSR was created
if [ ! -s "/tmp/pki_intermediate.csr" ]; then
  echo "ERROR: Intermediate CSR is empty or not created" >&2
  exit 1
fi

# Sign intermediate certificate
if ! vault write -field=certificate pki/root/sign-intermediate \
  issuer_ref="netweave-root" \
  csr=@/tmp/pki_intermediate.csr \
  format=pem_bundle \
  ttl=43800h > /tmp/intermediate.cert.pem; then
  echo "ERROR: Failed to sign intermediate certificate" >&2
  exit 1
fi

# Validate certificate was created
if [ ! -s "/tmp/intermediate.cert.pem" ]; then
  echo "ERROR: Intermediate certificate is empty or not created" >&2
  exit 1
fi

if ! vault write pki_int/intermediate/set-signed \
  certificate=@/tmp/intermediate.cert.pem > /dev/null; then
  echo "ERROR: Failed to set signed intermediate certificate" >&2
  exit 1
fi

if ! vault write pki_int/config/urls \
  issuing_certificates="https://vault:8200/v1/pki_int/ca" \
  crl_distribution_points="https://vault:8200/v1/pki_int/crl" > /dev/null; then
  echo "ERROR: Failed to configure intermediate PKI URLs" >&2
  exit 1
fi

# Create roles
echo "Creating certificate roles..."
if ! vault write pki_int/roles/netweave-client \
  issuer_ref="netweave-intermediate" \
  allowed_domains="netweave.local,*.netweave.local,localhost" \
  allow_subdomains=true \
  allow_bare_domains=true \
  allow_localhost=true \
  allow_ip_sans=true \
  max_ttl=8760h \
  key_type=rsa \
  key_bits=2048 > /dev/null; then
  echo "ERROR: Failed to create netweave-client role" >&2
  exit 1
fi

if ! vault write pki_int/roles/netweave-server \
  issuer_ref="netweave-intermediate" \
  allowed_domains="netweave.local,*.netweave.local,localhost" \
  allow_subdomains=true \
  allow_bare_domains=true \
  allow_localhost=true \
  allow_ip_sans=true \
  server_flag=true \
  client_flag=false \
  max_ttl=8760h \
  key_type=rsa \
  key_bits=2048 > /dev/null; then
  echo "ERROR: Failed to create netweave-server role" >&2
  exit 1
fi

# Create policies
echo "Creating access policies..."
if ! vault policy write gateway - <<EOF
path "pki_int/cert/ca" { capabilities = ["read"] }
path "pki_int/issue/netweave-client" { capabilities = ["create", "update"] }
path "pki_int/issue/netweave-server" { capabilities = ["create", "update"] }
path "pki_int/revoke" { capabilities = ["create", "update"] }
EOF
then
  echo "ERROR: Failed to create gateway policy" >&2
  exit 1
fi

if ! vault policy write keycloak - <<EOF
path "pki_int/issue/netweave-client" { capabilities = ["create", "update"] }
path "pki_int/cert/ca" { capabilities = ["read"] }
EOF
then
  echo "ERROR: Failed to create keycloak policy" >&2
  exit 1
fi

# Mark as initialized
touch /vault/init/initialized

echo "================================"
echo "Vault initialization complete!"
echo "================================"
echo ""
echo "IMPORTANT: Unseal keys and root token stored in /vault/init/keys.json"
echo "WARNING: Protect this file - it contains sensitive credentials"
echo ""
echo "For production deployment:"
echo "  1. Extract unseal keys and root token immediately"
echo "  2. Store in external HSM/KMS (AWS KMS, Azure Key Vault, GCP Cloud KMS)"
echo "  3. Delete the keys.json file"
echo "  4. Implement auto-unseal using cloud KMS"
echo "  5. Enable audit logging"
