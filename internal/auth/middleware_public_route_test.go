package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/auth"
)

// TestMiddleware_DefaultSkipPaths_NoLongerIncludesO2IMS guards against
// regressions of I12 (#503) — path-based skipping for /o2ims was replaced
// with an explicit handler-level opt-out.
func TestMiddleware_DefaultSkipPaths_NoLongerIncludesO2IMS(t *testing.T) {
	cfg := auth.DefaultMiddlewareConfig()
	for _, skip := range cfg.SkipPaths {
		require.NotEqualf(t, "/o2ims", skip,
			"DefaultMiddlewareConfig must not include /o2ims in SkipPaths; "+
				"mark the handler public via Middleware.MarkPublicRoute instead")
	}
}

// TestMiddleware_MarkPublicRoute_AllowsUnauthenticated verifies that routes
// registered via MarkPublicRoute bypass authentication, while unrelated
// routes still require credentials.
func TestMiddleware_MarkPublicRoute_AllowsUnauthenticated(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mw := auth.NewMiddleware(nil, &auth.MiddlewareConfig{
		Enabled:     true,
		RequireMTLS: true,
		SkipPaths:   []string{},
	}, zap.NewNop(), nil, nil)

	mw.MarkPublicRoute(http.MethodGet, "/o2ims")

	router := gin.New()
	router.Use(mw.AuthenticationMiddleware())
	router.GET("/o2ims", func(c *gin.Context) {
		c.String(http.StatusOK, "public-api-info")
	})
	router.GET("/o2ims/sensitive", func(c *gin.Context) {
		c.String(http.StatusOK, "should not be reached")
	})

	// Public route succeeds without credentials.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/o2ims", nil)
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code, "public /o2ims must bypass auth")
	assert.Equal(t, "public-api-info", w.Body.String())

	// Sibling path must NOT inherit the public behaviour — path-prefix drift
	// is exactly the kind of silent failure I12 was guarding against.
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/o2ims/sensitive", nil)
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code,
		"non-marked sibling /o2ims/sensitive must still require auth")
}
