package server

import (
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
	// Create GraphQL resolver with server dependencies
	// Note: SMO handler not included to avoid import cycles
	resolver := resolvers.NewResolver(
		s.adapter,
		s.store,
		s.dmsHandler,
		s.logger,
	)

	// Create GraphQL server with resolver
	gqlSrv := gqlserver.NewServer(resolver)

	// GraphQL query endpoint (POST /graphql)
	// Handles all GraphQL queries, mutations, and subscriptions
	s.router.POST("/graphql", gqlserver.GinHandler(gqlSrv))

	// GraphQL playground UI (GET /graphql)
	// Only enabled in development mode for security
	// Provides interactive IDE for exploring the GraphQL schema
	if s.config.Server.GinMode != "release" {
		s.router.GET("/graphql", gqlserver.PlaygroundHandler("/graphql"))
		s.logger.Info("GraphQL playground enabled at /graphql")
	}

	s.logger.Info("GraphQL API configured at /graphql")
}
