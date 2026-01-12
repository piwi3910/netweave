package azure

import (
	"context"
	"fmt"

	"github.com/piwi3910/netweave/internal/adapter"
	"go.uber.org/zap"
)

// GetDeploymentManager retrieves metadata about the Azure deployment manager.
// It provides information about the Azure subscription and region.
func (a *Adapter) GetDeploymentManager(_ context.Context, id string) (*adapter.DeploymentManager, error) {
	a.logger.Debug("GetDeploymentManager called",
		zap.String("id", id))

	if id != a.deploymentManagerID {
		return nil, fmt.Errorf("deployment manager not found: %s", id)
	}

	// List availability zones for this location
	supportedLocations := []string{a.location}

	// Common Azure availability zones (1, 2, 3) for supported regions
	azZones := []string{
		fmt.Sprintf("%s-1", a.location),
		fmt.Sprintf("%s-2", a.location),
		fmt.Sprintf("%s-3", a.location),
	}
	supportedLocations = append(supportedLocations, azZones...)

	// Construct deployment manager metadata
	dm := &adapter.DeploymentManager{
		DeploymentManagerID: a.deploymentManagerID,
		Name:                fmt.Sprintf("Azure %s", a.location),
		Description:         fmt.Sprintf("Azure cloud deployment in region %s", a.location),
		OCloudID:            a.oCloudID,
		ServiceURI:          fmt.Sprintf("https://management.azure.com/subscriptions/%s", a.subscriptionID),
		SupportedLocations:  supportedLocations,
		Capabilities: []string{
			"resource-pools",
			"resources",
			"resource-types",
			"subscriptions",
		},
		Extensions: map[string]interface{}{
			"azure.subscriptionId": a.subscriptionID,
			"azure.location":       a.location,
			"azure.poolMode":       a.poolMode,
			"azure.portalUrl":      fmt.Sprintf("https://portal.azure.com/#@/resource/subscriptions/%s", a.subscriptionID),
		},
	}

	a.logger.Info("retrieved deployment manager",
		zap.String("deploymentManagerID", dm.DeploymentManagerID),
		zap.String("location", a.location))

	return dm, nil
}
