// Package argocd provides an O2-DMS adapter implementation using ArgoCD.
// This adapter enables GitOps-based CNF/VNF deployment management through
// ArgoCD Applications deployed to Kubernetes clusters.
//
// IMPORTANT: This adapter uses the Kubernetes dynamic client to manage ArgoCD
// Application CRDs directly, rather than importing the ArgoCD library. This
// approach avoids dependency conflicts between ArgoCD v2 (which requires
// k8s.io/structured-merge-diff/v4) and newer Kubernetes client versions
// (which require structured-merge-diff/v6).
//
// See GitHub Issue #7 for details on the dependency conflict resolution.
package argocd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/piwi3910/netweave/internal/dms/adapter"
)

const (
	// AdapterName is the unique identifier for the ArgoCD adapter.
	AdapterName = "argocd"

	// AdapterVersion indicates the ArgoCD API version supported by this adapter.
	AdapterVersion = "v2.0"

	// DefaultNamespace is the default namespace where ArgoCD Applications are created.
	DefaultNamespace = "argocd"

	// DefaultSyncTimeout is the default timeout for sync operations.
	DefaultSyncTimeout = 10 * time.Minute

	// ApplicationGroup is the ArgoCD API group.
	ApplicationGroup = "argoproj.io"

	// ApplicationVersion is the ArgoCD Application API version.
	ApplicationVersion = "v1alpha1"

	// ApplicationResource is the ArgoCD Application resource name.
	ApplicationResource = "applications"
)

// applicationGVR is the GroupVersionResource for ArgoCD Applications.
var applicationGVR = schema.GroupVersionResource{
	Group:    ApplicationGroup,
	Version:  ApplicationVersion,
	Resource: ApplicationResource,
}

// ArgoCDAdapter implements the DMS adapter interface for ArgoCD deployments.
// It uses the Kubernetes dynamic client to manage ArgoCD Application CRDs,
// avoiding direct ArgoCD library dependencies and their version conflicts.
type ArgoCDAdapter struct {
	config        *Config
	dynamicClient dynamic.Interface
	initialized   bool
}

// Config contains configuration for the ArgoCD adapter.
type Config struct {
	// Kubeconfig is the path to the Kubernetes config file.
	// If empty, in-cluster config is used.
	Kubeconfig string

	// Namespace is the namespace where ArgoCD Applications are created.
	// Defaults to "argocd".
	Namespace string

	// ArgoServerURL is the ArgoCD server URL for API operations.
	// Optional - used for advanced operations if provided.
	ArgoServerURL string

	// DefaultProject is the default ArgoCD project for new Applications.
	// Defaults to "default".
	DefaultProject string

	// SyncTimeout is the timeout for sync operations.
	SyncTimeout time.Duration

	// AutoSync enables automatic syncing for new Applications.
	AutoSync bool

	// Prune enables pruning of resources not in Git during sync.
	Prune bool

	// SelfHeal enables automatic self-healing of out-of-sync resources.
	SelfHeal bool
}

// NewAdapter creates a new ArgoCD adapter instance.
// Returns an error if the adapter cannot be initialized.
func NewAdapter(config *Config) (*ArgoCDAdapter, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Apply defaults
	if config.Namespace == "" {
		config.Namespace = DefaultNamespace
	}
	if config.DefaultProject == "" {
		config.DefaultProject = "default"
	}
	if config.SyncTimeout == 0 {
		config.SyncTimeout = DefaultSyncTimeout
	}

	return &ArgoCDAdapter{
		config: config,
	}, nil
}

// Initialize performs lazy initialization of the Kubernetes dynamic client.
// This allows the adapter to be created without requiring immediate Kubernetes connectivity.
func (a *ArgoCDAdapter) Initialize(ctx context.Context) error {
	if a.initialized {
		return nil
	}

	var restConfig *rest.Config
	var err error

	if a.config.Kubeconfig != "" {
		// Use kubeconfig file
		restConfig, err = clientcmd.BuildConfigFromFlags("", a.config.Kubeconfig)
		if err != nil {
			return fmt.Errorf("failed to build config from kubeconfig: %w", err)
		}
	} else {
		// Use in-cluster config
		restConfig, err = rest.InClusterConfig()
		if err != nil {
			return fmt.Errorf("failed to get in-cluster config: %w", err)
		}
	}

	// Create dynamic client
	a.dynamicClient, err = dynamic.NewForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("failed to create dynamic client: %w", err)
	}

	a.initialized = true
	return nil
}

// Name returns the adapter name.
func (a *ArgoCDAdapter) Name() string {
	return AdapterName
}

// Version returns the ArgoCD version supported by this adapter.
func (a *ArgoCDAdapter) Version() string {
	return AdapterVersion
}

// Capabilities returns the capabilities supported by the ArgoCD adapter.
func (a *ArgoCDAdapter) Capabilities() []adapter.Capability {
	return []adapter.Capability{
		adapter.CapabilityDeploymentLifecycle,
		adapter.CapabilityGitOps,
		adapter.CapabilityRollback,
		adapter.CapabilityHealthChecks,
		adapter.CapabilityMetrics,
	}
}

// ListDeploymentPackages retrieves deployment packages (Git repositories) from ArgoCD.
// In ArgoCD, packages are Git repositories referenced in Applications.
func (a *ArgoCDAdapter) ListDeploymentPackages(ctx context.Context, filter *adapter.Filter) ([]*adapter.DeploymentPackage, error) {
	if err := a.Initialize(ctx); err != nil {
		return nil, err
	}

	// List all Applications to extract unique repository sources
	apps, err := a.listApplications(ctx, filter)
	if err != nil {
		return nil, err
	}

	// Extract unique repositories from applications
	repoMap := make(map[string]*adapter.DeploymentPackage)
	for _, app := range apps {
		source := a.extractSource(app)
		if source.RepoURL == "" {
			continue
		}

		id := generatePackageID(source.RepoURL, source.Path)
		if _, exists := repoMap[id]; !exists {
			repoMap[id] = &adapter.DeploymentPackage{
				ID:          id,
				Name:        source.Path,
				Version:     source.TargetRevision,
				PackageType: "git-repo",
				Description: fmt.Sprintf("Git repository: %s (path: %s)", source.RepoURL, source.Path),
				UploadedAt:  time.Now(), // ArgoCD doesn't track upload time
				Extensions: map[string]interface{}{
					"argocd.repoURL":        source.RepoURL,
					"argocd.path":           source.Path,
					"argocd.targetRevision": source.TargetRevision,
					"argocd.chart":          source.Chart,
				},
			}
		}
	}

	packages := make([]*adapter.DeploymentPackage, 0, len(repoMap))
	for _, pkg := range repoMap {
		packages = append(packages, pkg)
	}

	return packages, nil
}

// GetDeploymentPackage retrieves a specific deployment package by ID.
func (a *ArgoCDAdapter) GetDeploymentPackage(ctx context.Context, id string) (*adapter.DeploymentPackage, error) {
	if err := a.Initialize(ctx); err != nil {
		return nil, err
	}

	// Search through applications for this package
	packages, err := a.ListDeploymentPackages(ctx, nil)
	if err != nil {
		return nil, err
	}

	for _, pkg := range packages {
		if pkg.ID == id {
			return pkg, nil
		}
	}

	return nil, fmt.Errorf("deployment package not found: %s", id)
}

// UploadDeploymentPackage is not directly supported in ArgoCD.
// ArgoCD uses Git repositories as package sources.
func (a *ArgoCDAdapter) UploadDeploymentPackage(ctx context.Context, pkg *adapter.DeploymentPackageUpload) (*adapter.DeploymentPackage, error) {
	if err := a.Initialize(ctx); err != nil {
		return nil, err
	}

	if pkg == nil {
		return nil, fmt.Errorf("package cannot be nil")
	}

	// ArgoCD doesn't support traditional package uploads
	// Instead, create a reference to a Git repository
	repoURL, _ := pkg.Extensions["argocd.repoURL"].(string)
	path, _ := pkg.Extensions["argocd.path"].(string)

	if repoURL == "" {
		return nil, fmt.Errorf("argocd.repoURL extension is required for ArgoCD packages")
	}

	return &adapter.DeploymentPackage{
		ID:          generatePackageID(repoURL, path),
		Name:        pkg.Name,
		Version:     pkg.Version,
		PackageType: "git-repo",
		Description: pkg.Description,
		UploadedAt:  time.Now(),
		Extensions: map[string]interface{}{
			"argocd.repoURL":        repoURL,
			"argocd.path":           path,
			"argocd.targetRevision": pkg.Version,
		},
	}, nil
}

// DeleteDeploymentPackage is not directly supported in ArgoCD.
// Packages are Git repositories managed externally.
func (a *ArgoCDAdapter) DeleteDeploymentPackage(ctx context.Context, id string) error {
	// ArgoCD doesn't support deleting packages (Git repos)
	// This would need to be done in the Git server
	return fmt.Errorf("ArgoCD does not support package deletion - manage repositories directly")
}

// ListDeployments retrieves all ArgoCD Applications matching the filter.
func (a *ArgoCDAdapter) ListDeployments(ctx context.Context, filter *adapter.Filter) ([]*adapter.Deployment, error) {
	if err := a.Initialize(ctx); err != nil {
		return nil, err
	}

	apps, err := a.listApplications(ctx, filter)
	if err != nil {
		return nil, err
	}

	deployments := make([]*adapter.Deployment, 0, len(apps))
	for _, app := range apps {
		deployment := a.transformApplicationToDeployment(app)

		// Apply status filter if specified
		if filter != nil && filter.Status != "" && deployment.Status != filter.Status {
			continue
		}

		deployments = append(deployments, deployment)
	}

	// Apply pagination
	if filter != nil {
		deployments = a.applyPagination(deployments, filter.Limit, filter.Offset)
	}

	return deployments, nil
}

// GetDeployment retrieves a specific ArgoCD Application by name.
func (a *ArgoCDAdapter) GetDeployment(ctx context.Context, id string) (*adapter.Deployment, error) {
	if err := a.Initialize(ctx); err != nil {
		return nil, err
	}

	app, err := a.getApplication(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("deployment not found: %s: %w", id, err)
	}

	return a.transformApplicationToDeployment(app), nil
}

// CreateDeployment creates a new ArgoCD Application.
func (a *ArgoCDAdapter) CreateDeployment(ctx context.Context, req *adapter.DeploymentRequest) (*adapter.Deployment, error) {
	if err := a.Initialize(ctx); err != nil {
		return nil, err
	}

	if req == nil {
		return nil, fmt.Errorf("deployment request cannot be nil")
	}
	if req.Name == "" {
		return nil, fmt.Errorf("deployment name is required")
	}

	// Extract source configuration from PackageID or extensions
	repoURL, _ := req.Extensions["argocd.repoURL"].(string)
	path, _ := req.Extensions["argocd.path"].(string)
	targetRevision, _ := req.Extensions["argocd.targetRevision"].(string)
	chart, _ := req.Extensions["argocd.chart"].(string)

	if repoURL == "" {
		return nil, fmt.Errorf("argocd.repoURL extension is required")
	}

	if targetRevision == "" {
		targetRevision = "HEAD"
	}

	destNamespace := req.Namespace
	if destNamespace == "" {
		destNamespace = "default"
	}

	// Build ArgoCD Application manifest
	app := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": fmt.Sprintf("%s/%s", ApplicationGroup, ApplicationVersion),
			"kind":       "Application",
			"metadata": map[string]interface{}{
				"name":      req.Name,
				"namespace": a.config.Namespace,
			},
			"spec": map[string]interface{}{
				"project": a.config.DefaultProject,
				"source": map[string]interface{}{
					"repoURL":        repoURL,
					"path":           path,
					"targetRevision": targetRevision,
					"chart":          chart,
				},
				"destination": map[string]interface{}{
					"server":    "https://kubernetes.default.svc",
					"namespace": destNamespace,
				},
			},
		},
	}

	// Add sync policy if auto-sync is enabled
	if a.config.AutoSync {
		syncPolicy := map[string]interface{}{
			"automated": map[string]interface{}{
				"prune":    a.config.Prune,
				"selfHeal": a.config.SelfHeal,
			},
		}
		if err := unstructured.SetNestedField(app.Object, syncPolicy, "spec", "syncPolicy"); err != nil {
			return nil, fmt.Errorf("failed to set sync policy: %w", err)
		}
	}

	// Add Helm values if provided
	if len(req.Values) > 0 && chart != "" {
		helmParams := map[string]interface{}{
			"values": mustMarshalYAML(req.Values),
		}
		if err := unstructured.SetNestedField(app.Object, helmParams, "spec", "source", "helm"); err != nil {
			return nil, fmt.Errorf("failed to set helm values: %w", err)
		}
	}

	// Create the Application
	result, err := a.dynamicClient.Resource(applicationGVR).Namespace(a.config.Namespace).Create(ctx, app, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create ArgoCD Application: %w", err)
	}

	return a.transformApplicationToDeployment(result), nil
}

// UpdateDeployment updates an existing ArgoCD Application.
func (a *ArgoCDAdapter) UpdateDeployment(ctx context.Context, id string, update *adapter.DeploymentUpdate) (*adapter.Deployment, error) {
	if err := a.Initialize(ctx); err != nil {
		return nil, err
	}

	if update == nil {
		return nil, fmt.Errorf("deployment update cannot be nil")
	}

	// Get current application
	app, err := a.getApplication(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("deployment not found: %s: %w", id, err)
	}

	// Update target revision if specified
	if targetRevision, ok := update.Extensions["argocd.targetRevision"].(string); ok && targetRevision != "" {
		if err := unstructured.SetNestedField(app.Object, targetRevision, "spec", "source", "targetRevision"); err != nil {
			return nil, fmt.Errorf("failed to update target revision: %w", err)
		}
	}

	// Update Helm values if provided
	if len(update.Values) > 0 {
		helmParams := map[string]interface{}{
			"values": mustMarshalYAML(update.Values),
		}
		if err := unstructured.SetNestedField(app.Object, helmParams, "spec", "source", "helm"); err != nil {
			return nil, fmt.Errorf("failed to update helm values: %w", err)
		}
	}

	// Update the Application
	result, err := a.dynamicClient.Resource(applicationGVR).Namespace(a.config.Namespace).Update(ctx, app, metav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to update ArgoCD Application: %w", err)
	}

	return a.transformApplicationToDeployment(result), nil
}

// DeleteDeployment deletes an ArgoCD Application.
func (a *ArgoCDAdapter) DeleteDeployment(ctx context.Context, id string) error {
	if err := a.Initialize(ctx); err != nil {
		return err
	}

	// Set cascade deletion to delete associated resources
	propagation := metav1.DeletePropagationForeground
	err := a.dynamicClient.Resource(applicationGVR).Namespace(a.config.Namespace).Delete(ctx, id, metav1.DeleteOptions{
		PropagationPolicy: &propagation,
	})
	if err != nil {
		return fmt.Errorf("failed to delete ArgoCD Application: %w", err)
	}

	return nil
}

// ScaleDeployment scales a deployment by updating Helm values.
// ArgoCD doesn't directly support scaling, but we can update replica values.
func (a *ArgoCDAdapter) ScaleDeployment(ctx context.Context, id string, replicas int) error {
	if err := a.Initialize(ctx); err != nil {
		return err
	}

	if replicas < 0 {
		return fmt.Errorf("replicas must be non-negative")
	}

	// Update the deployment with new replica count
	update := &adapter.DeploymentUpdate{
		Values: map[string]interface{}{
			"replicaCount": replicas,
		},
	}

	_, err := a.UpdateDeployment(ctx, id, update)
	return err
}

// RollbackDeployment rolls back an ArgoCD Application to a previous revision.
func (a *ArgoCDAdapter) RollbackDeployment(ctx context.Context, id string, revision int) error {
	if err := a.Initialize(ctx); err != nil {
		return err
	}

	if revision < 0 {
		return fmt.Errorf("revision must be non-negative")
	}

	// Get the application
	app, err := a.getApplication(ctx, id)
	if err != nil {
		return fmt.Errorf("deployment not found: %s: %w", id, err)
	}

	// Get history to find the target revision
	history, _, _ := unstructured.NestedSlice(app.Object, "status", "history")
	if revision >= len(history) {
		return fmt.Errorf("revision %d not found in history", revision)
	}

	// Get the target revision's source
	targetHistory, ok := history[revision].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid history entry at revision %d", revision)
	}

	targetRevision, _, _ := unstructured.NestedString(targetHistory, "revision")
	if targetRevision == "" {
		return fmt.Errorf("no revision found at history index %d", revision)
	}

	// Update to the target revision
	if err := unstructured.SetNestedField(app.Object, targetRevision, "spec", "source", "targetRevision"); err != nil {
		return fmt.Errorf("failed to set target revision: %w", err)
	}

	// Update the Application
	_, err = a.dynamicClient.Resource(applicationGVR).Namespace(a.config.Namespace).Update(ctx, app, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to rollback ArgoCD Application: %w", err)
	}

	return nil
}

// GetDeploymentStatus retrieves detailed status for an ArgoCD Application.
func (a *ArgoCDAdapter) GetDeploymentStatus(ctx context.Context, id string) (*adapter.DeploymentStatusDetail, error) {
	if err := a.Initialize(ctx); err != nil {
		return nil, err
	}

	app, err := a.getApplication(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("deployment not found: %s: %w", id, err)
	}

	return a.transformApplicationToStatus(app), nil
}

// GetDeploymentHistory retrieves the revision history for an ArgoCD Application.
func (a *ArgoCDAdapter) GetDeploymentHistory(ctx context.Context, id string) (*adapter.DeploymentHistory, error) {
	if err := a.Initialize(ctx); err != nil {
		return nil, err
	}

	app, err := a.getApplication(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("deployment not found: %s: %w", id, err)
	}

	history, _, _ := unstructured.NestedSlice(app.Object, "status", "history")

	revisions := make([]adapter.DeploymentRevision, 0, len(history))
	for i, h := range history {
		historyEntry, ok := h.(map[string]interface{})
		if !ok {
			continue
		}

		revision, _, _ := unstructured.NestedString(historyEntry, "revision")
		deployedAtStr, _, _ := unstructured.NestedString(historyEntry, "deployedAt")

		deployedAt := time.Now()
		if deployedAtStr != "" {
			if parsed, err := time.Parse(time.RFC3339, deployedAtStr); err == nil {
				deployedAt = parsed
			}
		}

		revisions = append(revisions, adapter.DeploymentRevision{
			Revision:    i,
			Version:     revision,
			DeployedAt:  deployedAt,
			Status:      adapter.DeploymentStatusDeployed,
			Description: fmt.Sprintf("Revision %s", revision),
		})
	}

	return &adapter.DeploymentHistory{
		DeploymentID: id,
		Revisions:    revisions,
	}, nil
}

// GetDeploymentLogs retrieves logs for an ArgoCD Application.
// Note: ArgoCD doesn't directly provide logs, this returns status information.
func (a *ArgoCDAdapter) GetDeploymentLogs(ctx context.Context, id string, opts *adapter.LogOptions) ([]byte, error) {
	if err := a.Initialize(ctx); err != nil {
		return nil, err
	}

	// ArgoCD doesn't provide direct log access
	// Return application status information instead
	app, err := a.getApplication(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("deployment not found: %s: %w", id, err)
	}

	status, _, _ := unstructured.NestedMap(app.Object, "status")
	statusJSON, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal status: %w", err)
	}

	return statusJSON, nil
}

// SupportsRollback returns true as ArgoCD supports rollback via history.
func (a *ArgoCDAdapter) SupportsRollback() bool {
	return true
}

// SupportsScaling returns true as scaling can be done via Helm value updates.
func (a *ArgoCDAdapter) SupportsScaling() bool {
	return true
}

// SupportsGitOps returns true as ArgoCD is a GitOps tool.
func (a *ArgoCDAdapter) SupportsGitOps() bool {
	return true
}

// Health performs a health check on the ArgoCD backend.
func (a *ArgoCDAdapter) Health(ctx context.Context) error {
	if err := a.Initialize(ctx); err != nil {
		return fmt.Errorf("argocd adapter not healthy: %w", err)
	}

	// Try to list applications to verify connectivity
	_, err := a.dynamicClient.Resource(applicationGVR).Namespace(a.config.Namespace).List(ctx, metav1.ListOptions{
		Limit: 1,
	})
	if err != nil {
		return fmt.Errorf("argocd health check failed: %w", err)
	}

	return nil
}

// Close cleanly shuts down the adapter.
func (a *ArgoCDAdapter) Close() error {
	a.initialized = false
	a.dynamicClient = nil
	return nil
}

// listApplications retrieves ArgoCD Applications with optional filtering.
func (a *ArgoCDAdapter) listApplications(ctx context.Context, filter *adapter.Filter) ([]*unstructured.Unstructured, error) {
	namespace := a.config.Namespace
	if filter != nil && filter.Namespace != "" {
		namespace = filter.Namespace
	}

	opts := metav1.ListOptions{}
	if filter != nil && len(filter.Labels) > 0 {
		opts.LabelSelector = buildLabelSelector(filter.Labels)
	}

	list, err := a.dynamicClient.Resource(applicationGVR).Namespace(namespace).List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list ArgoCD Applications: %w", err)
	}

	apps := make([]*unstructured.Unstructured, 0, len(list.Items))
	for i := range list.Items {
		apps = append(apps, &list.Items[i])
	}

	return apps, nil
}

// getApplication retrieves a single ArgoCD Application by name.
func (a *ArgoCDAdapter) getApplication(ctx context.Context, name string) (*unstructured.Unstructured, error) {
	return a.dynamicClient.Resource(applicationGVR).Namespace(a.config.Namespace).Get(ctx, name, metav1.GetOptions{})
}

// transformApplicationToDeployment converts an ArgoCD Application to a Deployment.
func (a *ArgoCDAdapter) transformApplicationToDeployment(app *unstructured.Unstructured) *adapter.Deployment {
	name, _, _ := unstructured.NestedString(app.Object, "metadata", "name")
	namespace, _, _ := unstructured.NestedString(app.Object, "metadata", "namespace")

	// Extract source information
	source := a.extractSource(app)

	// Extract status
	healthStatus, _, _ := unstructured.NestedString(app.Object, "status", "health", "status")
	syncStatus, _, _ := unstructured.NestedString(app.Object, "status", "sync", "status")

	// Extract destination namespace
	destNamespace, _, _ := unstructured.NestedString(app.Object, "spec", "destination", "namespace")

	// Get timestamps
	creationTimestamp := app.GetCreationTimestamp().Time
	var updatedAt time.Time
	reconciledAt, _, _ := unstructured.NestedString(app.Object, "status", "reconciledAt")
	if reconciledAt != "" {
		if parsed, err := time.Parse(time.RFC3339, reconciledAt); err == nil {
			updatedAt = parsed
		}
	}
	if updatedAt.IsZero() {
		updatedAt = creationTimestamp
	}

	// Get history length as version
	history, _, _ := unstructured.NestedSlice(app.Object, "status", "history")
	version := len(history)

	return &adapter.Deployment{
		ID:          name,
		Name:        name,
		PackageID:   generatePackageID(source.RepoURL, source.Path),
		Namespace:   destNamespace,
		Status:      a.transformArgoCDStatus(healthStatus, syncStatus),
		Version:     version,
		Description: fmt.Sprintf("ArgoCD Application from %s", source.RepoURL),
		CreatedAt:   creationTimestamp,
		UpdatedAt:   updatedAt,
		Extensions: map[string]interface{}{
			"argocd.appNamespace":   namespace,
			"argocd.repoURL":        source.RepoURL,
			"argocd.path":           source.Path,
			"argocd.targetRevision": source.TargetRevision,
			"argocd.healthStatus":   healthStatus,
			"argocd.syncStatus":     syncStatus,
		},
	}
}

// transformApplicationToStatus converts an ArgoCD Application to detailed status.
func (a *ArgoCDAdapter) transformApplicationToStatus(app *unstructured.Unstructured) *adapter.DeploymentStatusDetail {
	name, _, _ := unstructured.NestedString(app.Object, "metadata", "name")

	// Extract status fields
	healthStatus, _, _ := unstructured.NestedString(app.Object, "status", "health", "status")
	healthMessage, _, _ := unstructured.NestedString(app.Object, "status", "health", "message")
	syncStatus, _, _ := unstructured.NestedString(app.Object, "status", "sync", "status")
	syncRevision, _, _ := unstructured.NestedString(app.Object, "status", "sync", "revision")

	var updatedAt time.Time
	reconciledAt, _, _ := unstructured.NestedString(app.Object, "status", "reconciledAt")
	if reconciledAt != "" {
		if parsed, err := time.Parse(time.RFC3339, reconciledAt); err == nil {
			updatedAt = parsed
		}
	}
	if updatedAt.IsZero() {
		updatedAt = time.Now()
	}

	// Build conditions
	conditions := []adapter.DeploymentCondition{
		{
			Type:               "Health",
			Status:             healthStatus,
			Reason:             healthStatus,
			Message:            healthMessage,
			LastTransitionTime: updatedAt,
		},
		{
			Type:               "Sync",
			Status:             syncStatus,
			Reason:             syncStatus,
			Message:            fmt.Sprintf("Synced to revision: %s", syncRevision),
			LastTransitionTime: updatedAt,
		},
	}

	// Calculate progress
	progress := a.calculateProgress(healthStatus, syncStatus)

	// Build message
	message := fmt.Sprintf("Health: %s, Sync: %s", healthStatus, syncStatus)
	if healthMessage != "" {
		message = fmt.Sprintf("%s - %s", message, healthMessage)
	}

	return &adapter.DeploymentStatusDetail{
		DeploymentID: name,
		Status:       a.transformArgoCDStatus(healthStatus, syncStatus),
		Message:      message,
		Progress:     progress,
		Conditions:   conditions,
		UpdatedAt:    updatedAt,
		Extensions: map[string]interface{}{
			"argocd.healthStatus": healthStatus,
			"argocd.syncStatus":   syncStatus,
			"argocd.syncRevision": syncRevision,
		},
	}
}

// transformArgoCDStatus converts ArgoCD health and sync status to DMS deployment status.
func (a *ArgoCDAdapter) transformArgoCDStatus(healthStatus, syncStatus string) adapter.DeploymentStatus {
	// Health statuses: Healthy, Progressing, Degraded, Suspended, Missing, Unknown
	// Sync statuses: Synced, OutOfSync, Unknown

	switch healthStatus {
	case "Healthy":
		if syncStatus == "Synced" {
			return adapter.DeploymentStatusDeployed
		}
		return adapter.DeploymentStatusDeploying
	case "Progressing":
		return adapter.DeploymentStatusDeploying
	case "Degraded", "Missing":
		return adapter.DeploymentStatusFailed
	case "Suspended":
		return adapter.DeploymentStatusPending
	default:
		if syncStatus == "OutOfSync" {
			return adapter.DeploymentStatusDeploying
		}
		return adapter.DeploymentStatusFailed
	}
}

// calculateProgress estimates deployment progress based on ArgoCD status.
func (a *ArgoCDAdapter) calculateProgress(healthStatus, syncStatus string) int {
	switch healthStatus {
	case "Healthy":
		if syncStatus == "Synced" {
			return 100
		}
		return 90
	case "Progressing":
		return 50
	case "Suspended":
		return 25
	default:
		return 0
	}
}

// extractSource extracts source configuration from an ArgoCD Application.
func (a *ArgoCDAdapter) extractSource(app *unstructured.Unstructured) ApplicationSource {
	repoURL, _, _ := unstructured.NestedString(app.Object, "spec", "source", "repoURL")
	path, _, _ := unstructured.NestedString(app.Object, "spec", "source", "path")
	targetRevision, _, _ := unstructured.NestedString(app.Object, "spec", "source", "targetRevision")
	chart, _, _ := unstructured.NestedString(app.Object, "spec", "source", "chart")

	return ApplicationSource{
		RepoURL:        repoURL,
		Path:           path,
		TargetRevision: targetRevision,
		Chart:          chart,
	}
}

// applyPagination applies limit and offset to deployment list.
func (a *ArgoCDAdapter) applyPagination(deployments []*adapter.Deployment, limit, offset int) []*adapter.Deployment {
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

// ApplicationSource represents the source configuration of an ArgoCD Application.
type ApplicationSource struct {
	RepoURL        string
	Path           string
	TargetRevision string
	Chart          string
}

// generatePackageID creates a unique package ID from repository URL and path.
func generatePackageID(repoURL, path string) string {
	// Create a simple ID from repo and path
	id := repoURL
	if path != "" {
		id = fmt.Sprintf("%s/%s", repoURL, path)
	}
	// Replace special characters
	id = strings.ReplaceAll(id, "://", "-")
	id = strings.ReplaceAll(id, "/", "-")
	id = strings.ReplaceAll(id, ".", "-")
	return id
}

// buildLabelSelector creates a Kubernetes label selector string from a map.
func buildLabelSelector(labels map[string]string) string {
	selectors := make([]string, 0, len(labels))
	for k, v := range labels {
		selectors = append(selectors, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(selectors, ",")
}

// mustMarshalYAML converts a map to YAML string, returning empty on error.
func mustMarshalYAML(values map[string]interface{}) string {
	// Convert to JSON first (easier than full YAML)
	data, err := json.Marshal(values)
	if err != nil {
		return ""
	}
	return string(data)
}
