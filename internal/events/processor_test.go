package events_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/piwi3910/netweave/internal/events"
	"github.com/piwi3910/netweave/internal/models"
	"github.com/piwi3910/netweave/internal/storage"
)

// --- Mock implementations for Processor tests ---

// mockGenerator implements events.Generator for testing.
type mockGenerator struct {
	eventCh  chan *events.Event
	startErr error
	stopErr  error
	stopped  bool
	mu       sync.Mutex
}

func newMockGenerator() *mockGenerator {
	return &mockGenerator{
		eventCh: make(chan *events.Event, 10),
	}
}

func (g *mockGenerator) Start(_ context.Context) (<-chan *events.Event, error) {
	if g.startErr != nil {
		return nil, g.startErr
	}
	return g.eventCh, nil
}

func (g *mockGenerator) Stop() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.stopped {
		return g.stopErr
	}
	g.stopped = true
	if g.stopErr != nil {
		return g.stopErr
	}
	close(g.eventCh)
	return nil
}

// mockQueue implements events.Queue for testing.
type mockQueue struct {
	publishedEvents []*events.Event
	subscribeCh     chan *events.Event
	publishErr      error
	subscribeErr    error
	closeErr        error
	mu              sync.Mutex
}

func newMockQueue() *mockQueue {
	return &mockQueue{
		publishedEvents: make([]*events.Event, 0),
		subscribeCh:     make(chan *events.Event, 10),
	}
}

func (q *mockQueue) Publish(_ context.Context, event *events.Event) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.publishErr != nil {
		return q.publishErr
	}
	q.publishedEvents = append(q.publishedEvents, event)
	// Also forward to subscribe channel for workers to pick up
	select {
	case q.subscribeCh <- event:
	default:
	}
	return nil
}

func (q *mockQueue) Subscribe(_ context.Context, _, _ string) (<-chan *events.Event, error) {
	if q.subscribeErr != nil {
		return nil, q.subscribeErr
	}
	return q.subscribeCh, nil
}

func (q *mockQueue) Acknowledge(_ context.Context, _, _ string) error {
	return nil
}

func (q *mockQueue) Close() error {
	if q.closeErr != nil {
		return q.closeErr
	}
	return nil
}

// mockFilter implements events.Filter for testing.
type mockFilter struct {
	subscriptions []*storage.Subscription
	matchErr      error
}

func (f *mockFilter) MatchSubscriptions(_ context.Context, _ *events.Event) ([]*storage.Subscription, error) {
	if f.matchErr != nil {
		return nil, f.matchErr
	}
	return f.subscriptions, nil
}

// mockNotifier implements events.Notifier for testing.
type mockNotifier struct {
	notifyErr  error
	retryErr   error
	deliveries []*events.NotificationDelivery
	closeErr   error
	mu         sync.Mutex
}

func (n *mockNotifier) Notify(_ context.Context, _ *events.Event, _ *storage.Subscription) error {
	return n.notifyErr
}

func (n *mockNotifier) NotifyWithRetry(_ context.Context, _ *events.Event, _ *storage.Subscription) (*events.NotificationDelivery, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.retryErr != nil {
		return nil, n.retryErr
	}
	delivery := &events.NotificationDelivery{
		ID:     "delivery-test",
		Status: events.DeliveryStatusDelivered,
	}
	n.deliveries = append(n.deliveries, delivery)
	return delivery, nil
}

func (n *mockNotifier) Close() error {
	return n.closeErr
}

func TestDefaultProcessorConfig(t *testing.T) {
	cfg := events.DefaultProcessorConfig()
	require.NotNil(t, cfg)
	assert.Equal(t, 5, cfg.Workers)
}

func TestNewProcessor(t *testing.T) {
	logger := zaptest.NewLogger(t)
	gen := newMockGenerator()
	queue := newMockQueue()
	filter := &mockFilter{}
	notifier := &mockNotifier{}
	tracker := &mockDeliveryTracker{}
	store := &mockStore{}

	t.Run("valid configuration", func(t *testing.T) {
		p := events.NewProcessor(gen, queue, filter, notifier, tracker, store, logger, nil)
		assert.NotNil(t, p)
	})

	t.Run("custom config", func(t *testing.T) {
		cfg := &events.ProcessorConfig{Workers: 3}
		p := events.NewProcessor(gen, queue, filter, notifier, tracker, store, logger, cfg)
		assert.NotNil(t, p)
	})

	t.Run("nil generator panics", func(t *testing.T) {
		assert.Panics(t, func() {
			events.NewProcessor(nil, queue, filter, notifier, tracker, store, logger, nil)
		})
	})

	t.Run("nil queue panics", func(t *testing.T) {
		assert.Panics(t, func() {
			events.NewProcessor(gen, nil, filter, notifier, tracker, store, logger, nil)
		})
	})

	t.Run("nil filter panics", func(t *testing.T) {
		assert.Panics(t, func() {
			events.NewProcessor(gen, queue, nil, notifier, tracker, store, logger, nil)
		})
	})

	t.Run("nil notifier panics", func(t *testing.T) {
		assert.Panics(t, func() {
			events.NewProcessor(gen, queue, filter, nil, tracker, store, logger, nil)
		})
	})

	t.Run("nil store panics", func(t *testing.T) {
		assert.Panics(t, func() {
			events.NewProcessor(gen, queue, filter, notifier, tracker, nil, logger, nil)
		})
	})

	t.Run("nil logger panics", func(t *testing.T) {
		assert.Panics(t, func() {
			events.NewProcessor(gen, queue, filter, notifier, tracker, store, nil, nil)
		})
	})
}

func TestProcessor_StartAndStop(t *testing.T) {
	logger := zaptest.NewLogger(t)
	gen := newMockGenerator()
	queue := newMockQueue()
	filter := &mockFilter{}
	notifier := &mockNotifier{}
	tracker := &mockDeliveryTracker{}
	store := &mockStore{}

	t.Run("start and stop successfully", func(t *testing.T) {
		p := events.NewProcessor(gen, queue, filter, notifier, tracker, store, logger, &events.ProcessorConfig{Workers: 1})

		ctx, cancel := context.WithCancel(context.Background())

		err := p.Start(ctx)
		require.NoError(t, err)

		// Allow some time for goroutines to start
		time.Sleep(50 * time.Millisecond)

		cancel()
		err = p.Stop()
		assert.NoError(t, err)
	})

	t.Run("start fails when generator fails", func(t *testing.T) {
		failGen := &mockGenerator{
			eventCh:  make(chan *events.Event, 1),
			startErr: errors.New("generator start failed"),
		}
		p := events.NewProcessor(failGen, queue, filter, notifier, tracker, store, logger, nil)

		ctx := context.Background()
		err := p.Start(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to start event generator")
	})
}

func TestProcessor_PublishEvents(t *testing.T) {
	logger := zaptest.NewLogger(t)

	t.Run("publishes events from generator to queue", func(t *testing.T) {
		gen := newMockGenerator()
		queue := newMockQueue()
		filter := &mockFilter{}
		notifier := &mockNotifier{}
		tracker := &mockDeliveryTracker{}
		store := &mockStore{}

		p := events.NewProcessor(gen, queue, filter, notifier, tracker, store, logger, &events.ProcessorConfig{Workers: 0})

		ctx, cancel := context.WithCancel(context.Background())

		err := p.Start(ctx)
		require.NoError(t, err)

		// Send an event to the generator
		testEvent := &events.Event{
			ID:           "test-event-1",
			Type:         models.EventTypeResourceCreated,
			ResourceType: events.ResourceTypeResource,
			ResourceID:   "resource-1",
			Timestamp:    time.Now().UTC(),
		}
		gen.eventCh <- testEvent

		// Wait for the event to be published
		time.Sleep(100 * time.Millisecond)

		queue.mu.Lock()
		publishedCount := len(queue.publishedEvents)
		queue.mu.Unlock()

		assert.Equal(t, 1, publishedCount)

		cancel()
		err = p.Stop()
		assert.NoError(t, err)
	})

	t.Run("handles publish error gracefully", func(t *testing.T) {
		gen := newMockGenerator()
		queue := newMockQueue()
		queue.publishErr = errors.New("publish failed")
		filter := &mockFilter{}
		notifier := &mockNotifier{}
		tracker := &mockDeliveryTracker{}
		store := &mockStore{}

		p := events.NewProcessor(gen, queue, filter, notifier, tracker, store, logger, &events.ProcessorConfig{Workers: 0})

		ctx, cancel := context.WithCancel(context.Background())

		err := p.Start(ctx)
		require.NoError(t, err)

		// Send an event - should not panic even though publish fails
		gen.eventCh <- &events.Event{
			ID:           "fail-event",
			Type:         models.EventTypeResourceCreated,
			ResourceType: events.ResourceTypeResource,
			ResourceID:   "resource-1",
			Timestamp:    time.Now().UTC(),
		}

		time.Sleep(100 * time.Millisecond)

		cancel()
		err = p.Stop()
		assert.NoError(t, err)
	})
}

func TestProcessor_NotificationWorker(t *testing.T) {
	logger := zaptest.NewLogger(t)

	t.Run("processes events and delivers notifications", func(t *testing.T) {
		gen := newMockGenerator()
		queue := newMockQueue()
		sub := &storage.Subscription{
			ID:       "sub-1",
			Callback: "http://example.com/cb",
		}
		filter := &mockFilter{
			subscriptions: []*storage.Subscription{sub},
		}
		notifier := &mockNotifier{}
		tracker := &mockDeliveryTracker{}
		store := &mockStore{}

		p := events.NewProcessor(gen, queue, filter, notifier, tracker, store, logger, &events.ProcessorConfig{Workers: 1})

		ctx, cancel := context.WithCancel(context.Background())

		err := p.Start(ctx)
		require.NoError(t, err)

		// Send an event directly to the queue subscribe channel
		testEvent := &events.Event{
			ID:           "worker-event-1",
			Type:         models.EventTypeResourceUpdated,
			ResourceType: events.ResourceTypeResource,
			ResourceID:   "resource-1",
			Timestamp:    time.Now().UTC(),
		}
		queue.subscribeCh <- testEvent

		// Wait for processing
		time.Sleep(200 * time.Millisecond)

		notifier.mu.Lock()
		deliveryCount := len(notifier.deliveries)
		notifier.mu.Unlock()

		assert.Equal(t, 1, deliveryCount)

		cancel()
		err = p.Stop()
		assert.NoError(t, err)
	})

	t.Run("handles no matching subscriptions", func(t *testing.T) {
		gen := newMockGenerator()
		queue := newMockQueue()
		filter := &mockFilter{
			subscriptions: []*storage.Subscription{}, // no matches
		}
		notifier := &mockNotifier{}
		tracker := &mockDeliveryTracker{}
		store := &mockStore{}

		p := events.NewProcessor(gen, queue, filter, notifier, tracker, store, logger, &events.ProcessorConfig{Workers: 1})

		ctx, cancel := context.WithCancel(context.Background())

		err := p.Start(ctx)
		require.NoError(t, err)

		queue.subscribeCh <- &events.Event{
			ID:           "no-match-event",
			Type:         models.EventTypeResourceCreated,
			ResourceType: events.ResourceTypeResource,
			ResourceID:   "resource-1",
			Timestamp:    time.Now().UTC(),
		}

		time.Sleep(200 * time.Millisecond)

		notifier.mu.Lock()
		deliveryCount := len(notifier.deliveries)
		notifier.mu.Unlock()

		assert.Equal(t, 0, deliveryCount)

		cancel()
		err = p.Stop()
		assert.NoError(t, err)
	})

	t.Run("handles filter error gracefully", func(t *testing.T) {
		gen := newMockGenerator()
		queue := newMockQueue()
		filter := &mockFilter{
			matchErr: errors.New("filter error"),
		}
		notifier := &mockNotifier{}
		tracker := &mockDeliveryTracker{}
		store := &mockStore{}

		p := events.NewProcessor(gen, queue, filter, notifier, tracker, store, logger, &events.ProcessorConfig{Workers: 1})

		ctx, cancel := context.WithCancel(context.Background())

		err := p.Start(ctx)
		require.NoError(t, err)

		queue.subscribeCh <- &events.Event{
			ID:           "filter-error-event",
			Type:         models.EventTypeResourceCreated,
			ResourceType: events.ResourceTypeResource,
			ResourceID:   "resource-1",
			Timestamp:    time.Now().UTC(),
		}

		time.Sleep(200 * time.Millisecond)

		cancel()
		err = p.Stop()
		assert.NoError(t, err)
	})

	t.Run("handles notification delivery error gracefully", func(t *testing.T) {
		gen := newMockGenerator()
		queue := newMockQueue()
		sub := &storage.Subscription{
			ID:       "sub-err",
			Callback: "http://example.com/cb",
		}
		filter := &mockFilter{
			subscriptions: []*storage.Subscription{sub},
		}
		notifier := &mockNotifier{
			retryErr: errors.New("delivery failed"),
		}
		tracker := &mockDeliveryTracker{}
		store := &mockStore{}

		p := events.NewProcessor(gen, queue, filter, notifier, tracker, store, logger, &events.ProcessorConfig{Workers: 1})

		ctx, cancel := context.WithCancel(context.Background())

		err := p.Start(ctx)
		require.NoError(t, err)

		queue.subscribeCh <- &events.Event{
			ID:           "delivery-error-event",
			Type:         models.EventTypeResourceDeleted,
			ResourceType: events.ResourceTypeResource,
			ResourceID:   "resource-1",
			Timestamp:    time.Now().UTC(),
		}

		time.Sleep(200 * time.Millisecond)

		cancel()
		err = p.Stop()
		assert.NoError(t, err)
	})

	t.Run("handles subscribe error for worker", func(t *testing.T) {
		gen := newMockGenerator()
		queue := newMockQueue()
		queue.subscribeErr = errors.New("subscribe failed")
		filter := &mockFilter{}
		notifier := &mockNotifier{}
		tracker := &mockDeliveryTracker{}
		store := &mockStore{}

		p := events.NewProcessor(gen, queue, filter, notifier, tracker, store, logger, &events.ProcessorConfig{Workers: 1})

		ctx, cancel := context.WithCancel(context.Background())

		err := p.Start(ctx)
		require.NoError(t, err)

		// Worker should handle the subscribe error gracefully
		time.Sleep(100 * time.Millisecond)

		cancel()
		err = p.Stop()
		assert.NoError(t, err)
	})
}

func TestProcessor_PublishEvents_ChannelClosed(t *testing.T) {
	logger := zaptest.NewLogger(t)
	gen := &mockGenerator{
		eventCh: make(chan *events.Event, 10),
		stopErr: nil,
	}
	queue := newMockQueue()
	filter := &mockFilter{}
	notifier := &mockNotifier{}
	tracker := &mockDeliveryTracker{}
	store := &mockStore{}

	// Use 0 workers so only the publisher goroutine runs
	p := events.NewProcessor(gen, queue, filter, notifier, tracker, store, logger, &events.ProcessorConfig{Workers: 0})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := p.Start(ctx)
	require.NoError(t, err)

	// Close the generator channel to trigger the "channel closed" path in publishEvents.
	// Mark the generator as already stopped so Stop() doesn't try to close it again.
	gen.mu.Lock()
	gen.stopped = true
	gen.mu.Unlock()
	close(gen.eventCh)

	// Wait for publisher to detect closed channel and exit
	time.Sleep(100 * time.Millisecond)

	err = p.Stop()
	assert.NoError(t, err)
}

func TestProcessor_NotificationWorker_ChannelClosed(t *testing.T) {
	logger := zaptest.NewLogger(t)
	gen := newMockGenerator()
	queue := newMockQueue()
	sub := &storage.Subscription{
		ID:       "sub-1",
		Callback: "http://example.com/cb",
	}
	filter := &mockFilter{
		subscriptions: []*storage.Subscription{sub},
	}
	notifier := &mockNotifier{}
	tracker := &mockDeliveryTracker{}
	store := &mockStore{}

	p := events.NewProcessor(gen, queue, filter, notifier, tracker, store, logger, &events.ProcessorConfig{Workers: 1})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := p.Start(ctx)
	require.NoError(t, err)

	// Wait for worker to start
	time.Sleep(50 * time.Millisecond)

	// Close the subscribe channel to trigger the "channel closed" path in notificationWorker
	close(queue.subscribeCh)

	// Wait for worker to detect closed channel
	time.Sleep(100 * time.Millisecond)

	err = p.Stop()
	assert.NoError(t, err)
}

func TestProcessor_StopWithErrors(t *testing.T) {
	logger := zaptest.NewLogger(t)
	gen := newMockGenerator()
	gen.stopErr = errors.New("generator stop error")
	queue := newMockQueue()
	queue.closeErr = errors.New("queue close error")
	filter := &mockFilter{}
	notifier := &mockNotifier{closeErr: errors.New("notifier close error")}
	tracker := &mockDeliveryTracker{}
	store := &mockStore{}

	p := events.NewProcessor(gen, queue, filter, notifier, tracker, store, logger, &events.ProcessorConfig{Workers: 0})

	ctx, cancel := context.WithCancel(context.Background())

	err := p.Start(ctx)
	require.NoError(t, err)

	cancel()
	time.Sleep(50 * time.Millisecond)

	// Stop should still return nil, errors are just logged
	err = p.Stop()
	assert.NoError(t, err)
}
