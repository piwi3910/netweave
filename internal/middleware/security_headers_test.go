package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecurityHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		config         *SecurityHeadersConfig
		expectedHeader map[string]string
		notExpected    []string
	}{
		{
			name:   "default config adds all security headers",
			config: middleware.DefaultSecurityHeadersConfig(),
			expectedHeader: map[string]string{
				"X-Content-Type-Options":  "nosniff",
				"X-Frame-Options":         "DENY",
				"X-XSS-Protection":        "1; mode=block",
				"Content-Security-Policy": "default-src 'none'; frame-ancestors 'none'",
				"Referrer-Policy":         "strict-origin-when-cross-origin",
				"Cache-Control":           "no-store",
				"Permissions-Policy":      "geolocation=(), microphone=(), camera=()",
			},
			notExpected: []string{"Strict-Transport-Security"}, // TLS not enabled
		},
		{
			name: "HSTS header added when TLS enabled",
			config: &SecurityHeadersConfig{
				Enabled:               true,
				TLSEnabled:            true,
				HSTSMaxAge:            31536000,
				HSTSIncludeSubDomains: true,
				HSTSPreload:           false,
				ContentSecurityPolicy: "default-src 'none'",
				FrameOptions:          "DENY",
				ReferrerPolicy:        "strict-origin-when-cross-origin",
			},
			expectedHeader: map[string]string{
				"X-Content-Type-Options":    "nosniff",
				"Strict-Transport-Security": "max-age=31536000; includeSubDomains",
			},
		},
		{
			name: "HSTS header with preload",
			config: &SecurityHeadersConfig{
				Enabled:               true,
				TLSEnabled:            true,
				HSTSMaxAge:            63072000, // 2 years
				HSTSIncludeSubDomains: true,
				HSTSPreload:           true,
				ContentSecurityPolicy: "default-src 'none'",
				FrameOptions:          "DENY",
				ReferrerPolicy:        "strict-origin-when-cross-origin",
			},
			expectedHeader: map[string]string{
				"Strict-Transport-Security": "max-age=63072000; includeSubDomains; preload",
			},
		},
		{
			name: "custom frame options",
			config: &SecurityHeadersConfig{
				Enabled:               true,
				ContentSecurityPolicy: "default-src 'self'",
				FrameOptions:          "SAMEORIGIN",
				ReferrerPolicy:        "no-referrer",
			},
			expectedHeader: map[string]string{
				"X-Frame-Options":         "SAMEORIGIN",
				"Content-Security-Policy": "default-src 'self'",
				"Referrer-Policy":         "no-referrer",
			},
		},
		{
			name: "disabled config skips headers",
			config: &SecurityHeadersConfig{
				Enabled: false,
			},
			notExpected: []string{
				"X-Content-Type-Options",
				"X-Frame-Options",
				"X-XSS-Protection",
				"Content-Security-Policy",
				"Referrer-Policy",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(SecurityHeaders(tt.config))
			router.GET("/test", func(c *gin.Context) {
				c.String(http.StatusOK, "OK")
			})

			w := httptest.NewRecorder()
			req, err := http.NewRequest(http.MethodGet, "/test", nil)
			require.NoError(t, err)

			router.ServeHTTP(w, req)

			// Check expected headers
			for header, expectedValue := range tt.expectedHeader {
				assert.Equal(t, expectedValue, w.Header().Get(header),
					"header %s should have value %s", header, expectedValue)
			}

			// Check headers that should not be present
			for _, header := range tt.notExpected {
				assert.Empty(t, w.Header().Get(header),
					"header %s should not be present", header)
			}
		})
	}
}

func TestSecurityHeadersNilConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(SecurityHeaders(nil))
	router.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "OK")
	})

	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, "/test", nil)
	require.NoError(t, err)

	router.ServeHTTP(w, req)

	// Should use default config
	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", w.Header().Get("X-Frame-Options"))
}

func TestDefaultSecurityHeadersConfig(t *testing.T) {
	config := middleware.DefaultSecurityHeadersConfig()

	assert.True(t, config.Enabled)
	assert.Equal(t, 31536000, config.HSTSMaxAge)
	assert.True(t, config.HSTSIncludeSubDomains)
	assert.False(t, config.HSTSPreload)
	assert.Equal(t, "default-src 'none'; frame-ancestors 'none'", config.ContentSecurityPolicy)
	assert.Equal(t, "DENY", config.FrameOptions)
	assert.Equal(t, "strict-origin-when-cross-origin", config.ReferrerPolicy)
	assert.False(t, config.TLSEnabled)
}

func TestBuildHSTSValue(t *testing.T) {
	tests := []struct {
		name     string
		config   *SecurityHeadersConfig
		expected string
	}{
		{
			name: "basic max-age",
			config: &SecurityHeadersConfig{
				HSTSMaxAge:            31536000,
				HSTSIncludeSubDomains: false,
				HSTSPreload:           false,
			},
			expected: "max-age=31536000",
		},
		{
			name: "with includeSubDomains",
			config: &SecurityHeadersConfig{
				HSTSMaxAge:            31536000,
				HSTSIncludeSubDomains: true,
				HSTSPreload:           false,
			},
			expected: "max-age=31536000; includeSubDomains",
		},
		{
			name: "with preload",
			config: &SecurityHeadersConfig{
				HSTSMaxAge:            63072000,
				HSTSIncludeSubDomains: true,
				HSTSPreload:           true,
			},
			expected: "max-age=63072000; includeSubDomains; preload",
		},
		{
			name: "zero max-age",
			config: &SecurityHeadersConfig{
				HSTSMaxAge:            0,
				HSTSIncludeSubDomains: true,
				HSTSPreload:           false,
			},
			expected: "max-age=0; includeSubDomains",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildHSTSValue(tt.config)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestServerHeaderRemoved(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("server header is empty by default", func(t *testing.T) {
		router := gin.New()
		router.Use(SecurityHeaders(middleware.DefaultSecurityHeadersConfig()))
		router.GET("/test", func(c *gin.Context) {
			c.String(http.StatusOK, "OK")
		})

		w := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/test", nil)
		require.NoError(t, err)

		router.ServeHTTP(w, req)

		// Server header should be empty/not set
		assert.Empty(t, w.Header().Get("Server"))
	})

	t.Run("server header remains empty after full request cycle", func(t *testing.T) {
		router := gin.New()
		router.Use(SecurityHeaders(middleware.DefaultSecurityHeadersConfig()))

		// Simulate a handler that might try to set headers
		router.GET("/test", func(c *gin.Context) {
			// Handler sets some custom headers
			c.Header("X-Custom-Header", "custom-value")
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})

		w := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/test", nil)
		require.NoError(t, err)

		router.ServeHTTP(w, req)

		// Verify response was successful
		assert.Equal(t, http.StatusOK, w.Code)

		// Verify custom header was set
		assert.Equal(t, "custom-value", w.Header().Get("X-Custom-Header"))

		// Server header should still be empty after full request/response cycle
		assert.Empty(t, w.Header().Get("Server"),
			"Server header should remain empty throughout request lifecycle")
	})

	t.Run("all security headers present after full request cycle", func(t *testing.T) {
		router := gin.New()
		router.Use(SecurityHeaders(middleware.DefaultSecurityHeadersConfig()))
		router.GET("/test", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"data": "test"})
		})

		w := httptest.NewRecorder()
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/test", nil)
		require.NoError(t, err)

		router.ServeHTTP(w, req)

		// Verify all security headers are present after response
		assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
		assert.Equal(t, "DENY", w.Header().Get("X-Frame-Options"))
		assert.Equal(t, "1; mode=block", w.Header().Get("X-XSS-Protection"))
		assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
		assert.Empty(t, w.Header().Get("Server"))
	})
}
