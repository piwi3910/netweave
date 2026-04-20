package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// MaxDNLength is the maximum allowed length for a DN string.
const MaxDNLength = 2048

// maxDNValueLength is the maximum allowed length for a single DN attribute value.
const maxDNValueLength = 256

// MiddlewareConfig holds configuration for authentication middleware.
type MiddlewareConfig struct {
	// Enabled determines if authentication is enforced.
	Enabled bool

	// SkipPaths is a list of paths that should skip authentication.
	// Prefer MarkPublicRoute at the handler level for any route that is
	// not a truly platform-level endpoint (health, metrics, root); path-
	// based skips silently expand if route prefixes drift.
	SkipPaths []string

	// RequireMTLS requires client certificates for authentication.
	RequireMTLS bool
}

// DefaultMiddlewareConfig returns a MiddlewareConfig with sensible defaults.
// Note: "/o2ims" was historically in SkipPaths but has been removed — the
// handleAPIInfo endpoint that serves "/o2ims" now opts out of auth explicitly
// via Middleware.MarkPublicRoute, so that registering any "/o2ims/<child>"
// route does not accidentally inherit the public behaviour.
func DefaultMiddlewareConfig() *MiddlewareConfig {
	return &MiddlewareConfig{
		Enabled: true,
		SkipPaths: []string{
			"/health",
			"/healthz",
			"/ready",
			"/readyz",
			"/metrics",
			"/",
		},
		RequireMTLS: true,
	}
}

// Middleware provides authentication and authorization middleware for Gin.
type Middleware struct {
	store               Store
	Config              *MiddlewareConfig    // Exported for testing
	Logger              *zap.Logger          // Exported for testing
	compiledPatterns    []*regexp.Regexp     // Pre-compiled regex patterns for skip paths
	oauth2Authenticator *OAuth2Authenticator // OAuth2 authentication handler
	oauth2Config        *OAuth2Config        // OAuth2 configuration

	// publicRoutesMu guards publicRoutes. Routes are typically registered
	// once at startup, but the map supports concurrent reads during request
	// handling without races.
	publicRoutesMu sync.RWMutex

	// publicRoutes is the explicit allowlist of (method, path) tuples that
	// bypass authentication. Populated via MarkPublicRoute — the preferred
	// alternative to string-based SkipPaths for non-platform endpoints.
	publicRoutes map[string]struct{}
}

// NewMiddleware creates a new authentication middleware.
// Pre-compiles regex patterns for skip paths during initialization for performance.
// Supports both mTLS and OAuth2 authentication methods.
func NewMiddleware(
	store Store,
	config *MiddlewareConfig,
	logger *zap.Logger,
	oauth2Authenticator *OAuth2Authenticator,
	oauth2Config *OAuth2Config,
) *Middleware {
	if config == nil {
		config = DefaultMiddlewareConfig()
	}

	// Pre-compile regex patterns for paths with wildcards
	compiledPatterns := make([]*regexp.Regexp, 0, len(config.SkipPaths))
	for _, pattern := range config.SkipPaths {
		if strings.Contains(pattern, "*") {
			// Convert glob pattern to regex using shared helper
			regexPattern := patternToRegex(pattern)
			compiled, err := regexp.Compile(regexPattern)
			if err != nil {
				// Log warning for invalid patterns
				logger.Warn("Failed to compile skip path pattern",
					zap.String("pattern", pattern),
					zap.Error(err))
				continue
			}
			compiledPatterns = append(compiledPatterns, compiled)
		}
	}

	return &Middleware{
		store:               store,
		Config:              config,
		Logger:              logger,
		compiledPatterns:    compiledPatterns,
		oauth2Authenticator: oauth2Authenticator,
		oauth2Config:        oauth2Config,
		publicRoutes:        make(map[string]struct{}),
	}
}

// MarkPublicRoute opts a specific (method, path) tuple out of authentication.
// Prefer this over SkipPaths for any non-platform endpoint — it does not
// expand to sibling paths and is explicit at call sites so reviewers can see
// which routes are intentionally unauthenticated.
//
// Method is normalized to upper-case; path must match exactly what Gin
// produces for c.Request.URL.Path (no trailing slash, no query string).
func (m *Middleware) MarkPublicRoute(method, path string) {
	key := publicRouteKey(method, path)
	m.publicRoutesMu.Lock()
	m.publicRoutes[key] = struct{}{}
	m.publicRoutesMu.Unlock()
}

// isPublicRoute reports whether the (method, path) pair was previously
// marked public via MarkPublicRoute.
func (m *Middleware) isPublicRoute(method, path string) bool {
	key := publicRouteKey(method, path)
	m.publicRoutesMu.RLock()
	_, ok := m.publicRoutes[key]
	m.publicRoutesMu.RUnlock()
	return ok
}

// publicRouteKey builds the map key used for public-route lookups.
func publicRouteKey(method, path string) string {
	return strings.ToUpper(method) + " " + path
}

// AuthenticationMiddleware extracts user identity from the request.
// It parses mTLS client certificates and looks up the user in the database.
//
// SECURITY NOTE: Path Matching and Normalization
// The shouldSkipAuth() function matches paths as-is without normalization.
// Path traversal sequences (../, ./, etc.) are NOT sanitized by this middleware.
// It is the caller's responsibility to ensure paths are normalized BEFORE reaching
// this middleware. Gin framework normalizes paths by default, but custom routers
// or proxies should ensure proper path sanitization.
func (m *Middleware) AuthenticationMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := uuid.New().String()
		c.Set("request_id", requestID)
		ctx := ContextWithRequestID(c.Request.Context(), requestID)
		c.Request = c.Request.WithContext(ctx)

		if m.ShouldSkipAuth(c.Request.URL.Path) || !m.Config.Enabled ||
			m.isPublicRoute(c.Request.Method, c.Request.URL.Path) {
			c.Next()
			return
		}

		authStart := time.Now()

		// Detect authentication method (OAuth2 vs mTLS)
		authMethod := m.detectAuthMethod(c)

		var (
			user       *TenantUser
			role       *Role
			tenant     *Tenant
			err        error
			subject    string
			commonName string
		)

		// Authenticate based on detected method
		switch authMethod {
		case AuthMethodOAuth2:
			// OAuth2 authentication
			user, role, tenant, err = m.oauth2Authenticator.Authenticate(ctx, c, requestID)
			if err != nil {
				m.handleOAuth2AuthenticationFailure(c, requestID, authStart, err)
				return
			}
			if user == nil {
				m.handleOAuth2AuthenticationFailure(c, requestID, authStart, fmt.Errorf("authentication returned nil user"))
				return
			}
			subject = user.OAuthSubject
			commonName = user.CommonName

		case AuthMethodMTLS:
			// mTLS authentication
			cert := m.extractCertificate(c)
			if cert == nil {
				m.handleMissingCertificate(c, requestID, authStart)
				return
			}
			subject = m.BuildSubject(cert)
			commonName = cert.Subject.CommonName

			m.Logger.Debug("authenticating client with mTLS",
				zap.String("subject", SanitizeForLogging(subject, 200)),
				zap.String("common_name", SanitizeForLogging(commonName, 100)),
				zap.String("request_id", requestID),
			)

			user, role, tenant, err = m.authenticateAndLoadContext(c.Request.Context(), subject, requestID)
			if err != nil {
				m.handleAuthenticationError(c, err, subject, requestID, authStart)
				return
			}

		default:
			// No valid authentication method detected.
			// If any auth is configured (OAuth2 or mTLS), require it.
			if (m.oauth2Config != nil && m.oauth2Config.Enabled) || m.Config.RequireMTLS {
				m.handleNoAuthMethod(c, requestID, authStart)
				return
			}
			// No auth configured — allow request through.
			m.Logger.Debug("no authentication method provided and no authentication configured",
				zap.String("request_id", requestID),
			)
			c.Next()
			return
		}

		// Finalize authentication (unified for both methods)
		m.finalizeAuthentication(ctx, c, user, role, tenant, subject, commonName, requestID, authStart, authMethod)
	}
}

// detectAuthMethod determines which authentication method to use based on the request.
func (m *Middleware) detectAuthMethod(c *gin.Context) AuthMethod {
	hasBearerToken := strings.HasPrefix(c.GetHeader("Authorization"), "Bearer ")
	hasCertificate := m.extractCertificate(c) != nil

	// OAuth2 enabled and has Bearer token
	if m.oauth2Config != nil && m.oauth2Config.Enabled && hasBearerToken {
		// Check priority: if OAuth2 has priority or no certificate present
		if m.oauth2Config.Priority || !hasCertificate {
			return AuthMethodOAuth2
		}
	}

	// mTLS if certificate present
	if hasCertificate {
		return AuthMethodMTLS
	}

	// OAuth2 as fallback if enabled and Bearer token present
	if m.oauth2Config != nil && m.oauth2Config.Enabled && hasBearerToken {
		return AuthMethodOAuth2
	}

	return "" // No valid auth method
}

// handleOAuth2AuthenticationFailure handles OAuth2 authentication failures.
func (m *Middleware) handleOAuth2AuthenticationFailure(
	c *gin.Context,
	requestID string,
	authStart time.Time,
	err error,
) {
	m.Logger.Warn("OAuth2 authentication failed",
		zap.String("path", c.Request.URL.Path),
		zap.String("client_ip", c.ClientIP()),
		zap.String("request_id", requestID),
		zap.Error(err),
	)

	RecordAuthenticationAttempt("failed", "oauth2")
	RecordAuthenticationDuration("failed", time.Since(authStart).Seconds())

	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
		"error":   "Unauthorized",
		"message": "Invalid or expired OAuth2 token",
		"code":    http.StatusUnauthorized,
	})
}

// handleNoAuthMethod handles cases where no valid authentication method was detected.
func (m *Middleware) handleNoAuthMethod(c *gin.Context, requestID string, authStart time.Time) {
	m.Logger.Warn("no valid authentication method detected",
		zap.String("path", c.Request.URL.Path),
		zap.String("client_ip", c.ClientIP()),
		zap.String("request_id", requestID),
	)

	RecordAuthenticationAttempt("failed", "none")
	RecordAuthenticationDuration("failed", time.Since(authStart).Seconds())

	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
		"error":   "Unauthorized",
		"message": "Authentication required (mTLS certificate or OAuth2 Bearer token)",
		"code":    http.StatusUnauthorized,
	})
}

func (m *Middleware) handleMissingCertificate(c *gin.Context, requestID string, authStart time.Time) {
	if !m.Config.RequireMTLS {
		c.Next()
		return
	}

	m.Logger.Warn("no client certificate provided",
		zap.String("path", c.Request.URL.Path),
		zap.String("client_ip", c.ClientIP()),
		zap.String("request_id", requestID),
	)
	m.logAuthFailure(c.Request.Context(), c, "", "no client certificate")
	RecordAuthenticationAttempt("failed", "mtls")
	RecordAuthenticationDuration("failed", time.Since(authStart).Seconds())
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
		"error":   "Unauthorized",
		"message": "Client certificate required",
		"code":    http.StatusUnauthorized,
	})
}

func (m *Middleware) authenticateAndLoadContext(
	ctx context.Context,
	subject, _ string,
) (*TenantUser, *Role, *Tenant, error) {
	user, err := m.store.GetUserBySubject(ctx, subject)
	if err != nil {
		return nil, nil, nil, &authError{kind: "user_lookup", err: err, subject: subject}
	}

	if !user.IsActive {
		return nil, nil, nil, &authError{kind: "user_inactive", subject: subject, userID: user.ID}
	}

	role, err := m.store.GetRole(ctx, user.RoleID)
	if err != nil {
		return nil, nil, nil, &authError{kind: "role_lookup", err: err, userID: user.ID, roleID: user.RoleID}
	}

	tenant, err := m.store.GetTenant(ctx, user.TenantID)
	if err != nil {
		return nil, nil, nil, &authError{kind: "tenant_lookup", err: err, userID: user.ID, tenantID: user.TenantID}
	}

	if !tenant.IsActive() {
		return nil, nil, nil, &authError{kind: "tenant_inactive", userID: user.ID, tenantID: user.TenantID}
	}

	return user, role, tenant, nil
}

type authError struct {
	kind     string
	err      error
	subject  string
	userID   string
	roleID   string
	tenantID string
}

func (e *authError) Error() string {
	return e.kind
}

func (m *Middleware) handleAuthenticationError(
	c *gin.Context,
	err error,
	subject, requestID string,
	authStart time.Time,
) {
	var aErr *authError
	if !errors.As(err, &aErr) {
		RecordAuthenticationDuration("error", time.Since(authStart).Seconds())
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error": "InternalError", "message": "Authentication failed", "code": http.StatusInternalServerError,
		})
		return
	}

	switch aErr.kind {
	case "user_lookup":
		if errors.Is(aErr.err, ErrUserNotFound) {
			m.Logger.Warn("unknown user certificate",
				zap.String("subject", SanitizeForLogging(subject, 200)),
				zap.String("client_ip", c.ClientIP()),
				zap.String("request_id", requestID),
			)
			m.logAuthFailure(c.Request.Context(), c, subject, "user not found")
			RecordAuthenticationAttempt("failed", "mtls")
			RecordAuthenticationDuration("failed", time.Since(authStart).Seconds())
			c.AbortWithStatusJSON(
				http.StatusForbidden,
				gin.H{"error": "Forbidden", "message": "Authentication failed", "code": http.StatusForbidden},
			)
		} else {
			m.Logger.Error("failed to lookup user",
				zap.String("subject", SanitizeForLogging(subject, 200)),
				zap.Error(aErr.err),
				zap.String("request_id", requestID),
			)
			RecordAuthenticationDuration("error", time.Since(authStart).Seconds())
			c.AbortWithStatusJSON(
				http.StatusInternalServerError,
				gin.H{"error": "InternalError", "message": "Authentication failed", "code": http.StatusInternalServerError},
			)
		}
	case "user_inactive":
		m.Logger.Warn("inactive user attempted access",
			zap.String("user_id", aErr.userID),
			zap.String("subject", SanitizeForLogging(subject, 200)),
			zap.String("request_id", requestID),
		)
		m.logAuthFailure(c.Request.Context(), c, subject, "user inactive")
		RecordAuthenticationAttempt("failed", "mtls")
		RecordAuthenticationDuration("failed", time.Since(authStart).Seconds())
		c.AbortWithStatusJSON(
			http.StatusForbidden,
			gin.H{"error": "Forbidden", "message": "Authentication failed", "code": http.StatusForbidden},
		)
	case "role_lookup":
		m.Logger.Error("failed to get user role",
			zap.String("user_id", aErr.userID),
			zap.String("role_id", aErr.roleID),
			zap.Error(aErr.err),
			zap.String("request_id", requestID),
		)
		RecordAuthenticationDuration("error", time.Since(authStart).Seconds())
		c.AbortWithStatusJSON(
			http.StatusInternalServerError,
			gin.H{
				"error":   "InternalError",
				"message": "Authentication service temporarily unavailable",
				"code":    http.StatusInternalServerError,
			},
		)
	case "tenant_lookup":
		if errors.Is(aErr.err, ErrTenantNotFound) {
			m.Logger.Warn("user's tenant not found",
				zap.String("user_id", aErr.userID),
				zap.String("tenant_id", aErr.tenantID),
				zap.String("request_id", requestID),
			)
			RecordAuthenticationAttempt("failed", "mtls")
			RecordAuthenticationDuration("failed", time.Since(authStart).Seconds())
			c.AbortWithStatusJSON(
				http.StatusForbidden,
				gin.H{"error": "Forbidden", "message": "Authentication failed", "code": http.StatusForbidden},
			)
		} else {
			m.Logger.Error("failed to get tenant",
				zap.String("tenant_id", aErr.tenantID),
				zap.Error(aErr.err),
				zap.String("request_id", requestID),
			)
			RecordAuthenticationDuration("error", time.Since(authStart).Seconds())
			c.AbortWithStatusJSON(
				http.StatusInternalServerError,
				gin.H{
					"error":   "InternalError",
					"message": "Authentication service temporarily unavailable",
					"code":    http.StatusInternalServerError,
				},
			)
		}
	case "tenant_inactive":
		m.Logger.Warn("access to suspended tenant",
			zap.String("user_id", aErr.userID),
			zap.String("tenant_id", aErr.tenantID),
			zap.String("request_id", requestID),
		)
		m.logAuthFailure(c.Request.Context(), c, subject, "tenant suspended")
		RecordAuthenticationAttempt("failed", "mtls")
		RecordAuthenticationDuration("failed", time.Since(authStart).Seconds())
		c.AbortWithStatusJSON(
			http.StatusForbidden,
			gin.H{"error": "Forbidden", "message": "Tenant is suspended", "code": http.StatusForbidden},
		)
	}
}

func (m *Middleware) finalizeAuthentication(
	ctx context.Context,
	c *gin.Context,
	user *TenantUser,
	role *Role,
	tenant *Tenant,
	subject, commonName, requestID string,
	authStart time.Time,
	authMethod AuthMethod,
) {
	authUser := &AuthenticatedUser{
		UserID:          user.ID,
		TenantID:        user.TenantID,
		Subject:         subject,
		CommonName:      commonName,
		Role:            role,
		IsPlatformAdmin: role.Type == RoleTypePlatform && role.Name == RolePlatformAdmin,
		AuthMethod:      authMethod,
	}

	c.Set("user", authUser)
	c.Set("tenant", tenant)
	c.Set("tenant_id", user.TenantID)
	c.Set("user_id", user.ID)

	ctx = ContextWithUser(ctx, authUser)
	ctx = ContextWithTenant(ctx, tenant)
	c.Request = c.Request.WithContext(ctx)

	// Update last login asynchronously
	// We create a new context derived from the parent to satisfy contextcheck linter,
	// but with a separate timeout to prevent cancellation if the request context is cancelled
	userID := user.ID
	go func(parentCtx context.Context, id string) {
		asyncCtx, cancel := context.WithTimeout(context.WithoutCancel(parentCtx), 5*time.Second)
		defer cancel()
		if err := m.store.UpdateLastLogin(asyncCtx, id); err != nil {
			m.Logger.Warn("failed to update last login", zap.String("user_id", id), zap.Error(err))
		}
	}(ctx, userID)

	m.Logger.Info("user authenticated",
		zap.String("user_id", user.ID),
		zap.String("tenant_id", user.TenantID),
		zap.String("role", SanitizeForLogging(string(role.Name), 50)),
		zap.String("request_id", requestID),
	)
	RecordAuthenticationAttempt("success", string(authMethod))
	RecordAuthenticationDuration("success", time.Since(authStart).Seconds())
	c.Next()
}

// RequirePermission returns a middleware that checks if the user has the required permission.
func (m *Middleware) RequirePermission(permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		requestID := c.GetString("request_id")

		// Get authenticated user from context.
		user := UserFromContext(ctx)
		if user == nil {
			m.Logger.Warn("no authenticated user in context",
				zap.String("path", c.Request.URL.Path),
				zap.String("request_id", requestID),
			)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "Unauthorized",
				"message": "Authentication required",
				"code":    http.StatusUnauthorized,
			})
			return
		}

		// Check permission.
		if !user.HasPermission(Permission(permission)) {
			m.Logger.Warn("permission denied",
				zap.String("user_id", user.UserID),
				zap.String("tenant_id", user.TenantID),
				zap.String("permission", permission),
				zap.String("path", c.Request.URL.Path),
				zap.String("request_id", requestID),
			)

			m.logAccessDenied(ctx, c, user, Permission(permission))
			RecordAuthorizationCheck("denied", Permission(permission))
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":   "Forbidden",
				"message": "Insufficient permissions for this operation",
				"code":    http.StatusForbidden,
			})
			return
		}

		RecordAuthorizationCheck("allowed", Permission(permission))

		c.Next()
	}
}

// RequireAnyPermission returns a middleware that checks if the user has any of the required permissions.
func (m *Middleware) RequireAnyPermission(permissions ...Permission) gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetString("request_id")

		user := UserFromContext(c.Request.Context())
		if user == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "Unauthorized",
				"message": "Authentication required",
				"code":    http.StatusUnauthorized,
			})
			return
		}

		for _, perm := range permissions {
			if user.HasPermission(perm) {
				c.Next()
				return
			}
		}

		m.Logger.Warn("permission denied (none of required permissions)",
			zap.String("user_id", user.UserID),
			zap.Strings("required", permissionsToStrings(permissions)),
			zap.String("path", c.Request.URL.Path),
			zap.String("request_id", requestID),
		)

		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"error":   "Forbidden",
			"message": "Insufficient permissions",
			"code":    http.StatusForbidden,
		})
	}
}

// RequirePlatformAdmin returns a middleware that ensures the user is a platform admin.
func (m *Middleware) RequirePlatformAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetString("request_id")

		user := UserFromContext(c.Request.Context())
		if user == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "Unauthorized",
				"message": "Authentication required",
				"code":    http.StatusUnauthorized,
			})
			return
		}

		if !user.IsPlatformAdmin {
			m.Logger.Warn("platform admin access denied",
				zap.String("user_id", user.UserID),
				zap.String("tenant_id", user.TenantID),
				zap.String("path", c.Request.URL.Path),
				zap.String("request_id", requestID),
			)

			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":   "Forbidden",
				"message": "Platform administrator access required",
				"code":    http.StatusForbidden,
			})
			return
		}

		c.Next()
	}
}

// RequireTenantAccess returns a middleware that ensures the user has access to the specified tenant.
// This is useful for multi-tenant endpoints where a tenant ID is in the path.
func (m *Middleware) RequireTenantAccess(tenantIDParam string) gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetString("request_id")
		targetTenantID := c.Param(tenantIDParam)

		user := UserFromContext(c.Request.Context())
		if user == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":   "Unauthorized",
				"message": "Authentication required",
				"code":    http.StatusUnauthorized,
			})
			return
		}

		// Platform admins can access any tenant.
		if user.IsPlatformAdmin {
			c.Next()
			return
		}

		// Regular users can only access their own tenant.
		if user.TenantID != targetTenantID {
			m.Logger.Warn("cross-tenant access denied",
				zap.String("user_id", user.UserID),
				zap.String("user_tenant", user.TenantID),
				zap.String("target_tenant", targetTenantID),
				zap.String("path", c.Request.URL.Path),
				zap.String("request_id", requestID),
			)

			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":   "Forbidden",
				"message": "Access to other tenants is not allowed",
				"code":    http.StatusForbidden,
			})
			return
		}

		c.Next()
	}
}

// CertificateInfo represents parsed certificate information.
type CertificateInfo struct {
	Subject    CertificateSubject
	CommonName string
	Email      string
}

// CertificateSubject contains parsed subject fields.
type CertificateSubject struct {
	CommonName         string
	Organization       []string
	OrganizationalUnit []string
	Country            []string
	Province           []string
	Locality           []string
}

// extractCertificate extracts the client certificate from the request.
func (m *Middleware) extractCertificate(c *gin.Context) *CertificateInfo {
	// Try to get from TLS connection (native Go TLS).
	if c.Request.TLS != nil && len(c.Request.TLS.PeerCertificates) > 0 {
		cert := c.Request.TLS.PeerCertificates[0]
		return &CertificateInfo{
			Subject: CertificateSubject{
				CommonName:         cert.Subject.CommonName,
				Organization:       cert.Subject.Organization,
				OrganizationalUnit: cert.Subject.OrganizationalUnit,
				Country:            cert.Subject.Country,
				Province:           cert.Subject.Province,
				Locality:           cert.Subject.Locality,
			},
			CommonName: cert.Subject.CommonName,
			Email:      m.ExtractEmail(cert.EmailAddresses),
		}
	}

	// Try to get from X-Forwarded-Client-Cert header (Envoy/Istio).
	xfcc := c.GetHeader("X-Forwarded-Client-Cert")
	if xfcc != "" {
		return m.ParseXFCCHeader(xfcc)
	}

	// Try to get from X-SSL-Client-DN header (Nginx).
	clientDN := c.GetHeader("X-SSL-Client-DN")
	if clientDN != "" {
		return m.ParseDNHeader(clientDN)
	}

	return nil
}

// BuildSubject constructs a normalized subject string from certificate info.
func (m *Middleware) BuildSubject(cert *CertificateInfo) string {
	parts := make([]string, 0)

	if cert.Subject.CommonName != "" {
		parts = append(parts, "CN="+cert.Subject.CommonName)
	}
	if len(cert.Subject.Organization) > 0 {
		parts = append(parts, "O="+cert.Subject.Organization[0])
	}
	if len(cert.Subject.OrganizationalUnit) > 0 {
		parts = append(parts, "OU="+cert.Subject.OrganizationalUnit[0])
	}

	return strings.Join(parts, ",")
}

// ParseXFCCHeader parses the X-Forwarded-Client-Cert header.
func (m *Middleware) ParseXFCCHeader(xfcc string) *CertificateInfo {
	// XFCC format: By=spiffe://..;Hash=...;Subject="CN=...,O=...";URI=spiffe://...

	// Extract Subject field.
	subjectStart := strings.Index(xfcc, "Subject=\"")
	if subjectStart == -1 {
		return nil
	}
	subjectStart += 9
	subjectEnd := strings.Index(xfcc[subjectStart:], "\"")
	if subjectEnd == -1 {
		return nil
	}
	subject := xfcc[subjectStart : subjectStart+subjectEnd]

	// Parse the subject DN.
	return m.ParseDNHeader(subject)
}

// ParseDNHeader parses a DN (Distinguished Name) string.
func (m *Middleware) ParseDNHeader(dn string) *CertificateInfo {
	if !m.validateDN(dn) {
		return nil
	}

	cert := &CertificateInfo{
		Subject: CertificateSubject{},
	}

	parseDNParts(dn, cert)

	if cert.Subject.CommonName == "" {
		return nil
	}

	return cert
}

// validateDN validates the DN string format.
func (m *Middleware) validateDN(dn string) bool {
	// Validate DN length.
	if len(dn) == 0 || len(dn) > MaxDNLength {
		m.Logger.Warn("invalid DN length", zap.Int("length", len(dn)))
		return false
	}

	// Check for null bytes or other control characters.
	if !IsValidDNString(dn) {
		m.Logger.Warn("invalid characters in DN string")
		return false
	}

	return true
}

// parseDNParts parses the DN parts and populates the certificate info.
func parseDNParts(dn string, cert *CertificateInfo) {
	// Split by comma, but handle escaped commas.
	parts := strings.Split(dn, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}

		key := strings.TrimSpace(kv[0])
		value := strings.TrimSpace(kv[1])

		// Validate key format (should be short attribute name).
		if len(key) == 0 || len(key) > 20 || !IsValidDNKey(key) {
			continue
		}

		// Validate and sanitize value.
		value = SanitizeDNValue(value)
		if len(value) == 0 || len(value) > maxDNValueLength {
			continue
		}

		assignDNValueToField(key, value, cert)
	}
}

// assignDNValueToField assigns a DN key-value pair to the appropriate certificate field.
func assignDNValueToField(key, value string, cert *CertificateInfo) {
	switch strings.ToUpper(key) {
	case "CN":
		cert.Subject.CommonName = value
		cert.CommonName = value
	case "O":
		cert.Subject.Organization = append(cert.Subject.Organization, value)
	case "OU":
		cert.Subject.OrganizationalUnit = append(cert.Subject.OrganizationalUnit, value)
	case "C":
		cert.Subject.Country = append(cert.Subject.Country, value)
	case "ST":
		cert.Subject.Province = append(cert.Subject.Province, value)
	case "L":
		cert.Subject.Locality = append(cert.Subject.Locality, value)
	case "EMAILADDRESS", "EMAIL":
		cert.Email = value
	}
}

// IsValidDNString checks if the DN string contains only valid characters.
func IsValidDNString(s string) bool {
	for _, r := range s {
		// Reject control characters except for common whitespace.
		if unicode.IsControl(r) && r != '\t' && r != ' ' {
			return false
		}
	}
	return true
}

// IsValidDNKey checks if the DN attribute key is valid.
func IsValidDNKey(key string) bool {
	for _, r := range key {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// SanitizeDNValue sanitizes a DN attribute value.
func SanitizeDNValue(value string) string {
	// Remove any control characters.
	var result strings.Builder
	for _, r := range value {
		if !unicode.IsControl(r) || r == ' ' {
			result.WriteRune(r)
		}
	}
	return strings.TrimSpace(result.String())
}

// SanitizeForLogging sanitizes a string for safe logging by removing control characters
// and enforcing a maximum length. This prevents log injection attacks and keeps logs readable.
// Parameters:
//   - s: the string to sanitize
//   - maxLen: maximum length (truncated with "..." if exceeded)
//
// Returns sanitized string safe for logging.
func SanitizeForLogging(s string, maxLen int) string {
	// Remove control characters except space and tab
	clean := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != ' ' && r != '\t' {
			return -1 // Remove character
		}
		return r
	}, s)

	// Truncate if too long
	if len(clean) > maxLen {
		return clean[:maxLen] + "..."
	}
	return clean
}

// ExtractEmail returns the first email from the list.
func (m *Middleware) ExtractEmail(emails []string) string {
	if len(emails) > 0 {
		return emails[0]
	}
	return ""
}

// ShouldSkipAuth checks if the path should skip authentication.
// Uses pre-compiled regex patterns for performance.
func (m *Middleware) ShouldSkipAuth(path string) bool {
	// First check exact matches (no wildcard patterns)
	for _, skipPath := range m.Config.SkipPaths {
		if !strings.Contains(skipPath, "*") && path == skipPath {
			return true
		}
	}

	// Then check cached compiled patterns (wildcard patterns)
	for _, pattern := range m.compiledPatterns {
		if pattern.MatchString(path) {
			return true
		}
	}

	return false
}

// patternToRegex converts a glob-style pattern to a regex string.
// Supports both simple trailing wildcards (/api/public/*) and
// glob-style wildcards (/api/*/public/*).
// The last * in a pattern can match multiple path segments.
func patternToRegex(pattern string) string {
	// Escape regex special characters except *
	regexPattern := regexp.QuoteMeta(pattern)

	// Split on escaped \* to handle each wildcard separately
	parts := strings.Split(regexPattern, "\\*")

	// Replace each wildcard:
	// - Non-trailing wildcards match single path segment: [^/]+
	// - Trailing wildcard matches everything: .*
	for i := 0; i < len(parts)-1; i++ {
		if i == len(parts)-2 && parts[i+1] == "" {
			// This is the last wildcard and it's at the end (trailing *)
			parts[i] += ".*"
		} else {
			// Non-trailing wildcard - matches single path segment
			parts[i] += "[^/]+"
		}
	}
	regexPattern = strings.Join(parts, "")

	// Anchor the pattern
	return "^" + regexPattern + "$"
}

// MatchesPathPattern checks if a path matches a pattern with wildcards.
// Used by tests to verify pattern matching logic.
// Production code uses pre-compiled patterns in ShouldSkipAuth for performance.
func MatchesPathPattern(path, pattern string) bool {
	// Exact match
	if path == pattern {
		return true
	}

	// No wildcards - no match
	if !strings.Contains(pattern, "*") {
		return false
	}

	// Convert pattern to regex and match
	regexPattern := patternToRegex(pattern)
	matched, err := regexp.MatchString(regexPattern, path)
	if err != nil {
		return false
	}
	return matched
}

// logAuthFailure logs an authentication failure audit event.
func (m *Middleware) logAuthFailure(ctx context.Context, c *gin.Context, subject, reason string) {
	event := &AuditEvent{
		ID:        uuid.New().String(),
		Type:      AuditEventAuthFailure,
		Subject:   subject,
		Action:    "authentication_failed",
		Details:   map[string]string{"reason": reason},
		ClientIP:  c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	}

	if err := m.store.LogEvent(ctx, event); err != nil {
		m.Logger.Warn("failed to log auth failure event", zap.Error(err))
	}
}

// logAccessDenied logs an access denied audit event.
func (m *Middleware) logAccessDenied(ctx context.Context, c *gin.Context, user *AuthenticatedUser, permission Permission) {
	event := &AuditEvent{
		ID:       uuid.New().String(),
		Type:     AuditEventAccessDenied,
		TenantID: user.TenantID,
		UserID:   user.UserID,
		Subject:  user.Subject,
		Action:   "access_denied",
		Details: map[string]string{
			"permission": string(permission),
			"path":       c.Request.URL.Path,
			"method":     c.Request.Method,
		},
		ClientIP:  c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	}

	if err := m.store.LogEvent(ctx, event); err != nil {
		m.Logger.Warn("failed to log access denied event", zap.Error(err))
	}
}

// permissionsToStrings converts a slice of permissions to strings.
func permissionsToStrings(perms []Permission) []string {
	strs := make([]string, len(perms))
	for i, p := range perms {
		strs[i] = string(p)
	}
	return strs
}
