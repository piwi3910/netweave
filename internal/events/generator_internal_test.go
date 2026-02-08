package events

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/models"
)

// testAdapter is a minimal adapter implementation for generator tests.
type testAdapter struct {
	adapter.Adapter
	getResourceFn func(ctx context.Context, id string) (*adapter.Resource, error)
}

func (a *testAdapter) Name() string                       { return "test" }
func (a *testAdapter) Version() string                    { return "1.0" }
func (a *testAdapter) Capabilities() []adapter.Capability { return nil }
func (a *testAdapter) Health(_ context.Context) error     { return nil }
func (a *testAdapter) Close() error                       { return nil }

func (a *testAdapter) GetResource(ctx context.Context, id string) (*adapter.Resource, error) {
	if a.getResourceFn != nil {
		return a.getResourceFn(ctx, id)
	}
	return &adapter.Resource{
		ResourceID:     id,
		ResourceTypeID: "compute-node",
		ResourcePoolID: "pool-1",
	}, nil
}

func TestMapWatchEventType(t *testing.T) {
	logger := zaptest.NewLogger(t)
	adp := &testAdapter{}
	// We cannot create a real clientset without a cluster, but we only need it
	// for the nil check - we won't call Start/watchNodes.
	// Instead we test the mapWatchEventType method directly since it's on the struct.

	gen := &K8sEventGenerator{
		adapter:      adp,
		logger:       logger,
		eventChannel: make(chan *Event, 10),
		stopChannel:  make(chan struct{}),
	}

	tests := []struct {
		name      string
		watchType watch.EventType
		wantType  models.EventType
		wantSkip  bool
	}{
		{
			name:      "added event",
			watchType: watch.Added,
			wantType:  models.EventTypeResourceCreated,
			wantSkip:  false,
		},
		{
			name:      "modified event",
			watchType: watch.Modified,
			wantType:  models.EventTypeResourceUpdated,
			wantSkip:  false,
		},
		{
			name:      "deleted event",
			watchType: watch.Deleted,
			wantType:  models.EventTypeResourceDeleted,
			wantSkip:  false,
		},
		{
			name:      "bookmark event is skipped",
			watchType: watch.Bookmark,
			wantType:  "",
			wantSkip:  true,
		},
		{
			name:      "error event is skipped",
			watchType: watch.Error,
			wantType:  "",
			wantSkip:  true,
		},
		{
			name:      "unknown event type is skipped",
			watchType: watch.EventType("Unknown"),
			wantType:  "",
			wantSkip:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eventType, shouldSkip := gen.mapWatchEventType(tt.watchType)
			assert.Equal(t, tt.wantSkip, shouldSkip)
			if !shouldSkip {
				assert.Equal(t, tt.wantType, eventType)
			}
		})
	}
}

func TestCreateDeletedResource(t *testing.T) {
	logger := zaptest.NewLogger(t)
	adp := &testAdapter{}

	gen := &K8sEventGenerator{
		adapter:      adp,
		logger:       logger,
		eventChannel: make(chan *Event, 10),
		stopChannel:  make(chan struct{}),
	}

	tests := []struct {
		name string
		node *corev1.Node
	}{
		{
			name: "node with labels",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "deleted-node-1",
					Labels: map[string]string{
						"env":  "production",
						"role": "worker",
					},
				},
			},
		},
		{
			name: "node without labels",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "deleted-node-2",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource := gen.createDeletedResource(tt.node)
			require.NotNil(t, resource)
			assert.Equal(t, tt.node.Name, resource.ResourceID)
			assert.Equal(t, "compute-node", resource.ResourceTypeID)
			assert.NotNil(t, resource.Extensions)
			assert.Equal(t, tt.node.Name, resource.Extensions["nodeName"])
		})
	}
}

func TestBuildEvent(t *testing.T) {
	logger := zaptest.NewLogger(t)
	adp := &testAdapter{}

	gen := &K8sEventGenerator{
		adapter:      adp,
		logger:       logger,
		eventChannel: make(chan *Event, 10),
		stopChannel:  make(chan struct{}),
	}

	resource := &adapter.Resource{
		ResourceID:     "node-1",
		ResourceTypeID: "compute-node",
		ResourcePoolID: "pool-1",
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-1",
			Labels: map[string]string{
				"env": "test",
			},
		},
	}

	event := gen.buildEvent(models.EventTypeResourceCreated, resource, node)
	require.NotNil(t, event)
	assert.NotEmpty(t, event.ID)
	assert.Equal(t, models.EventTypeResourceCreated, event.Type)
	assert.Equal(t, ResourceTypeResource, event.ResourceType)
	assert.Equal(t, "node-1", event.ResourceID)
	assert.Equal(t, "pool-1", event.ResourcePoolID)
	assert.Equal(t, "compute-node", event.ResourceTypeID)
	assert.Equal(t, resource, event.Resource)
	assert.Equal(t, node.Labels, event.Labels)
	assert.False(t, event.Timestamp.IsZero())
}

func TestSendEvent(t *testing.T) {
	logger := zaptest.NewLogger(t)
	adp := &testAdapter{}

	t.Run("sends event to channel successfully", func(t *testing.T) {
		gen := &K8sEventGenerator{
			adapter:      adp,
			logger:       logger,
			eventChannel: make(chan *Event, 10),
			stopChannel:  make(chan struct{}),
		}

		event := &Event{
			ID:           "test-event-1",
			Type:         models.EventTypeResourceCreated,
			ResourceType: ResourceTypeResource,
			ResourceID:   "resource-1",
			Timestamp:    time.Now().UTC(),
		}

		ctx := context.Background()
		err := gen.sendEvent(ctx, event)
		require.NoError(t, err)

		// Read the event from the channel
		select {
		case received := <-gen.eventChannel:
			assert.Equal(t, event.ID, received.ID)
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for event")
		}
	})

	t.Run("drops event when channel is full", func(t *testing.T) {
		gen := &K8sEventGenerator{
			adapter:      adp,
			logger:       logger,
			eventChannel: make(chan *Event, 1), // Small buffer
			stopChannel:  make(chan struct{}),
		}

		// Fill the channel
		gen.eventChannel <- &Event{ID: "filler"}

		event := &Event{
			ID:           "overflow-event",
			Type:         models.EventTypeResourceCreated,
			ResourceType: ResourceTypeResource,
			ResourceID:   "resource-2",
			Timestamp:    time.Now().UTC(),
		}

		ctx := context.Background()
		err := gen.sendEvent(ctx, event)
		// Should not error - just drops the event
		assert.NoError(t, err)
	})

	t.Run("returns error when context is canceled", func(t *testing.T) {
		gen := &K8sEventGenerator{
			adapter:      adp,
			logger:       logger,
			eventChannel: make(chan *Event), // Unbuffered - blocks
			stopChannel:  make(chan struct{}),
		}

		event := &Event{
			ID:           "canceled-event",
			Type:         models.EventTypeResourceCreated,
			ResourceType: ResourceTypeResource,
			ResourceID:   "resource-3",
			Timestamp:    time.Now().UTC(),
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		err := gen.sendEvent(ctx, event)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "canceled")
	})
}

func TestGetResourceForNode(t *testing.T) {
	logger := zaptest.NewLogger(t)

	t.Run("adapter returns resource successfully", func(t *testing.T) {
		adp := &testAdapter{
			getResourceFn: func(_ context.Context, id string) (*adapter.Resource, error) {
				return &adapter.Resource{
					ResourceID:     id,
					ResourceTypeID: "compute-node",
					ResourcePoolID: "pool-1",
				}, nil
			},
		}

		gen := &K8sEventGenerator{
			adapter:      adp,
			logger:       logger,
			eventChannel: make(chan *Event, 10),
			stopChannel:  make(chan struct{}),
		}

		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
		}

		ctx := context.Background()
		resource, err := gen.getResourceForNode(ctx, node, watch.Added)
		require.NoError(t, err)
		assert.Equal(t, "test-node", resource.ResourceID)
	})

	t.Run("adapter error on deleted node returns deleted resource", func(t *testing.T) {
		adp := &testAdapter{
			getResourceFn: func(_ context.Context, _ string) (*adapter.Resource, error) {
				return nil, adapter.ErrResourceNotFound
			},
		}

		gen := &K8sEventGenerator{
			adapter:      adp,
			logger:       logger,
			eventChannel: make(chan *Event, 10),
			stopChannel:  make(chan struct{}),
		}

		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "deleted-node"},
		}

		ctx := context.Background()
		resource, err := gen.getResourceForNode(ctx, node, watch.Deleted)
		require.NoError(t, err)
		assert.Equal(t, "deleted-node", resource.ResourceID)
		assert.Equal(t, "compute-node", resource.ResourceTypeID)
	})

	t.Run("adapter error on non-deleted node returns error", func(t *testing.T) {
		adp := &testAdapter{
			getResourceFn: func(_ context.Context, _ string) (*adapter.Resource, error) {
				return nil, adapter.ErrResourceNotFound
			},
		}

		gen := &K8sEventGenerator{
			adapter:      adp,
			logger:       logger,
			eventChannel: make(chan *Event, 10),
			stopChannel:  make(chan struct{}),
		}

		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "missing-node"},
		}

		ctx := context.Background()
		_, err := gen.getResourceForNode(ctx, node, watch.Added)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get resource from adapter")
	})
}

func TestHandleNodeEvent(t *testing.T) {
	logger := zaptest.NewLogger(t)

	t.Run("handles added node event", func(t *testing.T) {
		adp := &testAdapter{}
		gen := &K8sEventGenerator{
			adapter:      adp,
			logger:       logger,
			eventChannel: make(chan *Event, 10),
			stopChannel:  make(chan struct{}),
		}

		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "handled-node",
				Labels: map[string]string{"env": "test"},
			},
		}

		ctx := context.Background()
		watchEvent := watch.Event{
			Type:   watch.Added,
			Object: node,
		}

		err := gen.handleNodeEvent(ctx, watchEvent)
		require.NoError(t, err)

		select {
		case event := <-gen.eventChannel:
			assert.Equal(t, models.EventTypeResourceCreated, event.Type)
			assert.Equal(t, "handled-node", event.ResourceID)
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for event")
		}
	})

	t.Run("skips bookmark events", func(t *testing.T) {
		adp := &testAdapter{}
		gen := &K8sEventGenerator{
			adapter:      adp,
			logger:       logger,
			eventChannel: make(chan *Event, 10),
			stopChannel:  make(chan struct{}),
		}

		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "bookmark-node"},
		}

		ctx := context.Background()
		watchEvent := watch.Event{
			Type:   watch.Bookmark,
			Object: node,
		}

		err := gen.handleNodeEvent(ctx, watchEvent)
		require.NoError(t, err)

		// No event should be generated
		select {
		case <-gen.eventChannel:
			t.Fatal("unexpected event for bookmark")
		case <-time.After(50 * time.Millisecond):
			// Expected - no event generated
		}
	})

	t.Run("returns error when adapter fails for non-deleted node", func(t *testing.T) {
		adp := &testAdapter{
			getResourceFn: func(_ context.Context, _ string) (*adapter.Resource, error) {
				return nil, adapter.ErrResourceNotFound
			},
		}
		gen := &K8sEventGenerator{
			adapter:      adp,
			logger:       logger,
			eventChannel: make(chan *Event, 10),
			stopChannel:  make(chan struct{}),
		}

		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "error-node",
			},
		}

		ctx := context.Background()
		watchEvent := watch.Event{
			Type:   watch.Added,
			Object: node,
		}

		err := gen.handleNodeEvent(ctx, watchEvent)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get resource from adapter")
	})

	t.Run("returns error for non-node object", func(t *testing.T) {
		adp := &testAdapter{}
		gen := &K8sEventGenerator{
			adapter:      adp,
			logger:       logger,
			eventChannel: make(chan *Event, 10),
			stopChannel:  make(chan struct{}),
		}

		// Use a Pod instead of a Node
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "not-a-node"},
		}

		ctx := context.Background()
		watchEvent := watch.Event{
			Type:   watch.Added,
			Object: pod,
		}

		err := gen.handleNodeEvent(ctx, watchEvent)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a Node")
	})
}

func TestStartAndStop(t *testing.T) {
	logger := zaptest.NewLogger(t)
	adp := &testAdapter{}

	// Note: We can't use a real clientset here, but we can test the Start/Stop
	// flow with the internal struct fields.
	gen := &K8sEventGenerator{
		adapter:      adp,
		logger:       logger,
		eventChannel: make(chan *Event, 100),
		stopChannel:  make(chan struct{}),
	}

	// Test Stop
	err := gen.Stop()
	assert.NoError(t, err)
}
