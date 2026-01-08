package events

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/storage"
)

// Processor orchestrates the event notification flow.
// It receives events from the generator, queues them, filters subscriptions,
// and delivers notifications to subscribers.
type Processor struct {
	generator       Generator
	queue           Queue
	filter          Filter
	notifier        Notifier
	deliveryTracker DeliveryTracker
	store           storage.Store
	logger          *zap.Logger
	workers         int
	wg              sync.WaitGroup
	stopChannel     chan struct{}
}

// ProcessorConfig holds configuration for the event processor.
type ProcessorConfig struct {
	// Workers is the number of concurrent notification delivery workers
	Workers int
}

// DefaultProcessorConfig returns a ProcessorConfig with sensible defaults.
func DefaultProcessorConfig() *ProcessorConfig {
	return &ProcessorConfig{
		Workers: 5,
	}
}

// NewProcessor creates a new event processor.
func NewProcessor(
	generator Generator,
	queue Queue,
	filter Filter,
	notifier Notifier,
	deliveryTracker DeliveryTracker,
	store storage.Store,
	logger *zap.Logger,
	config *ProcessorConfig,
) *Processor {
	if generator == nil {
		panic("generator cannot be nil")
	}
	if queue == nil {
		panic("queue cannot be nil")
	}
	if filter == nil {
		panic("filter cannot be nil")
	}
	if notifier == nil {
		panic("notifier cannot be nil")
	}
	if store == nil {
		panic("storage cannot be nil")
	}
	if logger == nil {
		panic("logger cannot be nil")
	}
	if config == nil {
		config = DefaultProcessorConfig()
	}

	return &Processor{
		generator:       generator,
		queue:           queue,
		filter:          filter,
		notifier:        notifier,
		deliveryTracker: deliveryTracker,
		store:           store,
		logger:          logger,
		workers:         config.Workers,
		stopChannel:     make(chan struct{}),
	}
}

// Start starts the event processor.
// It launches the event generator, queue consumers, and notification workers.
func (p *Processor) Start(ctx context.Context) error {
	p.logger.Info("starting event processor",
		zap.Int("workers", p.workers),
	)

	// Start event generator
	eventCh, err := p.generator.Start(ctx)
	if err != nil {
		return err
	}

	// Start event publisher
	p.wg.Add(1)
	go p.publishEvents(ctx, eventCh)

	// Start notification workers
	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go p.notificationWorker(ctx, i)
	}

	// Record active workers
	RecordNotificationWorkersActive(p.workers)

	return nil
}

// Stop gracefully stops the event processor.
// It waits for in-flight notifications to complete.
func (p *Processor) Stop() error {
	p.logger.Info("stopping event processor")

	// Signal shutdown
	close(p.stopChannel)

	// Stop generator
	if err := p.generator.Stop(); err != nil {
		p.logger.Error("failed to stop generator", zap.Error(err))
	}

	// Wait for workers to finish
	p.wg.Wait()

	// Close components
	if err := p.queue.Close(); err != nil {
		p.logger.Error("failed to close queue", zap.Error(err))
	}
	if err := p.notifier.Close(); err != nil {
		p.logger.Error("failed to close notifier", zap.Error(err))
	}

	p.logger.Info("event processor stopped")
	return nil
}

// publishEvents publishes events from the generator to the queue.
func (p *Processor) publishEvents(ctx context.Context, eventCh <-chan *Event) {
	defer p.wg.Done()

	p.logger.Info("starting event publisher")

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("event publisher stopped by context")
			return
		case <-p.stopChannel:
			p.logger.Info("event publisher stopped")
			return
		case event, ok := <-eventCh:
			if !ok {
				p.logger.Info("event channel closed")
				return
			}

			// Publish to queue
			if err := p.queue.Publish(ctx, event); err != nil {
				p.logger.Error("failed to publish event to queue",
					zap.Error(err),
					zap.String("event_id", event.ID),
				)
				continue
			}

			p.logger.Debug("event published to queue",
				zap.String("event_id", event.ID),
				zap.String("event_type", string(event.Type)),
			)
		}
	}
}

// notificationWorker processes events from the queue and delivers notifications.
func (p *Processor) notificationWorker(ctx context.Context, workerID int) {
	defer p.wg.Done()

	p.logger.Info("starting notification worker",
		zap.Int("worker_id", workerID),
	)

	// Subscribe to event queue
	eventCh, err := p.queue.Subscribe(ctx, "notifiers", fmt.Sprintf("worker-%d", workerID))
	if err != nil {
		p.logger.Error("failed to subscribe to event queue",
			zap.Error(err),
			zap.Int("worker_id", workerID),
		)
		return
	}

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("notification worker stopped by context",
				zap.Int("worker_id", workerID),
			)
			return
		case <-p.stopChannel:
			p.logger.Info("notification worker stopped",
				zap.Int("worker_id", workerID),
			)
			return
		case event, ok := <-eventCh:
			if !ok {
				p.logger.Info("event channel closed",
					zap.Int("worker_id", workerID),
				)
				return
			}

			// Process event
			if err := p.processEvent(ctx, event); err != nil {
				p.logger.Error("failed to process event",
					zap.Error(err),
					zap.String("event_id", event.ID),
					zap.Int("worker_id", workerID),
				)
			}
		}
	}
}

// processEvent processes a single event by filtering subscriptions and delivering notifications.
func (p *Processor) processEvent(ctx context.Context, event *Event) error {
	// Find matching subscriptions
	subscriptions, err := p.filter.MatchSubscriptions(ctx, event)
	if err != nil {
		return err
	}

	if len(subscriptions) == 0 {
		p.logger.Debug("no matching subscriptions for event",
			zap.String("event_id", event.ID),
		)
		return nil
	}

	p.logger.Info("processing event notifications",
		zap.String("event_id", event.ID),
		zap.String("event_type", string(event.Type)),
		zap.Int("subscription_count", len(subscriptions)),
	)

	// Deliver notifications to all matching subscriptions
	for _, subscription := range subscriptions {
		// Deliver with retry
		delivery, err := p.notifier.NotifyWithRetry(ctx, event, subscription)
		if err != nil {
			p.logger.Error("notification delivery failed",
				zap.Error(err),
				zap.String("event_id", event.ID),
				zap.String("subscription_id", subscription.ID),
			)
			continue
		}

		p.logger.Info("notification delivered",
			zap.String("delivery_id", delivery.ID),
			zap.String("event_id", event.ID),
			zap.String("subscription_id", subscription.ID),
			zap.String("status", string(delivery.Status)),
			zap.Int("attempts", delivery.Attempts),
		)
	}

	return nil
}
