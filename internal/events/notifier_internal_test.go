package events

import (
	"fmt"
	"testing"
	"time"

	"github.com/sony/gobreaker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestEvictOldestLocked(t *testing.T) {
	logger := zaptest.NewLogger(t)

	n := &WebhookNotifier{
		logger:          logger,
		circuitBreakers: make(map[string]*gobreaker.CircuitBreaker),
		cbLastUsed:      make(map[string]time.Time),
		stopCleanup:     make(chan struct{}),
	}

	// Add 3 circuit breakers with staggered timestamps.
	now := time.Now()
	n.circuitBreakers["url-old"] = gobreaker.NewCircuitBreaker(gobreaker.Settings{Name: "url-old"})
	n.cbLastUsed["url-old"] = now.Add(-30 * time.Minute)

	n.circuitBreakers["url-mid"] = gobreaker.NewCircuitBreaker(gobreaker.Settings{Name: "url-mid"})
	n.cbLastUsed["url-mid"] = now.Add(-10 * time.Minute)

	n.circuitBreakers["url-new"] = gobreaker.NewCircuitBreaker(gobreaker.Settings{Name: "url-new"})
	n.cbLastUsed["url-new"] = now

	require.Len(t, n.circuitBreakers, 3)

	// Evict oldest — should remove "url-old".
	n.evictOldestLocked()

	assert.Len(t, n.circuitBreakers, 2)
	assert.Len(t, n.cbLastUsed, 2)
	_, hasOld := n.circuitBreakers["url-old"]
	assert.False(t, hasOld, "oldest entry should be evicted")
	_, hasMid := n.circuitBreakers["url-mid"]
	assert.True(t, hasMid)
	_, hasNew := n.circuitBreakers["url-new"]
	assert.True(t, hasNew)
}

func TestEvictOldestLocked_EmptyMap(t *testing.T) {
	logger := zaptest.NewLogger(t)

	n := &WebhookNotifier{
		logger:          logger,
		circuitBreakers: make(map[string]*gobreaker.CircuitBreaker),
		cbLastUsed:      make(map[string]time.Time),
		stopCleanup:     make(chan struct{}),
	}

	// Should not panic on empty map.
	n.evictOldestLocked()
	assert.Empty(t, n.circuitBreakers)
}

func TestPruneStaleCircuitBreakers(t *testing.T) {
	logger := zaptest.NewLogger(t)

	n := &WebhookNotifier{
		logger:          logger,
		circuitBreakers: make(map[string]*gobreaker.CircuitBreaker),
		cbLastUsed:      make(map[string]time.Time),
		stopCleanup:     make(chan struct{}),
	}

	now := time.Now()

	// Add a stale entry (older than cbCleanupInterval = 10 min).
	n.circuitBreakers["stale-url"] = gobreaker.NewCircuitBreaker(gobreaker.Settings{Name: "stale-url"})
	n.cbLastUsed["stale-url"] = now.Add(-15 * time.Minute)

	// Add a fresh entry.
	n.circuitBreakers["fresh-url"] = gobreaker.NewCircuitBreaker(gobreaker.Settings{Name: "fresh-url"})
	n.cbLastUsed["fresh-url"] = now

	require.Len(t, n.circuitBreakers, 2)

	n.pruneStaleCircuitBreakers()

	assert.Len(t, n.circuitBreakers, 1)
	_, hasStale := n.circuitBreakers["stale-url"]
	assert.False(t, hasStale, "stale entry should be pruned")
	_, hasFresh := n.circuitBreakers["fresh-url"]
	assert.True(t, hasFresh, "fresh entry should be kept")
}

func TestPruneStaleCircuitBreakers_NothingToPrune(t *testing.T) {
	logger := zaptest.NewLogger(t)

	n := &WebhookNotifier{
		logger:          logger,
		circuitBreakers: make(map[string]*gobreaker.CircuitBreaker),
		cbLastUsed:      make(map[string]time.Time),
		stopCleanup:     make(chan struct{}),
	}

	// All entries are fresh.
	n.circuitBreakers["url-1"] = gobreaker.NewCircuitBreaker(gobreaker.Settings{Name: "url-1"})
	n.cbLastUsed["url-1"] = time.Now()
	n.circuitBreakers["url-2"] = gobreaker.NewCircuitBreaker(gobreaker.Settings{Name: "url-2"})
	n.cbLastUsed["url-2"] = time.Now()

	n.pruneStaleCircuitBreakers()

	assert.Len(t, n.circuitBreakers, 2, "nothing should be pruned")
}

func TestGetCircuitBreaker_EvictsAtCapacity(t *testing.T) {
	logger := zaptest.NewLogger(t)

	n := &WebhookNotifier{
		logger:          logger,
		circuitBreakers: make(map[string]*gobreaker.CircuitBreaker),
		cbLastUsed:      make(map[string]time.Time),
		stopCleanup:     make(chan struct{}),
	}

	// Fill to maxCircuitBreakers.
	now := time.Now()
	for i := 0; i < maxCircuitBreakers; i++ {
		url := fmt.Sprintf("https://callback-%d.example.com", i)
		n.circuitBreakers[url] = gobreaker.NewCircuitBreaker(gobreaker.Settings{Name: url})
		n.cbLastUsed[url] = now.Add(time.Duration(i) * time.Second)
	}

	require.Len(t, n.circuitBreakers, maxCircuitBreakers)

	// Adding one more should trigger eviction of the oldest.
	cb := n.getCircuitBreaker("https://new-callback.example.com")
	assert.NotNil(t, cb)
	assert.Len(t, n.circuitBreakers, maxCircuitBreakers, "map should stay at max after eviction+add")

	// The oldest entry (callback-0 with the earliest timestamp) should be gone.
	_, hasOldest := n.circuitBreakers["https://callback-0.example.com"]
	assert.False(t, hasOldest, "oldest entry should have been evicted")
}
