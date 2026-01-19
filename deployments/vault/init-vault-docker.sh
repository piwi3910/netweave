#!/bin/bash
# Script to initialize Vault PKI inside Docker container
set -e

echo "Waiting for Vault to be ready..."
until docker exec netweave-vault vault status > /dev/null 2>&1; do
  sleep 2
done
echo "Vault is ready!"

echo ""
echo "=========================================="
echo "Setting up Vault PKI for NetWeave Gateway"
echo "=========================================="
echo ""

# Enable PKI secrets engine for root CA
echo "1. Enabling PKI secrets engine at pki/..."
docker exec -e VAULT_TOKEN=root netweave-vault vault secrets enable -path=pki pki

# Tune max lease TTL to 10 years for root CA
echo "2. Configuring PKI max lease TTL to 10 years..."
docker exec -e VAULT_TOKEN=root netweave-vault vault secrets tune -max-lease-ttl=87600h pki

# Generate root CA certificate
echo "3. Generating root CA certificate..."
docker exec -e VAULT_TOKEN=root netweave-vault vault write -field=certificate pki/root/generate/internal \
  common_name="NetWeave Root CA" \
  issuer_name="netweave-root" \
  ttl=87600h > /tmp/root_ca.crt

echo "   Root CA certificate saved to /tmp/root_ca.crt"

# Configure CA and CRL URLs
echo "4. Configuring CA and CRL URLs..."
docker exec -e VAULT_TOKEN=root netweave-vault vault write pki/config/urls \
  issuing_certificates="http://localhost:8200/v1/pki/ca" \
  crl_distribution_points="http://localhost:8200/v1/pki/crl"

# Enable PKI secrets engine for intermediate CA
echo "5. Enabling PKI secrets engine at pki_int/..."
docker exec -e VAULT_TOKEN=root netweave-vault vault secrets enable -path=pki_int pki

# Tune max lease TTL to 5 years for intermediate CA
echo "6. Configuring intermediate PKI max lease TTL to 5 years..."
docker exec -e VAULT_TOKEN=root netweave-vault vault secrets tune -max-lease-ttl=43800h pki_int

# Generate intermediate CA CSR
echo "7. Generating intermediate CA CSR..."
docker exec -e VAULT_TOKEN=root netweave-vault vault write -format=json pki_int/intermediate/generate/internal \
  common_name="NetWeave Intermediate CA" \
  issuer_name="netweave-intermediate" \
  | jq -r '.data.csr' > /tmp/pki_intermediate.csr

# Sign intermediate CA with root CA
echo "8. Signing intermediate CA with root CA..."
docker exec -e VAULT_TOKEN=root netweave-vault vault write -format=json pki/root/sign-intermediate \
  issuer_ref="netweave-root" \
  csr="$(cat /tmp/pki_intermediate.csr)" \
  format=pem_bundle \
  ttl=43800h \
  | jq -r '.data.certificate' > /tmp/intermediate.cert.pem

# Import signed intermediate certificate
echo "9. Importing signed intermediate certificate..."
docker exec -e VAULT_TOKEN=root netweave-vault sh -c "cat <<EOF | vault write pki_int/intermediate/set-signed certificate=-
$(cat /tmp/intermediate.cert.pem)
EOF"

# Configure intermediate CA URLs
echo "10. Configuring intermediate CA and CRL URLs..."
docker exec -e VAULT_TOKEN=root netweave-vault vault write pki_int/config/urls \
  issuing_certificates="http://localhost:8200/v1/pki_int/ca" \
  crl_distribution_points="http://localhost:8200/v1/pki_int/crl"

# Create role for NetWeave client certificates
echo "11. Creating netweave-client role..."
docker exec -e VAULT_TOKEN=root netweave-vault vault write pki_int/roles/netweave-client \
  issuer_ref="netweave-intermediate" \
  allowed_domains="netweave.local,localhost" \
  allow_subdomains=true \
  allow_bare_domains=true \
  allow_localhost=true \
  allow_ip_sans=true \
  max_ttl=8760h \
  key_type=rsa \
  key_bits=2048 \
  require_cn=true

# Create role for NetWeave server certificates
echo "12. Creating netweave-server role..."
docker exec -e VAULT_TOKEN=root netweave-vault vault write pki_int/roles/netweave-server \
  issuer_ref="netweave-intermediate" \
  allowed_domains="netweave.local,localhost" \
  allow_subdomains=true \
  allow_bare_domains=true \
  allow_localhost=true \
  allow_ip_sans=true \
  server_flag=true \
  client_flag=false \
  max_ttl=8760h \
  key_type=rsa \
  key_bits=2048

# Create policy for gateway to read CA and issue certs
echo "13. Creating vault policy for gateway..."
docker exec -e VAULT_TOKEN=root netweave-vault sh -c 'cat <<EOF | vault policy write gateway -
# Read CA certificate
path "pki_int/cert/ca" {
  capabilities = ["read"]
}

# Read CA certificate chain
path "pki_int/ca_chain" {
  capabilities = ["read"]
}

# Issue client certificates
path "pki_int/issue/netweave-client" {
  capabilities = ["create", "update"]
}

# Revoke certificates
path "pki_int/revoke" {
  capabilities = ["create", "update"]
}

# Read CRL
path "pki_int/crl" {
  capabilities = ["read"]
}
EOF'

# Create policy for Keycloak to issue certs
echo "14. Creating vault policy for Keycloak..."
docker exec -e VAULT_TOKEN=root netweave-vault sh -c 'cat <<EOF | vault policy write keycloak -
# Issue client certificates
path "pki_int/issue/netweave-client" {
  capabilities = ["create", "update"]
}

# Read CA certificate
path "pki_int/cert/ca" {
  capabilities = ["read"]
}
EOF'

# Generate test client certificate
echo "15. Generating test client certificate..."
docker exec -e VAULT_TOKEN=root netweave-vault vault write -format=json pki_int/issue/netweave-client \
  common_name="test-client.netweave.local" \
  alt_names="localhost" \
  ip_sans="127.0.0.1" \
  ttl=720h > /tmp/test-client-cert.json

cat /tmp/test-client-cert.json | jq -r '.data.certificate' > /tmp/test-client.crt
cat /tmp/test-client-cert.json | jq -r '.data.private_key' > /tmp/test-client.key
cat /tmp/test-client-cert.json | jq -r '.data.ca_chain[]' > /tmp/ca-chain.crt

echo "   Test client certificate saved to /tmp/test-client.crt"
echo "   Test client private key saved to /tmp/test-client.key"
echo "   CA chain saved to /tmp/ca-chain.crt"

# Generate test server certificate for gateway
echo "16. Generating test server certificate..."
docker exec -e VAULT_TOKEN=root netweave-vault vault write -format=json pki_int/issue/netweave-server \
  common_name="netweave-gateway.netweave.local" \
  alt_names="localhost" \
  ip_sans="127.0.0.1" \
  ttl=720h > /tmp/test-server-cert.json

cat /tmp/test-server-cert.json | jq -r '.data.certificate' > /tmp/test-server.crt
cat /tmp/test-server-cert.json | jq -r '.data.private_key' > /tmp/test-server.key

echo "   Test server certificate saved to /tmp/test-server.crt"
echo "   Test server private key saved to /tmp/test-server.key"

echo ""
echo "=========================================="
echo "Vault PKI setup complete!"
echo "=========================================="
echo ""
echo "Root CA Certificate: /tmp/root_ca.crt"
echo "Intermediate CA Certificate: /tmp/intermediate.cert.pem"
echo "Test Client Cert: /tmp/test-client.crt"
echo "Test Client Key: /tmp/test-client.key"
echo "Test Server Cert: /tmp/test-server.crt"
echo "Test Server Key: /tmp/test-server.key"
echo "CA Chain: /tmp/ca-chain.crt"
echo ""
echo "Vault UI: http://localhost:8200/ui"
echo "Root Token: root"
echo ""
