// Package handlers provides HTTP request handlers for the O2-IMS Gateway.
package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// FrontendPluginInfo represents a frontend plugin for API responses.
type FrontendPluginInfo struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"displayName"`
	Enabled     bool     `json:"enabled"`
	BasePaths   []string `json:"basePaths"`
}

// PluginRegistry defines the interface for frontend plugin management.
// This decouples the handler from the concrete server.FrontendPluginRegistry type.
type PluginRegistry interface {
	List() []FrontendPluginInfo
	Get(name string) (*FrontendPluginInfo, error)
	Enable(name string) error
	Disable(name string) error
}

// PluginHandler handles admin API requests for frontend plugin management.
type PluginHandler struct {
	registry PluginRegistry
	logger   *zap.Logger
}

// NewPluginHandler creates a new PluginHandler with the given registry and logger.
func NewPluginHandler(registry PluginRegistry, logger *zap.Logger) *PluginHandler {
	return &PluginHandler{
		registry: registry,
		logger:   logger,
	}
}

// ListPlugins returns all registered frontend plugins.
// GET /admin/platform/plugins.
func (h *PluginHandler) ListPlugins(c *gin.Context) {
	plugins := h.registry.List()

	c.JSON(http.StatusOK, gin.H{
		"plugins": plugins,
	})
}

// GetPlugin returns a single frontend plugin by name.
// GET /admin/platform/plugins/:name.
func (h *PluginHandler) GetPlugin(c *gin.Context) {
	name := c.Param("name")

	plugin, err := h.registry.Get(name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "NotFound",
			"message": "Plugin not found: " + name,
		})
		return
	}

	c.JSON(http.StatusOK, plugin)
}

// pluginUpdateRequest is the request body for PUT /admin/platform/plugins/:name.
type pluginUpdateRequest struct {
	Enabled bool `json:"enabled"`
}

// UpdatePlugin enables or disables a frontend plugin.
// PUT /admin/platform/plugins/:name.
func (h *PluginHandler) UpdatePlugin(c *gin.Context) {
	name := c.Param("name")

	var req pluginUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": "Invalid request body",
		})
		return
	}

	if req.Enabled {
		if err := h.registry.Enable(name); err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "NotFound",
				"message": "Plugin not found: " + name,
			})
			return
		}
		h.logger.Info("frontend plugin enabled", zap.String("plugin", name))
	} else {
		if err := h.registry.Disable(name); err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "NotFound",
				"message": "Plugin not found: " + name,
			})
			return
		}
		h.logger.Info("frontend plugin disabled", zap.String("plugin", name))
	}

	// Return the updated plugin state.
	plugin, err := h.registry.Get(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to retrieve plugin after update",
		})
		return
	}

	c.JSON(http.StatusOK, plugin)
}
