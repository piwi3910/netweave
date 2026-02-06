# Authentication

**Dual authentication support: mTLS client certificates and OAuth2/OIDC tokens for the O2-IMS Gateway.**

## Table of Contents

1. [Overview](#overview)
2. [mTLS Client Certificates](#mtls-client-certificates)
3. [OAuth2/OIDC Authentication](#oauth2oidc-authentication)
4. [Dual Authentication](#dual-authentication)
5. [Tenant Identification](#tenant-identification)
6. [Certificate Requirements](#certificate-requirements)
7. [Authentication Flow](#authentication-flow)
8. [Client Configuration](#client-configuration)
9. [Testing Authentication](#testing-authentication)

---

## Overview

### Authentication Methods

The O2-IMS Gateway supports **dual authentication** with both mTLS and OAuth2/OIDC:

| Method | Use Case | Security Level | Status |
|--------|----------|----------------|--------|
| **mTLS Client Certs** | Production SMO clients, legacy systems | High | ✅ Implemented |
| **OAuth2/OIDC Tokens** | Modern applications, SSO integration | High | ✅ Implemented |
| **API Keys** | Service accounts (optional) | Medium | 🔄 Future |

### Why mTLS?

- ✅ **Cryptographic Proof**: Stronger than passwords or tokens
- ✅ **No Shared Secrets**: Each client has unique certificate
- ✅ **Revocation**: Instant revocation via CRL or OCSP
- ✅ **Tenant Binding**: Tenant ID embedded in certificate
- ✅ **O-RAN Compliant**: Recommended by O2-IMS specification

### Why OAuth2/OIDC?

- ✅ **Single Sign-On**: Integrate with existing identity providers
- ✅ **Dynamic Tokens**: Short-lived tokens with automatic rotation
- ✅ **User Provisioning**: Automatic user creation from token claims
- ✅ **Group Mapping**: Map identity provider groups to internal roles
- ✅ **Modern Standard**: Industry-standard authentication protocol

### Choosing an Authentication Method

| Scenario | Recommended Method | Rationale |
|----------|-------------------|-----------|
| **Legacy O-RAN systems** | mTLS | Existing certificate infrastructure |
| **Modern web applications** | OAuth2/OIDC | Built-in SSO support |
| **Service-to-service** | mTLS | No user interaction required |
| **Interactive users** | OAuth2/OIDC | Better UX with SSO |
| **Kubernetes pods** | mTLS | Native Vault PKI integration |
| **Enterprise SSO** | OAuth2/OIDC | Leverage existing Keycloak/AD |

---

## mTLS Client Certificates

### Certificate Format

Client certificates use the following subject format (issued by Vault PKI):

```
Subject: CN=<common-name>, O=<organization>, OU=<organizational-unit>

Example (from Vault PKI netweave-client role):
CN=operator-1.smo-alpha.o2ims.example.com, O=SMO Alpha Inc, OU=smo-alpha
```

The gateway extracts this into a normalized subject string using `BuildSubject()`:
```
CN=operator-1.smo-alpha.o2ims.example.com,O=SMO Alpha Inc,OU=smo-alpha
```

### Extracting Identity

```go
// internal/auth/middleware.go

// BuildSubject constructs a normalized certificate subject (DN) string.
// This subject is used as the key to look up users in the auth store.
func (m *Middleware) BuildSubject(cert *CertificateInfo) string {
    parts := []string{"CN=" + cert.CommonName}
    if cert.Organization != "" {
        parts = append(parts, "O="+cert.Organization)
    }
    if cert.OrganizationalUnit != "" {
        parts = append(parts, "OU="+cert.OrganizationalUnit)
    }
    return strings.Join(parts, ",")
}

// After building the subject, the middleware looks up the user:
// user, err := m.store.GetUserBySubject(ctx, subject)
// This returns the TenantUser, which contains TenantID, RoleID, etc.
```

---

## OAuth2/OIDC Authentication

### Overview

OAuth2/OIDC authentication uses **Bearer tokens** issued by an identity provider (Keycloak). Tokens are verified on every request and contain user claims that are mapped to internal roles and tenants.

### Keycloak Integration

The gateway integrates with Keycloak for:
- **Token Verification**: Validate JWT tokens using Keycloak's public keys
- **Claims Extraction**: Extract user information from token claims
- **User Provisioning**: Automatically create users from token claims
- **Group Mapping**: Map Keycloak groups to internal roles

### Token Format

OAuth2/OIDC tokens are JWT (JSON Web Tokens) with standard and custom claims:

```json
{
  "sub": "f:12345678-1234-1234-1234-123456789abc:user@example.com",
  "email": "user@example.com",
  "preferred_username": "user@example.com",
  "name": "John Doe",
  "groups": [
    "/platform-admins",
    "/tenant-admins"
  ],
  "tenant_id": "smo-alpha",
  "iss": "https://keycloak.example.com/realms/netweave",
  "aud": "o2ims-gateway",
  "exp": 1737123456,
  "iat": 1737120000
}
```

**Standard Claims:**
- `sub`: Subject identifier (Keycloak user ID)
- `email`: User's email address
- `preferred_username`: Username for display
- `name`: User's full name
- `iss`: Token issuer (Keycloak realm URL)
- `aud`: Intended audience (client ID)
- `exp`: Token expiration timestamp
- `iat`: Token issued at timestamp

**Custom Claims:**
- `groups`: Keycloak groups for role mapping
- `tenant_id`: Custom claim for tenant association (optional)

### Authentication Request

Clients send Bearer tokens in the Authorization header:

```bash
curl -X GET https://o2ims-gateway.example.com/o2ims-infrastructureInventory/v1/resourcePools \
    -H "Authorization: Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9..."
```

### Token Verification Flow

```go
// internal/auth/oauth2.go
package auth

import (
    "context"
    "fmt"
    "strings"

    "github.com/gin-gonic/gin"
)

type OAuth2Authenticator struct {
    keycloakClient TokenVerifier
    store          Store
    config         *OAuth2Config
    logger         *zap.Logger
}

// Authenticate performs OAuth2 authentication
func (a *OAuth2Authenticator) Authenticate(
    ctx context.Context,
    c *gin.Context,
    requestID string,
) (*TenantUser, *Role, *Tenant, error) {
    // 1. Extract Bearer token
    token, err := a.extractBearerToken(c)
    if err != nil {
        return nil, nil, nil, fmt.Errorf("failed to extract bearer token: %w", err)
    }

    // 2. Verify token with Keycloak
    tokenClaims, err := a.keycloakClient.VerifyToken(ctx, token)
    if err != nil {
        return nil, nil, nil, fmt.Errorf("invalid token: %w", err)
    }

    // 3. Extract structured claims
    claims, err := a.extractClaims(tokenClaims)
    if err != nil {
        return nil, nil, nil, fmt.Errorf("failed to extract claims: %w", err)
    }

    // 4. Get or create user
    user, err := a.getOrCreateUser(ctx, claims, requestID)
    if err != nil {
        return nil, nil, nil, err
    }

    // 5. Load role and tenant
    role, err := a.store.GetRole(ctx, user.RoleID)
    tenant, err := a.store.GetTenant(ctx, user.TenantID)

    return user, role, tenant, nil
}
```

### User Auto-Provisioning

When `auto_provision_users` is enabled, the gateway automatically creates users from token claims:

**Provisioning Logic:**
1. Extract user information from token claims
2. Validate tenant exists and is active
3. Check tenant user quota
4. Map Keycloak groups to internal roles
5. Create user with OAuth subject and role
6. Return authenticated user context

**Example User Creation:**

```go
user := &TenantUser{
    ID:            uuid.New().String(),
    TenantID:      claims.TenantID,
    OAuthSubject:  claims.Subject,      // Keycloak user ID
    OAuthProvider: "keycloak",
    Email:         claims.Email,
    CommonName:    claims.PreferredUsername,
    RoleID:        roleID,              // From group mapping
    IsActive:      true,
    CreatedAt:     time.Now().UTC(),
}
```

### Group-to-Role Mapping

Keycloak groups are mapped to internal roles via configuration:

```yaml
oauth2:
  group_role_mapping:
    "/platform-admins": "platform-admin"
    "/tenant-admins": "tenant-admin"
    "/tenant-editors": "tenant-editor"
    "/tenant-viewers": "tenant-viewer"
```

**Mapping Priority:**
1. Check token `groups` claim
2. Match against `group_role_mapping`
3. Verify role exists in database
4. Fallback to `default_role` if no match
5. Error if no valid role found

### OAuth2 Configuration

```yaml
# config.yaml
oauth2:
  # Enable/disable OAuth2 authentication
  enabled: true

  # OAuth2 takes priority over mTLS when both present
  priority: true

  # Keycloak connection details
  keycloak_base_url: "https://keycloak.example.com"
  keycloak_realm: "netweave"
  keycloak_client_id: "o2ims-gateway"
  keycloak_secret: "${KEYCLOAK_CLIENT_SECRET}"

  # User provisioning
  auto_provision_users: true
  default_role: "tenant-viewer"
  require_tenant_claim: true

  # Group-to-role mappings
  group_role_mapping:
    "/platform-admins": "platform-admin"
    "/tenant-admins": "tenant-admin"
    "/tenant-editors": "tenant-editor"
    "/tenant-viewers": "tenant-viewer"
```

**Configuration Options:**

| Option | Type | Description | Default |
|--------|------|-------------|---------|
| `enabled` | bool | Enable OAuth2 authentication | `false` |
| `priority` | bool | OAuth2 takes priority over mTLS | `false` |
| `keycloak_base_url` | string | Keycloak server URL | Required |
| `keycloak_realm` | string | Keycloak realm name | Required |
| `keycloak_client_id` | string | OAuth2 client ID | Required |
| `keycloak_secret` | string | OAuth2 client secret | Required |
| `auto_provision_users` | bool | Auto-create users from tokens | `false` |
| `default_role` | string | Default role for new users | Required if auto-provisioning |
| `require_tenant_claim` | bool | Require `tenant_id` claim | `false` |
| `group_role_mapping` | map | Keycloak group to role mappings | `{}` |

---

## Dual Authentication

### Priority Configuration

When both authentication methods are present (Bearer token + client certificate), the gateway uses **priority configuration** to determine which to use:

**OAuth2 Priority (oauth2.priority: true):**
```
Request with Bearer token + Certificate → Uses OAuth2
Request with Bearer token only → Uses OAuth2
Request with Certificate only → Uses mTLS
Request with neither → Rejects (401 Unauthorized)
```

**mTLS Priority (oauth2.priority: false):**
```
Request with Bearer token + Certificate → Uses mTLS
Request with Bearer token only → Uses OAuth2
Request with Certificate only → Uses mTLS
Request with neither → Rejects (401 Unauthorized)
```

### Authentication Detection

```go
// internal/auth/middleware.go
func (m *Middleware) detectAuthMethod(c *gin.Context) AuthMethod {
    hasBearerToken := strings.HasPrefix(c.GetHeader("Authorization"), "Bearer ")
    hasCertificate := m.extractCertificate(c) != nil

    // OAuth2 enabled and has Bearer token
    if m.oauth2Config != nil && m.oauth2Config.Enabled && hasBearerToken {
        // Check priority
        if m.oauth2Config.Priority || !hasCertificate {
            return AuthMethodOAuth2
        }
    }

    // mTLS if certificate present
    if hasCertificate {
        return AuthMethodMTLS
    }

    // OAuth2 as fallback
    if m.oauth2Config != nil && m.oauth2Config.Enabled && hasBearerToken {
        return AuthMethodOAuth2
    }

    return ""  // No valid auth method
}
```

### Unified Authentication Context

Both authentication methods produce the same `AuthenticatedUser` context:

```go
type AuthenticatedUser struct {
    UserID          string      // User's unique ID
    TenantID        string      // Tenant ID
    Subject         string      // Certificate DN or OAuth subject
    CommonName      string      // Display name
    Role            *Role       // User's role
    IsPlatformAdmin bool        // Platform admin flag
    AuthMethod      AuthMethod  // "mtls" or "oauth2"
}
```

### Backward Compatibility

**100% backward compatible** with existing mTLS clients:
- ✅ Existing mTLS clients work unchanged
- ✅ OAuth2 can be disabled entirely (`oauth2.enabled: false`)
- ✅ mTLS-only mode available (`oauth2.enabled: false`)
- ✅ No breaking changes to API or behavior

**Migration Path:**
1. **Phase 1**: Deploy with `oauth2.enabled: false` (mTLS only)
2. **Phase 2**: Enable OAuth2 with `priority: false` (mTLS priority)
3. **Phase 3**: Switch to `priority: true` (OAuth2 priority)
4. **Phase 4**: OAuth2 primary, mTLS optional

---

## Tenant Identification

### Three Methods

The gateway supports three methods for tenant identification (in priority order):

#### 1. User Store Lookup (Recommended)

The tenant is determined by looking up the authenticated user in the auth store (Redis or Keycloak):

```go
// internal/auth/middleware.go
// 1. Build subject from certificate
subject := m.BuildSubject(certInfo)

// 2. Look up user by subject DN
user, err := m.store.GetUserBySubject(ctx, subject)

// 3. Tenant comes from the user record, not cert parsing
tenantID := user.TenantID
```

**Pros:**
- ✅ Most secure (user-tenant mapping managed in auth store)
- ✅ Tenant can be changed without certificate reissue
- ✅ Supports both mTLS and OAuth2 authentication

**Cons:**
- ❌ Requires user to be pre-provisioned in auth store

#### 2. Custom HTTP Header

Client sends tenant ID in header:

```bash
curl -X GET https://gateway.example.com/o2ims-infrastructureInventory/v1/resourcePools \
    --cert client.crt --key client.key --cacert ca.crt \
    -H "X-Tenant-ID: smo-alpha"
```

```go
func extractTenantFromHeader(r *http.Request) string {
    return r.Header.Get("X-Tenant-ID")
}
```

**Pros:**
- ✅ Flexible (can change without cert reissue)
- ✅ Simpler client integration

**Cons:**
- ❌ Less secure (header can be modified)
- ❌ Requires additional validation against cert

#### 3. URL Path (API v3+)

Tenant ID in URL path:

```bash
GET /o2ims/v3/tenants/smo-alpha/resourcePools
```

**Pros:**
- ✅ RESTful design
- ✅ Clear tenant scope in URL

**Cons:**
- ❌ Requires API v3+
- ❌ More complex routing

### Tenant Middleware

```go
// internal/middleware/tenant.go
package middleware

import (
    "context"
    "net/http"
    "github.com/gin-gonic/gin"
)

func ExtractTenant() gin.HandlerFunc {
    return func(c *gin.Context) {
        // Priority 1: Client certificate
        if c.Request.TLS != nil && len(c.Request.TLS.PeerCertificates) > 0 {
            cert := c.Request.TLS.PeerCertificates[0]
            if tenantID := extractTenantFromCert(cert); tenantID != "" {
                c.Set("tenantId", tenantID)
                c.Next()
                return
            }
        }

        // Priority 2: HTTP header
        if tenantID := c.GetHeader("X-Tenant-ID"); tenantID != "" {
            c.Set("tenantId", tenantID)
            c.Next()
            return
        }

        // Priority 3: URL parameter (v3 API)
        if tenantID := c.Param("tenantId"); tenantID != "" {
            c.Set("tenantId", tenantID)
            c.Next()
            return
        }

        // No tenant ID found
        c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
            "error": "missing tenant identifier",
        })
    }
}
```

---

## Certificate Requirements

### Client Certificate Specifications

```yaml
# Client Certificate Requirements
version: X.509 v3
validity: 90 days
key_algorithm: RSA
key_size: 4096 bits
signature_algorithm: SHA256WithRSA

# Subject
subject:
  common_name: "user-id.tenant-id.o2ims.example.com"
  organization: "Organization Name"
  organizational_unit: "tenant-id"
  country: "US"
  locality: "City"
  state: "State"

# Subject Alternative Names
subject_alternative_names:
  dns:
    - "user-id.tenant.tenant-id"
  email:
    - "user@example.com"

# Key Usage
key_usage:
  - Digital Signature
  - Key Encipherment

# Extended Key Usage
extended_key_usage:
  - Client Authentication

# Optional: Certificate Policies
certificate_policies:
  - "1.2.3.4.5.6.7.8.1"  # Organization policy OID
```

### Validation Rules

The gateway validates client certificates against these rules:

```go
// internal/auth/validator.go
package auth

import (
    "crypto/x509"
    "fmt"
    "time"
)

type CertificateValidator struct {
    trustedCAs *x509.CertPool
    crl        *x509.RevocationList
}

func (v *CertificateValidator) Validate(cert *x509.Certificate) error {
    // 1. Check expiration
    now := time.Now()
    if now.Before(cert.NotBefore) || now.After(cert.NotAfter) {
        return fmt.Errorf("certificate expired or not yet valid")
    }

    // 2. Verify chain
    opts := x509.VerifyOptions{
        Roots:     v.trustedCAs,
        KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
    }
    if _, err := cert.Verify(opts); err != nil {
        return fmt.Errorf("certificate verification failed: %w", err)
    }

    // 3. Check key usage
    if !hasKeyUsage(cert, x509.KeyUsageDigitalSignature) {
        return fmt.Errorf("certificate missing required key usage")
    }

    // 4. Check extended key usage
    if !hasExtKeyUsage(cert, x509.ExtKeyUsageClientAuth) {
        return fmt.Errorf("certificate missing client authentication extended key usage")
    }

    // 5. Check revocation (if CRL configured)
    if v.crl != nil && isRevoked(cert, v.crl) {
        return fmt.Errorf("certificate has been revoked")
    }

    return nil
}

func hasKeyUsage(cert *x509.Certificate, usage x509.KeyUsage) bool {
    return cert.KeyUsage&usage != 0
}

func hasExtKeyUsage(cert *x509.Certificate, usage x509.ExtKeyUsage) bool {
    for _, u := range cert.ExtKeyUsage {
        if u == usage {
            return true
        }
    }
    return false
}

func isRevoked(cert *x509.Certificate, crl *x509.RevocationList) bool {
    for _, revoked := range crl.RevokedCertificateEntries {
        if cert.SerialNumber.Cmp(revoked.SerialNumber) == 0 {
            return true
        }
    }
    return false
}
```

---

## Authentication Flow

### Dual Authentication Decision Flow

```mermaid
flowchart TB
    Start([HTTPS Request]) --> CheckAuth{Check Auth<br/>Headers}

    CheckAuth -->|Bearer Token| HasToken[Has Bearer Token]
    CheckAuth -->|Certificate| HasCert[Has Certificate]
    CheckAuth -->|Both| HasBoth[Has Both]
    CheckAuth -->|Neither| Reject401[401 Unauthorized]

    HasBoth --> CheckPriority{OAuth2<br/>Priority?}
    CheckPriority -->|Yes| OAuth2Flow
    CheckPriority -->|No| MTLSFlow

    HasToken --> CheckOAuth2{OAuth2<br/>Enabled?}
    CheckOAuth2 -->|Yes| OAuth2Flow[OAuth2 Authentication]
    CheckOAuth2 -->|No| Reject401

    HasCert --> MTLSFlow[mTLS Authentication]

    OAuth2Flow --> VerifyToken[Verify Token<br/>with Keycloak]
    VerifyToken -->|Invalid| Reject401
    VerifyToken -->|Valid| ExtractClaims[Extract Claims]
    ExtractClaims --> GetUser[Get/Create User]
    GetUser --> LoadContext

    MTLSFlow --> VerifyCert[Verify Certificate]
    VerifyCert -->|Invalid| Reject401
    VerifyCert -->|Valid| ExtractDN[Extract DN]
    ExtractDN --> LookupUser[Lookup User]
    LookupUser --> LoadContext[Load Role & Tenant]

    LoadContext --> SetContext[Set Auth Context]
    SetContext --> Authorize{Check<br/>Authorization}
    Authorize -->|Forbidden| Reject403[403 Forbidden]
    Authorize -->|Allowed| Success[200 OK + Response]

    style OAuth2Flow fill:#e1f5ff
    style MTLSFlow fill:#fff4e6
    style Success fill:#e8f5e9
    style Reject401 fill:#ffebee
    style Reject403 fill:#ffebee
```

### mTLS Authentication Sequence

```mermaid
sequenceDiagram
    participant Client as Client (mTLS)
    participant GW as Gateway
    participant TLS as TLS Layer
    participant Auth as Auth Service
    participant Store as User Store
    participant API as API Handler

    Client->>GW: HTTPS Request + Client Cert
    GW->>TLS: TLS Handshake
    TLS->>TLS: Verify Client Cert Chain
    TLS->>TLS: Check Expiration
    TLS->>TLS: Validate Key Usage

    alt Certificate Invalid
        TLS-->>Client: TLS Handshake Failure
    end

    TLS->>Auth: Certificate Verified
    Auth->>Auth: Extract User ID from DN
    Auth->>Auth: Extract Tenant ID from DN
    Auth->>Store: Lookup User by Subject
    Store-->>Auth: User + Role + Tenant

    Auth->>API: Set Auth Context (mTLS)
    API->>API: Handle Request
    API->>API: Check Authorization

    alt Authorized
        API-->>Client: 200 OK + Response
    else Unauthorized
        API-->>Client: 403 Forbidden
    end
```

### OAuth2 Authentication Sequence

```mermaid
sequenceDiagram
    participant Client as Client (OAuth2)
    participant GW as Gateway
    participant Auth as Auth Service
    participant KC as Keycloak
    participant Store as User Store
    participant API as API Handler

    Client->>GW: HTTPS Request + Bearer Token
    GW->>Auth: Extract Bearer Token
    Auth->>KC: Verify Token

    alt Token Invalid
        KC-->>Auth: Invalid Token
        Auth-->>Client: 401 Unauthorized
    end

    KC-->>Auth: Token Valid + Claims
    Auth->>Auth: Extract Claims<br/>(sub, email, groups, tenant_id)

    Auth->>Store: Lookup User by OAuth Subject

    alt User Not Found
        Auth->>Auth: Check Auto-Provision
        alt Auto-Provision Enabled
            Auth->>Store: Validate Tenant & Quota
            Auth->>Auth: Map Groups to Role
            Auth->>Store: Create User
            Store-->>Auth: New User Created
        else Auto-Provision Disabled
            Auth-->>Client: 401 User Not Found
        end
    end

    Store-->>Auth: User + Role + Tenant
    Auth->>API: Set Auth Context (OAuth2)
    API->>API: Handle Request
    API->>API: Check Authorization

    alt Authorized
        API-->>Client: 200 OK + Response
    else Unauthorized
        API-->>Client: 403 Forbidden
    end
```

### Implementation

```go
// Actual implementation: internal/auth/middleware.go
//
// The mTLS authentication flow:
// 1. Extract client certificate from TLS connection
// 2. Build normalized subject DN via BuildSubject()
// 3. Look up user via store.GetUserBySubject(ctx, subject)
// 4. Verify user is active
// 5. Load role and tenant from auth store
// 6. Set authenticated context for downstream handlers
//
// See internal/auth/middleware.go for the full implementation.
```

---

## Client Configuration

### Creating Client Certificates

#### Using Vault PKI

```bash
# Issue a client certificate using Vault PKI
vault write pki_int/issue/netweave-client \
  common_name="operator-1.smo-alpha.o2ims.example.com" \
  ttl="2160h"

# The certificate will have:
# - CN=operator-1.smo-alpha.o2ims.example.com
# - Signed by the Vault intermediate CA
# - Valid for 90 days (2160h)
# - Key usage: Digital Signature, Key Encipherment
# - Extended key usage: Client Authentication
```

#### Using OpenSSL (Development Only)

For development/testing when Vault is not available:

```bash
#!/bin/bash
USER_ID="operator-1"
TENANT_ID="smo-alpha"

# Generate private key
openssl genrsa -out "${USER_ID}.key" 4096

# Generate CSR
openssl req -new -key "${USER_ID}.key" \
    -out "${USER_ID}.csr" \
    -subj "/CN=${USER_ID}.${TENANT_ID}.o2ims.example.com/O=Development/OU=${TENANT_ID}"

# Sign with CA (development only)
openssl x509 -req -days 90 \
    -in "${USER_ID}.csr" \
    -CA ca.crt -CAkey ca.key \
    -CAcreateserial \
    -out "${USER_ID}.crt"
```

**Note:** In production, always use Vault PKI for certificate issuance.

### Obtaining OAuth2 Tokens

#### Interactive Login (Web Browser)

```bash
# 1. Open browser to Keycloak login page
open "https://keycloak.example.com/realms/netweave/protocol/openid-connect/auth?client_id=o2ims-gateway&response_type=token&redirect_uri=http://localhost:8080/callback"

# 2. After login, Keycloak redirects with token in URL fragment:
# http://localhost:8080/callback#access_token=eyJhbGc...&token_type=Bearer&expires_in=300

# 3. Extract access token from URL
TOKEN="eyJhbGc..."
```

#### Direct Token Request (Password Grant)

**⚠️ Not recommended for production - use for testing only**

```bash
#!/bin/bash
# scripts/get-token.sh

KEYCLOAK_URL="https://keycloak.example.com"
REALM="netweave"
CLIENT_ID="o2ims-gateway"
CLIENT_SECRET="your-client-secret"
USERNAME="user@example.com"
PASSWORD="your-password"

# Request token
TOKEN_RESPONSE=$(curl -s -X POST \
    "${KEYCLOAK_URL}/realms/${REALM}/protocol/openid-connect/token" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -d "client_id=${CLIENT_ID}" \
    -d "client_secret=${CLIENT_SECRET}" \
    -d "username=${USERNAME}" \
    -d "password=${PASSWORD}" \
    -d "grant_type=password")

# Extract access token
ACCESS_TOKEN=$(echo "$TOKEN_RESPONSE" | jq -r '.access_token')

echo "Access Token: $ACCESS_TOKEN"
```

#### Client Credentials Grant (Service Accounts)

```bash
#!/bin/bash
# For service-to-service authentication

KEYCLOAK_URL="https://keycloak.example.com"
REALM="netweave"
CLIENT_ID="o2ims-service"
CLIENT_SECRET="service-secret"

# Request token
TOKEN_RESPONSE=$(curl -s -X POST \
    "${KEYCLOAK_URL}/realms/${REALM}/protocol/openid-connect/token" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -d "client_id=${CLIENT_ID}" \
    -d "client_secret=${CLIENT_SECRET}" \
    -d "grant_type=client_credentials")

# Extract access token
ACCESS_TOKEN=$(echo "$TOKEN_RESPONSE" | jq -r '.access_token')

echo "Service Token: $ACCESS_TOKEN"
```

### Client Usage Examples

#### cURL with mTLS

```bash
# Make authenticated request with mTLS
curl -X GET https://o2ims-gateway.example.com/o2ims-infrastructureInventory/v1/resourcePools \
    --cert operator-1.crt \
    --key operator-1.key \
    --cacert ca.crt

# With custom tenant header (fallback)
curl -X GET https://o2ims-gateway.example.com/o2ims-infrastructureInventory/v1/resourcePools \
    --cert operator-1.crt \
    --key operator-1.key \
    --cacert ca.crt \
    -H "X-Tenant-ID: smo-alpha"
```

#### cURL with OAuth2

```bash
# Get token first (using helper script)
TOKEN=$(./scripts/get-token.sh)

# Make authenticated request with Bearer token
curl -X GET https://o2ims-gateway.example.com/o2ims-infrastructureInventory/v1/resourcePools \
    -H "Authorization: Bearer $TOKEN"

# Bearer token takes priority (if oauth2.priority=true)
curl -X GET https://o2ims-gateway.example.com/o2ims-infrastructureInventory/v1/resourcePools \
    -H "Authorization: Bearer $TOKEN" \
    --cert operator-1.crt \
    --key operator-1.key \
    --cacert ca.crt
```

#### Python with mTLS

```python
import requests

# Configure mTLS
response = requests.get(
    'https://o2ims-gateway.example.com/o2ims-infrastructureInventory/v1/resourcePools',
    cert=('operator-1.crt', 'operator-1.key'),
    verify='ca.crt'
)

print(response.json())
```

#### Python with OAuth2

```python
import requests
import json

# Get OAuth2 token
def get_token():
    token_url = "https://keycloak.example.com/realms/netweave/protocol/openid-connect/token"
    data = {
        "client_id": "o2ims-gateway",
        "client_secret": "your-client-secret",
        "username": "user@example.com",
        "password": "your-password",
        "grant_type": "password"
    }

    response = requests.post(token_url, data=data)
    response.raise_for_status()
    return response.json()["access_token"]

# Make authenticated request
token = get_token()
headers = {"Authorization": f"Bearer {token}"}

response = requests.get(
    'https://o2ims-gateway.example.com/o2ims-infrastructureInventory/v1/resourcePools',
    headers=headers
)

print(response.json())
```

#### Go with mTLS

```go
package main

import (
    "crypto/tls"
    "crypto/x509"
    "io"
    "net/http"
    "os"
)

func main() {
    // Load client certificate
    cert, err := tls.LoadX509KeyPair("operator-1.crt", "operator-1.key")
    if err != nil {
        panic(err)
    }

    // Load CA certificate
    caCert, err := os.ReadFile("ca.crt")
    if err != nil {
        panic(err)
    }

    caCertPool := x509.NewCertPool()
    caCertPool.AppendCertsFromPEM(caCert)

    // Configure TLS
    tlsConfig := &tls.Config{
        Certificates: []tls.Certificate{cert},
        RootCAs:      caCertPool,
        MinVersion:   tls.VersionTLS13,
    }

    // Create HTTP client
    client := &http.Client{
        Transport: &http.Transport{
            TLSClientConfig: tlsConfig,
        },
    }

    // Make request
    resp, err := client.Get("https://o2ims-gateway.example.com/o2ims-infrastructureInventory/v1/resourcePools")
    if err != nil {
        panic(err)
    }
    defer resp.Body.Close()

    body, _ := io.ReadAll(resp.Body)
    println(string(body))
}
```

#### Go with OAuth2

```go
package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "net/url"
)

type TokenResponse struct {
    AccessToken string `json:"access_token"`
    TokenType   string `json:"token_type"`
    ExpiresIn   int    `json:"expires_in"`
}

func getToken() (string, error) {
    tokenURL := "https://keycloak.example.com/realms/netweave/protocol/openid-connect/token"

    data := url.Values{}
    data.Set("client_id", "o2ims-gateway")
    data.Set("client_secret", "your-client-secret")
    data.Set("username", "user@example.com")
    data.Set("password", "your-password")
    data.Set("grant_type", "password")

    resp, err := http.Post(tokenURL, "application/x-www-form-urlencoded", bytes.NewBufferString(data.Encode()))
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()

    var tokenResp TokenResponse
    if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
        return "", err
    }

    return tokenResp.AccessToken, nil
}

func main() {
    // Get OAuth2 token
    token, err := getToken()
    if err != nil {
        panic(err)
    }

    // Create authenticated request
    req, err := http.NewRequest("GET", "https://o2ims-gateway.example.com/o2ims-infrastructureInventory/v1/resourcePools", nil)
    if err != nil {
        panic(err)
    }

    req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

    // Make request
    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil {
        panic(err)
    }
    defer resp.Body.Close()

    body, _ := io.ReadAll(resp.Body)
    println(string(body))
}
```

---

## Testing Authentication

### Manual Testing - mTLS

```bash
# 1. Test with valid certificate
curl -v -X GET https://o2ims-gateway.example.com/o2ims-infrastructureInventory/v1/api_versions \
    --cert client.crt --key client.key --cacert ca.crt
# Expected: 200 OK

# 2. Test without certificate
curl -v -X GET https://o2ims-gateway.example.com/o2ims-infrastructureInventory/v1/api_versions \
    --cacert ca.crt
# Expected: TLS handshake failure or 401 Unauthorized

# 3. Test with expired certificate
curl -v -X GET https://o2ims-gateway.example.com/o2ims-infrastructureInventory/v1/api_versions \
    --cert expired-client.crt --key expired-client.key --cacert ca.crt
# Expected: Certificate verification error

# 4. Test with wrong CA
curl -v -X GET https://o2ims-gateway.example.com/o2ims-infrastructureInventory/v1/api_versions \
    --cert wrong-client.crt --key wrong-client.key --cacert ca.crt
# Expected: Certificate verification error
```

### Manual Testing - OAuth2

```bash
# 1. Get valid token
TOKEN=$(./scripts/get-token.sh)

# 2. Test with valid token
curl -v -X GET https://o2ims-gateway.example.com/o2ims-infrastructureInventory/v1/api_versions \
    -H "Authorization: Bearer $TOKEN"
# Expected: 200 OK

# 3. Test without token
curl -v -X GET https://o2ims-gateway.example.com/o2ims-infrastructureInventory/v1/api_versions
# Expected: 401 Unauthorized (if oauth2.enabled=true and no mTLS)

# 4. Test with invalid token
curl -v -X GET https://o2ims-gateway.example.com/o2ims-infrastructureInventory/v1/api_versions \
    -H "Authorization: Bearer invalid.token.here"
# Expected: 401 Unauthorized

# 5. Test with expired token
EXPIRED_TOKEN="eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9..."  # Expired token
curl -v -X GET https://o2ims-gateway.example.com/o2ims-infrastructureInventory/v1/api_versions \
    -H "Authorization: Bearer $EXPIRED_TOKEN"
# Expected: 401 Unauthorized

# 6. Test dual auth priority (OAuth2 priority)
curl -v -X GET https://o2ims-gateway.example.com/o2ims-infrastructureInventory/v1/api_versions \
    -H "Authorization: Bearer $TOKEN" \
    --cert client.crt --key client.key --cacert ca.crt
# Expected: 200 OK (authenticated via OAuth2)

# 7. Test user auto-provisioning
# First request with new user token → user created
curl -v -X GET https://o2ims-gateway.example.com/o2ims-infrastructureInventory/v1/api_versions \
    -H "Authorization: Bearer $NEW_USER_TOKEN"
# Expected: 200 OK + user created in database
```

### Automated Testing

```go
// internal/auth/mtls_test.go
package auth_test

import (
    "crypto/tls"
    "crypto/x509"
    "testing"
    "time"
)

func TestClientCertificateAuthentication(t *testing.T) {
    tests := []struct {
        name        string
        cert        *x509.Certificate
        wantErr     bool
        wantUser    string
        wantTenant  string
    }{
        {
            name:       "valid certificate",
            cert:       loadTestCert("valid-client.crt"),
            wantErr:    false,
            wantUser:   "operator-1",
            wantTenant: "smo-alpha",
        },
        {
            name:    "expired certificate",
            cert:    loadExpiredCert(),
            wantErr: true,
        },
        {
            name:    "invalid CN format",
            cert:    loadCertWithInvalidCN(),
            wantErr: true,
        },
    }

    authenticator := auth.NewMTLSAuthenticator("testdata/ca.crt")

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            ctx, err := authenticator.Authenticate(tt.cert)

            if tt.wantErr {
                if err == nil {
                    t.Errorf("expected error, got nil")
                }
                return
            }

            if err != nil {
                t.Errorf("unexpected error: %v", err)
                return
            }

            if ctx.UserID != tt.wantUser {
                t.Errorf("UserID = %v, want %v", ctx.UserID, tt.wantUser)
            }

            if ctx.TenantID != tt.wantTenant {
                t.Errorf("TenantID = %v, want %v", ctx.TenantID, tt.wantTenant)
            }
        })
    }
}
```

---

## Authentication Backend Storage

### Overview

The gateway supports two backend storage options for authentication data (users, tenants, roles, audit logs):

| Backend | Use Case | Data Storage | Performance | Complexity |
|---------|----------|--------------|-------------|------------|
| **Redis** | Default, simple deployments | Redis database | Very fast | Low |
| **Keycloak** | Enterprise SSO integration | Keycloak Admin API | Fast | Medium |

### Redis Backend (Default)

**Configuration:**
```yaml
auth:
  backend: redis
```

**Features:**
- ✅ Simple, self-contained authentication
- ✅ Very fast (in-memory storage)
- ✅ No external dependencies beyond Redis
- ✅ Suitable for most deployments

**When to Use:**
- Simple deployments without SSO requirements
- Maximum performance with minimal latency
- Development and testing environments
- Self-contained systems without external identity providers

### Keycloak Backend

**Configuration:**
```yaml
auth:
  backend: keycloak
  keycloak:
    base_url: https://keycloak.example.com
    realm: netweave
    client_id: netweave-gateway
    client_secret_env_var: KEYCLOAK_CLIENT_SECRET
    admin_username: admin
    admin_password_env_var: KEYCLOAK_ADMIN_PASSWORD
    timeout: 30s
```

**Features:**
- ✅ Enterprise SSO integration (LDAP, AD, SAML)
- ✅ Centralized user management across applications
- ✅ Advanced authentication flows (MFA, social login)
- ✅ User federation and synchronization
- ✅ Compliance-ready identity management

**When to Use:**
- Enterprise SSO integration required
- Centralized user management needed
- Compliance requirements for identity management
- Multiple applications sharing the same user base
- Advanced authentication flows (MFA, conditional access)

**Keycloak Setup:**

1. **Create Realm:**
   ```bash
   # Create a realm for netweave
   # Realm name: netweave
   ```

2. **Create Client:**
   - Client ID: `netweave-gateway`
   - Client Protocol: `openid-connect`
   - Access Type: `confidential`
   - Service Accounts Enabled: `true`
   - Standard Flow Enabled: `true`

3. **Create Admin User:**
   - Create user with realm management permissions
   - Grant `realm-admin` role for full realm access

4. **Configure Gateway:**
   ```yaml
   auth:
     backend: keycloak
     keycloak:
       base_url: https://keycloak.example.com
       realm: netweave
       client_id: netweave-gateway
       client_secret_env_var: KEYCLOAK_CLIENT_SECRET
       admin_username: admin
       admin_password_env_var: KEYCLOAK_ADMIN_PASSWORD
   ```

5. **Set Environment Variables:**
   ```bash
   export KEYCLOAK_CLIENT_SECRET="your-client-secret"
   export KEYCLOAK_ADMIN_PASSWORD="your-admin-password"
   ```

**Backend Selection Decision Matrix:**

| Requirement | Redis | Keycloak |
|-------------|-------|----------|
| Simple deployment | ✅ Best | ⚠️ Overkill |
| Enterprise SSO | ❌ No | ✅ Best |
| Maximum performance | ✅ Best | ✅ Good |
| User federation (LDAP/AD) | ❌ No | ✅ Yes |
| Multi-application SSO | ❌ No | ✅ Yes |
| MFA support | ❌ No | ✅ Yes |
| Compliance (SOC2, HIPAA) | ⚠️ Manual | ✅ Built-in |
| Operational complexity | ✅ Low | ⚠️ Medium |

**Migration Between Backends:**

To migrate from Redis to Keycloak backend:

1. **Export existing data** from Redis:
   ```bash
   # Export tenants, users, roles, and audit logs
   redis-cli --scan --pattern "tenant:*" | xargs redis-cli DUMP > tenants.json
   redis-cli --scan --pattern "user:*" | xargs redis-cli DUMP > users.json
   ```

2. **Set up Keycloak** realm and client

3. **Import data to Keycloak** using Keycloak Admin API

4. **Update configuration** to use Keycloak backend

5. **Restart gateway** with new configuration

6. **Verify authentication** works with Keycloak backend

**Note:** Automatic migration tools are planned for a future release.

---

**Last Updated:** 2026-02-06
**Version:** 3.0 - Corrected cert subject format, auth flow, and Vault PKI integration
