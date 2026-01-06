package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// Redis key prefixes for auth data.
	tenantKeyPrefix  = "tenant:"
	tenantSetKey     = "tenants:active"
	userKeyPrefix    = "user:"
	userSubjectIndex = "users:subject:"
	userTenantIndex  = "users:tenant:"
	roleKeyPrefix    = "role:"
	roleSetKey       = "roles:all"
	roleTenantIndex  = "roles:tenant:"
	roleNameIndex    = "roles:name:"
	auditKeyPrefix   = "audit:"
	auditListKey     = "audit:events"
	auditTenantIndex = "audit:tenant:"
	auditUserIndex   = "audit:user:"
	auditTypeIndex   = "audit:type:"
	usageKeyPrefix   = "usage:"

	// Default TTL for audit events (30 days).
	auditEventTTL = 30 * 24 * time.Hour
)

// RedisConfig holds configuration for Redis connection.
type RedisConfig struct {
	// Addr is the Redis server address (host:port) for standalone mode.
	Addr string

	// Password for Redis authentication.
	Password string

	// DB is the Redis database number (0-15).
	DB int

	// UseSentinel enables Redis Sentinel mode for high availability.
	UseSentinel bool

	// SentinelAddrs is the list of Sentinel server addresses.
	SentinelAddrs []string

	// MasterName is the name of the Redis master in Sentinel mode.
	MasterName string

	// MaxRetries is the maximum number of retries for failed commands.
	MaxRetries int

	// DialTimeout is the timeout for establishing connections.
	DialTimeout time.Duration

	// ReadTimeout is the timeout for socket reads.
	ReadTimeout time.Duration

	// WriteTimeout is the timeout for socket writes.
	WriteTimeout time.Duration

	// PoolSize is the maximum number of socket connections.
	PoolSize int
}

// DefaultRedisConfig returns a RedisConfig with sensible defaults.
func DefaultRedisConfig() *RedisConfig {
	return &RedisConfig{
		Addr:         "localhost:6379",
		Password:     "",
		DB:           0,
		UseSentinel:  false,
		MaxRetries:   3,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     10,
	}
}

// RedisStore implements the Store interface using Redis as the backend.
type RedisStore struct {
	client redis.UniversalClient
	config *RedisConfig
}

// NewRedisStore creates a new RedisStore instance.
func NewRedisStore(cfg *RedisConfig) *RedisStore {
	if cfg == nil {
		cfg = DefaultRedisConfig()
	}

	var client redis.UniversalClient

	if cfg.UseSentinel {
		client = redis.NewFailoverClient(&redis.FailoverOptions{
			MasterName:    cfg.MasterName,
			SentinelAddrs: cfg.SentinelAddrs,
			Password:      cfg.Password,
			DB:            cfg.DB,
			MaxRetries:    cfg.MaxRetries,
			DialTimeout:   cfg.DialTimeout,
			ReadTimeout:   cfg.ReadTimeout,
			WriteTimeout:  cfg.WriteTimeout,
			PoolSize:      cfg.PoolSize,
		})
	} else {
		client = redis.NewClient(&redis.Options{
			Addr:         cfg.Addr,
			Password:     cfg.Password,
			DB:           cfg.DB,
			MaxRetries:   cfg.MaxRetries,
			DialTimeout:  cfg.DialTimeout,
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
			PoolSize:     cfg.PoolSize,
		})
	}

	return &RedisStore{
		client: client,
		config: cfg,
	}
}

// NewRedisStoreWithClient creates a new RedisStore with an existing Redis client.
// This is useful for testing or when sharing a Redis client.
func NewRedisStoreWithClient(client redis.UniversalClient) *RedisStore {
	return &RedisStore{
		client: client,
		config: DefaultRedisConfig(),
	}
}

// Close closes the Redis connection.
func (r *RedisStore) Close() error {
	return r.client.Close()
}

// Ping checks if Redis is available.
func (r *RedisStore) Ping(ctx context.Context) error {
	if err := r.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("%w: %v", ErrStorageUnavailable, err)
	}
	return nil
}

// CreateTenant creates a new tenant in Redis.
func (r *RedisStore) CreateTenant(ctx context.Context, tenant *Tenant) error {
	if tenant.ID == "" {
		return ErrInvalidTenantID
	}

	now := time.Now().UTC()
	tenant.CreatedAt = now
	tenant.UpdatedAt = now

	key := tenantKeyPrefix + tenant.ID

	// Check if tenant already exists.
	exists, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("failed to check tenant existence: %w", err)
	}
	if exists > 0 {
		return ErrTenantExists
	}

	data, err := json.Marshal(tenant)
	if err != nil {
		return fmt.Errorf("failed to marshal tenant: %w", err)
	}

	pipe := r.client.Pipeline()
	pipe.Set(ctx, key, data, 0)
	pipe.SAdd(ctx, tenantSetKey, tenant.ID)

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to create tenant: %w", err)
	}

	return nil
}

// GetTenant retrieves a tenant by ID.
func (r *RedisStore) GetTenant(ctx context.Context, id string) (*Tenant, error) {
	if id == "" {
		return nil, ErrInvalidTenantID
	}

	key := tenantKeyPrefix + id
	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, ErrTenantNotFound
		}
		return nil, fmt.Errorf("failed to get tenant: %w", err)
	}

	var tenant Tenant
	if err := json.Unmarshal(data, &tenant); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tenant: %w", err)
	}

	return &tenant, nil
}

// UpdateTenant updates an existing tenant.
func (r *RedisStore) UpdateTenant(ctx context.Context, tenant *Tenant) error {
	if tenant.ID == "" {
		return ErrInvalidTenantID
	}

	key := tenantKeyPrefix + tenant.ID

	exists, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("failed to check tenant existence: %w", err)
	}
	if exists == 0 {
		return ErrTenantNotFound
	}

	existing, err := r.GetTenant(ctx, tenant.ID)
	if err != nil {
		return err
	}

	tenant.CreatedAt = existing.CreatedAt
	tenant.UpdatedAt = time.Now().UTC()

	data, err := json.Marshal(tenant)
	if err != nil {
		return fmt.Errorf("failed to marshal tenant: %w", err)
	}

	if err := r.client.Set(ctx, key, data, 0).Err(); err != nil {
		return fmt.Errorf("failed to update tenant: %w", err)
	}

	return nil
}

// DeleteTenant deletes a tenant by ID.
func (r *RedisStore) DeleteTenant(ctx context.Context, id string) error {
	if id == "" {
		return ErrInvalidTenantID
	}

	key := tenantKeyPrefix + id

	exists, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("failed to check tenant existence: %w", err)
	}
	if exists == 0 {
		return ErrTenantNotFound
	}

	pipe := r.client.Pipeline()
	pipe.Del(ctx, key)
	pipe.SRem(ctx, tenantSetKey, id)
	pipe.Del(ctx, usageKeyPrefix+id)

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to delete tenant: %w", err)
	}

	return nil
}

// ListTenants retrieves all tenants.
func (r *RedisStore) ListTenants(ctx context.Context) ([]*Tenant, error) {
	ids, err := r.client.SMembers(ctx, tenantSetKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to list tenant IDs: %w", err)
	}

	if len(ids) == 0 {
		return []*Tenant{}, nil
	}

	tenants := make([]*Tenant, 0, len(ids))
	for _, id := range ids {
		tenant, err := r.GetTenant(ctx, id)
		if err != nil {
			continue
		}
		tenants = append(tenants, tenant)
	}

	return tenants, nil
}

// IncrementUsage atomically increments a usage counter.
func (r *RedisStore) IncrementUsage(ctx context.Context, tenantID, usageType string) error {
	if tenantID == "" {
		return ErrInvalidTenantID
	}

	tenant, err := r.GetTenant(ctx, tenantID)
	if err != nil {
		return err
	}

	// Check quota before incrementing.
	switch usageType {
	case "subscriptions":
		if tenant.Usage.Subscriptions >= tenant.Quota.MaxSubscriptions {
			return ErrQuotaExceeded
		}
		tenant.Usage.Subscriptions++
	case "resourcePools":
		if tenant.Usage.ResourcePools >= tenant.Quota.MaxResourcePools {
			return ErrQuotaExceeded
		}
		tenant.Usage.ResourcePools++
	case "deployments":
		if tenant.Usage.Deployments >= tenant.Quota.MaxDeployments {
			return ErrQuotaExceeded
		}
		tenant.Usage.Deployments++
	case "users":
		if tenant.Usage.Users >= tenant.Quota.MaxUsers {
			return ErrQuotaExceeded
		}
		tenant.Usage.Users++
	default:
		return fmt.Errorf("unknown usage type: %s", usageType)
	}

	return r.UpdateTenant(ctx, tenant)
}

// DecrementUsage atomically decrements a usage counter.
func (r *RedisStore) DecrementUsage(ctx context.Context, tenantID, usageType string) error {
	if tenantID == "" {
		return ErrInvalidTenantID
	}

	tenant, err := r.GetTenant(ctx, tenantID)
	if err != nil {
		return err
	}

	switch usageType {
	case "subscriptions":
		if tenant.Usage.Subscriptions > 0 {
			tenant.Usage.Subscriptions--
		}
	case "resourcePools":
		if tenant.Usage.ResourcePools > 0 {
			tenant.Usage.ResourcePools--
		}
	case "deployments":
		if tenant.Usage.Deployments > 0 {
			tenant.Usage.Deployments--
		}
	case "users":
		if tenant.Usage.Users > 0 {
			tenant.Usage.Users--
		}
	default:
		return fmt.Errorf("unknown usage type: %s", usageType)
	}

	return r.UpdateTenant(ctx, tenant)
}

// CreateUser creates a new user.
func (r *RedisStore) CreateUser(ctx context.Context, user *TenantUser) error {
	if user.ID == "" {
		return ErrInvalidUserID
	}

	now := time.Now().UTC()
	user.CreatedAt = now
	user.UpdatedAt = now

	key := userKeyPrefix + user.ID

	exists, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("failed to check user existence: %w", err)
	}
	if exists > 0 {
		return ErrUserExists
	}

	// Check if subject is already registered.
	subjectKey := userSubjectIndex + user.Subject
	existingID, err := r.client.Get(ctx, subjectKey).Result()
	if err == nil && existingID != "" {
		return ErrUserExists
	}

	data, err := json.Marshal(user)
	if err != nil {
		return fmt.Errorf("failed to marshal user: %w", err)
	}

	pipe := r.client.Pipeline()
	pipe.Set(ctx, key, data, 0)
	pipe.Set(ctx, subjectKey, user.ID, 0)
	pipe.SAdd(ctx, userTenantIndex+user.TenantID, user.ID)

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}

// GetUser retrieves a user by ID.
func (r *RedisStore) GetUser(ctx context.Context, id string) (*TenantUser, error) {
	if id == "" {
		return nil, ErrInvalidUserID
	}

	key := userKeyPrefix + id
	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	var user TenantUser
	if err := json.Unmarshal(data, &user); err != nil {
		return nil, fmt.Errorf("failed to unmarshal user: %w", err)
	}

	return &user, nil
}

// GetUserBySubject retrieves a user by certificate subject.
func (r *RedisStore) GetUserBySubject(ctx context.Context, subject string) (*TenantUser, error) {
	if subject == "" {
		return nil, ErrUserNotFound
	}

	subjectKey := userSubjectIndex + subject
	userID, err := r.client.Get(ctx, subjectKey).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user by subject: %w", err)
	}

	return r.GetUser(ctx, userID)
}

// UpdateUser updates an existing user.
func (r *RedisStore) UpdateUser(ctx context.Context, user *TenantUser) error {
	if user.ID == "" {
		return ErrInvalidUserID
	}

	key := userKeyPrefix + user.ID

	exists, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("failed to check user existence: %w", err)
	}
	if exists == 0 {
		return ErrUserNotFound
	}

	existing, err := r.GetUser(ctx, user.ID)
	if err != nil {
		return err
	}

	user.CreatedAt = existing.CreatedAt
	user.UpdatedAt = time.Now().UTC()

	data, err := json.Marshal(user)
	if err != nil {
		return fmt.Errorf("failed to marshal user: %w", err)
	}

	pipe := r.client.Pipeline()
	pipe.Set(ctx, key, data, 0)

	// Update subject index if changed.
	if existing.Subject != user.Subject {
		pipe.Del(ctx, userSubjectIndex+existing.Subject)
		pipe.Set(ctx, userSubjectIndex+user.Subject, user.ID, 0)
	}

	// Update tenant index if changed.
	if existing.TenantID != user.TenantID {
		pipe.SRem(ctx, userTenantIndex+existing.TenantID, user.ID)
		pipe.SAdd(ctx, userTenantIndex+user.TenantID, user.ID)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	return nil
}

// DeleteUser deletes a user by ID.
func (r *RedisStore) DeleteUser(ctx context.Context, id string) error {
	if id == "" {
		return ErrInvalidUserID
	}

	user, err := r.GetUser(ctx, id)
	if err != nil {
		return err
	}

	pipe := r.client.Pipeline()
	pipe.Del(ctx, userKeyPrefix+id)
	pipe.Del(ctx, userSubjectIndex+user.Subject)
	pipe.SRem(ctx, userTenantIndex+user.TenantID, id)

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	return nil
}

// ListUsersByTenant retrieves all users for a tenant.
func (r *RedisStore) ListUsersByTenant(ctx context.Context, tenantID string) ([]*TenantUser, error) {
	if tenantID == "" {
		return []*TenantUser{}, nil
	}

	ids, err := r.client.SMembers(ctx, userTenantIndex+tenantID).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to list user IDs: %w", err)
	}

	if len(ids) == 0 {
		return []*TenantUser{}, nil
	}

	users := make([]*TenantUser, 0, len(ids))
	for _, id := range ids {
		user, err := r.GetUser(ctx, id)
		if err != nil {
			continue
		}
		users = append(users, user)
	}

	return users, nil
}

// UpdateLastLogin updates the last login timestamp.
func (r *RedisStore) UpdateLastLogin(ctx context.Context, userID string) error {
	user, err := r.GetUser(ctx, userID)
	if err != nil {
		return err
	}

	user.LastLoginAt = time.Now().UTC()
	return r.UpdateUser(ctx, user)
}

// CreateRole creates a new role.
func (r *RedisStore) CreateRole(ctx context.Context, role *Role) error {
	if role.ID == "" {
		return ErrInvalidRoleID
	}

	now := time.Now().UTC()
	role.CreatedAt = now
	role.UpdatedAt = now

	key := roleKeyPrefix + role.ID

	exists, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("failed to check role existence: %w", err)
	}
	if exists > 0 {
		return ErrRoleExists
	}

	data, err := json.Marshal(role)
	if err != nil {
		return fmt.Errorf("failed to marshal role: %w", err)
	}

	pipe := r.client.Pipeline()
	pipe.Set(ctx, key, data, 0)
	pipe.SAdd(ctx, roleSetKey, role.ID)
	pipe.Set(ctx, roleNameIndex+string(role.Name), role.ID, 0)

	if role.TenantID != "" {
		pipe.SAdd(ctx, roleTenantIndex+role.TenantID, role.ID)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to create role: %w", err)
	}

	return nil
}

// GetRole retrieves a role by ID.
func (r *RedisStore) GetRole(ctx context.Context, id string) (*Role, error) {
	if id == "" {
		return nil, ErrInvalidRoleID
	}

	key := roleKeyPrefix + id
	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, ErrRoleNotFound
		}
		return nil, fmt.Errorf("failed to get role: %w", err)
	}

	var role Role
	if err := json.Unmarshal(data, &role); err != nil {
		return nil, fmt.Errorf("failed to unmarshal role: %w", err)
	}

	return &role, nil
}

// GetRoleByName retrieves a role by name.
func (r *RedisStore) GetRoleByName(ctx context.Context, name RoleName) (*Role, error) {
	nameKey := roleNameIndex + string(name)
	roleID, err := r.client.Get(ctx, nameKey).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, ErrRoleNotFound
		}
		return nil, fmt.Errorf("failed to get role by name: %w", err)
	}

	return r.GetRole(ctx, roleID)
}

// UpdateRole updates an existing role.
func (r *RedisStore) UpdateRole(ctx context.Context, role *Role) error {
	if role.ID == "" {
		return ErrInvalidRoleID
	}

	key := roleKeyPrefix + role.ID

	exists, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("failed to check role existence: %w", err)
	}
	if exists == 0 {
		return ErrRoleNotFound
	}

	existing, err := r.GetRole(ctx, role.ID)
	if err != nil {
		return err
	}

	role.CreatedAt = existing.CreatedAt
	role.UpdatedAt = time.Now().UTC()

	data, err := json.Marshal(role)
	if err != nil {
		return fmt.Errorf("failed to marshal role: %w", err)
	}

	pipe := r.client.Pipeline()
	pipe.Set(ctx, key, data, 0)

	// Update name index if changed.
	if existing.Name != role.Name {
		pipe.Del(ctx, roleNameIndex+string(existing.Name))
		pipe.Set(ctx, roleNameIndex+string(role.Name), role.ID, 0)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to update role: %w", err)
	}

	return nil
}

// DeleteRole deletes a role by ID.
func (r *RedisStore) DeleteRole(ctx context.Context, id string) error {
	if id == "" {
		return ErrInvalidRoleID
	}

	role, err := r.GetRole(ctx, id)
	if err != nil {
		return err
	}

	pipe := r.client.Pipeline()
	pipe.Del(ctx, roleKeyPrefix+id)
	pipe.SRem(ctx, roleSetKey, id)
	pipe.Del(ctx, roleNameIndex+string(role.Name))

	if role.TenantID != "" {
		pipe.SRem(ctx, roleTenantIndex+role.TenantID, id)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to delete role: %w", err)
	}

	return nil
}

// ListRoles retrieves all roles.
func (r *RedisStore) ListRoles(ctx context.Context) ([]*Role, error) {
	ids, err := r.client.SMembers(ctx, roleSetKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to list role IDs: %w", err)
	}

	if len(ids) == 0 {
		return []*Role{}, nil
	}

	roles := make([]*Role, 0, len(ids))
	for _, id := range ids {
		role, err := r.GetRole(ctx, id)
		if err != nil {
			continue
		}
		roles = append(roles, role)
	}

	return roles, nil
}

// ListRolesByTenant retrieves roles for a specific tenant.
func (r *RedisStore) ListRolesByTenant(ctx context.Context, tenantID string) ([]*Role, error) {
	// Get all roles (includes global roles).
	allRoles, err := r.ListRoles(ctx)
	if err != nil {
		return nil, err
	}

	// Filter to include global roles and tenant-specific roles.
	roles := make([]*Role, 0)
	for _, role := range allRoles {
		if role.TenantID == "" || role.TenantID == tenantID {
			roles = append(roles, role)
		}
	}

	return roles, nil
}

// InitializeDefaultRoles creates the default system roles if they don't exist.
func (r *RedisStore) InitializeDefaultRoles(ctx context.Context) error {
	defaultRoles := GetDefaultRoles()
	for _, role := range defaultRoles {
		err := r.CreateRole(ctx, role)
		if err != nil && err != ErrRoleExists {
			return fmt.Errorf("failed to create default role %s: %w", role.Name, err)
		}
	}
	return nil
}

// LogEvent creates a new audit event.
func (r *RedisStore) LogEvent(ctx context.Context, event *AuditEvent) error {
	if event.ID == "" {
		return fmt.Errorf("event ID is required")
	}

	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal audit event: %w", err)
	}

	key := auditKeyPrefix + event.ID

	pipe := r.client.Pipeline()
	pipe.Set(ctx, key, data, auditEventTTL)
	pipe.LPush(ctx, auditListKey, event.ID)
	pipe.LTrim(ctx, auditListKey, 0, 9999) // Keep last 10000 events.

	if event.TenantID != "" {
		pipe.LPush(ctx, auditTenantIndex+event.TenantID, event.ID)
		pipe.LTrim(ctx, auditTenantIndex+event.TenantID, 0, 999)
	}

	if event.UserID != "" {
		pipe.LPush(ctx, auditUserIndex+event.UserID, event.ID)
		pipe.LTrim(ctx, auditUserIndex+event.UserID, 0, 999)
	}

	pipe.LPush(ctx, auditTypeIndex+string(event.Type), event.ID)
	pipe.LTrim(ctx, auditTypeIndex+string(event.Type), 0, 999)

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to log audit event: %w", err)
	}

	return nil
}

// ListEvents retrieves audit events with optional filtering.
func (r *RedisStore) ListEvents(ctx context.Context, tenantID string, limit, offset int) ([]*AuditEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}

	var listKey string
	if tenantID != "" {
		listKey = auditTenantIndex + tenantID
	} else {
		listKey = auditListKey
	}

	ids, err := r.client.LRange(ctx, listKey, int64(offset), int64(offset+limit-1)).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to list audit event IDs: %w", err)
	}

	if len(ids) == 0 {
		return []*AuditEvent{}, nil
	}

	events := make([]*AuditEvent, 0, len(ids))
	for _, id := range ids {
		event, err := r.getAuditEvent(ctx, id)
		if err != nil {
			continue
		}
		events = append(events, event)
	}

	return events, nil
}

// ListEventsByType retrieves audit events of a specific type.
func (r *RedisStore) ListEventsByType(ctx context.Context, eventType AuditEventType, limit int) ([]*AuditEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}

	listKey := auditTypeIndex + string(eventType)
	ids, err := r.client.LRange(ctx, listKey, 0, int64(limit-1)).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to list audit events by type: %w", err)
	}

	if len(ids) == 0 {
		return []*AuditEvent{}, nil
	}

	events := make([]*AuditEvent, 0, len(ids))
	for _, id := range ids {
		event, err := r.getAuditEvent(ctx, id)
		if err != nil {
			continue
		}
		events = append(events, event)
	}

	return events, nil
}

// ListEventsByUser retrieves audit events for a specific user.
func (r *RedisStore) ListEventsByUser(ctx context.Context, userID string, limit int) ([]*AuditEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}

	listKey := auditUserIndex + userID
	ids, err := r.client.LRange(ctx, listKey, 0, int64(limit-1)).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to list audit events by user: %w", err)
	}

	if len(ids) == 0 {
		return []*AuditEvent{}, nil
	}

	events := make([]*AuditEvent, 0, len(ids))
	for _, id := range ids {
		event, err := r.getAuditEvent(ctx, id)
		if err != nil {
			continue
		}
		events = append(events, event)
	}

	return events, nil
}

// getAuditEvent retrieves an audit event by ID.
func (r *RedisStore) getAuditEvent(ctx context.Context, id string) (*AuditEvent, error) {
	key := auditKeyPrefix + id
	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		return nil, err
	}

	var event AuditEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, err
	}

	return &event, nil
}
