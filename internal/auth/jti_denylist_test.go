package auth_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/piwi3910/netweave/internal/auth"
)

// newTestDenylist wires a RedisJTIDenylist against an ephemeral miniredis.
func newTestDenylist(t *testing.T) (*auth.RedisJTIDenylist, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})
	return auth.NewRedisJTIDenylist(client), mr
}

func TestRedisJTIDenylist_IsRevoked_EmptyJTI(t *testing.T) {
	denylist, _ := newTestDenylist(t)

	// Empty JTI is tolerated — callers at a higher layer decide whether an
	// empty jti should cause a hard rejection (e.g. via policy).
	revoked, err := denylist.IsRevoked(context.Background(), "")
	require.NoError(t, err)
	assert.False(t, revoked)
}

func TestRedisJTIDenylist_RoundTrip(t *testing.T) {
	denylist, _ := newTestDenylist(t)
	ctx := context.Background()

	// Not yet revoked.
	revoked, err := denylist.IsRevoked(ctx, "jti-1")
	require.NoError(t, err)
	assert.False(t, revoked)

	// Revoke and confirm.
	require.NoError(t, denylist.Revoke(ctx, "jti-1", 10*time.Minute))
	revoked, err = denylist.IsRevoked(ctx, "jti-1")
	require.NoError(t, err)
	assert.True(t, revoked)

	// Unrelated JTI remains clean.
	revoked, err = denylist.IsRevoked(ctx, "jti-2")
	require.NoError(t, err)
	assert.False(t, revoked)
}

func TestRedisJTIDenylist_Revoke_TTLExpires(t *testing.T) {
	denylist, mr := newTestDenylist(t)
	ctx := context.Background()

	require.NoError(t, denylist.Revoke(ctx, "jti-expire", 30*time.Second))
	revoked, err := denylist.IsRevoked(ctx, "jti-expire")
	require.NoError(t, err)
	assert.True(t, revoked)

	// Fast-forward past TTL using miniredis.
	mr.FastForward(31 * time.Second)
	revoked, err = denylist.IsRevoked(ctx, "jti-expire")
	require.NoError(t, err)
	assert.False(t, revoked, "denylist entries must expire at Redis TTL")
}

func TestRedisJTIDenylist_Revoke_Validation(t *testing.T) {
	denylist, _ := newTestDenylist(t)
	ctx := context.Background()

	require.Error(t, denylist.Revoke(ctx, "", time.Minute), "empty jti must be rejected")
	require.Error(t, denylist.Revoke(ctx, "jti", 0), "zero ttl must be rejected")
	require.Error(t, denylist.Revoke(ctx, "jti", -1*time.Second), "negative ttl must be rejected")
}
