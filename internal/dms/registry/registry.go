// Package registry provides DMS adapter registration and management.
// It maintains a registry of DMS adapter plugins and provides lifecycle management.
package registry

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/piwi3910/netweave/internal/dms/adapter"
	"go.uber.org/zap"
)

// Default configuration values for the registry.
const (
	// DefaultHealthCheckInterval is the default interval between health checks.
	DefaultHealthCheckInterval = 30 * time.Second

	// DefaultHealthCheckTimeout is the default timeout for each health check.
	DefaultHealthCheckTimeout = 5 * time.Second
)

// PluginMetadata contains metadata about a registered DMS plugin.
type PluginMetadata struct {
	// Name is the unique identifier for this plugin.
	Name string

	// Type is the plugin type (e.g., "helm", "argocd", "flux").
	Type string

	// Version is the plugin version.
	Version string

	// Enabled indicates if the plugin is currently enabled.
	Enabled bool

	// Default indicates if this is the default plugin.
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

// Registry manages DMS adapter plugins and their lifecycle.
// It provides thread-safe plugin registration, lookup, and health monitoring.
type Registry struct {
	mu            sync.RWMutex
	plugins       map[string]adapter.DMSAdapter
	meta          map[string]*PluginMetadata
	defaultPlugin string
	logger        *zap.Logger

	// Health check configuration.
	healthCheckInterval time.Duration
	healthCheckTimeout  time.Duration
	stopHealthCheck     chan struct{}
	healthCheckWg       sync.WaitGroup
}

// Config contains configuration for the DMS registry.
type Config struct {
	// HealthCheckInterval is how often to perform health checks.
	HealthCheckInterval time.Duration

	// HealthCheckTimeout is the timeout for each health check.
	HealthCheckTimeout time.Duration
}

// NewRegistry creates a new DMS plugin registry.
func NewRegistry(logger *zap.Logger, config *Config) *Registry {
	if config == nil {
		config = &Config{}
	}
	if config.HealthCheckInterval == 0 {
		config.HealthCheckInterval = DefaultHealthCheckInterval
	}
	if config.HealthCheckTimeout == 0 {
		config.HealthCheckTimeout = DefaultHealthCheckTimeout
	}

	return &Registry{
		plugins:             make(map[string]adapter.DMSAdapter),
		meta:                make(map[string]*PluginMetadata),
		logger:              logger,
		healthCheckInterval: config.HealthCheckInterval,
		healthCheckTimeout:  config.HealthCheckTimeout,
		stopHealthCheck:     make(chan struct{}),
	}
}

// Register registers a DMS plugin with the registry.
// Returns an error if a plugin with the same name is already registered.
func (r *Registry) Register(
	ctx context.Context,
	name string,
	typ string,
	plugin adapter.DMSAdapter,
	config map[string]interface{},
	isDefault bool,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.plugins[name]; exists {
		return fmt.Errorf("DMS plugin %s already registered", name)
	}

	// Perform initial health check.
	healthy := true
	var healthErr error
	healthCtx, cancel := context.WithTimeout(ctx, r.healthCheckTimeout)
	defer cancel()

	if err := plugin.Health(healthCtx); err != nil {
		healthy = false
		healthErr = err
		r.logger.Warn("DMS plugin failed initial health check",
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

	r.logger.Info("DMS plugin registered",
		zap.String("plugin", name),
		zap.String("type", typ),
		zap.String("version", plugin.Version()),
		zap.Bool("default", isDefault),
		zap.Bool("healthy", healthy),
	)

	return nil
}

// Unregister removes a DMS plugin from the registry.
// It closes the plugin before removing it.
func (r *Registry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	plugin, exists := r.plugins[name]
	if !exists {
		return fmt.Errorf("DMS plugin %s not found", name)
	}

	if err := plugin.Close(); err != nil {
		r.logger.Warn("error closing DMS plugin",
			zap.String("plugin", name),
			zap.Error(err),
		)
	}

	delete(r.plugins, name)
	delete(r.meta, name)

	if r.defaultPlugin == name {
		r.defaultPlugin = ""
	}

	r.logger.Info("DMS plugin unregistered",
		zap.String("plugin", name),
	)

	return nil
}

// Get retrieves a DMS plugin by name.
// Returns nil if the plugin is not found.
func (r *Registry) Get(name string) adapter.DMSAdapter {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.plugins[name]
}

// GetDefault returns the default DMS plugin.
// Returns nil if no default is set.
func (r *Registry) GetDefault() adapter.DMSAdapter {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.defaultPlugin == "" {
		return nil
	}

	return r.plugins[r.defaultPlugin]
}

// GetDefaultName returns the name of the default DMS plugin.
// Returns empty string if no default is set.
func (r *Registry) GetDefaultName() string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.defaultPlugin
}

// GetMetadata retrieves metadata for a DMS plugin.
// Returns nil if the plugin is not found.
func (r *Registry) GetMetadata(name string) *PluginMetadata {
	r.mu.RLock()
	defer r.mu.RUnlock()

	meta := r.meta[name]
	if meta == nil {
		return nil
	}

	// Return a deep copy to prevent data races and external mutations.
	metaCopy := &PluginMetadata{
		Name:            meta.Name,
		Type:            meta.Type,
		Version:         meta.Version,
		Enabled:         meta.Enabled,
		Default:         meta.Default,
		RegisteredAt:    meta.RegisteredAt,
		LastHealthCheck: meta.LastHealthCheck,
		Healthy:         meta.Healthy,
		HealthError:     meta.HealthError,
	}

	// Deep copy Capabilities slice.
	if meta.Capabilities != nil {
		metaCopy.Capabilities = make([]adapter.Capability, len(meta.Capabilities))
		copy(metaCopy.Capabilities, meta.Capabilities)
	}

	// Deep copy Config map.
	if meta.Config != nil {
		metaCopy.Config = make(map[string]interface{}, len(meta.Config))
		for k, v := range meta.Config {
			metaCopy.Config[k] = v
		}
	}

	return metaCopy
}

// List returns all registered DMS plugins.
func (r *Registry) List() []adapter.DMSAdapter {
	r.mu.RLock()
	defer r.mu.RUnlock()

	plugins := make([]adapter.DMSAdapter, 0, len(r.plugins))
	for _, p := range r.plugins {
		plugins = append(plugins, p)
	}

	return plugins
}

// ListMetadata returns metadata for all registered DMS plugins.
// Returns deep copies to prevent data races and external mutations.
func (r *Registry) ListMetadata() []*PluginMetadata {
	r.mu.RLock()
	defer r.mu.RUnlock()

	metadata := make([]*PluginMetadata, 0, len(r.meta))
	for _, m := range r.meta {
		// Create a deep copy to prevent data races.
		metaCopy := &PluginMetadata{
			Name:            m.Name,
			Type:            m.Type,
			Version:         m.Version,
			Enabled:         m.Enabled,
			Default:         m.Default,
			RegisteredAt:    m.RegisteredAt,
			LastHealthCheck: m.LastHealthCheck,
			Healthy:         m.Healthy,
			HealthError:     m.HealthError,
		}

		// Deep copy Capabilities slice.
		if m.Capabilities != nil {
			metaCopy.Capabilities = make([]adapter.Capability, len(m.Capabilities))
			copy(metaCopy.Capabilities, m.Capabilities)
		}

		// Deep copy Config map.
		if m.Config != nil {
			metaCopy.Config = make(map[string]interface{}, len(m.Config))
			for k, v := range m.Config {
				metaCopy.Config[k] = v
			}
		}

		metadata = append(metadata, metaCopy)
	}

	return metadata
}

// ListHealthy returns all healthy DMS plugins.
func (r *Registry) ListHealthy() []adapter.DMSAdapter {
	r.mu.RLock()
	defer r.mu.RUnlock()

	plugins := make([]adapter.DMSAdapter, 0, len(r.plugins))
	for name, p := range r.plugins {
		if meta := r.meta[name]; meta != nil && meta.Healthy {
			plugins = append(plugins, p)
		}
	}

	return plugins
}

// FindByCapability returns all DMS plugins that support a specific capability.
func (r *Registry) FindByCapability(cap adapter.Capability) []adapter.DMSAdapter {
	r.mu.RLock()
	defer r.mu.RUnlock()

	plugins := make([]adapter.DMSAdapter, 0)
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

// FindByType returns all DMS plugins of a specific type.
func (r *Registry) FindByType(typ string) []adapter.DMSAdapter {
	r.mu.RLock()
	defer r.mu.RUnlock()

	plugins := make([]adapter.DMSAdapter, 0)
	for name, p := range r.plugins {
		if meta := r.meta[name]; meta != nil && meta.Type == typ && meta.Enabled {
			plugins = append(plugins, p)
		}
	}

	return plugins
}

// Enable enables a DMS plugin.
func (r *Registry) Enable(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	meta := r.meta[name]
	if meta == nil {
		return fmt.Errorf("DMS plugin %s not found", name)
	}

	meta.Enabled = true

	r.logger.Info("DMS plugin enabled",
		zap.String("plugin", name),
	)

	return nil
}

// Disable disables a DMS plugin without unregistering it.
func (r *Registry) Disable(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	meta := r.meta[name]
	if meta == nil {
		return fmt.Errorf("DMS plugin %s not found", name)
	}

	meta.Enabled = false

	r.logger.Info("DMS plugin disabled",
		zap.String("plugin", name),
	)

	return nil
}

// SetDefault sets the default DMS plugin.
func (r *Registry) SetDefault(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.plugins[name]; !exists {
		return fmt.Errorf("DMS plugin %s not found", name)
	}

	// Clear previous default.
	if r.defaultPlugin != "" {
		if meta := r.meta[r.defaultPlugin]; meta != nil {
			meta.Default = false
		}
	}

	r.defaultPlugin = name
	if meta := r.meta[name]; meta != nil {
		meta.Default = true
	}

	r.logger.Info("default DMS plugin set",
		zap.String("plugin", name),
	)

	return nil
}

// StartHealthChecks starts background health checking for all DMS plugins.
func (r *Registry) StartHealthChecks(ctx context.Context) {
	r.healthCheckWg.Add(1)
	go r.healthCheckLoop(ctx)

	r.logger.Info("DMS health check started",
		zap.Duration("interval", r.healthCheckInterval),
		zap.Duration("timeout", r.healthCheckTimeout),
	)
}

// StopHealthChecks stops background health checking.
func (r *Registry) StopHealthChecks() {
	select {
	case <-r.stopHealthCheck:
		// Already stopped.
		return
	default:
		close(r.stopHealthCheck)
	}

	r.healthCheckWg.Wait()

	r.logger.Info("DMS health check stopped")
}

// healthCheckLoop runs periodic health checks on all DMS plugins.
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

// performHealthChecks checks health of all registered DMS plugins.
func (r *Registry) performHealthChecks(ctx context.Context) {
	r.mu.RLock()
	plugins := make(map[string]adapter.DMSAdapter, len(r.plugins))
	for name, p := range r.plugins {
		plugins[name] = p
	}
	r.mu.RUnlock()

	for name, plugin := range plugins {
		r.checkPluginHealth(ctx, name, plugin)
	}
}

// checkPluginHealth performs a health check on a single DMS plugin.
func (r *Registry) checkPluginHealth(ctx context.Context, name string, plugin adapter.DMSAdapter) {
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

		// Log health status changes.
		if previouslyHealthy != healthy {
			if healthy {
				r.logger.Info("DMS plugin recovered",
					zap.String("plugin", name),
				)
			} else {
				r.logger.Warn("DMS plugin unhealthy",
					zap.String("plugin", name),
					zap.Error(err),
				)
			}
		}
	}
	r.mu.Unlock()
}

// Close closes all registered DMS plugins and stops health checks.
func (r *Registry) Close() error {
	r.StopHealthChecks()

	r.mu.Lock()
	defer r.mu.Unlock()

	var lastErr error
	for name, plugin := range r.plugins {
		if err := plugin.Close(); err != nil {
			r.logger.Error("error closing DMS plugin",
				zap.String("plugin", name),
				zap.Error(err),
			)
			lastErr = err
		}
	}

	r.plugins = make(map[string]adapter.DMSAdapter)
	r.meta = make(map[string]*PluginMetadata)
	r.defaultPlugin = ""

	r.logger.Info("DMS registry closed")

	return lastErr
}
