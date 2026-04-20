package vmware

import (
	"context"
	"fmt"

	"github.com/piwi3910/netweave/internal/adapter"
	"go.uber.org/zap"
)

// GetDeploymentManager retrieves metadata about the vSphere deployment manager.
// It provides information about the vCenter and datacenter.
func (a *Adapter) GetDeploymentManager(ctx context.Context, id string) (*adapter.DeploymentManager, error) {
	a.logger.Debug("GetDeploymentManager called",
		zap.String("id", id))

	// Accept "default" and "" as aliases for the configured DM ID,
	// matching the behavior used by routes.go and handlers.
	if id != a.deploymentManagerID && id != "default" && id != "" {
		return nil, fmt.Errorf("deployment manager %s: %w", id, adapter.ErrDeploymentManagerNotFound)
	}

	// List clusters in the datacenter as supported locations
	clusters, err := a.finder.ClusterComputeResourceList(ctx, "*")
	if err != nil {
		return nil, fmt.Errorf("failed to list clusters: %w", err)
	}

	supportedLocations := make([]string, 0, len(clusters)+1)
	supportedLocations = append(supportedLocations, a.datacenterName)
	for _, cluster := range clusters {
		supportedLocations = append(supportedLocations, cluster.Name())
	}

	// Get vCenter version info
	version := "unknown"
	if a.client != nil && a.client.Client != nil {
		version = a.client.ServiceContent.About.Version
	}

	// Construct deployment manager metadata
	dm := &adapter.DeploymentManager{
		DeploymentManagerID: a.deploymentManagerID,
		Name:                fmt.Sprintf("VMware vSphere %s", a.datacenterName),
		Description:         fmt.Sprintf("VMware vSphere datacenter %s", a.datacenterName),
		OCloudID:            a.oCloudID,
		ServiceURI:          a.vcenterURL,
		SupportedLocations:  supportedLocations,
		Capabilities: []string{
			"resource-pools",
			"resources",
			"resource-types",
			"subscriptions",
		},
		Extensions: map[string]interface{}{
			"vmware.vCenterURL":   a.vcenterURL,
			"vmware.datacenter":   a.datacenterName,
			"vmware.poolMode":     a.poolMode,
			"vmware.version":      version,
			"vmware.clusterCount": len(clusters),
		},
	}

	a.logger.Info("retrieved deployment manager",
		zap.String("deployment_manager_id", dm.DeploymentManagerID),
		zap.String("datacenter", a.datacenterName))

	return dm, nil
}

// ListDeploymentManagers retrieves all deployment managers.
// VMware has a single deployment manager per adapter instance.
func (a *Adapter) ListDeploymentManagers(ctx context.Context, _ *adapter.Filter) ([]*adapter.DeploymentManager, error) {
	a.logger.Debug("ListDeploymentManagers called")

	dm, err := a.GetDeploymentManager(ctx, a.deploymentManagerID)
	if err != nil {
		return nil, fmt.Errorf("failed to list deployment managers: %w", err)
	}

	return []*adapter.DeploymentManager{dm}, nil
}
