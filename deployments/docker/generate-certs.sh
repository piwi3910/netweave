#!/bin/bash
set -e

echo "================================"
echo "Generating TLS Certificates"
echo "================================"

# Create directories
mkdir -p vault/tls keycloak/tls redis/tls secrets

# Generate CA
echo "1. Generating Certificate Authority..."
openssl genrsa -out ca-key.pem 4096
openssl req -new -x509 -days 3650 -key ca-key.pem -out ca.crt \
  -subj "/C=US/ST=State/L=City/O=NetWeave/OU=Development/CN=NetWeave CA"

# Generate Vault certificates
echo "2. Generating Vault certificates..."
openssl genrsa -out vault/tls/tls.key 2048

cat > vault/tls/cert.conf <<EOF
[req]
req_extensions = v3_req
distinguished_name = req_distinguished_name

[req_distinguished_name]

[v3_req]
basicConstraints = CA:FALSE
keyUsage = nonRepudiation, digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth, clientAuth
subjectAltName = @alt_names

[alt_names]
DNS.1 = vault
DNS.2 = localhost
IP.1 = 127.0.0.1
EOF

openssl req -new -key vault/tls/tls.key -out vault/tls/tls.csr \
  -config vault/tls/cert.conf \
  -subj "/C=US/ST=State/L=City/O=NetWeave/OU=Development/CN=vault"

openssl x509 -req -in vault/tls/tls.csr -CA ca.crt -CAkey ca-key.pem \
  -CAcreateserial -out vault/tls/tls.crt -days 365 \
  -extensions v3_req -extfile vault/tls/cert.conf

cp ca.crt vault/tls/ca.crt

# Generate Keycloak certificates
echo "3. Generating Keycloak certificates..."
openssl genrsa -out keycloak/tls/tls.key 2048

cat > keycloak/tls/cert.conf <<EOF
[req]
req_extensions = v3_req
distinguished_name = req_distinguished_name

[req_distinguished_name]

[v3_req]
basicConstraints = CA:FALSE
keyUsage = nonRepudiation, digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth, clientAuth
subjectAltName = @alt_names

[alt_names]
DNS.1 = keycloak
DNS.2 = localhost
IP.1 = 127.0.0.1
EOF

openssl req -new -key keycloak/tls/tls.key -out keycloak/tls/tls.csr \
  -config keycloak/tls/cert.conf \
  -subj "/C=US/ST=State/L=City/O=NetWeave/OU=Development/CN=keycloak"

openssl x509 -req -in keycloak/tls/tls.csr -CA ca.crt -CAkey ca-key.pem \
  -CAcreateserial -out keycloak/tls/tls.crt -days 365 \
  -extensions v3_req -extfile keycloak/tls/cert.conf

cp ca.crt keycloak/tls/ca.crt

# Generate Redis certificates
echo "4. Generating Redis certificates..."
openssl genrsa -out redis/tls/tls.key 2048

cat > redis/tls/cert.conf <<EOF
[req]
req_extensions = v3_req
distinguished_name = req_distinguished_name

[req_distinguished_name]

[v3_req]
basicConstraints = CA:FALSE
keyUsage = nonRepudiation, digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth, clientAuth
subjectAltName = @alt_names

[alt_names]
DNS.1 = redis
DNS.2 = localhost
IP.1 = 127.0.0.1
EOF

openssl req -new -key redis/tls/tls.key -out redis/tls/tls.csr \
  -config redis/tls/cert.conf \
  -subj "/C=US/ST=State/L=City/O=NetWeave/OU=Development/CN=redis"

openssl x509 -req -in redis/tls/tls.csr -CA ca.crt -CAkey ca-key.pem \
  -CAcreateserial -out redis/tls/tls.crt -days 365 \
  -extensions v3_req -extfile redis/tls/cert.conf

cp ca.crt redis/tls/ca.crt

# Generate secrets
echo "5. Generating secrets..."
openssl rand -base64 32 > secrets/postgres_password.txt
openssl rand -base64 32 > secrets/keycloak_admin_password.txt
openssl rand -base64 32 > secrets/redis_password.txt

# Set proper permissions
chmod 600 vault/tls/tls.key keycloak/tls/tls.key redis/tls/tls.key
chmod 600 ca-key.pem
chmod 600 secrets/*.txt

# Clean up CSRs and configs
rm -f vault/tls/*.csr vault/tls/cert.conf
rm -f keycloak/tls/*.csr keycloak/tls/cert.conf
rm -f redis/tls/*.csr redis/tls/cert.conf

echo ""
echo "================================"
echo "Certificates Generated!"
echo "================================"
echo ""
echo "Generated files:"
echo "  - ca.crt (Root CA certificate)"
echo "  - vault/tls/tls.{crt,key}"
echo "  - keycloak/tls/tls.{crt,key}"
echo "  - redis/tls/tls.{crt,key}"
echo "  - secrets/postgres_password.txt"
echo "  - secrets/keycloak_admin_password.txt"
echo "  - secrets/redis_password.txt"
echo ""
echo "⚠️  Security Notes:"
echo "  1. Certificates are self-signed (not for production)"
echo "  2. Keep ca-key.pem secure (private CA key)"
echo "  3. Add ca.crt to your system trust store for local testing"
echo "  4. Never commit secrets/ directory to git"
echo ""
