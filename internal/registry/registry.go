// Package registry provides plugin registration and management for the netweave gateway.
// It maintains a registry of adapter plugins and provides plugin lifecycle management.
package registry

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/piwi3910/netweave/internal/adapter"
	"go.uber.org/zap"
)

// PluginMetadata contains metadata about a registered plugin.
type PluginMetadata struct {
	// Name is the unique identifier for this plugin.
	Name string

	// Type is the plugin type (e.g., "kubernetes", "openstack", "dtias").
	Type string

	// Version is the plugin version.
	Version string

	// Enabled indicates if the plugin is currently enabled.
	Enabled bool

	// Default indicates if this is the default plugin for its category.
	Default bool

	// Capabilities lists the features this plugin supports.
	Capabilities []adapter.Capability

	// RegisteredAt is when the plugin was registered.
	RegisteredAt time.Time

	// LastHealthCheck is the last time health was checked.
	LastHealthCheck time.Time

	// Healthy indicates if the plugin passed the last health check.
	Healthy bool

	// HealthError contains the last health check error if any.
	HealthError error

	// Config contains the plugin-specific configuration.
	Config map[string]interface{}
}

// Registry manages adapter plugins and their lifecycle.
// It provides thread-safe plugin registration, lookup, and health monitoring.
type Registry struct {
	mu              sync.RWMutex
	plugins         map[string]adapter.Adapter
	meta            map[string]*PluginMetadata
	defaultPlugin   string
	logger          *zap.Logger

	// Health check configuration
	healthCheckInterval time.Duration
	healthCheckTimeout  time.Duration
	stopHealthCheck     chan struct{}
	healthCheckWg       sync.WaitGroup
}

// Config contains configuration for the registry.
type Config struct {
	// HealthCheckInterval is how often to perform health checks.
	// Default: 30 seconds.
	HealthCheckInterval time.Duration

	// HealthCheckTimeout is the timeout for each health check.
	// Default: 5 seconds.
	HealthCheckTimeout time.Duration
}

// NewRegistry creates a new plugin registry.
func NewRegistry(logger *zap.Logger, config *Config) *Registry {
	if config == nil {
		config = &Config{}
	}
	if config.HealthCheckInterval == 0 {
		config.HealthCheckInterval = 30 * time.Second
	}
	if config.HealthCheckTimeout == 0 {
		config.HealthCheckTimeout = 5 * time.Second
	}

	return &Registry{
		plugins:             make(map[string]adapter.Adapter),
		meta:                make(map[string]*PluginMetadata),
		logger:              logger,
		healthCheckInterval: config.HealthCheckInterval,
		healthCheckTimeout:  config.HealthCheckTimeout,
		stopHealthCheck:     make(chan struct{}),
	}
}

// Register registers a plugin with the registry.
// Returns an error if a plugin with the same name is already registered.
func (r *Registry) Register(
	ctx context.Context,
	name string,
	typ string,
	plugin adapter.Adapter,
	config map[string]interface{},
	isDefault bool,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.plugins[name]; exists {
		return fmt.Errorf("plugin %s already registered", name)
	}

	// Perform initial health check
	healthy := true
	var healthErr error
	healthCtx, cancel := context.WithTimeout(ctx, r.healthCheckTimeout)
	defer cancel()

	if err := plugin.Health(healthCtx); err != nil {
		healthy = false
		healthErr = err
		r.logger.Warn("plugin failed initial health check",
			zap.String("plugin", name),
			zap.Error(err),
		)
	}

	meta := &PluginMetadata{
		Name:            name,
		Type:            typ,
		Version:         plugin.Version(),
		Enabled:         true,
		Default:         isDefault,
		Capabilities:    plugin.Capabilities(),
		RegisteredAt:    time.Now(),
		LastHealthCheck: time.Now(),
		Healthy:         healthy,
		HealthError:     healthErr,
		Config:          config,
	}

	r.plugins[name] = plugin
	r.meta[name] = meta

	if isDefault {
		r.defaultPlugin = name
	}

	r.logger.Info("plugin registered",
		zap.String("plugin", name),
		zap.String("type", typ),
		zap.String("version", plugin.Version()),
		zap.Bool("default", isDefault),
		zap.Bool("healthy", healthy),
	)

	return nil
}

// Unregister removes a plugin from the registry.
// It closes the plugin before removing it.
func (r *Registry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	plugin, exists := r.plugins[name]
	if !exists {
		return fmt.Errorf("plugin %s not found", name)
	}

	if err := plugin.Close(); err != nil {
		r.logger.Warn("error closing plugin",
			zap.String("plugin", name),
			zap.Error(err),
		)
	}

	delete(r.plugins, name)
	delete(r.meta, name)

	if r.defaultPlugin == name {
		r.defaultPlugin = ""
	}

	r.logger.Info("plugin unregistered",
		zap.String("plugin", name),
	)

	return nil
}

// Get retrieves a plugin by name.
// Returns nil if the plugin is not found.
func (r *Registry) Get(name string) adapter.Adapter {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.plugins[name]
}

// GetDefault returns the default plugin.
// Returns nil if no default is set.
func (r *Registry) GetDefault() adapter.Adapter {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.defaultPlugin == "" {
		return nil
	}

	return r.plugins[r.defaultPlugin]
}

// GetMetadata retrieves metadata for a plugin.
// Returns nil if the plugin is not found.
func (r *Registry) GetMetadata(name string) *PluginMetadata {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.meta[name]
}

// List returns all registered plugins.
func (r *Registry) List() []adapter.Adapter {
	r.mu.RLock()
	defer r.mu.RUnlock()

	plugins := make([]adapter.Adapter, 0, len(r.plugins))
	for _, p := range r.plugins {
		plugins = append(plugins, p)
	}

	return plugins
}

// ListMetadata returns metadata for all registered plugins.
func (r *Registry) ListMetadata() []*PluginMetadata {
	r.mu.RLock()
	defer r.mu.RUnlock()

	metadata := make([]*PluginMetadata, 0, len(r.meta))
	for _, m := range r.meta {
		metadata = append(metadata, m)
	}

	return metadata
}

// ListHealthy returns all healthy plugins.
func (r *Registry) ListHealthy() []adapter.Adapter {
	r.mu.RLock()
	defer r.mu.RUnlock()

	plugins := make([]adapter.Adapter, 0, len(r.plugins))
	for name, p := range r.plugins {
		if meta := r.meta[name]; meta != nil && meta.Healthy {
			plugins = append(plugins, p)
		}
	}

	return plugins
}

// FindByCapability returns all plugins that support a specific capability.
func (r *Registry) FindByCapability(cap adapter.Capability) []adapter.Adapter {
	r.mu.RLock()
	defer r.mu.RUnlock()

	plugins := make([]adapter.Adapter, 0)
	for name, p := range r.plugins {
		meta := r.meta[name]
		if meta == nil || !meta.Enabled || !meta.Healthy {
			continue
		}

		for _, c := range meta.Capabilities {
			if c == cap {
				plugins = append(plugins, p)
				break
			}
		}
	}

	return plugins
}

// FindByType returns all plugins of a specific type.
func (r *Registry) FindByType(typ string) []adapter.Adapter {
	r.mu.RLock()
	defer r.mu.RUnlock()

	plugins := make([]adapter.Adapter, 0)
	for name, p := range r.plugins {
		if meta := r.meta[name]; meta != nil && meta.Type == typ && meta.Enabled {
			plugins = append(plugins, p)
		}
	}

	return plugins
}

// Enable enables a plugin.
func (r *Registry) Enable(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	meta := r.meta[name]
	if meta == nil {
		return fmt.Errorf("plugin %s not found", name)
	}

	meta.Enabled = true

	r.logger.Info("plugin enabled",
		zap.String("plugin", name),
	)

	return nil
}

// Disable disables a plugin without unregistering it.
func (r *Registry) Disable(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	meta := r.meta[name]
	if meta == nil {
		return fmt.Errorf("plugin %s not found", name)
	}

	meta.Enabled = false

	r.logger.Info("plugin disabled",
		zap.String("plugin", name),
	)

	return nil
}

// SetDefault sets the default plugin.
func (r *Registry) SetDefault(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.plugins[name]; !exists {
		return fmt.Errorf("plugin %s not found", name)
	}

	// Clear previous default
	if r.defaultPlugin != "" {
		if meta := r.meta[r.defaultPlugin]; meta != nil {
			meta.Default = false
		}
	}

	r.defaultPlugin = name
	if meta := r.meta[name]; meta != nil {
		meta.Default = true
	}

	r.logger.Info("default plugin set",
		zap.String("plugin", name),
	)

	return nil
}

// StartHealthChecks starts background health checking for all plugins.
func (r *Registry) StartHealthChecks(ctx context.Context) {
	r.healthCheckWg.Add(1)
	go r.healthCheckLoop(ctx)

	r.logger.Info("health check started",
		zap.Duration("interval", r.healthCheckInterval),
		zap.Duration("timeout", r.healthCheckTimeout),
	)
}

// StopHealthChecks stops background health checking.
func (r *Registry) StopHealthChecks() {
	select {
	case <-r.stopHealthCheck:
		// Already stopped
		return
	default:
		close(r.stopHealthCheck)
	}

	r.healthCheckWg.Wait()

	r.logger.Info("health check stopped")
}

// healthCheckLoop runs periodic health checks on all plugins.
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
			r.performHealthChecks(ctx)
		}
	}
}

// performHealthChecks checks health of all registered plugins.
func (r *Registry) performHealthChecks(ctx context.Context) {
	r.mu.RLock()
	plugins := make(map[string]adapter.Adapter, len(r.plugins))
	for name, p := range r.plugins {
		plugins[name] = p
	}
	r.mu.RUnlock()

	for name, plugin := range plugins {
		r.checkPluginHealth(ctx, name, plugin)
	}
}

// checkPluginHealth performs a health check on a single plugin.
func (r *Registry) checkPluginHealth(ctx context.Context, name string, plugin adapter.Adapter) {
	healthCtx, cancel := context.WithTimeout(ctx, r.healthCheckTimeout)
	defer cancel()

	err := plugin.Health(healthCtx)
	healthy := err == nil

	r.mu.Lock()
	meta := r.meta[name]
	if meta != nil {
		previouslyHealthy := meta.Healthy
		meta.Healthy = healthy
		meta.HealthError = err
		meta.LastHealthCheck = time.Now()

		// Log health status changes
		if previouslyHealthy != healthy {
			if healthy {
				r.logger.Info("plugin recovered",
					zap.String("plugin", name),
				)
			} else {
				r.logger.Warn("plugin unhealthy",
					zap.String("plugin", name),
					zap.Error(err),
				)
			}
		}
	}
	r.mu.Unlock()
}

// Close closes all registered plugins and stops health checks.
func (r *Registry) Close() error {
	r.StopHealthChecks()

	r.mu.Lock()
	defer r.mu.Unlock()

	var lastErr error
	for name, plugin := range r.plugins {
		if err := plugin.Close(); err != nil {
			r.logger.Error("error closing plugin",
				zap.String("plugin", name),
				zap.Error(err),
			)
			lastErr = err
		}
	}

	r.plugins = make(map[string]adapter.Adapter)
	r.meta = make(map[string]*PluginMetadata)
	r.defaultPlugin = ""

	r.logger.Info("registry closed")

	return lastErr
}
