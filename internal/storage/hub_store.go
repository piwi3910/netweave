package storage

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	// ErrHubNotFound is returned when a hub registration is not found.
	ErrHubNotFound = errors.New("hub registration not found")
	// ErrHubExists is returned when attempting to create a hub that already exists.
	ErrHubExists = errors.New("hub registration already exists")
)

// HubRegistration represents a TMF688 hub registration with O2-IMS subscription mapping.
type HubRegistration struct {
	HubID          string                 `json:"hubId"`
	Callback       string                 `json:"callback"`
	Query          string                 `json:"query"`
	SubscriptionID string                 `json:"subscriptionId"`
	CreatedAt      time.Time              `json:"createdAt"`
	Extensions     map[string]interface{} `json:"extensions,omitempty"`
}

// HubStore provides storage operations for TMF688 hub registrations.
type HubStore interface {
	// Create stores a new hub registration
	Create(ctx context.Context, hub *HubRegistration) error

	// Get retrieves a hub registration by ID
	Get(ctx context.Context, hubID string) (*HubRegistration, error)

	// List retrieves all hub registrations
	List(ctx context.Context) ([]*HubRegistration, error)

	// Delete removes a hub registration by ID
	Delete(ctx context.Context, hubID string) error

	// Close releases any resources held by the store
	Close() error
}

// InMemoryHubStore implements HubStore using in-memory storage.
type InMemoryHubStore struct {
	mu   sync.RWMutex
	hubs map[string]*HubRegistration
}

// NewInMemoryHubStore creates a new in-memory hub store.
func NewInMemoryHubStore() *InMemoryHubStore {
	return &InMemoryHubStore{
		hubs: make(map[string]*HubRegistration),
	}
}

// Create stores a new hub registration.
func (s *InMemoryHubStore) Create(_ context.Context, hub *HubRegistration) error {
	if hub == nil {
		return errors.New("hub registration cannot be nil")
	}
	if hub.HubID == "" {
		return errors.New("hub ID cannot be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.hubs[hub.HubID]; exists {
		return ErrHubExists
	}

	s.hubs[hub.HubID] = hub
	return nil
}

// Get retrieves a hub registration by ID.
func (s *InMemoryHubStore) Get(_ context.Context, hubID string) (*HubRegistration, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	hub, exists := s.hubs[hubID]
	if !exists {
		return nil, ErrHubNotFound
	}

	return hub, nil
}

// List retrieves all hub registrations.
func (s *InMemoryHubStore) List(_ context.Context) ([]*HubRegistration, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	hubs := make([]*HubRegistration, 0, len(s.hubs))
	for _, hub := range s.hubs {
		hubs = append(hubs, hub)
	}

	return hubs, nil
}

// Delete removes a hub registration by ID.
func (s *InMemoryHubStore) Delete(_ context.Context, hubID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.hubs[hubID]; !exists {
		return ErrHubNotFound
	}

	delete(s.hubs, hubID)
	return nil
}

// Close releases any resources held by the store.
func (s *InMemoryHubStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.hubs = nil
	return nil
}
