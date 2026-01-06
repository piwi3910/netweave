package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDocsHandlers(t *testing.T) {
	// Set Gin to test mode
	gin.SetMode(gin.TestMode)

	// Set up test OpenAPI spec
	testSpec := []byte(`openapi: 3.0.3
info:
  title: Test API
  version: 1.0.0
paths: {}`)

	// Save original spec and restore after test
	originalSpec := OpenAPISpec
	defer func() { OpenAPISpec = originalSpec }()

	t.Run("handleOpenAPIYAML with spec loaded", func(t *testing.T) {
		OpenAPISpec = testSpec

		router := gin.New()
		srv := &Server{router: router}
		router.GET("/openapi.yaml", srv.handleOpenAPIYAML)

		req := httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "application/x-yaml", w.Header().Get("Content-Type"))
		assert.Equal(t, "public, max-age=3600", w.Header().Get("Cache-Control"))
		assert.Equal(t, string(testSpec), w.Body.String())
	})

	t.Run("handleOpenAPIYAML without spec loaded", func(t *testing.T) {
		OpenAPISpec = nil

		router := gin.New()
		srv := &Server{router: router}
		router.GET("/openapi.yaml", srv.handleOpenAPIYAML)

		req := httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Body.String(), "OpenAPI specification not loaded")
	})

	t.Run("handleOpenAPIJSON redirects to YAML", func(t *testing.T) {
		OpenAPISpec = testSpec

		router := gin.New()
		srv := &Server{router: router}
		router.GET("/openapi.json", srv.handleOpenAPIJSON)

		req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusTemporaryRedirect, w.Code)
		assert.Equal(t, "/docs/openapi.yaml", w.Header().Get("Location"))
	})

	t.Run("handleOpenAPIJSON without spec loaded", func(t *testing.T) {
		OpenAPISpec = nil

		router := gin.New()
		srv := &Server{router: router}
		router.GET("/openapi.json", srv.handleOpenAPIJSON)

		req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Body.String(), "OpenAPI specification not loaded")
	})

	t.Run("handleSwaggerUIRedirect", func(t *testing.T) {
		router := gin.New()
		srv := &Server{router: router}
		router.GET("/docs", srv.handleSwaggerUIRedirect)

		req := httptest.NewRequest(http.MethodGet, "/docs", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusMovedPermanently, w.Code)
		assert.Equal(t, "/docs/", w.Header().Get("Location"))
	})

	t.Run("handleSwaggerUI returns HTML page", func(t *testing.T) {
		router := gin.New()
		srv := &Server{router: router}
		router.GET("/docs/", srv.handleSwaggerUI)

		req := httptest.NewRequest(http.MethodGet, "/docs/", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "text/html; charset=utf-8", w.Header().Get("Content-Type"))

		body := w.Body.String()
		assert.Contains(t, body, "<!DOCTYPE html>")
		assert.Contains(t, body, "O2-IMS API Documentation")
		assert.Contains(t, body, "swagger-ui")
		assert.Contains(t, body, "/docs/openapi.yaml")
	})
}

func TestSetupDocsRoutes(t *testing.T) {
	// Set Gin to test mode
	gin.SetMode(gin.TestMode)

	// Set up test OpenAPI spec
	testSpec := []byte(`openapi: 3.0.3
info:
  title: Test API
  version: 1.0.0`)
	originalSpec := OpenAPISpec
	defer func() { OpenAPISpec = originalSpec }()
	OpenAPISpec = testSpec

	// Create a minimal server for testing routes
	router := gin.New()
	srv := &Server{router: router}
	srv.setupDocsRoutes()

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
				assert.Contains(t, body, "swagger-ui")
			},
		},
		{
			name:           "openapi yaml in docs",
			path:           "/docs/openapi.yaml",
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, body string) {
				assert.Contains(t, body, "openapi:")
			},
		},
		{
			name:           "openapi yaml at root",
			path:           "/openapi.yaml",
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, body string) {
				assert.Contains(t, body, "openapi:")
			},
		},
		{
			name:           "openapi json redirects to yaml",
			path:           "/openapi.json",
			expectedStatus: http.StatusTemporaryRedirect,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

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

	originalSpec := OpenAPISpec
	defer func() { OpenAPISpec = originalSpec }()
	OpenAPISpec = []byte(testSpec)

	router := gin.New()
	srv := &Server{router: router}
	router.GET("/openapi.yaml", srv.handleOpenAPIYAML)

	req := httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	body := w.Body.String()
	// Verify key API elements are present
	assert.True(t, strings.Contains(body, "/subscriptions"))
	assert.True(t, strings.Contains(body, "/resourcePools"))
	assert.True(t, strings.Contains(body, "O2-IMS API"))
}
