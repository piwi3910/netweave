package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/piwi3910/netweave/internal/config"
	"github.com/piwi3910/netweave/internal/server"
)

func TestNewFrontendPluginRegistry_AllDisabledByDefault(t *testing.T) {
	cfg := config.FrontendPlugins{}
	registry := server.NewFrontendPluginRegistry(cfg)

	plugins := registry.List()
	require.Len(t, plugins, 5, "should have exactly 5 plugins")

	for _, p := range plugins {
		assert.False(t, p.Enabled, "plugin %s should be disabled by default", p.Name)
	}
}

func TestNewFrontendPluginRegistry_ConfigOverrides(t *testing.T) {
	cfg := config.FrontendPlugins{
		O2IMS:   config.FrontendPluginConfig{Enabled: true},
		GraphQL: config.FrontendPluginConfig{Enabled: true},
	}
	registry := server.NewFrontendPluginRegistry(cfg)

	assert.True(t, registry.IsEnabled("o2ims"), "o2ims should be enabled from config")
	assert.True(t, registry.IsEnabled("graphql"), "graphql should be enabled from config")
	assert.False(t, registry.IsEnabled("o2dms"), "o2dms should remain disabled")
	assert.False(t, registry.IsEnabled("o2smo"), "o2smo should remain disabled")
	assert.False(t, registry.IsEnabled("tmforum"), "tmforum should remain disabled")
}

func TestFrontendPluginRegistry_IsEnabled_UnknownPlugin(t *testing.T) {
	cfg := config.FrontendPlugins{}
	registry := server.NewFrontendPluginRegistry(cfg)

	assert.False(t, registry.IsEnabled("nonexistent"), "unknown plugin should return false")
}

func TestFrontendPluginRegistry_EnableDisable(t *testing.T) {
	cfg := config.FrontendPlugins{}
	registry := server.NewFrontendPluginRegistry(cfg)

	require.False(t, registry.IsEnabled("o2ims"))

	err := registry.Enable("o2ims")
	require.NoError(t, err)
	assert.True(t, registry.IsEnabled("o2ims"))

	err = registry.Disable("o2ims")
	require.NoError(t, err)
	assert.False(t, registry.IsEnabled("o2ims"))
}

func TestFrontendPluginRegistry_Enable_UnknownPlugin(t *testing.T) {
	cfg := config.FrontendPlugins{}
	registry := server.NewFrontendPluginRegistry(cfg)

	err := registry.Enable("nonexistent")
	require.Error(t, err)
	assert.ErrorIs(t, err, server.ErrPluginNotFound)
}

func TestFrontendPluginRegistry_Disable_UnknownPlugin(t *testing.T) {
	cfg := config.FrontendPlugins{}
	registry := server.NewFrontendPluginRegistry(cfg)

	err := registry.Disable("nonexistent")
	require.Error(t, err)
	assert.ErrorIs(t, err, server.ErrPluginNotFound)
}

func TestFrontendPluginRegistry_List(t *testing.T) {
	cfg := config.FrontendPlugins{
		O2IMS: config.FrontendPluginConfig{Enabled: true},
	}
	registry := server.NewFrontendPluginRegistry(cfg)

	plugins := registry.List()
	require.Len(t, plugins, 5)

	// Verify sorted by name.
	names := make([]string, len(plugins))
	for i, p := range plugins {
		names[i] = p.Name
	}
	assert.Equal(t, []string{"graphql", "o2dms", "o2ims", "o2smo", "tmforum"}, names)

	// Verify o2ims is enabled.
	for _, p := range plugins {
		if p.Name == "o2ims" {
			assert.True(t, p.Enabled)
			assert.Equal(t, "O2-IMS Infrastructure Inventory", p.DisplayName)
			assert.Equal(t, []string{"/o2ims-infrastructureInventory/v1"}, p.BasePaths)
		}
	}
}

func TestFrontendPluginRegistry_Get(t *testing.T) {
	cfg := config.FrontendPlugins{}
	registry := server.NewFrontendPluginRegistry(cfg)

	t.Run("returns existing plugin", func(t *testing.T) {
		p, err := registry.Get("o2dms")
		require.NoError(t, err)
		assert.Equal(t, "o2dms", p.Name)
		assert.Equal(t, "O2-DMS Deployment Management", p.DisplayName)
		assert.Equal(t, []string{"/o2dms/v1"}, p.BasePaths)
		assert.False(t, p.Enabled)
	})

	t.Run("returns error for unknown plugin", func(t *testing.T) {
		_, err := registry.Get("nonexistent")
		require.Error(t, err)
		assert.ErrorIs(t, err, server.ErrPluginNotFound)
	})
}

func TestFrontendPluginRegistry_ConcurrentAccess(t *testing.T) {
	cfg := config.FrontendPlugins{}
	registry := server.NewFrontendPluginRegistry(cfg)

	var wg sync.WaitGroup
	pluginNames := []string{"o2ims", "o2dms", "o2smo", "tmforum", "graphql"}

	// Spawn goroutines that concurrently enable and disable plugins.
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			name := pluginNames[idx%len(pluginNames)]

			if idx%2 == 0 {
				_ = registry.Enable(name)
			} else {
				_ = registry.Disable(name)
			}
			registry.IsEnabled(name)
			registry.List()
			_, _ = registry.Get(name)
		}(i)
	}

	wg.Wait()
	// No race condition panic means the test passes.
}

func TestPluginGuard_DisabledPlugin_Returns404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := config.FrontendPlugins{}
	registry := server.NewFrontendPluginRegistry(cfg)

	router := gin.New()
	router.GET("/test", server.PluginGuard(registry, "o2ims"), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var body map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)
	assert.Equal(t, "NotFound", body["error"])
	assert.Contains(t, body["message"], "o2ims")
	assert.Contains(t, body["message"], "not enabled")
}

func TestPluginGuard_EnabledPlugin_PassesThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := config.FrontendPlugins{
		O2IMS: config.FrontendPluginConfig{Enabled: true},
	}
	registry := server.NewFrontendPluginRegistry(cfg)

	router := gin.New()
	router.GET("/test", server.PluginGuard(registry, "o2ims"), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestPluginGuard_NilRegistry_PassesThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/test", server.PluginGuard(nil, "o2ims"), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestPluginGuard_RuntimeToggle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := config.FrontendPlugins{}
	registry := server.NewFrontendPluginRegistry(cfg)

	router := gin.New()
	router.GET("/test", server.PluginGuard(registry, "tmforum"), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// Initially disabled.
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Enable at runtime.
	err := registry.Enable("tmforum")
	require.NoError(t, err)

	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Disable again at runtime.
	err = registry.Disable("tmforum")
	require.NoError(t, err)

	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}
