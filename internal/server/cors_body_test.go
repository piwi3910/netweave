package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/config"
	"github.com/piwi3910/netweave/internal/server"
)

// TestCORSMiddleware_EmptyOriginsIsDenyAll verifies issue #495: an empty
// AllowedOrigins list must deny all cross-origin requests, not allow all.
func TestCORSMiddleware_EmptyOriginsIsDenyAll(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{Port: 8080, GinMode: gin.TestMode},
		Security: config.SecurityConfig{
			EnableCORS:     true,
			AllowedOrigins: []string{}, // empty list
			AllowedHeaders: []string{"Authorization"},
			AllowedMethods: []string{"GET", "POST"},
		},
	}
	srv, _ := server.NewTestServerWithMetrics(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{})

	router := gin.New()
	router.Use(srv.CORSMiddleware())
	router.GET("/test", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"),
		"empty AllowedOrigins must not reflect Origin")
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Credentials"),
		"empty AllowedOrigins must never emit credentials=true")
}

// TestCORSMiddleware_ExactOriginMatch ensures that an explicit allow-list
// entry mirrors the request Origin and allows credentials.
func TestCORSMiddleware_ExactOriginMatch(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{Port: 8080, GinMode: gin.TestMode},
		Security: config.SecurityConfig{
			EnableCORS:     true,
			AllowedOrigins: []string{"https://ui.example.com"},
			AllowedHeaders: []string{"Authorization", "Content-Type"},
			AllowedMethods: []string{"GET", "POST"},
		},
	}
	srv, _ := server.NewTestServerWithMetrics(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{})

	router := gin.New()
	router.Use(srv.CORSMiddleware())
	router.GET("/test", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "https://ui.example.com")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, "https://ui.example.com", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))
	assert.Equal(t, "Origin", w.Header().Get("Vary"))
}

// TestCORSMiddleware_WildcardNoCredentials ensures the wildcard origin
// never pairs with Access-Control-Allow-Credentials: true.
func TestCORSMiddleware_WildcardNoCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{Port: 8080, GinMode: gin.TestMode},
		Security: config.SecurityConfig{
			EnableCORS:     true,
			AllowedOrigins: []string{"*"},
			AllowedHeaders: []string{"Authorization"},
			AllowedMethods: []string{"GET"},
		},
	}
	srv, _ := server.NewTestServerWithMetrics(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{})

	router := gin.New()
	router.Use(srv.CORSMiddleware())
	router.GET("/test", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "https://anywhere.example.com")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Credentials"),
		"wildcard origin must never set credentials=true")
}

// TestCORSMiddleware_UnlistedOriginBlocked verifies that origins not in the
// allow-list get no CORS headers (and thus the browser blocks the response).
func TestCORSMiddleware_UnlistedOriginBlocked(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{Port: 8080, GinMode: gin.TestMode},
		Security: config.SecurityConfig{
			EnableCORS:     true,
			AllowedOrigins: []string{"https://ui.example.com"},
			AllowedHeaders: []string{"Authorization"},
			AllowedMethods: []string{"GET"},
		},
	}
	srv, _ := server.NewTestServerWithMetrics(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{})

	router := gin.New()
	router.Use(srv.CORSMiddleware())
	router.GET("/test", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Credentials"))
}

// TestBodyLimitMiddleware_ServerWiring ensures the server.bodyLimitMiddleware
// helper honors validation.MaxBodySize from config (issue #494).
func TestBodyLimitMiddleware_ServerWiring(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		Server: config.ServerConfig{Port: 8080, GinMode: gin.TestMode},
		Validation: config.ValidationConfig{
			MaxBodySize: 64,
		},
	}
	srv, _ := server.NewTestServerWithMetrics(cfg, zap.NewNop(), &mockAdapter{}, &mockStore{})

	router := gin.New()
	router.Use(srv.BodyLimitMiddleware())
	router.POST("/graphql", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	body := strings.Repeat("a", 128) // > 64
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(body))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
}
