// Package helpers provides common test utilities for integration tests.
//
//go:build integration
// +build integration

package helpers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// WebhookNotification represents a webhook notification received during tests.
type WebhookNotification struct {
	SubscriptionID string                 `json:"subscriptionId"`
	EventType      string                 `json:"eventType"`
	Resource       map[string]interface{} `json:"resource"`
	Timestamp      time.Time              `json:"timestamp"`
	ReceivedAt     time.Time              `json:"-"` // Set by server when received
}

// WebhookServer is a test HTTP server that captures webhook notifications.
type WebhookServer struct {
	server        *httptest.Server
	notifications []WebhookNotification
	mu            sync.RWMutex
	notifyChan    chan WebhookNotification
	t             *testing.T
}

// NewWebhookServer creates a new webhook test server.
func NewWebhookServer(t *testing.T) *WebhookServer {
	t.Helper()

	ws := &WebhookServer{
		notifications: make([]WebhookNotification, 0),
		notifyChan:    make(chan WebhookNotification, 100),
		t:             t,
	}

	// Create HTTP handler
	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", ws.handleWebhook)
	mux.HandleFunc("/health", ws.handleHealth)

	// Start test server
	ws.server = httptest.NewServer(mux)

	t.Cleanup(func() {
		ws.Close()
	})

	return ws
}

// handleWebhook processes incoming webhook notifications.
func (ws *WebhookServer) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var notification WebhookNotification
	if err := json.NewDecoder(r.Body).Decode(&notification); err != nil {
		ws.t.Logf("Failed to decode webhook: %v", err)
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Set received timestamp
	notification.ReceivedAt = time.Now()

	// Store notification
	ws.mu.Lock()
	ws.notifications = append(ws.notifications, notification)
	ws.mu.Unlock()

	// Send to channel for waiting tests
	select {
	case ws.notifyChan <- notification:
	default:
		ws.t.Logf("Warning: notification channel full, dropping notification")
	}

	ws.t.Logf("Received webhook: %s - %s", notification.EventType, notification.SubscriptionID)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "received",
	})
}

// handleHealth responds to health check requests.
func (ws *WebhookServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
	})
}

// URL returns the webhook server URL.
func (ws *WebhookServer) URL() string {
	return ws.server.URL + "/webhook"
}

// GetNotifications returns all received notifications.
func (ws *WebhookServer) GetNotifications() []WebhookNotification {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	// Return a copy
	result := make([]WebhookNotification, len(ws.notifications))
	copy(result, ws.notifications)
	return result
}

// WaitForNotification waits for a notification to be received within the timeout.
// Returns the notification or nil if timeout expires.
func (ws *WebhookServer) WaitForNotification(timeout time.Duration) *WebhookNotification {
	select {
	case notification := <-ws.notifyChan:
		return &notification
	case <-time.After(timeout):
		ws.t.Logf("Timeout waiting for webhook notification after %v", timeout)
		return nil
	}
}

// WaitForNotifications waits for a specific number of notifications within the timeout.
func (ws *WebhookServer) WaitForNotifications(count int, timeout time.Duration) []WebhookNotification {
	result := make([]WebhookNotification, 0, count)
	deadline := time.Now().Add(timeout)

	for i := 0; i < count; i++ {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			ws.t.Logf("Timeout waiting for %d notifications (received %d)", count, i)
			return result
		}

		notification := ws.WaitForNotification(remaining)
		if notification == nil {
			ws.t.Logf("Failed to receive notification %d/%d", i+1, count)
			return result
		}

		result = append(result, *notification)
	}

	return result
}

// Clear clears all received notifications.
func (ws *WebhookServer) Clear() {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	ws.notifications = ws.notifications[:0]

	// Drain channel
	for {
		select {
		case <-ws.notifyChan:
		default:
			return
		}
	}
}

// Close closes the webhook server.
func (ws *WebhookServer) Close() {
	if ws.server != nil {
		ws.server.Close()
	}
	close(ws.notifyChan)
}
