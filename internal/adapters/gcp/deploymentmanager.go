package gcp

import (
	"context"
	"fmt"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/piwi3910/netweave/internal/adapter"
	"go.uber.org/zap"
)

// GetDeploymentManager retrieves metadata about the GCP deployment manager.
// It provides information about the GCP project and region.
func (a *GCPAdapter) GetDeploymentManager(ctx context.Context, id string) (*adapter.DeploymentManager, error) {
	a.logger.Debug("GetDeploymentManager called",
		zap.String("id", id))

	if id != a.deploymentManagerID {
		return nil, fmt.Errorf("deployment manager not found: %s", id)
	}

	// Get region information
	region, err := a.regionsClient.Get(ctx, &computepb.GetRegionRequest{
		Project: a.projectID,
		Region:  a.region,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get region: %w", err)
	}

	// List zones in this region
	supportedLocations := []string{a.region}
	if region.Zones != nil {
		for _, zoneURL := range region.Zones {
			// Extract zone name from URL
			zoneName := extractZoneName(zoneURL)
			if zoneName != "" {
				supportedLocations = append(supportedLocations, zoneName)
			}
		}
	}

	// Construct deployment manager metadata
	dm := &adapter.DeploymentManager{
		DeploymentManagerID: a.deploymentManagerID,
		Name:                fmt.Sprintf("GCP %s", a.region),
		Description:         fmt.Sprintf("Google Cloud Platform deployment in region %s", a.region),
		OCloudID:            a.oCloudID,
		ServiceURI:          fmt.Sprintf("https://compute.googleapis.com/compute/v1/projects/%s", a.projectID),
		SupportedLocations:  supportedLocations,
		Capabilities: []string{
			"resource-pools",
			"resources",
			"resource-types",
			"subscriptions",
		},
		Extensions: map[string]interface{}{
			"gcp.projectId":   a.projectID,
			"gcp.region":      a.region,
			"gcp.poolMode":    a.poolMode,
			"gcp.status":      ptrToString(region.Status),
			"gcp.description": ptrToString(region.Description),
		},
	}

	a.logger.Info("retrieved deployment manager",
		zap.String("deploymentManagerID", dm.DeploymentManagerID),
		zap.String("region", a.region))

	return dm, nil
}

// extractZoneName extracts the zone name from a GCP zone URL.
// e.g., "https://compute.googleapis.com/compute/v1/projects/my-project/zones/us-central1-a" -> "us-central1-a".
func extractZoneName(zoneURL string) string {
	// Find the last "/" and return everything after it
	for i := len(zoneURL) - 1; i >= 0; i-- {
		if zoneURL[i] == '/' {
			return zoneURL[i+1:]
		}
	}
	return zoneURL
}
