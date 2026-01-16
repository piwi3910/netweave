// Package registry provides a unified plugin registry for O2-IMS/DMS/SMO adapters.
// This registry enables configuration-driven plugin management and intelligent routing.
package registry

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/zap"
)

// PluginCategory defines the category of a plugin.
type PluginCategory string

const (
	// CategoryIMS represents O2-IMS infrastructure management plugins.
	CategoryIMS PluginCategory = "ims"

	// CategoryDMS represents O2-DMS deployment management plugins.
	CategoryDMS PluginCategory = "dms"

	// CategorySMO represents O2-SMO orchestration plugins.
	CategorySMO PluginCategory = "smo"

	// CategoryObservability represents observability plugins (metrics, logs, traces).
	CategoryObservability PluginCategory = "observability"
)

// PluginStatus represents the operational status of a plugin.
type PluginStatus string

const (
	// StatusActive indicates the plugin is active and available.
	StatusActive PluginStatus = "active"

	// StatusDisabled indicates the plugin is disabled by configuration.
	StatusDisabled PluginStatus = "disabled"

	// StatusFailed indicates the plugin failed to initialize.
	StatusFailed PluginStatus = "failed"

	// StatusUnhealthy indicates the plugin health check is failing.
	StatusUnhealthy PluginStatus = "unhealthy"
)

// Plugin represents a registered plugin with metadata.
type Plugin struct {
	// Category is the plugin category (IMS, DMS, SMO, etc.)
	Category PluginCategory

	// Name is the unique identifier for the plugin within its category.
	Name string

	// Version is the plugin version string.
	Version string

	// Priority determines selection order when multiple plugins match (higher = preferred).
	Priority int

	// Status is the current operational status.
	Status PluginStatus

	// Capabilities lists the capabilities this plugin provides.
	Capabilities []string

	// Instance is the actual plugin instance (adapter, handler, etc.)
	Instance interface{}

	// Metadata contains additional plugin information.
	Metadata map[string]interface{}
}

// Registry is a unified plugin registry for all adapter types.
type Registry struct {
	mu      sync.RWMutex
	plugins map[PluginCategory]map[string]*Plugin
	logger  *zap.Logger
}

// New creates a new unified plugin registry.
func New(logger *zap.Logger) *Registry {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &Registry{
		plugins: make(map[PluginCategory]map[string]*Plugin),
		logger:  logger,
	}
}

// Register registers a plugin in the registry.
// Returns an error if a plugin with the same name already exists in the category.
func (r *Registry) Register(plugin *Plugin) error {
	if plugin == nil {
		return fmt.Errorf("plugin cannot be nil")
	}
	if plugin.Name == "" {
		return fmt.Errorf("plugin name cannot be empty")
	}
	if plugin.Category == "" {
		return fmt.Errorf("plugin category cannot be empty")
	}
	if plugin.Instance == nil {
		return fmt.Errorf("plugin instance cannot be nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Initialize category map if needed
	if r.plugins[plugin.Category] == nil {
		r.plugins[plugin.Category] = make(map[string]*Plugin)
	}

	// Check for duplicates
	if existing, exists := r.plugins[plugin.Category][plugin.Name]; exists {
		return fmt.Errorf("plugin %s/%s already registered (status: %s)",
			plugin.Category, plugin.Name, existing.Status)
	}

	r.plugins[plugin.Category][plugin.Name] = plugin
	r.logger.Info("registered plugin",
		zap.String("category", string(plugin.Category)),
		zap.String("name", plugin.Name),
		zap.String("version", plugin.Version),
		zap.String("status", string(plugin.Status)),
		zap.Int("priority", plugin.Priority),
	)

	return nil
}

// Unregister removes a plugin from the registry.
func (r *Registry) Unregister(category PluginCategory, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	categoryPlugins, exists := r.plugins[category]
	if !exists {
		return fmt.Errorf("no plugins registered for category %s", category)
	}

	if _, exists := categoryPlugins[name]; !exists {
		return fmt.Errorf("plugin %s/%s not found", category, name)
	}

	delete(categoryPlugins, name)
	r.logger.Info("unregistered plugin",
		zap.String("category", string(category)),
		zap.String("name", name),
	)

	return nil
}

// Get retrieves a specific plugin by category and name.
func (r *Registry) Get(category PluginCategory, name string) (*Plugin, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	categoryPlugins, exists := r.plugins[category]
	if !exists {
		return nil, fmt.Errorf("no plugins registered for category %s", category)
	}

	plugin, exists := categoryPlugins[name]
	if !exists {
		return nil, fmt.Errorf("plugin %s/%s not found", category, name)
	}

	return plugin, nil
}

// List returns all plugins in a category.
func (r *Registry) List(category PluginCategory) []*Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	categoryPlugins, exists := r.plugins[category]
	if !exists {
		return []*Plugin{}
	}

	plugins := make([]*Plugin, 0, len(categoryPlugins))
	for _, plugin := range categoryPlugins {
		plugins = append(plugins, plugin)
	}

	return plugins
}

// ListAll returns all registered plugins across all categories.
func (r *Registry) ListAll() map[PluginCategory][]*Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[PluginCategory][]*Plugin)
	for category, categoryPlugins := range r.plugins {
		plugins := make([]*Plugin, 0, len(categoryPlugins))
		for _, plugin := range categoryPlugins {
			plugins = append(plugins, plugin)
		}
		result[category] = plugins
	}

	return result
}

// UpdateStatus updates the status of a plugin.
func (r *Registry) UpdateStatus(category PluginCategory, name string, status PluginStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	categoryPlugins, exists := r.plugins[category]
	if !exists {
		return fmt.Errorf("no plugins registered for category %s", category)
	}

	plugin, exists := categoryPlugins[name]
	if !exists {
		return fmt.Errorf("plugin %s/%s not found", category, name)
	}

	oldStatus := plugin.Status
	plugin.Status = status

	r.logger.Info("updated plugin status",
		zap.String("category", string(category)),
		zap.String("name", name),
		zap.String("old_status", string(oldStatus)),
		zap.String("new_status", string(status)),
	)

	return nil
}

// SelectPlugin intelligently selects a plugin based on criteria.
// Returns the highest priority active plugin that matches the criteria.
func (r *Registry) SelectPlugin(category PluginCategory, criteria map[string]interface{}) (*Plugin, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	categoryPlugins, exists := r.plugins[category]
	if !exists || len(categoryPlugins) == 0 {
		return nil, fmt.Errorf("no plugins available for category %s", category)
	}

	var selected *Plugin
	highestPriority := -1

	for _, plugin := range categoryPlugins {
		// Skip non-active plugins
		if plugin.Status != StatusActive {
			continue
		}

		// Check if plugin matches criteria
		if !matchesCriteria(plugin, criteria) {
			continue
		}

		// Select highest priority plugin
		if plugin.Priority > highestPriority {
			selected = plugin
			highestPriority = plugin.Priority
		}
	}

	if selected == nil {
		return nil, fmt.Errorf("no active plugin found for category %s matching criteria", category)
	}

	return selected, nil
}

// HealthChecker interface for plugins that support health checks.
type HealthChecker interface {
	Health(context.Context) error
}

// HealthCheck performs health checks on all active plugins.
// Returns a map of plugin names to their health status.
func (r *Registry) HealthCheck(ctx context.Context) map[string]error {
	r.mu.RLock()
	allPlugins := make([]*Plugin, 0)
	for _, categoryPlugins := range r.plugins {
		for _, plugin := range categoryPlugins {
			if plugin.Status == StatusActive {
				allPlugins = append(allPlugins, plugin)
			}
		}
	}
	r.mu.RUnlock()

	results := make(map[string]error)
	var resultsMu sync.Mutex
	var wg sync.WaitGroup

	for _, plugin := range allPlugins {
		wg.Add(1)
		go func(p *Plugin) {
			defer wg.Done()

			key := fmt.Sprintf("%s/%s", p.Category, p.Name)

			// Try to call Health() if the plugin supports it
			healthChecker, ok := p.Instance.(HealthChecker)
			if !ok {
				// Plugin doesn't support health checks
				resultsMu.Lock()
				results[key] = nil
				resultsMu.Unlock()
				return
			}

			err := healthChecker.Health(ctx)

			resultsMu.Lock()
			results[key] = err
			resultsMu.Unlock()

			// Update status based on health check
			if err != nil {
				_ = r.UpdateStatus(p.Category, p.Name, StatusUnhealthy)
			} else {
				_ = r.UpdateStatus(p.Category, p.Name, StatusActive)
			}
		}(plugin)
	}

	wg.Wait()
	return results
}

// matchesCriteria checks if a plugin matches the given criteria.
func matchesCriteria(plugin *Plugin, criteria map[string]interface{}) bool {
	if len(criteria) == 0 {
		return true
	}

	// Check name match
	if name, ok := criteria["name"].(string); ok && name != plugin.Name {
		return false
	}

	// Check capability match
	if requiredCap, ok := criteria["capability"].(string); ok {
		hasCapability := false
		for _, cap := range plugin.Capabilities {
			if cap == requiredCap {
				hasCapability = true
				break
			}
		}
		if !hasCapability {
			return false
		}
	}

	// Check metadata match
	if metadata, ok := criteria["metadata"].(map[string]interface{}); ok {
		for key, value := range metadata {
			pluginValue, exists := plugin.Metadata[key]
			if !exists || pluginValue != value {
				return false
			}
		}
	}

	return true
}

// Stats returns statistics about the registry.
func (r *Registry) Stats() map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := map[string]interface{}{
		"total_plugins": 0,
		"by_category":   make(map[string]int),
		"by_status":     make(map[string]int),
	}

	totalPlugins := 0
	byCategory := make(map[string]int)
	byStatus := make(map[string]int)

	for category, categoryPlugins := range r.plugins {
		byCategory[string(category)] = len(categoryPlugins)
		totalPlugins += len(categoryPlugins)

		for _, plugin := range categoryPlugins {
			byStatus[string(plugin.Status)]++
		}
	}

	stats["total_plugins"] = totalPlugins
	stats["by_category"] = byCategory
	stats["by_status"] = byStatus

	return stats
}
