# Security Hardening Guide

This guide provides comprehensive security hardening recommendations for production deployments of the O2-IMS Gateway.

## Table of Contents

1. [TLS/mTLS Configuration](#tlsmtls-configuration)
2. [Authentication & Authorization](#authentication--authorization)
3. [Network Security](#network-security)
4. [Secrets Management](#secrets-management)
5. [RBAC & Multi-Tenancy](#rbac--multi-tenancy)
6. [Production Security Checklist](#production-security-checklist)

---

## TLS/mTLS Configuration

### TLS 1.3 Requirements

The O2-IMS Gateway requires TLS 1.3 for all external connections. This is enforced by default.

```yaml
# config.yaml
server:
  tls_enabled: true
  tls_cert_file: /etc/o2ims/certs/server.crt
  tls_key_file: /etc/o2ims/certs/server.key
  tls_min_version: "1.3"
```

### Certificate Generation

#### Self-Signed Certificates (Development Only)

```bash
# Generate CA certificate
openssl genrsa -out ca.key 4096
openssl req -new -x509 -days 3650 -key ca.key -out ca.crt \
    -subj "/CN=O2-IMS-CA/O=ORAN/OU=O-Cloud"

# Generate server certificate
openssl genrsa -out server.key 4096
openssl req -new -key server.key -out server.csr \
    -subj "/CN=o2ims-gateway/O=ORAN/OU=O-Cloud"

# Sign with CA
openssl x509 -req -days 365 -in server.csr -CA ca.crt -CAkey ca.key \
    -CAcreateserial -out server.crt \
    -extfile <(echo "subjectAltName=DNS:o2ims-gateway,DNS:localhost,IP:127.0.0.1")
```

#### Production Certificates

For production, obtain certificates from a trusted Certificate Authority (CA):

1. **Public CA**: Use Let's Encrypt, DigiCert, or similar for publicly accessible endpoints
2. **Enterprise CA**: Use your organization's internal CA for internal services
3. **Cloud CA**: Use cloud-native certificate services (AWS ACM, GCP Certificate Manager, Azure Key Vault)

### mTLS Configuration

Enable mutual TLS to authenticate both clients and servers:

```yaml
# config.yaml
server:
  mtls_enabled: true
  mtls_client_ca_file: /etc/o2ims/certs/client-ca.crt
  mtls_client_cert_verification: "require_and_verify"
```

Client certificate requirements:
- **Subject CN**: Used for user identification
- **Subject O**: Organization identifier
- **Subject OU**: Tenant identifier (optional)

### Certificate Rotation

Implement automated certificate rotation:

```yaml
# Kubernetes cert-manager configuration
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: o2ims-gateway-cert
  namespace: o2ims
spec:
  secretName: o2ims-gateway-tls
  duration: 2160h    # 90 days
  renewBefore: 360h  # 15 days before expiry
  issuerRef:
    name: letsencrypt-prod
    kind: ClusterIssuer
  dnsNames:
    - o2ims-gateway.example.com
```

### Cipher Suite Selection

The gateway enforces secure cipher suites by default:

```go
// Recommended TLS 1.3 cipher suites (enforced by default)
TLS_AES_256_GCM_SHA384
TLS_CHACHA20_POLY1305_SHA256
TLS_AES_128_GCM_SHA256
```

---

## Authentication & Authorization

### Tenant Identification

Clients are identified by their mTLS certificate:

```
Certificate Subject: CN=smo-client-1,O=TelcoOperator,OU=tenant-abc
                     ^^^^^^^^^^^^^^^^  ^^^^^^^^^^^^^  ^^^^^^^^^^
                     Common Name       Organization   Tenant ID
```

Map certificates to tenants in the configuration:

```yaml
# config.yaml
auth:
  tenant_mapping:
    - subject_pattern: "CN=.*,OU=tenant-abc"
      tenant_id: "tenant-abc"
    - organization: "TelcoOperator"
      tenant_id: "telco-main"
```

### Client Certificate Requirements

1. **Valid CA Chain**: Certificate must be signed by a trusted CA
2. **Not Expired**: Certificate must be within validity period
3. **Key Usage**: Must include "Digital Signature" and "Key Encipherment"
4. **Extended Key Usage**: Should include "Client Authentication"

### API Key Management

For service accounts without mTLS:

```yaml
# Create API key for a service account
POST /o2ims/v1/auth/api-keys
{
  "name": "monitoring-service",
  "tenantId": "tenant-abc",
  "permissions": ["resources:read", "resourcePools:read"],
  "expiresAt": "2025-12-31T23:59:59Z"
}
```

Best practices:
- Set expiration dates (max 1 year recommended)
- Use least-privilege permissions
- Rotate API keys regularly (every 90 days)
- Revoke immediately if compromised

### Service Account Security

```yaml
# Kubernetes ServiceAccount configuration
apiVersion: v1
kind: ServiceAccount
metadata:
  name: o2ims-gateway
  namespace: o2ims
  annotations:
    # AWS: Use IRSA for role assumption
    eks.amazonaws.com/role-arn: arn:aws:iam::123456789:role/o2ims-gateway
    # GCP: Use Workload Identity
    iam.gke.io/gcp-service-account: o2ims@project.iam.gserviceaccount.com
```

---

## Network Security

### Network Policies

Restrict network access using Kubernetes NetworkPolicies:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: o2ims-gateway-policy
  namespace: o2ims
spec:
  podSelector:
    matchLabels:
      app: o2ims-gateway
  policyTypes:
    - Ingress
    - Egress
  ingress:
    # Allow HTTPS from SMO/NMS
    - from:
        - namespaceSelector:
            matchLabels:
              name: smo
        - ipBlock:
            cidr: 10.0.0.0/8
            except:
              - 10.255.0.0/16
      ports:
        - protocol: TCP
          port: 8443
  egress:
    # Allow Redis connections
    - to:
        - podSelector:
            matchLabels:
              app: redis
      ports:
        - protocol: TCP
          port: 6379
    # Allow Kubernetes API
    - to:
        - namespaceSelector: {}
          podSelector:
            matchLabels:
              component: kube-apiserver
      ports:
        - protocol: TCP
          port: 6443
    # Allow external adapters
    - to:
        - ipBlock:
            cidr: 0.0.0.0/0
            except:
              - 10.0.0.0/8
              - 172.16.0.0/12
              - 192.168.0.0/16
      ports:
        - protocol: TCP
          port: 443
```

### Ingress Controller Hardening

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: o2ims-gateway
  annotations:
    # NGINX ingress annotations
    nginx.ingress.kubernetes.io/ssl-redirect: "true"
    nginx.ingress.kubernetes.io/force-ssl-redirect: "true"
    nginx.ingress.kubernetes.io/ssl-passthrough: "false"
    nginx.ingress.kubernetes.io/backend-protocol: "HTTPS"

    # Rate limiting
    nginx.ingress.kubernetes.io/limit-rps: "100"
    nginx.ingress.kubernetes.io/limit-connections: "50"

    # Security headers
    nginx.ingress.kubernetes.io/configuration-snippet: |
      more_set_headers "X-Frame-Options: DENY";
      more_set_headers "X-Content-Type-Options: nosniff";
      more_set_headers "X-XSS-Protection: 1; mode=block";
spec:
  ingressClassName: nginx
  tls:
    - hosts:
        - o2ims-gateway.example.com
      secretName: o2ims-gateway-tls
  rules:
    - host: o2ims-gateway.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: o2ims-gateway
                port:
                  number: 8443
```

### Firewall Rules

Example firewall rules for cloud environments:

```bash
# AWS Security Group
aws ec2 create-security-group \
    --group-name o2ims-gateway-sg \
    --description "O2-IMS Gateway security group"

# Allow HTTPS from known SMO IP ranges
aws ec2 authorize-security-group-ingress \
    --group-name o2ims-gateway-sg \
    --protocol tcp \
    --port 8443 \
    --cidr 203.0.113.0/24

# Deny all other inbound by default
```

### DDoS Mitigation

1. **Rate Limiting**: Enable at gateway and ingress levels
2. **Connection Limits**: Set maximum concurrent connections
3. **Request Size Limits**: Limit request body size
4. **Timeout Configuration**: Set appropriate timeouts

```yaml
# config.yaml
server:
  read_timeout: 30s
  write_timeout: 30s
  max_header_bytes: 65536
  max_request_body_size: 10485760  # 10MB

security:
  rate_limit_enabled: true
  rate_limit:
    tenant:
      requests_per_second: 1000
      burst_size: 2000
```

---

## Secrets Management

### Kubernetes Secrets

Store sensitive configuration in Kubernetes Secrets:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: o2ims-gateway-secrets
  namespace: o2ims
type: Opaque
stringData:
  redis-password: "${REDIS_PASSWORD}"
  webhook-hmac-secret: "${WEBHOOK_SECRET}"
  adapter-api-keys: |
    dtias: "${DTIAS_API_KEY}"
    onap: "${ONAP_API_KEY}"
```

### External Secrets Operator

Integrate with external secret managers:

```yaml
# AWS Secrets Manager integration
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: o2ims-gateway-secrets
  namespace: o2ims
spec:
  refreshInterval: 1h
  secretStoreRef:
    name: aws-secrets-manager
    kind: ClusterSecretStore
  target:
    name: o2ims-gateway-secrets
  data:
    - secretKey: redis-password
      remoteRef:
        key: o2ims/redis
        property: password
    - secretKey: webhook-secret
      remoteRef:
        key: o2ims/webhook
        property: hmac-secret
```

### Secret Rotation

Implement automated secret rotation:

```yaml
# Example rotation policy
secrets:
  rotation:
    - name: webhook-hmac-secret
      interval: 90d
      strategy: dual-write  # Support both old and new during transition
    - name: redis-password
      interval: 30d
      strategy: rolling     # Zero-downtime rotation
```

### Preventing Secret Exposure

1. **Environment Variables**: Never log environment variables
2. **Request Logging**: Redact sensitive headers and body fields
3. **Error Messages**: Never include secrets in error responses
4. **Audit Logs**: Mask sensitive data in audit events

```yaml
# config.yaml
observability:
  logging:
    redact_headers:
      - Authorization
      - X-API-Key
      - X-O2IMS-Signature
    redact_fields:
      - password
      - secret
      - api_key
      - token
```

---

## RBAC & Multi-Tenancy

### Least Privilege Principle

Assign minimal required permissions:

| Role | Use Case | Permissions |
|------|----------|-------------|
| `viewer` | Read-only monitoring | `*:read` |
| `operator` | Day-to-day operations | `subscriptions:*`, `resources:read` |
| `admin` | Tenant administration | All except tenant management |
| `platform-admin` | Platform administration | All permissions |

### Role Definition

```yaml
# Create custom role with specific permissions
POST /o2ims/v1/auth/roles
{
  "name": "smo-integration",
  "type": "tenant",
  "permissions": [
    "subscriptions:create",
    "subscriptions:read",
    "subscriptions:delete",
    "resources:read",
    "resourcePools:read",
    "deploymentManagers:read"
  ],
  "description": "Role for SMO integration services"
}
```

### Tenant Isolation

Tenants are isolated at multiple levels:

1. **Data Isolation**: Each tenant's data is prefixed and cannot be accessed by others
2. **Resource Isolation**: Quotas prevent resource exhaustion
3. **Network Isolation**: Optional namespace-level network policies
4. **Audit Isolation**: Audit logs are tenant-scoped

### Verifying Tenant Isolation

```bash
# Test that tenant A cannot access tenant B's resources
curl -X GET https://o2ims-gateway.example.com/o2ims/v1/subscriptions \
    --cert tenant-a-client.crt \
    --key tenant-a-client.key

# Should return only tenant A's subscriptions
# Attempting to access tenant B's subscription should return 404
curl -X GET https://o2ims-gateway.example.com/o2ims/v1/subscriptions/tenant-b-sub-123 \
    --cert tenant-a-client.crt \
    --key tenant-a-client.key
# Returns: 404 Not Found
```

### Audit Logging

Enable comprehensive audit logging:

```yaml
# config.yaml
audit:
  enabled: true
  retention_days: 90
  include_request_body: true
  include_response_body: false
  events:
    - auth.*
    - access.denied
    - tenant.*
    - user.*
    - subscription.*
    - resource.*
```

---

## Production Security Checklist

Use this checklist before deploying to production:

### TLS/mTLS

- [ ] TLS 1.3 enforced for all connections
- [ ] Valid CA-signed certificates (not self-signed)
- [ ] mTLS enabled for client authentication
- [ ] Certificate rotation automated
- [ ] Certificate expiry monitoring configured

### Authentication & Authorization

- [ ] mTLS client verification required
- [ ] API keys have expiration dates
- [ ] Service accounts use workload identity
- [ ] No hardcoded credentials in code/config
- [ ] Default admin credentials changed

### Network Security

- [ ] NetworkPolicies restrict pod communication
- [ ] Ingress rate limiting enabled
- [ ] External IPs allowlisted (if applicable)
- [ ] Egress filtering configured
- [ ] DDoS protection enabled

### Secrets Management

- [ ] Secrets stored in external secret manager
- [ ] Secret rotation automated
- [ ] Secrets not logged or exposed in errors
- [ ] Encryption at rest enabled
- [ ] Access to secrets audited

### RBAC & Multi-Tenancy

- [ ] Least privilege roles assigned
- [ ] Tenant isolation verified
- [ ] Quotas configured per tenant
- [ ] Audit logging enabled
- [ ] Regular access reviews scheduled

### Security Scanning

- [ ] Container images scanned for vulnerabilities
- [ ] Dependencies checked for known CVEs
- [ ] Static code analysis passing
- [ ] Penetration testing completed
- [ ] Security headers configured

### Monitoring & Alerting

- [ ] Security event alerting configured
- [ ] Failed authentication attempts monitored
- [ ] Rate limit violations tracked
- [ ] Certificate expiry alerts set
- [ ] Unusual access patterns detected

### Compliance

- [ ] SOC 2 controls documented
- [ ] ISO 27001 requirements mapped
- [ ] Audit log retention meets requirements
- [ ] Data encryption verified
- [ ] Access controls documented

---

## Security Scanning Commands

Run these commands regularly:

```bash
# Scan container image for vulnerabilities
trivy image o2ims-gateway:latest

# Check Go dependencies for vulnerabilities
govulncheck ./...

# Run static security analysis
gosec -fmt=json -out=gosec-report.json ./...

# Scan Kubernetes manifests
kubesec scan deployment.yaml

# Check for exposed secrets
gitleaks detect --source .
```

---

## Incident Response

### Security Incident Playbook

1. **Detection**: Security event triggers alert
2. **Containment**: Isolate affected components
3. **Investigation**: Analyze audit logs and metrics
4. **Remediation**: Apply fixes and patches
5. **Recovery**: Restore normal operations
6. **Post-Incident**: Document lessons learned

### Useful Commands

```bash
# Check recent security events
curl -X GET "https://o2ims-gateway/o2ims/v1/audit/events?type=access.denied&limit=100" \
    --cert admin.crt --key admin.key

# Revoke compromised API key
curl -X DELETE "https://o2ims-gateway/o2ims/v1/auth/api-keys/compromised-key-id" \
    --cert admin.crt --key admin.key

# Suspend tenant (if compromised)
curl -X PATCH "https://o2ims-gateway/o2ims/v1/tenants/tenant-id" \
    -d '{"status": "suspended"}' \
    --cert admin.crt --key admin.key
```

---

## References

- [O-RAN O2 IMS Specification](https://specifications.o-ran.org/)
- [NIST Cybersecurity Framework](https://www.nist.gov/cyberframework)
- [CIS Kubernetes Benchmark](https://www.cisecurity.org/benchmark/kubernetes)
- [OWASP API Security Top 10](https://owasp.org/www-project-api-security/)
- [Cloud Native Security Whitepaper](https://github.com/cncf/tag-security/blob/main/security-whitepaper/cloud-native-security-whitepaper.md)
