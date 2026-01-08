package events

import (
	"context"

	"github.com/piwi3910/netweave/internal/storage"
)

// Generator defines the interface for event generation from resource changes.
// Implementations watch backend resources and generate events when changes occur.
type Generator interface {
	// Start begins watching for resource changes and generating events.
	// Returns a channel that receives generated events.
	// The context is used for cancellation.
	Start(ctx context.Context) (<-chan *Event, error)

	// Stop stops the event generator and releases resources.
	Stop() error
}

// Queue defines the interface for event queuing and distribution.
// Implementations provide reliable, persistent event storage using Redis Streams.
type Queue interface {
	// Publish adds an event to the queue for processing.
	// Returns an error if the event cannot be queued.
	Publish(ctx context.Context, event *Event) error

	// Subscribe returns a channel that receives events from the queue.
	// The consumer group name is used for load distribution across multiple workers.
	// The context is used for cancellation.
	Subscribe(ctx context.Context, consumerGroup, consumerName string) (<-chan *Event, error)

	// Acknowledge marks an event as successfully processed.
	// This removes it from the pending list.
	Acknowledge(ctx context.Context, consumerGroup, eventID string) error

	// Close closes the queue connection and releases resources.
	Close() error
}

// Filter defines the interface for event filtering based on subscription criteria.
// Implementations determine which subscriptions should receive which events.
type Filter interface {
	// MatchSubscriptions returns the subscriptions that should receive the event.
	// Filtering is based on subscription filter criteria (resource pool, type, ID, labels).
	MatchSubscriptions(ctx context.Context, event *Event) ([]*storage.Subscription, error)
}

// Notifier defines the interface for webhook notification delivery.
// Implementations handle HTTP POST requests to subscriber callback URLs.
type Notifier interface {
	// Notify sends a notification to a subscriber's callback URL.
	// Returns an error if delivery fails.
	// The context is used for timeout control.
	Notify(ctx context.Context, event *Event, subscription *storage.Subscription) error

	// NotifyWithRetry sends a notification with automatic retry logic.
	// Uses exponential backoff for retries.
	// Returns the delivery tracking information.
	NotifyWithRetry(ctx context.Context, event *Event, subscription *storage.Subscription) (*NotificationDelivery, error)

	// Close closes the notifier and releases resources.
	Close() error
}

// DeliveryTracker defines the interface for tracking notification delivery status.
// Implementations store delivery attempts and status for monitoring and debugging.
type DeliveryTracker interface {
	// Track records a delivery attempt.
	Track(ctx context.Context, delivery *NotificationDelivery) error

	// Get retrieves delivery information by ID.
	Get(ctx context.Context, deliveryID string) (*NotificationDelivery, error)

	// ListByEvent retrieves all deliveries for a specific event.
	ListByEvent(ctx context.Context, eventID string) ([]*NotificationDelivery, error)

	// ListBySubscription retrieves all deliveries for a specific subscription.
	ListBySubscription(ctx context.Context, subscriptionID string) ([]*NotificationDelivery, error)

	// ListFailed retrieves all failed deliveries that need attention.
	ListFailed(ctx context.Context) ([]*NotificationDelivery, error)
}
