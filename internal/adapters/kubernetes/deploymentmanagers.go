package kubernetes

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/piwi3910/netweave/internal/adapter"
)

// ListDeploymentManagers retrieves deployment manager metadata.
// In Kubernetes, there is typically one deployment manager per cluster.
func (a *Adapter) ListDeploymentManagers(
	_ context.Context,
	filter *adapter.Filter,
) ([]*adapter.DeploymentManager, error) {
	a.logger.Debug("ListDeploymentManagers called",
		zap.Any("filter", filter))

	// Get the deployment manager
	dm := a.getDeploymentManager()

	// Apply filter if provided
	managers := []*adapter.DeploymentManager{}
	if adapter.MatchesFilter(filter, dm.DeploymentManagerID, "", "", nil) {
		managers = append(managers, dm)
	}

	a.logger.Info("listed deployment managers",
		zap.Int("count", len(managers)))

	return managers, nil
}

// GetDeploymentManager retrieves metadata about the deployment manager.
func (a *Adapter) GetDeploymentManager(_ context.Context, id string) (*adapter.DeploymentManager, error) {
	a.logger.Debug("GetDeploymentManager called",
		zap.String("id", id))

	// Validate ID matches configured deployment manager
	if id != a.deploymentManagerID {
		return nil, fmt.Errorf("deployment manager %s not found", id)
	}

	// Get server version for capabilities
	version, err := a.client.Discovery().ServerVersion()
	if err != nil {
		a.logger.Warn("failed to get server version",
			zap.Error(err))
	}

	dm := a.getDeploymentManager()

	// Add version information if available
	if version != nil {
		dm.Extensions["kubernetes.io/version"] = version.GitVersion
		dm.Extensions["kubernetes.io/platform"] = version.Platform
		dm.Extensions["kubernetes.io/go-version"] = version.GoVersion
	}

	a.logger.Info("retrieved deployment manager",
		zap.String("deploymentManagerID", dm.DeploymentManagerID),
		zap.String("name", dm.Name))

	return dm, nil
}

// CreateDeploymentManager creates a new deployment manager.
// This operation is not supported as deployment managers represent the Kubernetes cluster itself.
func (a *Adapter) CreateDeploymentManager(
	_ context.Context,
	dm *adapter.DeploymentManager,
) (*adapter.DeploymentManager, error) {
	a.logger.Debug("CreateDeploymentManager called",
		zap.String("name", dm.Name))

	return nil, fmt.Errorf(
		"creating deployment managers is not supported; " +
			"deployment managers represent Kubernetes clusters which must be provisioned externally",
	)
}

// getDeploymentManager returns the deployment manager metadata for this Kubernetes cluster.
func (a *Adapter) getDeploymentManager() *adapter.DeploymentManager {
	return &adapter.DeploymentManager{
		DeploymentManagerID: a.deploymentManagerID,
		Name:                fmt.Sprintf("Kubernetes Cluster: %s", a.deploymentManagerID),
		Description:         "Kubernetes-based O2-IMS Deployment Manager",
		OCloudID:            a.oCloudID,
		ServiceURI:          fmt.Sprintf("/o2ims/v1/deploymentManagers/%s", a.deploymentManagerID),
		Capabilities: []string{
			"resource-pools",
			"resources",
			"resource-types",
			"subscriptions",
			"health-checks",
		},
		Extensions: map[string]interface{}{
			"kubernetes.io/deployment-manager-id": a.deploymentManagerID,
			"kubernetes.io/o-cloud-id":            a.oCloudID,
			"kubernetes.io/namespace":             a.namespace,
			"kubernetes.io/adapter-version":       a.Version(),
		},
	}
}

// GetOCloudInfrastructure retrieves O-Cloud infrastructure metadata.
func (a *Adapter) GetOCloudInfrastructure(ctx context.Context) (map[string]interface{}, error) {
	a.logger.Debug("GetOCloudInfrastructure called")

	// Get server version
	version, err := a.client.Discovery().ServerVersion()
	if err != nil {
		a.logger.Error("failed to get server version",
			zap.Error(err))
		return nil, fmt.Errorf("failed to get Kubernetes server version: %w", err)
	}

	// Get API server endpoints
	endpoints, err := a.client.CoreV1().Endpoints("default").Get(ctx, "kubernetes", metav1.GetOptions{})
	if err != nil {
		a.logger.Warn("failed to get API server endpoints",
			zap.Error(err))
	}

	// Build infrastructure metadata
	infrastructure := map[string]interface{}{
		"oCloudId":     a.oCloudID,
		"name":         fmt.Sprintf("Kubernetes O-Cloud: %s", a.oCloudID),
		"description":  "Kubernetes-based O-RAN O-Cloud Infrastructure",
		"resourceType": "kubernetes-cluster",
		"version":      version.GitVersion,
		"kubernetes": map[string]interface{}{
			"version":    version.GitVersion,
			"platform":   version.Platform,
			"buildDate":  version.BuildDate,
			"goVersion":  version.GoVersion,
			"compiler":   version.Compiler,
			"gitCommit":  version.GitCommit,
			"gitVersion": version.GitVersion,
		},
	}

	// Add API server endpoints if available
	if endpoints != nil && len(endpoints.Subsets) > 0 {
		var apiEndpoints []string
		for i := range endpoints.Subsets {
			subset := &endpoints.Subsets[i]
			for j := range subset.Addresses {
				addr := &subset.Addresses[j]
				for k := range subset.Ports {
					port := &subset.Ports[k]
					endpoint := fmt.Sprintf("https://%s", net.JoinHostPort(addr.IP, strconv.Itoa(int(port.Port))))
					apiEndpoints = append(apiEndpoints, endpoint)
				}
			}
		}
		infrastructure["apiEndpoints"] = apiEndpoints
	}

	// Add deployment manager reference
	infrastructure["deploymentManagers"] = []string{a.deploymentManagerID}

	a.logger.Info("retrieved O-Cloud infrastructure",
		zap.String("oCloudId", a.oCloudID),
		zap.String("version", version.GitVersion))

	return infrastructure, nil
}
