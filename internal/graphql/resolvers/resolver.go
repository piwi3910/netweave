package resolvers

import (
	"github.com/piwi3910/netweave/internal/adapter"
	dmshandlers "github.com/piwi3910/netweave/internal/dms/handlers"
	"github.com/piwi3910/netweave/internal/storage"
	"go.uber.org/zap"
)

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require
// here.

// Resolver provides GraphQL resolver implementations.
// It holds dependencies needed to resolve GraphQL queries, mutations, and subscriptions.
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
	adapter adapter.Adapter,
	store storage.Store,
	dmsHandler *dmshandlers.Handler,
	logger *zap.Logger,
) *Resolver {
	return &Resolver{
		adapter:    adapter,
		store:      store,
		dmsHandler: dmsHandler,
		logger:     logger,
	}
}
