package dtias

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapter"
)

// GetDeploymentManager retrieves metadata about the DTIAS deployment manager.
// This provides O2-IMS clients with information about the DTIAS backend infrastructure.
func (a *Adapter) GetDeploymentManager(ctx context.Context, id string) (*adapter.DeploymentManager, error) {
	a.logger.Debug("GetDeploymentManager called",
		zap.String("id", id))

	// Validate that the requested ID matches our deployment manager ID
	if id != a.DeploymentManagerID && id != "default" {
		return nil, fmt.Errorf("%w: %s", adapter.ErrDeploymentManagerNotFound, id)
	}

	// Query DTIAS API for datacenter metadata (used to build deployment manager info)
	datacenterInfo, err := a.getDatacenterInfo(ctx, a.Config.Datacenter)
	if err != nil {
		a.logger.Warn("failed to get datacenter info, using defaults",
			zap.Error(err))
		// Continue with defaults if datacenter query fails
	}

	// Build deployment manager metadata
	deploymentManager := &adapter.DeploymentManager{
		DeploymentManagerID: a.DeploymentManagerID,
		Name:                fmt.Sprintf("DTIAS Bare-Metal Infrastructure - %s", a.Config.Datacenter),
		Description:         fmt.Sprintf("Dell DTIAS bare-metal deployment manager for datacenter %s", a.Config.Datacenter),
		OCloudID:            a.OCloudID,
		ServiceURI:          a.Config.Endpoint,
		Capabilities: []string{
			"bare-metal-provisioning",
			"hardware-inventory",
			"power-management",
			"health-monitoring",
			"bios-configuration",
			"server-pools",
		},
		Extensions: map[string]interface{}{
			"dtias.endpoint":            a.Config.Endpoint,
			"dtias.datacenter":          a.Config.Datacenter,
			"dtias.apiVersion":          "1.0",
			"dtias.adapterVersion":      a.Version(),
			"dtias.tlsEnabled":          a.Config.ClientCert != "",
			"dtias.nativeSubscriptions": false, // DTIAS has no native event system
		},
	}

	// Add datacenter information if available
	if datacenterInfo != nil {
		deploymentManager.SupportedLocations = []string{a.Config.Datacenter}
		if datacenterInfo.City != "" {
			deploymentManager.Extensions["dtias.location.city"] = datacenterInfo.City
		}
		if datacenterInfo.Country != "" {
			deploymentManager.Extensions["dtias.location.country"] = datacenterInfo.Country
		}
		if datacenterInfo.Latitude != 0 && datacenterInfo.Longitude != 0 {
			deploymentManager.Extensions["dtias.location.latitude"] = datacenterInfo.Latitude
			deploymentManager.Extensions["dtias.location.longitude"] = datacenterInfo.Longitude
		}
	}

	a.logger.Debug("retrieved deployment manager",
		zap.String("id", deploymentManager.DeploymentManagerID),
		zap.String("name", deploymentManager.Name))

	return deploymentManager, nil
}

// DatacenterInfo represents DTIAS datacenter metadata.
type DatacenterInfo struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	City      string  `json:"city"`
	Country   string  `json:"country"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// getDatacenterInfo retrieves datacenter metadata from DTIAS API.
func (a *Adapter) getDatacenterInfo(ctx context.Context, datacenterID string) (*DatacenterInfo, error) {
	// Query DTIAS API for site info (DTIAS uses "sites" instead of "datacenters")
	path := fmt.Sprintf("/v2/inventory/sites/%s", datacenterID)
	resp, err := a.client.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get datacenter info: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			a.logger.Warn("failed to close response body", zap.Error(closeErr))
		}
	}()

	// Parse response
	var datacenterInfo DatacenterInfo
	if err := a.client.parseResponse(resp, &datacenterInfo); err != nil {
		return nil, fmt.Errorf("failed to parse datacenter info response: %w", err)
	}

	return &datacenterInfo, nil
}
