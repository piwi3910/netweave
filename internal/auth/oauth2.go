package auth

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// TokenVerifier interface for token verification (allows mocking in tests).
type TokenVerifier interface {
	VerifyToken(ctx context.Context, token string) (map[string]interface{}, error)
}

// OAuth2Authenticator handles OAuth2/OIDC authentication using Keycloak.
type OAuth2Authenticator struct {
	keycloakClient TokenVerifier
	store          Store
	config         *OAuth2Config
	logger         *zap.Logger
	denylist       JTIDenylist
}

// OAuth2Config contains configuration for OAuth2 authentication.
type OAuth2Config struct {
	// Enabled indicates whether OAuth2 authentication is enabled.
	Enabled bool

	// Priority indicates whether OAuth2 takes priority over mTLS when both are present.
	Priority bool

	// AutoProvisionUsers enables automatic user creation from OAuth2 claims.
	AutoProvisionUsers bool

	// DefaultRole is the role ID to assign to auto-provisioned users.
	DefaultRole string

	// GroupRoleMapping maps Keycloak groups to role IDs.
	GroupRoleMapping map[string]string

	// RequireTenantClaim requires the tenant_id claim to be present in tokens.
	RequireTenantClaim bool

	// ExpectedAudience is the audience value that incoming tokens must carry
	// in their "aud" claim. When empty, audience enforcement is skipped
	// (not recommended for production).
	ExpectedAudience string

	// ExpectedIssuer is the issuer value that incoming tokens must carry in
	// their "iss" claim. When empty, issuer enforcement is skipped
	// (not recommended for production).
	ExpectedIssuer string

	// AllowedClientIDs is the set of Keycloak client IDs whose tokens are
	// accepted by this gateway. When empty, client_id enforcement is skipped
	// (not recommended for production).
	AllowedClientIDs []string
}

// OAuth2Claims represents structured claims from an OAuth2/OIDC token.
type OAuth2Claims struct {
	// Subject is the "sub" claim (Keycloak user ID).
	Subject string

	// Email is the "email" claim.
	Email string

	// PreferredUsername is the "preferred_username" claim.
	PreferredUsername string

	// Name is the "name" claim (full name).
	Name string

	// Groups is the "groups" claim (Keycloak groups for role mapping).
	Groups []string

	// TenantID is a custom "tenant_id" claim for tenant association.
	TenantID string

	// JTI is the "jti" (JWT ID) claim used for per-token revocation.
	JTI string

	// Audience is the "aud" claim. OAuth2 tokens may carry either a string
	// or an array of strings; we preserve all values here.
	Audience []string

	// Issuer is the "iss" claim used to confirm the token was minted by the
	// expected Keycloak realm.
	Issuer string

	// ClientID is the "client_id" (or "azp") claim identifying the OAuth2
	// client that obtained the token.
	ClientID string

	// ExpiresAt is the "exp" claim in seconds since the Unix epoch. Used to
	// size JTI denylist TTLs so revocations cover the natural token lifetime.
	ExpiresAt int64
}

// NewOAuth2Authenticator creates a new OAuth2 authenticator.
func NewOAuth2Authenticator(
	keycloakClient TokenVerifier,
	store Store,
	config *OAuth2Config,
	logger *zap.Logger,
) *OAuth2Authenticator {
	return &OAuth2Authenticator{
		keycloakClient: keycloakClient,
		store:          store,
		config:         config,
		logger:         logger,
	}
}

// WithDenylist attaches a JTI denylist to the authenticator.
// The denylist is consulted on every authentication and a token whose jti is
// present is rejected with ErrTokenRevoked.
func (a *OAuth2Authenticator) WithDenylist(denylist JTIDenylist) *OAuth2Authenticator {
	a.denylist = denylist
	return a
}

// Authenticate performs OAuth2 authentication for an incoming request.
// Returns the authenticated user, role, and tenant on success.
func (a *OAuth2Authenticator) Authenticate(
	ctx context.Context,
	c *gin.Context,
	requestID string,
) (*TenantUser, *Role, *Tenant, error) {
	claims, err := a.verifyAndExtractClaims(ctx, c, requestID)
	if err != nil {
		return nil, nil, nil, err
	}

	// Defence-in-depth: verify audience, issuer, and client_id in addition to
	// the introspection "active" check. Prevents cross-client/realm token
	// replay when the Keycloak instance hosts multiple trust domains.
	if err := a.verifyTokenBinding(claims); err != nil {
		return nil, nil, nil, err
	}

	// Check the Redis-backed denylist so revocations propagate within the
	// remaining lifetime of a token, even before its natural expiry.
	if err := a.checkRevocation(ctx, claims); err != nil {
		return nil, nil, nil, err
	}

	// Get or create user from claims
	user, err := a.getOrCreateUser(ctx, claims, requestID)
	if err != nil {
		return nil, nil, nil, err
	}

	// Reject disabled users even if their Keycloak session is still active.
	// This mirrors the mTLS branch in Middleware.authenticateAndLoadContext.
	if !user.IsActive {
		a.logger.Warn("inactive user attempted OAuth2 access",
			zap.String("requestID", requestID),
			zap.String("userID", user.ID),
		)
		return nil, nil, nil, ErrAccountDisabled
	}

	// Load user's role
	role, err := a.store.GetRole(ctx, user.RoleID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to load role: %w", err)
	}

	// Load user's tenant
	tenant, err := a.store.GetTenant(ctx, user.TenantID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to load tenant: %w", err)
	}

	// Tenant must also be active; otherwise deny access on parity with mTLS.
	if !tenant.IsActive() {
		a.logger.Warn("OAuth2 access denied for suspended tenant",
			zap.String("requestID", requestID),
			zap.String("userID", user.ID),
			zap.String("tenantID", tenant.ID),
		)
		return nil, nil, nil, ErrTenantSuspended
	}

	// Update last login timestamp
	_ = a.store.UpdateLastLogin(ctx, user.ID)

	return user, role, tenant, nil
}

// verifyAndExtractClaims performs token extraction, introspection, and
// structured claim parsing. Extracted to keep Authenticate concise.
func (a *OAuth2Authenticator) verifyAndExtractClaims(
	ctx context.Context,
	c *gin.Context,
	requestID string,
) (*OAuth2Claims, error) {
	token, err := a.extractBearerToken(c)
	if err != nil {
		return nil, fmt.Errorf("failed to extract bearer token: %w", err)
	}

	tokenClaims, err := a.keycloakClient.VerifyToken(ctx, token)
	if err != nil {
		a.logger.Warn("OAuth2 token verification failed",
			zap.String("requestID", requestID),
			zap.Error(err),
		)
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	claims, err := a.extractClaims(tokenClaims)
	if err != nil {
		return nil, fmt.Errorf("failed to extract claims: %w", err)
	}

	if err := a.validateClaims(claims); err != nil {
		return nil, err
	}

	return claims, nil
}

// verifyTokenBinding validates aud, iss, and client_id against configured
// allowlists. Checks are skipped only when the corresponding config field is
// empty (opt-in enforcement to remain backwards-compatible for tests).
func (a *OAuth2Authenticator) verifyTokenBinding(claims *OAuth2Claims) error {
	if a.config.ExpectedAudience != "" {
		if !containsString(claims.Audience, a.config.ExpectedAudience) {
			return fmt.Errorf("token audience mismatch: expected %q", a.config.ExpectedAudience)
		}
	}

	if a.config.ExpectedIssuer != "" && claims.Issuer != a.config.ExpectedIssuer {
		return fmt.Errorf("token issuer mismatch: expected %q, got %q",
			a.config.ExpectedIssuer, claims.Issuer)
	}

	if len(a.config.AllowedClientIDs) > 0 {
		if !containsString(a.config.AllowedClientIDs, claims.ClientID) {
			return fmt.Errorf("token client_id %q is not on the allowlist", claims.ClientID)
		}
	}

	return nil
}

// checkRevocation consults the optional JTI denylist. Empty jti claims are
// tolerated unless enforcement is desired at the config layer.
func (a *OAuth2Authenticator) checkRevocation(ctx context.Context, claims *OAuth2Claims) error {
	if a.denylist == nil || claims.JTI == "" {
		return nil
	}
	revoked, err := a.denylist.IsRevoked(ctx, claims.JTI)
	if err != nil {
		// Log and fail closed. Revocation is a security-critical check; if
		// the backing store is unavailable we'd rather reject the request
		// than risk accepting a revoked token.
		a.logger.Error("JTI denylist lookup failed",
			zap.String("jti", claims.JTI),
			zap.Error(err),
		)
		return fmt.Errorf("token revocation check failed: %w", err)
	}
	if revoked {
		return ErrTokenRevoked
	}
	return nil
}

// containsString reports whether needle is present in haystack.
func containsString(haystack []string, needle string) bool {
	for _, v := range haystack {
		if v == needle {
			return true
		}
	}
	return false
}

// extractBearerToken extracts the Bearer token from the Authorization header.
func (a *OAuth2Authenticator) extractBearerToken(c *gin.Context) (string, error) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		return "", fmt.Errorf("missing Authorization header")
	}

	// Parse "Bearer <token>"
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return "", fmt.Errorf("invalid Authorization header format")
	}

	return parts[1], nil
}

// extractClaims extracts structured claims from the raw token claims map.
func (a *OAuth2Authenticator) extractClaims(tokenClaims map[string]interface{}) (*OAuth2Claims, error) {
	claims := &OAuth2Claims{}

	// Required: sub claim
	if sub, ok := tokenClaims["sub"].(string); ok {
		claims.Subject = sub
	} else {
		return nil, fmt.Errorf("missing required 'sub' claim")
	}

	// Optional: email
	if email, ok := tokenClaims["email"].(string); ok {
		claims.Email = email
	}

	// Optional: preferred_username
	if username, ok := tokenClaims["preferred_username"].(string); ok {
		claims.PreferredUsername = username
	}

	// Optional: name
	if name, ok := tokenClaims["name"].(string); ok {
		claims.Name = name
	}

	claims.Groups = extractGroupsClaim(tokenClaims)

	// Optional: tenant_id (custom claim)
	if tenantID, ok := tokenClaims["tenant_id"].(string); ok {
		claims.TenantID = tenantID
	}

	// Security-relevant claims for audience/issuer/client_id pinning and
	// revocation. All are optional at extraction time; enforcement happens
	// in verifyTokenBinding / checkRevocation based on configuration.
	if jti, ok := tokenClaims["jti"].(string); ok {
		claims.JTI = jti
	}
	claims.Audience = extractAudienceClaim(tokenClaims)
	if iss, ok := tokenClaims["iss"].(string); ok {
		claims.Issuer = iss
	}
	claims.ClientID = extractClientIDClaim(tokenClaims)
	if exp, ok := tokenClaims["exp"].(float64); ok {
		claims.ExpiresAt = int64(exp)
	}

	return claims, nil
}

// extractGroupsClaim returns string entries from the "groups" claim.
// Non-string entries are dropped silently.
func extractGroupsClaim(tokenClaims map[string]interface{}) []string {
	raw, ok := tokenClaims["groups"].([]interface{})
	if !ok {
		return nil
	}
	var groups []string
	for _, g := range raw {
		if s, ok := g.(string); ok {
			groups = append(groups, s)
		}
	}
	return groups
}

// extractAudienceClaim accepts both a single string and a []string/[]interface{}
// shape for the "aud" claim, per RFC 7519 section 4.1.3.
func extractAudienceClaim(tokenClaims map[string]interface{}) []string {
	raw, ok := tokenClaims["aud"]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	case []interface{}:
		audiences := make([]string, 0, len(v))
		for _, entry := range v {
			if s, ok := entry.(string); ok && s != "" {
				audiences = append(audiences, s)
			}
		}
		return audiences
	case []string:
		return v
	default:
		return nil
	}
}

// extractClientIDClaim reads the client identifier from the token.
// Keycloak introspection exposes this as "client_id"; id-tokens use "azp".
// We prefer "client_id" but fall back to "azp" for compatibility.
func extractClientIDClaim(tokenClaims map[string]interface{}) string {
	if cid, ok := tokenClaims["client_id"].(string); ok && cid != "" {
		return cid
	}
	if azp, ok := tokenClaims["azp"].(string); ok {
		return azp
	}
	return ""
}

// validateClaims validates that required claims are present.
func (a *OAuth2Authenticator) validateClaims(claims *OAuth2Claims) error {
	if claims.Subject == "" {
		return fmt.Errorf("missing required 'sub' claim")
	}

	if a.config.RequireTenantClaim && claims.TenantID == "" {
		return fmt.Errorf("missing required 'tenant_id' claim")
	}

	return nil
}

// getOrCreateUser retrieves an existing user by OAuth subject or provisions a new one.
// It first tries OAuth subject lookup, then falls back to email lookup (to link
// mTLS-bootstrapped users with their OAuth identity), and finally auto-provisions.
func (a *OAuth2Authenticator) getOrCreateUser(
	ctx context.Context,
	claims *OAuth2Claims,
	requestID string,
) (*TenantUser, error) {
	// Try to find existing user by OAuth subject
	user, err := a.store.GetUserByOAuthSubject(ctx, claims.Subject)
	if err == nil {
		return user, nil
	}
	if err != ErrUserNotFound {
		return nil, fmt.Errorf("failed to lookup user by OAuth subject: %w", err)
	}

	// OAuth subject not found — try email fallback to link existing users
	// (e.g., users bootstrapped via mTLS/CLI that don't yet have an OAuthSubject)
	if claims.Email != "" {
		user, err = a.linkExistingUserByEmail(ctx, claims, requestID)
		if err == nil {
			return user, nil
		}
		// Only continue if the error was "not found"; propagate other errors
		if err != ErrUserNotFound {
			return nil, err
		}
	}

	// User not found by any method — check if auto-provisioning is enabled
	if !a.config.AutoProvisionUsers {
		return nil, fmt.Errorf("user not found and auto-provisioning disabled")
	}

	// Auto-provision new user
	return a.provisionUser(ctx, claims, requestID)
}

// linkExistingUserByEmail finds a user by email and links their OAuth subject.
// This bridges mTLS-bootstrapped users with OAuth2 identity on first OAuth2 login.
func (a *OAuth2Authenticator) linkExistingUserByEmail(
	ctx context.Context,
	claims *OAuth2Claims,
	requestID string,
) (*TenantUser, error) {
	user, err := a.store.GetUserByEmail(ctx, claims.Email)
	if err != nil {
		return nil, err
	}

	// Link the OAuth subject to this existing user
	user.OAuthSubject = claims.Subject
	user.OAuthProvider = "keycloak"
	if err := a.store.UpdateUser(ctx, user); err != nil {
		return nil, fmt.Errorf("failed to link OAuth subject to existing user: %w", err)
	}

	a.logger.Info("Linked OAuth2 subject to existing user",
		zap.String("request_id", requestID),
		zap.String("user_id", user.ID),
		zap.String("email", claims.Email),
		zap.String("oauth_subject", claims.Subject),
	)

	return user, nil
}

// provisionUser creates a new user from OAuth2 claims.
func (a *OAuth2Authenticator) provisionUser(
	ctx context.Context,
	claims *OAuth2Claims,
	requestID string,
) (*TenantUser, error) {
	tenantID := claims.TenantID
	if tenantID == "" {
		return nil, fmt.Errorf("cannot provision user without tenant_id claim")
	}

	// Validate tenant and quota
	_, err := a.validateTenantForProvisioning(ctx, tenantID)
	if err != nil {
		return nil, err
	}

	// Determine role from group mappings
	roleID, err := a.determineRole(ctx, claims, tenantID)
	if err != nil {
		return nil, err
	}

	// Build and create user
	user := a.buildUserFromClaims(claims, tenantID, roleID)
	if err := a.store.CreateUser(ctx, user); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	a.logger.Info("Auto-provisioned OAuth2 user",
		zap.String("request_id", requestID),
		zap.String("user_id", user.ID),
		zap.String("tenant_id", tenantID),
		zap.String("oauth_subject", claims.Subject),
		zap.String("email", claims.Email),
	)

	return user, nil
}

// validateTenantForProvisioning validates tenant exists, is active, and has quota available.
func (a *OAuth2Authenticator) validateTenantForProvisioning(
	ctx context.Context,
	tenantID string,
) (*Tenant, error) {
	tenant, err := a.store.GetTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("tenant not found: %w", err)
	}
	if !tenant.IsActive() {
		return nil, fmt.Errorf("tenant is not active")
	}

	// Check tenant user quota
	users, err := a.store.ListUsersByTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to check tenant quota: %w", err)
	}
	if tenant.Quota.MaxUsers > 0 && len(users) >= tenant.Quota.MaxUsers {
		return nil, fmt.Errorf("tenant user quota exceeded")
	}

	return tenant, nil
}

// buildUserFromClaims constructs a TenantUser from OAuth2 claims.
func (a *OAuth2Authenticator) buildUserFromClaims(
	claims *OAuth2Claims,
	tenantID string,
	roleID string,
) *TenantUser {
	// Determine common name (prefer preferred_username, fallback to email or name)
	commonName := claims.PreferredUsername
	if commonName == "" {
		commonName = claims.Email
	}
	if commonName == "" {
		commonName = claims.Name
	}

	return &TenantUser{
		ID:            uuid.New().String(),
		TenantID:      tenantID,
		Subject:       "", // No certificate DN for OAuth users
		OAuthSubject:  claims.Subject,
		OAuthProvider: "keycloak",
		Email:         claims.Email,
		CommonName:    commonName,
		RoleID:        roleID,
		IsActive:      true,
		CreatedAt:     time.Now().UTC(),
	}
}

// determineRole maps Keycloak groups to a role ID.
func (a *OAuth2Authenticator) determineRole(
	ctx context.Context,
	claims *OAuth2Claims,
	tenantID string,
) (string, error) {
	// Check group-to-role mappings
	for _, group := range claims.Groups {
		if roleID, ok := a.config.GroupRoleMapping[group]; ok {
			// Verify role exists
			role, err := a.store.GetRole(ctx, roleID)
			if err == nil {
				a.logger.Debug("Mapped OAuth group to role",
					zap.String("group", group),
					zap.String("role_id", roleID),
					zap.String("role_name", string(role.Name)),
				)
				return roleID, nil
			}
		}
	}

	// Fall back to default role
	if a.config.DefaultRole != "" {
		// Verify default role exists
		role, err := a.store.GetRole(ctx, a.config.DefaultRole)
		if err == nil {
			a.logger.Debug("Using default role for OAuth user",
				zap.String("role_id", a.config.DefaultRole),
				zap.String("role_name", string(role.Name)),
			)
			return a.config.DefaultRole, nil
		}
	}

	return "", fmt.Errorf(
		"no valid role found for user (checked %d groups, no mapping or default role)",
		len(claims.Groups),
	)
}
