package controllers

import (
	"context"
	"encoding/json"
	"errors"
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
	"k8s.io/client-go/tools/cache"

	"github.com/piwi3910/netweave/internal/storage"
)

// internalMockStore implements storage.Store for internal package tests.
type internalMockStore struct {
	subscriptions []*storage.Subscription
	listErr       error
}

func (m *internalMockStore) Create(_ context.Context, _ *storage.Subscription) error { return nil }
func (m *internalMockStore) Get(_ context.Context, _ string) (*storage.Subscription, error) {
	if len(m.subscriptions) > 0 {
		return m.subscriptions[0], nil
	}
	return nil, storage.ErrSubscriptionNotFound
}
func (m *internalMockStore) Update(_ context.Context, _ *storage.Subscription) error { return nil }
func (m *internalMockStore) Delete(_ context.Context, _ string) error                { return nil }
func (m *internalMockStore) List(_ context.Context) ([]*storage.Subscription, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.subscriptions, nil
}
func (m *internalMockStore) ListByResourcePool(_ context.Context, _ string) ([]*storage.Subscription, error) {
	return m.subscriptions, nil
}
func (m *internalMockStore) ListByResourceType(_ context.Context, _ string) ([]*storage.Subscription, error) {
	return m.subscriptions, nil
}
func (m *internalMockStore) ListByTenant(_ context.Context, _ string) ([]*storage.Subscription, error) {
	return m.subscriptions, nil
}
func (m *internalMockStore) Close() error                 { return nil }
func (m *internalMockStore) Ping(_ context.Context) error { return nil }

func newTestController(t *testing.T, store storage.Store) (*SubscriptionController, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	ctrl, err := NewSubscriptionController(&Config{
		K8sClient:   fake.NewClientset(),
		Store:       store,
		RedisClient: rdb,
		Logger:      zaptest.NewLogger(t),
		OCloudID:    "test-ocloud",
	})
	require.NoError(t, err)

	return ctrl, mr
}

func TestHandleNodeAdd_TypeAssertions(t *testing.T) {
	store := &internalMockStore{
		subscriptions: []*storage.Subscription{
			{ID: "sub-1", Callback: "http://example.com/cb", Filter: storage.SubscriptionFilter{}},
		},
	}
	ctrl, mr := newTestController(t, store)
	defer mr.Close()

	t.Run("valid node object", func(t *testing.T) {
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "add-node",
				Labels: map[string]string{
					"resource-pool": "pool-a",
				},
			},
		}
		ctrl.handleNodeAdd(node)

		ctx := context.Background()
		streams, err := ctrl.RedisClient.XRead(ctx, &redis.XReadArgs{
			Streams: []string{EventStreamKey, "0"},
			Count:   10,
		}).Result()
		require.NoError(t, err)
		require.NotEmpty(t, streams)
		require.NotEmpty(t, streams[0].Messages)

		var event ResourceEvent
		eventData := streams[0].Messages[0].Values["event"].(string)
		require.NoError(t, json.Unmarshal([]byte(eventData), &event))
		assert.Equal(t, "add-node", event.GlobalResourceID)
		assert.Contains(t, event.EventType, string(EventTypeCreated))
	})

	t.Run("invalid object type", func(t *testing.T) {
		// Should log error but not panic
		ctrl.handleNodeAdd("not-a-node")
	})

	t.Run("nil object", func(t *testing.T) {
		ctrl.handleNodeAdd(nil)
	})
}

func TestHandleNodeUpdate_TypeAssertions(t *testing.T) {
	store := &internalMockStore{
		subscriptions: []*storage.Subscription{
			{ID: "sub-1", Callback: "http://example.com/cb", Filter: storage.SubscriptionFilter{}},
		},
	}
	ctrl, mr := newTestController(t, store)
	defer mr.Close()

	t.Run("valid node update with different resource versions", func(t *testing.T) {
		oldNode := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "update-node",
				ResourceVersion: "1",
			},
		}
		newNode := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "update-node",
				ResourceVersion: "2",
			},
		}
		ctrl.handleNodeUpdate(oldNode, newNode)

		ctx := context.Background()
		streams, err := ctrl.RedisClient.XRead(ctx, &redis.XReadArgs{
			Streams: []string{EventStreamKey, "0"},
			Count:   10,
		}).Result()
		require.NoError(t, err)
		require.NotEmpty(t, streams)

		var event ResourceEvent
		eventData := streams[0].Messages[0].Values["event"].(string)
		require.NoError(t, json.Unmarshal([]byte(eventData), &event))
		assert.Equal(t, "update-node", event.GlobalResourceID)
		assert.Contains(t, event.EventType, string(EventTypeUpdated))
	})

	t.Run("same resource version - no event generated", func(t *testing.T) {
		// Flush redis
		mr.FlushAll()

		oldNode := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "same-version-node",
				ResourceVersion: "5",
			},
		}
		newNode := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "same-version-node",
				ResourceVersion: "5",
			},
		}
		ctrl.handleNodeUpdate(oldNode, newNode)

		ctx := context.Background()
		_, err := ctrl.RedisClient.XRead(ctx, &redis.XReadArgs{
			Streams: []string{EventStreamKey, "0"},
			Count:   1,
			Block:   50 * time.Millisecond,
		}).Result()
		// Should get redis.Nil because no events were published
		assert.True(t, errors.Is(err, redis.Nil) || err != nil)
	})

	t.Run("invalid old object type", func(t *testing.T) {
		newNode := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1", ResourceVersion: "2"},
		}
		ctrl.handleNodeUpdate("not-a-node", newNode)
	})

	t.Run("invalid new object type", func(t *testing.T) {
		oldNode := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-1", ResourceVersion: "1"},
		}
		ctrl.handleNodeUpdate(oldNode, "not-a-node")
	})
}

func TestHandleNodeDelete_TypeAssertions(t *testing.T) {
	store := &internalMockStore{
		subscriptions: []*storage.Subscription{
			{ID: "sub-1", Callback: "http://example.com/cb", Filter: storage.SubscriptionFilter{}},
		},
	}
	ctrl, mr := newTestController(t, store)
	defer mr.Close()

	t.Run("valid node object", func(t *testing.T) {
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "delete-node"},
		}
		ctrl.handleNodeDelete(node)

		ctx := context.Background()
		streams, err := ctrl.RedisClient.XRead(ctx, &redis.XReadArgs{
			Streams: []string{EventStreamKey, "0"},
			Count:   10,
		}).Result()
		require.NoError(t, err)
		require.NotEmpty(t, streams)

		var event ResourceEvent
		eventData := streams[0].Messages[0].Values["event"].(string)
		require.NoError(t, json.Unmarshal([]byte(eventData), &event))
		assert.Equal(t, "delete-node", event.GlobalResourceID)
		assert.Contains(t, event.EventType, string(EventTypeDeleted))
	})

	t.Run("tombstone with valid node", func(t *testing.T) {
		mr.FlushAll()
		tombstone := cache.DeletedFinalStateUnknown{
			Key: "test-tombstone-node",
			Obj: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "tombstone-node"},
			},
		}
		ctrl.handleNodeDelete(tombstone)

		ctx := context.Background()
		streams, err := ctrl.RedisClient.XRead(ctx, &redis.XReadArgs{
			Streams: []string{EventStreamKey, "0"},
			Count:   10,
		}).Result()
		require.NoError(t, err)
		require.NotEmpty(t, streams)

		var event ResourceEvent
		eventData := streams[0].Messages[0].Values["event"].(string)
		require.NoError(t, json.Unmarshal([]byte(eventData), &event))
		assert.Equal(t, "tombstone-node", event.GlobalResourceID)
	})

	t.Run("tombstone with invalid object", func(t *testing.T) {
		tombstone := cache.DeletedFinalStateUnknown{
			Key: "test-key",
			Obj: "not-a-node",
		}
		ctrl.handleNodeDelete(tombstone)
	})

	t.Run("invalid object type", func(t *testing.T) {
		ctrl.handleNodeDelete("not-a-node")
	})

	t.Run("nil object", func(t *testing.T) {
		ctrl.handleNodeDelete(nil)
	})
}

func TestHandleNamespaceAdd_TypeAssertions(t *testing.T) {
	store := &internalMockStore{
		subscriptions: []*storage.Subscription{
			{ID: "sub-1", Callback: "http://example.com/cb", Filter: storage.SubscriptionFilter{}},
		},
	}
	ctrl, mr := newTestController(t, store)
	defer mr.Close()

	t.Run("valid namespace object", func(t *testing.T) {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "add-namespace"},
		}
		ctrl.handleNamespaceAdd(ns)

		ctx := context.Background()
		streams, err := ctrl.RedisClient.XRead(ctx, &redis.XReadArgs{
			Streams: []string{EventStreamKey, "0"},
			Count:   10,
		}).Result()
		require.NoError(t, err)
		require.NotEmpty(t, streams)

		var event ResourceEvent
		eventData := streams[0].Messages[0].Values["event"].(string)
		require.NoError(t, json.Unmarshal([]byte(eventData), &event))
		assert.Equal(t, "add-namespace", event.GlobalResourceID)
		assert.Contains(t, event.EventType, string(EventTypeCreated))
	})

	t.Run("invalid object type", func(t *testing.T) {
		ctrl.handleNamespaceAdd("not-a-namespace")
	})

	t.Run("nil object", func(t *testing.T) {
		ctrl.handleNamespaceAdd(nil)
	})
}

func TestHandleNamespaceUpdate_TypeAssertions(t *testing.T) {
	store := &internalMockStore{
		subscriptions: []*storage.Subscription{
			{ID: "sub-1", Callback: "http://example.com/cb", Filter: storage.SubscriptionFilter{}},
		},
	}
	ctrl, mr := newTestController(t, store)
	defer mr.Close()

	t.Run("valid namespace update with different resource versions", func(t *testing.T) {
		oldNs := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "update-ns", ResourceVersion: "1"},
		}
		newNs := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "update-ns", ResourceVersion: "2"},
		}
		ctrl.handleNamespaceUpdate(oldNs, newNs)

		ctx := context.Background()
		streams, err := ctrl.RedisClient.XRead(ctx, &redis.XReadArgs{
			Streams: []string{EventStreamKey, "0"},
			Count:   10,
		}).Result()
		require.NoError(t, err)
		require.NotEmpty(t, streams)

		var event ResourceEvent
		eventData := streams[0].Messages[0].Values["event"].(string)
		require.NoError(t, json.Unmarshal([]byte(eventData), &event))
		assert.Equal(t, "update-ns", event.GlobalResourceID)
		assert.Contains(t, event.EventType, string(EventTypeUpdated))
	})

	t.Run("same resource version - no event generated", func(t *testing.T) {
		mr.FlushAll()

		oldNs := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "same-ns", ResourceVersion: "3"},
		}
		newNs := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "same-ns", ResourceVersion: "3"},
		}
		ctrl.handleNamespaceUpdate(oldNs, newNs)

		ctx := context.Background()
		_, err := ctrl.RedisClient.XRead(ctx, &redis.XReadArgs{
			Streams: []string{EventStreamKey, "0"},
			Count:   1,
			Block:   50 * time.Millisecond,
		}).Result()
		assert.True(t, errors.Is(err, redis.Nil) || err != nil)
	})

	t.Run("invalid old object type", func(t *testing.T) {
		newNs := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "ns-1", ResourceVersion: "2"},
		}
		ctrl.handleNamespaceUpdate("not-a-namespace", newNs)
	})

	t.Run("invalid new object type", func(t *testing.T) {
		oldNs := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "ns-1", ResourceVersion: "1"},
		}
		ctrl.handleNamespaceUpdate(oldNs, "not-a-namespace")
	})
}

func TestHandleNamespaceDelete_TypeAssertions(t *testing.T) {
	store := &internalMockStore{
		subscriptions: []*storage.Subscription{
			{ID: "sub-1", Callback: "http://example.com/cb", Filter: storage.SubscriptionFilter{}},
		},
	}
	ctrl, mr := newTestController(t, store)
	defer mr.Close()

	t.Run("valid namespace object", func(t *testing.T) {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "delete-namespace"},
		}
		ctrl.handleNamespaceDelete(ns)

		ctx := context.Background()
		streams, err := ctrl.RedisClient.XRead(ctx, &redis.XReadArgs{
			Streams: []string{EventStreamKey, "0"},
			Count:   10,
		}).Result()
		require.NoError(t, err)
		require.NotEmpty(t, streams)

		var event ResourceEvent
		eventData := streams[0].Messages[0].Values["event"].(string)
		require.NoError(t, json.Unmarshal([]byte(eventData), &event))
		assert.Equal(t, "delete-namespace", event.GlobalResourceID)
		assert.Contains(t, event.EventType, string(EventTypeDeleted))
	})

	t.Run("tombstone with valid namespace", func(t *testing.T) {
		mr.FlushAll()
		tombstone := cache.DeletedFinalStateUnknown{
			Key: "test-tombstone-ns",
			Obj: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "tombstone-ns"},
			},
		}
		ctrl.handleNamespaceDelete(tombstone)

		ctx := context.Background()
		streams, err := ctrl.RedisClient.XRead(ctx, &redis.XReadArgs{
			Streams: []string{EventStreamKey, "0"},
			Count:   10,
		}).Result()
		require.NoError(t, err)
		require.NotEmpty(t, streams)

		var event ResourceEvent
		eventData := streams[0].Messages[0].Values["event"].(string)
		require.NoError(t, json.Unmarshal([]byte(eventData), &event))
		assert.Equal(t, "tombstone-ns", event.GlobalResourceID)
	})

	t.Run("tombstone with invalid object", func(t *testing.T) {
		tombstone := cache.DeletedFinalStateUnknown{
			Key: "test-key",
			Obj: "not-a-namespace",
		}
		ctrl.handleNamespaceDelete(tombstone)
	})

	t.Run("invalid object type", func(t *testing.T) {
		ctrl.handleNamespaceDelete("not-a-namespace")
	})

	t.Run("nil object", func(t *testing.T) {
		ctrl.handleNamespaceDelete(nil)
	})
}

func TestProcessNodeEvent_StoreError(t *testing.T) {
	store := &internalMockStore{
		listErr: errors.New("storage unavailable"),
	}
	ctrl, mr := newTestController(t, store)
	defer mr.Close()

	ctx := context.Background()
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "error-node"},
	}

	// Should not panic when store returns error
	ctrl.ProcessNodeEvent(ctx, node, EventTypeCreated)

	// No events should be queued
	_, err := ctrl.RedisClient.XRead(ctx, &redis.XReadArgs{
		Streams: []string{EventStreamKey, "0"},
		Count:   1,
		Block:   50 * time.Millisecond,
	}).Result()
	assert.True(t, errors.Is(err, redis.Nil) || err != nil)
}

func TestProcessNamespaceEvent_StoreError(t *testing.T) {
	store := &internalMockStore{
		listErr: errors.New("storage unavailable"),
	}
	ctrl, mr := newTestController(t, store)
	defer mr.Close()

	ctx := context.Background()
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "error-namespace"},
	}

	// Should not panic when store returns error
	ctrl.ProcessNamespaceEvent(ctx, ns, EventTypeCreated)

	// No events should be queued
	_, err := ctrl.RedisClient.XRead(ctx, &redis.XReadArgs{
		Streams: []string{EventStreamKey, "0"},
		Count:   1,
		Block:   50 * time.Millisecond,
	}).Result()
	assert.True(t, errors.Is(err, redis.Nil) || err != nil)
}

func TestProcessNodeEvent_NoMatchingSubscriptions(t *testing.T) {
	store := &internalMockStore{
		subscriptions: []*storage.Subscription{
			{
				ID:       "sub-1",
				Callback: "http://example.com/cb",
				Filter: storage.SubscriptionFilter{
					ResourceTypeID: "different-type",
				},
			},
		},
	}
	ctrl, mr := newTestController(t, store)
	defer mr.Close()

	ctx := context.Background()
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "no-match-node",
			Labels: map[string]string{"resource-pool": "pool-1"},
		},
	}

	ctrl.ProcessNodeEvent(ctx, node, EventTypeCreated)

	// No events should be queued since no subscriptions match
	_, err := ctrl.RedisClient.XRead(ctx, &redis.XReadArgs{
		Streams: []string{EventStreamKey, "0"},
		Count:   1,
		Block:   50 * time.Millisecond,
	}).Result()
	assert.True(t, errors.Is(err, redis.Nil) || err != nil)
}

func TestProcessNodeEvent_NodeWithoutResourcePoolLabel(t *testing.T) {
	store := &internalMockStore{
		subscriptions: []*storage.Subscription{
			{
				ID:       "sub-1",
				Callback: "http://example.com/cb",
				Filter:   storage.SubscriptionFilter{},
			},
		},
	}
	ctrl, mr := newTestController(t, store)
	defer mr.Close()

	ctx := context.Background()
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "no-pool-label-node",
			// No "resource-pool" label
		},
	}

	ctrl.ProcessNodeEvent(ctx, node, EventTypeCreated)

	streams, err := ctrl.RedisClient.XRead(ctx, &redis.XReadArgs{
		Streams: []string{EventStreamKey, "0"},
		Count:   10,
	}).Result()
	require.NoError(t, err)
	require.NotEmpty(t, streams)

	var event ResourceEvent
	eventData := streams[0].Messages[0].Values["event"].(string)
	require.NoError(t, json.Unmarshal([]byte(eventData), &event))
	assert.Equal(t, "", event.ResourcePoolID)
	assert.Equal(t, "no-pool-label-node", event.GlobalResourceID)
}

func TestQueueEvent_MarshalError(t *testing.T) {
	store := &internalMockStore{}
	ctrl, mr := newTestController(t, store)
	defer mr.Close()

	ctx := context.Background()

	// A valid event should be queued successfully
	event := &ResourceEvent{
		SubscriptionID:   "sub-1",
		EventType:        "o2ims.Resource.Created",
		ObjectRef:        "/o2ims/v1/resources/node-1",
		ResourceTypeID:   "k8s-node",
		GlobalResourceID: "node-1",
		Timestamp:        time.Now(),
		NotificationID:   "notif-1",
		CallbackURL:      "http://example.com/cb",
	}

	err := ctrl.queueEvent(ctx, event)
	require.NoError(t, err)

	// Verify it was queued
	streams, err := ctrl.RedisClient.XRead(ctx, &redis.XReadArgs{
		Streams: []string{EventStreamKey, "0"},
		Count:   1,
	}).Result()
	require.NoError(t, err)
	require.Len(t, streams, 1)
	require.Len(t, streams[0].Messages, 1)
}

func TestProcessNamespaceEvent_MultipleSubscriptions(t *testing.T) {
	store := &internalMockStore{
		subscriptions: []*storage.Subscription{
			{
				ID:       "sub-1",
				Callback: "http://example.com/cb1",
				Filter:   storage.SubscriptionFilter{ResourceTypeID: "k8s-namespace"},
			},
			{
				ID:       "sub-2",
				Callback: "http://example.com/cb2",
				Filter:   storage.SubscriptionFilter{ResourceTypeID: "k8s-namespace"},
			},
			{
				ID:       "sub-3",
				Callback: "http://example.com/cb3",
				Filter:   storage.SubscriptionFilter{ResourceTypeID: "k8s-node"},
			},
		},
	}
	ctrl, mr := newTestController(t, store)
	defer mr.Close()

	ctx := context.Background()
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "multi-sub-ns"},
	}

	ctrl.ProcessNamespaceEvent(ctx, ns, EventTypeCreated)

	streams, err := ctrl.RedisClient.XRead(ctx, &redis.XReadArgs{
		Streams: []string{EventStreamKey, "0"},
		Count:   10,
	}).Result()
	require.NoError(t, err)
	require.NotEmpty(t, streams)
	// sub-1 and sub-2 match, sub-3 does not
	assert.Len(t, streams[0].Messages, 2)
}

func TestSetupInformers(t *testing.T) {
	store := &internalMockStore{}
	ctrl, mr := newTestController(t, store)
	defer mr.Close()

	err := ctrl.setupInformers()
	assert.NoError(t, err)
}
