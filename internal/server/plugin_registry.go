package server

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/piwi3910/netweave/internal/config"
	"github.com/piwi3910/netweave/internal/handlers"
)

// ErrPluginNotFound is returned when a plugin name is not recognized.
var ErrPluginNotFound = errors.New("plugin not found")

// FrontendPlugin represents a toggleable API frontend plugin.
type FrontendPlugin struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"displayName"`
	Enabled     bool     `json:"enabled"`
	BasePaths   []string `json:"basePaths"`
}

// FrontendPluginRegistry is a thread-safe registry of frontend plugins.
// It tracks which API frontends are enabled and supports runtime toggling.
type FrontendPluginRegistry struct {
	mu      sync.RWMutex
	plugins map[string]*FrontendPlugin
}

// NewFrontendPluginRegistry creates a new FrontendPluginRegistry initialized from config.
// All five toggleable plugins are registered with their initial enabled state from config.
func NewFrontendPluginRegistry(cfg config.FrontendPlugins) *FrontendPluginRegistry {
	r := &FrontendPluginRegistry{
		plugins: make(map[string]*FrontendPlugin, 5),
	}

	r.plugins["o2ims"] = &FrontendPlugin{
		Name:        "o2ims",
		DisplayName: "O2-IMS Infrastructure Inventory",
		Enabled:     cfg.O2IMS.Enabled,
		BasePaths:   []string{"/o2ims-infrastructureInventory/v1"},
	}
	r.plugins["o2dms"] = &FrontendPlugin{
		Name:        "o2dms",
		DisplayName: "O2-DMS Deployment Management",
		Enabled:     cfg.O2DMS.Enabled,
		BasePaths:   []string{"/o2dms/v1"},
	}
	r.plugins["o2smo"] = &FrontendPlugin{
		Name:        "o2smo",
		DisplayName: "O2-SMO Service Management & Orchestration",
		Enabled:     cfg.O2SMO.Enabled,
		BasePaths:   []string{"/o2smo/v1"},
	}
	r.plugins["tmforum"] = &FrontendPlugin{
		Name:        "tmforum",
		DisplayName: "TMForum Open APIs",
		Enabled:     cfg.TMForum.Enabled,
		BasePaths:   []string{"/tmf-api"},
	}
	r.plugins["graphql"] = &FrontendPlugin{
		Name:        "graphql",
		DisplayName: "GraphQL API",
		Enabled:     cfg.GraphQL.Enabled,
		BasePaths:   []string{"/graphql"},
	}

	return r
}

// IsEnabled returns whether the named plugin is enabled.
// Returns false for unknown plugin names.
func (r *FrontendPluginRegistry) IsEnabled(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.plugins[name]
	if !ok {
		return false
	}
	return p.Enabled
}

// Enable enables a plugin by name.
// Returns ErrPluginNotFound if the name is not recognized.
func (r *FrontendPluginRegistry) Enable(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, ok := r.plugins[name]
	if !ok {
		return fmt.Errorf("%w: %s", ErrPluginNotFound, name)
	}
	p.Enabled = true
	return nil
}

// Disable disables a plugin by name.
// Returns ErrPluginNotFound if the name is not recognized.
func (r *FrontendPluginRegistry) Disable(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, ok := r.plugins[name]
	if !ok {
		return fmt.Errorf("%w: %s", ErrPluginNotFound, name)
	}
	p.Enabled = false
	return nil
}

// List returns a snapshot of all registered plugins sorted by name.
func (r *FrontendPluginRegistry) List() []FrontendPlugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]FrontendPlugin, 0, len(r.plugins))
	for _, p := range r.plugins {
		result = append(result, *p)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result
}

// Get returns a copy of the named plugin.
// Returns ErrPluginNotFound if the name is not recognized.
func (r *FrontendPluginRegistry) Get(name string) (*FrontendPlugin, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.plugins[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrPluginNotFound, name)
	}

	// Return a copy to avoid data races on the caller side.
	cp := *p
	cpPaths := make([]string, len(p.BasePaths))
	copy(cpPaths, p.BasePaths)
	cp.BasePaths = cpPaths
	return &cp, nil
}

// pluginRegistryAdapter wraps FrontendPluginRegistry to implement handlers.PluginRegistry.
// This bridges the server and handlers packages without circular imports.
type pluginRegistryAdapter struct {
	registry *FrontendPluginRegistry
}

// newPluginRegistryAdapter creates a new adapter wrapping the given registry.
func newPluginRegistryAdapter(registry *FrontendPluginRegistry) *pluginRegistryAdapter {
	return &pluginRegistryAdapter{registry: registry}
}

// List returns all plugins as handler-compatible FrontendPluginInfo values.
func (a *pluginRegistryAdapter) List() []handlers.FrontendPluginInfo {
	plugins := a.registry.List()
	result := make([]handlers.FrontendPluginInfo, len(plugins))
	for i, p := range plugins {
		result[i] = toPluginInfo(p)
	}
	return result
}

// Get returns a single plugin as a handler-compatible FrontendPluginInfo.
func (a *pluginRegistryAdapter) Get(name string) (*handlers.FrontendPluginInfo, error) {
	p, err := a.registry.Get(name)
	if err != nil {
		return nil, err
	}
	info := toPluginInfo(*p)
	return &info, nil
}

// Enable enables a plugin by name.
func (a *pluginRegistryAdapter) Enable(name string) error {
	return a.registry.Enable(name)
}

// Disable disables a plugin by name.
func (a *pluginRegistryAdapter) Disable(name string) error {
	return a.registry.Disable(name)
}

// toPluginInfo converts a FrontendPlugin to handlers.FrontendPluginInfo.
func toPluginInfo(p FrontendPlugin) handlers.FrontendPluginInfo {
	return handlers.FrontendPluginInfo{
		Name:        p.Name,
		DisplayName: p.DisplayName,
		Enabled:     p.Enabled,
		BasePaths:   p.BasePaths,
	}
}

// PluginGuard returns Gin middleware that blocks requests when the named plugin is disabled.
// If the registry is nil, the guard is a no-op (all requests pass through).
// This preserves backward compatibility for tests that do not set up a plugin registry.
func PluginGuard(registry *FrontendPluginRegistry, pluginName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// nil registry means all plugins enabled (backward compatibility).
		if registry == nil {
			c.Next()
			return
		}

		if !registry.IsEnabled(pluginName) {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "NotFound",
				"message": fmt.Sprintf("The %s API is not enabled. Contact your platform administrator.", pluginName),
			})
			c.Abort()
			return
		}
		c.Next()
	}
}
