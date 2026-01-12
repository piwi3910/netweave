// Package smo provides the plugin interface and registry for SMO (Service Management and Orchestration) integration.
package smo

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// PluginInfo contains metadata about a registered plugin.
type PluginInfo struct {
	Name         string       `json:"name"`
	Version      string       `json:"version"`
	Description  string       `json:"description"`
	Vendor       string       `json:"vendor"`
	Capabilities []Capability `json:"capabilities"`
	Healthy      bool         `json:"healthy"`
	IsDefault    bool         `json:"isDefault"`
	RegisteredAt time.Time    `json:"registeredAt"`
	LastHealthAt time.Time    `json:"lastHealthAt"`
}

// Registry manages SMO plugin registration and discovery.
// It provides thread-safe access to registered plugins and handles
// plugin lifecycle, health monitoring, and capability-based lookups.
type Registry struct {
	mu            sync.RWMutex
	plugins       map[string]Plugin
	pluginInfo    map[string]*PluginInfo
	defaultPlugin string
	logger        *zap.Logger

	// Health check configuration
	healthCheckInterval time.Duration
	healthCheckTimeout  time.Duration
	stopHealthCheck     chan struct{}
	healthCheckWg       sync.WaitGroup
	healthCheckRunning  atomic.Bool // Prevents duplicate health check loops
}

// RegistryOption is a functional option for configuring Registry.
type RegistryOption func(*Registry)

// WithHealthCheckInterval sets the health check interval.
func WithHealthCheckInterval(interval time.Duration) RegistryOption {
	return func(r *Registry) {
		if interval > 0 {
			r.healthCheckInterval = interval
		}
	}
}

// WithHealthCheckTimeout sets the health check timeout.
func WithHealthCheckTimeout(timeout time.Duration) RegistryOption {
	return func(r *Registry) {
		if timeout > 0 {
			r.healthCheckTimeout = timeout
		}
	}
}

// NewRegistry creates a new SMO plugin registry with the provided logger.
// Optional RegistryOption functions can be provided to configure health check intervals.
func NewRegistry(logger *zap.Logger, opts ...RegistryOption) *Registry {
	if logger == nil {
		logger = zap.NewNop()
	}

	r := &Registry{
		plugins:             make(map[string]Plugin),
		pluginInfo:          make(map[string]*PluginInfo),
		logger:              logger,
		healthCheckInterval: 30 * time.Second, // Default: 30 seconds
		healthCheckTimeout:  5 * time.Second,  // Default: 5 seconds
		stopHealthCheck:     make(chan struct{}),
	}

	// Apply optional configurations
	for _, opt := range opts {
		opt(r)
	}

	return r
}

// Register registers a new SMO plugin with the registry.
// If isDefault is true, this plugin becomes the default for operations.
func (r *Registry) Register(ctx context.Context, name string, plugin Plugin, isDefault bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if name == "" {
		return fmt.Errorf("plugin name cannot be empty")
	}

	if plugin == nil {
		return fmt.Errorf("plugin cannot be nil")
	}

	if _, exists := r.plugins[name]; exists {
		return fmt.Errorf("plugin %s already registered", name)
	}

	// Get plugin metadata
	metadata := plugin.Metadata()

	// Perform initial health check
	health := plugin.Health(ctx)

	// Store plugin and its info
	r.plugins[name] = plugin
	r.pluginInfo[name] = &PluginInfo{
		Name:         metadata.Name,
		Version:      metadata.Version,
		Description:  metadata.Description,
		Vendor:       metadata.Vendor,
		Capabilities: plugin.Capabilities(),
		Healthy:      health.Healthy,
		IsDefault:    isDefault,
		RegisteredAt: time.Now(),
		LastHealthAt: time.Now(),
	}

	if isDefault || r.defaultPlugin == "" {
		r.defaultPlugin = name
	}

	r.logger.Info("registered SMO plugin",
		zap.String("name", name),
		zap.String("version", metadata.Version),
		zap.Bool("isDefault", isDefault),
		zap.Bool("healthy", health.Healthy),
	)

	return nil
}

// Unregister removes a plugin from the registry.
func (r *Registry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	plugin, exists := r.plugins[name]
	if !exists {
		return fmt.Errorf("plugin %s not found", name)
	}

	// Close the plugin
	if err := plugin.Close(); err != nil {
		r.logger.Warn("error closing plugin during unregister",
			zap.String("name", name),
			zap.Error(err),
		)
	}

	delete(r.plugins, name)
	delete(r.pluginInfo, name)

	// Update default plugin if necessary
	if r.defaultPlugin == name {
		r.defaultPlugin = ""
		for n := range r.plugins {
			r.defaultPlugin = n
			if info, ok := r.pluginInfo[n]; ok {
				info.IsDefault = true
			}
			break
		}
	}

	r.logger.Info("unregistered SMO plugin", zap.String("name", name))

	return nil
}

// get retrieves a plugin by name (internal method).
func (r *Registry) get(name string) (Plugin, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	plugin, exists := r.plugins[name]
	if !exists {
		return nil, fmt.Errorf("plugin %s not found", name)
	}

	return plugin, nil
}

// Get retrieves a plugin by name.
func (r *Registry) Get(name string) (Plugin, error) {
	return r.get(name)
}

// getDefault retrieves the default plugin (internal method).
func (r *Registry) getDefault() (Plugin, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.defaultPlugin == "" {
		return nil, fmt.Errorf("no default plugin configured")
	}

	plugin, exists := r.plugins[r.defaultPlugin]
	if !exists {
		return nil, fmt.Errorf("default plugin %s not found", r.defaultPlugin)
	}

	return plugin, nil
}

// GetDefault retrieves the default plugin.
func (r *Registry) GetDefault() (Plugin, error) {
	return r.getDefault()
}

// SetDefault sets the default plugin by name.
func (r *Registry) SetDefault(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.plugins[name]; !exists {
		return fmt.Errorf("plugin %s not found", name)
	}

	// Update previous default
	if prevInfo, ok := r.pluginInfo[r.defaultPlugin]; ok {
		prevInfo.IsDefault = false
	}

	r.defaultPlugin = name
	if info, ok := r.pluginInfo[name]; ok {
		info.IsDefault = true
	}

	r.logger.Info("set default SMO plugin", zap.String("name", name))

	return nil
}

// List returns information about all registered plugins.
// Returns deep copies to prevent external modification of internal state.
func (r *Registry) List() []*PluginInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*PluginInfo, 0, len(r.pluginInfo))
	for _, info := range r.pluginInfo {
		// Create a deep copy to avoid exposing internal state
		infoCopy := *info
		// Deep copy the Capabilities slice to prevent external modification
		if info.Capabilities != nil {
			infoCopy.Capabilities = make([]Capability, len(info.Capabilities))
			copy(infoCopy.Capabilities, info.Capabilities)
		}
		result = append(result, &infoCopy)
	}

	return result
}

// FindByCapability finds all plugins that support the given capability.
func (r *Registry) FindByCapability(capability Capability) []Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Plugin, 0)
	for name, plugin := range r.plugins {
		info := r.pluginInfo[name]
		if info == nil || !info.Healthy {
			continue
		}

		for _, c := range info.Capabilities {
			if c == capability {
				result = append(result, plugin)
				break
			}
		}
	}

	return result
}

// GetHealthy returns all healthy plugins.
func (r *Registry) GetHealthy() []Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Plugin, 0)
	for name, plugin := range r.plugins {
		info := r.pluginInfo[name]
		if info != nil && info.Healthy {
			result = append(result, plugin)
		}
	}

	return result
}

// StartHealthChecks starts periodic health checking for all registered plugins.
// This function is idempotent - multiple calls will not spawn duplicate goroutines.
func (r *Registry) StartHealthChecks(ctx context.Context) {
	// Use atomic.Bool to prevent duplicate health check loops
	if !r.healthCheckRunning.CompareAndSwap(false, true) {
		r.logger.Debug("health check loop already running, skipping")
		return
	}

	r.healthCheckWg.Add(1)
	go r.healthCheckLoop(ctx)
}

// StopHealthChecks stops the periodic health check loop.
func (r *Registry) StopHealthChecks() {
	if !r.healthCheckRunning.Load() {
		return // Not running, nothing to stop
	}

	close(r.stopHealthCheck)
	r.healthCheckWg.Wait()
	r.healthCheckRunning.Store(false)
}

// healthCheckLoop periodically checks health of all registered plugins.
func (r *Registry) healthCheckLoop(ctx context.Context) {
	defer r.healthCheckWg.Done()

	ticker := time.NewTicker(r.healthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-r.stopHealthCheck:
			return
		case <-ticker.C:
			r.checkAllPluginsHealth(ctx)
		}
	}
}

// checkAllPluginsHealth checks health of all registered plugins.
func (r *Registry) checkAllPluginsHealth(ctx context.Context) {
	r.mu.RLock()
	plugins := make(map[string]Plugin, len(r.plugins))
	for name, plugin := range r.plugins {
		plugins[name] = plugin
	}
	r.mu.RUnlock()

	for name, plugin := range plugins {
		// Use anonymous function to ensure cancel is always called via defer
		func() {
			checkCtx, cancel := context.WithTimeout(ctx, r.healthCheckTimeout)
			defer cancel()

			health := plugin.Health(checkCtx)

			r.mu.Lock()
			defer r.mu.Unlock()

			info, exists := r.pluginInfo[name]
			if !exists {
				return
			}

			wasHealthy := info.Healthy
			info.Healthy = health.Healthy
			info.LastHealthAt = time.Now()

			// Log health status changes
			if wasHealthy == health.Healthy {
				return
			}

			if health.Healthy {
				r.logger.Info("SMO plugin became healthy",
					zap.String("name", name),
				)
			} else {
				r.logger.Warn("SMO plugin became unhealthy",
					zap.String("name", name),
					zap.String("message", health.Message),
				)
			}
		}()
	}
}

// Close closes all registered plugins and stops health checks.
func (r *Registry) Close() error {
	r.StopHealthChecks()

	r.mu.Lock()
	defer r.mu.Unlock()

	var errs []error
	for name, plugin := range r.plugins {
		if err := plugin.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close plugin %s: %w", name, err))
		}
	}

	r.plugins = make(map[string]Plugin)
	r.pluginInfo = make(map[string]*PluginInfo)
	r.defaultPlugin = ""

	if len(errs) > 0 {
		return fmt.Errorf("errors closing plugins: %v", errs)
	}

	return nil
}

// Count returns the number of registered plugins.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.plugins)
}
