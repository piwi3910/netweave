package controllers

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/piwi3910/netweave/internal/storage"
)

// cancelBlockingStore is a storage.Store whose List blocks until its own
// context is canceled. It exposes channels so tests can synchronize on
// "in-flight work started" and "work released".
type cancelBlockingStore struct {
	entered  chan struct{}
	released chan struct{}
	listErr  error
}

func (b *cancelBlockingStore) Create(_ context.Context, _ *storage.Subscription) error {
	return nil
}

func (b *cancelBlockingStore) Get(_ context.Context, _ string) (*storage.Subscription, error) {
	return nil, storage.ErrSubscriptionNotFound
}

func (b *cancelBlockingStore) Update(_ context.Context, _ *storage.Subscription) error {
	return nil
}

func (b *cancelBlockingStore) Delete(_ context.Context, _ string) error {
	return nil
}

func (b *cancelBlockingStore) List(ctx context.Context) ([]*storage.Subscription, error) {
	// Signal the test that we've entered List. Non-blocking so repeated
	// calls do not deadlock.
	select {
	case b.entered <- struct{}{}:
	default:
	}
	select {
	case <-ctx.Done():
		b.listErr = ctx.Err()
		close(b.released)
		return nil, fmt.Errorf("cancelBlockingStore List: %w", ctx.Err())
	case <-time.After(10 * time.Second):
		// Safety net: if cancellation never arrives we still return so
		// the test fails deterministically rather than hangs.
		close(b.released)
		return nil, nil
	}
}

func (b *cancelBlockingStore) ListByResourcePool(_ context.Context, _ string) ([]*storage.Subscription, error) {
	return nil, nil
}

func (b *cancelBlockingStore) ListByResourceType(_ context.Context, _ string) ([]*storage.Subscription, error) {
	return nil, nil
}

func (b *cancelBlockingStore) ListByTenant(_ context.Context, _ string) ([]*storage.Subscription, error) {
	return nil, nil
}

func (b *cancelBlockingStore) Close() error { return nil }

func (b *cancelBlockingStore) Ping(_ context.Context) error { return nil }

// TestHandleNodeAddUnwindsOnCancel verifies the fix for H14: canceling the
// controller-scoped context unwinds in-flight informer-handler work promptly
// instead of blocking until pod termination.
func TestHandleNodeAddUnwindsOnCancel(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() { _ = rdb.Close() }()

	store := &cancelBlockingStore{
		entered:  make(chan struct{}, 1),
		released: make(chan struct{}),
	}
	logger := zaptest.NewLogger(t)

	ctrl, err := NewSubscriptionController(&Config{
		K8sClient:   fake.NewClientset(),
		Store:       store,
		RedisClient: rdb,
		Logger:      logger,
		OCloudID:    "test-ocloud",
	})
	require.NoError(t, err)

	// Wire up the controller-scoped context the same way Start does, but
	// without running the informer factory (which requires a live API
	// server). This keeps the test focused on the ctx-propagation fix.
	parentCtx, parentCancel := context.WithCancel(context.Background())
	ctrl.ctxMu.Lock()
	ctrl.ctx = parentCtx
	ctrl.cancel = parentCancel
	ctrl.ctxMu.Unlock()

	// Kick off the informer handler. It derives a bounded child of the
	// controller ctx and calls Store.List, which blocks until ctx is
	// canceled.
	handlerDone := make(chan struct{})
	go func() {
		ctrl.handleNodeAdd(&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
		})
		close(handlerDone)
	}()

	// Make sure List is actually in flight before we cancel.
	select {
	case <-store.entered:
	case <-time.After(2 * time.Second):
		parentCancel()
		t.Fatal("blocking store List was never invoked")
	}

	start := time.Now()
	// Simulate a controller shutdown by calling Stop, which cancels the
	// controller-scoped ctx. In-flight work must unwind promptly.
	require.NoError(t, ctrl.Stop())

	select {
	case <-handlerDone:
		// Good: handler unwound.
	case <-time.After(3 * time.Second):
		t.Fatal("informer handler did not unwind after controller Stop")
	}

	// Confirm we actually released via cancellation, not the safety
	// deadline. And that it happened quickly, not after the 10s safety
	// net.
	select {
	case <-store.released:
	case <-time.After(time.Second):
		t.Fatal("blocking store List did not release")
	}
	require.ErrorIs(t, store.listErr, context.Canceled)
	assert.Less(t, time.Since(start), 3*time.Second,
		"cancellation should unwind in-flight work well before the safety deadline")
}

// TestHandlerContextFallsBackWhenNotStarted verifies that handlerContext
// returns a usable timeout-bounded context even when Start has not been
// called (defensive path used by direct-call tests and panic-recovery).
func TestHandlerContextFallsBackWhenNotStarted(t *testing.T) {
	logger := zaptest.NewLogger(t)
	mr := miniredis.RunT(t)
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() { _ = rdb.Close() }()

	ctrl, err := NewSubscriptionController(&Config{
		K8sClient:   fake.NewClientset(),
		Store:       &cancelBlockingStore{entered: make(chan struct{}, 1), released: make(chan struct{})},
		RedisClient: rdb,
		Logger:      logger,
		OCloudID:    "test-ocloud",
	})
	require.NoError(t, err)

	ctx, cancel := ctrl.handlerContext()
	defer cancel()

	require.NotNil(t, ctx)
	deadline, ok := ctx.Deadline()
	require.True(t, ok, "handlerContext must impose a deadline")
	assert.WithinDuration(t, time.Now().Add(HandlerTimeout), deadline, time.Second)
}
