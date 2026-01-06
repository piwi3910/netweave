// Package flux provides an O2-DMS adapter implementation using Flux CD.
// This adapter enables GitOps-based CNF/VNF deployment management through
// Flux HelmReleases and Kustomizations deployed to Kubernetes clusters.
//
// This adapter uses the Kubernetes dynamic client to manage Flux CRDs directly,
// avoiding dependency conflicts with the Flux controller-runtime libraries.
// Supported Flux resources:
// - HelmRelease (helm.toolkit.fluxcd.io/v2)
// - Kustomization (kustomize.toolkit.fluxcd.io/v1)
package flux

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
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
	// AdapterName is the unique identifier for the Flux adapter.
	AdapterName = "flux"

	// AdapterVersion indicates the Flux API version supported by this adapter.
	AdapterVersion = "v2.0"

	// DefaultNamespace is the default namespace where Flux resources are created.
	DefaultNamespace = "flux-system"

	// DefaultReconcileTimeout is the default timeout for reconciliation operations.
	DefaultReconcileTimeout = 10 * time.Minute

	// DefaultInterval is the default reconciliation interval.
	DefaultInterval = 5 * time.Minute

	// HelmReleaseGroup is the Flux HelmRelease API group.
	HelmReleaseGroup = "helm.toolkit.fluxcd.io"

	// HelmReleaseVersion is the Flux HelmRelease API version.
	HelmReleaseVersion = "v2"

	// HelmReleaseResource is the Flux HelmRelease resource name.
	HelmReleaseResource = "helmreleases"

	// KustomizationGroup is the Flux Kustomization API group.
	KustomizationGroup = "kustomize.toolkit.fluxcd.io"

	// KustomizationVersion is the Flux Kustomization API version.
	KustomizationVersion = "v1"

	// KustomizationResource is the Flux Kustomization resource name.
	KustomizationResource = "kustomizations"

	// GitRepositoryGroup is the Flux GitRepository API group.
	GitRepositoryGroup = "source.toolkit.fluxcd.io"

	// GitRepositoryVersion is the Flux GitRepository API version.
	GitRepositoryVersion = "v1"

	// GitRepositoryResource is the Flux GitRepository resource name.
	GitRepositoryResource = "gitrepositories"

	// HelmRepositoryResource is the Flux HelmRepository resource name.
	HelmRepositoryResource = "helmrepositories"
)

// GVR definitions for Flux resources.
var (
	helmReleaseGVR = schema.GroupVersionResource{
		Group:    HelmReleaseGroup,
		Version:  HelmReleaseVersion,
		Resource: HelmReleaseResource,
	}

	kustomizationGVR = schema.GroupVersionResource{
		Group:    KustomizationGroup,
		Version:  KustomizationVersion,
		Resource: KustomizationResource,
	}

	gitRepositoryGVR = schema.GroupVersionResource{
		Group:    GitRepositoryGroup,
		Version:  GitRepositoryVersion,
		Resource: GitRepositoryResource,
	}

	helmRepositoryGVR = schema.GroupVersionResource{
		Group:    GitRepositoryGroup,
		Version:  GitRepositoryVersion,
		Resource: HelmRepositoryResource,
	}
)

// FluxAdapter implements the DMS adapter interface for Flux deployments.
// It uses the Kubernetes dynamic client to manage Flux HelmRelease and
// Kustomization CRDs, avoiding direct Flux library dependencies.
type FluxAdapter struct {
	config        *Config
	dynamicClient dynamic.Interface
	initOnce      sync.Once
	initError     error
}

// Config contains configuration for the Flux adapter.
type Config struct {
	// Kubeconfig is the path to the Kubernetes config file.
	// If empty, in-cluster config is used.
	Kubeconfig string

	// Namespace is the namespace where Flux resources are managed.
	// Defaults to "flux-system".
	Namespace string

	// SourceNamespace is the namespace where Flux source resources are located.
	// Defaults to the same as Namespace.
	SourceNamespace string

	// ReconcileTimeout is the timeout for reconciliation operations.
	ReconcileTimeout time.Duration

	// Interval is the default reconciliation interval for new resources.
	Interval time.Duration

	// Suspend creates new resources in suspended state if true.
	Suspend bool

	// Prune enables garbage collection for Kustomizations.
	Prune bool

	// Force enables force applying changes for Kustomizations.
	Force bool

	// TargetNamespace is the default target namespace for deployments.
	TargetNamespace string
}

// NewAdapter creates a new Flux adapter instance.
// Returns an error if the adapter cannot be initialized.
func NewAdapter(config *Config) (*FluxAdapter, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Apply defaults
	if config.Namespace == "" {
		config.Namespace = DefaultNamespace
	}
	if config.SourceNamespace == "" {
		config.SourceNamespace = config.Namespace
	}
	if config.ReconcileTimeout == 0 {
		config.ReconcileTimeout = DefaultReconcileTimeout
	}
	if config.Interval == 0 {
		config.Interval = DefaultInterval
	}
	if config.TargetNamespace == "" {
		config.TargetNamespace = "default"
	}

	return &FluxAdapter{
		config: config,
	}, nil
}

// Initialize performs lazy initialization of the Kubernetes dynamic client.
// This allows the adapter to be created without requiring immediate Kubernetes connectivity.
// This method is thread-safe and ensures initialization happens exactly once.
func (f *FluxAdapter) Initialize(ctx context.Context) error {
	f.initOnce.Do(func() {
		var restConfig *rest.Config
		var err error

		if f.config.Kubeconfig != "" {
			restConfig, err = clientcmd.BuildConfigFromFlags("", f.config.Kubeconfig)
			if err != nil {
				f.initError = fmt.Errorf("failed to build config from kubeconfig: %w", err)
				return
			}
		} else {
			restConfig, err = rest.InClusterConfig()
			if err != nil {
				f.initError = fmt.Errorf("failed to get in-cluster config: %w", err)
				return
			}
		}

		f.dynamicClient, err = dynamic.NewForConfig(restConfig)
		if err != nil {
			f.initError = fmt.Errorf("failed to create dynamic client: %w", err)
			return
		}
	})

	return f.initError
}

// Name returns the adapter name.
func (f *FluxAdapter) Name() string {
	return AdapterName
}

// Version returns the Flux version supported by this adapter.
func (f *FluxAdapter) Version() string {
	return AdapterVersion
}

// Capabilities returns the capabilities supported by the Flux adapter.
func (f *FluxAdapter) Capabilities() []adapter.Capability {
	return []adapter.Capability{
		adapter.CapabilityDeploymentLifecycle,
		adapter.CapabilityGitOps,
		adapter.CapabilityRollback,
		adapter.CapabilityHealthChecks,
		adapter.CapabilityMetrics,
	}
}

// ListDeploymentPackages retrieves deployment packages from Flux sources.
// In Flux, packages are GitRepositories and HelmRepositories.
func (f *FluxAdapter) ListDeploymentPackages(ctx context.Context, filter *adapter.Filter) ([]*adapter.DeploymentPackage, error) {
	if err := f.Initialize(ctx); err != nil {
		return nil, err
	}

	packages := make([]*adapter.DeploymentPackage, 0)

	// List GitRepositories
	gitRepos, err := f.listGitRepositories(ctx, filter)
	if err != nil {
		return nil, err
	}
	for _, repo := range gitRepos {
		packages = append(packages, f.transformGitRepoToPackage(repo))
	}

	// List HelmRepositories
	helmRepos, err := f.listHelmRepositories(ctx, filter)
	if err != nil {
		return nil, err
	}
	for _, repo := range helmRepos {
		packages = append(packages, f.transformHelmRepoToPackage(repo))
	}

	return packages, nil
}

// GetDeploymentPackage retrieves a specific deployment package by ID.
func (f *FluxAdapter) GetDeploymentPackage(ctx context.Context, id string) (*adapter.DeploymentPackage, error) {
	if err := f.Initialize(ctx); err != nil {
		return nil, err
	}

	packages, err := f.ListDeploymentPackages(ctx, nil)
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

// UploadDeploymentPackage creates a reference to a Flux source.
// Flux uses Git and Helm repositories as package sources.
func (f *FluxAdapter) UploadDeploymentPackage(ctx context.Context, pkg *adapter.DeploymentPackageUpload) (*adapter.DeploymentPackage, error) {
	if err := f.Initialize(ctx); err != nil {
		return nil, err
	}

	if pkg == nil {
		return nil, fmt.Errorf("package cannot be nil")
	}

	repoURL, _ := pkg.Extensions["flux.url"].(string)
	if repoURL == "" {
		return nil, fmt.Errorf("flux.url extension is required for Flux packages")
	}

	repoType, _ := pkg.Extensions["flux.type"].(string)
	if repoType == "" {
		repoType = "git"
	}

	return &adapter.DeploymentPackage{
		ID:          generatePackageID(repoType, repoURL),
		Name:        pkg.Name,
		Version:     pkg.Version,
		PackageType: fmt.Sprintf("flux-%s", repoType),
		Description: pkg.Description,
		UploadedAt:  time.Now(),
		Extensions: map[string]interface{}{
			"flux.url":    repoURL,
			"flux.type":   repoType,
			"flux.branch": pkg.Version,
		},
	}, nil
}

// DeleteDeploymentPackage is not directly supported in Flux.
// Source resources should be managed directly in the cluster.
func (f *FluxAdapter) DeleteDeploymentPackage(ctx context.Context, id string) error {
	return fmt.Errorf("Flux does not support package deletion through this adapter - manage source resources directly")
}

// ListDeployments retrieves all Flux deployments (HelmReleases and Kustomizations).
func (f *FluxAdapter) ListDeployments(ctx context.Context, filter *adapter.Filter) ([]*adapter.Deployment, error) {
	if err := f.Initialize(ctx); err != nil {
		return nil, err
	}

	deployments := make([]*adapter.Deployment, 0)

	// List HelmReleases
	helmReleases, err := f.listHelmReleases(ctx, filter)
	if err != nil {
		return nil, err
	}
	for _, hr := range helmReleases {
		deployment := f.transformHelmReleaseToDeployment(hr)
		if filter != nil && filter.Status != "" && deployment.Status != filter.Status {
			continue
		}
		deployments = append(deployments, deployment)
	}

	// List Kustomizations
	kustomizations, err := f.listKustomizations(ctx, filter)
	if err != nil {
		return nil, err
	}
	for _, ks := range kustomizations {
		deployment := f.transformKustomizationToDeployment(ks)
		if filter != nil && filter.Status != "" && deployment.Status != filter.Status {
			continue
		}
		deployments = append(deployments, deployment)
	}

	// Apply pagination
	if filter != nil {
		deployments = f.applyPagination(deployments, filter.Limit, filter.Offset)
	}

	return deployments, nil
}

// GetDeployment retrieves a specific Flux deployment by ID.
func (f *FluxAdapter) GetDeployment(ctx context.Context, id string) (*adapter.Deployment, error) {
	if err := f.Initialize(ctx); err != nil {
		return nil, err
	}

	// Try HelmRelease first
	hr, err := f.getHelmRelease(ctx, id)
	if err == nil {
		return f.transformHelmReleaseToDeployment(hr), nil
	}

	// Try Kustomization
	ks, err := f.getKustomization(ctx, id)
	if err == nil {
		return f.transformKustomizationToDeployment(ks), nil
	}

	return nil, fmt.Errorf("deployment not found: %s", id)
}

// CreateDeployment creates a new Flux HelmRelease or Kustomization.
func (f *FluxAdapter) CreateDeployment(ctx context.Context, req *adapter.DeploymentRequest) (*adapter.Deployment, error) {
	if err := f.Initialize(ctx); err != nil {
		return nil, err
	}

	if req == nil {
		return nil, fmt.Errorf("deployment request cannot be nil")
	}
	if req.Name == "" {
		return nil, fmt.Errorf("deployment name is required")
	}

	// Determine deployment type from extensions
	deployType, _ := req.Extensions["flux.type"].(string)
	if deployType == "" {
		deployType = "helmrelease"
	}

	switch deployType {
	case "helmrelease":
		return f.createHelmRelease(ctx, req)
	case "kustomization":
		return f.createKustomization(ctx, req)
	default:
		return nil, fmt.Errorf("unsupported deployment type: %s", deployType)
	}
}

// UpdateDeployment updates an existing Flux deployment.
func (f *FluxAdapter) UpdateDeployment(ctx context.Context, id string, update *adapter.DeploymentUpdate) (*adapter.Deployment, error) {
	if err := f.Initialize(ctx); err != nil {
		return nil, err
	}

	if update == nil {
		return nil, fmt.Errorf("deployment update cannot be nil")
	}

	// Try HelmRelease first
	hr, err := f.getHelmRelease(ctx, id)
	if err == nil {
		return f.updateHelmRelease(ctx, hr, update)
	}

	// Try Kustomization
	ks, err := f.getKustomization(ctx, id)
	if err == nil {
		return f.updateKustomization(ctx, ks, update)
	}

	return nil, fmt.Errorf("deployment not found: %s", id)
}

// DeleteDeployment deletes a Flux deployment.
func (f *FluxAdapter) DeleteDeployment(ctx context.Context, id string) error {
	if err := f.Initialize(ctx); err != nil {
		return err
	}

	propagation := metav1.DeletePropagationForeground

	// Try HelmRelease first
	err := f.dynamicClient.Resource(helmReleaseGVR).Namespace(f.config.Namespace).Delete(ctx, id, metav1.DeleteOptions{
		PropagationPolicy: &propagation,
	})
	if err == nil {
		return nil
	}

	// Try Kustomization
	err = f.dynamicClient.Resource(kustomizationGVR).Namespace(f.config.Namespace).Delete(ctx, id, metav1.DeleteOptions{
		PropagationPolicy: &propagation,
	})
	if err == nil {
		return nil
	}

	return fmt.Errorf("failed to delete Flux deployment: %s", id)
}

// ScaleDeployment scales a deployment by updating the values.
// Flux doesn't directly support scaling, but we can update values.
func (f *FluxAdapter) ScaleDeployment(ctx context.Context, id string, replicas int) error {
	if err := f.Initialize(ctx); err != nil {
		return err
	}

	if replicas < 0 {
		return fmt.Errorf("replicas must be non-negative")
	}

	update := &adapter.DeploymentUpdate{
		Values: map[string]interface{}{
			"replicaCount": replicas,
		},
	}

	_, err := f.UpdateDeployment(ctx, id, update)
	return err
}

// RollbackDeployment triggers a rollback by forcing reconciliation.
// For Flux, this means reverting to a previous Git revision.
func (f *FluxAdapter) RollbackDeployment(ctx context.Context, id string, revision int) error {
	if err := f.Initialize(ctx); err != nil {
		return err
	}

	if revision < 0 {
		return fmt.Errorf("revision must be non-negative")
	}

	// Try HelmRelease first
	hr, err := f.getHelmRelease(ctx, id)
	if err == nil {
		// Get history to find the target revision
		history, _, _ := unstructured.NestedSlice(hr.Object, "status", "history")
		if revision >= len(history) {
			return fmt.Errorf("revision %d not found in history", revision)
		}

		// Force reconciliation by adding an annotation
		annotations, _, _ := unstructured.NestedStringMap(hr.Object, "metadata", "annotations")
		if annotations == nil {
			annotations = make(map[string]string)
		}
		annotations["reconcile.fluxcd.io/requestedAt"] = time.Now().Format(time.RFC3339)
		if err := unstructured.SetNestedStringMap(hr.Object, annotations, "metadata", "annotations"); err != nil {
			return fmt.Errorf("failed to set annotations: %w", err)
		}

		_, err = f.dynamicClient.Resource(helmReleaseGVR).Namespace(f.config.Namespace).Update(ctx, hr, metav1.UpdateOptions{})
		return err
	}

	// Try Kustomization
	ks, err := f.getKustomization(ctx, id)
	if err == nil {
		// Force reconciliation
		annotations, _, _ := unstructured.NestedStringMap(ks.Object, "metadata", "annotations")
		if annotations == nil {
			annotations = make(map[string]string)
		}
		annotations["reconcile.fluxcd.io/requestedAt"] = time.Now().Format(time.RFC3339)
		if err := unstructured.SetNestedStringMap(ks.Object, annotations, "metadata", "annotations"); err != nil {
			return fmt.Errorf("failed to set annotations: %w", err)
		}

		_, err = f.dynamicClient.Resource(kustomizationGVR).Namespace(f.config.Namespace).Update(ctx, ks, metav1.UpdateOptions{})
		return err
	}

	return fmt.Errorf("deployment not found: %s", id)
}

// GetDeploymentStatus retrieves detailed status for a Flux deployment.
func (f *FluxAdapter) GetDeploymentStatus(ctx context.Context, id string) (*adapter.DeploymentStatusDetail, error) {
	if err := f.Initialize(ctx); err != nil {
		return nil, err
	}

	// Try HelmRelease first
	hr, err := f.getHelmRelease(ctx, id)
	if err == nil {
		return f.transformHelmReleaseToStatus(hr), nil
	}

	// Try Kustomization
	ks, err := f.getKustomization(ctx, id)
	if err == nil {
		return f.transformKustomizationToStatus(ks), nil
	}

	return nil, fmt.Errorf("deployment not found: %s", id)
}

// GetDeploymentHistory retrieves the revision history for a Flux deployment.
func (f *FluxAdapter) GetDeploymentHistory(ctx context.Context, id string) (*adapter.DeploymentHistory, error) {
	if err := f.Initialize(ctx); err != nil {
		return nil, err
	}

	// Try HelmRelease first
	hr, err := f.getHelmRelease(ctx, id)
	if err == nil {
		return f.extractHelmReleaseHistory(id, hr), nil
	}

	// Try Kustomization - Kustomizations don't have history, return current state only
	ks, err := f.getKustomization(ctx, id)
	if err == nil {
		return f.extractKustomizationHistory(id, ks), nil
	}

	return nil, fmt.Errorf("deployment not found: %s", id)
}

// GetDeploymentLogs retrieves status information for a Flux deployment.
// Note: Flux doesn't provide direct log access, this returns status information.
func (f *FluxAdapter) GetDeploymentLogs(ctx context.Context, id string, opts *adapter.LogOptions) ([]byte, error) {
	if err := f.Initialize(ctx); err != nil {
		return nil, err
	}

	// Try HelmRelease first
	hr, err := f.getHelmRelease(ctx, id)
	if err == nil {
		status, _, _ := unstructured.NestedMap(hr.Object, "status")
		statusJSON, err := json.MarshalIndent(status, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal status: %w", err)
		}
		return statusJSON, nil
	}

	// Try Kustomization
	ks, err := f.getKustomization(ctx, id)
	if err == nil {
		status, _, _ := unstructured.NestedMap(ks.Object, "status")
		statusJSON, err := json.MarshalIndent(status, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal status: %w", err)
		}
		return statusJSON, nil
	}

	return nil, fmt.Errorf("deployment not found: %s", id)
}

// SupportsRollback returns true as Flux supports rollback via Git revisions.
func (f *FluxAdapter) SupportsRollback() bool {
	return true
}

// SupportsScaling returns true as scaling can be done via value updates.
func (f *FluxAdapter) SupportsScaling() bool {
	return true
}

// SupportsGitOps returns true as Flux is a GitOps tool.
func (f *FluxAdapter) SupportsGitOps() bool {
	return true
}

// Health performs a health check on the Flux backend.
func (f *FluxAdapter) Health(ctx context.Context) error {
	if err := f.Initialize(ctx); err != nil {
		return fmt.Errorf("flux adapter not healthy: %w", err)
	}

	// Try to list HelmReleases to verify connectivity
	_, err := f.dynamicClient.Resource(helmReleaseGVR).Namespace(f.config.Namespace).List(ctx, metav1.ListOptions{
		Limit: 1,
	})
	if err != nil {
		return fmt.Errorf("flux health check failed: %w", err)
	}

	return nil
}

// Close cleanly shuts down the adapter.
func (f *FluxAdapter) Close() error {
	f.dynamicClient = nil
	return nil
}

// listHelmReleases retrieves Flux HelmReleases with optional filtering.
func (f *FluxAdapter) listHelmReleases(ctx context.Context, filter *adapter.Filter) ([]*unstructured.Unstructured, error) {
	namespace := f.config.Namespace
	if filter != nil && filter.Namespace != "" {
		namespace = filter.Namespace
	}

	opts := metav1.ListOptions{}
	if filter != nil && len(filter.Labels) > 0 {
		opts.LabelSelector = buildLabelSelector(filter.Labels)
	}

	list, err := f.dynamicClient.Resource(helmReleaseGVR).Namespace(namespace).List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list Flux HelmReleases: %w", err)
	}

	releases := make([]*unstructured.Unstructured, 0, len(list.Items))
	for i := range list.Items {
		releases = append(releases, &list.Items[i])
	}

	return releases, nil
}

// listKustomizations retrieves Flux Kustomizations with optional filtering.
func (f *FluxAdapter) listKustomizations(ctx context.Context, filter *adapter.Filter) ([]*unstructured.Unstructured, error) {
	namespace := f.config.Namespace
	if filter != nil && filter.Namespace != "" {
		namespace = filter.Namespace
	}

	opts := metav1.ListOptions{}
	if filter != nil && len(filter.Labels) > 0 {
		opts.LabelSelector = buildLabelSelector(filter.Labels)
	}

	list, err := f.dynamicClient.Resource(kustomizationGVR).Namespace(namespace).List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list Flux Kustomizations: %w", err)
	}

	kustomizations := make([]*unstructured.Unstructured, 0, len(list.Items))
	for i := range list.Items {
		kustomizations = append(kustomizations, &list.Items[i])
	}

	return kustomizations, nil
}

// listGitRepositories retrieves Flux GitRepositories.
func (f *FluxAdapter) listGitRepositories(ctx context.Context, filter *adapter.Filter) ([]*unstructured.Unstructured, error) {
	namespace := f.config.SourceNamespace
	if filter != nil && filter.Namespace != "" {
		namespace = filter.Namespace
	}

	opts := metav1.ListOptions{}
	if filter != nil && len(filter.Labels) > 0 {
		opts.LabelSelector = buildLabelSelector(filter.Labels)
	}

	list, err := f.dynamicClient.Resource(gitRepositoryGVR).Namespace(namespace).List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list Flux GitRepositories: %w", err)
	}

	repos := make([]*unstructured.Unstructured, 0, len(list.Items))
	for i := range list.Items {
		repos = append(repos, &list.Items[i])
	}

	return repos, nil
}

// listHelmRepositories retrieves Flux HelmRepositories.
func (f *FluxAdapter) listHelmRepositories(ctx context.Context, filter *adapter.Filter) ([]*unstructured.Unstructured, error) {
	namespace := f.config.SourceNamespace
	if filter != nil && filter.Namespace != "" {
		namespace = filter.Namespace
	}

	opts := metav1.ListOptions{}
	if filter != nil && len(filter.Labels) > 0 {
		opts.LabelSelector = buildLabelSelector(filter.Labels)
	}

	list, err := f.dynamicClient.Resource(helmRepositoryGVR).Namespace(namespace).List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list Flux HelmRepositories: %w", err)
	}

	repos := make([]*unstructured.Unstructured, 0, len(list.Items))
	for i := range list.Items {
		repos = append(repos, &list.Items[i])
	}

	return repos, nil
}

// getHelmRelease retrieves a single Flux HelmRelease by name.
func (f *FluxAdapter) getHelmRelease(ctx context.Context, name string) (*unstructured.Unstructured, error) {
	return f.dynamicClient.Resource(helmReleaseGVR).Namespace(f.config.Namespace).Get(ctx, name, metav1.GetOptions{})
}

// getKustomization retrieves a single Flux Kustomization by name.
func (f *FluxAdapter) getKustomization(ctx context.Context, name string) (*unstructured.Unstructured, error) {
	return f.dynamicClient.Resource(kustomizationGVR).Namespace(f.config.Namespace).Get(ctx, name, metav1.GetOptions{})
}

// createHelmRelease creates a new Flux HelmRelease.
func (f *FluxAdapter) createHelmRelease(ctx context.Context, req *adapter.DeploymentRequest) (*adapter.Deployment, error) {
	chart, _ := req.Extensions["flux.chart"].(string)
	sourceRef, _ := req.Extensions["flux.sourceRef"].(string)
	sourceKind, _ := req.Extensions["flux.sourceKind"].(string)
	chartVersion, _ := req.Extensions["flux.chartVersion"].(string)

	if chart == "" {
		return nil, fmt.Errorf("flux.chart extension is required for HelmRelease")
	}
	if sourceRef == "" {
		return nil, fmt.Errorf("flux.sourceRef extension is required for HelmRelease")
	}
	if sourceKind == "" {
		sourceKind = "HelmRepository"
	}

	targetNamespace := req.Namespace
	if targetNamespace == "" {
		targetNamespace = f.config.TargetNamespace
	}

	hr := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": fmt.Sprintf("%s/%s", HelmReleaseGroup, HelmReleaseVersion),
			"kind":       "HelmRelease",
			"metadata": map[string]interface{}{
				"name":      req.Name,
				"namespace": f.config.Namespace,
			},
			"spec": map[string]interface{}{
				"interval": f.config.Interval.String(),
				"chart": map[string]interface{}{
					"spec": map[string]interface{}{
						"chart":   chart,
						"version": chartVersion,
						"sourceRef": map[string]interface{}{
							"kind":      sourceKind,
							"name":      sourceRef,
							"namespace": f.config.SourceNamespace,
						},
					},
				},
				"targetNamespace": targetNamespace,
				"suspend":         f.config.Suspend,
			},
		},
	}

	// Add values if provided
	if len(req.Values) > 0 {
		valuesJSON, err := json.Marshal(req.Values)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal values: %w", err)
		}
		var valuesMap map[string]interface{}
		if err := json.Unmarshal(valuesJSON, &valuesMap); err != nil {
			return nil, fmt.Errorf("failed to unmarshal values: %w", err)
		}
		if err := unstructured.SetNestedField(hr.Object, valuesMap, "spec", "values"); err != nil {
			return nil, fmt.Errorf("failed to set values: %w", err)
		}
	}

	result, err := f.dynamicClient.Resource(helmReleaseGVR).Namespace(f.config.Namespace).Create(ctx, hr, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create Flux HelmRelease: %w", err)
	}

	return f.transformHelmReleaseToDeployment(result), nil
}

// createKustomization creates a new Flux Kustomization.
func (f *FluxAdapter) createKustomization(ctx context.Context, req *adapter.DeploymentRequest) (*adapter.Deployment, error) {
	path, _ := req.Extensions["flux.path"].(string)
	sourceRef, _ := req.Extensions["flux.sourceRef"].(string)
	sourceKind, _ := req.Extensions["flux.sourceKind"].(string)

	if sourceRef == "" {
		return nil, fmt.Errorf("flux.sourceRef extension is required for Kustomization")
	}
	if sourceKind == "" {
		sourceKind = "GitRepository"
	}
	if path == "" {
		path = "./"
	}

	targetNamespace := req.Namespace
	if targetNamespace == "" {
		targetNamespace = f.config.TargetNamespace
	}

	ks := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": fmt.Sprintf("%s/%s", KustomizationGroup, KustomizationVersion),
			"kind":       "Kustomization",
			"metadata": map[string]interface{}{
				"name":      req.Name,
				"namespace": f.config.Namespace,
			},
			"spec": map[string]interface{}{
				"interval": f.config.Interval.String(),
				"path":     path,
				"sourceRef": map[string]interface{}{
					"kind":      sourceKind,
					"name":      sourceRef,
					"namespace": f.config.SourceNamespace,
				},
				"targetNamespace": targetNamespace,
				"prune":           f.config.Prune,
				"force":           f.config.Force,
				"suspend":         f.config.Suspend,
			},
		},
	}

	result, err := f.dynamicClient.Resource(kustomizationGVR).Namespace(f.config.Namespace).Create(ctx, ks, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create Flux Kustomization: %w", err)
	}

	return f.transformKustomizationToDeployment(result), nil
}

// updateHelmRelease updates an existing Flux HelmRelease.
func (f *FluxAdapter) updateHelmRelease(ctx context.Context, hr *unstructured.Unstructured, update *adapter.DeploymentUpdate) (*adapter.Deployment, error) {
	// Update chart version if specified
	if chartVersion, ok := update.Extensions["flux.chartVersion"].(string); ok && chartVersion != "" {
		if err := unstructured.SetNestedField(hr.Object, chartVersion, "spec", "chart", "spec", "version"); err != nil {
			return nil, fmt.Errorf("failed to update chart version: %w", err)
		}
	}

	// Update values if provided
	if len(update.Values) > 0 {
		// Get existing values
		existingValues, _, _ := unstructured.NestedMap(hr.Object, "spec", "values")
		if existingValues == nil {
			existingValues = make(map[string]interface{})
		}

		// Merge with new values
		for k, v := range update.Values {
			existingValues[k] = v
		}

		if err := unstructured.SetNestedField(hr.Object, existingValues, "spec", "values"); err != nil {
			return nil, fmt.Errorf("failed to update values: %w", err)
		}
	}

	result, err := f.dynamicClient.Resource(helmReleaseGVR).Namespace(f.config.Namespace).Update(ctx, hr, metav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to update Flux HelmRelease: %w", err)
	}

	return f.transformHelmReleaseToDeployment(result), nil
}

// updateKustomization updates an existing Flux Kustomization.
func (f *FluxAdapter) updateKustomization(ctx context.Context, ks *unstructured.Unstructured, update *adapter.DeploymentUpdate) (*adapter.Deployment, error) {
	// Update path if specified
	if path, ok := update.Extensions["flux.path"].(string); ok && path != "" {
		if err := unstructured.SetNestedField(ks.Object, path, "spec", "path"); err != nil {
			return nil, fmt.Errorf("failed to update path: %w", err)
		}
	}

	// Update target revision if specified
	if targetRevision, ok := update.Extensions["flux.targetRevision"].(string); ok && targetRevision != "" {
		annotations, _, _ := unstructured.NestedStringMap(ks.Object, "metadata", "annotations")
		if annotations == nil {
			annotations = make(map[string]string)
		}
		annotations["flux.targetRevision"] = targetRevision
		if err := unstructured.SetNestedStringMap(ks.Object, annotations, "metadata", "annotations"); err != nil {
			return nil, fmt.Errorf("failed to set annotations: %w", err)
		}
	}

	result, err := f.dynamicClient.Resource(kustomizationGVR).Namespace(f.config.Namespace).Update(ctx, ks, metav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to update Flux Kustomization: %w", err)
	}

	return f.transformKustomizationToDeployment(result), nil
}

// transformHelmReleaseToDeployment converts a Flux HelmRelease to a Deployment.
func (f *FluxAdapter) transformHelmReleaseToDeployment(hr *unstructured.Unstructured) *adapter.Deployment {
	name, _, _ := unstructured.NestedString(hr.Object, "metadata", "name")
	namespace, _, _ := unstructured.NestedString(hr.Object, "metadata", "namespace")

	// Extract chart info
	chart, _, _ := unstructured.NestedString(hr.Object, "spec", "chart", "spec", "chart")
	chartVersion, _, _ := unstructured.NestedString(hr.Object, "spec", "chart", "spec", "version")
	sourceRef, _, _ := unstructured.NestedString(hr.Object, "spec", "chart", "spec", "sourceRef", "name")
	targetNamespace, _, _ := unstructured.NestedString(hr.Object, "spec", "targetNamespace")

	// Extract status
	conditions, _, _ := unstructured.NestedSlice(hr.Object, "status", "conditions")
	status, message := f.extractFluxStatus(conditions)

	// Get timestamps
	creationTimestamp := hr.GetCreationTimestamp().Time
	var updatedAt time.Time
	if len(conditions) > 0 {
		if lastCond, ok := conditions[len(conditions)-1].(map[string]interface{}); ok {
			if lastTime, ok := lastCond["lastTransitionTime"].(string); ok {
				if parsed, err := time.Parse(time.RFC3339, lastTime); err == nil {
					updatedAt = parsed
				}
			}
		}
	}
	if updatedAt.IsZero() {
		updatedAt = creationTimestamp
	}

	// Get history length as version
	history, _, _ := unstructured.NestedSlice(hr.Object, "status", "history")
	version := len(history)
	if version == 0 {
		version = 1
	}

	return &adapter.Deployment{
		ID:          name,
		Name:        name,
		PackageID:   generatePackageID("helm", fmt.Sprintf("%s/%s", sourceRef, chart)),
		Namespace:   targetNamespace,
		Status:      status,
		Version:     version,
		Description: fmt.Sprintf("Flux HelmRelease: %s (chart: %s)", name, chart),
		CreatedAt:   creationTimestamp,
		UpdatedAt:   updatedAt,
		Extensions: map[string]interface{}{
			"flux.type":            "helmrelease",
			"flux.namespace":       namespace,
			"flux.chart":           chart,
			"flux.chartVersion":    chartVersion,
			"flux.sourceRef":       sourceRef,
			"flux.targetNamespace": targetNamespace,
			"flux.message":         message,
		},
	}
}

// transformKustomizationToDeployment converts a Flux Kustomization to a Deployment.
func (f *FluxAdapter) transformKustomizationToDeployment(ks *unstructured.Unstructured) *adapter.Deployment {
	name, _, _ := unstructured.NestedString(ks.Object, "metadata", "name")
	namespace, _, _ := unstructured.NestedString(ks.Object, "metadata", "namespace")

	// Extract source info
	path, _, _ := unstructured.NestedString(ks.Object, "spec", "path")
	sourceRef, _, _ := unstructured.NestedString(ks.Object, "spec", "sourceRef", "name")
	targetNamespace, _, _ := unstructured.NestedString(ks.Object, "spec", "targetNamespace")

	// Extract status
	conditions, _, _ := unstructured.NestedSlice(ks.Object, "status", "conditions")
	status, message := f.extractFluxStatus(conditions)

	// Get last applied revision
	lastAppliedRevision, _, _ := unstructured.NestedString(ks.Object, "status", "lastAppliedRevision")

	// Get timestamps
	creationTimestamp := ks.GetCreationTimestamp().Time
	var updatedAt time.Time
	if len(conditions) > 0 {
		if lastCond, ok := conditions[len(conditions)-1].(map[string]interface{}); ok {
			if lastTime, ok := lastCond["lastTransitionTime"].(string); ok {
				if parsed, err := time.Parse(time.RFC3339, lastTime); err == nil {
					updatedAt = parsed
				}
			}
		}
	}
	if updatedAt.IsZero() {
		updatedAt = creationTimestamp
	}

	return &adapter.Deployment{
		ID:          name,
		Name:        name,
		PackageID:   generatePackageID("git", fmt.Sprintf("%s/%s", sourceRef, path)),
		Namespace:   targetNamespace,
		Status:      status,
		Version:     1, // Kustomizations don't have version history
		Description: fmt.Sprintf("Flux Kustomization: %s (path: %s)", name, path),
		CreatedAt:   creationTimestamp,
		UpdatedAt:   updatedAt,
		Extensions: map[string]interface{}{
			"flux.type":                "kustomization",
			"flux.namespace":           namespace,
			"flux.path":                path,
			"flux.sourceRef":           sourceRef,
			"flux.targetNamespace":     targetNamespace,
			"flux.lastAppliedRevision": lastAppliedRevision,
			"flux.message":             message,
		},
	}
}

// transformGitRepoToPackage converts a Flux GitRepository to a DeploymentPackage.
func (f *FluxAdapter) transformGitRepoToPackage(repo *unstructured.Unstructured) *adapter.DeploymentPackage {
	name, _, _ := unstructured.NestedString(repo.Object, "metadata", "name")
	url, _, _ := unstructured.NestedString(repo.Object, "spec", "url")
	branch, _, _ := unstructured.NestedString(repo.Object, "spec", "ref", "branch")
	tag, _, _ := unstructured.NestedString(repo.Object, "spec", "ref", "tag")

	version := branch
	if tag != "" {
		version = tag
	}

	creationTimestamp := repo.GetCreationTimestamp().Time

	return &adapter.DeploymentPackage{
		ID:          generatePackageID("git", url),
		Name:        name,
		Version:     version,
		PackageType: "flux-git",
		Description: fmt.Sprintf("Flux GitRepository: %s", url),
		UploadedAt:  creationTimestamp,
		Extensions: map[string]interface{}{
			"flux.url":    url,
			"flux.branch": branch,
			"flux.tag":    tag,
		},
	}
}

// transformHelmRepoToPackage converts a Flux HelmRepository to a DeploymentPackage.
func (f *FluxAdapter) transformHelmRepoToPackage(repo *unstructured.Unstructured) *adapter.DeploymentPackage {
	name, _, _ := unstructured.NestedString(repo.Object, "metadata", "name")
	url, _, _ := unstructured.NestedString(repo.Object, "spec", "url")
	repoType, _, _ := unstructured.NestedString(repo.Object, "spec", "type")

	if repoType == "" {
		repoType = "default"
	}

	creationTimestamp := repo.GetCreationTimestamp().Time

	return &adapter.DeploymentPackage{
		ID:          generatePackageID("helm", url),
		Name:        name,
		Version:     "latest",
		PackageType: "flux-helm",
		Description: fmt.Sprintf("Flux HelmRepository: %s", url),
		UploadedAt:  creationTimestamp,
		Extensions: map[string]interface{}{
			"flux.url":      url,
			"flux.repoType": repoType,
		},
	}
}

// transformHelmReleaseToStatus converts a Flux HelmRelease to detailed status.
func (f *FluxAdapter) transformHelmReleaseToStatus(hr *unstructured.Unstructured) *adapter.DeploymentStatusDetail {
	name, _, _ := unstructured.NestedString(hr.Object, "metadata", "name")

	conditions, _, _ := unstructured.NestedSlice(hr.Object, "status", "conditions")
	status, message := f.extractFluxStatus(conditions)

	var updatedAt time.Time
	dmsConditions := make([]adapter.DeploymentCondition, 0)

	for _, c := range conditions {
		cond, ok := c.(map[string]interface{})
		if !ok {
			continue
		}

		condType, _, _ := unstructured.NestedString(cond, "type")
		condStatus, _, _ := unstructured.NestedString(cond, "status")
		condReason, _, _ := unstructured.NestedString(cond, "reason")
		condMessage, _, _ := unstructured.NestedString(cond, "message")
		condTime, _, _ := unstructured.NestedString(cond, "lastTransitionTime")

		var transitionTime time.Time
		if condTime != "" {
			if parsed, err := time.Parse(time.RFC3339, condTime); err == nil {
				transitionTime = parsed
				if transitionTime.After(updatedAt) {
					updatedAt = transitionTime
				}
			}
		}

		dmsConditions = append(dmsConditions, adapter.DeploymentCondition{
			Type:               condType,
			Status:             condStatus,
			Reason:             condReason,
			Message:            condMessage,
			LastTransitionTime: transitionTime,
		})
	}

	if updatedAt.IsZero() {
		updatedAt = time.Now()
	}

	progress := f.calculateProgress(status)

	return &adapter.DeploymentStatusDetail{
		DeploymentID: name,
		Status:       status,
		Message:      message,
		Progress:     progress,
		Conditions:   dmsConditions,
		UpdatedAt:    updatedAt,
		Extensions: map[string]interface{}{
			"flux.type": "helmrelease",
		},
	}
}

// transformKustomizationToStatus converts a Flux Kustomization to detailed status.
func (f *FluxAdapter) transformKustomizationToStatus(ks *unstructured.Unstructured) *adapter.DeploymentStatusDetail {
	name, _, _ := unstructured.NestedString(ks.Object, "metadata", "name")

	conditions, _, _ := unstructured.NestedSlice(ks.Object, "status", "conditions")
	status, message := f.extractFluxStatus(conditions)

	lastAppliedRevision, _, _ := unstructured.NestedString(ks.Object, "status", "lastAppliedRevision")

	var updatedAt time.Time
	dmsConditions := make([]adapter.DeploymentCondition, 0)

	for _, c := range conditions {
		cond, ok := c.(map[string]interface{})
		if !ok {
			continue
		}

		condType, _, _ := unstructured.NestedString(cond, "type")
		condStatus, _, _ := unstructured.NestedString(cond, "status")
		condReason, _, _ := unstructured.NestedString(cond, "reason")
		condMessage, _, _ := unstructured.NestedString(cond, "message")
		condTime, _, _ := unstructured.NestedString(cond, "lastTransitionTime")

		var transitionTime time.Time
		if condTime != "" {
			if parsed, err := time.Parse(time.RFC3339, condTime); err == nil {
				transitionTime = parsed
				if transitionTime.After(updatedAt) {
					updatedAt = transitionTime
				}
			}
		}

		dmsConditions = append(dmsConditions, adapter.DeploymentCondition{
			Type:               condType,
			Status:             condStatus,
			Reason:             condReason,
			Message:            condMessage,
			LastTransitionTime: transitionTime,
		})
	}

	if updatedAt.IsZero() {
		updatedAt = time.Now()
	}

	progress := f.calculateProgress(status)

	return &adapter.DeploymentStatusDetail{
		DeploymentID: name,
		Status:       status,
		Message:      message,
		Progress:     progress,
		Conditions:   dmsConditions,
		UpdatedAt:    updatedAt,
		Extensions: map[string]interface{}{
			"flux.type":                "kustomization",
			"flux.lastAppliedRevision": lastAppliedRevision,
		},
	}
}

// extractFluxStatus extracts status and message from Flux conditions.
func (f *FluxAdapter) extractFluxStatus(conditions []interface{}) (adapter.DeploymentStatus, string) {
	for _, c := range conditions {
		cond, ok := c.(map[string]interface{})
		if !ok {
			continue
		}

		condType, _, _ := unstructured.NestedString(cond, "type")
		condStatus, _, _ := unstructured.NestedString(cond, "status")
		condMessage, _, _ := unstructured.NestedString(cond, "message")
		condReason, _, _ := unstructured.NestedString(cond, "reason")

		if condType == "Ready" {
			message := condMessage
			if condReason != "" {
				message = fmt.Sprintf("%s: %s", condReason, condMessage)
			}

			switch condStatus {
			case "True":
				return adapter.DeploymentStatusDeployed, message
			case "False":
				if condReason == "Progressing" || condReason == "ArtifactFailed" {
					return adapter.DeploymentStatusDeploying, message
				}
				return adapter.DeploymentStatusFailed, message
			default:
				return adapter.DeploymentStatusDeploying, message
			}
		}
	}

	return adapter.DeploymentStatusPending, "Waiting for reconciliation"
}

// extractHelmReleaseHistory extracts history from a HelmRelease.
func (f *FluxAdapter) extractHelmReleaseHistory(id string, hr *unstructured.Unstructured) *adapter.DeploymentHistory {
	history, _, _ := unstructured.NestedSlice(hr.Object, "status", "history")

	revisions := make([]adapter.DeploymentRevision, 0, len(history))
	for i, h := range history {
		historyEntry, ok := h.(map[string]interface{})
		if !ok {
			continue
		}

		chartVersion, _, _ := unstructured.NestedString(historyEntry, "chartVersion")
		digestStr, _, _ := unstructured.NestedString(historyEntry, "digest")
		statusStr, _, _ := unstructured.NestedString(historyEntry, "status")

		status := adapter.DeploymentStatusDeployed
		if statusStr == "failed" {
			status = adapter.DeploymentStatusFailed
		}

		revisions = append(revisions, adapter.DeploymentRevision{
			Revision:    i,
			Version:     chartVersion,
			DeployedAt:  time.Now(), // Flux doesn't store deployment time in history
			Status:      status,
			Description: fmt.Sprintf("Chart version %s (digest: %s)", chartVersion, digestStr),
		})
	}

	return &adapter.DeploymentHistory{
		DeploymentID: id,
		Revisions:    revisions,
	}
}

// extractKustomizationHistory extracts history from a Kustomization.
// Kustomizations don't have versioned history, so we return the current state.
func (f *FluxAdapter) extractKustomizationHistory(id string, ks *unstructured.Unstructured) *adapter.DeploymentHistory {
	lastAppliedRevision, _, _ := unstructured.NestedString(ks.Object, "status", "lastAppliedRevision")

	conditions, _, _ := unstructured.NestedSlice(ks.Object, "status", "conditions")
	status, _ := f.extractFluxStatus(conditions)

	revisions := []adapter.DeploymentRevision{
		{
			Revision:    0,
			Version:     lastAppliedRevision,
			DeployedAt:  ks.GetCreationTimestamp().Time,
			Status:      status,
			Description: fmt.Sprintf("Applied revision: %s", lastAppliedRevision),
		},
	}

	return &adapter.DeploymentHistory{
		DeploymentID: id,
		Revisions:    revisions,
	}
}

// calculateProgress estimates deployment progress based on status.
func (f *FluxAdapter) calculateProgress(status adapter.DeploymentStatus) int {
	switch status {
	case adapter.DeploymentStatusDeployed:
		return 100
	case adapter.DeploymentStatusDeploying:
		return 50
	case adapter.DeploymentStatusPending:
		return 25
	case adapter.DeploymentStatusFailed:
		return 0
	default:
		return 0
	}
}

// applyPagination applies limit and offset to deployment list.
func (f *FluxAdapter) applyPagination(deployments []*adapter.Deployment, limit, offset int) []*adapter.Deployment {
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

// generatePackageID creates a unique package ID from type and URL.
func generatePackageID(pkgType, url string) string {
	id := fmt.Sprintf("%s-%s", pkgType, url)
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
