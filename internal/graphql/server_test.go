package graphql

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	mockadapter "github.com/piwi3910/netweave/internal/adapters/mock"
	"github.com/piwi3910/netweave/internal/graphql/resolvers"
)

func TestNewServer(t *testing.T) {
	adp := mockadapter.New(&mockadapter.Config{})
	logger := zap.NewNop()
	resolver := resolvers.NewResolver(adp, nil, nil, logger)

	srv := NewServer(resolver, ServerOptions{
		GinMode:              "debug",
		WebSocketRequireAuth: false,
	})
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
	adp := mockadapter.New(&mockadapter.Config{})
	logger := zap.NewNop()
	resolver := resolvers.NewResolver(adp, nil, nil, logger)
	srv := NewServer(resolver, ServerOptions{
		GinMode:              "debug",
		WebSocketRequireAuth: false,
	})

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

// TestNewServer_IntrospectionGating verifies that the introspection extension
// is only registered when GinMode != "release" (issue #492).
//
// We use a short introspection query and check the error payload: when
// introspection is disabled gqlgen returns a validation error containing
// "introspection"; when enabled, the schema is returned successfully.
func TestNewServer_IntrospectionGating(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()

	const introspectionBody = `{"query":"{ __schema { queryType { name } } }"}`

	cases := []struct {
		name                string
		mode                string
		wantIntrospectError bool
	}{
		{
			name:                "release_mode_disables_introspection",
			mode:                "release",
			wantIntrospectError: true,
		},
		{
			name:                "debug_mode_enables_introspection",
			mode:                "debug",
			wantIntrospectError: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			adp := mockadapter.New(&mockadapter.Config{})
			resolver := resolvers.NewResolver(adp, nil, nil, logger)
			srv := NewServer(resolver, ServerOptions{
				GinMode:              tc.mode,
				WebSocketRequireAuth: false,
			})

			router := gin.New()
			router.POST("/graphql", GinHandler(srv))

			req := httptest.NewRequest(http.MethodPost, "/graphql",
				strings.NewReader(introspectionBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)
			body := strings.ToLower(w.Body.String())
			if tc.wantIntrospectError {
				assert.Contains(t, body, "errors",
					"release mode must reject __schema introspection")
				assert.Contains(t, body, "introspection",
					"error message should reference introspection being disabled")
			} else {
				assert.NotContains(t, body, "\"errors\"",
					"debug mode must allow introspection")
				assert.Contains(t, body, "\"__schema\"",
					"debug mode must return the schema payload")
			}
		})
	}
}

// TestExtractWebSocketToken covers the InitPayload token parsing branches
// of issue #493 (Authorization header, bare Authorization, authToken key).
func TestExtractWebSocketToken(t *testing.T) {
	cases := []struct {
		name    string
		payload transport.InitPayload
		want    string
	}{
		{
			name:    "bearer header",
			payload: transport.InitPayload{"Authorization": "Bearer abc.def.ghi"},
			want:    "abc.def.ghi",
		},
		{
			name:    "bare authorization",
			payload: transport.InitPayload{"Authorization": "abc.def.ghi"},
			want:    "abc.def.ghi",
		},
		{
			name:    "authToken fallback",
			payload: transport.InitPayload{"authToken": "zzz"},
			want:    "zzz",
		},
		{
			name:    "empty payload",
			payload: transport.InitPayload{},
			want:    "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractWebSocketToken(tc.payload)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestWebSocketInitFunc_RequiresAuth verifies the InitFunc rejects a
// connection when requireAuth is true and no authenticator is supplied
// (issue #493 — subscriptions must not bypass auth).
func TestWebSocketInitFunc_RequiresAuth(t *testing.T) {
	init := newWebSocketInitFunc(nil, true)
	_, _, err := init(context.Background(), transport.InitPayload{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "require authentication")
}

// TestWebSocketInitFunc_NoAuthAllowedWhenNotRequired ensures the legacy
// test/dev path still works when authentication is explicitly disabled.
func TestWebSocketInitFunc_NoAuthAllowedWhenNotRequired(t *testing.T) {
	init := newWebSocketInitFunc(nil, false)
	ctx, _, err := init(context.Background(), transport.InitPayload{})
	require.NoError(t, err)
	assert.NotNil(t, ctx)
}
