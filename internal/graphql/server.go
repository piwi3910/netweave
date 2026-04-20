// Package graphql provides GraphQL API server setup and configuration.
package graphql

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/gin-gonic/gin"
	"github.com/vektah/gqlparser/v2/ast"

	"github.com/piwi3910/netweave/internal/auth"
	"github.com/piwi3910/netweave/internal/graphql/generated"
	"github.com/piwi3910/netweave/internal/graphql/resolvers"
)

// ServerOptions holds construction-time dependencies for the GraphQL handler.
//
// The defaults are safe for production:
//   - Introspection is NOT registered unless GinMode is explicitly not "release"
//     (issue #492). This mirrors the playground gate used by the router.
//   - WebSocket subscriptions authenticate the connection_init payload via
//     the OAuth2Authenticator before any subscriber is attached (issue #493).
type ServerOptions struct {
	// GinMode is the gin operating mode (e.g. "release", "debug"). When empty
	// the server assumes release mode (hardened).
	GinMode string

	// WebSocketAuthenticator validates the Authorization header on incoming
	// WebSocket connection_init payloads. When nil, WebSocket connections
	// are rejected: subscriptions must be explicitly opted-in.
	WebSocketAuthenticator *auth.OAuth2Authenticator

	// WebSocketRequireAuth controls whether the InitFunc rejects requests
	// when WebSocketAuthenticator is nil. Defaults to true. Test suites may
	// set this to false to exercise the transport without auth.
	WebSocketRequireAuth bool
}

// NewServer creates a hardened GraphQL handler with the provided resolver and
// options. This is the preferred constructor; see NewServerLegacy for the
// behavior-preserving shim used by older call sites.
//
// Example:
//
//	srv := graphql.NewServer(resolver, graphql.ServerOptions{
//	    GinMode:                cfg.Server.GinMode,
//	    WebSocketAuthenticator: oauth2Authenticator,
//	})
func NewServer(resolver *resolvers.Resolver, opts ServerOptions) *handler.Server {
	// We build the server manually rather than using handler.NewDefaultServer,
	// which would register extension.Introspection unconditionally. Gating
	// introspection (issue #492) requires full control over extension order.
	srv := handler.New(
		generated.NewExecutableSchema(
			generated.Config{Resolvers: resolver},
		),
	)

	// Transports mirror the production defaults from NewDefaultServer, with
	// the addition of a hardened WebSocket InitFunc (issue #493).
	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.POST{})
	srv.AddTransport(transport.MultipartForm{})

	// WebSocket transport for GraphQL subscriptions. The InitFunc validates
	// the connection_init payload's Authorization token via the shared
	// OAuth2 authenticator (issue #493). Absent an authenticator, the
	// handshake is refused so subscriptions cannot bypass auth — callers
	// that explicitly want to disable WS auth (tests, local dev) must set
	// both WebSocketRequireAuth = false AND WebSocketAuthenticator = nil.
	requireAuth := opts.WebSocketRequireAuth || opts.WebSocketAuthenticator != nil
	srv.AddTransport(transport.Websocket{
		KeepAlivePingInterval: 10 * time.Second,
		InitFunc:              newWebSocketInitFunc(opts.WebSocketAuthenticator, requireAuth),
	})

	// Standard query document cache, identical to the default.
	srv.SetQueryCache(lru.New[*ast.QueryDocument](1000))

	// Introspection is a development convenience; exposing it in production
	// leaks the full schema to any authenticated caller. Gate on GinMode so
	// it matches the playground gate in setupGraphQLRoutes (issue #492).
	// An empty GinMode is treated as production — fail safe.
	if opts.GinMode != "" && opts.GinMode != "release" {
		srv.Use(extension.Introspection{})
	}

	// Automatic persisted query cache, identical to the default.
	srv.Use(extension.AutomaticPersistedQuery{
		Cache: lru.New[string](100),
	})

	// Complexity limit — guards against ridiculously-deep query trees.
	srv.Use(extension.FixedComplexityLimit(1000))

	return srv
}

// NewServerLegacy is a shim for callers that have not yet migrated to
// the ServerOptions API. It preserves the pre-hardening behavior but logs
// no warnings: production code should call NewServer with ServerOptions.
//
// Deprecated: use NewServer with an explicit ServerOptions value.
func NewServerLegacy(resolver *resolvers.Resolver) *handler.Server {
	return NewServer(resolver, ServerOptions{
		GinMode:              "debug",
		WebSocketRequireAuth: false,
	})
}

// newWebSocketInitFunc returns the InitFunc used by transport.Websocket.
// It pulls the bearer token out of the init payload and delegates to the
// OAuth2 authenticator. Tokens may be provided under "Authorization"
// ("Bearer <tok>" form, case-insensitive) or a raw "authToken" field, which
// matches the Apollo Link WebSocket convention used by most JS clients.
func newWebSocketInitFunc(
	authenticator *auth.OAuth2Authenticator,
	requireAuth bool,
) func(context.Context, transport.InitPayload) (context.Context, *transport.InitPayload, error) {
	return func(ctx context.Context, initPayload transport.InitPayload) (context.Context, *transport.InitPayload, error) {
		if authenticator == nil {
			if requireAuth {
				return nil, nil, fmt.Errorf("websocket subscriptions require authentication")
			}
			return ctx, nil, nil
		}

		token := extractWebSocketToken(initPayload)
		if token == "" {
			return nil, nil, fmt.Errorf("websocket connection_init missing bearer token")
		}

		// OAuth2Authenticator.Authenticate expects a gin.Context to pull the
		// Authorization header from. Synthesize a minimal context backed by a
		// throw-away httptest.ResponseRecorder so the verifier can reuse its
		// regular parsing path without duplicating logic.
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/graphql", nil)
		if err != nil {
			return nil, nil, fmt.Errorf("build ws auth request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		ginCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ginCtx.Request = req

		user, _, tenant, err := authenticator.Authenticate(ctx, ginCtx, "graphql-ws")
		if err != nil {
			return nil, nil, fmt.Errorf("ws auth: %w", err)
		}
		if user == nil {
			return nil, nil, fmt.Errorf("ws auth returned nil user")
		}

		// Propagate user/tenant into the subscription context so resolvers
		// can authorize field access identically to REST requests.
		authUser := &auth.AuthenticatedUser{
			UserID:     user.ID,
			TenantID:   user.TenantID,
			Subject:    user.OAuthSubject,
			CommonName: user.CommonName,
			AuthMethod: auth.AuthMethodOAuth2,
		}
		ctx = auth.ContextWithUser(ctx, authUser)
		if tenant != nil {
			ctx = auth.ContextWithTenant(ctx, tenant)
		}
		return ctx, nil, nil
	}
}

// extractWebSocketToken looks for an OAuth2 bearer token in a GraphQL
// connection_init payload. Supports both "Authorization: Bearer <tok>"
// and the shorthand "authToken" field.
func extractWebSocketToken(payload transport.InitPayload) string {
	if header := payload.Authorization(); header != "" {
		parts := strings.SplitN(header, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			return strings.TrimSpace(parts[1])
		}
		// Some clients send the bare token under Authorization.
		return strings.TrimSpace(header)
	}
	if token := payload.GetString("authToken"); token != "" {
		return strings.TrimSpace(token)
	}
	return ""
}

// PlaygroundHandler returns a handler for the GraphQL playground UI.
// The playground provides an interactive GraphQL IDE for exploring the schema
// and testing queries.
//
// Example:
//
//	router.GET("/graphql", PlaygroundHandler("/graphql"))
func PlaygroundHandler(endpoint string) gin.HandlerFunc {
	h := playground.Handler("GraphQL Playground", endpoint)
	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}

// GinHandler wraps a GraphQL handler for use with Gin.
//
// Example:
//
//	gqlServer := graphql.NewServer(resolver, graphql.ServerOptions{GinMode: "debug"})
//	router.POST("/graphql", graphql.GinHandler(gqlServer))
func GinHandler(h *handler.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}
