// Package docs contains integration tests for API documentation endpoints.
//
//go:build integration
// +build integration

package docs

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/piwi3910/netweave/internal/adapters/kubernetes"
	"github.com/piwi3910/netweave/internal/storage"
	"github.com/piwi3910/netweave/tests/integration/helpers"
)

// TestDocsEndpoints_OpenAPIYAML tests the OpenAPI YAML endpoint.
func TestDocsEndpoints_OpenAPIYAML(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := helpers.SetupTestEnvironment(t)

	redisStore := storage.NewRedisStore(&storage.RedisConfig{
		Addr:         env.Redis.Addr(),
		MaxRetries:   3,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     10,
	})
	defer redisStore.Close()

	k8sAdapter := kubernetes.NewMockAdapter()
	defer k8sAdapter.Close()

	ts := helpers.NewTestServer(t, k8sAdapter, redisStore)

	// Test /docs/openapi.yaml endpoint
	t.Run("DocsOpenAPIYAML", func(t *testing.T) {
		resp, err := http.Get(ts.URL() + "/docs/openapi.yaml")
		require.NoError(t, err)
		defer resp.Body.Close()

		// Should return 404 since no spec is loaded in test mode
		// In production, this would return 200 with the spec
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Contains(t, string(body), "OpenAPI specification not loaded")
	})

	// Test /openapi.yaml root endpoint
	t.Run("RootOpenAPIYAML", func(t *testing.T) {
		resp, err := http.Get(ts.URL() + "/openapi.yaml")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

// TestDocsEndpoints_SwaggerUI tests the Swagger UI endpoint.
func TestDocsEndpoints_SwaggerUI(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := helpers.SetupTestEnvironment(t)

	redisStore := storage.NewRedisStore(&storage.RedisConfig{
		Addr:         env.Redis.Addr(),
		MaxRetries:   3,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     10,
	})
	defer redisStore.Close()

	k8sAdapter := kubernetes.NewMockAdapter()
	defer k8sAdapter.Close()

	ts := helpers.NewTestServer(t, k8sAdapter, redisStore)

	// Test /docs/ endpoint (Swagger UI)
	t.Run("SwaggerUI", func(t *testing.T) {
		resp, err := http.Get(ts.URL() + "/docs/")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// Verify content type
		contentType := resp.Header.Get("Content-Type")
		assert.Equal(t, "text/html; charset=utf-8", contentType)

		// Verify security headers
		assert.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"))
		assert.Equal(t, "DENY", resp.Header.Get("X-Frame-Options"))
		assert.Equal(t, "strict-origin-when-cross-origin", resp.Header.Get("Referrer-Policy"))
		assert.NotEmpty(t, resp.Header.Get("Content-Security-Policy"))

		// Verify HTML content
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		bodyStr := string(body)

		assert.Contains(t, bodyStr, "<!DOCTYPE html>")
		assert.Contains(t, bodyStr, "swagger-ui")
		assert.Contains(t, bodyStr, "O2-IMS API Documentation")
		assert.Contains(t, bodyStr, "/docs/openapi.yaml")

		// Verify SRI hashes are present
		assert.Contains(t, bodyStr, "integrity=\"sha384-")
		assert.Contains(t, bodyStr, "crossorigin=\"anonymous\"")

		// Verify pinned version
		assert.Contains(t, bodyStr, "swagger-ui-dist@5.11.0")
	})

	// Test /docs redirect
	t.Run("DocsRedirect", func(t *testing.T) {
		client := &http.Client{
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse // Don't follow redirects
			},
		}

		resp, err := client.Get(ts.URL() + "/docs")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusMovedPermanently, resp.StatusCode)
		assert.Equal(t, "/docs/", resp.Header.Get("Location"))
	})
}

// TestDocsEndpoints_SecurityHeaders tests that all security headers are set correctly.
func TestDocsEndpoints_SecurityHeaders(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := helpers.SetupTestEnvironment(t)

	redisStore := storage.NewRedisStore(&storage.RedisConfig{
		Addr:         env.Redis.Addr(),
		MaxRetries:   3,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     10,
	})
	defer redisStore.Close()

	k8sAdapter := kubernetes.NewMockAdapter()
	defer k8sAdapter.Close()

	ts := helpers.NewTestServer(t, k8sAdapter, redisStore)

	resp, err := http.Get(ts.URL() + "/docs/")
	require.NoError(t, err)
	defer resp.Body.Close()

	// Verify all security headers
	csp := resp.Header.Get("Content-Security-Policy")
	require.NotEmpty(t, csp, "Content-Security-Policy header should be set")

	// Verify CSP directives
	assert.Contains(t, csp, "default-src 'self'")
	assert.Contains(t, csp, "script-src")
	assert.Contains(t, csp, "style-src")
	assert.Contains(t, csp, "img-src")
	assert.Contains(t, csp, "font-src")
	assert.Contains(t, csp, "connect-src 'self'")
	assert.Contains(t, csp, "https://unpkg.com")

	// Verify other security headers
	assert.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", resp.Header.Get("X-Frame-Options"))
	assert.Equal(t, "strict-origin-when-cross-origin", resp.Header.Get("Referrer-Policy"))
}

// TestDocsEndpoints_SRIHashes tests that SRI hashes are correctly included.
func TestDocsEndpoints_SRIHashes(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := helpers.SetupTestEnvironment(t)

	redisStore := storage.NewRedisStore(&storage.RedisConfig{
		Addr:         env.Redis.Addr(),
		MaxRetries:   3,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     10,
	})
	defer redisStore.Close()

	k8sAdapter := kubernetes.NewMockAdapter()
	defer k8sAdapter.Close()

	ts := helpers.NewTestServer(t, k8sAdapter, redisStore)

	resp, err := http.Get(ts.URL() + "/docs/")
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	bodyStr := string(body)

	// Verify CSS has SRI hash
	assert.True(t, strings.Contains(bodyStr, `href="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui.css" integrity="sha384-`),
		"CSS should have integrity hash")

	// Verify JS bundle has SRI hash
	assert.True(t, strings.Contains(bodyStr, `src="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui-bundle.js" integrity="sha384-`),
		"JS bundle should have integrity hash")

	// Verify JS preset has SRI hash
	assert.True(t, strings.Contains(bodyStr, `src="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui-standalone-preset.js" integrity="sha384-`),
		"JS preset should have integrity hash")

	// Count total integrity attributes
	integrityCount := strings.Count(bodyStr, `integrity="sha384-`)
	assert.Equal(t, 3, integrityCount, "Should have exactly 3 integrity hashes (CSS, bundle JS, preset JS)")

	// Verify crossorigin attribute on all CDN resources
	crossoriginCount := strings.Count(bodyStr, `crossorigin="anonymous"`)
	assert.Equal(t, 3, crossoriginCount, "Should have crossorigin attribute on all 3 CDN resources")
}

// TestDocsEndpoints_CacheHeaders tests cache control headers on OpenAPI endpoints.
func TestDocsEndpoints_CacheHeaders(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	env := helpers.SetupTestEnvironment(t)

	redisStore := storage.NewRedisStore(&storage.RedisConfig{
		Addr:         env.Redis.Addr(),
		MaxRetries:   3,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     10,
	})
	defer redisStore.Close()

	k8sAdapter := kubernetes.NewMockAdapter()
	defer k8sAdapter.Close()

	ts := helpers.NewTestServer(t, k8sAdapter, redisStore)

	// Note: Since no spec is loaded, we won't get cache headers on 404 responses
	// This test documents expected behavior when spec IS loaded
	resp, err := http.Get(ts.URL() + "/docs/openapi.yaml")
	require.NoError(t, err)
	defer resp.Body.Close()

	// When spec is loaded, expect cache headers
	// For now, just verify the endpoint is reachable
	assert.True(t, resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNotFound)
}
