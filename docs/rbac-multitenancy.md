# RBAC and Multi-Tenancy Architecture

**Version:** 1.0
**Date:** 2026-01-06
**Status:** Design Specification

## Table of Contents

1. [Overview](#overview)
2. [Multi-Tenancy Model](#multi-tenancy-model)
3. [RBAC Architecture](#rbac-architecture)
4. [Authentication](#authentication)
5. [Authorization](#authorization)
6. [Tenant Isolation](#tenant-isolation)
7. [API Design](#api-design)
8. [Implementation Guide](#implementation-guide)
9. [Security Considerations](#security-considerations)

---

## Overview

### Design Goals

The netweave O2-IMS Gateway is designed with **enterprise-grade multi-tenancy and RBAC** from the ground up to support:

1. **Multi-Tenant SMO Environments**: Multiple SMO systems sharing the same O2-IMS gateway
2. **Role-Based Access Control**: Fine-grained permissions based on roles
3. **Resource Isolation**: Strict tenant boundaries preventing cross-tenant access
4. **Scalability**: Support for 100+ tenants with thousands of resources per tenant
5. **Compliance**: Audit logging for all operations with tenant context

### Key Principles

- ✅ **Zero-Trust Security**: Every request is authenticated and authorized
- ✅ **Tenant Isolation by Default**: Resources are isolated unless explicitly shared
- ✅ **Least Privilege**: Users have minimum permissions needed for their role
- ✅ **Immutable Audit Trail**: All operations logged with tenant and user context
- ✅ **Backward Compatible**: Can operate in single-tenant mode for simple deployments

---

## Multi-Tenancy Model

### Tenancy Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    netweave O2-IMS Gateway                  │
│                                                             │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │  Tenant A    │  │  Tenant B    │  │  Tenant C    │     │
│  │  (SMO-Alpha) │  │  (SMO-Beta)  │  │  (SMO-Gamma) │     │
│  │              │  │              │  │              │     │
│  │ • Users      │  │ • Users      │  │ • Users      │     │
│  │ • Roles      │  │ • Roles      │  │ • Roles      │     │
│  │ • Resources  │  │ • Resources  │  │ • Resources  │     │
│  │ • Policies   │  │ • Policies   │  │ • Policies   │     │
│  └──────────────┘  └──────────────┘  └──────────────┘     │
│                                                             │
└─────────────────────────────────────────────────────────────┘
         │                    │                    │
         ▼                    ▼                    ▼
    ┌─────────┐          ┌─────────┐          ┌─────────┐
    │ O-Cloud │          │ O-Cloud │          │ O-Cloud │
    │    A    │          │    B    │          │    C    │
    └─────────┘          └─────────┘          └─────────┘
```

### Tenant Data Model

```go
// internal/tenant/tenant.go

package tenant

import (
    "time"
)

// Tenant represents an isolated customer environment
type Tenant struct {
    // Identity
    TenantID    string    `json:"tenantId"`    // Unique identifier (UUID)
    Name        string    `json:"name"`         // Display name (e.g., "SMO Alpha")
    DisplayName string    `json:"displayName"`  // Human-readable name

    // Organizational
    Organization string   `json:"organization"` // Organization name
    ContactEmail string   `json:"contactEmail"` // Primary contact

    // Configuration
    Status       TenantStatus `json:"status"`   // active, suspended, deleted
    Settings     TenantSettings `json:"settings"`

    // Resource Limits (quotas)
    Quotas       ResourceQuotas `json:"quotas"`

    // Metadata
    CreatedAt    time.Time `json:"createdAt"`
    UpdatedAt    time.Time `json:"updatedAt"`
    CreatedBy    string    `json:"createdBy"`  // User who created tenant

    // Extensions
    Extensions   map[string]interface{} `json:"extensions,omitempty"`
}

type TenantStatus string

const (
    TenantStatusActive    TenantStatus = "active"
    TenantStatusSuspended TenantStatus = "suspended"
    TenantStatusDeleted   TenantStatus = "deleted"
)

type TenantSettings struct {
    // Isolation mode
    IsolationLevel  IsolationLevel `json:"isolationLevel"`

    // Resource sharing
    AllowSharedResources bool `json:"allowSharedResources"`

    // Defaults
    DefaultOCloudID string `json:"defaultOCloudId,omitempty"`
}

type IsolationLevel string

const (
    IsolationStrict  IsolationLevel = "strict"   // No resource sharing
    IsolationShared  IsolationLevel = "shared"   // Allow shared resource pools
    IsolationHybrid  IsolationLevel = "hybrid"   // Mix of strict and shared
)

type ResourceQuotas struct {
    MaxResourcePools     int `json:"maxResourcePools"`
    MaxResources         int `json:"maxResources"`
    MaxSubscriptions     int `json:"maxSubscriptions"`
    MaxDeploymentManagers int `json:"maxDeploymentManagers"`

    // Compute quotas
    MaxCPUCores     int `json:"maxCpuCores,omitempty"`
    MaxMemoryGB     int `json:"maxMemoryGb,omitempty"`
    MaxStorageGB    int `json:"maxStorageGb,omitempty"`
}
```

### Tenant Identification

**Three methods for identifying tenants in requests:**

#### Method 1: Client Certificate (Recommended for Production)

```go
// Extract tenant from client certificate CN or SAN
func extractTenantFromCert(cert *x509.Certificate) string {
    // CN format: "smo-alpha.tenant.o2ims.example.com"
    cn := cert.Subject.CommonName

    // Extract tenant ID from CN
    parts := strings.Split(cn, ".")
    if len(parts) >= 2 && parts[1] == "tenant" {
        return parts[0] // "smo-alpha"
    }

    // Fallback: Check SANs
    for _, san := range cert.DNSNames {
        if strings.Contains(san, ".tenant.") {
            parts := strings.Split(san, ".")
            if len(parts) >= 2 {
                return parts[0]
            }
        }
    }

    return ""
}
```

#### Method 2: Custom HTTP Header

```bash
# Client sends tenant ID in header
curl -X GET https://netweave.example.com/o2ims/v1/resourcePools \
  --cert client.crt --key client.key --cacert ca.crt \
  -H "X-Tenant-ID: smo-alpha"
```

```go
func extractTenantFromHeader(r *http.Request) string {
    return r.Header.Get("X-Tenant-ID")
}
```

#### Method 3: URL Path (API v3+)

```bash
# Tenant ID in URL path
GET /o2ims/v3/tenants/smo-alpha/resourcePools
```

### Tenant Middleware

```go
// internal/middleware/tenant.go

package middleware

import (
    "context"
    "net/http"
    "github.com/gin-gonic/gin"
    "github.com/yourorg/netweave/internal/tenant"
)

type TenantMiddleware struct {
    store tenant.Store
}

func NewTenantMiddleware(store tenant.Store) *TenantMiddleware {
    return &TenantMiddleware{store: store}
}

// ExtractTenant extracts and validates tenant from request
func (m *TenantMiddleware) ExtractTenant() gin.HandlerFunc {
    return func(c *gin.Context) {
        // 1. Extract tenant ID from request
        tenantID := m.extractTenantID(c)
        if tenantID == "" {
            c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
                "error": "missing tenant identifier",
            })
            return
        }

        // 2. Load tenant from store
        tenant, err := m.store.Get(c.Request.Context(), tenantID)
        if err != nil {
            c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
                "error": "tenant not found",
            })
            return
        }

        // 3. Validate tenant status
        if tenant.Status != tenant.TenantStatusActive {
            c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
                "error": "tenant is not active",
                "status": tenant.Status,
            })
            return
        }

        // 4. Store tenant in context
        ctx := context.WithValue(c.Request.Context(), "tenant", tenant)
        c.Request = c.Request.WithContext(ctx)

        // Also set in gin context for convenience
        c.Set("tenant", tenant)
        c.Set("tenantId", tenantID)

        c.Next()
    }
}

func (m *TenantMiddleware) extractTenantID(c *gin.Context) string {
    // Priority 1: Client certificate
    if c.Request.TLS != nil && len(c.Request.TLS.PeerCertificates) > 0 {
        if tenantID := extractTenantFromCert(c.Request.TLS.PeerCertificates[0]); tenantID != "" {
            return tenantID
        }
    }

    // Priority 2: Custom header
    if tenantID := c.GetHeader("X-Tenant-ID"); tenantID != "" {
        return tenantID
    }

    // Priority 3: URL path (v3+ API)
    if tenantID := c.Param("tenantId"); tenantID != "" {
        return tenantID
    }

    return ""
}
```

---

## RBAC Architecture

### Role Hierarchy

```
┌─────────────────────────────────────────────────────────┐
│                    System Roles                          │
│  (Cross-tenant, platform administration)                │
│                                                          │
│  • PlatformAdmin  - Full system access                  │
│  • TenantAdmin    - Create/manage tenants               │
│  • Auditor        - Read-only audit access              │
└─────────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────┐
│                    Tenant Roles                          │
│  (Scoped to specific tenant)                            │
│                                                          │
│  • Owner          - Full tenant access                  │
│  • Admin          - Manage users, resources, policies   │
│  • Operator       - CRUD on resources                   │
│  • Viewer         - Read-only access                    │
│  • Custom Roles   - User-defined permissions            │
└─────────────────────────────────────────────────────────┘
```

### Permission Model

```go
// internal/rbac/rbac.go

package rbac

// Permission represents a specific action on a resource
type Permission struct {
    Resource string   `json:"resource"` // "ResourcePool", "Resource", "Subscription"
    Action   Action   `json:"action"`   // "create", "read", "update", "delete", "list"
    Scope    Scope    `json:"scope"`    // "tenant", "shared", "all"
}

type Action string

const (
    ActionCreate  Action = "create"
    ActionRead    Action = "read"
    ActionUpdate  Action = "update"
    ActionDelete  Action = "delete"
    ActionList    Action = "list"

    // Special actions
    ActionManage  Action = "manage"  // All CRUD operations
    ActionExecute Action = "execute" // Execute operations (scale, rollback, etc.)
)

type Scope string

const (
    ScopeTenant Scope = "tenant"  // Only tenant's resources
    ScopeShared Scope = "shared"  // Shared resources across tenants
    ScopeAll    Scope = "all"     // All resources (system admin)
)

// Role defines a set of permissions
type Role struct {
    RoleID      string       `json:"roleId"`
    Name        string       `json:"name"`
    Description string       `json:"description"`
    TenantID    string       `json:"tenantId,omitempty"` // Empty for system roles
    Permissions []Permission `json:"permissions"`
    IsSystem    bool         `json:"isSystem"`           // Built-in role
    CreatedAt   time.Time    `json:"createdAt"`
    UpdatedAt   time.Time    `json:"updatedAt"`
}

// User represents an authenticated user
type User struct {
    UserID       string    `json:"userId"`
    Username     string    `json:"username"`
    Email        string    `json:"email"`
    TenantID     string    `json:"tenantId,omitempty"` // Primary tenant
    Roles        []string  `json:"roles"`              // Role IDs
    Enabled      bool      `json:"enabled"`
    CreatedAt    time.Time `json:"createdAt"`
    LastLoginAt  time.Time `json:"lastLoginAt,omitempty"`
}

// RoleBinding associates users with roles
type RoleBinding struct {
    BindingID   string    `json:"bindingId"`
    UserID      string    `json:"userId"`
    RoleID      string    `json:"roleId"`
    TenantID    string    `json:"tenantId"`           // Tenant scope
    ResourceID  string    `json:"resourceId,omitempty"` // Optional: role for specific resource
    CreatedAt   time.Time `json:"createdAt"`
    CreatedBy   string    `json:"createdBy"`
}
```

### Built-in System Roles

```yaml
# System Roles (cross-tenant)

- roleId: platform-admin
  name: Platform Administrator
  isSystem: true
  permissions:
    # Full access to all resources across all tenants
    - resource: "*"
      action: manage
      scope: all

    # Tenant management
    - resource: Tenant
      action: create
      scope: all
    - resource: Tenant
      action: delete
      scope: all

- roleId: tenant-admin
  name: Tenant Administrator
  isSystem: true
  permissions:
    # Can create and manage tenants
    - resource: Tenant
      action: create
      scope: all
    - resource: Tenant
      action: read
      scope: all
    - resource: Tenant
      action: update
      scope: all

- roleId: auditor
  name: System Auditor
  isSystem: true
  permissions:
    # Read-only access to all resources
    - resource: "*"
      action: read
      scope: all
    - resource: "*"
      action: list
      scope: all

    # Access to audit logs
    - resource: AuditLog
      action: read
      scope: all
```

### Built-in Tenant Roles

```yaml
# Tenant Roles (scoped to specific tenant)

- roleId: owner
  name: Tenant Owner
  isSystem: true
  permissions:
    # Full control within tenant
    - resource: "*"
      action: manage
      scope: tenant

    # User management
    - resource: User
      action: manage
      scope: tenant
    - resource: RoleBinding
      action: manage
      scope: tenant

- roleId: admin
  name: Tenant Administrator
  isSystem: true
  permissions:
    # Manage resources
    - resource: ResourcePool
      action: manage
      scope: tenant
    - resource: Resource
      action: manage
      scope: tenant
    - resource: Subscription
      action: manage
      scope: tenant

    # Manage users (but not role assignments)
    - resource: User
      action: read
      scope: tenant
    - resource: User
      action: update
      scope: tenant

- roleId: operator
  name: Operator
  isSystem: true
  permissions:
    # CRUD on resources
    - resource: ResourcePool
      action: create
      scope: tenant
    - resource: ResourcePool
      action: read
      scope: tenant
    - resource: ResourcePool
      action: update
      scope: tenant
    - resource: ResourcePool
      action: delete
      scope: tenant

    - resource: Resource
      action: manage
      scope: tenant

    - resource: Subscription
      action: manage
      scope: tenant

- roleId: viewer
  name: Viewer
  isSystem: true
  permissions:
    # Read-only access
    - resource: "*"
      action: read
      scope: tenant
    - resource: "*"
      action: list
      scope: tenant
```

### Custom Roles

```go
// Example: Create custom role for specific use case

func createCNFManagerRole(tenantID string) *Role {
    return &Role{
        RoleID:      uuid.New().String(),
        Name:        "CNF Manager",
        Description: "Manage CNF deployments but not infrastructure",
        TenantID:    tenantID,
        IsSystem:    false,
        Permissions: []Permission{
            // Read infrastructure
            {Resource: "ResourcePool", Action: ActionRead, Scope: ScopeTenant},
            {Resource: "Resource", Action: ActionRead, Scope: ScopeTenant},

            // Manage deployments (O2-DMS)
            {Resource: "Deployment", Action: ActionManage, Scope: ScopeTenant},
            {Resource: "DeploymentPackage", Action: ActionManage, Scope: ScopeTenant},

            // Manage subscriptions for deployments
            {Resource: "Subscription", Action: ActionCreate, Scope: ScopeTenant},
            {Resource: "Subscription", Action: ActionRead, Scope: ScopeTenant},
            {Resource: "Subscription", Action: ActionDelete, Scope: ScopeTenant},
        },
    }
}
```

---

## Authentication

### mTLS-Based Authentication

**Primary authentication method** for production deployments:

```go
// internal/auth/mtls.go

package auth

import (
    "crypto/x509"
    "fmt"
)

type MTLSAuthenticator struct {
    trustedCAs *x509.CertPool
}

func NewMTLSAuthenticator(caCertPath string) (*MTLSAuthenticator, error) {
    caCert, err := os.ReadFile(caCertPath)
    if err != nil {
        return nil, err
    }

    caCertPool := x509.NewCertPool()
    caCertPool.AppendCertsFromPEM(caCert)

    return &MTLSAuthenticator{
        trustedCAs: caCertPool,
    }, nil
}

// Authenticate extracts user identity from client certificate
func (a *MTLSAuthenticator) Authenticate(cert *x509.Certificate) (*AuthContext, error) {
    // 1. Verify certificate is signed by trusted CA
    opts := x509.VerifyOptions{
        Roots:     a.trustedCAs,
        KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
    }

    if _, err := cert.Verify(opts); err != nil {
        return nil, fmt.Errorf("certificate verification failed: %w", err)
    }

    // 2. Extract user identity from certificate
    // CN format: "user-id.tenant-id.o2ims.example.com"
    cn := cert.Subject.CommonName
    parts := strings.Split(cn, ".")

    if len(parts) < 2 {
        return nil, fmt.Errorf("invalid certificate CN format")
    }

    userID := parts[0]
    tenantID := parts[1]

    // 3. Extract additional claims from certificate extensions
    claims := extractCertificateClaims(cert)

    return &AuthContext{
        UserID:   userID,
        TenantID: tenantID,
        Method:   "mtls",
        Claims:   claims,
    }, nil
}

type AuthContext struct {
    UserID   string
    TenantID string
    Method   string
    Claims   map[string]interface{}
}

func extractCertificateClaims(cert *x509.Certificate) map[string]interface{} {
    claims := make(map[string]interface{})

    // Extract email from SAN
    if len(cert.EmailAddresses) > 0 {
        claims["email"] = cert.EmailAddresses[0]
    }

    // Extract organization
    if len(cert.Subject.Organization) > 0 {
        claims["organization"] = cert.Subject.Organization[0]
    }

    return claims
}
```

### Certificate Issuance for Multi-Tenancy

```yaml
# cert-manager Certificate for tenant user

apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: smo-alpha-operator-cert
  namespace: o2ims-system
spec:
  secretName: smo-alpha-operator-tls
  duration: 2160h  # 90 days
  renewBefore: 360h  # 15 days

  # Subject configuration
  subject:
    organizations:
      - "SMO Alpha Inc"
  commonName: "operator-1.smo-alpha.o2ims.example.com"

  # DNS SANs
  dnsNames:
    - "operator-1.tenant.smo-alpha"

  # Email SANs
  emailAddresses:
    - "operator1@smo-alpha.example.com"

  # Key usage
  usages:
    - client auth

  # Issuer
  issuerRef:
    name: client-ca-issuer
    kind: ClusterIssuer
```

---

## Authorization

### Authorization Engine

```go
// internal/authz/authz.go

package authz

import (
    "context"
    "fmt"
    "github.com/yourorg/netweave/internal/rbac"
)

type Authorizer struct {
    roleStore    rbac.RoleStore
    bindingStore rbac.BindingStore
}

func NewAuthorizer(roleStore rbac.RoleStore, bindingStore rbac.BindingStore) *Authorizer {
    return &Authorizer{
        roleStore:    roleStore,
        bindingStore: bindingStore,
    }
}

// Authorize checks if user can perform action on resource
func (a *Authorizer) Authorize(
    ctx context.Context,
    userID string,
    tenantID string,
    resource string,
    action rbac.Action,
    resourceID string,
) (bool, error) {
    // 1. Get user's role bindings
    bindings, err := a.bindingStore.GetUserBindings(ctx, userID, tenantID)
    if err != nil {
        return false, err
    }

    if len(bindings) == 0 {
        return false, nil // No roles = no permissions
    }

    // 2. Collect all permissions from roles
    permissions := make([]rbac.Permission, 0)
    for _, binding := range bindings {
        role, err := a.roleStore.Get(ctx, binding.RoleID)
        if err != nil {
            continue
        }
        permissions = append(permissions, role.Permissions...)
    }

    // 3. Check if any permission allows the action
    return a.matchPermission(permissions, resource, action, tenantID, resourceID), nil
}

func (a *Authorizer) matchPermission(
    permissions []rbac.Permission,
    resource string,
    action rbac.Action,
    tenantID string,
    resourceID string,
) bool {
    for _, perm := range permissions {
        // Match resource (exact or wildcard)
        if perm.Resource != "*" && perm.Resource != resource {
            continue
        }

        // Match action (exact or "manage" which includes all)
        if perm.Action != action && perm.Action != rbac.ActionManage {
            continue
        }

        // Match scope
        switch perm.Scope {
        case rbac.ScopeAll:
            // System admin - always allowed
            return true
        case rbac.ScopeTenant:
            // Allowed for tenant's resources
            return true
        case rbac.ScopeShared:
            // Check if resource is marked as shared
            // (would need to query resource metadata)
            return true
        }
    }

    return false
}
```

### Authorization Middleware

```go
// internal/middleware/authz.go

package middleware

import (
    "net/http"
    "github.com/gin-gonic/gin"
    "github.com/yourorg/netweave/internal/authz"
    "github.com/yourorg/netweave/internal/rbac"
)

type AuthzMiddleware struct {
    authorizer *authz.Authorizer
}

func NewAuthzMiddleware(authorizer *authz.Authorizer) *AuthzMiddleware {
    return &AuthzMiddleware{authorizer: authorizer}
}

// RequirePermission creates middleware requiring specific permission
func (m *AuthzMiddleware) RequirePermission(resource string, action rbac.Action) gin.HandlerFunc {
    return func(c *gin.Context) {
        // Extract user and tenant from context
        userID := c.GetString("userId")
        tenantID := c.GetString("tenantId")

        if userID == "" || tenantID == "" {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
                "error": "missing authentication context",
            })
            return
        }

        // Get resource ID from URL if present
        resourceID := c.Param("id")

        // Check authorization
        allowed, err := m.authorizer.Authorize(
            c.Request.Context(),
            userID,
            tenantID,
            resource,
            action,
            resourceID,
        )

        if err != nil {
            c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
                "error": "authorization check failed",
            })
            return
        }

        if !allowed {
            c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
                "error": "insufficient permissions",
                "required": map[string]string{
                    "resource": resource,
                    "action":   string(action),
                },
            })
            return
        }

        c.Next()
    }
}
```

### API Endpoint Protection

```go
// internal/server/routes.go

func (s *Server) setupProtectedRoutes() {
    v1 := s.router.Group("/o2ims/v1")
    v1.Use(s.tenantMiddleware.ExtractTenant())
    v1.Use(s.authMiddleware.Authenticate())

    // Resource Pools - require permissions
    v1.GET("/resourcePools",
        s.authzMiddleware.RequirePermission("ResourcePool", rbac.ActionList),
        s.handleListResourcePools)

    v1.GET("/resourcePools/:id",
        s.authzMiddleware.RequirePermission("ResourcePool", rbac.ActionRead),
        s.handleGetResourcePool)

    v1.POST("/resourcePools",
        s.authzMiddleware.RequirePermission("ResourcePool", rbac.ActionCreate),
        s.handleCreateResourcePool)

    v1.PUT("/resourcePools/:id",
        s.authzMiddleware.RequirePermission("ResourcePool", rbac.ActionUpdate),
        s.handleUpdateResourcePool)

    v1.DELETE("/resourcePools/:id",
        s.authzMiddleware.RequirePermission("ResourcePool", rbac.ActionDelete),
        s.handleDeleteResourcePool)

    // Resources
    v1.GET("/resources",
        s.authzMiddleware.RequirePermission("Resource", rbac.ActionList),
        s.handleListResources)

    v1.POST("/resources",
        s.authzMiddleware.RequirePermission("Resource", rbac.ActionCreate),
        s.handleCreateResource)

    v1.DELETE("/resources/:id",
        s.authzMiddleware.RequirePermission("Resource", rbac.ActionDelete),
        s.handleDeleteResource)

    // Subscriptions
    v1.POST("/subscriptions",
        s.authzMiddleware.RequirePermission("Subscription", rbac.ActionCreate),
        s.handleCreateSubscription)
}
```

---

## Tenant Isolation

### Resource Filtering

All list operations MUST filter by tenant:

```go
// internal/handlers/resource_pools.go

func (h *Handler) ListResourcePools(c *gin.Context) {
    // Extract tenant from context
    tenant, _ := c.Get("tenant")
    tenantObj := tenant.(*tenant.Tenant)

    // Parse filter from query params
    filter := parseFilter(c)

    // CRITICAL: Add tenant filter
    filter.TenantID = tenantObj.TenantID

    // Route to backend
    adapter, err := h.registry.Route("ResourcePool", filter)
    if err != nil {
        c.JSON(500, gin.H{"error": "routing failed"})
        return
    }

    // List resources (backend will filter by tenant)
    pools, err := adapter.ListResourcePools(c.Request.Context(), filter)
    if err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }

    c.JSON(200, pools)
}
```

### Backend Tenant Isolation

Kubernetes adapter enforces tenant isolation via labels:

```go
// internal/adapters/k8s/resourcepools.go

func (a *KubernetesAdapter) ListResourcePools(
    ctx context.Context,
    filter *adapter.Filter,
) ([]*adapter.ResourcePool, error) {
    // List MachineSets with tenant label selector
    listOpts := &client.ListOptions{
        LabelSelector: labels.SelectorFromSet(labels.Set{
            "o2ims.oran.org/tenant": filter.TenantID,
        }),
    }

    machineSets := &machinev1beta1.MachineSetList{}
    if err := a.client.List(ctx, machineSets, listOpts); err != nil {
        return nil, err
    }

    // Transform and return
    pools := make([]*adapter.ResourcePool, 0, len(machineSets.Items))
    for i := range machineSets.Items {
        pool := a.transformMachineSetToResourcePool(&machineSets.Items[i])

        // Double-check tenant isolation
        if pool.TenantID != filter.TenantID {
            continue // Skip resources from other tenants
        }

        pools = append(pools, pool)
    }

    return pools, nil
}
```

### Kubernetes Label Strategy

All Kubernetes resources MUST be labeled with tenant ID:

```yaml
apiVersion: machine.openshift.io/v1beta1
kind: MachineSet
metadata:
  name: production-pool
  namespace: openshift-machine-api
  labels:
    # Tenant isolation label (REQUIRED)
    o2ims.oran.org/tenant: smo-alpha

    # O2-IMS resource labels
    o2ims.oran.org/resource-pool-id: pool-123
    o2ims.oran.org/o-cloud-id: ocloud-1
spec:
  replicas: 5
  template:
    metadata:
      labels:
        o2ims.oran.org/tenant: smo-alpha
```

### Cross-Tenant Resource Access Prevention

```go
// internal/handlers/resource_pools.go

func (h *Handler) GetResourcePool(c *gin.Context) {
    poolID := c.Param("id")
    tenantID := c.GetString("tenantId")

    // Route to backend
    adapter, _ := h.registry.Route("ResourcePool", &adapter.Filter{})

    // Get resource pool
    pool, err := adapter.GetResourcePool(c.Request.Context(), poolID)
    if err != nil {
        c.JSON(404, gin.H{"error": "resource pool not found"})
        return
    }

    // CRITICAL: Verify tenant ownership
    if pool.TenantID != tenantID {
        // User is trying to access another tenant's resource
        c.JSON(404, gin.H{"error": "resource pool not found"})
        return
    }

    c.JSON(200, pool)
}
```

---

## API Design

### Multi-Tenant API Endpoints

#### Admin API (System-Level)

```
# Tenant Management
POST   /admin/v1/tenants                    # Create tenant
GET    /admin/v1/tenants                    # List all tenants
GET    /admin/v1/tenants/:id                # Get tenant
PUT    /admin/v1/tenants/:id                # Update tenant
DELETE /admin/v1/tenants/:id                # Delete tenant

# User Management (cross-tenant)
GET    /admin/v1/users                      # List all users
GET    /admin/v1/tenants/:tenantId/users    # List tenant users
POST   /admin/v1/tenants/:tenantId/users    # Create user

# Role Management (system roles)
GET    /admin/v1/roles                      # List system roles
POST   /admin/v1/roles                      # Create system role
PUT    /admin/v1/roles/:id                  # Update system role

# Audit Logs
GET    /admin/v1/audit                      # Query audit logs
```

#### O2-IMS API (Tenant-Scoped)

```
# Automatically scoped to authenticated tenant

# Resource Pools
GET    /o2ims/v1/resourcePools              # List (tenant's only)
POST   /o2ims/v1/resourcePools              # Create (in tenant)
GET    /o2ims/v1/resourcePools/:id          # Get (tenant check)
PUT    /o2ims/v1/resourcePools/:id          # Update (tenant check)
DELETE /o2ims/v1/resourcePools/:id          # Delete (tenant check)

# Resources
GET    /o2ims/v1/resources                  # List (tenant's only)
POST   /o2ims/v1/resources                  # Create (in tenant)
DELETE /o2ims/v1/resources/:id              # Delete (tenant check)

# Subscriptions
GET    /o2ims/v1/subscriptions              # List (tenant's only)
POST   /o2ims/v1/subscriptions              # Create (in tenant)
DELETE /o2ims/v1/subscriptions/:id          # Delete (tenant check)
```

#### Tenant API (v3+ with explicit tenant in URL)

```
# Explicit tenant scoping in URL

GET    /o2ims/v3/tenants/:tenantId/resourcePools
POST   /o2ims/v3/tenants/:tenantId/resourcePools
GET    /o2ims/v3/tenants/:tenantId/resourcePools/:id

# User management (tenant-scoped)
GET    /o2ims/v3/tenants/:tenantId/users
POST   /o2ims/v3/tenants/:tenantId/users
GET    /o2ims/v3/tenants/:tenantId/users/:userId

# Role management (tenant-scoped)
GET    /o2ims/v3/tenants/:tenantId/roles
POST   /o2ims/v3/tenants/:tenantId/roles
PUT    /o2ims/v3/tenants/:tenantId/roles/:roleId

# Role bindings
GET    /o2ims/v3/tenants/:tenantId/roleBindings
POST   /o2ims/v3/tenants/:tenantId/roleBindings
DELETE /o2ims/v3/tenants/:tenantId/roleBindings/:bindingId
```

---

## Implementation Guide

### Phase 1: Core Multi-Tenancy (Week 1-2)

**Deliverables**:
1. Tenant data model and store (Redis)
2. Tenant middleware (extraction and validation)
3. Resource labeling strategy for Kubernetes
4. Tenant filtering in all adapters

**Implementation Steps**:

```bash
# 1. Create tenant package
mkdir -p internal/tenant
touch internal/tenant/{tenant.go,store.go,store_redis.go,middleware.go}

# 2. Update adapter interface to include tenant filtering
# Edit: internal/adapter/adapter.go

# 3. Implement tenant store
# Edit: internal/tenant/store_redis.go

# 4. Add tenant middleware to server
# Edit: internal/server/server.go

# 5. Update all handlers to filter by tenant
# Edit: internal/handlers/*.go
```

### Phase 2: RBAC Foundation (Week 3-4)

**Deliverables**:
1. RBAC data models (Role, Permission, User, RoleBinding)
2. Role and binding stores (Redis)
3. Built-in system and tenant roles
4. Authorization engine

**Implementation Steps**:

```bash
# 1. Create RBAC package
mkdir -p internal/rbac
touch internal/rbac/{rbac.go,role_store.go,binding_store.go,builtin_roles.go}

# 2. Create authorization package
mkdir -p internal/authz
touch internal/authz/{authz.go,middleware.go}

# 3. Implement authorization middleware
# Edit: internal/middleware/authz.go

# 4. Protect all API endpoints
# Edit: internal/server/routes.go
```

### Phase 3: Admin API (Week 5-6)

**Deliverables**:
1. Admin API handlers (tenant, user, role management)
2. API endpoints for admin operations
3. Audit logging for all operations

**Implementation Steps**:

```bash
# 1. Create admin handlers
mkdir -p internal/handlers/admin
touch internal/handlers/admin/{tenants.go,users.go,roles.go,audit.go}

# 2. Add admin routes
# Edit: internal/server/routes_admin.go

# 3. Implement audit logging
mkdir -p internal/audit
touch internal/audit/{audit.go,logger.go}
```

### Phase 4: Testing & Documentation (Week 7-8)

**Deliverables**:
1. Unit tests for RBAC and multi-tenancy
2. Integration tests for tenant isolation
3. E2E tests for authorization flows
4. Documentation updates

---

## Security Considerations

### Threat Model

| Threat | Mitigation |
|--------|-----------|
| **Cross-Tenant Data Access** | Label-based filtering, tenant verification on all operations |
| **Privilege Escalation** | Immutable system roles, role binding validation |
| **Tenant Enumeration** | No tenant list endpoint for non-admins, opaque error messages |
| **Session Hijacking** | mTLS-based auth (no sessions), short-lived tokens if using JWT |
| **Resource Exhaustion** | Per-tenant quotas, rate limiting per tenant |

### Security Best Practices

1. **Defense in Depth**:
   - Tenant filtering at API layer
   - Tenant filtering at adapter layer
   - Kubernetes RBAC for backend isolation
   - Network policies for pod-level isolation

2. **Least Privilege**:
   - Users start with no permissions
   - Explicit role bindings required
   - Built-in roles follow principle of least privilege

3. **Audit Everything**:
   - Log all tenant operations
   - Log all authorization decisions
   - Log all admin operations

4. **Secure Defaults**:
   - Tenants created in suspended state
   - New users have no roles by default
   - Strict isolation mode by default

---

**Next Steps**: Integrate RBAC and multi-tenancy into architecture documentation and begin Phase 1 implementation.
