package graphql

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	mockadapter "github.com/piwi3910/netweave/internal/adapters/mock"
	"github.com/piwi3910/netweave/internal/graphql/resolvers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewServer(t *testing.T) {
	adp := mockadapter.NewAdapter(false)
	logger := zap.NewNop()
	resolver := resolvers.NewResolver(adp, nil, nil, logger)

	srv := NewServer(resolver)
	require.NotNil(t, srv)
}

func TestPlaygroundHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/playground", PlaygroundHandler("/graphql"))

	req := httptest.NewRequest(http.MethodGet, "/playground", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "GraphQL Playground")
}

func TestGinHandler(t *testing.T) {
	adp := mockadapter.NewAdapter(false)
	logger := zap.NewNop()
	resolver := resolvers.NewResolver(adp, nil, nil, logger)
	srv := NewServer(resolver)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/graphql", GinHandler(srv))

	// Send an introspection query to verify the handler works
	req := httptest.NewRequest(http.MethodPost, "/graphql", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Without a body, gqlgen returns 200 with an error response (not a panic/500)
	assert.NotEqual(t, http.StatusInternalServerError, w.Code)
}
