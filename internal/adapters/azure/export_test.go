package azure

import (
	"github.com/piwi3910/netweave/internal/adapter"
	"go.uber.org/zap"
)

// NewTestAdapter creates an Adapter for testing with a no-op logger.
func NewTestAdapter() *Adapter {
	return &Adapter{
		logger:        zap.NewNop(),
		subscriptions: make(map[string]*adapter.Subscription),
	}
}

// ExportSubscriptions exposes the subscriptions map for tests.
// Callers must not access it concurrently with adapter operations.
func (a *Adapter) ExportSubscriptions() map[string]*adapter.Subscription {
	return a.subscriptions
}
