package starlingx

import (
	"context"
	"fmt"

	"github.com/piwi3910/netweave/internal/adapter"
	"go.uber.org/zap"
)

// GetDeploymentManager retrieves the StarlingX deployment manager information.
func (a *Adapter) GetDeploymentManager(ctx context.Context, id string) (*adapter.DeploymentManager, error) {
	if id != a.deploymentManagerID {
		a.logger.Warn("deployment manager not found",
			zap.String("requested", id),
			zap.String("expected", a.deploymentManagerID),
		)
		return nil, adapter.ErrDeploymentManagerNotFound
	}

	// List systems (should return one system)
	systems, err := a.client.ListSystems(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list systems: %w", err)
	}

	if len(systems) == 0 {
		return nil, fmt.Errorf("no starlingx system found")
	}

	// Use the first system
	system := &systems[0]

	// Build service URI
	serviceURI := fmt.Sprintf("%s/o2ims-infrastructureInventory/v1", a.client.endpoint)

	dm := MapSystemToDeploymentManager(system, a.deploymentManagerID, a.oCloudID, serviceURI)

	a.logger.Debug("retrieved deployment manager",
		zap.String("id", dm.DeploymentManagerID),
		zap.String("name", dm.Name),
		zap.String("system_type", system.SystemType),
	)

	return dm, nil
}
