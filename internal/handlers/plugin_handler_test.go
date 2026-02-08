package handlers_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/handlers"
)

// mockPluginRegistry implements handlers.PluginRegistry for testing.
type mockPluginRegistry struct {
	plugins map[string]*handlers.FrontendPluginInfo
}

func newMockPluginRegistry() *mockPluginRegistry {
	return &mockPluginRegistry{
		plugins: map[string]*handlers.FrontendPluginInfo{
			"o2ims": {
				Name:        "o2ims",
				DisplayName: "O2-IMS Infrastructure Inventory",
				Enabled:     false,
				BasePaths:   []string{"/o2ims-infrastructureInventory/v1"},
			},
			"o2dms": {
				Name:        "o2dms",
				DisplayName: "O2-DMS Deployment Management",
				Enabled:     false,
				BasePaths:   []string{"/o2dms/v1"},
			},
			"o2smo": {
				Name:        "o2smo",
				DisplayName: "O2-SMO Service Management & Orchestration",
				Enabled:     false,
				BasePaths:   []string{"/o2smo/v1"},
			},
			"tmforum": {
				Name:        "tmforum",
				DisplayName: "TMForum Open APIs",
				Enabled:     false,
				BasePaths:   []string{"/tmf-api"},
			},
			"graphql": {
				Name:        "graphql",
				DisplayName: "GraphQL API",
				Enabled:     false,
				BasePaths:   []string{"/graphql"},
			},
		},
	}
}

func (m *mockPluginRegistry) List() []handlers.FrontendPluginInfo {
	result := make([]handlers.FrontendPluginInfo, 0, len(m.plugins))
	for _, p := range m.plugins {
		result = append(result, *p)
	}
	return result
}

func (m *mockPluginRegistry) Get(name string) (*handlers.FrontendPluginInfo, error) {
	p, ok := m.plugins[name]
	if !ok {
		return nil, errors.New("plugin not found: " + name)
	}
	cp := *p
	return &cp, nil
}

func (m *mockPluginRegistry) Enable(name string) error {
	p, ok := m.plugins[name]
	if !ok {
		return errors.New("plugin not found: " + name)
	}
	p.Enabled = true
	return nil
}

func (m *mockPluginRegistry) Disable(name string) error {
	p, ok := m.plugins[name]
	if !ok {
		return errors.New("plugin not found: " + name)
	}
	p.Enabled = false
	return nil
}

func setupPluginRouter(registry handlers.PluginRegistry) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := handlers.NewPluginHandler(registry, zap.NewNop())

	plugins := router.Group("/admin/platform/plugins")
	{
		plugins.GET("", handler.ListPlugins)
		plugins.GET("/:name", handler.GetPlugin)
		plugins.PUT("/:name", handler.UpdatePlugin)
	}

	return router
}

func TestPluginHandler_ListPlugins(t *testing.T) {
	registry := newMockPluginRegistry()
	router := setupPluginRouter(registry)

	req := httptest.NewRequest(http.MethodGet, "/admin/platform/plugins", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)

	plugins, ok := body["plugins"].([]interface{})
	require.True(t, ok, "plugins should be an array")
	assert.Len(t, plugins, 5, "should have 5 plugins")
}

func TestPluginHandler_GetPlugin(t *testing.T) {
	tests := []struct {
		name           string
		pluginName     string
		expectedStatus int
		checkBody      func(t *testing.T, body map[string]interface{})
	}{
		{
			name:           "returns existing plugin",
			pluginName:     "o2ims",
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, body map[string]interface{}) {
				t.Helper()
				assert.Equal(t, "o2ims", body["name"])
				assert.Equal(t, "O2-IMS Infrastructure Inventory", body["displayName"])
				assert.Equal(t, false, body["enabled"])
			},
		},
		{
			name:           "returns 404 for unknown plugin",
			pluginName:     "nonexistent",
			expectedStatus: http.StatusNotFound,
			checkBody: func(t *testing.T, body map[string]interface{}) {
				t.Helper()
				assert.Equal(t, "NotFound", body["error"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := newMockPluginRegistry()
			router := setupPluginRouter(registry)

			req := httptest.NewRequest(http.MethodGet, "/admin/platform/plugins/"+tt.pluginName, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var body map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &body)
			require.NoError(t, err)
			tt.checkBody(t, body)
		})
	}
}

func TestPluginHandler_UpdatePlugin_Enable(t *testing.T) {
	registry := newMockPluginRegistry()
	router := setupPluginRouter(registry)

	body := `{"enabled": true}`
	req := httptest.NewRequest(http.MethodPut, "/admin/platform/plugins/o2ims", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, true, resp["enabled"])
	assert.Equal(t, "o2ims", resp["name"])
}

func TestPluginHandler_UpdatePlugin_Disable(t *testing.T) {
	registry := newMockPluginRegistry()
	// Pre-enable the plugin.
	err := registry.Enable("o2ims")
	require.NoError(t, err)

	router := setupPluginRouter(registry)

	body := `{"enabled": false}`
	req := httptest.NewRequest(http.MethodPut, "/admin/platform/plugins/o2ims", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, false, resp["enabled"])
}

func TestPluginHandler_UpdatePlugin_InvalidName(t *testing.T) {
	registry := newMockPluginRegistry()
	router := setupPluginRouter(registry)

	body := `{"enabled": true}`
	req := httptest.NewRequest(http.MethodPut, "/admin/platform/plugins/nonexistent", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestPluginHandler_UpdatePlugin_InvalidBody(t *testing.T) {
	registry := newMockPluginRegistry()
	router := setupPluginRouter(registry)

	body := `not-json`
	req := httptest.NewRequest(http.MethodPut, "/admin/platform/plugins/o2ims", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
