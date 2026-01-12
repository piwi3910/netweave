package auth

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// maxDNLength is the maximum allowed length for a DN string.
const maxDNLength = 2048

// maxDNValueLength is the maximum allowed length for a single DN attribute value.
const maxDNValueLength = 256

// MiddlewareConfig holds configuration for authentication middleware.
type MiddlewareConfig struct {
	// Enabled determines if authentication is enforced.
	Enabled bool

	// SkipPaths is a list of paths that should skip authentication.
	SkipPaths []string

	// RequireMTLS requires client certificates for authentication.
	RequireMTLS bool
}

// DefaultMiddlewareConfig returns a MiddlewareConfig with sensible defaults.
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
			"/o2ims",
		},
		RequireMTLS: true,
	}
}

// Middleware provides authentication and authorization middleware for Gin.
type Middleware struct {
	store            Store
	config           *MiddlewareConfig
	logger           *zap.Logger
	compiledPatterns []*regexp.Regexp // Pre-compiled regex patterns for skip paths
}

// NewMiddleware creates a new authentication middleware.
// Pre-compiles regex patterns for skip paths during initialization for performance.
func NewMiddleware(store Store, config *MiddlewareConfig, logger *zap.Logger) *Middleware {
	if config == nil {
		config = DefaultMiddlewareConfig()
	}

	// Pre-compile regex patterns for paths with wildcards
	compiledPatterns := make([]*regexp.Regexp, 0, len(config.SkipPaths))
	for _, pattern := range config.SkipPaths {
		if strings.Contains(pattern, "*") {
			// Convert glob pattern to regex
			regexPattern := regexp.QuoteMeta(pattern)
			parts := strings.Split(regexPattern, "\\*")

			// Replace each wildcard with appropriate regex
			for i := 0; i < len(parts)-1; i++ {
				if i == len(parts)-2 && parts[i+1] == "" {
					// Trailing wildcard matches everything
					parts[i] += ".*"
				} else {
					// Non-trailing wildcard matches single segment
					parts[i] += "[^/]+"
				}
			}

			regexPattern = "^" + strings.Join(parts, "") + "$"
			if compiled, err := regexp.Compile(regexPattern); err == nil {
				compiledPatterns = append(compiledPatterns, compiled)
			}
		}
	}

	return &Middleware{
		store:            store,
		config:           config,
		logger:           logger,
		compiledPatterns: compiledPatterns,
	}
}

// AuthenticationMiddleware extracts user identity from the request.
// It parses mTLS client certificates and looks up the user in the database.
func (m *Middleware) AuthenticationMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Generate request ID.
		requestID := uuid.New().String()
		c.Set("request_id", requestID)
		ctx := ContextWithRequestID(c.Request.Context(), requestID)
		c.Request = c.Request.WithContext(ctx)

		// Skip authentication for excluded paths.
		if m.shouldSkipAuth(c.Request.URL.Path) {
			c.Next()
			return
		}

		// Check if authentication is enabled.
		if !m.config.Enabled {
			c.Next()
			return
		}

		// Start timing authentication.
		authStart := time.Now()

		// Extract client certificate.
		cert := m.extractCertificate(c)
		if cert == nil {
			if m.config.RequireMTLS {
				m.logger.Warn("no client certificate provided",
					zap.String("path", c.Request.URL.Path),
					zap.String("client_ip", c.ClientIP()),
					zap.String("request_id", requestID),
				)

				m.logAuthFailure(c, "", "no client certificate")
				RecordAuthenticationAttempt("failed", "mtls")
				RecordAuthenticationDuration("failed", time.Since(authStart).Seconds())
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"error":   "Unauthorized",
					"message": "Client certificate required",
					"code":    http.StatusUnauthorized,
				})
				return
			}
			c.Next()
			return
		}

		// Build certificate subject.
		subject := m.buildSubject(cert)
		commonName := cert.Subject.CommonName

		m.logger.Debug("authenticating client",
			zap.String("subject", sanitizeForLogging(subject, 200)),
			zap.String("common_name", sanitizeForLogging(commonName, 100)),
			zap.String("request_id", requestID),
		)

		// Look up user by certificate subject.
		user, err := m.store.GetUserBySubject(c.Request.Context(), subject)
		if err != nil {
			if errors.Is(err, ErrUserNotFound) {
				m.logger.Warn("unknown user certificate",
					zap.String("subject", sanitizeForLogging(subject, 200)),
					zap.String("client_ip", c.ClientIP()),
					zap.String("request_id", requestID),
				)

				m.logAuthFailure(c, subject, "user not found")
				RecordAuthenticationAttempt("failed", "mtls")
				RecordAuthenticationDuration("failed", time.Since(authStart).Seconds())
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"error":   "Forbidden",
					"message": "Authentication failed",
					"code":    http.StatusForbidden,
				})
				return
			}

			m.logger.Error("failed to lookup user",
				zap.String("subject", sanitizeForLogging(subject, 200)),
				zap.Error(err),
				zap.String("request_id", requestID),
			)
			RecordAuthenticationDuration("error", time.Since(authStart).Seconds())
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error":   "InternalError",
				"message": "Authentication failed",
				"code":    http.StatusInternalServerError,
			})
			return
		}

		// Check if user is active.
		if !user.IsActive {
			m.logger.Warn("inactive user attempted access",
				zap.String("user_id", user.ID),
				zap.String("subject", sanitizeForLogging(subject, 200)),
				zap.String("request_id", requestID),
			)

			m.logAuthFailure(c, subject, "user inactive")
			RecordAuthenticationAttempt("failed", "mtls")
			RecordAuthenticationDuration("failed", time.Since(authStart).Seconds())
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":   "Forbidden",
				"message": "Authentication failed",
				"code":    http.StatusForbidden,
			})
			return
		}

		// Get user's role.
		role, err := m.store.GetRole(c.Request.Context(), user.RoleID)
		if err != nil {
			m.logger.Error("failed to get user role",
				zap.String("user_id", user.ID),
				zap.String("role_id", user.RoleID),
				zap.Error(err),
				zap.String("request_id", requestID),
			)
			RecordAuthenticationDuration("error", time.Since(authStart).Seconds())
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error":   "InternalError",
				"message": "Authentication service temporarily unavailable",
				"code":    http.StatusInternalServerError,
			})
			return
		}

		// Get tenant information.
		tenant, err := m.store.GetTenant(c.Request.Context(), user.TenantID)
		if err != nil {
			if errors.Is(err, ErrTenantNotFound) {
				m.logger.Warn("user's tenant not found",
					zap.String("user_id", user.ID),
					zap.String("tenant_id", user.TenantID),
					zap.String("request_id", requestID),
				)
				RecordAuthenticationAttempt("failed", "mtls")
				RecordAuthenticationDuration("failed", time.Since(authStart).Seconds())
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"error":   "Forbidden",
					"message": "Authentication failed",
					"code":    http.StatusForbidden,
				})
				return
			}

			m.logger.Error("failed to get tenant",
				zap.String("tenant_id", user.TenantID),
				zap.Error(err),
				zap.String("request_id", requestID),
			)
			RecordAuthenticationDuration("error", time.Since(authStart).Seconds())
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error":   "InternalError",
				"message": "Authentication service temporarily unavailable",
				"code":    http.StatusInternalServerError,
			})
			return
		}

		// Check if tenant is active.
		if !tenant.IsActive() {
			m.logger.Warn("access to suspended tenant",
				zap.String("user_id", user.ID),
				zap.String("tenant_id", user.TenantID),
				zap.String("request_id", requestID),
			)

			m.logAuthFailure(c, subject, "tenant suspended")
			RecordAuthenticationAttempt("failed", "mtls")
			RecordAuthenticationDuration("failed", time.Since(authStart).Seconds())
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":   "Forbidden",
				"message": "Tenant is suspended",
				"code":    http.StatusForbidden,
			})
			return
		}

		// Create authenticated user context.
		authUser := &AuthenticatedUser{
			UserID:          user.ID,
			TenantID:        user.TenantID,
			Subject:         subject,
			CommonName:      commonName,
			Role:            role,
			IsPlatformAdmin: role.Type == RoleTypePlatform && role.Name == RolePlatformAdmin,
		}

		// Store in Gin context and request context.
		c.Set("user", authUser)
		c.Set("tenant", tenant)
		c.Set("tenant_id", user.TenantID)
		c.Set("user_id", user.ID)

		ctx = ContextWithUser(ctx, authUser)
		ctx = ContextWithTenant(ctx, tenant)
		c.Request = c.Request.WithContext(ctx)

		// Update last login timestamp (async, non-blocking).
		// Use a new context with timeout since the request context may be cancelled.
		userID := user.ID
		go func() {
			asyncCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := m.store.UpdateLastLogin(asyncCtx, userID); err != nil {
				m.logger.Warn("failed to update last login",
					zap.String("user_id", userID),
					zap.Error(err),
				)
			}
		}()

		m.logger.Info("user authenticated",
			zap.String("user_id", user.ID),
			zap.String("tenant_id", user.TenantID),
			zap.String("role", sanitizeForLogging(string(role.Name), 50)),
			zap.String("request_id", requestID),
		)

		RecordAuthenticationAttempt("success", "mtls")
		RecordAuthenticationDuration("success", time.Since(authStart).Seconds())
		c.Next()
	}
}

// RequirePermission returns a middleware that checks if the user has the required permission.
func (m *Middleware) RequirePermission(permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetString("request_id")

		// Get authenticated user from context.
		user := UserFromContext(c.Request.Context())
		if user == nil {
			m.logger.Warn("no authenticated user in context",
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
			m.logger.Warn("permission denied",
				zap.String("user_id", user.UserID),
				zap.String("tenant_id", user.TenantID),
				zap.String("permission", permission),
				zap.String("path", c.Request.URL.Path),
				zap.String("request_id", requestID),
			)

			m.logAccessDenied(c, user, Permission(permission))
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

		m.logger.Warn("permission denied (none of required permissions)",
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
			m.logger.Warn("platform admin access denied",
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
			m.logger.Warn("cross-tenant access denied",
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
			Email:      m.extractEmail(cert.EmailAddresses),
		}
	}

	// Try to get from X-Forwarded-Client-Cert header (Envoy/Istio).
	xfcc := c.GetHeader("X-Forwarded-Client-Cert")
	if xfcc != "" {
		return m.parseXFCCHeader(xfcc)
	}

	// Try to get from X-SSL-Client-DN header (Nginx).
	clientDN := c.GetHeader("X-SSL-Client-DN")
	if clientDN != "" {
		return m.parseDNHeader(clientDN)
	}

	return nil
}

// buildSubject constructs a normalized subject string from certificate info.
func (m *Middleware) buildSubject(cert *CertificateInfo) string {
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

// parseXFCCHeader parses the X-Forwarded-Client-Cert header.
func (m *Middleware) parseXFCCHeader(xfcc string) *CertificateInfo {
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
	return m.parseDNHeader(subject)
}

// parseDNHeader parses a DN (Distinguished Name) string.
func (m *Middleware) parseDNHeader(dn string) *CertificateInfo {
	// Validate DN length.
	if len(dn) == 0 || len(dn) > maxDNLength {
		m.logger.Warn("invalid DN length", zap.Int("length", len(dn)))
		return nil
	}

	// Check for null bytes or other control characters.
	if !isValidDNString(dn) {
		m.logger.Warn("invalid characters in DN string")
		return nil
	}

	cert := &CertificateInfo{
		Subject: CertificateSubject{},
	}

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
		if len(key) == 0 || len(key) > 20 || !isValidDNKey(key) {
			continue
		}

		// Validate and sanitize value.
		value = sanitizeDNValue(value)
		if len(value) == 0 || len(value) > maxDNValueLength {
			continue
		}

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

	if cert.Subject.CommonName == "" {
		return nil
	}

	return cert
}

// isValidDNString checks if the DN string contains only valid characters.
func isValidDNString(s string) bool {
	for _, r := range s {
		// Reject control characters except for common whitespace.
		if unicode.IsControl(r) && r != '\t' && r != ' ' {
			return false
		}
	}
	return true
}

// isValidDNKey checks if the DN attribute key is valid.
func isValidDNKey(key string) bool {
	for _, r := range key {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// sanitizeDNValue sanitizes a DN attribute value.
func sanitizeDNValue(value string) string {
	// Remove any control characters.
	var result strings.Builder
	for _, r := range value {
		if !unicode.IsControl(r) || r == ' ' {
			result.WriteRune(r)
		}
	}
	return strings.TrimSpace(result.String())
}

// sanitizeForLogging sanitizes a string for safe logging by removing control characters
// and enforcing a maximum length. This prevents log injection attacks and keeps logs readable.
// Parameters:
//   - s: the string to sanitize
//   - maxLen: maximum length (truncated with "..." if exceeded)
//
// Returns sanitized string safe for logging.
func sanitizeForLogging(s string, maxLen int) string {
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

// extractEmail returns the first email from the list.
func (m *Middleware) extractEmail(emails []string) string {
	if len(emails) > 0 {
		return emails[0]
	}
	return ""
}

// shouldSkipAuth checks if the path should skip authentication.
// Uses pre-compiled regex patterns for performance.
func (m *Middleware) shouldSkipAuth(path string) bool {
	// First check exact matches (no wildcard patterns)
	for _, skipPath := range m.config.SkipPaths {
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

// matchesPathPattern checks if a path matches a pattern with wildcards.
// Supports both simple trailing wildcards (/api/public/*) and
// glob-style wildcards (/api/*/public/*).
// The last * in a pattern can match multiple path segments.
func matchesPathPattern(path, pattern string) bool {
	// Exact match
	if path == pattern {
		return true
	}

	// No wildcards - no match
	if !strings.Contains(pattern, "*") {
		return false
	}

	// Convert pattern to regex
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
	regexPattern = "^" + regexPattern + "$"

	matched, err := regexp.MatchString(regexPattern, path)
	if err != nil {
		return false
	}
	return matched
}

// logAuthFailure logs an authentication failure audit event.
func (m *Middleware) logAuthFailure(c *gin.Context, subject, reason string) {
	event := &AuditEvent{
		ID:        uuid.New().String(),
		Type:      AuditEventAuthFailure,
		Subject:   subject,
		Action:    "authentication_failed",
		Details:   map[string]string{"reason": reason},
		ClientIP:  c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	}

	if err := m.store.LogEvent(c.Request.Context(), event); err != nil {
		m.logger.Warn("failed to log auth failure event", zap.Error(err))
	}
}

// logAccessDenied logs an access denied audit event.
func (m *Middleware) logAccessDenied(c *gin.Context, user *AuthenticatedUser, permission Permission) {
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

	if err := m.store.LogEvent(c.Request.Context(), event); err != nil {
		m.logger.Warn("failed to log access denied event", zap.Error(err))
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
