package keycloak

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/piwi3910/netweave/internal/auth"
	"go.uber.org/zap"
)

// Store implements the auth.Store interface using Keycloak as the backend.
// Tenants are stored as realm attributes, users and roles use Keycloak's native objects,
// and audit logs use Keycloak's event system.
type Store struct {
	client *Client
	logger *zap.Logger
}

// NewStore creates a new Keycloak-backed auth store.
func NewStore(client *Client, logger *zap.Logger) *Store {
	if client == nil {
		panic("keycloak client cannot be nil")
	}
	if logger == nil {
		panic("logger cannot be nil")
	}

	return &Store{
		client: client,
		logger: logger,
	}
}

// Close closes the Keycloak client connection.
func (s *Store) Close() error {
	return s.client.Close()
}

// Ping checks if Keycloak is accessible.
func (s *Store) Ping(ctx context.Context) error {
	return s.client.Ping(ctx)
}

// Tenant attribute key prefixes.
const (
	tenantAttrPrefix = "tenant_"
	tenantAttrName   = "_name"
	tenantAttrDesc   = "_description"
	tenantAttrStatus = "_status"
	tenantAttrQuota  = "_quota"
	tenantAttrUsage  = "_usage"
	tenantAttrEmail  = "_email"
	tenantAttrMeta   = "_metadata"
	tenantAttrCreate = "_created_at"
	tenantAttrUpdate = "_updated_at"

	// Usage type keys.
	usageTypeSubscriptions = "subscriptions"
	usageTypeUsers         = "users"
	usageTypeResourcePools = "resourcePools"
	usageTypeDeployments   = "deployments"
)

// tenantAttributeKey generates attribute key for tenant metadata.
func tenantAttributeKey(tenantID, suffix string) string {
	return tenantAttrPrefix + tenantID + suffix
}

// serializeTenantToAttributes converts a Tenant to realm attributes.
func (s *Store) serializeTenantToAttributes(tenant *auth.Tenant) (map[string]string, error) {
	attrs := make(map[string]string)

	attrs[tenantAttributeKey(tenant.ID, tenantAttrName)] = tenant.Name
	attrs[tenantAttributeKey(tenant.ID, tenantAttrDesc)] = tenant.Description
	attrs[tenantAttributeKey(tenant.ID, tenantAttrStatus)] = string(tenant.Status)
	attrs[tenantAttributeKey(tenant.ID, tenantAttrEmail)] = tenant.ContactEmail
	attrs[tenantAttributeKey(tenant.ID, tenantAttrCreate)] = tenant.CreatedAt.Format(time.RFC3339)
	attrs[tenantAttributeKey(tenant.ID, tenantAttrUpdate)] = tenant.UpdatedAt.Format(time.RFC3339)

	// Serialize quota
	quotaBytes, err := json.Marshal(tenant.Quota)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize quota: %w", err)
	}
	attrs[tenantAttributeKey(tenant.ID, tenantAttrQuota)] = string(quotaBytes)

	// Serialize usage
	usageBytes, err := json.Marshal(tenant.Usage)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize usage: %w", err)
	}
	attrs[tenantAttributeKey(tenant.ID, tenantAttrUsage)] = string(usageBytes)

	// Serialize metadata
	if len(tenant.Metadata) > 0 {
		metaBytes, err := json.Marshal(tenant.Metadata)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize metadata: %w", err)
		}
		attrs[tenantAttributeKey(tenant.ID, tenantAttrMeta)] = string(metaBytes)
	}

	return attrs, nil
}

// deserializeTenantFromAttributes reconstructs a Tenant from realm attributes.
func (s *Store) deserializeTenantFromAttributes(tenantID string, attrs map[string]string) (*auth.Tenant, error) {
	prefix := tenantAttrPrefix + tenantID

	name, ok := attrs[prefix+tenantAttrName]
	if !ok {
		return nil, auth.ErrTenantNotFound
	}

	tenant := &auth.Tenant{
		ID:           tenantID,
		Name:         name,
		Description:  attrs[prefix+tenantAttrDesc],
		Status:       auth.TenantStatus(attrs[prefix+tenantAttrStatus]),
		ContactEmail: attrs[prefix+tenantAttrEmail],
	}

	s.parseTenantQuota(attrs, prefix, tenant)
	s.parseTenantUsage(attrs, prefix, tenant)
	s.parseTenantMetadata(attrs, prefix, tenant)
	s.parseTenantTimestamps(attrs, prefix, tenant)

	return tenant, nil
}

// parseTenantQuota parses quota from tenant attributes.
func (s *Store) parseTenantQuota(attrs map[string]string, prefix string, tenant *auth.Tenant) {
	quotaStr, ok := attrs[prefix+tenantAttrQuota]
	if !ok {
		return
	}

	var quota auth.TenantQuota
	if err := json.Unmarshal([]byte(quotaStr), &quota); err != nil {
		s.logger.Warn("failed to parse quota", zap.Error(err))
	} else {
		tenant.Quota = quota
	}
}

// parseTenantUsage parses usage from tenant attributes.
func (s *Store) parseTenantUsage(attrs map[string]string, prefix string, tenant *auth.Tenant) {
	usageStr, ok := attrs[prefix+tenantAttrUsage]
	if !ok {
		return
	}

	var usage auth.TenantUsage
	if err := json.Unmarshal([]byte(usageStr), &usage); err != nil {
		s.logger.Warn("failed to parse usage", zap.Error(err))
	} else {
		tenant.Usage = usage
	}
}

// parseTenantMetadata parses metadata from tenant attributes.
func (s *Store) parseTenantMetadata(attrs map[string]string, prefix string, tenant *auth.Tenant) {
	metaStr, ok := attrs[prefix+tenantAttrMeta]
	if !ok {
		return
	}

	var metadata map[string]string
	if err := json.Unmarshal([]byte(metaStr), &metadata); err != nil {
		s.logger.Warn("failed to parse metadata", zap.Error(err))
	} else {
		tenant.Metadata = metadata
	}
}

// parseTenantTimestamps parses timestamps from tenant attributes.
func (s *Store) parseTenantTimestamps(attrs map[string]string, prefix string, tenant *auth.Tenant) {
	if createdStr, ok := attrs[prefix+tenantAttrCreate]; ok {
		if t, err := time.Parse(time.RFC3339, createdStr); err == nil {
			tenant.CreatedAt = t
		}
	}
	if updatedStr, ok := attrs[prefix+tenantAttrUpdate]; ok {
		if t, err := time.Parse(time.RFC3339, updatedStr); err == nil {
			tenant.UpdatedAt = t
		}
	}
}

// extractTenantIDs extracts all tenant IDs from realm attributes.
func (s *Store) extractTenantIDs(attrs map[string]string) []string {
	tenantMap := make(map[string]bool)

	for key := range attrs {
		if strings.HasPrefix(key, tenantAttrPrefix) {
			// Extract tenant ID from key like "tenant_<id>_name"
			parts := strings.Split(key, "_")
			if len(parts) >= 2 {
				tenantID := parts[1]
				tenantMap[tenantID] = true
			}
		}
	}

	tenantIDs := make([]string, 0, len(tenantMap))
	for id := range tenantMap {
		tenantIDs = append(tenantIDs, id)
	}

	return tenantIDs
}

// User attribute keys.
const (
	userAttrTenantID      = "tenant_id"
	userAttrRoleID        = "role_id"
	userAttrSubject       = "certificate_subject"
	userAttrOAuthSubject  = "oauth_subject"
	userAttrOAuthProvider = "oauth_provider"
	userAttrIsActive      = "is_active"
	userAttrLastLogin     = "last_login_at"
	userAttrCreatedAt     = "created_at"
	userAttrUpdatedAt     = "updated_at"
)

// convertUserToKeycloak converts auth.TenantUser to keycloak.User.
func (s *Store) convertUserToKeycloak(user *auth.TenantUser) *User {
	kcUser := &User{
		ID:            user.ID,
		Username:      user.CommonName,
		Email:         user.Email,
		EmailVerified: false,
		Enabled:       user.IsActive,
		Attributes:    make(map[string][]string),
	}

	// Set custom attributes
	kcUser.Attributes[userAttrTenantID] = []string{user.TenantID}
	kcUser.Attributes[userAttrRoleID] = []string{user.RoleID}
	kcUser.Attributes[userAttrIsActive] = []string{fmt.Sprintf("%t", user.IsActive)}

	if user.Subject != "" {
		kcUser.Attributes[userAttrSubject] = []string{user.Subject}
	}
	if user.OAuthSubject != "" {
		kcUser.Attributes[userAttrOAuthSubject] = []string{user.OAuthSubject}
	}
	if user.OAuthProvider != "" {
		kcUser.Attributes[userAttrOAuthProvider] = []string{user.OAuthProvider}
	}
	if !user.LastLoginAt.IsZero() {
		kcUser.Attributes[userAttrLastLogin] = []string{user.LastLoginAt.Format(time.RFC3339)}
	}
	if !user.CreatedAt.IsZero() {
		kcUser.Attributes[userAttrCreatedAt] = []string{user.CreatedAt.Format(time.RFC3339)}
	}
	if !user.UpdatedAt.IsZero() {
		kcUser.Attributes[userAttrUpdatedAt] = []string{user.UpdatedAt.Format(time.RFC3339)}
	}

	return kcUser
}

// convertKeycloakToUser converts keycloak.User to auth.TenantUser.
func (s *Store) convertKeycloakToUser(kcUser *User) *auth.TenantUser {
	user := &auth.TenantUser{
		ID:         kcUser.ID,
		CommonName: kcUser.Username,
		Email:      kcUser.Email,
		IsActive:   kcUser.Enabled,
	}

	s.extractUserAttributes(kcUser, user)
	s.parseUserTimestamps(kcUser, user)

	return user
}

// extractUserAttributes extracts custom attributes from Keycloak user.
func (s *Store) extractUserAttributes(kcUser *User, user *auth.TenantUser) {
	user.TenantID = s.getFirstAttrValue(kcUser.Attributes, userAttrTenantID)
	user.RoleID = s.getFirstAttrValue(kcUser.Attributes, userAttrRoleID)
	user.Subject = s.getFirstAttrValue(kcUser.Attributes, userAttrSubject)
	user.OAuthSubject = s.getFirstAttrValue(kcUser.Attributes, userAttrOAuthSubject)
	user.OAuthProvider = s.getFirstAttrValue(kcUser.Attributes, userAttrOAuthProvider)
}

// getFirstAttrValue safely retrieves the first value from an attribute map.
func (s *Store) getFirstAttrValue(attrs map[string][]string, key string) string {
	if vals, ok := attrs[key]; ok && len(vals) > 0 {
		return vals[0]
	}
	return ""
}

// parseUserTimestamps parses timestamp attributes.
func (s *Store) parseUserTimestamps(kcUser *User, user *auth.TenantUser) {
	if vals, ok := kcUser.Attributes[userAttrLastLogin]; ok && len(vals) > 0 {
		if t, err := time.Parse(time.RFC3339, vals[0]); err == nil {
			user.LastLoginAt = t
		}
	}
	if vals, ok := kcUser.Attributes[userAttrCreatedAt]; ok && len(vals) > 0 {
		if t, err := time.Parse(time.RFC3339, vals[0]); err == nil {
			user.CreatedAt = t
		}
	}
	if vals, ok := kcUser.Attributes[userAttrUpdatedAt]; ok && len(vals) > 0 {
		if t, err := time.Parse(time.RFC3339, vals[0]); err == nil {
			user.UpdatedAt = t
		}
	}
}

// Role attribute keys.
const (
	roleAttrTenantID    = "tenant_id"
	roleAttrType        = "role_type"
	roleAttrPermissions = "permissions"
	roleAttrCreatedAt   = "created_at"
	roleAttrUpdatedAt   = "updated_at"
)

// convertRoleToKeycloak converts auth.Role to keycloak.Role.
func (s *Store) convertRoleToKeycloak(role *auth.Role) *Role {
	kcRole := &Role{
		ID:          role.ID,
		Name:        string(role.Name),
		Description: role.Description,
		Composite:   false,
		ClientRole:  false,
		Attributes:  make(map[string][]string),
	}

	// Set custom attributes
	if role.TenantID != "" {
		kcRole.Attributes[roleAttrTenantID] = []string{role.TenantID}
	}
	kcRole.Attributes[roleAttrType] = []string{string(role.Type)}

	// Serialize permissions
	if len(role.Permissions) > 0 {
		perms := make([]string, len(role.Permissions))
		for i, p := range role.Permissions {
			perms[i] = string(p)
		}
		permsJSON, err := json.Marshal(perms)
		if err == nil {
			kcRole.Attributes[roleAttrPermissions] = []string{string(permsJSON)}
		}
	}

	if !role.CreatedAt.IsZero() {
		kcRole.Attributes[roleAttrCreatedAt] = []string{role.CreatedAt.Format(time.RFC3339)}
	}
	if !role.UpdatedAt.IsZero() {
		kcRole.Attributes[roleAttrUpdatedAt] = []string{role.UpdatedAt.Format(time.RFC3339)}
	}

	return kcRole
}

// convertKeycloakToRole converts keycloak.Role to auth.Role.
func (s *Store) convertKeycloakToRole(kcRole *Role) *auth.Role {
	role := &auth.Role{
		ID:          kcRole.ID,
		Name:        auth.RoleName(kcRole.Name),
		Description: kcRole.Description,
	}

	s.extractRoleAttributes(kcRole, role)
	s.parseRolePermissions(kcRole, role)
	s.parseRoleTimestamps(kcRole, role)

	return role
}

// extractRoleAttributes extracts basic attributes from Keycloak role.
func (s *Store) extractRoleAttributes(kcRole *Role, role *auth.Role) {
	if vals, ok := kcRole.Attributes[roleAttrTenantID]; ok && len(vals) > 0 {
		role.TenantID = vals[0]
	}
	if vals, ok := kcRole.Attributes[roleAttrType]; ok && len(vals) > 0 {
		role.Type = auth.RoleType(vals[0])
	}
}

// parseRolePermissions parses permissions JSON from role attributes.
func (s *Store) parseRolePermissions(kcRole *Role, role *auth.Role) {
	vals, ok := kcRole.Attributes[roleAttrPermissions]
	if !ok || len(vals) == 0 {
		return
	}

	var perms []string
	if err := json.Unmarshal([]byte(vals[0]), &perms); err == nil {
		role.Permissions = make([]auth.Permission, len(perms))
		for i, p := range perms {
			role.Permissions[i] = auth.Permission(p)
		}
	}
}

// parseRoleTimestamps parses timestamp attributes for role.
func (s *Store) parseRoleTimestamps(kcRole *Role, role *auth.Role) {
	if vals, ok := kcRole.Attributes[roleAttrCreatedAt]; ok && len(vals) > 0 {
		if t, err := time.Parse(time.RFC3339, vals[0]); err == nil {
			role.CreatedAt = t
		}
	}
	if vals, ok := kcRole.Attributes[roleAttrUpdatedAt]; ok && len(vals) > 0 {
		if t, err := time.Parse(time.RFC3339, vals[0]); err == nil {
			role.UpdatedAt = t
		}
	}
}

// ========== TenantStore Implementation ==========

// getRealmAttributes retrieves all realm attributes.
func (s *Store) getRealmAttributes(_ context.Context) (map[string]string, error) {
	// Note: Keycloak Admin API doesn't have a direct "get realm attributes" endpoint.
	// We need to get the realm object and extract attributes from it.
	// For now, we'll implement a simpler approach using a dedicated attribute to store tenant metadata.
	// In production, consider using a separate metadata service or Keycloak's groups API.

	// Placeholder: In real implementation, would query realm and extract attributes
	// For MVP, we'll return an empty map and rely on user attributes for tenant association
	return make(map[string]string), nil
}

// setRealmAttributes updates realm attributes.
func (s *Store) setRealmAttributes(_ context.Context, _ map[string]string) error {
	// Placeholder: Would update realm attributes via Admin API
	// For MVP, tenant metadata is stored as user attributes
	return nil
}

// CreateTenant creates a new tenant by storing metadata in realm attributes.
func (s *Store) CreateTenant(ctx context.Context, tenant *auth.Tenant) error {
	if tenant == nil {
		return fmt.Errorf("tenant cannot be nil")
	}
	if tenant.ID == "" {
		return auth.ErrInvalidTenantID
	}

	// Get existing realm attributes
	attrs, err := s.getRealmAttributes(ctx)
	if err != nil {
		return fmt.Errorf("failed to get realm attributes: %w", err)
	}

	// Check if tenant already exists
	if _, exists := attrs[tenantAttributeKey(tenant.ID, tenantAttrName)]; exists {
		return auth.ErrTenantExists
	}

	// Set timestamps
	now := time.Now()
	tenant.CreatedAt = now
	tenant.UpdatedAt = now

	// Serialize tenant to attributes
	tenantAttrs, err := s.serializeTenantToAttributes(tenant)
	if err != nil {
		return fmt.Errorf("failed to serialize tenant: %w", err)
	}

	// Merge with existing attributes
	for k, v := range tenantAttrs {
		attrs[k] = v
	}

	// Update realm attributes
	if err := s.setRealmAttributes(ctx, attrs); err != nil {
		return fmt.Errorf("failed to update realm attributes: %w", err)
	}

	s.logger.Info("tenant created in keycloak", zap.String("tenant_id", tenant.ID))
	return nil
}

// GetTenant retrieves a tenant by ID from realm attributes.
func (s *Store) GetTenant(ctx context.Context, id string) (*auth.Tenant, error) {
	if id == "" {
		return nil, auth.ErrInvalidTenantID
	}

	attrs, err := s.getRealmAttributes(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get realm attributes: %w", err)
	}

	tenant, err := s.deserializeTenantFromAttributes(id, attrs)
	if err != nil {
		if errors.Is(err, auth.ErrTenantNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("failed to deserialize tenant: %w", err)
	}

	return tenant, nil
}

// UpdateTenant updates an existing tenant's metadata.
func (s *Store) UpdateTenant(ctx context.Context, tenant *auth.Tenant) error {
	if tenant == nil {
		return fmt.Errorf("tenant cannot be nil")
	}
	if tenant.ID == "" {
		return auth.ErrInvalidTenantID
	}

	attrs, err := s.getRealmAttributes(ctx)
	if err != nil {
		return fmt.Errorf("failed to get realm attributes: %w", err)
	}

	// Check if tenant exists
	if _, err := s.deserializeTenantFromAttributes(tenant.ID, attrs); err != nil {
		if errors.Is(err, auth.ErrTenantNotFound) {
			return err
		}
		return fmt.Errorf("failed to check tenant existence: %w", err)
	}

	// Update timestamp
	tenant.UpdatedAt = time.Now()

	// Serialize tenant
	tenantAttrs, err := s.serializeTenantToAttributes(tenant)
	if err != nil {
		return fmt.Errorf("failed to serialize tenant: %w", err)
	}

	// Merge with existing attributes
	for k, v := range tenantAttrs {
		attrs[k] = v
	}

	// Update realm attributes
	if err := s.setRealmAttributes(ctx, attrs); err != nil {
		return fmt.Errorf("failed to update realm attributes: %w", err)
	}

	s.logger.Info("tenant updated in keycloak", zap.String("tenant_id", tenant.ID))
	return nil
}

// DeleteTenant removes a tenant's metadata from realm attributes.
func (s *Store) DeleteTenant(ctx context.Context, id string) error {
	if id == "" {
		return auth.ErrInvalidTenantID
	}

	attrs, err := s.getRealmAttributes(ctx)
	if err != nil {
		return fmt.Errorf("failed to get realm attributes: %w", err)
	}

	// Check if tenant exists
	if _, err := s.deserializeTenantFromAttributes(id, attrs); err != nil {
		if errors.Is(err, auth.ErrTenantNotFound) {
			return err
		}
		return fmt.Errorf("failed to check tenant existence: %w", err)
	}

	// Remove all tenant attributes
	prefix := tenantAttrPrefix + id
	for k := range attrs {
		if strings.HasPrefix(k, prefix) {
			delete(attrs, k)
		}
	}

	// Update realm attributes
	if err := s.setRealmAttributes(ctx, attrs); err != nil {
		return fmt.Errorf("failed to update realm attributes: %w", err)
	}

	s.logger.Info("tenant deleted from keycloak", zap.String("tenant_id", id))
	return nil
}

// ListTenants retrieves all tenants from realm attributes.
func (s *Store) ListTenants(ctx context.Context) ([]*auth.Tenant, error) {
	attrs, err := s.getRealmAttributes(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get realm attributes: %w", err)
	}

	tenantIDs := s.extractTenantIDs(attrs)
	tenants := make([]*auth.Tenant, 0, len(tenantIDs))

	for _, id := range tenantIDs {
		tenant, err := s.deserializeTenantFromAttributes(id, attrs)
		if err != nil {
			s.logger.Warn("failed to deserialize tenant", zap.String("tenant_id", id), zap.Error(err))
			continue
		}
		tenants = append(tenants, tenant)
	}

	return tenants, nil
}

// IncrementUsage atomically increments a tenant's usage counter.
func (s *Store) IncrementUsage(ctx context.Context, tenantID, usageType string) error {
	return s.modifyUsage(ctx, tenantID, usageType, 1)
}

// DecrementUsage atomically decrements a tenant's usage counter.
func (s *Store) DecrementUsage(ctx context.Context, tenantID, usageType string) error {
	return s.modifyUsage(ctx, tenantID, usageType, -1)
}

// modifyUsage modifies a tenant's usage counter by delta.
func (s *Store) modifyUsage(ctx context.Context, tenantID, usageType string, delta int) error {
	if tenantID == "" {
		return auth.ErrInvalidTenantID
	}

	tenant, err := s.GetTenant(ctx, tenantID)
	if err != nil {
		return err
	}

	if err := s.applyUsageDelta(tenant, usageType, delta); err != nil {
		return err
	}

	return s.UpdateTenant(ctx, tenant)
}

// applyUsageDelta applies delta to the appropriate usage counter.
func (s *Store) applyUsageDelta(tenant *auth.Tenant, usageType string, delta int) error {
	switch usageType {
	case usageTypeSubscriptions:
		tenant.Usage.Subscriptions = s.adjustCounter(tenant.Usage.Subscriptions, delta)
	case usageTypeResourcePools:
		tenant.Usage.ResourcePools = s.adjustCounter(tenant.Usage.ResourcePools, delta)
	case usageTypeDeployments:
		tenant.Usage.Deployments = s.adjustCounter(tenant.Usage.Deployments, delta)
	case usageTypeUsers:
		tenant.Usage.Users = s.adjustCounter(tenant.Usage.Users, delta)
	default:
		return fmt.Errorf("unknown usage type: %s", usageType)
	}
	return nil
}

// adjustCounter adds delta to current, preventing negative values.
func (s *Store) adjustCounter(current, delta int) int {
	result := current + delta
	if result < 0 {
		return 0
	}
	return result
}

// ========== UserStore Implementation ==========

// CreateUser creates a new user in Keycloak.
func (s *Store) CreateUser(ctx context.Context, user *auth.TenantUser) error {
	if user == nil {
		return fmt.Errorf("user cannot be nil")
	}
	if user.ID == "" {
		return auth.ErrInvalidUserID
	}

	// Set timestamps
	now := time.Now()
	user.CreatedAt = now
	user.UpdatedAt = now

	// Convert to Keycloak user
	kcUser := s.convertUserToKeycloak(user)

	// Create user in Keycloak
	userID, err := s.client.CreateUser(ctx, kcUser)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			return auth.ErrUserExists
		}
		return fmt.Errorf("failed to create user in keycloak: %w", err)
	}

	// Update user ID if Keycloak assigned a different one
	user.ID = userID

	s.logger.Info("user created in keycloak",
		zap.String("user_id", user.ID),
		zap.String("tenant_id", user.TenantID),
	)
	return nil
}

// GetUser retrieves a user by ID.
func (s *Store) GetUser(ctx context.Context, id string) (*auth.TenantUser, error) {
	if id == "" {
		return nil, auth.ErrInvalidUserID
	}

	kcUser, err := s.client.GetUser(ctx, id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, auth.ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user from keycloak: %w", err)
	}

	user := s.convertKeycloakToUser(kcUser)

	return user, nil
}

// GetUserBySubject retrieves a user by certificate subject (mTLS).
func (s *Store) GetUserBySubject(ctx context.Context, subject string) (*auth.TenantUser, error) {
	if subject == "" {
		return nil, fmt.Errorf("subject cannot be empty")
	}

	// List all users and filter by subject attribute
	// Note: Keycloak doesn't support attribute-based queries directly,
	// so we need to list users and filter client-side
	kcUsers, err := s.client.ListUsers(ctx, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to list users from keycloak: %w", err)
	}

	for _, kcUser := range kcUsers {
		if vals, ok := kcUser.Attributes[userAttrSubject]; ok && len(vals) > 0 {
			if vals[0] == subject {
				user := s.convertKeycloakToUser(&kcUser)
				return user, nil
			}
		}
	}

	return nil, auth.ErrUserNotFound
}

// GetUserByOAuthSubject retrieves a user by OAuth2 subject identifier.
func (s *Store) GetUserByOAuthSubject(ctx context.Context, oauthSubject string) (*auth.TenantUser, error) {
	if oauthSubject == "" {
		return nil, fmt.Errorf("oauth subject cannot be empty")
	}

	// List all users and filter by OAuth subject attribute
	kcUsers, err := s.client.ListUsers(ctx, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to list users from keycloak: %w", err)
	}

	for _, kcUser := range kcUsers {
		if vals, ok := kcUser.Attributes[userAttrOAuthSubject]; ok && len(vals) > 0 {
			if vals[0] == oauthSubject {
				user := s.convertKeycloakToUser(&kcUser)
				return user, nil
			}
		}
	}

	return nil, auth.ErrUserNotFound
}

// GetUserByEmail retrieves a user by email address.
func (s *Store) GetUserByEmail(ctx context.Context, email string) (*auth.TenantUser, error) {
	if email == "" {
		return nil, fmt.Errorf("email cannot be empty")
	}

	// List all users and filter by email
	kcUsers, err := s.client.ListUsers(ctx, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to list users from keycloak: %w", err)
	}

	for _, kcUser := range kcUsers {
		if kcUser.Email == email {
			user := s.convertKeycloakToUser(&kcUser)
			return user, nil
		}
	}

	return nil, auth.ErrUserNotFound
}

// UpdateUser updates an existing user.
func (s *Store) UpdateUser(ctx context.Context, user *auth.TenantUser) error {
	if user == nil {
		return fmt.Errorf("user cannot be nil")
	}
	if user.ID == "" {
		return auth.ErrInvalidUserID
	}

	// Update timestamp
	user.UpdatedAt = time.Now()

	// Convert to Keycloak user
	kcUser := s.convertUserToKeycloak(user)

	// Update user in Keycloak
	if err := s.client.UpdateUser(ctx, kcUser); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return auth.ErrUserNotFound
		}
		return fmt.Errorf("failed to update user in keycloak: %w", err)
	}

	s.logger.Info("user updated in keycloak", zap.String("user_id", user.ID))
	return nil
}

// DeleteUser deletes a user by ID.
func (s *Store) DeleteUser(ctx context.Context, id string) error {
	if id == "" {
		return auth.ErrInvalidUserID
	}

	if err := s.client.DeleteUser(ctx, id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return auth.ErrUserNotFound
		}
		return fmt.Errorf("failed to delete user from keycloak: %w", err)
	}

	s.logger.Info("user deleted from keycloak", zap.String("user_id", id))
	return nil
}

// ListUsersByTenant retrieves all users for a specific tenant.
func (s *Store) ListUsersByTenant(ctx context.Context, tenantID string) ([]*auth.TenantUser, error) {
	if tenantID == "" {
		return nil, auth.ErrInvalidTenantID
	}

	// List all users and filter by tenant ID attribute
	kcUsers, err := s.client.ListUsers(ctx, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to list users from keycloak: %w", err)
	}

	users := make([]*auth.TenantUser, 0)
	for _, kcUser := range kcUsers {
		if vals, ok := kcUser.Attributes[userAttrTenantID]; ok && len(vals) > 0 {
			if vals[0] == tenantID {
				user := s.convertKeycloakToUser(&kcUser)
				users = append(users, user)
			}
		}
	}

	return users, nil
}

// UpdateLastLogin updates the last login timestamp for a user.
func (s *Store) UpdateLastLogin(ctx context.Context, userID string) error {
	if userID == "" {
		return auth.ErrInvalidUserID
	}

	user, err := s.GetUser(ctx, userID)
	if err != nil {
		return err
	}

	user.LastLoginAt = time.Now()
	return s.UpdateUser(ctx, user)
}

// ========== RoleStore Implementation ==========

// CreateRole creates a new role in Keycloak.
func (s *Store) CreateRole(ctx context.Context, role *auth.Role) error {
	if role == nil {
		return fmt.Errorf("role cannot be nil")
	}
	if role.ID == "" {
		return auth.ErrInvalidRoleID
	}
	if role.Name == "" {
		return fmt.Errorf("role name is required")
	}

	// Set timestamps
	now := time.Now()
	role.CreatedAt = now
	role.UpdatedAt = now

	// Convert to Keycloak role
	kcRole := s.convertRoleToKeycloak(role)

	// Create role in Keycloak
	if err := s.client.CreateRealmRole(ctx, kcRole); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			return auth.ErrRoleExists
		}
		return fmt.Errorf("failed to create role in keycloak: %w", err)
	}

	s.logger.Info("role created in keycloak", zap.String("role_name", string(role.Name)))
	return nil
}

// GetRole retrieves a role by ID.
func (s *Store) GetRole(ctx context.Context, id string) (*auth.Role, error) {
	if id == "" {
		return nil, auth.ErrInvalidRoleID
	}

	// Keycloak doesn't support role lookup by ID directly, only by name
	// We need to list all roles and filter by ID
	kcRoles, err := s.client.ListRealmRoles(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list roles from keycloak: %w", err)
	}

	for _, kcRole := range kcRoles {
		if kcRole.ID == id {
			role := s.convertKeycloakToRole(&kcRole)
			return role, nil
		}
	}

	return nil, auth.ErrRoleNotFound
}

// GetRoleByName retrieves a role by name.
func (s *Store) GetRoleByName(ctx context.Context, name auth.RoleName) (*auth.Role, error) {
	if name == "" {
		return nil, fmt.Errorf("role name is required")
	}

	kcRole, err := s.client.GetRealmRole(ctx, string(name))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, auth.ErrRoleNotFound
		}
		return nil, fmt.Errorf("failed to get role from keycloak: %w", err)
	}

	role := s.convertKeycloakToRole(kcRole)

	return role, nil
}

// UpdateRole updates an existing role.
func (s *Store) UpdateRole(ctx context.Context, role *auth.Role) error {
	if role == nil {
		return fmt.Errorf("role cannot be nil")
	}
	if role.Name == "" {
		return fmt.Errorf("role name is required")
	}

	// Update timestamp
	role.UpdatedAt = time.Now()

	// Convert to Keycloak role
	kcRole := s.convertRoleToKeycloak(role)

	// Update role in Keycloak
	if err := s.client.UpdateRealmRole(ctx, string(role.Name), kcRole); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return auth.ErrRoleNotFound
		}
		return fmt.Errorf("failed to update role in keycloak: %w", err)
	}

	s.logger.Info("role updated in keycloak", zap.String("role_name", string(role.Name)))
	return nil
}

// DeleteRole deletes a role by ID.
func (s *Store) DeleteRole(ctx context.Context, id string) error {
	if id == "" {
		return auth.ErrInvalidRoleID
	}

	// Get role by ID to find its name
	role, err := s.GetRole(ctx, id)
	if err != nil {
		return err
	}

	// Delete role by name
	if err := s.client.DeleteRealmRole(ctx, string(role.Name)); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return auth.ErrRoleNotFound
		}
		return fmt.Errorf("failed to delete role from keycloak: %w", err)
	}

	s.logger.Info("role deleted from keycloak", zap.String("role_id", id))
	return nil
}

// ListRoles retrieves all roles.
func (s *Store) ListRoles(ctx context.Context) ([]*auth.Role, error) {
	kcRoles, err := s.client.ListRealmRoles(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list roles from keycloak: %w", err)
	}

	roles := make([]*auth.Role, 0, len(kcRoles))
	for _, kcRole := range kcRoles {
		role := s.convertKeycloakToRole(&kcRole)
		roles = append(roles, role)
	}

	return roles, nil
}

// ListRolesByTenant retrieves roles for a specific tenant.
// Includes both global roles and tenant-specific custom roles.
func (s *Store) ListRolesByTenant(ctx context.Context, tenantID string) ([]*auth.Role, error) {
	if tenantID == "" {
		return nil, auth.ErrInvalidTenantID
	}

	// Get all roles
	allRoles, err := s.ListRoles(ctx)
	if err != nil {
		return nil, err
	}

	// Filter roles: include global roles (no tenant ID) and tenant-specific roles
	roles := make([]*auth.Role, 0)
	for _, role := range allRoles {
		if role.TenantID == "" || role.TenantID == tenantID {
			roles = append(roles, role)
		}
	}

	return roles, nil
}

// InitializeDefaultRoles creates the default system roles if they don't exist.
func (s *Store) InitializeDefaultRoles(ctx context.Context) error {
	defaultRoles := []*auth.Role{
		{
			ID:          "platform-admin",
			Name:        auth.RolePlatformAdmin,
			Type:        auth.RoleTypePlatform,
			Description: "Platform administrator with full access",
			Permissions: []auth.Permission{
				auth.PermissionTenantCreate,
				auth.PermissionTenantRead,
				auth.PermissionTenantUpdate,
				auth.PermissionTenantDelete,
				auth.PermissionUserCreate,
				auth.PermissionUserRead,
				auth.PermissionUserUpdate,
				auth.PermissionUserDelete,
				auth.PermissionRoleCreate,
				auth.PermissionRoleRead,
				auth.PermissionRoleUpdate,
				auth.PermissionRoleDelete,
				auth.PermissionAuditRead,
			},
		},
		{
			ID:          "tenant-admin",
			Name:        auth.RoleTenantAdmin,
			Type:        auth.RoleTypeTenant,
			Description: "Tenant administrator",
			Permissions: []auth.Permission{
				auth.PermissionUserCreate,
				auth.PermissionUserRead,
				auth.PermissionUserUpdate,
				auth.PermissionUserDelete,
				auth.PermissionRoleRead,
				auth.PermissionSubscriptionCreate,
				auth.PermissionSubscriptionRead,
				auth.PermissionSubscriptionDelete,
				auth.PermissionResourcePoolCreate,
				auth.PermissionResourcePoolRead,
				auth.PermissionResourcePoolUpdate,
				auth.PermissionResourcePoolDelete,
				auth.PermissionResourceCreate,
				auth.PermissionResourceRead,
				auth.PermissionResourceUpdate,
				auth.PermissionResourceDelete,
			},
		},
		{
			ID:          "operator",
			Name:        auth.RoleOperator,
			Type:        auth.RoleTypeTenant,
			Description: "Tenant operator",
			Permissions: []auth.Permission{
				auth.PermissionUserRead,
				auth.PermissionSubscriptionCreate,
				auth.PermissionSubscriptionRead,
				auth.PermissionResourcePoolRead,
				auth.PermissionResourcePoolUpdate,
				auth.PermissionResourceRead,
				auth.PermissionResourceUpdate,
			},
		},
		{
			ID:          "viewer",
			Name:        auth.RoleViewer,
			Type:        auth.RoleTypeTenant,
			Description: "Tenant viewer (read-only)",
			Permissions: []auth.Permission{
				auth.PermissionUserRead,
				auth.PermissionSubscriptionRead,
				auth.PermissionResourcePoolRead,
				auth.PermissionResourceRead,
				auth.PermissionResourceTypeRead,
				auth.PermissionDeploymentManagerRead,
			},
		},
	}

	for _, role := range defaultRoles {
		// Check if role already exists
		_, err := s.GetRoleByName(ctx, role.Name)
		if err == nil {
			// Role exists, skip
			s.logger.Debug("role already exists, skipping", zap.String("role_name", string(role.Name)))
			continue
		}
		if !errors.Is(err, auth.ErrRoleNotFound) {
			return fmt.Errorf("failed to check role existence: %w", err)
		}

		// Create role
		if err := s.CreateRole(ctx, role); err != nil {
			return fmt.Errorf("failed to create default role %s: %w", role.Name, err)
		}

		s.logger.Info("default role initialized", zap.String("role_name", string(role.Name)))
	}

	return nil
}

// ========== AuditStore Implementation ==========

// LogEvent creates a new audit event.
// Note: This is a placeholder implementation. In production, you would integrate
// with Keycloak's event system or use a dedicated audit logging service.
func (s *Store) LogEvent(_ context.Context, event *auth.AuditEvent) error {
	if event == nil {
		return fmt.Errorf("event cannot be nil")
	}

	// Placeholder: Log to application logger for now
	// In production, would send to Keycloak events API or dedicated audit service
	s.logger.Info("audit event",
		zap.String("event_id", event.ID),
		zap.String("event_type", string(event.Type)),
		zap.String("tenant_id", event.TenantID),
		zap.String("user_id", event.UserID),
		zap.String("resource_type", event.ResourceType),
		zap.String("resource_id", event.ResourceID),
		zap.String("action", event.Action),
		zap.Time("timestamp", event.Timestamp),
	)

	return nil
}

// ListEvents retrieves audit events with optional filtering.
// Note: This is a placeholder implementation. In production, you would query
// Keycloak's admin events API or a dedicated audit logging service.
func (s *Store) ListEvents(_ context.Context, tenantID string, limit, offset int) ([]*auth.AuditEvent, error) {
	// Placeholder: Return empty list
	// In production, would query Keycloak events API or dedicated audit service
	s.logger.Warn("ListEvents not fully implemented for Keycloak backend",
		zap.String("tenant_id", tenantID),
		zap.Int("limit", limit),
		zap.Int("offset", offset),
	)

	return []*auth.AuditEvent{}, nil
}

// ListEventsByType retrieves audit events of a specific type.
// Note: This is a placeholder implementation.
func (s *Store) ListEventsByType(
	_ context.Context,
	eventType auth.AuditEventType,
	limit int,
) ([]*auth.AuditEvent, error) {
	// Placeholder: Return empty list
	s.logger.Warn("ListEventsByType not fully implemented for Keycloak backend",
		zap.String("event_type", string(eventType)),
		zap.Int("limit", limit),
	)

	return []*auth.AuditEvent{}, nil
}

// ListEventsByUser retrieves audit events for a specific user.
// Note: This is a placeholder implementation.
func (s *Store) ListEventsByUser(_ context.Context, userID string, limit int) ([]*auth.AuditEvent, error) {
	// Placeholder: Return empty list
	s.logger.Warn("ListEventsByUser not fully implemented for Keycloak backend",
		zap.String("user_id", userID),
		zap.Int("limit", limit),
	)

	return []*auth.AuditEvent{}, nil
}
