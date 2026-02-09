# Certificate Management

**HashiCorp Vault PKI engine for automated mTLS certificate lifecycle management.**

## Table of Contents

1. [Overview](#overview)
2. [Architecture](#architecture)
3. [Vault PKI Setup](#vault-pki-setup)
4. [Certificate Issuance](#certificate-issuance)
5. [Certificate Renewal](#certificate-renewal)
6. [Certificate Renewal Notifications](#certificate-renewal-notifications)
7. [Certificate Revocation](#certificate-revocation)
8. [Monitoring](#monitoring)
9. [Troubleshooting](#troubleshooting)

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

## Certificate Renewal Notifications

### Overview

The certificate manager includes a built-in notification system that sends webhook-based alerts for certificate lifecycle events. This enables operators and external monitoring systems to react to certificate state changes in real time, such as upcoming expirations, successful renewals, and failures.

**Note:** The legacy `internal/certmanager/` package has been removed. The notification system described below is planned for reimplementation on Vault PKI (see GitHub issue #411). Certificate operations now use `internal/vault/` directly.

### Certificate Lifecycle Events

The following event types are emitted during the certificate lifecycle:

| Event Type | Constant | Description |
|-----------|----------|-------------|
| `certificate.issued` | `CertEventIssued` | A new certificate was successfully issued |
| `certificate.renewed` | `CertEventRenewed` | A certificate was successfully renewed |
| `certificate.renewal_failed` | `CertEventRenewalFailed` | A certificate renewal attempt failed |
| `certificate.expiring_soon` | `CertEventExpiringSoon` | A certificate will expire within the renewal window |
| `certificate.expired` | `CertEventExpired` | A certificate has expired |
| `certificate.revoked` | `CertEventRevoked` | A certificate was revoked |

Each event is delivered as a JSON payload containing the event type, full certificate details, a timestamp, and a human-readable message.

**Event Payload Example:**

```json
{
  "event_type": "certificate.expiring_soon",
  "certificate": {
    "serial_number": "3a:1f:2e:4d:5c:6b",
    "common_name": "operator-1.smo-alpha.o2ims.example.com",
    "user_id": "user-42",
    "tenant_id": "tenant-7",
    "status": "expiring_soon",
    "issued_at": "2026-01-01T00:00:00Z",
    "expires_at": "2026-04-01T00:00:00Z"
  },
  "timestamp": "2026-03-25T10:30:00Z",
  "message": "Certificate 3a:1f:2e:4d:5c:6b (operator-1.smo-alpha.o2ims.example.com) is expiring soon at 2026-04-01T00:00:00Z"
}
```

### Notification Channel: Webhooks

The notification system currently supports **HTTP POST webhooks** as the delivery channel. The `WebhookCertNotifier` sends JSON-encoded event payloads to a configured endpoint with the following characteristics:

- **HTTP Method**: POST
- **Content-Type**: `application/json`
- **User-Agent**: `Netweave-CertManager/1.0`
- **Retry Policy**: Up to 3 delivery attempts with 1-second backoff between retries
- **Timeout**: 10 seconds per HTTP request
- **Non-Blocking**: Notification failures are logged as warnings but never block certificate lifecycle operations (issuance, renewal, revocation continue regardless)

### HMAC Signature Verification

When an `HMACSecret` is configured, each webhook request includes an `X-Signature-SHA256` header containing an HMAC-SHA256 signature of the request body. This allows the receiving endpoint to verify the authenticity and integrity of the notification.

**Signature Format:** `sha256=<hex-encoded-hmac>`

**Verification Example (receiver side):**

```go
import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
)

func verifySignature(body []byte, signature string, secret string) bool {
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write(body)
    expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
    return hmac.Equal([]byte(expected), []byte(signature))
}
```

### Configuration

The notification system is configured via the `NotificationConfig` struct:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `WebhookURL` | `string` | Yes | The HTTP endpoint to send notification events to |
| `EnableAdminNotifications` | `bool` | No | Enable sending notifications to administrators |
| `EnableUserNotifications` | `bool` | No | Enable sending notifications to certificate owners |
| `HMACSecret` | `string` | No | Shared secret for HMAC-SHA256 webhook signature verification |

**Configuration Example:**

```yaml
# config.yaml
certificate_notifications:
  webhook_url: "https://monitoring.example.com/cert-events"
  enable_admin_notifications: true
  enable_user_notifications: false
  hmac_secret: "${CERT_WEBHOOK_HMAC_SECRET}"  # From environment variable
```

### Integration with the Gateway

The notification system integrates with the certificate manager `Service` through the `SetNotifier` method:

1. **Optional Setup**: Notifications are optional. If no notifier is configured, all certificate lifecycle operations proceed without sending notifications.
2. **Automatic Triggers**: The certificate monitoring loop automatically sends notifications when it detects certificates in `expiring_soon` or `expired` states via the `handleExpiringSoon` and `handleExpired` methods.
3. **Renewal Integration**: After a successful or failed renewal attempt, the appropriate event (`certificate.renewed` or `certificate.renewal_failed`) is emitted.
4. **Graceful Degradation**: If webhook delivery fails after all retry attempts, the failure is logged but does not affect the certificate lifecycle. This ensures that transient network issues or webhook endpoint outages do not disrupt certificate management.

```mermaid
sequenceDiagram
    participant Monitor as Certificate Monitor
    participant Service as CertManager Service
    participant Notifier as WebhookCertNotifier
    participant Webhook as External Webhook

    Monitor->>Service: scanAndRenew()
    Service->>Service: Check certificate expiry

    alt Certificate expiring soon
        Service->>Notifier: Notify(CertEventExpiringSoon)
        Notifier->>Webhook: POST /cert-events (with retry)
        Webhook-->>Notifier: 200 OK
        Service->>Service: Trigger auto-renewal
    end

    alt Certificate expired
        Service->>Notifier: Notify(CertEventExpired)
        Notifier->>Webhook: POST /cert-events (with retry)
        Webhook-->>Notifier: 200 OK
    end

    alt Renewal succeeded
        Service->>Notifier: Notify(CertEventRenewed)
        Notifier->>Webhook: POST /cert-events
    end

    alt Renewal failed
        Service->>Notifier: Notify(CertEventRenewalFailed)
        Notifier->>Webhook: POST /cert-events
    end
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

**Last Updated:** 2026-02-06
**Version:** 1.1
