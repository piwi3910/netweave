# Security Policy

## Reporting Security Vulnerabilities

**DO NOT** open public GitHub issues for security vulnerabilities.

Instead, please report security issues privately:

1. **Email**: security@netweave.io (if available)
2. **GitHub Security Advisory**: Use [Private Vulnerability Reporting](https://github.com/piwi3910/netweave/security/advisories/new)

We will acknowledge your report within 48 hours and provide a detailed response within 5 business days.

## Supported Versions

We provide security updates for the following versions:

| Version | Supported          |
| ------- | ------------------ |
| main    | :white_check_mark: |
| < 1.0   | :x:                |

## Security Best Practices

### Production Deployment Requirements

**CRITICAL**: The following security measures are **MANDATORY** for production deployments:

#### 1. Vault Auto-Unseal with Cloud KMS (CRITICAL)

🔴 **NEVER use Kubernetes secrets for Vault unseal keys in production.**

The development setup stores unseal keys in `vault-unseal-keys` Kubernetes secret. This is **ONLY suitable for development/testing** and is fundamentally insecure:

- Kubernetes secrets are only base64-encoded (NOT encrypted)
- Anyone with cluster-admin access can read unseal keys
- Compromised unseal keys = complete access to all Vault data
- Root token stored alongside keys compounds the risk

**Required for Production:**

Implement Vault auto-unseal with cloud KMS:

- **AWS KMS**: Use AWS Key Management Service with IRSA (IAM Roles for Service Accounts)
- **Azure Key Vault**: Use Azure Key Vault with Workload Identity
- **GCP Cloud KMS**: Use Google Cloud KMS with Workload Identity Federation

**Implementation**: See Issue #299 for detailed implementation guide.

**References**:
- [Vault Auto-Unseal Documentation](https://developer.hashicorp.com/vault/docs/concepts/seal#auto-unseal)
- [AWS KMS Seal](https://developer.hashicorp.com/vault/docs/configuration/seal/awskms)
- [Azure Key Vault Seal](https://developer.hashicorp.com/vault/docs/configuration/seal/azurekeyvault)
- [GCP Cloud KMS Seal](https://developer.hashicorp.com/vault/docs/configuration/seal/gcpckms)

#### 2. TLS/mTLS Configuration

✅ **Always use TLS 1.3** for all communications:

- Vault API: TLS 1.3 with proper certificate verification (VAULT_CACERT configured)
- Gateway API: mTLS with certificate-based authentication
- Keycloak: TLS 1.3 for all external communications

❌ **NEVER use VAULT_SKIP_VERIFY=true** in production (only for development)

#### 3. Network Security

**NetworkPolicy**: Apply Kubernetes NetworkPolicies to:
- Restrict pod-to-pod communication to necessary paths only
- Limit egress to required external services
- Deny all by default, allow explicitly

**Service Mesh**: Consider using Istio or Linkerd for:
- Automatic mTLS between services
- Fine-grained traffic policies
- Observability and tracing

#### 4. Secret Management

**Kubernetes Secrets Encryption**:
- Enable encryption at rest for etcd
- Use KMS provider for secret encryption (AWS KMS, Azure Key Vault, GCP KMS)
- Rotate encryption keys regularly

**Certificate Management**:
- Automated certificate lifecycle with Vault PKI
- Short-lived certificates (1 year max TTL)
- Automatic renewal before expiry
- Revocation support for compromised certificates

#### 5. Access Control

**RBAC**: Implement least-privilege RBAC:
- Separate roles for gateway, Keycloak, administrators
- Service-specific service accounts
- No cluster-admin for application workloads

**Pod Security Standards**:
- Apply `restricted` Pod Security Standard
- Run as non-root user (UID > 0)
- Read-only root filesystem
- Drop all capabilities
- No privilege escalation

#### 6. Audit Logging

**Vault Audit Logs**:
- Enable file audit backend (automatically enabled in deployment)
- Ship logs to external SIEM
- Retain logs for compliance period (typically 1+ year)

**Kubernetes Audit Logs**:
- Enable Kubernetes audit logging
- Monitor access to secrets and certificates
- Alert on suspicious activities

#### 7. Image Security

**Container Images**:
- Use official images from trusted registries
- Pin images to specific digests (not tags)
- Scan images for vulnerabilities (Trivy, Snyk)
- No images from unknown sources

**Supply Chain Security**:
- Verify image signatures (Sigstore/Cosign)
- Generate and maintain SBOM (Software Bill of Materials)
- Monitor for supply chain attacks

#### 8. Monitoring and Alerting

**Security Monitoring**:
- Vault seal status (critical alert if sealed)
- Certificate expiry (alert 30 days before)
- Failed authentication attempts
- Anomalous API access patterns

**Prometheus Metrics**:
- `vault_core_unsealed` - Seal status
- `certmanager_certificate_expirations` - Certificate expiry
- `gateway_auth_failures_total` - Authentication failures

#### 9. Backup and Disaster Recovery

**Vault Backups**:
- Daily Raft snapshots
- Store snapshots in encrypted object storage (S3, Azure Blob, GCS)
- Test restore procedures quarterly
- Encrypt backups with separate keys

**Certificate Backups**:
- CA certificates backed up securely
- Root CA offline in HSM or air-gapped storage
- Recovery procedures documented and tested

#### 10. Incident Response

**Security Incident Plan**:
1. Detect: Monitor logs and metrics for anomalies
2. Contain: Revoke compromised certificates, rotate secrets
3. Investigate: Analyze audit logs, determine scope
4. Remediate: Apply patches, update configurations
5. Document: Post-mortem and lessons learned

**Certificate Compromise**:
- Immediately revoke compromised certificate
- Investigate usage in audit logs
- Rotate all related secrets
- Issue new certificates to affected users

## Security Configuration Checklist

Use this checklist before deploying to production:

### Vault
- [ ] Auto-unseal configured with cloud KMS
- [ ] TLS 1.3 enabled with proper certificates
- [ ] VAULT_SKIP_VERIFY disabled (VAULT_CACERT configured)
- [ ] Audit logging enabled and shipped to SIEM
- [ ] Root token rotated after initial setup
- [ ] Unseal keys removed from Kubernetes secrets
- [ ] NetworkPolicy applied and tested
- [ ] Resource limits configured
- [ ] Raft backups scheduled and tested

### Gateway
- [ ] mTLS enabled for client authentication
- [ ] Certificate validation enforced
- [ ] Rate limiting configured
- [ ] RBAC policies applied
- [ ] Metrics and monitoring configured
- [ ] Audit logging enabled
- [ ] Pod Security Standard: restricted

### Keycloak
- [ ] TLS 1.3 for external access
- [ ] Database connections encrypted
- [ ] Admin console access restricted
- [ ] MFA enabled for administrators
- [ ] Session timeouts configured
- [ ] Password policies enforced

### Kubernetes
- [ ] etcd encryption enabled
- [ ] Pod Security Standards enforced
- [ ] NetworkPolicies applied
- [ ] RBAC least-privilege model
- [ ] Audit logging enabled
- [ ] Secret encryption with KMS

### Monitoring
- [ ] Prometheus metrics exported
- [ ] Critical alerts configured
- [ ] Log aggregation setup
- [ ] Security dashboards created
- [ ] On-call rotation established

## Security Testing

### Vulnerability Scanning

Run security scans before each release:

```bash
# Container image scanning
trivy image o2ims-gateway:latest

# Kubernetes manifest scanning
kubesec scan deployments/kubernetes/

# Dependency scanning
go list -json -m all | nancy sleuth

# Secret scanning
gitleaks detect --source . --verbose
```

### Penetration Testing

Conduct penetration testing:
- Before initial production deployment
- Annually for production systems
- After significant security-related changes

### Security Audits

Schedule regular security audits:
- Quarterly internal reviews
- Annual external security audits
- Compliance audits as required (SOC 2, ISO 27001, etc.)

## Compliance

NetWeave Gateway is designed to support:

- **SOC 2 Type II**: Security controls and monitoring
- **ISO 27001**: Information security management
- **NIST 800-53**: Federal security controls
- **PCI DSS**: If handling payment data
- **HIPAA**: If handling health information

Refer to specific compliance documentation for detailed mappings.

## Security Architecture

For detailed security architecture:
- See [docs/security/architecture.md](docs/security/architecture.md)
- See [ARCHITECTURE_SUMMARY.md](ARCHITECTURE_SUMMARY.md)
- See [deployments/kubernetes/vault/README.md](deployments/kubernetes/vault/README.md)

## Security Updates

We release security updates as needed:

- **Critical**: Within 24 hours
- **High**: Within 1 week
- **Medium**: Next scheduled release
- **Low**: As convenient

Subscribe to [GitHub Security Advisories](https://github.com/piwi3910/netweave/security/advisories) for notifications.

## Contact

For security questions or concerns:
- **Security Team**: security@netweave.io
- **GitHub Security**: [Private Vulnerability Reporting](https://github.com/piwi3910/netweave/security/advisories/new)

---

**Remember**: We build production systems for critical telecom infrastructure. Security is not optional.
