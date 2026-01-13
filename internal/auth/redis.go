package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	redis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// sanitizeSubjectKey creates a safe Redis key from a certificate subject.
// This prevents NoSQL injection attacks by hashing the subject to create
// a fixed-length, safe key without special characters.
func sanitizeSubjectKey(subject string) string {
	hash := sha256.Sum256([]byte(subject))
	return hex.EncodeToString(hash[:])
}

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

// Lua script for atomic quota check and increment.
// KEYS[1] = tenant key
// ARGV[1] = usage type (subscriptions, resourcePools, deployments, users)
// Returns: 1 if incremented successfully, 0 if quota exceeded, -1 if tenant not found.
var incrementUsageScript = redis.NewScript(`
local tenantData = redis.call('GET', KEYS[1])
if not tenantData then
    return -1
end

local tenant = cjson.decode(tenantData)
local usageType = ARGV[1]
local usage = tenant.usage or {}
local quota = tenant.quota or {}

local currentUsage = 0
local maxQuota = 0

if usageType == "subscriptions" then
    currentUsage = usage.subscriptions or 0
    maxQuota = quota.maxSubscriptions or 0
elseif usageType == "resourcePools" then
    currentUsage = usage.resourcePools or 0
    maxQuota = quota.maxResourcePools or 0
elseif usageType == "deployments" then
    currentUsage = usage.deployments or 0
    maxQuota = quota.maxDeployments or 0
elseif usageType == "users" then
    currentUsage = usage.users or 0
    maxQuota = quota.maxUsers or 0
else
    return -2
end

if currentUsage >= maxQuota then
    return 0
end

-- Increment usage
if usageType == "subscriptions" then
    tenant.usage.subscriptions = currentUsage + 1
elseif usageType == "resourcePools" then
    tenant.usage.resourcePools = currentUsage + 1
elseif usageType == "deployments" then
    tenant.usage.deployments = currentUsage + 1
elseif usageType == "users" then
    tenant.usage.users = currentUsage + 1
end

-- Update timestamp
tenant.updatedAt = ARGV[2]

redis.call('SET', KEYS[1], cjson.encode(tenant))
return 1
`)

// Lua script for atomic user creation.
// KEYS[1] = user key, KEYS[2] = subject index key, KEYS[3] = tenant user set key
// ARGV[1] = user data JSON, ARGV[2] = user ID
// Returns: 1 if created successfully, 0 if user ID exists, -1 if subject exists.
var createUserScript = redis.NewScript(`
local userKey = KEYS[1]
local subjectKey = KEYS[2]
local tenantSetKey = KEYS[3]
local userData = ARGV[1]
local userID = ARGV[2]

-- Check if user ID already exists
if redis.call('EXISTS', userKey) == 1 then
    return 0
end

-- Check if subject already exists
local existingID = redis.call('GET', subjectKey)
if existingID and existingID ~= '' then
    return -1
end

-- Create user atomically
redis.call('SET', userKey, userData)
redis.call('SET', subjectKey, userID)
redis.call('SADD', tenantSetKey, userID)

return 1
`)

// Lua script for atomic decrement.
// KEYS[1] = tenant key
// ARGV[1] = usage type
// Returns: 1 if decremented successfully, -1 if tenant not found.
var decrementUsageScript = redis.NewScript(`
local tenantData = redis.call('GET', KEYS[1])
if not tenantData then
    return -1
end

local tenant = cjson.decode(tenantData)
local usageType = ARGV[1]
local usage = tenant.usage or {}

if usageType == "subscriptions" then
    if (usage.subscriptions or 0) > 0 then
        tenant.usage.subscriptions = (usage.subscriptions or 0) - 1
    end
elseif usageType == "resourcePools" then
    if (usage.resourcePools or 0) > 0 then
        tenant.usage.resourcePools = (usage.resourcePools or 0) - 1
    end
elseif usageType == "deployments" then
    if (usage.deployments or 0) > 0 then
        tenant.usage.deployments = (usage.deployments or 0) - 1
    end
elseif usageType == "users" then
    if (usage.users or 0) > 0 then
        tenant.usage.users = (usage.users or 0) - 1
    end
else
    return -2
end

-- Update timestamp
tenant.updatedAt = ARGV[2]

redis.call('SET', KEYS[1], cjson.encode(tenant))
return 1
`)

// RedisConfig holds configuration for Redis connection.
type RedisConfig struct {
	// Addr is the Redis server address (host:port) for standalone mode.
	Addr string

	// Password for Redis authentication.
	Password string

	// SentinelPassword for Redis Sentinel authentication.
	// Used in Sentinel mode to authenticate with Sentinel servers.
	// Best practice: Use separate passwords for Sentinel and Redis.
	SentinelPassword string

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
	logger *zap.Logger
}

// NewRedisStore creates a new RedisStore instance.
func NewRedisStore(cfg *RedisConfig) *RedisStore {
	if cfg == nil {
		cfg = DefaultRedisConfig()
	}

	var client redis.UniversalClient

	if cfg.UseSentinel {
		client = redis.NewFailoverClient(&redis.FailoverOptions{
			MasterName:       cfg.MasterName,
			SentinelAddrs:    cfg.SentinelAddrs,
			SentinelPassword: cfg.SentinelPassword,
			Password:         cfg.Password,
			DB:               cfg.DB,
			MaxRetries:       cfg.MaxRetries,
			DialTimeout:      cfg.DialTimeout,
			ReadTimeout:      cfg.ReadTimeout,
			WriteTimeout:     cfg.WriteTimeout,
			PoolSize:         cfg.PoolSize,
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
		logger: zap.L().Named("redis-store"),
	}
}

// NewRedisStoreWithClient creates a new RedisStore with an existing Redis client.
// This is useful for testing or when sharing a Redis client.
func NewRedisStoreWithClient(client redis.UniversalClient) *RedisStore {
	return &RedisStore{
		client: client,
		config: DefaultRedisConfig(),
		logger: zap.L().Named("redis-store"),
	}
}

// Close closes the Redis connection.
func (r *RedisStore) Close() error {
	if err := r.client.Close(); err != nil {
		return fmt.Errorf("failed to close Redis client: %w", err)
	}
	return nil
}

// Ping checks if Redis is available.
func (r *RedisStore) Ping(ctx context.Context) error {
	if err := r.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("%w: %w", ErrStorageUnavailable, err)
	}
	return nil
}

// batchListFromSet is a generic helper for listing entities stored in Redis.
// It retrieves IDs from a set, uses MGET for batch retrieval, and unmarshals JSON results.
func batchListFromSet[T any](
	ctx context.Context,
	client redis.UniversalClient,
	logger *zap.Logger,
	setKey string,
	keyPrefix string,
	entityType string,
	idFieldName string,
) ([]T, error) {
	ids, err := client.SMembers(ctx, setKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to list %s IDs: %w", entityType, err)
	}
	if len(ids) == 0 {
		return []T{}, nil
	}

	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = keyPrefix + id
	}

	results, err := client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to batch get %ss: %w", entityType, err)
	}

	items := make([]T, 0, len(ids))
	for i, result := range results {
		if result == nil {
			logger.Warn(entityType+" data not found during list operation", zap.String(idFieldName, ids[i]))
			continue
		}
		data, ok := result.(string)
		if !ok {
			logger.Warn("unexpected "+entityType+" data type during list operation", zap.String(idFieldName, ids[i]))
			continue
		}
		var item T
		if err := json.Unmarshal([]byte(data), &item); err != nil {
			logger.Warn("failed to unmarshal "+entityType+" during list operation", zap.String(idFieldName, ids[i]), zap.Error(err))
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

// CreateTenant creates a new tenant in Redis.
// Uses SetNX for atomic creation to prevent race conditions.
func (r *RedisStore) CreateTenant(ctx context.Context, tenant *Tenant) error {
	if tenant.ID == "" {
		return ErrInvalidTenantID
	}

	now := time.Now().UTC()
	tenant.CreatedAt = now
	tenant.UpdatedAt = now

	key := tenantKeyPrefix + tenant.ID

	data, err := json.Marshal(tenant)
	if err != nil {
		return fmt.Errorf("failed to marshal tenant: %w", err)
	}

	// Use SetNX for atomic creation - only sets if key doesn't exist.
	wasSet, err := r.client.SetNX(ctx, key, data, 0).Result()
	if err != nil {
		return fmt.Errorf("failed to create tenant: %w", err)
	}
	if !wasSet {
		return ErrTenantExists
	}

	// Add to tenant set (this is idempotent, so safe even if there was a prior failure).
	if err := r.client.SAdd(ctx, tenantSetKey, tenant.ID).Err(); err != nil {
		// Rollback: delete the tenant key if we can't add to set.
		r.client.Del(ctx, key)
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
		if errors.Is(err, redis.Nil) {
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
// Uses MGET for efficient batch retrieval instead of N+1 queries.
func (r *RedisStore) ListTenants(ctx context.Context) ([]*Tenant, error) {
	tenants, err := batchListFromSet[*Tenant](ctx, r.client, r.logger, tenantSetKey, tenantKeyPrefix, "tenant", "tenant_id")
	if err != nil {
		return nil, err
	}
	return tenants, nil
}

// IncrementUsage atomically increments a usage counter.
// Uses a Lua script to ensure atomicity and prevent race conditions.
func (r *RedisStore) IncrementUsage(ctx context.Context, tenantID, usageType string) error {
	if tenantID == "" {
		return ErrInvalidTenantID
	}

	// Validate usage type.
	switch usageType {
	case "subscriptions", "resourcePools", "deployments", "users":
		// Valid usage type.
	default:
		return fmt.Errorf("unknown usage type: %s", usageType)
	}

	key := tenantKeyPrefix + tenantID
	timestamp := time.Now().UTC().Format(time.RFC3339Nano)

	result, err := incrementUsageScript.Run(ctx, r.client, []string{key}, usageType, timestamp).Int()
	if err != nil {
		return fmt.Errorf("failed to increment usage: %w", err)
	}

	switch result {
	case 1:
		return nil
	case 0:
		return ErrQuotaExceeded
	case -1:
		return ErrTenantNotFound
	case -2:
		return fmt.Errorf("unknown usage type: %s", usageType)
	default:
		return fmt.Errorf("unexpected result from increment script: %d", result)
	}
}

// DecrementUsage atomically decrements a usage counter.
// Uses a Lua script to ensure atomicity and prevent race conditions.
func (r *RedisStore) DecrementUsage(ctx context.Context, tenantID, usageType string) error {
	if tenantID == "" {
		return ErrInvalidTenantID
	}

	// Validate usage type.
	switch usageType {
	case "subscriptions", "resourcePools", "deployments", "users":
		// Valid usage type.
	default:
		return fmt.Errorf("unknown usage type: %s", usageType)
	}

	key := tenantKeyPrefix + tenantID
	timestamp := time.Now().UTC().Format(time.RFC3339Nano)

	result, err := decrementUsageScript.Run(ctx, r.client, []string{key}, usageType, timestamp).Int()
	if err != nil {
		return fmt.Errorf("failed to decrement usage: %w", err)
	}

	switch result {
	case 1:
		return nil
	case -1:
		return ErrTenantNotFound
	case -2:
		return fmt.Errorf("unknown usage type: %s", usageType)
	default:
		return fmt.Errorf("unexpected result from decrement script: %d", result)
	}
}

// CreateUser creates a new user.
// Uses a Lua script for atomic creation to prevent race conditions.
func (r *RedisStore) CreateUser(ctx context.Context, user *TenantUser) error {
	if user.ID == "" {
		return ErrInvalidUserID
	}

	now := time.Now().UTC()
	user.CreatedAt = now
	user.UpdatedAt = now

	data, err := json.Marshal(user)
	if err != nil {
		return fmt.Errorf("failed to marshal user: %w", err)
	}

	userKey := userKeyPrefix + user.ID
	subjectKey := userSubjectIndex + sanitizeSubjectKey(user.Subject)
	tenantSetKey := userTenantIndex + user.TenantID

	result, err := createUserScript.Run(ctx, r.client,
		[]string{userKey, subjectKey, tenantSetKey},
		string(data), user.ID,
	).Int()
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	switch result {
	case 1:
		return nil
	case 0:
		return ErrUserExists
	case -1:
		return ErrUserExists // Subject already exists
	default:
		return fmt.Errorf("unexpected result from create user script: %d", result)
	}
}

// GetUser retrieves a user by ID.
func (r *RedisStore) GetUser(ctx context.Context, id string) (*TenantUser, error) {
	if id == "" {
		return nil, ErrInvalidUserID
	}

	key := userKeyPrefix + id
	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
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

	subjectKey := userSubjectIndex + sanitizeSubjectKey(subject)
	userID, err := r.client.Get(ctx, subjectKey).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
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
		pipe.Del(ctx, userSubjectIndex+sanitizeSubjectKey(existing.Subject))
		pipe.Set(ctx, userSubjectIndex+sanitizeSubjectKey(user.Subject), user.ID, 0)
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
	pipe.Del(ctx, userSubjectIndex+sanitizeSubjectKey(user.Subject))
	pipe.SRem(ctx, userTenantIndex+user.TenantID, id)

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	return nil
}

// ListUsersByTenant retrieves all users for a tenant.
// Uses MGET for efficient batch retrieval instead of N+1 queries.
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

	// Build keys for batch retrieval.
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = userKeyPrefix + id
	}

	// Use MGET for efficient batch retrieval.
	results, err := r.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to batch get users: %w", err)
	}

	users := make([]*TenantUser, 0, len(ids))
	for i, result := range results {
		if result == nil {
			r.logger.Warn("user data not found during list operation",
				zap.String("user_id", ids[i]),
				zap.String("tenant_id", tenantID),
			)
			continue
		}

		data, ok := result.(string)
		if !ok {
			r.logger.Warn("unexpected user data type during list operation",
				zap.String("user_id", ids[i]),
				zap.String("tenant_id", tenantID),
			)
			continue
		}

		var user TenantUser
		if err := json.Unmarshal([]byte(data), &user); err != nil {
			r.logger.Warn("failed to unmarshal user during list operation",
				zap.String("user_id", ids[i]),
				zap.String("tenant_id", tenantID),
				zap.Error(err),
			)
			continue
		}

		users = append(users, &user)
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
// Uses SetNX for atomic creation to prevent race conditions.
func (r *RedisStore) CreateRole(ctx context.Context, role *Role) error {
	if role.ID == "" {
		return ErrInvalidRoleID
	}

	now := time.Now().UTC()
	role.CreatedAt = now
	role.UpdatedAt = now

	key := roleKeyPrefix + role.ID

	data, err := json.Marshal(role)
	if err != nil {
		return fmt.Errorf("failed to marshal role: %w", err)
	}

	// Use SetNX for atomic creation - only sets if key doesn't exist.
	wasSet, err := r.client.SetNX(ctx, key, data, 0).Result()
	if err != nil {
		return fmt.Errorf("failed to create role: %w", err)
	}
	if !wasSet {
		return ErrRoleExists
	}

	// Add to indices (these are idempotent operations).
	pipe := r.client.TxPipeline()
	pipe.SAdd(ctx, roleSetKey, role.ID)
	pipe.Set(ctx, roleNameIndex+string(role.Name), role.ID, 0)

	if role.TenantID != "" {
		pipe.SAdd(ctx, roleTenantIndex+role.TenantID, role.ID)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		// Rollback: delete the role key if we can't add indices.
		r.client.Del(ctx, key)
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
		if errors.Is(err, redis.Nil) {
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
		if errors.Is(err, redis.Nil) {
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
// Uses MGET for efficient batch retrieval instead of N+1 queries.
func (r *RedisStore) ListRoles(ctx context.Context) ([]*Role, error) {
	roles, err := batchListFromSet[*Role](ctx, r.client, r.logger, roleSetKey, roleKeyPrefix, "role", "role_id")
	if err != nil {
		return nil, err
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
		if err != nil && !errors.Is(err, ErrRoleExists) {
			return fmt.Errorf("failed to create default role %s: %w", role.Name, err)
		}
	}
	return nil
}

// LogEvent creates a new audit event.
// Uses sorted sets with timestamp scores for consistent TTL behavior.
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
	score := float64(event.Timestamp.UnixNano())
	// Calculate expiration cutoff for cleanup (events older than TTL).
	expirationCutoff := float64(time.Now().Add(-auditEventTTL).UnixNano())

	pipe := r.client.TxPipeline()

	// Store the event with TTL.
	pipe.Set(ctx, key, data, auditEventTTL)

	// Use sorted sets with timestamp scores for better time-based queries.
	pipe.ZAdd(ctx, auditListKey, redis.Z{Score: score, Member: event.ID})

	// Cleanup old entries from sorted sets (older than TTL).
	pipe.ZRemRangeByScore(ctx, auditListKey, "-inf", fmt.Sprintf("%f", expirationCutoff))

	if event.TenantID != "" {
		pipe.ZAdd(ctx, auditTenantIndex+event.TenantID, redis.Z{Score: score, Member: event.ID})
		pipe.ZRemRangeByScore(ctx, auditTenantIndex+event.TenantID, "-inf", fmt.Sprintf("%f", expirationCutoff))
	}

	if event.UserID != "" {
		pipe.ZAdd(ctx, auditUserIndex+event.UserID, redis.Z{Score: score, Member: event.ID})
		pipe.ZRemRangeByScore(ctx, auditUserIndex+event.UserID, "-inf", fmt.Sprintf("%f", expirationCutoff))
	}

	pipe.ZAdd(ctx, auditTypeIndex+string(event.Type), redis.Z{Score: score, Member: event.ID})
	pipe.ZRemRangeByScore(ctx, auditTypeIndex+string(event.Type), "-inf", fmt.Sprintf("%f", expirationCutoff))

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to log audit event: %w", err)
	}

	return nil
}

// ListEvents retrieves audit events with optional filtering.
// Uses ZREVRANGE on sorted sets for most recent events first.
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

	// Use ZREVRANGE to get most recent events first (highest scores).
	ids, err := r.client.ZRevRange(ctx, listKey, int64(offset), int64(offset+limit-1)).Result()
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
			// Log at debug level since event expiration is expected behavior.
			r.logger.Debug("skipping audit event (likely expired)",
				zap.String("event_id", id),
				zap.Error(err),
			)
			continue
		}
		events = append(events, event)
	}

	return events, nil
}

// ListEventsByType retrieves audit events of a specific type.
// Uses ZREVRANGE on sorted sets for most recent events first.
func (r *RedisStore) ListEventsByType(ctx context.Context, eventType AuditEventType, limit int) ([]*AuditEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}

	listKey := auditTypeIndex + string(eventType)
	ids, err := r.client.ZRevRange(ctx, listKey, 0, int64(limit-1)).Result()
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
			// Log at debug level since event expiration is expected behavior.
			r.logger.Debug("skipping audit event (likely expired)",
				zap.String("event_id", id),
				zap.String("event_type", string(eventType)),
				zap.Error(err),
			)
			continue
		}
		events = append(events, event)
	}

	return events, nil
}

// ListEventsByUser retrieves audit events for a specific user.
// Uses ZREVRANGE on sorted sets for most recent events first.
func (r *RedisStore) ListEventsByUser(ctx context.Context, userID string, limit int) ([]*AuditEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}

	listKey := auditUserIndex + userID
	ids, err := r.client.ZRevRange(ctx, listKey, 0, int64(limit-1)).Result()
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
			// Log at debug level since event expiration is expected behavior.
			r.logger.Debug("skipping audit event (likely expired)",
				zap.String("event_id", id),
				zap.String("user_id", userID),
				zap.Error(err),
			)
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
		return nil, fmt.Errorf("failed to get audit event from Redis: %w", err)
	}

	var event AuditEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, fmt.Errorf("failed to unmarshal audit event: %w", err)
	}

	return &event, nil
}
