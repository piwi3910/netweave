package aws

import (
	"github.com/piwi3910/netweave/internal/adapter"
	"go.uber.org/zap"
)

// NewTestAdapter creates an Adapter for testing.
// It allows tests in external packages (aws_test) to construct an adapter
// without exposing internal fields.
func NewTestAdapter(
	logger *zap.Logger,
	oCloudID, deploymentManagerID, region, poolMode string,
) *Adapter {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Adapter{
		logger:              logger,
		oCloudID:            oCloudID,
		deploymentManagerID: deploymentManagerID,
		region:              region,
		poolMode:            poolMode,
		subscriptions:       make(map[string]*adapter.Subscription),
	}
}

// NewTestAdapterWithSubs creates an Adapter for testing with a preset subscriptions map.
func NewTestAdapterWithSubs(
	logger *zap.Logger,
	subscriptions map[string]*adapter.Subscription,
) *Adapter {
	if logger == nil {
		logger = zap.NewNop()
	}
	if subscriptions == nil {
		subscriptions = make(map[string]*adapter.Subscription)
	}
	return &Adapter{
		logger:        logger,
		subscriptions: subscriptions,
	}
}

// ExportOCloudID exposes the oCloudID field for tests.
func (a *Adapter) ExportOCloudID() string { return a.oCloudID }

// ExportDeploymentManagerID exposes the deploymentManagerID field for tests.
func (a *Adapter) ExportDeploymentManagerID() string { return a.deploymentManagerID }

// ExportRegion exposes the region field for tests.
func (a *Adapter) ExportRegion() string { return a.region }

// ExportPoolMode exposes the poolMode field for tests.
func (a *Adapter) ExportPoolMode() string { return a.poolMode }

// ExportSetPoolMode sets the poolMode field for tests.
func (a *Adapter) ExportSetPoolMode(mode string) { a.poolMode = mode }

// ExportSubscriptions exposes the subscriptions map for tests.
// The returned map is the same map the adapter uses, so it can be read and written
// in tests, but callers must not access it concurrently with adapter operations.
func (a *Adapter) ExportSubscriptions() map[string]*adapter.Subscription {
	return a.subscriptions
}
