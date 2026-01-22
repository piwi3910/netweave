# Certificate Management

**HashiCorp Vault PKI engine for automated mTLS certificate lifecycle management.**

## Table of Contents

1. [Overview](#overview)
2. [Architecture](#architecture)
3. [Vault PKI Setup](#vault-pki-setup)
4. [Certificate Issuance](#certificate-issuance)
5. [Certificate Renewal](#certificate-renewal)
6. [Certificate Revocation](#certificate-revocation)
7. [Monitoring](#monitoring)
8. [Troubleshooting](#troubleshooting)

---

## Overview

### Why Vault for Certificate Management?

The O2-IMS Gateway uses HashiCorp Vault's PKI secrets engine for automated certificate lifecycle management:

- ✅ **Automated Issuance**: On-demand certificate generation via REST API
- ✅ **Automatic Renewal**: Certificates renewed before expiration
- ✅ **Instant Revocation**: Compromised certificates revoked immediately
- ✅ **Audit Trail**: All certificate operations logged
- ✅ **High Availability**: Vault HA cluster for reliability
- ✅ **Policy-Based**: Fine-grained controls via Vault policies

### Certificate Types

| Type | Purpose | Lifetime | Renewal |
|------|---------|----------|---------|
| **Root CA** | Trust anchor | 10 years | Manual |
| **Intermediate CA** | Signing certificates | 5 years | Manual |
| **mTLS Client Certs** | Client authentication | 90 days | Automatic |
| **mTLS Server Certs** | Server TLS | 90 days | Automatic |
| **Service Certs** | Internal services | 30 days | Automatic |

---

## Architecture

### Certificate Hierarchy

```mermaid
graph TB
    subgraph "Certificate Authority Hierarchy"
        ROOT[Root CA<br/>Offline, 10 year lifetime]
        INT[Intermediate CA<br/>Online, 5 year lifetime<br/>Vault PKI]

        ROOT -->|Signs| INT

        INT -->|Issues| CLIENT[Client Certificates<br/>mTLS authentication<br/>90 day lifetime]
        INT -->|Issues| SERVER[Server Certificates<br/>Gateway TLS<br/>90 day lifetime]
        INT -->|Issues| SERVICE[Service Certificates<br/>Internal mTLS<br/>30 day lifetime]
    end

    subgraph "Certificate Lifecycle"
        ISSUE[Issue]
        USE[Use]
        RENEW[Renew]
        REVOKE[Revoke]

        ISSUE --> USE
        USE --> RENEW
        USE -.->|If compromised| REVOKE
        RENEW --> USE
    end

    style ROOT fill:#ffebee
    style INT fill:#fff4e6
    style CLIENT fill:#e8f5e9
    style SERVER fill:#e8f5e9
    style SERVICE fill:#e8f5e9
```

### Vault PKI Integration

```mermaid
sequenceDiagram
    participant Client as SMO Client
    participant GW as Gateway
    participant Vault as Vault PKI
    participant CA as Intermediate CA

    Note over Client: Certificate Request
    Client->>Vault: POST /pki/issue/client-role<br/>{common_name, ttl}
    Vault->>CA: Generate certificate
    CA->>Vault: Signed certificate
    Vault->>Client: Certificate + Private Key

    Note over Client: Use Certificate
    Client->>GW: Request with mTLS cert
    GW->>Vault: Verify certificate (CRL check)
    Vault->>GW: Valid/Revoked status
    GW->>Client: Authorized response

    Note over Client: Renewal (before expiry)
    Client->>Vault: POST /pki/issue/client-role<br/>(reuse subject)
    Vault->>Client: New certificate

    Note over Client: Revocation (if compromised)
    Client->>Vault: POST /pki/revoke<br/>{serial_number}
    Vault->>Vault: Add to CRL
    Vault->>Client: Revocation confirmed
```

---

## Vault PKI Setup

### Prerequisites

```bash
# Vault deployed and initialized
kubectl get pods -n vault

# Vault CLI configured
export VAULT_ADDR=https://vault.example.com:8200
export VAULT_TOKEN=<admin-token>

# Verify Vault access
vault status
```

### Initialize PKI Engine

**Step 1: Enable PKI secrets engine**

```bash
# Enable PKI engine for O2-IMS Gateway
vault secrets enable -path=o2ims-pki pki

# Set max lease TTL to 10 years for root CA
vault secrets tune -max-lease-ttl=87600h o2ims-pki
```

**Step 2: Generate root CA**

```bash
# Generate self-signed root CA (for development)
vault write o2ims-pki/root/generate/internal \
    common_name="O2-IMS Root CA" \
    ttl=87600h \
    key_bits=4096 \
    exclude_cn_from_sans=true

# Or import existing root CA (for production)
vault write o2ims-pki/config/ca \
    pem_bundle=@root-ca-bundle.pem
```

**Step 3: Enable intermediate CA**

```bash
# Enable intermediate PKI engine
vault secrets enable -path=o2ims-pki-int pki

# Set max lease TTL to 5 years
vault secrets tune -max-lease-ttl=43800h o2ims-pki-int

# Generate CSR for intermediate CA
vault write -format=json o2ims-pki-int/intermediate/generate/internal \
    common_name="O2-IMS Intermediate CA" \
    ttl=43800h \
    key_bits=4096 \
    | jq -r '.data.csr' > intermediate.csr

# Sign intermediate CSR with root CA
vault write -format=json o2ims-pki/root/sign-intermediate \
    csr=@intermediate.csr \
    format=pem_bundle \
    ttl=43800h \
    | jq -r '.data.certificate' > intermediate.cert.pem

# Import signed intermediate certificate
vault write o2ims-pki-int/intermediate/set-signed \
    certificate=@intermediate.cert.pem
```

### Configure Certificate Roles

**Client Certificate Role:**

```bash
vault write o2ims-pki-int/roles/client \
    allowed_domains="o2ims.example.com" \
    allow_subdomains=true \
    allow_bare_domains=false \
    allow_glob_domains=false \
    key_bits=2048 \
    key_type=rsa \
    ttl=2160h \
    max_ttl=2160h \
    require_cn=true \
    allowed_uri_sans="email:*@example.com" \
    organization="O2-IMS Gateway" \
    ou="Clients"
```

**Server Certificate Role:**

```bash
vault write o2ims-pki-int/roles/server \
    allowed_domains="o2ims.example.com,*.o2ims.example.com" \
    allow_subdomains=true \
    server_flag=true \
    client_flag=false \
    key_bits=2048 \
    key_type=rsa \
    ttl=2160h \
    max_ttl=2160h \
    organization="O2-IMS Gateway" \
    ou="Servers"
```

**Service Certificate Role:**

```bash
vault write o2ims-pki-int/roles/service \
    allowed_domains="svc.cluster.local,*.svc.cluster.local" \
    allow_subdomains=true \
    server_flag=true \
    client_flag=true \
    key_bits=2048 \
    key_type=rsa \
    ttl=720h \
    max_ttl=720h \
    organization="O2-IMS Gateway" \
    ou="Services"
```

### Configure CRL

```bash
# Enable CRL endpoints
vault write o2ims-pki-int/config/crl \
    expiry=72h \
    disable=false

# Set CRL distribution points
vault write o2ims-pki-int/config/urls \
    issuing_certificates="https://vault.example.com:8200/v1/o2ims-pki-int/ca" \
    crl_distribution_points="https://vault.example.com:8200/v1/o2ims-pki-int/crl"
```

---

## Certificate Issuance

### Issue Client Certificate

**Via Vault CLI:**

```bash
# Issue certificate for SMO client
vault write -format=json o2ims-pki-int/issue/client \
    common_name="operator-1.smo-alpha.o2ims.example.com" \
    alt_names="operator-1.tenant.smo-alpha" \
    ttl=2160h \
    | jq -r '.data' > client-cert.json

# Extract certificate and key
jq -r '.certificate' client-cert.json > client.crt
jq -r '.private_key' client-cert.json > client.key
jq -r '.ca_chain[]' client-cert.json > ca-chain.crt
```

**Via REST API:**

```bash
curl -X POST \
    -H "X-Vault-Token: $VAULT_TOKEN" \
    -d '{"common_name":"operator-1.smo-alpha.o2ims.example.com","ttl":"2160h"}' \
    https://vault.example.com:8200/v1/o2ims-pki-int/issue/client
```

**Response:**

```json
{
  "request_id": "abc-123",
  "data": {
    "certificate": "-----BEGIN CERTIFICATE-----\n...",
    "issuing_ca": "-----BEGIN CERTIFICATE-----\n...",
    "ca_chain": ["-----BEGIN CERTIFICATE-----\n..."],
    "private_key": "-----BEGIN RSA PRIVATE KEY-----\n...",
    "private_key_type": "rsa",
    "serial_number": "3a:1f:2e:...",
    "expiration": 1744992000
  }
}
```

### Issue Server Certificate

```bash
vault write -format=json o2ims-pki-int/issue/server \
    common_name="o2ims-gateway.example.com" \
    alt_names="o2ims-gateway.svc.cluster.local,*.o2ims-gateway.svc.cluster.local" \
    ip_sans="10.96.0.10" \
    ttl=2160h \
    | jq -r '.data' > server-cert.json
```

### Verify Certificate

```bash
# View certificate details
openssl x509 -in client.crt -text -noout

# Verify certificate chain
openssl verify -CAfile ca-chain.crt client.crt

# Check expiration
openssl x509 -in client.crt -noout -dates
```

---

## Certificate Renewal

### Automatic Renewal (Recommended)

The gateway includes a certificate renewal service that automatically renews certificates before expiration.

**Configuration:**

```yaml
# config.yaml
certificate_renewal:
  enabled: true
  renew_before_expiry: 168h  # Renew 7 days before expiration
  check_interval: 1h
  vault_role: "client"
```

**Renewal Process:**

```go
// internal/certs/renewal.go
package certs

import (
    "context"
    "time"
)

// RenewalService handles automatic certificate renewal
type RenewalService struct {
    vaultClient   *vault.Client
    certPath      string
    keyPath       string
    renewBefore   time.Duration
    checkInterval time.Duration
}

// Start begins certificate monitoring and renewal
func (s *RenewalService) Start(ctx context.Context) error {
    ticker := time.NewTicker(s.checkInterval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-ticker.C:
            if err := s.checkAndRenew(ctx); err != nil {
                log.Error("certificate renewal failed", "error", err)
            }
        }
    }
}

// checkAndRenew checks certificate expiry and renews if needed
func (s *RenewalService) checkAndRenew(ctx context.Context) error {
    cert, err := tls.LoadX509KeyPair(s.certPath, s.keyPath)
    if err != nil {
        return fmt.Errorf("failed to load certificate: %w", err)
    }

    leaf, err := x509.ParseCertificate(cert.Certificate[0])
    if err != nil {
        return fmt.Errorf("failed to parse certificate: %w", err)
    }

    timeUntilExpiry := time.Until(leaf.NotAfter)
    if timeUntilExpiry > s.renewBefore {
        log.Debug("certificate still valid", "expires_in", timeUntilExpiry)
        return nil
    }

    log.Info("renewing certificate", "expires_in", timeUntilExpiry)
    return s.renewCertificate(ctx, leaf)
}
```

### Manual Renewal

```bash
# Renew by issuing new certificate with same CN
vault write -format=json o2ims-pki-int/issue/client \
    common_name="operator-1.smo-alpha.o2ims.example.com" \
    ttl=2160h \
    | jq -r '.data' > client-cert-renewed.json

# Replace old certificate
cp client.crt client.crt.backup
jq -r '.certificate' client-cert-renewed.json > client.crt
jq -r '.private_key' client-cert-renewed.json > client.key

# Restart services using the certificate
kubectl rollout restart deployment/o2ims-gateway -n o2ims-system
```

---

## Certificate Revocation

### Revoke Certificate

**When to Revoke:**
- Private key compromised
- Certificate issued incorrectly
- User access terminated
- Security incident

**Revocation Process:**

```bash
# Revoke by serial number
vault write o2ims-pki-int/revoke \
    serial_number="3a:1f:2e:4d:5c:6b:7a:8e:9f:0d:1c:2b:3a:4d:5c:6e"

# Revoke by certificate file
vault write o2ims-pki-int/revoke \
    certificate=@client.crt
```

**Verify Revocation:**

```bash
# Check CRL
curl https://vault.example.com:8200/v1/o2ims-pki-int/crl | \
    openssl crl -inform DER -text -noout | \
    grep -A1 "Serial Number"

# Test with gateway
curl -k --cert revoked-client.crt --key revoked-client.key \
    https://o2ims-gateway.example.com/health
# Expected: TLS handshake failure or 401 Unauthorized
```

### CRL Management

**Rotate CRL:**

```bash
# Trigger CRL rotation
vault write -f o2ims-pki-int/crl/rotate
```

**Configure CRL Refresh:**

```yaml
# Gateway configuration
tls:
  crl_check: true
  crl_url: https://vault.example.com:8200/v1/o2ims-pki-int/crl
  crl_refresh_interval: 1h
```

---

## Monitoring

### Certificate Expiry Monitoring

**Prometheus Metrics:**

```promql
# Certificates expiring in 7 days
certificate_expiry_seconds{job="o2ims-gateway"} < 604800

# Certificate renewal failures
rate(certificate_renewal_failures_total[5m]) > 0
```

**Alert Rules:**

```yaml
# prometheus-rules.yaml
groups:
  - name: certificates
    rules:
      - alert: CertificateExpiringSoon
        expr: certificate_expiry_seconds < 604800
        for: 1h
        labels:
          severity: warning
        annotations:
          summary: "Certificate expiring soon"
          description: "Certificate {{ $labels.serial }} expires in {{ $value | humanizeDuration }}"

      - alert: CertificateRenewalFailed
        expr: rate(certificate_renewal_failures_total[5m]) > 0
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Certificate renewal failing"
          description: "Certificate renewal has failed {{ $value }} times in the last 5 minutes"
```

### Vault PKI Monitoring

```bash
# Monitor PKI engine status
vault read o2ims-pki-int/config/crl
vault list o2ims-pki-int/certs

# Check issued certificates
vault list o2ims-pki-int/certs | wc -l

# View specific certificate
vault read o2ims-pki-int/cert/<serial-number>
```

---

## Troubleshooting

### Common Issues

**Issue: Certificate Renewal Failing**

```bash
# Check Vault connectivity
vault status

# Verify Vault token
vault token lookup

# Check PKI role configuration
vault read o2ims-pki-int/roles/client

# Check gateway logs
kubectl logs -n o2ims-system -l app=gateway | grep "certificate renewal"
```

**Issue: Certificate Verification Failing**

```bash
# Verify certificate chain
openssl verify -CAfile ca-chain.crt client.crt

# Check CRL
curl -s https://vault.example.com:8200/v1/o2ims-pki-int/crl | \
    openssl crl -inform DER -text -noout

# Test TLS handshake
openssl s_client -connect o2ims-gateway.example.com:443 \
    -cert client.crt -key client.key -CAfile ca-chain.crt
```

**Issue: Vault PKI Misconfigured**

```bash
# Verify PKI engine enabled
vault secrets list | grep o2ims-pki

# Check intermediate CA
vault read o2ims-pki-int/cert/ca

# Verify roles exist
vault list o2ims-pki-int/roles

# Test certificate issuance
vault write -format=json o2ims-pki-int/issue/client \
    common_name="test.smo-alpha.o2ims.example.com" \
    ttl=1h
```

### Debug Mode

Enable debug logging for certificate operations:

```yaml
# config.yaml
logging:
  level: debug

certificate_renewal:
  enabled: true
  debug: true
```

```bash
# View debug logs
kubectl logs -n o2ims-system -l app=gateway --tail=100 | grep -i cert
```

---

## Best Practices

### Security

- ✅ **Keep Root CA Offline**: Store root CA private key offline
- ✅ **Rotate Intermediate CAs**: Rotate intermediate CA every 2-3 years
- ✅ **Short Certificate Lifetimes**: Use 90-day certificates for clients
- ✅ **Monitor Expiry**: Alert on certificates expiring within 30 days
- ✅ **Revoke Quickly**: Revoke compromised certificates immediately
- ✅ **Audit Everything**: Log all certificate operations

### Operations

- ✅ **Automate Renewal**: Use automatic renewal service
- ✅ **Test Renewals**: Test renewal process monthly
- ✅ **Backup Certificates**: Backup root and intermediate CAs
- ✅ **Document Procedures**: Maintain runbooks for operations
- ✅ **Monitor Vault Health**: Alert on Vault unavailability

### Compliance

- ✅ **Maintain Audit Logs**: Retain logs for 90+ days
- ✅ **Regular Reviews**: Review issued certificates quarterly
- ✅ **Access Controls**: Restrict Vault PKI access via policies
- ✅ **Disaster Recovery**: Test CA recovery procedures

---

## Resources

### Documentation

- [HashiCorp Vault PKI Engine](https://developer.hashicorp.com/vault/docs/secrets/pki)
- [X.509 Certificate Standards](https://tools.ietf.org/html/rfc5280)
- [TLS/mTLS Guide](tls-mtls.md)
- [Authentication Guide](authentication.md)

### Tools

- [cert-manager](https://cert-manager.io/) - Kubernetes certificate management
- [openssl](https://www.openssl.org/) - Certificate utilities
- [cfssl](https://github.com/cloudflare/cfssl) - PKI toolkit

---

**Last Updated:** 2026-01-22
**Version:** 1.0
