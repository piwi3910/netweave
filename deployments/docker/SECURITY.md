# Security Considerations for Production Deployment

**⚠️ CRITICAL: This Docker Compose deployment is for DEVELOPMENT ONLY**

The current configuration has security limitations that MUST be addressed before production use.

## 🔴 Critical Security Issues for Production

### 1. Unseal Key Protection

**Current State (Development):**
- Unseal keys stored in `/vault/init/keys.json` file in Docker volume
- Protected only by filesystem permissions
- Anyone with volume access can extract keys

**Required for Production:**
1. **Use Auto-Unseal with Cloud KMS**
   - AWS KMS
   - Azure Key Vault
   - GCP Cloud KMS
   - HSM integration

2. **If Manual Unseal Required:**
   - Extract unseal keys immediately after initialization
   - Store in external HSM or secure key management system
   - Delete `keys.json` file after extraction
   - Distribute keys to separate trusted individuals (Shamir's Secret Sharing)
   - Document key rotation procedures

**Example Auto-Unseal Configuration (AWS KMS):**
```hcl
seal "awskms" {
  region     = "us-west-2"
  kms_key_id = "arn:aws:kms:us-west-2:123456789012:key/12345678-1234-1234-1234-123456789012"
}
```

### 2. Root Token Management

**Current State (Development):**
- Root token stored in `keys.json` alongside unseal keys
- No expiration
- Full unlimited access

**Required for Production:**
1. **Immediately After Initialization:**
   - Extract root token
   - Store in password manager or vault (separate from Vault itself)
   - Revoke root token after initial configuration
   - Use time-limited tokens for admin operations

2. **Operational Use:**
   - Never use root token for normal operations
   - Create role-based policies with minimum required permissions
   - Use short-lived tokens (< 24 hours)
   - Enable MFA for sensitive operations

### 3. TLS Certificate Validation

**Current State (Development):**
- Self-signed certificates in `deployments/docker/*/tls/`
- CA certificate configured via `VAULT_CACERT`
- Suitable for local development only

**Required for Production:**
1. Use certificates from trusted CA (Let's Encrypt, commercial CA)
2. Implement certificate rotation
3. Never use self-signed certificates in production
4. Configure proper certificate validation (no `VAULT_SKIP_VERIFY`)

### 4. Audit Logging

**Current State (Development):**
- No audit logging enabled
- Cannot track who accessed which secrets

**Required for Production:**
1. **Enable File Audit Backend:**
```bash
vault audit enable file file_path=/vault/logs/audit.log
```

2. **Configure Log Rotation:**
- Use external log aggregation (Splunk, ELK, CloudWatch)
- Retain audit logs per compliance requirements
- Alert on suspicious access patterns

3. **Monitor These Events:**
- Failed authentication attempts
- Secret access by service accounts
- Policy changes
- Seal/unseal operations

### 5. Network Security

**Current State (Development):**
- Services exposed on localhost (127.0.0.1)
- Docker bridge network
- No egress filtering

**Required for Production:**
1. **Network Segmentation:**
   - Place Vault in isolated subnet/VLAN
   - Implement network policies (K8s NetworkPolicy, Security Groups)
   - Restrict egress to only required services

2. **Access Control:**
   - Use service mesh (Istio, Linkerd) for mTLS
   - Implement API gateway with rate limiting
   - Configure firewall rules

3. **Monitoring:**
   - Enable Prometheus metrics
   - Alert on anomalous connection patterns
   - Monitor failed authentication attempts

## 🟡 Additional Hardening Recommendations

### Backup and Disaster Recovery

1. **Automated Backups:**
   - Use `vault operator raft snapshot save` for Raft storage
   - Encrypt backups at rest
   - Store backups in geographically separate location
   - Test restoration procedures regularly

2. **High Availability:**
   - Deploy Vault in HA mode (3+ nodes)
   - Use Raft integrated storage or Consul
   - Configure health checks and auto-recovery
   - Document failover procedures

### Secret Rotation

1. **Implement Secret Rotation:**
   - Database credentials: 30-90 days
   - API keys: 90 days
   - Root CA: 10 years, intermediate CA: 1-5 years
   - TLS certificates: 90 days (use Let's Encrypt)

2. **Automated Rotation:**
```bash
# Example: Database secret rotation
vault write database/rotate-root/my-database
```

### Access Policies

1. **Principle of Least Privilege:**
   - Create specific roles for each application
   - Limit token TTL to minimum required
   - Use CIDR binding for tokens
   - Implement namespace isolation

2. **Example Policy (Restrictive):**
```hcl
# Application-specific read-only access
path "pki_int/issue/app-certificates" {
  capabilities = ["create", "update"]
}

path "pki_int/cert/ca" {
  capabilities = ["read"]
}

# Deny all other paths
path "*" {
  capabilities = ["deny"]
}
```

### Compliance and Governance

1. **Regulatory Requirements:**
   - PCI DSS: Audit logging, access controls, encryption
   - HIPAA: PHI encryption, audit trails, access controls
   - SOC 2: Change management, access reviews, incident response

2. **Operational Procedures:**
   - Document break-glass procedures
   - Conduct quarterly access reviews
   - Implement change management process
   - Schedule security assessments

## ✅ Production Deployment Checklist

Before deploying to production, verify:

- [ ] Auto-unseal configured with cloud KMS
- [ ] Root token revoked after initial setup
- [ ] Valid TLS certificates from trusted CA
- [ ] Audit logging enabled and monitored
- [ ] Network policies implemented
- [ ] Backup procedures tested
- [ ] HA configuration deployed (3+ nodes)
- [ ] Monitoring and alerting configured
- [ ] Secret rotation procedures documented
- [ ] Break-glass procedures documented
- [ ] Security assessment completed
- [ ] Compliance requirements validated

## 📚 Additional Resources

- [Vault Production Hardening](https://developer.hashicorp.com/vault/tutorials/operations/production-hardening)
- [Vault Security Model](https://developer.hashicorp.com/vault/docs/internals/security)
- [Auto-Unseal with Cloud KMS](https://developer.hashicorp.com/vault/docs/concepts/seal#auto-unseal)
- [Vault Audit Devices](https://developer.hashicorp.com/vault/docs/audit)
- [Vault Policies](https://developer.hashicorp.com/vault/docs/concepts/policies)
