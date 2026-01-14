package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/piwi3910/netweave/internal/server"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestServer creates a minimal server.server for testing documentation handlers.
func createTestServer() *server.Server {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	return server.NewTestServerWithRouter(router, nil)
}

func TestDocsHandlers(t *testing.T) {
	// Set Gin to test mode
	gin.SetMode(gin.TestMode)

	testSpec := []byte(`openapi: 3.0.3
info:
  title: Test API
  version: 1.0.0
paths: {}`)

	t.Run("handleOpenAPIYAML with spec loaded", func(t *testing.T) {
		srv := createTestServer()
		srv.SetOpenAPISpec(testSpec)
		srv.Router().GET("/openapi.yaml", srv.HandleOpenAPIYAML)

		req := httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil)
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/x-yaml", w.Header().Get("Content-Type"))
		assert.Equal(t, "public, max-age=3600", w.Header().Get("Cache-Control"))
		assert.Equal(t, string(testSpec), w.Body.String())
	})

	t.Run("handleOpenAPIYAML without spec loaded", func(t *testing.T) {
		srv := createTestServer()
		// Don't set OpenAPISpec
		srv.Router().GET("/openapi.yaml", srv.HandleOpenAPIYAML)

		req := httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil)
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Body.String(), "OpenAPI specification not loaded")
	})

	t.Run("handleOpenAPIJSON redirects to YAML", func(t *testing.T) {
		srv := createTestServer()
		srv.Router().GET("/openapi.json", srv.HandleOpenAPIJSON)

		req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)

		// Should redirect to YAML endpoint
		assert.Equal(t, http.StatusPermanentRedirect, w.Code)
		assert.Equal(t, "/docs/openapi.yaml", w.Header().Get("Location"))
	})

	t.Run("handleSwaggerUIRedirect", func(t *testing.T) {
		srv := createTestServer()
		srv.Router().GET("/docs", srv.HandleSwaggerUIRedirect)

		req := httptest.NewRequest(http.MethodGet, "/docs", nil)
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)

		assert.Equal(t, http.StatusMovedPermanently, w.Code)
		assert.Equal(t, "/docs/", w.Header().Get("Location"))
	})

	t.Run("handleSwaggerUI returns HTML page", func(t *testing.T) {
		srv := createTestServer()
		srv.Router().GET("/docs/", srv.HandleSwaggerUI)

		req := httptest.NewRequest(http.MethodGet, "/docs/", nil)
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "text/html; charset=utf-8", w.Header().Get("Content-Type"))

		body := w.Body.String()
		assert.Contains(t, body, "<!DOCTYPE html>")
		assert.Contains(t, body, "O2-IMS API Documentation")
		assert.Contains(t, body, "swagger-ui")
		assert.Contains(t, body, "/docs/openapi.yaml")
		// Verify pinned version is used
		assert.Contains(t, body, "swagger-ui-dist@5.11.0")
	})
}

func TestSetupDocsRoutes(t *testing.T) {
	// Set Gin to test mode
	gin.SetMode(gin.TestMode)

	testSpec := []byte(`openapi: 3.0.3
info:
  title: Test API
  version: 1.0.0`)

	srv := createTestServer()
	srv.SetOpenAPISpec(testSpec)
	srv.SetupDocsRoutes()

	tests := []struct {
		name           string
		path           string
		expectedStatus int
		checkBody      func(t *testing.T, body string)
	}{
		{
			name:           "docs redirect",
			path:           "/docs",
			expectedStatus: http.StatusMovedPermanently,
		},
		{
			name:           "swagger UI",
			path:           "/docs/",
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, "swagger-ui")
			},
		},
		{
			name:           "openapi yaml in docs",
			path:           "/docs/openapi.yaml",
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, "openapi:")
			},
		},
		{
			name:           "openapi yaml at root",
			path:           "/openapi.yaml",
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, body string) {
				t.Helper()
				assert.Contains(t, body, "openapi:")
			},
		},
		{
			name:           "openapi json redirects to yaml",
			path:           "/openapi.json",
			expectedStatus: http.StatusPermanentRedirect,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			w := httptest.NewRecorder()
			srv.Router().ServeHTTP(w, req)

			assert.Equal(t, tc.expectedStatus, w.Code)

			if tc.checkBody != nil {
				tc.checkBody(t, w.Body.String())
			}
		})
	}
}

func TestOpenAPISpecContent(t *testing.T) {
	// Set Gin to test mode
	gin.SetMode(gin.TestMode)

	// Create a comprehensive test spec that mimics the real one
	testSpec := `openapi: 3.0.3
info:
  title: O2-IMS API
  version: 1.0.0
paths:
  /subscriptions:
    get:
      summary: List subscriptions
  /resourcePools:
    get:
      summary: List resource pools
components:
  schemas:
    Subscription:
      type: object`

	srv := createTestServer()
	srv.SetOpenAPISpec([]byte(testSpec))
	srv.Router().GET("/openapi.yaml", srv.HandleOpenAPIYAML)

	req := httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	body := w.Body.String()
	// Verify key API elements are present
	assert.True(t, strings.Contains(body, "/subscriptions"))
	assert.True(t, strings.Contains(body, "/resourcePools"))
	assert.True(t, strings.Contains(body, "O2-IMS API"))
}

func TestGetOpenAPISpec(t *testing.T) {
	srv := createTestServer()

	// Initially nil
	assert.Nil(t, srv.GetOpenAPISpec())

	// After setting
	testSpec := []byte("test spec")
	srv.SetOpenAPISpec(testSpec)
	assert.Equal(t, testSpec, srv.GetOpenAPISpec())
}

func TestSwaggerUIVersionPinning(t *testing.T) {
	// Verify that version constants are properly defined
	assert.Equal(t, "5.11.0", server.SwaggerUIVersion)
	assert.Contains(t, server.SwaggerUICSSURL, server.SwaggerUIVersion)
	assert.Contains(t, server.SwaggerUIBundleURL, server.SwaggerUIVersion)
	assert.Contains(t, server.SwaggerUIPresetURL, server.SwaggerUIVersion)
}

func TestSwaggerUISecurityHeaders(t *testing.T) {
	// Set Gin to test mode
	gin.SetMode(gin.TestMode)

	srv := createTestServer()
	srv.Router().GET("/docs/", srv.HandleSwaggerUI)

	req := httptest.NewRequest(http.MethodGet, "/docs/", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify security headers
	assert.Equal(t, "text/html; charset=utf-8", w.Header().Get("Content-Type"))
	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", w.Header().Get("X-Frame-Options"))
	assert.Equal(t, "strict-origin-when-cross-origin", w.Header().Get("Referrer-Policy"))

	// Verify CSP is set and contains required directives
	csp := w.Header().Get("Content-Security-Policy")
	assert.NotEmpty(t, csp)
	assert.Contains(t, csp, "default-src 'self'")
	assert.Contains(t, csp, "script-src")
	assert.Contains(t, csp, "style-src")
	assert.Contains(t, csp, "https://unpkg.com")
}

func TestSwaggerUISRIHashes(t *testing.T) {
	// Set Gin to test mode
	gin.SetMode(gin.TestMode)

	srv := createTestServer()
	srv.Router().GET("/docs/", srv.HandleSwaggerUI)

	req := httptest.NewRequest(http.MethodGet, "/docs/", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)

	body := w.Body.String()

	// Verify SRI hashes are present
	assert.Contains(t, body, server.SwaggerUICSSSRI)
	assert.Contains(t, body, server.SwaggerUIBundleSRI)
	assert.Contains(t, body, server.SwaggerUIPresetSRI)

	// Verify integrity attributes are properly formatted
	assert.Contains(t, body, `integrity="`+server.SwaggerUICSSSRI+`"`)
	assert.Contains(t, body, `integrity="`+server.SwaggerUIBundleSRI+`"`)
	assert.Contains(t, body, `integrity="`+server.SwaggerUIPresetSRI+`"`)

	// Verify crossorigin attributes
	assert.Equal(t, 3, strings.Count(body, `crossorigin="anonymous"`))
}

func TestSwaggerUICSPConstants(t *testing.T) {
	// Verify CSP constant is properly defined
	assert.NotEmpty(t, server.SwaggerUICSP)
	assert.Contains(t, server.SwaggerUICSP, "default-src 'self'")
	assert.Contains(t, server.SwaggerUICSP, "script-src")
	assert.Contains(t, server.SwaggerUICSP, "style-src")
	assert.Contains(t, server.SwaggerUICSP, "img-src")
	assert.Contains(t, server.SwaggerUICSP, "font-src")
	assert.Contains(t, server.SwaggerUICSP, "connect-src 'self'")
	assert.Contains(t, server.SwaggerUICSP, "https://unpkg.com")
}
