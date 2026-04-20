package middleware_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/middleware"
)

// TestBodyLimit_RejectsOversizeContentLength verifies fail-fast on
// Content-Length > limit (issue #494 — no body read performed).
func TestBodyLimit_RejectsOversizeContentLength(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(middleware.BodyLimit(middleware.BodyLimitConfig{Default: 16, Logger: zap.NewNop()}))
	router.POST("/graphql", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	body := strings.Repeat("a", 1024)
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(body))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
	assert.Contains(t, w.Body.String(), "RequestEntityTooLarge")
}

// TestBodyLimit_AllowsSmallBody verifies that well-sized bodies pass through.
func TestBodyLimit_AllowsSmallBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(middleware.BodyLimit(middleware.BodyLimitConfig{Default: 1024, Logger: zap.NewNop()}))
	router.POST("/graphql", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader([]byte(`{"q":"query{a}"}`)))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestBodyLimit_PerRouteOverride verifies that a per-route override is honored.
func TestBodyLimit_PerRouteOverride(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(middleware.BodyLimit(middleware.BodyLimitConfig{
		Default: 16,
		PerRoute: map[string]int64{
			"POST /bulk": 4096,
		},
		Logger: zap.NewNop(),
	}))
	router.POST("/bulk", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	router.POST("/regular", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	t.Run("override allows larger body", func(t *testing.T) {
		body := strings.Repeat("a", 512)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/bulk", strings.NewReader(body)))
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("default rejects same body on different route", func(t *testing.T) {
		body := strings.Repeat("a", 512)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/regular", strings.NewReader(body)))
		assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
	})
}

// TestBodyLimit_WrapsBodyForChunkedReads ensures that when Content-Length is
// unknown (chunked transfer), downstream readers still observe the cap.
func TestBodyLimit_WrapsBodyForChunkedReads(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(middleware.BodyLimit(middleware.BodyLimitConfig{Default: 16, Logger: zap.NewNop()}))
	router.POST("/chunked", func(c *gin.Context) {
		buf := make([]byte, 1024)
		n, err := c.Request.Body.Read(buf)
		if err != nil && !middleware.IsMaxBytesError(err) {
			// Unexpected error; surface to the test.
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		c.String(http.StatusOK, string(buf[:n]))
	})

	body := strings.NewReader(strings.Repeat("b", 512))
	req := httptest.NewRequest(http.MethodPost, "/chunked", body)
	req.ContentLength = -1 // simulate chunked
	req.TransferEncoding = []string{"chunked"}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// The handler should read at most `limit` bytes before MaxBytesReader errors.
	assert.LessOrEqual(t, len(w.Body.String()), 16)
}

// TestBodyLimit_SkipsBodyLessMethods verifies GET/HEAD/etc. are not touched.
func TestBodyLimit_SkipsBodyLessMethods(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(middleware.BodyLimit(middleware.BodyLimitConfig{Default: 1, Logger: zap.NewNop()}))
	router.GET("/health", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

// TestBodyLimit_DefaultFallback verifies DefaultBodyLimit is applied
// when the caller provides no explicit default.
func TestBodyLimit_DefaultFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(middleware.BodyLimit(middleware.BodyLimitConfig{}))
	router.POST("/echo", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	// 2 MiB exceeds the 1 MiB default.
	body := strings.Repeat("a", int(middleware.DefaultBodyLimit)+1)
	req := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader(body))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
}
