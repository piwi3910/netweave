#!/bin/sh
set -e

echo "Initializing Vault..."

# Check if already initialized
if [ -f "/vault/init/initialized" ]; then
  echo "Vault already initialized"
  exit 0
fi

# Initialize Vault
vault operator init -key-shares=5 -key-threshold=3 -format=json > /vault/init/keys.json

# Unseal Vault
for i in 0 1 2; do
  KEY=$(jq -r ".unseal_keys_b64[$i]" /vault/init/keys.json)
  vault operator unseal "$KEY"
done

# Login and configure PKI
export VAULT_TOKEN=$(jq -r '.root_token' /vault/init/keys.json)

# Enable PKI
vault secrets enable -path=pki pki
vault secrets tune -max-lease-ttl=87600h pki
vault write pki/root/generate/internal common_name="NetWeave Root CA" issuer_name="netweave-root" ttl=87600h
vault write pki/config/urls issuing_certificates="https://vault:8200/v1/pki/ca" crl_distribution_points="https://vault:8200/v1/pki/crl"

# Enable intermediate PKI
vault secrets enable -path=pki_int pki
vault secrets tune -max-lease-ttl=43800h pki_int
vault write -format=json pki_int/intermediate/generate/internal common_name="NetWeave Intermediate CA" issuer_name="netweave-intermediate" | jq -r '.data.csr' > /tmp/pki_intermediate.csr
vault write -format=json pki/root/sign-intermediate issuer_ref="netweave-root" csr=@/tmp/pki_intermediate.csr format=pem_bundle ttl=43800h | jq -r '.data.certificate' > /tmp/intermediate.cert.pem
vault write pki_int/intermediate/set-signed certificate=@/tmp/intermediate.cert.pem
vault write pki_int/config/urls issuing_certificates="https://vault:8200/v1/pki_int/ca" crl_distribution_points="https://vault:8200/v1/pki_int/crl"

# Create roles
vault write pki_int/roles/netweave-client issuer_ref="netweave-intermediate" allowed_domains="netweave.local,*.netweave.local,localhost" allow_subdomains=true allow_bare_domains=true allow_localhost=true allow_ip_sans=true max_ttl=8760h key_type=rsa key_bits=2048
vault write pki_int/roles/netweave-server issuer_ref="netweave-intermediate" allowed_domains="netweave.local,*.netweave.local,localhost" allow_subdomains=true allow_bare_domains=true allow_localhost=true allow_ip_sans=true server_flag=true client_flag=false max_ttl=8760h key_type=rsa key_bits=2048

# Create policies
vault policy write gateway - <<EOF
path "pki_int/cert/ca" { capabilities = ["read"] }
path "pki_int/issue/netweave-client" { capabilities = ["create", "update"] }
path "pki_int/issue/netweave-server" { capabilities = ["create", "update"] }
path "pki_int/revoke" { capabilities = ["create", "update"] }
EOF

vault policy write keycloak - <<EOF
path "pki_int/issue/netweave-client" { capabilities = ["create", "update"] }
path "pki_int/cert/ca" { capabilities = ["read"] }
EOF

touch /vault/init/initialized
echo "Vault initialization complete!"
