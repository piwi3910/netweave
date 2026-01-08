// Package helm provides an O2-DMS adapter implementation using Helm 3.
// This adapter enables CNF/VNF deployment management through Helm charts
// deployed to Kubernetes clusters.
package helm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/repo"
	"helm.sh/helm/v3/pkg/storage/driver"

	"github.com/piwi3910/netweave/internal/dms/adapter"
)

const (
	// AdapterName is the unique identifier for the Helm adapter.
	AdapterName = "helm"

	// AdapterVersion indicates the Helm version supported by this adapter.
	AdapterVersion = "3.14.0"

	// DefaultTimeout is the default timeout for Helm operations.
	DefaultTimeout = 10 * time.Minute

	// DefaultMaxHistory is the default number of revisions to keep.
	DefaultMaxHistory = 10
)

// HelmAdapter implements the DMS adapter interface for Helm deployments.
type HelmAdapter struct {
	config      *Config
	settings    *cli.EnvSettings
	actionCfg   *action.Configuration
	repoIndex   map[string]*repo.IndexFile
	initialized bool
}

// Config contains configuration for the Helm adapter.
type Config struct {
	// Kubeconfig is the path to the Kubernetes config file.
	Kubeconfig string

	// Namespace is the default Kubernetes namespace for deployments.
	Namespace string

	// RepositoryURL is the Helm chart repository URL (e.g., ChartMuseum, Harbor).
	RepositoryURL string

	// RepositoryUsername is the username for repository authentication.
	RepositoryUsername string

	// RepositoryPassword is the password for repository authentication.
	RepositoryPassword string

	// Timeout is the default timeout for Helm operations.
	Timeout time.Duration

	// MaxHistory is the maximum number of revisions to keep per release.
	MaxHistory int

	// Debug enables verbose Helm output.
	Debug bool
}

// NewAdapter creates a new Helm adapter instance.
// Returns an error if the adapter cannot be initialized.
func NewAdapter(config *Config) (*HelmAdapter, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Apply defaults
	if config.Namespace == "" {
		config.Namespace = "default"
	}
	if config.Timeout == 0 {
		config.Timeout = DefaultTimeout
	}
	if config.MaxHistory == 0 {
		config.MaxHistory = DefaultMaxHistory
	}

	// Initialize Helm settings
	settings := cli.New()
	if config.Kubeconfig != "" {
		settings.KubeConfig = config.Kubeconfig
	}
	settings.SetNamespace(config.Namespace)
	settings.Debug = config.Debug

	adapter := &HelmAdapter{
		config:    config,
		settings:  settings,
		repoIndex: make(map[string]*repo.IndexFile),
	}

	return adapter, nil
}

// Initialize performs lazy initialization of the Helm action configuration.
// This allows the adapter to be created without requiring immediate Kubernetes connectivity.
func (h *HelmAdapter) Initialize(_ context.Context) error {
	if h.initialized {
		return nil
	}

	// Initialize action configuration
	actionCfg := new(action.Configuration)

	// Setup debug logger that respects debug flag
	debugOut := io.Discard
	if h.config.Debug {
		debugOut = os.Stderr
	}
	debugLog := func(format string, v ...interface{}) {
		log.New(debugOut, "[helm] ", log.LstdFlags).Printf(format, v...)
	}

	// Initialize with Kubernetes backend
	if err := actionCfg.Init(
		h.settings.RESTClientGetter(),
		h.config.Namespace,
		"secret", // Use Kubernetes secrets for storage
		debugLog,
	); err != nil {
		return fmt.Errorf("failed to initialize Helm action configuration: %w", err)
	}

	h.actionCfg = actionCfg
	h.initialized = true

	return nil
}

// Name returns the adapter name.
func (h *HelmAdapter) Name() string {
	return AdapterName
}

// Version returns the Helm version supported by this adapter.
func (h *HelmAdapter) Version() string {
	return AdapterVersion
}

// Capabilities returns the capabilities supported by the Helm adapter.
func (h *HelmAdapter) Capabilities() []adapter.Capability {
	return []adapter.Capability{
		adapter.CapabilityPackageManagement,
		adapter.CapabilityDeploymentLifecycle,
		adapter.CapabilityRollback,
		adapter.CapabilityScaling,
		adapter.CapabilityHealthChecks,
		adapter.CapabilityMetrics,
	}
}

// ListDeploymentPackages retrieves all Helm charts from the configured repository.
func (h *HelmAdapter) ListDeploymentPackages(
	ctx context.Context,
	_ *adapter.Filter,
) ([]*adapter.DeploymentPackage, error) {
	if err := h.Initialize(ctx); err != nil {
		return nil, err
	}

	// For Helm, packages are charts in repositories
	// This would query the chart repository index
	packages := make([]*adapter.DeploymentPackage, 0)

	// TODO(#60): Implement repository query
	// For now, return empty list

	return packages, nil
}

// GetDeploymentPackage retrieves a specific Helm chart by ID.
func (h *HelmAdapter) GetDeploymentPackage(ctx context.Context, id string) (*adapter.DeploymentPackage, error) {
	if err := h.Initialize(ctx); err != nil {
		return nil, err
	}

	// TODO(#60): Implement chart lookup by ID
	return nil, fmt.Errorf("chart not found: %s", id)
}

// UploadDeploymentPackage uploads a new Helm chart to the repository.
func (h *HelmAdapter) UploadDeploymentPackage(
	ctx context.Context,
	pkg *adapter.DeploymentPackageUpload,
) (*adapter.DeploymentPackage, error) {
	if err := h.Initialize(ctx); err != nil {
		return nil, err
	}

	if pkg == nil {
		return nil, fmt.Errorf("package cannot be nil")
	}

	// For Helm, this would push the chart to the repository
	// Implementation depends on repository type (ChartMuseum, Harbor, OCI)

	deploymentPkg := &adapter.DeploymentPackage{
		ID:          fmt.Sprintf("%s-%s", pkg.Name, pkg.Version),
		Name:        pkg.Name,
		Version:     pkg.Version,
		PackageType: "helm-chart",
		Description: pkg.Description,
		UploadedAt:  time.Now(),
		Extensions: map[string]interface{}{
			"helm.chartName":    pkg.Name,
			"helm.chartVersion": pkg.Version,
			"helm.repository":   h.config.RepositoryURL,
		},
	}

	return deploymentPkg, nil
}

// DeleteDeploymentPackage deletes a Helm chart from the repository.
func (h *HelmAdapter) DeleteDeploymentPackage(ctx context.Context, _ string) error {
	if err := h.Initialize(ctx); err != nil {
		return err
	}

	// TODO(#60): Implement chart deletion from repository
	return nil
}

// ListDeployments retrieves all Helm releases matching the filter.
func (h *HelmAdapter) ListDeployments(ctx context.Context, filter *adapter.Filter) ([]*adapter.Deployment, error) {
	if err := h.Initialize(ctx); err != nil {
		return nil, err
	}

	releases, err := h.fetchAllReleases()
	if err != nil {
		return nil, err
	}

	deployments := h.filterAndTransformReleases(releases, filter)

	if filter != nil {
		deployments = h.applyPagination(deployments, filter.Limit, filter.Offset)
	}

	return deployments, nil
}

// fetchAllReleases retrieves all Helm releases.
func (h *HelmAdapter) fetchAllReleases() ([]*release.Release, error) {
	client := action.NewList(h.actionCfg)
	client.All = true
	client.AllNamespaces = true

	releases, err := client.Run()
	if err != nil {
		return nil, fmt.Errorf("failed to list Helm releases: %w", err)
	}
	return releases, nil
}

// filterAndTransformReleases transforms releases and applies filters.
func (h *HelmAdapter) filterAndTransformReleases(
	releases []*release.Release,
	filter *adapter.Filter,
) []*adapter.Deployment {
	deployments := make([]*adapter.Deployment, 0, len(releases))
	for _, rel := range releases {
		deployment := h.transformReleaseToDeployment(rel)
		if h.matchesDeploymentFilter(rel, deployment, filter) {
			deployments = append(deployments, deployment)
		}
	}
	return deployments
}

// matchesDeploymentFilter checks if a release matches the filter criteria.
func (h *HelmAdapter) matchesDeploymentFilter(
	rel *release.Release,
	deployment *adapter.Deployment,
	filter *adapter.Filter,
) bool {
	if filter == nil {
		return true
	}
	if filter.Namespace != "" && rel.Namespace != filter.Namespace {
		return false
	}
	if filter.Status != "" && deployment.Status != filter.Status {
		return false
	}
	if !h.matchesLabels(deployment, filter.Labels) {
		return false
	}
	return true
}

// GetDeployment retrieves a specific Helm release by ID.
func (h *HelmAdapter) GetDeployment(ctx context.Context, id string) (*adapter.Deployment, error) {
	if err := h.Initialize(ctx); err != nil {
		return nil, err
	}

	client := action.NewGet(h.actionCfg)
	rel, err := client.Run(id)
	if err != nil {
		if errors.Is(err, driver.ErrReleaseNotFound) {
			return nil, fmt.Errorf("deployment not found: %s", id)
		}
		return nil, fmt.Errorf("failed to get Helm release: %w", err)
	}

	return h.transformReleaseToDeployment(rel), nil
}

// CreateDeployment installs a new Helm release.
func (h *HelmAdapter) CreateDeployment(
	ctx context.Context,
	req *adapter.DeploymentRequest,
) (*adapter.Deployment, error) {
	if err := h.Initialize(ctx); err != nil {
		return nil, err
	}

	if req == nil {
		return nil, fmt.Errorf("deployment request cannot be nil")
	}
	if req.Name == "" {
		return nil, fmt.Errorf("deployment name is required")
	}
	if req.PackageID == "" {
		return nil, fmt.Errorf("package ID is required")
	}

	client := action.NewInstall(h.actionCfg)
	client.Namespace = req.Namespace
	if client.Namespace == "" {
		client.Namespace = h.config.Namespace
	}
	client.ReleaseName = req.Name
	client.Wait = true
	client.Timeout = h.config.Timeout
	client.CreateNamespace = true

	// Load chart
	chartPath, err := client.LocateChart(req.PackageID, h.settings)
	if err != nil {
		return nil, fmt.Errorf("failed to locate chart %s: %w", req.PackageID, err)
	}

	chartRequested, err := loader.Load(chartPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load chart: %w", err)
	}

	// Install release
	rel, err := client.RunWithContext(ctx, chartRequested, req.Values)
	if err != nil {
		return nil, fmt.Errorf("helm install failed: %w", err)
	}

	return h.transformReleaseToDeployment(rel), nil
}

// UpdateDeployment upgrades an existing Helm release.
func (h *HelmAdapter) UpdateDeployment(
	ctx context.Context,
	id string,
	update *adapter.DeploymentUpdate,
) (*adapter.Deployment, error) {
	if err := h.Initialize(ctx); err != nil {
		return nil, err
	}

	if update == nil {
		return nil, fmt.Errorf("deployment update cannot be nil")
	}

	client := action.NewUpgrade(h.actionCfg)
	client.Wait = true
	client.Timeout = h.config.Timeout
	client.MaxHistory = h.config.MaxHistory

	// Get current release to obtain chart information
	getClient := action.NewGet(h.actionCfg)
	currentRelease, err := getClient.Run(id)
	if err != nil {
		return nil, fmt.Errorf("failed to get current release: %w", err)
	}

	// Upgrade with new values
	rel, err := client.RunWithContext(ctx, id, currentRelease.Chart, update.Values)
	if err != nil {
		return nil, fmt.Errorf("helm upgrade failed: %w", err)
	}

	return h.transformReleaseToDeployment(rel), nil
}

// DeleteDeployment uninstalls a Helm release.
func (h *HelmAdapter) DeleteDeployment(ctx context.Context, id string) error {
	if err := h.Initialize(ctx); err != nil {
		return err
	}

	client := action.NewUninstall(h.actionCfg)
	client.Wait = true
	client.Timeout = h.config.Timeout

	_, err := client.Run(id)
	if err != nil {
		if errors.Is(err, driver.ErrReleaseNotFound) {
			return fmt.Errorf("deployment not found: %s", id)
		}
		return fmt.Errorf("helm uninstall failed: %w", err)
	}

	return nil
}

// ScaleDeployment scales a deployment by updating replica values.
func (h *HelmAdapter) ScaleDeployment(ctx context.Context, id string, replicas int) error {
	if err := h.Initialize(ctx); err != nil {
		return err
	}

	if replicas < 0 {
		return fmt.Errorf("replicas must be non-negative")
	}

	// Get current release
	getClient := action.NewGet(h.actionCfg)
	currentRelease, err := getClient.Run(id)
	if err != nil {
		return fmt.Errorf("failed to get release: %w", err)
	}

	// Update values with new replica count
	values := currentRelease.Config
	if values == nil {
		values = make(map[string]interface{})
	}
	values["replicaCount"] = replicas

	// Perform upgrade with new replica count
	upgradeClient := action.NewUpgrade(h.actionCfg)
	upgradeClient.Wait = true
	upgradeClient.Timeout = h.config.Timeout
	upgradeClient.MaxHistory = h.config.MaxHistory
	upgradeClient.ReuseValues = true

	_, err = upgradeClient.RunWithContext(ctx, id, currentRelease.Chart, values)
	if err != nil {
		return fmt.Errorf("failed to scale deployment: %w", err)
	}

	return nil
}

// RollbackDeployment rolls back a release to a previous revision.
func (h *HelmAdapter) RollbackDeployment(ctx context.Context, id string, revision int) error {
	if err := h.Initialize(ctx); err != nil {
		return err
	}

	if revision < 0 {
		return fmt.Errorf("revision must be non-negative")
	}

	client := action.NewRollback(h.actionCfg)
	client.Version = revision
	client.Wait = true
	client.Timeout = h.config.Timeout
	client.CleanupOnFail = true

	if err := client.Run(id); err != nil {
		return fmt.Errorf("helm rollback failed: %w", err)
	}

	return nil
}

// GetDeploymentStatus retrieves detailed status for a deployment.
func (h *HelmAdapter) GetDeploymentStatus(ctx context.Context, id string) (*adapter.DeploymentStatusDetail, error) {
	if err := h.Initialize(ctx); err != nil {
		return nil, err
	}

	client := action.NewStatus(h.actionCfg)
	rel, err := client.Run(id)
	if err != nil {
		return nil, fmt.Errorf("failed to get release status: %w", err)
	}

	return h.transformReleaseToStatus(rel), nil
}

// GetDeploymentHistory retrieves the revision history for a deployment.
func (h *HelmAdapter) GetDeploymentHistory(ctx context.Context, id string) (*adapter.DeploymentHistory, error) {
	if err := h.Initialize(ctx); err != nil {
		return nil, err
	}

	client := action.NewHistory(h.actionCfg)
	client.Max = h.config.MaxHistory

	releases, err := client.Run(id)
	if err != nil {
		return nil, fmt.Errorf("failed to get release history: %w", err)
	}

	revisions := make([]adapter.DeploymentRevision, 0, len(releases))
	for _, rel := range releases {
		revisions = append(revisions, adapter.DeploymentRevision{
			Revision:    rel.Version,
			Version:     rel.Chart.Metadata.Version,
			DeployedAt:  rel.Info.LastDeployed.Time,
			Status:      h.transformHelmStatus(rel.Info.Status),
			Description: rel.Info.Description,
		})
	}

	return &adapter.DeploymentHistory{
		DeploymentID: id,
		Revisions:    revisions,
	}, nil
}

// GetDeploymentLogs retrieves logs for a deployment.
// Note: Helm doesn't directly provide logs, so this queries Kubernetes pods.
func (h *HelmAdapter) GetDeploymentLogs(ctx context.Context, id string, _ *adapter.LogOptions) ([]byte, error) {
	if err := h.Initialize(ctx); err != nil {
		return nil, err
	}

	// TODO(#60): Implement pod log retrieval for Helm release
	// This would query Kubernetes API to get pods for the release
	// and stream their logs

	return []byte(fmt.Sprintf("Logs for deployment %s (log retrieval not yet implemented)", id)), nil
}

// SupportsRollback returns true as Helm supports rollback.
func (h *HelmAdapter) SupportsRollback() bool {
	return true
}

// SupportsScaling returns true as scaling can be done via value updates.
func (h *HelmAdapter) SupportsScaling() bool {
	return true
}

// SupportsGitOps returns false as this is direct Helm, not GitOps-based.
func (h *HelmAdapter) SupportsGitOps() bool {
	return false
}

// Health performs a health check on the Helm backend.
func (h *HelmAdapter) Health(ctx context.Context) error {
	if err := h.Initialize(ctx); err != nil {
		return fmt.Errorf("helm adapter not healthy: %w", err)
	}

	// Try to list releases to verify connectivity
	client := action.NewList(h.actionCfg)
	client.Limit = 1

	_, err := client.Run()
	if err != nil {
		return fmt.Errorf("helm health check failed: %w", err)
	}

	return nil
}

// Close cleanly shuts down the adapter.
func (h *HelmAdapter) Close() error {
	h.initialized = false
	h.actionCfg = nil
	return nil
}

// transformReleaseToDeployment converts a Helm release to a Deployment.
func (h *HelmAdapter) transformReleaseToDeployment(rel *release.Release) *adapter.Deployment {
	return &adapter.Deployment{
		ID:          rel.Name,
		Name:        rel.Name,
		PackageID:   fmt.Sprintf("%s-%s", rel.Chart.Name(), rel.Chart.Metadata.Version),
		Namespace:   rel.Namespace,
		Status:      h.transformHelmStatus(rel.Info.Status),
		Version:     rel.Version,
		Description: rel.Info.Description,
		CreatedAt:   rel.Info.FirstDeployed.Time,
		UpdatedAt:   rel.Info.LastDeployed.Time,
		Extensions: map[string]interface{}{
			"helm.releaseName":  rel.Name,
			"helm.revision":     rel.Version,
			"helm.chart":        rel.Chart.Name(),
			"helm.chartVersion": rel.Chart.Metadata.Version,
			"helm.appVersion":   rel.Chart.Metadata.AppVersion,
			"helm.namespace":    rel.Namespace,
		},
	}
}

// transformReleaseToStatus converts a Helm release to detailed status.
func (h *HelmAdapter) transformReleaseToStatus(rel *release.Release) *adapter.DeploymentStatusDetail {
	status := &adapter.DeploymentStatusDetail{
		DeploymentID: rel.Name,
		Status:       h.transformHelmStatus(rel.Info.Status),
		Message:      rel.Info.Description,
		Progress:     h.calculateProgress(rel),
		UpdatedAt:    rel.Info.LastDeployed.Time,
		Extensions: map[string]interface{}{
			"helm.status":    rel.Info.Status.String(),
			"helm.revision":  rel.Version,
			"helm.notes":     rel.Info.Notes,
			"helm.resources": rel.Info.Resources,
		},
	}

	// Add conditions based on Helm status
	status.Conditions = h.buildConditions(rel)

	return status
}

// transformHelmStatus converts Helm release status to DMS deployment status.
func (h *HelmAdapter) transformHelmStatus(helmStatus release.Status) adapter.DeploymentStatus {
	switch helmStatus {
	case release.StatusPendingInstall:
		return adapter.DeploymentStatusPending
	case release.StatusPendingUpgrade:
		return adapter.DeploymentStatusDeploying
	case release.StatusDeployed:
		return adapter.DeploymentStatusDeployed
	case release.StatusFailed:
		return adapter.DeploymentStatusFailed
	case release.StatusPendingRollback:
		return adapter.DeploymentStatusRollingBack
	case release.StatusUninstalling, release.StatusUninstalled:
		return adapter.DeploymentStatusDeleting
	case release.StatusSuperseded, release.StatusUnknown:
		return adapter.DeploymentStatusFailed
	default:
		return adapter.DeploymentStatusFailed
	}
}

// calculateProgress estimates deployment progress based on Helm status.
func (h *HelmAdapter) calculateProgress(rel *release.Release) int {
	switch rel.Info.Status {
	case release.StatusDeployed:
		return 100
	case release.StatusFailed, release.StatusUninstalled, release.StatusSuperseded, release.StatusUnknown:
		return 0
	case release.StatusPendingInstall:
		return 25
	case release.StatusPendingUpgrade, release.StatusPendingRollback:
		return 50
	case release.StatusUninstalling:
		return 75
	default:
		return 0
	}
}

// buildConditions creates deployment conditions from Helm release info.
func (h *HelmAdapter) buildConditions(rel *release.Release) []adapter.DeploymentCondition {
	conditions := []adapter.DeploymentCondition{}

	// Add deployment condition
	deployedCondition := adapter.DeploymentCondition{
		Type:               "Deployed",
		Status:             "True",
		LastTransitionTime: rel.Info.LastDeployed.Time,
	}

	if rel.Info.Status == release.StatusDeployed {
		deployedCondition.Reason = "DeploymentSuccessful"
		deployedCondition.Message = "Release deployed successfully"
	} else {
		deployedCondition.Status = "False"
		deployedCondition.Reason = "DeploymentInProgress"
		deployedCondition.Message = fmt.Sprintf("Release status: %s", rel.Info.Status.String())
	}

	conditions = append(conditions, deployedCondition)

	return conditions
}

// matchesLabels checks if deployment matches label filters.
func (h *HelmAdapter) matchesLabels(_ *adapter.Deployment, labels map[string]string) bool {
	if len(labels) == 0 {
		return true
	}

	// Helm doesn't have native label support for releases
	// This would need to check labels on deployed Kubernetes resources
	// For now, return true
	return true
}

// applyPagination applies limit and offset to deployment list.
func (h *HelmAdapter) applyPagination(deployments []*adapter.Deployment, limit, offset int) []*adapter.Deployment {
	if offset >= len(deployments) {
		return []*adapter.Deployment{}
	}

	start := offset
	end := len(deployments)

	if limit > 0 && start+limit < end {
		end = start + limit
	}

	return deployments[start:end]
}
