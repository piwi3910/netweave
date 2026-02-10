# Security Audit Report - Netweave O2-IMS Gateway

**Audit Date:** 2026-02-06
**Auditor:** Pascal Watteel
**Scope:** Comprehensive Security Audit
**Classification:** CONFIDENTIAL

---

## Executive Summary

This report presents the findings of a comprehensive security audit of the Netweave O2-IMS Gateway, an ORAN-compliant API gateway written in Go. The audit covered application security, authentication and authorization mechanisms, cryptographic operations, infrastructure configuration, and dependency analysis.

The overall security posture is **Moderate** with identified areas requiring immediate remediation. The codebase demonstrates solid security fundamentals including proper input validation, structured logging with sanitization, tenant isolation enforcement, and defense-in-depth patterns. However, several high-severity issues related to insecure default configurations, non-production-ready storage backends, and missing cryptographic validations need attention before production deployment.

**Summary of Findings:**

| Severity | Count |
|----------|-------|
| Critical | 2 |
| High | 5 |
| Medium | 8 |
| Low | 4 |
| Informational | 3 |
| **Total** | **22** |

---

## Scope

### Components Reviewed

| Component | Path | Description |
|-----------|------|-------------|
| Auth Middleware | `internal/auth/middleware.go` | mTLS and OAuth2 authentication middleware |
| OAuth2 Authentication | `internal/auth/oauth2.go` | Token verification and user provisioning |
| RBAC Models | `internal/auth/models.go` | Roles, permissions, tenant models |
| Redis Auth Store | `internal/auth/redis.go` | Redis-backed authentication storage |
| Audit Logger | `internal/auth/audit_logger.go` | Audit event logging |
| Keycloak Client | `internal/keycloak/client.go` | Keycloak API integration |
| Keycloak Store | `internal/keycloak/store.go` | Keycloak-backed auth store |
| Keycloak Roles | `internal/keycloak/roles.go` | Role management via Keycloak API |
| Vault Client | `internal/vault/client.go` | HashiCorp Vault PKI integration |
| Vault Certificates | `internal/vault/certificates.go` | Certificate issuance and signing |
| Vault Revocation | `internal/vault/revocation.go` | Certificate revocation and CRL |
| *(Removed)* | *(Legacy `internal/certmanager/` removed — see #408)* | Certificate lifecycle automation planned via Vault PKI |
| Security Headers | `internal/middleware/security_headers.go` | HTTP security headers |
| Rate Limiter | `internal/middleware/ratelimit.go` | Redis-based rate limiting |
| Auth Routes | `internal/server/auth_routes.go` | Route setup and authorization |
| Helm Values | `deployments/helm/netweave/values.yaml` | Deployment configuration |
| Gateway Secret | `deployments/helm/netweave/templates/gateway-secret.yaml` | Secret management |
| Network Policy | `deployments/helm/netweave/templates/gateway-networkpolicy.yaml` | Network isolation |
| Vault RBAC | `deployments/kubernetes/vault/rbac.yaml` | Vault Kubernetes RBAC |

### Review Areas

- Authentication bypass vectors
- Token validation completeness
- Tenant isolation enforcement
- Certificate validation and lifecycle
- Input validation and output encoding
- Error handling and information disclosure
- Secret management practices
- RBAC model correctness
- OWASP Top 10 compliance
- Infrastructure security configuration
- Dependency vulnerability analysis

---

## Methodology

The audit employed the following techniques:

1. **Manual Code Review**: Line-by-line analysis of all security-critical code paths
2. **Static Application Security Testing (SAST)**: gosec scanner for Go-specific vulnerability patterns
3. **Software Composition Analysis (SCA)**: govulncheck for known CVEs in dependencies, go.mod analysis
4. **Threat Modeling**: STRIDE-based analysis of the system architecture
5. **Configuration Review**: Helm charts, Kubernetes manifests, and default configurations
6. **Architecture Analysis**: Data flow tracing across authentication, authorization, and certificate management boundaries

---

## Findings

### Critical Severity

#### CRIT-01: ~~In-Memory Certificate Storage Not Production-Ready~~ (RESOLVED)

**Status:** Resolved — the legacy `internal/certmanager/` package has been removed. Certificate operations now use HashiCorp Vault PKI directly (`internal/vault/`). Certificate lifecycle automation (auto-renewal, expiry monitoring, metrics) is tracked in GitHub issue #408.

---

#### CRIT-02: Keycloak User Lookup Lists All Users (O(n) Scan)

**Component:** `internal/keycloak/store.go`
**CWE:** CWE-400 (Uncontrolled Resource Consumption), CWE-799 (Improper Control of Interaction Frequency)
**CVSS 3.1:** 9.0 (Critical)

**Description:**
The Keycloak store methods `GetUserBySubject`, `GetUserByOAuthSubject`, and `GetUserByEmail` all call `ListUsers` which retrieves every user in the Keycloak realm, then iterates client-side to find a match. This is invoked on every authentication request.

The pattern in the store is:
```go
func (s *Store) GetUserByOAuthSubject(ctx context.Context, oauthSubject string) (*auth.TenantUser, error) {
    // Lists ALL users from Keycloak, then filters client-side
    users, err := s.client.ListUsers(ctx)
    if err != nil {
        return nil, fmt.Errorf("failed to list users: %w", err)
    }
    for _, kcUser := range users {
        // ... iterate to find match
    }
}
```

**Impact:**
- Denial of Service: As user count grows, every authentication request triggers a full user enumeration against Keycloak, creating O(n) latency per request
- Network amplification: Each auth request generates potentially megabytes of API traffic to Keycloak
- Keycloak overload: High request rates will overwhelm the Keycloak server with expensive list operations
- With even 10,000 users, authentication latency becomes unacceptable

**Remediation:**
Use Keycloak's attribute-based search queries (`GET /admin/realms/{realm}/users?q=oauth_subject:{value}`) instead of listing all users. Alternatively, maintain a local index (in Redis) mapping OAuth subjects and certificate subjects to Keycloak user IDs.

---

### High Severity

#### HIGH-01: Default Configurations Use Insecure HTTP for Keycloak and Vault

**Component:** `internal/keycloak/client.go`, `internal/vault/client.go`
**CWE:** CWE-319 (Cleartext Transmission of Sensitive Information)
**CVSS 3.1:** 8.1 (High)

**Description:**
All default configurations specify plain HTTP URLs for sensitive services:

```go
// internal/keycloak/client.go - DefaultConfig()
BaseURL: "http://localhost:8090"

// internal/vault/client.go - DefaultConfig()
Address: "http://localhost:8200"

// Note: internal/certmanager/ was removed (legacy). Vault PKI is used directly.
```

The Vault client (`internal/vault/client.go`) has no TLS configuration options at all. Additionally, the Helm chart includes `vault.skipTLSVerify: true`.

**Impact:**
- Admin credentials (username/password) sent in cleartext to Keycloak
- Vault tokens transmitted in cleartext in `X-Vault-Token` headers
- Client secrets, certificate private keys, and PKI operations exposed to network interception
- Man-in-the-middle attacks can compromise all certificate operations
- If developers copy default configs to production, all secrets are exposed

**Remediation:**
1. Change all default URLs to use HTTPS
2. Add TLS configuration struct to the Vault client with mandatory CA certificate validation
3. Set `vault.skipTLSVerify: false` as the Helm default
4. Add configuration validation that warns or fails when HTTP is used in non-development environments
5. Document TLS requirements prominently

---

#### HIGH-02: No Local JWT Signature Validation

**Component:** `internal/keycloak/client.go` (VerifyToken method)
**CWE:** CWE-345 (Insufficient Verification of Data Authenticity)
**CVSS 3.1:** 7.5 (High)

**Description:**
Token verification relies entirely on Keycloak's token introspection endpoint. There is no local JWT signature validation using Keycloak's JWKS (JSON Web Key Set).

```go
func (c *Client) VerifyToken(ctx context.Context, token string) (map[string]interface{}, error) {
    introspectURL := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token/introspect",
        c.config.BaseURL, c.config.Realm)
    // ... sends token to Keycloak for validation
}
```

**Impact:**
- Single point of failure: If Keycloak is unavailable, all authentication fails
- Network dependency: Every API request requires a round-trip to Keycloak, adding latency
- No audience (`aud`) or issuer (`iss`) validation at the application level
- No token expiration check at the application level (relies entirely on Keycloak's response)
- Replay window: Tokens could be used between Keycloak marking them inactive and the next introspection call

**Remediation:**
Implement local JWT signature verification using Keycloak's JWKS endpoint (`/realms/{realm}/protocol/openid-connect/certs`). Cache the JWKS keys with periodic refresh. Validate `iss`, `aud`, `exp`, `nbf`, and `iat` claims locally. Use introspection as a fallback for revocation checks.

---

#### HIGH-03: Static Vault Token Authentication

**Component:** `internal/vault/client.go`
**CWE:** CWE-798 (Use of Hard-Coded Credentials)
**CVSS 3.1:** 7.5 (High)

**Description:**
The Vault client authenticates using a static token passed via configuration:

```go
// internal/vault/client.go
type Config struct {
    Token string // Static Vault authentication token
}

// Token used in every request
req.Header.Set("X-Vault-Token", c.config.Token)
```

Note: The legacy `internal/certmanager/` package has been removed. Vault PKI is now used directly.

**Impact:**
- Static tokens cannot be automatically rotated, increasing the window of exposure if compromised
- A leaked token provides persistent access to all PKI operations (issue, revoke, sign certificates)
- The config comment mentions Kubernetes auth method but it is not implemented

**Remediation:**
1. Implement Vault Kubernetes auth method (`auth/kubernetes/login`) for dynamic token acquisition
2. Add token rotation support with periodic re-authentication
3. Remove static token support in production configurations

---

#### HIGH-04: Network Policies Disabled by Default

**Component:** `deployments/helm/netweave/values.yaml`
**CWE:** CWE-284 (Improper Access Control)
**CVSS 3.1:** 7.5 (High)

**Description:**
The Helm chart defaults to disabled network policies:

```yaml
networkPolicy:
  enabled: false
```

When disabled, all pods can communicate with any other pod in the cluster without restriction.

**Impact:**
- Lateral movement: A compromised pod can access Vault, Keycloak, Redis, and PostgreSQL directly
- No network-level segmentation between gateway, Vault, Keycloak, and data stores
- Violates the principle of least privilege at the network layer
- In a telecom infrastructure environment, this significantly increases blast radius of any compromise

**Remediation:**
Set `networkPolicy.enabled: true` as the default. The existing `gateway-networkpolicy.yaml` template is well-structured with appropriate ingress/egress rules -- it simply needs to be enabled.

---

#### HIGH-05: TLS InsecureSkipVerify in Multiple Adapters

**Component:** `internal/smo/adapters/onap/client_aai.go`, `internal/events/notifier.go`, `internal/adapters/dtias/client.go`
**CWE:** CWE-295 (Improper Certificate Validation)
**CVSS 3.1:** 7.4 (High)

**Description:**
Three separate components use configurable `InsecureSkipVerify` in their TLS configuration. Identified by gosec (G402):

1. `internal/smo/adapters/onap/client_aai.go:303` - ONAP AAI adapter
2. `internal/events/notifier.go:111` - Event notifier
3. `internal/adapters/dtias/client.go:94` - DTIAS adapter

```go
tlsConfig := &tls.Config{
    InsecureSkipVerify: config.TLSInsecureSkipVerify,
    MinVersion:         tls.VersionTLS12,
}
```

**Impact:**
- When enabled, the gateway will accept any TLS certificate, including self-signed or expired certificates
- Man-in-the-middle attacks become trivial against these connections
- In the ONAP adapter, this could allow interception of infrastructure management traffic
- The event notifier sends webhook notifications -- MITM could capture or modify notification payloads

**Remediation:**
1. Remove `InsecureSkipVerify` as a configurable option, or restrict it to development/test profiles only
2. Provide a proper CA certificate configuration option instead
3. Log a prominent warning if `InsecureSkipVerify` is ever enabled
4. Add deployment validation that rejects `InsecureSkipVerify: true` in production environments

---

### Medium Severity

#### MED-01: Rate Limiter Fails Open on Redis Errors

**Component:** `internal/middleware/ratelimit.go:170-177`
**CWE:** CWE-636 (Not Failing Securely)
**CVSS 3.1:** 6.5 (Medium)

**Description:**
The rate limiter allows all requests through when Redis is unavailable:

```go
result, err := rl.Client.Eval(ctx, script, []string{key}, now, requestsPerSecond, burstSize, windowSize).Result()
if err != nil {
    rl.Logger.Error("rate limit check failed",
        zap.String("key", key),
        zap.Error(err),
    )
    // Fail open: allow request if Redis fails
    return true
}
```

**Impact:**
- An attacker who can cause Redis to fail (e.g., network partition, resource exhaustion) can bypass all rate limiting
- During legitimate Redis outages, the gateway becomes vulnerable to brute force and DoS attacks
- This is explicitly documented as "fail open" but has no compensating controls

**Remediation:**
Implement a local in-memory rate limiter as a fallback when Redis is unavailable. Use a more conservative rate limit for the fallback to maintain protection during outages. Add an alert/metric for Redis rate limit failures so operators are notified immediately.

---

#### MED-02: Keycloak Audit Store Is Placeholder Only

**Component:** `internal/keycloak/store.go`
**CWE:** CWE-778 (Insufficient Logging)
**CVSS 3.1:** 6.5 (Medium)

**Description:**
The Keycloak store's audit implementation is a non-functional placeholder. `LogEvent` only writes to the application log and does not persist audit events. `ListEvents` always returns an empty slice.

**Impact:**
- No persistent audit trail when using the Keycloak store backend
- Cannot perform forensic analysis or compliance reporting
- Security incidents may go undetected due to lack of queryable audit records
- Does not meet compliance requirements for audit logging (SOC2, PCI-DSS)

**Remediation:**
Implement persistent audit event storage using Keycloak's event system or a dedicated audit database. At minimum, forward events to an external logging system (e.g., Elasticsearch, CloudWatch) for searchable retention.

---

#### MED-03: Usage Counter Race Condition (TOCTOU)

**Component:** `internal/keycloak/store.go`
**CWE:** CWE-367 (Time-of-Check Time-of-Use Race Condition)
**CVSS 3.1:** 5.9 (Medium)

**Description:**
Tenant usage modification in the Keycloak store follows a non-atomic read-modify-write pattern. The code reads the current usage, modifies it locally, then writes it back. Concurrent requests can overwrite each other's changes.

**Impact:**
- Tenant quotas can be exceeded if multiple requests provision users simultaneously
- Usage counters can drift from actual values, leading to incorrect quota enforcement
- In a multi-pod deployment, the race window is wider as each pod operates independently

**Remediation:**
Implement optimistic locking using Keycloak realm attribute versioning, or use an external atomic counter (e.g., Redis INCR/DECR) for usage tracking.

---

#### MED-04: Role Names Not URL-Encoded in API Paths

**Component:** `internal/keycloak/roles.go:66,103,130`
**CWE:** CWE-116 (Improper Encoding or Escaping of Output)
**CVSS 3.1:** 5.4 (Medium)

**Description:**
Role names are inserted directly into URL paths without URL encoding:

```go
path := fmt.Sprintf("/roles/%s", roleName)
```

This pattern appears in `GetRealmRole`, `UpdateRealmRole`, `DeleteRealmRole`, and client role operations.

**Impact:**
- Role names containing special characters (spaces, slashes, percent signs, unicode) will produce malformed URLs
- Could potentially be exploited for URL path injection if role names come from user input
- May cause unexpected behavior when interacting with the Keycloak admin API

**Remediation:**
Use `net/url.PathEscape()` for all dynamic URL path segments:
```go
path := fmt.Sprintf("/roles/%s", url.PathEscape(roleName))
```

---

#### MED-05: HSTS Disabled by Default

**Component:** `internal/middleware/security_headers.go:51`
**CWE:** CWE-319 (Cleartext Transmission of Sensitive Information)
**CVSS 3.1:** 5.3 (Medium)

**Description:**
The default security headers configuration sets `TLSEnabled: false`, which prevents the HSTS header from being sent:

```go
func DefaultSecurityHeadersConfig() *SecurityHeadersConfig {
    return &SecurityHeadersConfig{
        Enabled:    true,
        TLSEnabled: false, // HSTS will not be sent
        // ...
    }
}
```

**Impact:**
- Browsers will not enforce HTTPS connections, allowing protocol downgrade attacks
- Initial HTTP connections before redirect are vulnerable to interception
- Does not meet security best practices for API services handling sensitive data

**Remediation:**
When TLS is configured for the gateway, ensure `TLSEnabled` is automatically set to `true`. Add validation that warns when TLS termination is configured but HSTS is not enabled. Consider setting HSTS as default when behind a TLS-terminating reverse proxy or load balancer.

---

#### MED-06: Redis Default Configuration Has Empty Password

**Component:** `internal/auth/redis.go`, `deployments/helm/netweave/values.yaml`
**CWE:** CWE-521 (Weak Password Requirements)
**CVSS 3.1:** 5.3 (Medium)

**Description:**
The Redis auth store default configuration specifies no password:

```go
func DefaultRedisConfig() *RedisConfig {
    return &RedisConfig{
        Addrs:    []string{"localhost:6379"},
        Password: "",
        DB:       0,
    }
}
```

The Helm chart also defaults to empty Redis password:
```yaml
redis:
  auth:
    enabled: true
    password: ""
```

**Impact:**
- Redis instances deployed with defaults are accessible without authentication
- Any pod that can reach Redis can read and modify authentication data, audit logs, and rate limit state
- Stored authentication data (user records, tenant data, role assignments) is exposed

**Remediation:**
Generate a strong random password by default in the Helm chart (similar to other secrets). Add configuration validation that fails on empty passwords in non-development environments.

---

#### MED-07: Potential File Inclusion via Variable

**Component:** `cmd/gateway/main.go:535`, `cmd/compliance/main.go:178`
**CWE:** CWE-22 (Improper Limitation of a Pathname to a Restricted Directory)
**CVSS 3.1:** 5.0 (Medium)

**Description:**
Identified by gosec (G304). Two locations use `os.ReadFile` with variable paths:

```go
// cmd/gateway/main.go:535
data, err := os.ReadFile(path)

// cmd/compliance/main.go:178
content, err := os.ReadFile(path)
```

The gateway code has a comment indicating the path comes from a hardcoded list. The compliance tool reads a user-specified path.

**Impact:**
- If path sources are not properly constrained, directory traversal could allow reading arbitrary files
- The gateway case has low practical risk due to the hardcoded path list
- The compliance tool case has higher risk as it processes user-provided file paths

**Remediation:**
Use `os.Root` (available in Go 1.24+) to scope file access under a fixed root directory. Validate that resolved paths remain within expected directories. For the compliance tool, add path canonicalization and boundary checking.

---

#### MED-08: Auto-Provisioning With Attacker-Controlled Tenant ID

**Component:** `internal/auth/oauth2.go:240-277`
**CWE:** CWE-284 (Improper Access Control)
**CVSS 3.1:** 5.0 (Medium)

**Description:**
When `AutoProvisionUsers` is enabled, the OAuth2 authenticator creates users based on the `tenant_id` claim from the token. If an attacker can influence this claim (e.g., through a compromised or misconfigured identity provider), they could provision themselves into any tenant.

```go
func (a *OAuth2Authenticator) provisionUser(ctx context.Context, claims *OAuth2Claims, requestID string) (*TenantUser, error) {
    tenantID := claims.TenantID  // From token claim
    // ... provisions user into this tenant
}
```

While `validateTenantForProvisioning` checks that the tenant exists and is active, it does not verify that the Keycloak user should have access to that specific tenant.

**Impact:**
- Cross-tenant user provisioning if `tenant_id` claim can be manipulated
- Attacker could gain access to any active tenant's resources
- Tenant isolation could be violated during the provisioning flow

**Remediation:**
1. Restrict which tenants accept auto-provisioned users via an allowlist
2. Validate the `tenant_id` claim against a trusted source (e.g., Keycloak realm roles or group membership)
3. Enable `RequireTenantClaim` as default when auto-provisioning is active
4. Add rate limiting specifically for user provisioning operations

---

### Low Severity

#### LOW-01: Error Messages May Leak Keycloak Response Bodies

**Component:** `internal/keycloak/client.go`, `internal/keycloak/roles.go`
**CWE:** CWE-209 (Information Exposure Through an Error Message)
**CVSS 3.1:** 3.7 (Low)

**Description:**
Multiple Keycloak client methods include the full HTTP response body in error messages:

```go
body, _ := io.ReadAll(resp.Body)
return nil, fmt.Errorf("token introspection failed: status=%d, body=%s", resp.StatusCode, string(body))
```

**Impact:**
- Internal Keycloak error details (version information, stack traces, configuration details) may be propagated to callers
- If these errors reach API responses, they disclose internal infrastructure information
- Assists attackers in mapping the backend architecture

**Remediation:**
Log the full response body at DEBUG/WARN level internally. Return generic error messages to callers without the response body content.

---

#### LOW-02: High-Cardinality Prometheus Labels

**Component:** `internal/auth/metrics.go`
**CWE:** CWE-400 (Uncontrolled Resource Consumption)
**CVSS 3.1:** 3.1 (Low)

**Description:**
Authentication metrics use `tenant_id` as a label, which creates a unique time series for every tenant. As tenant count grows, this increases Prometheus memory usage.

**Impact:**
- Prometheus cardinality explosion with large numbers of tenants
- Increased memory consumption on the monitoring stack
- Potential monitoring system degradation

**Remediation:**
Consider using metric histograms aggregated without tenant labels for global metrics, and expose per-tenant metrics only through a dedicated admin endpoint.

---

#### LOW-03: Tenant Data Stored as Realm Attributes (Scalability)

**Component:** `internal/keycloak/store.go`
**CWE:** CWE-400 (Uncontrolled Resource Consumption)
**CVSS 3.1:** 3.1 (Low)

**Description:**
Tenant data is serialized into Keycloak realm attributes with a naming convention:

```go
func tenantAttributeKey(tenantID, suffix string) string {
    return tenantAttrPrefix + tenantID + suffix
}
```

Each tenant generates approximately 10 realm attributes. The realm object is loaded in full for every tenant operation.

**Impact:**
- Realm attribute count grows linearly with tenant count (10 attributes per tenant)
- Keycloak realm operations become slower as the attribute set grows
- This is not the intended use of Keycloak realm attributes and may hit undocumented limits

**Remediation:**
For production scale, migrate tenant storage to a dedicated database (PostgreSQL or Redis) rather than overloading Keycloak realm attributes.

---

#### LOW-04: Password Grant Flow Available

**Component:** `internal/keycloak/client.go:197-233`
**CWE:** CWE-522 (Insufficiently Protected Credentials)
**CVSS 3.1:** 2.7 (Low)

**Description:**
The `ExchangePasswordCredentials` method implements the OAuth2 Resource Owner Password Credentials (ROPC) grant type. The code comment notes this is "not recommended for production."

**Impact:**
- ROPC grant requires the application to handle raw user credentials
- Deprecated by OAuth 2.1 specification
- Credentials could be logged or leaked through error messages
- Encourages direct password handling instead of browser-based flows

**Remediation:**
Remove the ROPC grant implementation or gate it behind a development-only flag. Use Authorization Code flow with PKCE for all production authentication.

---

### Informational

#### INFO-01: govulncheck Unable to Complete Scan

**Component:** Build environment
**Description:**
The govulncheck scan could not complete due to a Go version mismatch. The scanner was built with Go 1.24 but the project requires Go 1.25:

```
package requires newer Go version go1.25.7 (application built with go1.24)
```

**Recommendation:**
Update the local Go toolchain to 1.25.7+ and re-run `govulncheck ./...` to identify known vulnerabilities in dependencies. Include this in the CI pipeline.

---

#### INFO-02: Vault Unseal Keys in Kubernetes Secrets (Development)

**Component:** `deployments/helm/netweave/values.yaml`
**Description:**
The Helm chart stores Vault unseal keys in Kubernetes secrets with a clear warning: "WARNING: This is for development/testing only." This is acceptable for development but must not be used in production.

**Recommendation:**
Document the production requirement to use Vault auto-unseal with KMS (AWS KMS, GCP KMS, or Azure Key Vault) and remove the development-only unseal key storage from production configurations.

---

#### INFO-03: Strong Security Patterns Observed

**Description:**
The codebase demonstrates several commendable security practices that should be maintained:

1. **Log injection prevention**: `SanitizeForLogging` in `internal/auth/middleware.go` removes control characters and limits string length
2. **DN validation**: MaxDNLength (2048) and maxDNValueLength (256) limits prevent buffer-based attacks
3. **SHA256 hashing for Redis keys**: `sanitizeSubjectKey` in `internal/auth/redis.go` prevents NoSQL injection via DN strings
4. **Atomic operations**: Lua scripts in Redis store for user creation and quota management
5. **Private key protection**: `GetCertificate` strips `PrivateKeyPEM` before returning certificate records
6. **Tenant isolation middleware**: `RequireTenantAccess` enforces cross-tenant access controls with PlatformAdmin bypass
7. **Context propagation**: All operations respect context cancellation for timeout control
8. **Audit logging**: Comprehensive audit event types covering authentication, authorization, and resource changes
9. **Security headers**: Full set of defensive headers (CSP, X-Frame-Options, X-Content-Type-Options, Cache-Control)
10. **OpenTelemetry tracing**: Certificate operations are instrumented for observability

---

## Automated Scan Results

### gosec (Static Analysis)

**Scanner:** gosec (dev version)
**Files scanned:** 128
**Lines scanned:** 42,010
**Findings:** 5

| ID | Severity | Confidence | CWE | File | Description |
|----|----------|------------|-----|------|-------------|
| G402 | HIGH | LOW | CWE-295 | `internal/smo/adapters/onap/client_aai.go:303` | TLS InsecureSkipVerify may be true |
| G402 | HIGH | LOW | CWE-295 | `internal/events/notifier.go:111` | TLS InsecureSkipVerify may be true |
| G402 | HIGH | LOW | CWE-295 | `internal/adapters/dtias/client.go:94` | TLS InsecureSkipVerify may be true |
| G304 | MEDIUM | HIGH | CWE-22 | `cmd/gateway/main.go:535` | Potential file inclusion via variable |
| G304 | MEDIUM | HIGH | CWE-22 | `cmd/compliance/main.go:178` | Potential file inclusion via variable |

**Analysis:**
- The G402 findings (HIGH-05) represent real risk if enabled in production environments
- The G304 findings (MED-07) have mitigating factors (hardcoded paths) but should be addressed
- No suppression directives (`//nosec`) are inappropriately used (only 1 `nosec` in 42,010 lines)

### govulncheck (Dependency Vulnerability Scan)

**Scanner:** govulncheck v1.1.4
**Database:** vuln.go.dev (last modified: 2025-12-30)
**Status:** FAILED -- Go version mismatch

The scan could not complete because the local Go toolchain (1.24) does not meet the project requirement (1.25.7). No vulnerability data was produced.

**Action Required:** Re-run with Go 1.25.7+ toolchain.

### go.mod Dependency Analysis

**Go version:** 1.25.7
**Key dependencies reviewed:**

| Dependency | Version | Assessment |
|------------|---------|------------|
| github.com/gin-gonic/gin | v1.11.0 | Current stable release |
| github.com/redis/go-redis/v9 | v9.17.2 | Recent release |
| k8s.io/client-go | v0.35.0 | Current stable release |
| github.com/prometheus/client_golang | v1.23.2 | Current stable release |
| go.uber.org/zap | v1.27.0 | Current stable release |
| helm.sh/helm/v3 | v3.18.6 | Current stable release |
| golang.org/x/crypto | v0.38.0 | Should verify no known CVEs |
| golang.org/x/net | v0.47.0 | Should verify no known CVEs |

**Note:** A successful govulncheck run is needed to confirm no known CVEs affect the dependency tree at the symbol level.

---

## OWASP Top 10 (2021) Compliance Matrix

| # | Category | Status | Details |
|---|----------|--------|---------|
| A01 | Broken Access Control | PARTIAL | Tenant isolation is enforced (RequireTenantAccess). RBAC model is well-designed. However, auto-provisioning with controllable tenant_id (MED-08) and the TOCTOU race in usage counters (MED-03) are gaps. |
| A02 | Cryptographic Failures | PARTIAL | TLS 1.3 enforced for some adapters, TLS 1.2 minimum for others. SHA256 used for key hashing. However, default HTTP configs (HIGH-01), InsecureSkipVerify (HIGH-05), and HSTS disabled (MED-05) are significant gaps. |
| A03 | Injection | PASS | SHA256 hashing of Redis keys prevents NoSQL injection. Input validation on DN strings. URL path injection risk in Keycloak roles API (MED-04) is minor. |
| A04 | Insecure Design | PARTIAL | Architecture is sound with defense-in-depth. In-memory cert storage (CRIT-01) and O(n) user lookups (CRIT-02) are design-level issues requiring architectural changes. |
| A05 | Security Misconfiguration | FAIL | Network policies disabled (HIGH-04), insecure defaults (HIGH-01), empty Redis password (MED-06), HSTS disabled (MED-05). Multiple default configurations prioritize ease of development over security. |
| A06 | Vulnerable and Outdated Components | UNKNOWN | Dependencies appear current but govulncheck could not complete (INFO-01). Full SCA required. |
| A07 | Identification and Authentication Failures | PARTIAL | Dual mTLS/OAuth2 authentication is strong. No local JWT validation (HIGH-02) and static Vault tokens (HIGH-03) are gaps. ROPC grant available (LOW-04). |
| A08 | Software and Data Integrity Failures | PASS | No deserialization vulnerabilities identified. JSON decoding uses Go standard library. Binary marshaling for Redis uses explicit format. |
| A09 | Security Logging and Monitoring Failures | PARTIAL | Redis auth store has comprehensive audit logging with 30-day TTL. Keycloak store audit is placeholder only (MED-02). OpenTelemetry tracing is well-implemented. Rate limiter fails open without alerting (MED-01). |
| A10 | Server-Side Request Forgery | PASS | URLs for Keycloak and Vault are configured at startup, not derived from user input. Webhook callbacks validate URLs. No SSRF vectors identified. |

---

## Recommendations

### Immediate (Sprint 1 -- Critical/High)

1. **Implement persistent certificate storage** (CRIT-01): Replace in-memory map with Redis-backed storage using the existing Redis infrastructure
2. **Optimize Keycloak user lookups** (CRIT-02): Use attribute-based search or maintain a local index
3. **Enforce HTTPS defaults** (HIGH-01): Change all default URLs to HTTPS, add TLS configuration to Vault client
4. **Implement local JWT validation** (HIGH-02): Add JWKS-based local token verification
5. **Enable network policies by default** (HIGH-04): Set `networkPolicy.enabled: true` in Helm defaults
6. **Remediate InsecureSkipVerify** (HIGH-05): Add environment-based gating for TLS verification bypass

### Short-Term (Sprint 2-3 -- Medium)

7. **Add local rate limit fallback** (MED-01): Implement in-memory token bucket when Redis is unavailable
8. **Implement Keycloak audit persistence** (MED-02): Forward audit events to persistent storage
9. **Add atomic usage counters** (MED-03): Use Redis INCR/DECR for quota tracking
10. **URL-encode role names** (MED-04): Use `url.PathEscape` for all dynamic URL path segments
11. **Auto-enable HSTS with TLS** (MED-05): Link HSTS activation to TLS configuration
12. **Generate Redis passwords** (MED-06): Add random password generation in Helm chart
13. **Scope file access** (MED-07): Use `os.Root` for file operations
14. **Restrict auto-provisioning** (MED-08): Add tenant allowlist for auto-provisioned users

### Long-Term (Quarterly -- Low/Informational)

15. **Implement Vault Kubernetes auth** (HIGH-03): Replace static tokens with dynamic authentication
16. **Migrate tenant storage** (LOW-03): Move from realm attributes to dedicated database
17. **Remove ROPC grant** (LOW-04): Enforce Authorization Code flow with PKCE
18. **Reduce metric cardinality** (LOW-02): Restructure Prometheus labels
19. **Sanitize error messages** (LOW-01): Strip response bodies from returned errors
20. **Complete govulncheck** (INFO-01): Update toolchain and integrate into CI

---

## Conclusion

The Netweave O2-IMS Gateway demonstrates a security-conscious codebase with proper input validation, tenant isolation, audit logging, and defense-in-depth patterns. The RBAC model is well-structured with appropriate permission granularity, and the dual mTLS/OAuth2 authentication approach provides flexibility for different deployment scenarios.

The primary areas of concern are:

1. **Default configurations** that prioritize developer convenience over security, creating risk when defaults are inadvertently carried to production
2. **Non-production-ready storage backends** (in-memory certificates, realm-attribute-based tenants) that must be replaced before production deployment
3. **Missing cryptographic validations** (no local JWT verification, static Vault tokens, InsecureSkipVerify) that weaken the authentication chain

None of the findings indicate active vulnerabilities in a properly configured production deployment. However, the gap between default configurations and production-safe configurations is too large and should be reduced by changing defaults to secure values.

With the recommended remediations implemented, the gateway would achieve a strong security posture appropriate for production telecom infrastructure.

---

*This report was generated as part of GitHub Issue #280.*
