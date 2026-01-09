package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// Redis key prefixes.
	subscriptionKeyPrefix         = "subscription:"
	subscriptionSetKey            = "subscriptions:active"
	subscriptionPoolIndexPrefix   = "subscriptions:pool:"
	subscriptionTypeIndexPrefix   = "subscriptions:type:"
	subscriptionTenantIndexPrefix = "subscriptions:tenant:"
	subscriptionEventChannel      = "subscriptions:events"

	// Default TTL for subscription keys (0 = no expiration).
	subscriptionTTL = 0
)

// RedisConfig holds configuration for Redis connection.
type RedisConfig struct {
	// Addr is the Redis server address (host:port) for standalone mode.
	// Ignored if UseSentinel is true.
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
	// Required if UseSentinel is true.
	SentinelAddrs []string

	// MasterName is the name of the Redis master in Sentinel mode.
	// Required if UseSentinel is true.
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

	// AllowInsecureCallbacks allows HTTP (non-HTTPS) webhook callbacks.
	// This should ONLY be enabled in development/testing environments.
	// Production deployments MUST enforce HTTPS for webhook callbacks to prevent
	// man-in-the-middle attacks and ensure data confidentiality.
	AllowInsecureCallbacks bool
}

// DefaultRedisConfig returns a RedisConfig with sensible defaults.
func DefaultRedisConfig() *RedisConfig {
	return &RedisConfig{
		Addr:                   "localhost:6379",
		Password:               "",
		DB:                     0,
		UseSentinel:            false,
		MaxRetries:             3,
		DialTimeout:            5 * time.Second,
		ReadTimeout:            3 * time.Second,
		WriteTimeout:           3 * time.Second,
		PoolSize:               10,
		AllowInsecureCallbacks: false, // Enforce HTTPS by default
	}
}

// RedisStore implements the Store interface using Redis as the backend.
// It supports both standalone Redis and Redis Sentinel for high availability.
//
// Data Model:
//   - subscription:<id> (hash) - Subscription data
//   - subscriptions:active (set) - Set of active subscription IDs
//   - subscriptions:pool:<poolID> (set) - Index by resource pool ID
//   - subscriptions:type:<typeID> (set) - Index by resource type ID
//
// Example:
//
//	cfg := DefaultRedisConfig()
//	cfg.Addr = "redis.example.com:6379"
//	store := NewRedisStore(cfg)
//	defer store.Close()
//
//	sub := &Subscription{
//	    ID:       uuid.New().String(),
//	    Callback: "https://smo.example.com/notify",
//	}
//	err := store.Create(ctx, sub)
type RedisStore struct {
	client redis.UniversalClient
	config *RedisConfig
}

// NewRedisStore creates a new RedisStore instance.
// It automatically configures Redis Sentinel if enabled in the config.
func NewRedisStore(cfg *RedisConfig) *RedisStore {
	if cfg == nil {
		cfg = DefaultRedisConfig()
	}

	var client redis.UniversalClient

	if cfg.UseSentinel {
		// Redis Sentinel mode for HA
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
		// Standalone Redis mode
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

// Create creates a new subscription in Redis.
// Returns ErrSubscriptionExists if a subscription with the same ID already exists.
// Returns ErrInvalidCallback if the callback URL is invalid.
// Returns ErrInvalidID if the subscription ID is empty.
func (r *RedisStore) Create(ctx context.Context, sub *Subscription) error {
	// Validate input
	if sub.ID == "" {
		return ErrInvalidID
	}
	if err := r.validateCallbackURL(sub.Callback); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidCallback, err)
	}

	// Set timestamps
	now := time.Now().UTC()
	sub.CreatedAt = now
	sub.UpdatedAt = now

	key := subscriptionKeyPrefix + sub.ID

	// Check if subscription already exists
	exists, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("failed to check subscription existence: %w", err)
	}
	if exists > 0 {
		return ErrSubscriptionExists
	}

	// Serialize subscription
	data, err := json.Marshal(sub)
	if err != nil {
		return fmt.Errorf("failed to marshal subscription: %w", err)
	}

	// Use pipeline for atomic operations
	pipe := r.client.Pipeline()

	// Store subscription data
	pipe.Set(ctx, key, data, subscriptionTTL)

	// Add to active subscriptions set
	pipe.SAdd(ctx, subscriptionSetKey, sub.ID)

	// Add to resource pool index if filter specified
	if sub.Filter.ResourcePoolID != "" {
		poolKey := subscriptionPoolIndexPrefix + sub.Filter.ResourcePoolID
		pipe.SAdd(ctx, poolKey, sub.ID)
	}

	// Add to resource type index if filter specified
	if sub.Filter.ResourceTypeID != "" {
		typeKey := subscriptionTypeIndexPrefix + sub.Filter.ResourceTypeID
		pipe.SAdd(ctx, typeKey, sub.ID)
	}

	// Add to tenant index if tenant ID specified (multi-tenancy)
	if sub.TenantID != "" {
		tenantKey := subscriptionTenantIndexPrefix + sub.TenantID
		pipe.SAdd(ctx, tenantKey, sub.ID)
	}

	// Publish subscription created event
	eventData := map[string]interface{}{
		"event": "created",
		"id":    sub.ID,
	}
	eventJSON, _ := json.Marshal(eventData)
	pipe.Publish(ctx, subscriptionEventChannel, eventJSON)

	// Execute pipeline
	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to create subscription: %w", err)
	}

	return nil
}

// Get retrieves a subscription by ID.
// Returns ErrSubscriptionNotFound if the subscription does not exist.
func (r *RedisStore) Get(ctx context.Context, id string) (*Subscription, error) {
	if id == "" {
		return nil, ErrInvalidID
	}

	key := subscriptionKeyPrefix + id

	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrSubscriptionNotFound
		}
		return nil, fmt.Errorf("failed to get subscription: %w", err)
	}

	var sub Subscription
	if err := json.Unmarshal(data, &sub); err != nil {
		return nil, fmt.Errorf("failed to unmarshal subscription: %w", err)
	}

	return &sub, nil
}

// Update updates an existing subscription.
// Returns ErrSubscriptionNotFound if the subscription does not exist.
// Returns ErrInvalidCallback if the callback URL is invalid.
func (r *RedisStore) Update(ctx context.Context, sub *Subscription) error {
	if err := r.validateUpdate(ctx, sub); err != nil {
		return err
	}

	existing, err := r.Get(ctx, sub.ID)
	if err != nil {
		return err
	}

	sub.UpdatedAt = time.Now().UTC()
	sub.CreatedAt = existing.CreatedAt

	data, err := json.Marshal(sub)
	if err != nil {
		return fmt.Errorf("failed to marshal subscription: %w", err)
	}

	pipe := r.client.Pipeline()
	key := subscriptionKeyPrefix + sub.ID

	pipe.Set(ctx, key, data, subscriptionTTL)
	r.updateIndexesInPipeline(ctx, pipe, existing, sub)
	r.publishUpdateEvent(ctx, pipe, sub.ID)

	if _, err = pipe.Exec(ctx); err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	return nil
}

// validateUpdate validates the subscription update request.
func (r *RedisStore) validateUpdate(ctx context.Context, sub *Subscription) error {
	if sub.ID == "" {
		return ErrInvalidID
	}
	if err := r.validateCallbackURL(sub.Callback); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidCallback, err)
	}

	key := subscriptionKeyPrefix + sub.ID
	exists, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("failed to check subscription existence: %w", err)
	}
	if exists == 0 {
		return ErrSubscriptionNotFound
	}

	return nil
}

// updateIndexesInPipeline updates resource pool and type indexes if filters changed.
func (r *RedisStore) updateIndexesInPipeline(
	ctx context.Context,
	pipe redis.Pipeliner,
	existing, updated *Subscription,
) {
	r.updateResourcePoolIndex(ctx, pipe, existing, updated)
	r.updateResourceTypeIndex(ctx, pipe, existing, updated)
}

// updateResourcePoolIndex updates the resource pool index if changed.
func (r *RedisStore) updateResourcePoolIndex(
	ctx context.Context,
	pipe redis.Pipeliner,
	existing, updated *Subscription,
) {
	if existing.Filter.ResourcePoolID == updated.Filter.ResourcePoolID {
		return
	}

	if existing.Filter.ResourcePoolID != "" {
		oldPoolKey := subscriptionPoolIndexPrefix + existing.Filter.ResourcePoolID
		pipe.SRem(ctx, oldPoolKey, updated.ID)
	}
	if updated.Filter.ResourcePoolID != "" {
		newPoolKey := subscriptionPoolIndexPrefix + updated.Filter.ResourcePoolID
		pipe.SAdd(ctx, newPoolKey, updated.ID)
	}
}

// updateResourceTypeIndex updates the resource type index if changed.
func (r *RedisStore) updateResourceTypeIndex(
	ctx context.Context,
	pipe redis.Pipeliner,
	existing, updated *Subscription,
) {
	if existing.Filter.ResourceTypeID == updated.Filter.ResourceTypeID {
		return
	}

	if existing.Filter.ResourceTypeID != "" {
		oldTypeKey := subscriptionTypeIndexPrefix + existing.Filter.ResourceTypeID
		pipe.SRem(ctx, oldTypeKey, updated.ID)
	}
	if updated.Filter.ResourceTypeID != "" {
		newTypeKey := subscriptionTypeIndexPrefix + updated.Filter.ResourceTypeID
		pipe.SAdd(ctx, newTypeKey, updated.ID)
	}
}

// publishUpdateEvent publishes a subscription updated event.
func (r *RedisStore) publishUpdateEvent(ctx context.Context, pipe redis.Pipeliner, subID string) {
	eventData := map[string]interface{}{
		"event": "updated",
		"id":    subID,
	}
	eventJSON, _ := json.Marshal(eventData)
	pipe.Publish(ctx, subscriptionEventChannel, eventJSON)
}

// Delete deletes a subscription by ID.
// Returns ErrSubscriptionNotFound if the subscription does not exist.
func (r *RedisStore) Delete(ctx context.Context, id string) error {
	if id == "" {
		return ErrInvalidID
	}

	// Get existing subscription to access filter data
	existing, err := r.Get(ctx, id)
	if err != nil {
		return err
	}

	key := subscriptionKeyPrefix + id

	// Use pipeline for atomic operations
	pipe := r.client.Pipeline()

	// Delete subscription data
	pipe.Del(ctx, key)

	// Remove from active subscriptions set
	pipe.SRem(ctx, subscriptionSetKey, id)

	// Remove from resource pool index
	if existing.Filter.ResourcePoolID != "" {
		poolKey := subscriptionPoolIndexPrefix + existing.Filter.ResourcePoolID
		pipe.SRem(ctx, poolKey, id)
	}

	// Remove from resource type index
	if existing.Filter.ResourceTypeID != "" {
		typeKey := subscriptionTypeIndexPrefix + existing.Filter.ResourceTypeID
		pipe.SRem(ctx, typeKey, id)
	}

	// Remove from tenant index
	if existing.TenantID != "" {
		tenantKey := subscriptionTenantIndexPrefix + existing.TenantID
		pipe.SRem(ctx, tenantKey, id)
	}

	// Publish subscription deleted event
	eventData := map[string]interface{}{
		"event": "deleted",
		"id":    id,
	}
	eventJSON, _ := json.Marshal(eventData)
	pipe.Publish(ctx, subscriptionEventChannel, eventJSON)

	// Execute pipeline
	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete subscription: %w", err)
	}

	return nil
}

// List retrieves all subscriptions.
// Returns an empty slice if no subscriptions exist.
func (r *RedisStore) List(ctx context.Context) ([]*Subscription, error) {
	// Get all subscription IDs from the active set
	ids, err := r.client.SMembers(ctx, subscriptionSetKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to list subscription IDs: %w", err)
	}

	if len(ids) == 0 {
		return []*Subscription{}, nil
	}

	// Retrieve all subscriptions
	subs := make([]*Subscription, 0, len(ids))
	for _, id := range ids {
		sub, err := r.Get(ctx, id)
		if err != nil {
			// Skip subscriptions that failed to load (e.g., corrupted data)
			continue
		}
		subs = append(subs, sub)
	}

	return subs, nil
}

// ListByResourcePool retrieves subscriptions filtered by resource pool ID.
// Returns an empty slice if no matching subscriptions exist.
func (r *RedisStore) ListByResourcePool(ctx context.Context, resourcePoolID string) ([]*Subscription, error) {
	if resourcePoolID == "" {
		return []*Subscription{}, nil
	}

	poolKey := subscriptionPoolIndexPrefix + resourcePoolID

	// Get subscription IDs from pool index
	ids, err := r.client.SMembers(ctx, poolKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to list subscriptions by pool: %w", err)
	}

	if len(ids) == 0 {
		return []*Subscription{}, nil
	}

	// Retrieve subscriptions
	subs := make([]*Subscription, 0, len(ids))
	for _, id := range ids {
		sub, err := r.Get(ctx, id)
		if err != nil {
			continue
		}
		subs = append(subs, sub)
	}

	return subs, nil
}

// ListByResourceType retrieves subscriptions filtered by resource type ID.
// Returns an empty slice if no matching subscriptions exist.
func (r *RedisStore) ListByResourceType(ctx context.Context, resourceTypeID string) ([]*Subscription, error) {
	if resourceTypeID == "" {
		return []*Subscription{}, nil
	}

	typeKey := subscriptionTypeIndexPrefix + resourceTypeID

	// Get subscription IDs from type index
	ids, err := r.client.SMembers(ctx, typeKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to list subscriptions by type: %w", err)
	}

	if len(ids) == 0 {
		return []*Subscription{}, nil
	}

	// Retrieve subscriptions
	subs := make([]*Subscription, 0, len(ids))
	for _, id := range ids {
		sub, err := r.Get(ctx, id)
		if err != nil {
			continue
		}
		subs = append(subs, sub)
	}

	return subs, nil
}

// ListByTenant retrieves subscriptions filtered by tenant ID.
// Returns an empty slice if no matching subscriptions exist.
func (r *RedisStore) ListByTenant(ctx context.Context, tenantID string) ([]*Subscription, error) {
	if tenantID == "" {
		return []*Subscription{}, nil
	}

	tenantKey := subscriptionTenantIndexPrefix + tenantID

	// Get subscription IDs from tenant index
	ids, err := r.client.SMembers(ctx, tenantKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to list subscriptions by tenant: %w", err)
	}

	if len(ids) == 0 {
		return []*Subscription{}, nil
	}

	// Retrieve subscriptions
	subs := make([]*Subscription, 0, len(ids))
	for _, id := range ids {
		sub, err := r.Get(ctx, id)
		if err != nil {
			continue
		}
		subs = append(subs, sub)
	}

	return subs, nil
}

// Close closes the Redis connection and releases resources.
func (r *RedisStore) Close() error {
	if err := r.client.Close(); err != nil {
		return fmt.Errorf("failed to close Redis client: %w", err)
	}
	return nil
}

// Ping checks if Redis is available.
// Returns ErrStorageUnavailable if Redis cannot be reached.
func (r *RedisStore) Ping(ctx context.Context) error {
	if err := r.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("%w: %w", ErrStorageUnavailable, err)
	}
	return nil
}

// Client returns the underlying Redis client.
// This is used by middleware that needs direct Redis access (e.g., rate limiting).
func (r *RedisStore) Client() redis.UniversalClient {
	return r.client
}

// validateCallbackURL validates that a callback URL is properly formatted and secure.
// It enforces HTTPS unless AllowInsecureCallbacks is enabled in the configuration.
func (r *RedisStore) validateCallbackURL(callback string) error {
	if callback == "" {
		return fmt.Errorf("callback URL is empty")
	}

	u, err := url.Parse(callback)
	if err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}

	// Enforce HTTPS unless explicitly allowed
	if u.Scheme == "http" {
		if !r.config.AllowInsecureCallbacks {
			return fmt.Errorf("HTTP callbacks are not allowed in production. Use HTTPS for secure webhook delivery. To allow HTTP callbacks in development/testing, set allow_insecure_callbacks=true in security configuration")
		}
	} else if u.Scheme != "https" {
		return fmt.Errorf("callback URL must use http or https scheme")
	}

	if u.Host == "" {
		return fmt.Errorf("callback URL must have a host")
	}

	return nil
}
