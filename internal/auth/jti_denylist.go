package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// ErrAccountDisabled is returned when an authenticated user has been disabled.
var ErrAccountDisabled = errors.New("account disabled")

// ErrTokenRevoked is returned when a token's JTI has been placed on the denylist.
var ErrTokenRevoked = errors.New("token revoked")

// JTIDenylist provides revocation checks for OAuth2 bearer tokens keyed by JWT ID (jti).
// Implementations must be safe for concurrent use.
type JTIDenylist interface {
	// IsRevoked reports whether the given JTI has been revoked. A nil error with
	// the zero value means the token is not on the denylist.
	IsRevoked(ctx context.Context, jti string) (bool, error)

	// Revoke places the given JTI on the denylist until ttl elapses. A non-positive
	// ttl is rejected to prevent permanent pollution of the denylist.
	Revoke(ctx context.Context, jti string, ttl time.Duration) error
}

// jtiDenylistKeyPrefix is the Redis key prefix for JTI denylist entries.
const jtiDenylistKeyPrefix = "auth:jti:revoked:"

// RedisJTIDenylist implements JTIDenylist using Redis for short-lived revocation records.
// Entries expire automatically at Redis TTL, so the denylist only needs to cover the
// remaining lifetime of the revoked token.
type RedisJTIDenylist struct {
	client redis.UniversalClient
}

// NewRedisJTIDenylist creates a new Redis-backed JTI denylist.
func NewRedisJTIDenylist(client redis.UniversalClient) *RedisJTIDenylist {
	return &RedisJTIDenylist{client: client}
}

// IsRevoked returns true if the JTI exists in Redis.
// An empty jti is always considered not revoked (callers are expected to reject
// empty JTIs at a higher level if the policy requires a jti claim).
func (d *RedisJTIDenylist) IsRevoked(ctx context.Context, jti string) (bool, error) {
	if jti == "" {
		return false, nil
	}
	n, err := d.client.Exists(ctx, jtiDenylistKeyPrefix+jti).Result()
	if err != nil {
		return false, fmt.Errorf("jti denylist lookup failed: %w", err)
	}
	return n > 0, nil
}

// Revoke places a JTI on the denylist with the given TTL.
// Returns an error for empty jti or non-positive ttl.
func (d *RedisJTIDenylist) Revoke(ctx context.Context, jti string, ttl time.Duration) error {
	if jti == "" {
		return fmt.Errorf("jti is required")
	}
	if ttl <= 0 {
		return fmt.Errorf("ttl must be positive")
	}
	if err := d.client.Set(ctx, jtiDenylistKeyPrefix+jti, "1", ttl).Err(); err != nil {
		return fmt.Errorf("jti denylist write failed: %w", err)
	}
	return nil
}
