// Package e2e provides end-to-end testing framework.
//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"
)

// WebhookEvent represents a notification received from the gateway.
type WebhookEvent struct {
	// ID is the unique event identifier
	ID string `json:"id"`

	// Type is the event type (e.g., "resource.created", "resource.updated")
	Type string `json:"type"`

	// Timestamp is when the event occurred
	Timestamp time.Time `json:"timestamp"`

	// ResourceType is the type of resource (e.g., "Node", "Namespace")
	ResourceType string `json:"resourceType"`

	// ResourceID is the unique identifier of the resource
	ResourceID string `json:"resourceId"`

	// Data contains the event-specific data
	Data map[string]any `json:"data,omitempty"`

	// SubscriptionID is the ID of the subscription that triggered this event
	SubscriptionID string `json:"subscriptionId"`
}

// WebhookServer is a mock HTTP server for receiving subscription notifications.
type WebhookServer struct {
	server   *http.Server
	listener net.Listener
	logger   *zap.Logger

	// Events channel receives all webhook notifications
	events chan *WebhookEvent

	// mu protects the fields below
	mu sync.RWMutex

	// receivedEvents stores all events for inspection
	receivedEvents []*WebhookEvent

	// port is the server port
	port int

	// started indicates if the server is running
	started bool
}

// NewWebhookServer creates a new mock webhook server.
func NewWebhookServer(logger *zap.Logger) *WebhookServer {
	return &WebhookServer{
		logger:         logger,
		events:         make(chan *WebhookEvent, 100),
		receivedEvents: make([]*WebhookEvent, 0),
	}
}

// Start starts the webhook server on an available port.
func (ws *WebhookServer) Start() error {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	if ws.started {
		return fmt.Errorf("webhook server already started")
	}

	// Create listener on random available port
	lc := net.ListenConfig{}
	listener, err := lc.Listen(context.Background(), "tcp", "localhost:0")
	if err != nil {
		return fmt.Errorf("failed to create listener: %w", err)
	}

	ws.listener = listener
	ws.port = listener.Addr().(*net.TCPAddr).Port

	// Create HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", ws.handleWebhook)
	mux.HandleFunc("/health", ws.handleHealth)

	ws.server = &http.Server{
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// Start server in background
	go func() {
		if err := ws.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			ws.logger.Error("Webhook server error", zap.Error(err))
		}
	}()

	ws.started = true
	ws.logger.Info("Webhook server started", zap.Int("port", ws.port))

	return nil
}

// Stop stops the webhook server.
func (ws *WebhookServer) Stop() error {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	if !ws.started {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := ws.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown webhook server: %w", err)
	}

	ws.started = false
	close(ws.events)
	ws.logger.Info("Webhook server stopped")

	return nil
}

// URL returns the webhook server URL.
func (ws *WebhookServer) URL() string {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	return fmt.Sprintf("http://localhost:%d/webhook", ws.port)
}

// Port returns the server port.
func (ws *WebhookServer) Port() int {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	return ws.port
}

// handleWebhook handles incoming webhook notifications.
func (ws *WebhookServer) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var event WebhookEvent
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		ws.logger.Error("Failed to decode webhook event", zap.Error(err))
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	ws.logger.Info("Received webhook event",
		zap.String("id", event.ID),
		zap.String("type", event.Type),
		zap.String("resourceType", event.ResourceType),
		zap.String("resourceId", event.ResourceID),
	)

	// Store event
	ws.mu.Lock()
	ws.receivedEvents = append(ws.receivedEvents, &event)
	ws.mu.Unlock()

	// Send to channel (non-blocking)
	select {
	case ws.events <- &event:
	default:
		ws.logger.Warn("Event channel full, dropping event")
	}

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(`{"status":"ok"}`)); err != nil {
		ws.logger.Error("Failed to write response", zap.Error(err))
	}
}

// handleHealth handles health check requests.
func (ws *WebhookServer) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(`{"status":"healthy"}`)); err != nil {
		ws.logger.Error("Failed to write health response", zap.Error(err))
	}
}

// WaitForEvent waits for a webhook event with timeout.
// Returns the event or an error if timeout expires.
func (ws *WebhookServer) WaitForEvent(timeout time.Duration) (*WebhookEvent, error) {
	select {
	case event, ok := <-ws.events:
		if !ok {
			return nil, fmt.Errorf("webhook server stopped")
		}
		return event, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout waiting for webhook event")
	}
}

// WaitForEventWithFilter waits for an event matching the filter function.
func (ws *WebhookServer) WaitForEventWithFilter(
	timeout time.Duration,
	filter func(*WebhookEvent) bool,
) (*WebhookEvent, error) {
	deadline := time.Now().Add(timeout)

	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, fmt.Errorf("timeout waiting for matching webhook event")
		}

		event, err := ws.WaitForEvent(remaining)
		if err != nil {
			return nil, err
		}

		if filter(event) {
			return event, nil
		}

		// Event didn't match, continue waiting
	}
}

// GetReceivedEvents returns all received events.
func (ws *WebhookServer) GetReceivedEvents() []*WebhookEvent {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	// Return a copy
	events := make([]*WebhookEvent, len(ws.receivedEvents))
	copy(events, ws.receivedEvents)
	return events
}

// GetEventCount returns the number of received events.
func (ws *WebhookServer) GetEventCount() int {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	return len(ws.receivedEvents)
}

// ClearEvents clears all stored events.
func (ws *WebhookServer) ClearEvents() {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	ws.receivedEvents = make([]*WebhookEvent, 0)

	// Drain channel
	for {
		select {
		case <-ws.events:
		default:
			return
		}
	}
}

// FindEvent finds an event matching the given predicate.
func (ws *WebhookServer) FindEvent(predicate func(*WebhookEvent) bool) *WebhookEvent {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	for _, event := range ws.receivedEvents {
		if predicate(event) {
			return event
		}
	}

	return nil
}
