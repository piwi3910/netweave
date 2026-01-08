package controllers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/piwi3910/netweave/internal/storage"
)

func TestNewSubscriptionController(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil config",
			cfg:     nil,
			wantErr: true,
			errMsg:  "config cannot be nil",
		},
		{
			name: "nil k8s client",
			cfg: &Config{
				Store:       &mockStore{},
				RedisClient: &redis.Client{},
				Logger:      zaptest.NewLogger(t),
				OCloudID:    "test-ocloud",
			},
			wantErr: true,
			errMsg:  "k8s client cannot be nil",
		},
		{
			name: "nil store",
			cfg: &Config{
				K8sClient:   fake.NewSimpleClientset(),
				RedisClient: &redis.Client{},
				Logger:      zaptest.NewLogger(t),
				OCloudID:    "test-ocloud",
			},
			wantErr: true,
			errMsg:  "store cannot be nil",
		},
		{
			name: "nil redis client",
			cfg: &Config{
				K8sClient: fake.NewSimpleClientset(),
				Store:     &mockStore{},
				Logger:    zaptest.NewLogger(t),
				OCloudID:  "test-ocloud",
			},
			wantErr: true,
			errMsg:  "redis client cannot be nil",
		},
		{
			name: "nil logger",
			cfg: &Config{
				K8sClient:   fake.NewSimpleClientset(),
				Store:       &mockStore{},
				RedisClient: &redis.Client{},
				OCloudID:    "test-ocloud",
			},
			wantErr: true,
			errMsg:  "logger cannot be nil",
		},
		{
			name: "empty ocloud id",
			cfg: &Config{
				K8sClient:   fake.NewSimpleClientset(),
				Store:       &mockStore{},
				RedisClient: &redis.Client{},
				Logger:      zaptest.NewLogger(t),
			},
			wantErr: true,
			errMsg:  "oCloudID cannot be empty",
		},
		{
			name: "valid config",
			cfg: &Config{
				K8sClient:   fake.NewSimpleClientset(),
				Store:       &mockStore{},
				RedisClient: &redis.Client{},
				Logger:      zaptest.NewLogger(t),
				OCloudID:    "test-ocloud",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl, err := NewSubscriptionController(tt.cfg)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, ctrl)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, ctrl)
				assert.Equal(t, tt.cfg.K8sClient, ctrl.k8sClient)
				assert.Equal(t, tt.cfg.Store, ctrl.store)
				assert.Equal(t, tt.cfg.RedisClient, ctrl.redisClient)
				assert.Equal(t, tt.cfg.Logger, ctrl.logger)
				assert.Equal(t, tt.cfg.OCloudID, ctrl.oCloudID)
			}
		})
	}
}

func TestSubscriptionController_ProcessNodeEvent(t *testing.T) {
	// Setup miniredis for event queue
	mr := miniredis.RunT(t)
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer func() {
		require.NoError(t, rdb.Close())
	}()

	// Create fake K8s client
	k8sClient := fake.NewSimpleClientset()

	// Create test node
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node-1",
			Labels: map[string]string{
				"resource-pool": "test-pool",
			},
		},
	}

	// Create test subscription
	sub := &storage.Subscription{
		ID:       "sub-123",
		Callback: "https://smo.example.com/notify",
		Filter: storage.SubscriptionFilter{
			ResourcePoolID: "test-pool",
			ResourceTypeID: "k8s-node",
		},
		CreatedAt: time.Now(),
	}

	store := &mockStore{
		subscriptions: []*storage.Subscription{sub},
	}

	// Create controller
	ctrl, err := NewSubscriptionController(&Config{
		K8sClient:   k8sClient,
		Store:       store,
		RedisClient: rdb,
		Logger:      zaptest.NewLogger(t),
		OCloudID:    "test-ocloud",
	})
	require.NoError(t, err)

	ctx := context.Background()

	// Process node creation event
	ctrl.processNodeEvent(node, EventTypeCreated)

	// Verify event was queued to Redis Stream
	streams, err := rdb.XRead(ctx, &redis.XReadArgs{
		Streams: []string{EventStreamKey, "0"},
		Count:   1,
	}).Result()
	require.NoError(t, err)
	require.Len(t, streams, 1)
	require.Len(t, streams[0].Messages, 1)

	// Parse event
	eventData := streams[0].Messages[0].Values["event"].(string)
	var event ResourceEvent
	err = json.Unmarshal([]byte(eventData), &event)
	require.NoError(t, err)

	// Verify event content
	assert.Equal(t, "sub-123", event.SubscriptionID)
	assert.Equal(t, "o2ims.Resource.Created", event.EventType)
	assert.Equal(t, "/o2ims/v1/resources/test-node-1", event.ObjectRef)
	assert.Equal(t, "k8s-node", event.ResourceTypeID)
	assert.Equal(t, "test-pool", event.ResourcePoolID)
	assert.Equal(t, "test-node-1", event.GlobalResourceID)
	assert.Equal(t, "https://smo.example.com/notify", event.CallbackURL)
}

func TestSubscriptionController_ProcessNamespaceEvent(t *testing.T) {
	// Setup miniredis for event queue
	mr := miniredis.RunT(t)
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer func() {
		require.NoError(t, rdb.Close())
	}()

	// Create fake K8s client
	k8sClient := fake.NewSimpleClientset()

	// Create test namespace
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-namespace",
		},
	}

	// Create test subscription
	sub := &storage.Subscription{
		ID:       "sub-456",
		Callback: "https://smo.example.com/notify",
		Filter: storage.SubscriptionFilter{
			ResourceTypeID: "k8s-namespace",
		},
		CreatedAt: time.Now(),
	}

	store := &mockStore{
		subscriptions: []*storage.Subscription{sub},
	}

	// Create controller
	ctrl, err := NewSubscriptionController(&Config{
		K8sClient:   k8sClient,
		Store:       store,
		RedisClient: rdb,
		Logger:      zaptest.NewLogger(t),
		OCloudID:    "test-ocloud",
	})
	require.NoError(t, err)

	ctx := context.Background()

	// Process namespace creation event
	ctrl.processNamespaceEvent(ns, EventTypeCreated)

	// Verify event was queued to Redis Stream
	streams, err := rdb.XRead(ctx, &redis.XReadArgs{
		Streams: []string{EventStreamKey, "0"},
		Count:   1,
	}).Result()
	require.NoError(t, err)
	require.Len(t, streams, 1)
	require.Len(t, streams[0].Messages, 1)

	// Parse event
	eventData := streams[0].Messages[0].Values["event"].(string)
	var event ResourceEvent
	err = json.Unmarshal([]byte(eventData), &event)
	require.NoError(t, err)

	// Verify event content
	assert.Equal(t, "sub-456", event.SubscriptionID)
	assert.Equal(t, "o2ims.ResourcePool.Created", event.EventType)
	assert.Equal(t, "/o2ims/v1/resourcePools/test-namespace", event.ObjectRef)
	assert.Equal(t, "k8s-namespace", event.ResourceTypeID)
	assert.Equal(t, "test-namespace", event.GlobalResourceID)
	assert.Equal(t, "https://smo.example.com/notify", event.CallbackURL)
}

func TestSubscriptionController_MatchesFilter(t *testing.T) {
	ctrl := &SubscriptionController{}

	tests := []struct {
		name         string
		sub          *storage.Subscription
		resourceType string
		resourcePool string
		resourceID   string
		wantMatch    bool
	}{
		{
			name: "empty filter matches all",
			sub: &storage.Subscription{
				Filter: storage.SubscriptionFilter{},
			},
			resourceType: "k8s-node",
			resourcePool: "pool-1",
			resourceID:   "node-1",
			wantMatch:    true,
		},
		{
			name: "resource type filter matches",
			sub: &storage.Subscription{
				Filter: storage.SubscriptionFilter{
					ResourceTypeID: "k8s-node",
				},
			},
			resourceType: "k8s-node",
			resourcePool: "pool-1",
			resourceID:   "node-1",
			wantMatch:    true,
		},
		{
			name: "resource type filter does not match",
			sub: &storage.Subscription{
				Filter: storage.SubscriptionFilter{
					ResourceTypeID: "k8s-node",
				},
			},
			resourceType: "k8s-namespace",
			resourcePool: "pool-1",
			resourceID:   "ns-1",
			wantMatch:    false,
		},
		{
			name: "resource pool filter matches",
			sub: &storage.Subscription{
				Filter: storage.SubscriptionFilter{
					ResourcePoolID: "pool-1",
				},
			},
			resourceType: "k8s-node",
			resourcePool: "pool-1",
			resourceID:   "node-1",
			wantMatch:    true,
		},
		{
			name: "resource pool filter does not match",
			sub: &storage.Subscription{
				Filter: storage.SubscriptionFilter{
					ResourcePoolID: "pool-1",
				},
			},
			resourceType: "k8s-node",
			resourcePool: "pool-2",
			resourceID:   "node-1",
			wantMatch:    false,
		},
		{
			name: "multiple filters all match",
			sub: &storage.Subscription{
				Filter: storage.SubscriptionFilter{
					ResourceTypeID: "k8s-node",
					ResourcePoolID: "pool-1",
				},
			},
			resourceType: "k8s-node",
			resourcePool: "pool-1",
			resourceID:   "node-1",
			wantMatch:    true,
		},
		{
			name: "multiple filters one does not match",
			sub: &storage.Subscription{
				Filter: storage.SubscriptionFilter{
					ResourceTypeID: "k8s-node",
					ResourcePoolID: "pool-1",
				},
			},
			resourceType: "k8s-node",
			resourcePool: "pool-2",
			resourceID:   "node-1",
			wantMatch:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := ctrl.matchesFilter(tt.sub, tt.resourceType, tt.resourcePool, tt.resourceID)
			assert.Equal(t, tt.wantMatch, matches)
		})
	}
}

// mockStore is a mock implementation of storage.Store for testing.
type mockStore struct {
	subscriptions []*storage.Subscription
	getErr        error
	listErr       error
}

func (m *mockStore) Create(_ context.Context, _ *storage.Subscription) error {
	return nil
}

func (m *mockStore) Get(_ context.Context, _ string) (*storage.Subscription, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if len(m.subscriptions) > 0 {
		return m.subscriptions[0], nil
	}
	return nil, storage.ErrSubscriptionNotFound
}

func (m *mockStore) Update(_ context.Context, _ *storage.Subscription) error {
	return nil
}

func (m *mockStore) Delete(_ context.Context, _ string) error {
	return nil
}

func (m *mockStore) List(_ context.Context) ([]*storage.Subscription, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.subscriptions, nil
}

func (m *mockStore) ListByResourcePool(_ context.Context, _ string) ([]*storage.Subscription, error) {
	return m.subscriptions, nil
}

func (m *mockStore) ListByResourceType(_ context.Context, _ string) ([]*storage.Subscription, error) {
	return m.subscriptions, nil
}

func (m *mockStore) ListByTenant(_ context.Context, _ string) ([]*storage.Subscription, error) {
	return m.subscriptions, nil
}

func (m *mockStore) Close() error {
	return nil
}

func (m *mockStore) Ping(_ context.Context) error {
	return nil
}
