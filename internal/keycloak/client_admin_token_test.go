package keycloak

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestClient_GetAdminToken_SingleFlight verifies that bursts of concurrent
// getAdminToken callers coalesce into a single HTTP grant to Keycloak.
//
// Run with the race detector: go test -race ./internal/keycloak/...
//
// This test is the regression guard for H2 (#475): before the singleflight +
// RWMutex fix, N concurrent callers would each POST admin credentials to
// /realms/master/.../token, which Keycloak's brute-force detector would
// interpret as an attack and lock out the master realm.
func TestClient_GetAdminToken_SingleFlight(t *testing.T) {
	const callers = 64

	var grantCount int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/realms/master/protocol/openid-connect/token" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		// Count every admin-grant hit. The SUT must collapse all in-flight
		// refreshers so this counter stays at 1 for a single burst.
		atomic.AddInt64(&grantCount, 1)

		// Slow the response slightly so concurrent callers contend on the
		// singleflight slot rather than racing sequentially on the cache.
		time.Sleep(25 * time.Millisecond)

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(TokenResponse{
			AccessToken: fmt.Sprintf("admin-token-%d", atomic.LoadInt64(&grantCount)),
			ExpiresIn:   3600,
			TokenType:   "Bearer",
		})
	}))
	defer server.Close()

	client, err := NewClient(&Config{
		BaseURL:       server.URL,
		Realm:         "test",
		ClientID:      AdminCLIClientID,
		AdminUsername: "admin",
		AdminPassword: "admin",
		Timeout:       5 * time.Second,
	})
	require.NoError(t, err)
	defer client.Close()

	ctx := context.Background()
	tokens := make([]string, callers)
	errs := make([]error, callers)

	var start sync.WaitGroup
	var done sync.WaitGroup
	start.Add(1)
	done.Add(callers)

	for i := 0; i < callers; i++ {
		go func(idx int) {
			defer done.Done()
			start.Wait() // Release all goroutines simultaneously.
			tokens[idx], errs[idx] = client.getAdminToken(ctx)
		}(i)
	}

	start.Done()
	done.Wait()

	require.Equal(t, int64(1), atomic.LoadInt64(&grantCount),
		"admin-credentials grant must be issued exactly once under concurrent load")

	// Every caller should observe the same token and no error.
	for i := 0; i < callers; i++ {
		require.NoErrorf(t, errs[i], "caller %d", i)
		assert.Equalf(t, tokens[0], tokens[i], "caller %d observed divergent token", i)
	}

	// Subsequent cache hits must not trigger a new HTTP call.
	cachedToken, err := client.getAdminToken(ctx)
	require.NoError(t, err)
	assert.Equal(t, tokens[0], cachedToken, "cache hit must return the same token")
	assert.Equal(t, int64(1), atomic.LoadInt64(&grantCount),
		"cache hit must not trigger an additional grant")
}

// TestClient_GetAdminToken_CacheExpiryRefresh verifies that once the cached
// token passes its expiry, a subsequent call triggers a single refresh.
func TestClient_GetAdminToken_CacheExpiryRefresh(t *testing.T) {
	var grantCount int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&grantCount, 1)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(TokenResponse{
			AccessToken: fmt.Sprintf("token-%d", atomic.LoadInt64(&grantCount)),
			ExpiresIn:   3600,
			TokenType:   "Bearer",
		})
	}))
	defer server.Close()

	client, err := NewClient(&Config{
		BaseURL:       server.URL,
		Realm:         "test",
		ClientID:      AdminCLIClientID,
		AdminUsername: "admin",
		AdminPassword: "admin",
		Timeout:       5 * time.Second,
	})
	require.NoError(t, err)
	defer client.Close()

	ctx := context.Background()

	first, err := client.getAdminToken(ctx)
	require.NoError(t, err)
	assert.Equal(t, "token-1", first)
	assert.Equal(t, int64(1), atomic.LoadInt64(&grantCount))

	// Force the cached expiry into the past and confirm we refresh.
	client.tokenMu.Lock()
	client.tokenExpiry = time.Now().Add(-1 * time.Second)
	client.tokenMu.Unlock()

	second, err := client.getAdminToken(ctx)
	require.NoError(t, err)
	assert.Equal(t, "token-2", second)
	assert.Equal(t, int64(2), atomic.LoadInt64(&grantCount))
}
