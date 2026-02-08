package backend

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapter"
)

// ErrNoBackendAccess is returned when a tenant has no backend access configured.
var ErrNoBackendAccess = fmt.Errorf("no backend access configured for tenant")

// ErrAdapterNotFound is returned when an adapter for a given backend ID is not in the registry.
var ErrAdapterNotFound = fmt.Errorf("adapter not found in registry")

// AdapterRegistry holds live adapter instances keyed by backend instance ID.
// It is thread-safe and supports loading adapters from the backend store on startup,
// as well as refreshing individual adapters when backend configuration changes.
type AdapterRegistry struct {
	mu       sync.RWMutex
	adapters map[string]adapter.Adapter
	factory  *AdapterFactory
	logger   *zap.Logger
}

// NewAdapterRegistry creates a new AdapterRegistry.
func NewAdapterRegistry(factory *AdapterFactory, logger *zap.Logger) *AdapterRegistry {
	return &AdapterRegistry{
		adapters: make(map[string]adapter.Adapter),
		factory:  factory,
		logger:   logger,
	}
}

// LoadFromStore loads all active backend instances from the store and creates adapters.
// Instances with status "error" or "disabled" are skipped.
// Returns the number of adapters successfully loaded.
func (r *AdapterRegistry) LoadFromStore(ctx context.Context, store Store) (int, error) {
	instances, err := store.ListBackendsByCategory(ctx, "ims")
	if err != nil {
		return 0, fmt.Errorf("failed to list IMS backends: %w", err)
	}

	loaded := 0
	for _, inst := range instances {
		if inst.Status == "error" || inst.Status == "disabled" {
			r.logger.Info("skipping backend with non-active status",
				zap.String("backend_id", inst.ID),
				zap.String("status", inst.Status),
			)
			continue
		}

		adp, createErr := r.factory.CreateAdapter(inst)
		if createErr != nil {
			r.logger.Warn("failed to create adapter for backend, skipping",
				zap.String("backend_id", inst.ID),
				zap.String("adapter_type", inst.AdapterType),
				zap.Error(createErr),
			)
			continue
		}

		r.mu.Lock()
		r.adapters[inst.ID] = adp
		r.mu.Unlock()

		r.logger.Info("adapter loaded for backend",
			zap.String("backend_id", inst.ID),
			zap.String("adapter_type", inst.AdapterType),
			zap.String("adapter_name", adp.Name()),
		)
		loaded++
	}

	return loaded, nil
}

// GetAdapter returns the adapter for a given backend instance ID.
func (r *AdapterRegistry) GetAdapter(backendID string) (adapter.Adapter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	adp, ok := r.adapters[backendID]
	if !ok {
		return nil, fmt.Errorf("backend %s: %w", backendID, ErrAdapterNotFound)
	}
	return adp, nil
}

// GetAdapterForTenant looks up the tenant's backend access and returns the corresponding adapter.
// If a tenant has access to multiple backends, the first one with an adapter is returned.
func (r *AdapterRegistry) GetAdapterForTenant(
	ctx context.Context,
	tenantID string,
	accessStore Store,
) (adapter.Adapter, error) {
	accesses, err := accessStore.ListBackendAccessByTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to list backend access for tenant %s: %w", tenantID, err)
	}

	if len(accesses) == 0 {
		return nil, fmt.Errorf("tenant %s: %w", tenantID, ErrNoBackendAccess)
	}

	// Return the first backend that has an adapter in the registry
	for _, access := range accesses {
		adp, adpErr := r.GetAdapter(access.BackendID)
		if adpErr == nil {
			return adp, nil
		}
	}

	return nil, fmt.Errorf(
		"tenant %s: no active adapters found for assigned backends: %w",
		tenantID, ErrAdapterNotFound,
	)
}

// RefreshAdapter reloads a single backend adapter from the store.
// This is useful when a backend's configuration changes.
func (r *AdapterRegistry) RefreshAdapter(ctx context.Context, store Store, backendID string) error {
	instance, err := store.GetBackend(ctx, backendID)
	if err != nil {
		return fmt.Errorf("failed to get backend %s: %w", backendID, err)
	}

	adp, createErr := r.factory.CreateAdapter(instance)
	if createErr != nil {
		return fmt.Errorf("failed to create adapter for backend %s: %w", backendID, createErr)
	}

	// Close old adapter if it exists
	r.mu.Lock()
	if oldAdp, ok := r.adapters[backendID]; ok {
		if closeErr := oldAdp.Close(); closeErr != nil {
			r.logger.Warn("failed to close old adapter during refresh",
				zap.String("backend_id", backendID),
				zap.Error(closeErr),
			)
		}
	}
	r.adapters[backendID] = adp
	r.mu.Unlock()

	r.logger.Info("adapter refreshed for backend",
		zap.String("backend_id", backendID),
		zap.String("adapter_type", instance.AdapterType),
	)

	return nil
}

// Count returns the number of adapters in the registry.
func (r *AdapterRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.adapters)
}

// Close shuts down all adapters in the registry.
func (r *AdapterRegistry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for id, adp := range r.adapters {
		if err := adp.Close(); err != nil {
			r.logger.Warn("failed to close adapter",
				zap.String("backend_id", id),
				zap.Error(err),
			)
		}
	}

	r.adapters = make(map[string]adapter.Adapter)
	return nil
}
