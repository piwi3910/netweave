// Package graphql provides GraphQL API server setup and configuration.
package graphql

import (
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/gin-gonic/gin"
	"github.com/piwi3910/netweave/internal/graphql/generated"
	"github.com/piwi3910/netweave/internal/graphql/resolvers"
)

// NewServer creates a new GraphQL handler with the provided resolver.
// It configures the GraphQL server with:
// - Introspection support for schema exploration
// - Complexity limits to prevent DoS attacks
// - WebSocket transport for subscriptions
// - Query complexity analysis
//
// Example:
//
//	resolver := resolvers.NewResolver(adapter, store, dmsHandler, smoHandler, logger)
//	gqlHandler := graphql.NewServer(resolver)
func NewServer(resolver *resolvers.Resolver) *handler.Server {
	srv := handler.NewDefaultServer(
		generated.NewExecutableSchema(
			generated.Config{Resolvers: resolver},
		),
	)

	// Enable introspection for schema exploration (GraphQL playground, tools)
	srv.Use(extension.Introspection{})

	// Enable automatic query complexity limits to prevent expensive queries
	// A query's complexity is calculated based on the number of fields requested
	// and nested queries. This prevents clients from overwhelming the server.
	srv.Use(extension.FixedComplexityLimit(1000))

	// Add WebSocket transport for GraphQL subscriptions
	// This allows clients to receive real-time updates when resources change
	srv.AddTransport(transport.Websocket{
		KeepAlivePingInterval: 10 * time.Second,
	})

	return srv
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
//	gqlServer := graphql.NewServer(resolver)
//	router.POST("/graphql", graphql.GinHandler(gqlServer))
func GinHandler(h *handler.Server) gin.HandlerFunc {
	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}
