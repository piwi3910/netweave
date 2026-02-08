// Package resolvers implements GraphQL resolvers for the O2-IMS API.
package resolvers

import (
	"context"

	"github.com/piwi3910/netweave/internal/adapter"
	dmshandlers "github.com/piwi3910/netweave/internal/dms/handlers"
	"github.com/piwi3910/netweave/internal/storage"
	"go.uber.org/zap"
)

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require
// here.

// contextKeyIMSAdapter is the context key for storing the resolved IMS adapter per-request.
type contextKeyIMSAdapter struct{}

// Resolver provides GraphQL resolver implementations.
// It holds dependencies needed to resolve GraphQL queries, mutations, and subscriptions.
// The adapter field serves as a fallback when no per-request adapter is available in context.
type Resolver struct {
	adapter    adapter.Adapter
	store      storage.Store
	dmsHandler *dmshandlers.Handler
	logger     *zap.Logger
}

// NewResolver creates a new GraphQL resolver with the required dependencies.
// Note: SMO handler is not included to avoid import cycles.
// SMO resolvers will be implemented in a future iteration.
func NewResolver(
	adp adapter.Adapter,
	store storage.Store,
	dmsHandler *dmshandlers.Handler,
	logger *zap.Logger,
) *Resolver {
	return &Resolver{
		adapter:    adp,
		store:      store,
		dmsHandler: dmsHandler,
		logger:     logger,
	}
}

// getActiveAdapter returns the IMS adapter for the current request.
// It checks the context for a per-request override (set by dynamic routing middleware),
// falling back to the resolver's static adapter.
func (r *Resolver) getActiveAdapter(ctx context.Context) adapter.Adapter {
	if adp, ok := ctx.Value(contextKeyIMSAdapter{}).(adapter.Adapter); ok && adp != nil {
		return adp
	}
	return r.adapter
}

// WithAdapter returns a new context with the given adapter stored for GraphQL resolution.
// This is called by the server's GraphQL middleware to inject the per-request adapter.
func WithAdapter(ctx context.Context, adp adapter.Adapter) context.Context {
	return context.WithValue(ctx, contextKeyIMSAdapter{}, adp)
}
