package server

import (
	"github.com/gin-gonic/gin"
	gqlserver "github.com/piwi3910/netweave/internal/graphql"
	"github.com/piwi3910/netweave/internal/graphql/resolvers"
)

// setupGraphQLRoutes configures GraphQL API endpoints.
// Provides a flexible query interface alongside the REST API.
//
// Endpoints:
//   - POST /graphql - GraphQL query endpoint
//   - GET /graphql - GraphQL playground UI (dev mode only)
//
// The GraphQL API provides:
//   - Flexible queries with field selection
//   - Nested resource queries (e.g., get pools with resources)
//   - Filtering and pagination
//   - Real-time subscriptions via WebSocket
//   - Introspection for schema exploration
func (s *Server) setupGraphQLRoutes() {
	// GraphQL requires at least one source of adapter resolution.
	// With dynamic routing, the adapter is resolved per-request via middleware.
	// Without dynamic routing, a static adapter must be present.
	if s.adapter == nil && s.adapterRegistry == nil {
		s.logger.Info("GraphQL API deferred (no adapter source available)")
		return
	}

	// Create GraphQL resolver with server dependencies.
	// The static adapter serves as a fallback for when dynamic routing is not configured.
	// Note: SMO handler not included to avoid import cycles.
	resolver := resolvers.NewResolver(
		s.adapter,
		s.store,
		s.dmsHandler,
		s.logger,
	)

	// Create GraphQL server with resolver.
	gqlSrv := gqlserver.NewServer(resolver)

	gqlGuard := PluginGuard(s.pluginRegistry, "graphql")

	// GraphQL query endpoint (POST /graphql).
	// The graphqlContextMiddleware resolves the adapter per-request and injects it into
	// the standard context.Context so that gqlgen resolvers can access it.
	s.graphqlRouter.POST("/graphql", gqlGuard, s.graphqlContextMiddleware(), gqlserver.GinHandler(gqlSrv))

	// GraphQL playground UI (GET /graphql).
	// Only enabled in development mode for security.
	// Provides interactive IDE for exploring the GraphQL schema.
	if s.config.Server.GinMode != "release" {
		s.graphqlRouter.GET("/graphql", gqlGuard, gqlserver.PlaygroundHandler("/graphql"))
		s.logger.Info("GraphQL playground enabled at /graphql")
	}

	s.logger.Info("GraphQL API configured at /graphql")
}

// graphqlContextMiddleware resolves the IMS adapter per-request and injects it into
// the standard context.Context so that GraphQL resolvers can access it via context.Value.
// This is necessary because gqlgen resolvers receive context.Context, not gin.Context.
func (s *Server) graphqlContextMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		adp := s.resolveAdapterSilent(c)
		if adp != nil {
			// Inject adapter into the standard request context for GraphQL resolvers.
			ctx := resolvers.WithAdapter(c.Request.Context(), adp)
			c.Request = c.Request.WithContext(ctx)
		}
		c.Next()
	}
}
