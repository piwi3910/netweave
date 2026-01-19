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

// Authenticate performs OAuth2 authentication for an incoming request.
// Returns the authenticated user, role, and tenant on success.
func (a *OAuth2Authenticator) Authenticate(
	ctx context.Context,
	c *gin.Context,
	requestID string,
) (*TenantUser, *Role, *Tenant, error) {
	// Extract Bearer token from Authorization header
	token, err := a.extractBearerToken(c)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to extract bearer token: %w", err)
	}

	// Verify token with Keycloak
	tokenClaims, err := a.keycloakClient.VerifyToken(ctx, token)
	if err != nil {
		a.logger.Warn("OAuth2 token verification failed",
			zap.String("requestID", requestID),
			zap.Error(err),
		)
		return nil, nil, nil, fmt.Errorf("invalid token: %w", err)
	}

	// Extract structured claims from token
	claims, err := a.extractClaims(tokenClaims)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to extract claims: %w", err)
	}

	// Validate required claims
	if err := a.validateClaims(claims); err != nil {
		return nil, nil, nil, err
	}

	// Get or create user from claims
	user, err := a.getOrCreateUser(ctx, claims, requestID)
	if err != nil {
		return nil, nil, nil, err
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

	// Update last login timestamp
	_ = a.store.UpdateLastLogin(ctx, user.ID)

	return user, role, tenant, nil
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

	// Optional: groups (array of strings)
	if groupsRaw, ok := tokenClaims["groups"].([]interface{}); ok {
		for _, g := range groupsRaw {
			if groupStr, ok := g.(string); ok {
				claims.Groups = append(claims.Groups, groupStr)
			}
		}
	}

	// Optional: tenant_id (custom claim)
	if tenantID, ok := tokenClaims["tenant_id"].(string); ok {
		claims.TenantID = tenantID
	}

	return claims, nil
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
func (a *OAuth2Authenticator) getOrCreateUser(
	ctx context.Context,
	claims *OAuth2Claims,
	requestID string,
) (*TenantUser, error) {
	// Try to find existing user by OAuth subject
	user, err := a.store.GetUserByOAuthSubject(ctx, claims.Subject)
	if err == nil {
		// User found
		return user, nil
	}
	if err != ErrUserNotFound {
		return nil, fmt.Errorf("failed to lookup user by OAuth subject: %w", err)
	}

	// User not found - check if auto-provisioning is enabled
	if !a.config.AutoProvisionUsers {
		return nil, fmt.Errorf("user not found and auto-provisioning disabled")
	}

	// Auto-provision new user
	return a.provisionUser(ctx, claims, requestID)
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
		zap.String("requestID", requestID),
		zap.String("userID", user.ID),
		zap.String("tenantID", tenantID),
		zap.String("oauthSubject", claims.Subject),
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
					zap.String("roleID", roleID),
					zap.String("roleName", string(role.Name)),
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
				zap.String("roleID", a.config.DefaultRole),
				zap.String("roleName", string(role.Name)),
			)
			return a.config.DefaultRole, nil
		}
	}

	return "", fmt.Errorf(
		"no valid role found for user (checked %d groups, no mapping or default role)",
		len(claims.Groups),
	)
}
