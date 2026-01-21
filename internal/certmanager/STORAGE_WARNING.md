# ⚠️ CRITICAL: In-Memory Storage Limitation ⚠️

## Current State

The certificate manager service currently uses **in-memory storage** for certificate records (see `service.go:23`):

```go
certificates map[string]*Certificate // keyed by serial number
```

## ❌ NOT PRODUCTION READY

This implementation has **CRITICAL limitations** that make it **UNSUITABLE FOR PRODUCTION USE**:

### 1. Data Loss on Restart
- **All certificate records are lost** when the pod restarts
- No persistence = no recovery after crashes
- Certificate tracking (renewal status, attempts) is lost

### 2. No Audit Trail
- Cannot track certificate history across restarts
- No compliance audit capability
- No investigation trail for security incidents

### 3. Multi-Pod Inconsistency
- Each pod has its own certificate map
- Different pods will have different views of certificate state
- Renewal operations may conflict or duplicate

### 4. No Horizontal Scaling
- Cannot add more pods for load distribution
- Single pod = single point of failure
- Manual certificate data migration required

## ✅ Production Requirements

Before deploying to production, implement **ONE** of these solutions:

### Option 1: Redis Persistence (Recommended)
```go
// Matches architecture - already using Redis for subscriptions
type RedisStore struct {
    client *redis.Client
}

// Store certificate
func (s *RedisStore) StoreCertificate(ctx context.Context, cert *Certificate) error {
    data, err := json.Marshal(cert)
    if err != nil {
        return err
    }
    return s.client.Set(ctx, "cert:"+cert.SerialNumber, data, 0).Err()
}
```

**Benefits:**
- Consistent with existing architecture
- Fast access for renewal operations
- Natural fit for distributed systems

### Option 2: PostgreSQL Backend
```go
// For stronger consistency and audit requirements
type PostgresStore struct {
    db *sql.DB
}

CREATE TABLE certificates (
    serial_number VARCHAR(255) PRIMARY KEY,
    user_id VARCHAR(255) NOT NULL,
    tenant_id VARCHAR(255) NOT NULL,
    common_name VARCHAR(255) NOT NULL,
    certificate_pem TEXT NOT NULL,
    issued_at TIMESTAMP NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    status VARCHAR(50) NOT NULL,
    renewal_attempts INTEGER DEFAULT 0,
    INDEX idx_user_id (user_id),
    INDEX idx_tenant_id (tenant_id),
    INDEX idx_status (status),
    INDEX idx_expires_at (expires_at)
);
```

**Benefits:**
- Full audit trail
- ACID transactions
- Better for compliance requirements

### Option 3: Kubernetes Secrets
```go
// Store certificates as Kubernetes secrets
// Only for small-scale deployments
func (s *K8sStore) StoreCertificate(ctx context.Context, cert *Certificate) error {
    secret := &corev1.Secret{
        ObjectMeta: metav1.ObjectMeta{
            Name: "cert-" + cert.SerialNumber,
            Labels: map[string]string{
                "certmanager.netweave.io/user-id": cert.UserID,
                "certmanager.netweave.io/status": string(cert.Status),
            },
        },
        Data: map[string][]byte{
            "certificate": []byte(cert.CertificatePEM),
            "metadata":    marshalledMetadata,
        },
    }
    _, err := s.clientset.CoreV1().Secrets(s.namespace).Create(ctx, secret, metav1.CreateOptions{})
    return err
}
```

**Limitations:**
- Not suitable for high volume (thousands of certificates)
- Slower access patterns
- etcd storage limits

## 🚨 Current Deployment Warning

If deploying the current implementation:

1. **Document the limitation** in deployment guides
2. **Set expectations** with operations team
3. **Plan migration** to persistent storage
4. **Implement backup** strategy (periodic export to object storage)
5. **Monitor** for pod restarts (will lose all certificate tracking)

## Migration Path

When adding persistent storage:

```go
// Add storage interface
type CertificateStore interface {
    Store(ctx context.Context, cert *Certificate) error
    Get(ctx context.Context, serialNumber string) (*Certificate, error)
    List(ctx context.Context, filters map[string]string) ([]*Certificate, error)
    Update(ctx context.Context, cert *Certificate) error
    Delete(ctx context.Context, serialNumber string) error
}

// Update Service struct
type Service struct {
    config         *Config
    vaultClient    *vault.Client
    keycloakClient *keycloak.Client
    logger         *zap.Logger

    // Use interface instead of map
    store          CertificateStore

    // ... rest of fields
}
```

This allows swapping storage implementations without changing business logic.

## References

- Issue #276: Integration tests with real Vault/Keycloak
- PR #294: Certificate automation service
- Architecture Decision: Redis for shared state (subscriptions pattern)
