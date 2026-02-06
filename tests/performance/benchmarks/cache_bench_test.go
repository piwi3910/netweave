package benchmarks

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/piwi3910/netweave/internal/storage"
)

// BenchmarkRedisSet benchmarks Redis SET operations.
// Measures write performance for subscription storage.
func BenchmarkRedisSet(b *testing.B) {
	s, store := setupRedisStore(b)
	defer s.Close()

	ctx := context.Background()
	sub := createTestSubscription()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		sub.ID = fmt.Sprintf("test-sub-%d", i)
		if err := store.Create(ctx, sub); err != nil {
			b.Fatalf("failed to create subscription: %v", err)
		}
	}
}

// BenchmarkRedisGet benchmarks Redis GET operations.
// Measures read performance for subscription retrieval.
func BenchmarkRedisGet(b *testing.B) {
	s, store := setupRedisStore(b)
	defer s.Close()

	ctx := context.Background()
	sub := createTestSubscription()

	// Pre-populate
	if err := store.Create(ctx, sub); err != nil {
		b.Fatalf("failed to create subscription: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if _, err := store.Get(ctx, sub.ID); err != nil {
			b.Fatalf("failed to get subscription: %v", err)
		}
	}
}

// BenchmarkRedisDelete benchmarks Redis DEL operations.
// Measures delete performance.
func BenchmarkRedisDelete(b *testing.B) {
	s, store := setupRedisStore(b)
	defer s.Close()

	ctx := context.Background()

	// Pre-populate
	subs := make([]*storage.Subscription, b.N)
	for i := 0; i < b.N; i++ {
		sub := createTestSubscription()
		sub.ID = fmt.Sprintf("test-sub-%d", i)
		subs[i] = sub
		if err := store.Create(ctx, sub); err != nil {
			b.Fatalf("failed to create subscription: %v", err)
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if err := store.Delete(ctx, subs[i].ID); err != nil {
			b.Fatalf("failed to delete subscription: %v", err)
		}
	}
}

// BenchmarkRedisList benchmarks Redis list operations.
// Measures performance of listing all subscriptions.
func BenchmarkRedisList(b *testing.B) {
	s, store := setupRedisStore(b)
	defer s.Close()

	ctx := context.Background()

	// Pre-populate with 100 subscriptions
	for i := 0; i < 100; i++ {
		sub := createTestSubscription()
		sub.ID = fmt.Sprintf("test-sub-%d", i)
		if err := store.Create(ctx, sub); err != nil {
			b.Fatalf("failed to create subscription: %v", err)
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if _, err := store.List(ctx); err != nil {
			b.Fatalf("failed to list subscriptions: %v", err)
		}
	}
}

// BenchmarkRedisUpdate benchmarks Redis update operations.
// Measures performance of subscription updates.
func BenchmarkRedisUpdate(b *testing.B) {
	s, store := setupRedisStore(b)
	defer s.Close()

	ctx := context.Background()
	sub := createTestSubscription()

	// Pre-populate
	if err := store.Create(ctx, sub); err != nil {
		b.Fatalf("failed to create subscription: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		sub.Callback = "https://smo.example.com/notify/updated"
		if err := store.Update(ctx, sub); err != nil {
			b.Fatalf("failed to update subscription: %v", err)
		}
	}
}

// BenchmarkConcurrentRedisOps benchmarks concurrent Redis operations.
// Simulates realistic multi-goroutine access patterns.
func BenchmarkConcurrentRedisOps(b *testing.B) {
	s, store := setupRedisStore(b)
	defer s.Close()

	ctx := context.Background()

	// Pre-populate
	for i := 0; i < 100; i++ {
		sub := createTestSubscription()
		sub.ID = fmt.Sprintf("test-sub-%d", i)
		if err := store.Create(ctx, sub); err != nil {
			b.Fatalf("failed to create subscription: %v", err)
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			subID := fmt.Sprintf("test-sub-%d", i%100)
			if _, err := store.Get(ctx, subID); err != nil {
				b.Errorf("failed to get subscription: %v", err)
			}
			i++
		}
	})
}

// BenchmarkRedisConnectionPool benchmarks connection pool overhead.
// Measures performance with multiple concurrent connections.
func BenchmarkRedisConnectionPool(b *testing.B) {
	s, store := setupRedisStore(b)
	defer s.Close()

	ctx := context.Background()
	sub := createTestSubscription()

	// Pre-populate
	if err := store.Create(ctx, sub); err != nil {
		b.Fatalf("failed to create subscription: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// Mix of operations to test connection pool
			if _, err := store.Get(ctx, sub.ID); err != nil {
				b.Errorf("failed to get subscription: %v", err)
			}

			if _, err := store.List(ctx); err != nil {
				b.Errorf("failed to list subscriptions: %v", err)
			}
		}
	})
}

// Helper functions

func setupRedisStore(b *testing.B) (*miniredis.Miniredis, storage.Store) {
	b.Helper()

	s := miniredis.RunT(b)

	cfg := &storage.RedisConfig{
		Addr:         s.Addr(),
		DB:           0,
		MaxRetries:   3,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     10,
	}

	store := storage.NewRedisStore(cfg)
	return s, store
}

func createTestSubscription() *storage.Subscription {
	return &storage.Subscription{
		ID:                     "test-sub-1",
		Callback:               "https://smo.example.com/notify",
		ConsumerSubscriptionID: "consumer-1",
	}
}
