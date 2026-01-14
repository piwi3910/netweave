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
	"errors"
	"fmt"
	"reflect"
	"regexp"
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

// Sentinel errors for Flux adapter operations.
// These provide typed errors for better error handling by callers.
var (
	// ErrDeploymentNotFound is returned when a deployment cannot be found.
	ErrDeploymentNotFound = errors.New("deployment not found")

	// ErrPackageNotFound is returned when a deployment package cannot be found.
	ErrPackageNotFound = errors.New("deployment package not found")

	// ErrInvalidName is returned when a resource name fails validation.
	ErrInvalidName = errors.New("invalid resource name")

	// ErrInvalidPath is returned when a path contains invalid characters or traversal attempts.
	ErrInvalidPath = errors.New("invalid path")

	// ErrOperationNotSupported is returned for operations not supported by Flux.
	ErrOperationNotSupported = errors.New("operation not supported")
)

// dns1123LabelRegex validates DNS-1123 label format for Kubernetes resource names.
var dns1123LabelRegex = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

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

	// progressDeployed indicates a fully deployed and healthy deployment (100%).
	progressDeployed = 100

	// progressDeploying indicates an in-progress deployment (50%).
	progressDeploying = 50

	// progressPending indicates a deployment waiting to start (25%).
	progressPending = 25

	// progressFailed indicates a failed deployment (0%).
	progressFailed = 0
)

// GVR definitions for Flux resources.
var (
	HelmReleaseGVR = schema.GroupVersionResource{
		Group:    HelmReleaseGroup,
		Version:  HelmReleaseVersion,
		Resource: HelmReleaseResource,
	}

	KustomizationGVR = schema.GroupVersionResource{
		Group:    KustomizationGroup,
		Version:  KustomizationVersion,
		Resource: KustomizationResource,
	}

	GitRepositoryGVR = schema.GroupVersionResource{
		Group:    GitRepositoryGroup,
		Version:  GitRepositoryVersion,
		Resource: GitRepositoryResource,
	}

	HelmRepositoryGVR = schema.GroupVersionResource{
		Group:    GitRepositoryGroup,
		Version:  GitRepositoryVersion,
		Resource: HelmRepositoryResource,
	}
)

// Adapter implements the DMS adapter interface for Flux deployments.
// It uses the Kubernetes dynamic client to manage Flux HelmRelease and
// Kustomization CRDs, avoiding direct Flux library dependencies.
type Adapter struct {
	Config        *Config           // Exported for testing
	DynamicClient dynamic.Interface // Exported for testing
	InitOnce      sync.Once         // Exported for testing
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
func NewAdapter(config *Config) (*Adapter, error) {
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

	return &Adapter{
		Config: config,
	}, nil
}

// checkContext checks if the context has been cancelled and returns an error if so.
// This should be called at the beginning of operations to fail fast on cancelled contexts.
func checkContext(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("context cancelled: %w", ctx.Err())
	default:
		return nil
	}
}

// ValidateName validates that a resource name conforms to DNS-1123 label format.
// Kubernetes resource names must be lowercase alphanumeric with hyphens, max 63 chars.
func ValidateName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: name cannot be empty", ErrInvalidName)
	}
	if len(name) > 63 {
		return fmt.Errorf("%w: name exceeds 63 characters", ErrInvalidName)
	}
	if !dns1123LabelRegex.MatchString(name) {
		return fmt.Errorf("%w: name must be lowercase alphanumeric with hyphens", ErrInvalidName)
	}
	return nil
}

// ValidatePath validates that a path does not contain directory traversal attempts.
// This prevents security issues with path manipulation.
func ValidatePath(path string) error {
	if path == "" {
		return nil // Empty path is allowed (defaults to "./")
	}
	// Check for directory traversal attempts
	if strings.Contains(path, "..") {
		return fmt.Errorf("%w: path cannot contain '..'", ErrInvalidPath)
	}
	// Check for absolute paths (security risk)
	if strings.HasPrefix(path, "/") {
		return fmt.Errorf("%w: absolute paths not allowed", ErrInvalidPath)
	}
	return nil
}

// Initialize performs lazy initialization of the Kubernetes dynamic client.
// This allows the adapter to be created without requiring immediate Kubernetes connectivity.
// This method is thread-safe and ensures initialization happens exactly once.
// The context is checked before starting initialization to fail fast on cancelled contexts.
func (f *Adapter) Initialize(ctx context.Context) error {
	f.InitOnce.Do(func() {
		// Check context inside Do() to prevent hanging on cancelled contexts
		if err := checkContext(ctx); err != nil {
			f.initError = fmt.Errorf("initialization cancelled: %w", err)
			return
		}

		var restConfig *rest.Config
		var err error

		if f.Config.Kubeconfig != "" {
			restConfig, err = clientcmd.BuildConfigFromFlags("", f.Config.Kubeconfig)
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

		f.DynamicClient, err = dynamic.NewForConfig(restConfig)
		if err != nil {
			f.initError = fmt.Errorf("failed to create dynamic client: %w", err)
			return
		}
	})

	return f.initError
}

// Name returns the adapter name.
func (f *Adapter) Name() string {
	return AdapterName
}

// Version returns the Flux version supported by this adapter.
func (f *Adapter) Version() string {
	return AdapterVersion
}

// Capabilities returns the capabilities supported by the Flux adapter.
func (f *Adapter) Capabilities() []adapter.Capability {
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
func (f *Adapter) ListDeploymentPackages(
	ctx context.Context, filter *adapter.Filter,
) ([]*adapter.DeploymentPackage, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	if err := f.Initialize(ctx); err != nil {
		return nil, err
	}

	// Fetch both lists first to enable preallocation
	gitRepos, err := f.listGitRepositories(ctx, filter)
	if err != nil {
		return nil, err
	}
	helmRepos, err := f.listHelmRepositories(ctx, filter)
	if err != nil {
		return nil, err
	}

	// Preallocate with combined capacity
	packages := make([]*adapter.DeploymentPackage, 0, len(gitRepos)+len(helmRepos))

	for _, repo := range gitRepos {
		packages = append(packages, f.transformGitRepoToPackage(repo))
	}
	for _, repo := range helmRepos {
		packages = append(packages, f.transformHelmRepoToPackage(repo))
	}

	return packages, nil
}

// GetDeploymentPackage retrieves a specific deployment package by ID.
// The ID format is "{type}-{sanitized-url}" (e.g., "git-https-github-com-example-repo").
func (f *Adapter) GetDeploymentPackage(ctx context.Context, id string) (*adapter.DeploymentPackage, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	if err := f.Initialize(ctx); err != nil {
		return nil, err
	}

	// Determine package type from ID prefix for more efficient lookup
	switch {
	case strings.HasPrefix(id, "git-"):
		return f.searchGitRepositories(ctx, id)
	case strings.HasPrefix(id, "helm-"):
		return f.searchHelmRepositories(ctx, id)
	default:
		return f.searchAllRepositories(ctx, id)
	}
}

// searchGitRepositories searches only Git repositories for the given package ID.
func (f *Adapter) searchGitRepositories(ctx context.Context, id string) (*adapter.DeploymentPackage, error) {
	gitRepos, err := f.listGitRepositories(ctx, nil)
	if err != nil {
		return nil, err
	}

	for _, repo := range gitRepos {
		pkg := f.transformGitRepoToPackage(repo)
		if pkg.ID == id {
			return pkg, nil
		}
	}

	return nil, fmt.Errorf("%w: %s", ErrPackageNotFound, id)
}

// searchHelmRepositories searches only Helm repositories for the given package ID.
func (f *Adapter) searchHelmRepositories(ctx context.Context, id string) (*adapter.DeploymentPackage, error) {
	helmRepos, err := f.listHelmRepositories(ctx, nil)
	if err != nil {
		return nil, err
	}

	for _, repo := range helmRepos {
		pkg := f.transformHelmRepoToPackage(repo)
		if pkg.ID == id {
			return pkg, nil
		}
	}

	return nil, fmt.Errorf("%w: %s", ErrPackageNotFound, id)
}

// searchAllRepositories searches both Git and Helm repositories for the given package ID.
func (f *Adapter) searchAllRepositories(ctx context.Context, id string) (*adapter.DeploymentPackage, error) {
	// Try Git repositories first
	if pkg, err := f.searchGitRepositories(ctx, id); err == nil {
		return pkg, nil
	}

	// Then try Helm repositories
	return f.searchHelmRepositories(ctx, id)
}

// UploadDeploymentPackage creates a reference to a Flux source.
// Flux uses Git and Helm repositories as package sources.
func (f *Adapter) UploadDeploymentPackage(
	ctx context.Context, pkg *adapter.DeploymentPackageUpload,
) (*adapter.DeploymentPackage, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
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
		ID:          GeneratePackageID(repoType, repoURL),
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
func (f *Adapter) DeleteDeploymentPackage(_ context.Context, _ string) error {
	return fmt.Errorf("%w: Flux does not support package deletion through this adapter "+
		"- manage source resources directly", ErrOperationNotSupported)
}

// ListDeployments retrieves all Flux deployments (HelmReleases and Kustomizations).
func (f *Adapter) ListDeployments(ctx context.Context, filter *adapter.Filter) ([]*adapter.Deployment, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	if err := f.Initialize(ctx); err != nil {
		return nil, err
	}

	helmReleases, kustomizations, err := f.fetchFluxResources(ctx, filter)
	if err != nil {
		return nil, err
	}

	deployments := f.transformAndFilterDeployments(helmReleases, kustomizations, filter)

	if filter != nil {
		deployments = f.ApplyPagination(deployments, filter.Limit, filter.Offset)
	}

	return deployments, nil
}

func (f *Adapter) fetchFluxResources(
	ctx context.Context, filter *adapter.Filter,
) ([]*unstructured.Unstructured, []*unstructured.Unstructured, error) {
	helmReleases, err := f.listHelmReleases(ctx, filter)
	if err != nil {
		return nil, nil, err
	}
	kustomizations, err := f.listKustomizations(ctx, filter)
	if err != nil {
		return nil, nil, err
	}
	return helmReleases, kustomizations, nil
}

func (f *Adapter) transformAndFilterDeployments(
	helmReleases, kustomizations []*unstructured.Unstructured,
	filter *adapter.Filter,
) []*adapter.Deployment {
	deployments := make([]*adapter.Deployment, 0, len(helmReleases)+len(kustomizations))

	for _, hr := range helmReleases {
		if deployment := f.TransformHelmReleaseToDeployment(hr); f.matchesStatusFilter(deployment, filter) {
			deployments = append(deployments, deployment)
		}
	}
	for _, ks := range kustomizations {
		if deployment := f.TransformKustomizationToDeployment(ks); f.matchesStatusFilter(deployment, filter) {
			deployments = append(deployments, deployment)
		}
	}

	return deployments
}

func (f *Adapter) matchesStatusFilter(deployment *adapter.Deployment, filter *adapter.Filter) bool {
	return filter == nil || filter.Status == "" || deployment.Status == filter.Status
}

// GetDeployment retrieves a specific Flux deployment by ID.
func (f *Adapter) GetDeployment(ctx context.Context, id string) (*adapter.Deployment, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	if err := f.Initialize(ctx); err != nil {
		return nil, err
	}

	// Try HelmRelease first
	hr, err := f.getHelmRelease(ctx, id)
	if err == nil {
		return f.TransformHelmReleaseToDeployment(hr), nil
	}

	// Try Kustomization
	ks, err := f.getKustomization(ctx, id)
	if err == nil {
		return f.TransformKustomizationToDeployment(ks), nil
	}

	return nil, fmt.Errorf("%w: %s", ErrDeploymentNotFound, id)
}

// CreateDeployment creates a new Flux HelmRelease or Kustomization.
func (f *Adapter) CreateDeployment(ctx context.Context, req *adapter.DeploymentRequest) (*adapter.Deployment, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	if err := f.Initialize(ctx); err != nil {
		return nil, err
	}

	if req == nil {
		return nil, fmt.Errorf("deployment request cannot be nil")
	}

	// Validate deployment name using DNS-1123 label format
	if err := ValidateName(req.Name); err != nil {
		return nil, err
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
func (f *Adapter) UpdateDeployment(
	ctx context.Context, id string, update *adapter.DeploymentUpdate,
) (*adapter.Deployment, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
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

	return nil, fmt.Errorf("%w: %s", ErrDeploymentNotFound, id)
}

// DeleteDeployment deletes a Flux deployment.
func (f *Adapter) DeleteDeployment(ctx context.Context, id string) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	if err := f.Initialize(ctx); err != nil {
		return err
	}

	propagation := metav1.DeletePropagationForeground

	// Try HelmRelease first
	err := f.DynamicClient.Resource(HelmReleaseGVR).Namespace(f.Config.Namespace).Delete(ctx, id, metav1.DeleteOptions{
		PropagationPolicy: &propagation,
	})
	if err == nil {
		return nil
	}

	// Try Kustomization
	err = f.DynamicClient.Resource(KustomizationGVR).Namespace(f.Config.Namespace).Delete(ctx, id, metav1.DeleteOptions{
		PropagationPolicy: &propagation,
	})
	if err == nil {
		return nil
	}

	return fmt.Errorf("%w: %s", ErrDeploymentNotFound, id)
}

// ScaleDeployment scales a deployment by updating the values.
// Flux doesn't directly support scaling, but we can update values.
func (f *Adapter) ScaleDeployment(ctx context.Context, id string, replicas int) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
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
func (f *Adapter) RollbackDeployment(ctx context.Context, id string, revision int) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	if err := f.Initialize(ctx); err != nil {
		return err
	}

	if revision < 0 {
		return fmt.Errorf("revision must be non-negative")
	}

	// Try HelmRelease first.
	hr, err := f.getHelmRelease(ctx, id)
	if err == nil {
		return f.rollbackHelmRelease(ctx, hr, revision)
	}

	// Try Kustomization.
	ks, err := f.getKustomization(ctx, id)
	if err == nil {
		return f.forceReconciliation(ctx, ks, KustomizationGVR)
	}

	return fmt.Errorf("%w: %s", ErrDeploymentNotFound, id)
}

// rollbackHelmRelease rolls back a HelmRelease to the specified revision.
func (f *Adapter) rollbackHelmRelease(ctx context.Context, hr *unstructured.Unstructured, revision int) error {
	// Get history to find the target revision.
	history, _, _ := unstructured.NestedSlice(hr.Object, "status", "history")
	if revision >= len(history) {
		return fmt.Errorf("revision %d not found in history", revision)
	}

	// Force reconciliation.
	if err := f.addReconciliationAnnotation(hr); err != nil {
		return err
	}

	_, err := f.DynamicClient.Resource(HelmReleaseGVR).
		Namespace(f.Config.Namespace).Update(ctx, hr, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update helm release: %w", err)
	}
	return nil
}

// forceReconciliation forces Flux to reconcile a resource.
func (f *Adapter) forceReconciliation(
	ctx context.Context, obj *unstructured.Unstructured, gvr schema.GroupVersionResource,
) error {
	if err := f.addReconciliationAnnotation(obj); err != nil {
		return err
	}

	_, err := f.DynamicClient.Resource(gvr).Namespace(f.Config.Namespace).Update(ctx, obj, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to force reconciliation: %w", err)
	}
	return nil
}

// addReconciliationAnnotation adds the Flux reconciliation annotation to an object.
func (f *Adapter) addReconciliationAnnotation(obj *unstructured.Unstructured) error {
	annotations, _, _ := unstructured.NestedStringMap(obj.Object, "metadata", "annotations")
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations["reconcile.fluxcd.io/requestedAt"] = time.Now().Format(time.RFC3339)
	if err := unstructured.SetNestedStringMap(obj.Object, annotations, "metadata", "annotations"); err != nil {
		return fmt.Errorf("failed to set reconciliation annotation: %w", err)
	}
	return nil
}

// GetDeploymentStatus retrieves detailed status for a Flux deployment.
func (f *Adapter) GetDeploymentStatus(ctx context.Context, id string) (*adapter.DeploymentStatusDetail, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
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

	return nil, fmt.Errorf("%w: %s", ErrDeploymentNotFound, id)
}

// GetDeploymentHistory retrieves the revision history for a Flux deployment.
func (f *Adapter) GetDeploymentHistory(ctx context.Context, id string) (*adapter.DeploymentHistory, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
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

	return nil, fmt.Errorf("%w: %s", ErrDeploymentNotFound, id)
}

// GetDeploymentLogs retrieves status information for a Flux deployment.
// Note: Flux doesn't provide direct log access, this returns status information.
func (f *Adapter) GetDeploymentLogs(ctx context.Context, id string, _ *adapter.LogOptions) ([]byte, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
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

	return nil, fmt.Errorf("%w: %s", ErrDeploymentNotFound, id)
}

// SupportsRollback returns true as Flux supports rollback via Git revisions.
func (f *Adapter) SupportsRollback() bool {
	return true
}

// SupportsScaling returns true as scaling can be done via value updates.
func (f *Adapter) SupportsScaling() bool {
	return true
}

// SupportsGitOps returns true as Flux is a GitOps tool.
func (f *Adapter) SupportsGitOps() bool {
	return true
}

// Health performs a health check on the Flux backend.
func (f *Adapter) Health(ctx context.Context) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	if err := f.Initialize(ctx); err != nil {
		return fmt.Errorf("flux adapter not healthy: %w", err)
	}

	// Try to list HelmReleases to verify connectivity
	_, err := f.DynamicClient.Resource(HelmReleaseGVR).Namespace(f.Config.Namespace).List(ctx, metav1.ListOptions{
		Limit: 1,
	})
	if err != nil {
		return fmt.Errorf("flux health check failed: %w", err)
	}

	return nil
}

// Close cleanly shuts down the adapter.
func (f *Adapter) Close() error {
	f.DynamicClient = nil
	return nil
}

// listHelmReleases retrieves Flux HelmReleases with optional filtering.
func (f *Adapter) listHelmReleases(ctx context.Context, filter *adapter.Filter) ([]*unstructured.Unstructured, error) {
	namespace := f.Config.Namespace
	if filter != nil && filter.Namespace != "" {
		namespace = filter.Namespace
	}

	opts := metav1.ListOptions{}
	if filter != nil && len(filter.Labels) > 0 {
		opts.LabelSelector = BuildLabelSelector(filter.Labels)
	}

	list, err := f.DynamicClient.Resource(HelmReleaseGVR).Namespace(namespace).List(ctx, opts)
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
func (f *Adapter) listKustomizations(
	ctx context.Context, filter *adapter.Filter,
) ([]*unstructured.Unstructured, error) {
	namespace := f.Config.Namespace
	if filter != nil && filter.Namespace != "" {
		namespace = filter.Namespace
	}

	opts := metav1.ListOptions{}
	if filter != nil && len(filter.Labels) > 0 {
		opts.LabelSelector = BuildLabelSelector(filter.Labels)
	}

	list, err := f.DynamicClient.Resource(KustomizationGVR).Namespace(namespace).List(ctx, opts)
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
func (f *Adapter) listGitRepositories(
	ctx context.Context, filter *adapter.Filter,
) ([]*unstructured.Unstructured, error) {
	namespace := f.Config.SourceNamespace
	if filter != nil && filter.Namespace != "" {
		namespace = filter.Namespace
	}

	opts := metav1.ListOptions{}
	if filter != nil && len(filter.Labels) > 0 {
		opts.LabelSelector = BuildLabelSelector(filter.Labels)
	}

	list, err := f.DynamicClient.Resource(GitRepositoryGVR).Namespace(namespace).List(ctx, opts)
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
func (f *Adapter) listHelmRepositories(
	ctx context.Context, filter *adapter.Filter,
) ([]*unstructured.Unstructured, error) {
	namespace := f.Config.SourceNamespace
	if filter != nil && filter.Namespace != "" {
		namespace = filter.Namespace
	}

	opts := metav1.ListOptions{}
	if filter != nil && len(filter.Labels) > 0 {
		opts.LabelSelector = BuildLabelSelector(filter.Labels)
	}

	list, err := f.DynamicClient.Resource(HelmRepositoryGVR).Namespace(namespace).List(ctx, opts)
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
func (f *Adapter) getHelmRelease(ctx context.Context, name string) (*unstructured.Unstructured, error) {
	resource, err := f.DynamicClient.Resource(HelmReleaseGVR).
		Namespace(f.Config.Namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get helm release: %w", err)
	}
	return resource, nil
}

// getKustomization retrieves a single Flux Kustomization by name.
func (f *Adapter) getKustomization(ctx context.Context, name string) (*unstructured.Unstructured, error) {
	resource, err := f.DynamicClient.Resource(KustomizationGVR).
		Namespace(f.Config.Namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get kustomization: %w", err)
	}
	return resource, nil
}

// createHelmRelease creates a new Flux HelmRelease.
func (f *Adapter) createHelmRelease(ctx context.Context, req *adapter.DeploymentRequest) (*adapter.Deployment, error) {
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
		targetNamespace = f.Config.TargetNamespace
	}

	hr := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": fmt.Sprintf("%s/%s", HelmReleaseGroup, HelmReleaseVersion),
			"kind":       "HelmRelease",
			"metadata": map[string]interface{}{
				"name":      req.Name,
				"namespace": f.Config.Namespace,
			},
			"spec": map[string]interface{}{
				"interval": f.Config.Interval.String(),
				"chart": map[string]interface{}{
					"spec": map[string]interface{}{
						"chart":   chart,
						"version": chartVersion,
						"sourceRef": map[string]interface{}{
							"kind":      sourceKind,
							"name":      sourceRef,
							"namespace": f.Config.SourceNamespace,
						},
					},
				},
				"targetNamespace": targetNamespace,
				"suspend":         f.Config.Suspend,
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

	result, err := f.DynamicClient.Resource(HelmReleaseGVR).
		Namespace(f.Config.Namespace).Create(ctx, hr, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create Flux HelmRelease: %w", err)
	}

	return f.TransformHelmReleaseToDeployment(result), nil
}

// createKustomization creates a new Flux Kustomization.
func (f *Adapter) createKustomization(
	ctx context.Context, req *adapter.DeploymentRequest,
) (*adapter.Deployment, error) {
	path, _ := req.Extensions["flux.path"].(string)
	sourceRef, _ := req.Extensions["flux.sourceRef"].(string)
	sourceKind, _ := req.Extensions["flux.sourceKind"].(string)

	if sourceRef == "" {
		return nil, fmt.Errorf("flux.sourceRef extension is required for Kustomization")
	}
	if sourceKind == "" {
		sourceKind = "GitRepository"
	}
	// Validate path to prevent directory traversal attacks
	if err := ValidatePath(path); err != nil {
		return nil, err
	}
	if path == "" {
		path = "./"
	}

	targetNamespace := req.Namespace
	if targetNamespace == "" {
		targetNamespace = f.Config.TargetNamespace
	}

	ks := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": fmt.Sprintf("%s/%s", KustomizationGroup, KustomizationVersion),
			"kind":       "Kustomization",
			"metadata": map[string]interface{}{
				"name":      req.Name,
				"namespace": f.Config.Namespace,
			},
			"spec": map[string]interface{}{
				"interval": f.Config.Interval.String(),
				"path":     path,
				"sourceRef": map[string]interface{}{
					"kind":      sourceKind,
					"name":      sourceRef,
					"namespace": f.Config.SourceNamespace,
				},
				"targetNamespace": targetNamespace,
				"prune":           f.Config.Prune,
				"force":           f.Config.Force,
				"suspend":         f.Config.Suspend,
			},
		},
	}

	result, err := f.DynamicClient.Resource(KustomizationGVR).
		Namespace(f.Config.Namespace).Create(ctx, ks, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create Flux Kustomization: %w", err)
	}

	return f.TransformKustomizationToDeployment(result), nil
}

// updateHelmRelease updates an existing Flux HelmRelease.
func (f *Adapter) updateHelmRelease(
	ctx context.Context, hr *unstructured.Unstructured, update *adapter.DeploymentUpdate,
) (*adapter.Deployment, error) {
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

		// Merge with new values, normalizing types for JSON compatibility
		for k, v := range update.Values {
			existingValues[k] = normalizeValueForJSON(v)
		}

		if err := unstructured.SetNestedField(hr.Object, existingValues, "spec", "values"); err != nil {
			return nil, fmt.Errorf("failed to update values: %w", err)
		}
	}

	result, err := f.DynamicClient.Resource(HelmReleaseGVR).
		Namespace(f.Config.Namespace).Update(ctx, hr, metav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to update Flux HelmRelease: %w", err)
	}

	return f.TransformHelmReleaseToDeployment(result), nil
}

// updateKustomization updates an existing Flux Kustomization.
func (f *Adapter) updateKustomization(
	ctx context.Context, ks *unstructured.Unstructured, update *adapter.DeploymentUpdate,
) (*adapter.Deployment, error) {
	// Update path if specified
	if path, ok := update.Extensions["flux.path"].(string); ok && path != "" {
		// Validate path to prevent directory traversal attacks
		if err := ValidatePath(path); err != nil {
			return nil, err
		}
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

	result, err := f.DynamicClient.Resource(KustomizationGVR).
		Namespace(f.Config.Namespace).Update(ctx, ks, metav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to update Flux Kustomization: %w", err)
	}

	return f.TransformKustomizationToDeployment(result), nil
}

// TransformHelmReleaseToDeployment converts a Flux HelmRelease to a Deployment.
func (f *Adapter) TransformHelmReleaseToDeployment(hr *unstructured.Unstructured) *adapter.Deployment {
	name, _, _ := unstructured.NestedString(hr.Object, "metadata", "name")
	namespace, _, _ := unstructured.NestedString(hr.Object, "metadata", "namespace")

	// Extract chart info
	chart, _, _ := unstructured.NestedString(hr.Object, "spec", "chart", "spec", "chart")
	chartVersion, _, _ := unstructured.NestedString(hr.Object, "spec", "chart", "spec", "version")
	sourceRef, _, _ := unstructured.NestedString(hr.Object, "spec", "chart", "spec", "sourceRef", "name")
	targetNamespace, _, _ := unstructured.NestedString(hr.Object, "spec", "targetNamespace")

	// Extract status
	conditions, _, _ := unstructured.NestedSlice(hr.Object, "status", "conditions")
	status, message := f.ExtractFluxStatus(conditions)

	// Get timestamps
	creationTimestamp := hr.GetCreationTimestamp().Time
	updatedAt := extractUpdatedAtFromConditions(conditions)
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
		PackageID:   GeneratePackageID("helm", fmt.Sprintf("%s/%s", sourceRef, chart)),
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

// TransformKustomizationToDeployment converts a Flux Kustomization to a Deployment.
func (f *Adapter) TransformKustomizationToDeployment(ks *unstructured.Unstructured) *adapter.Deployment {
	name, _, _ := unstructured.NestedString(ks.Object, "metadata", "name")
	namespace, _, _ := unstructured.NestedString(ks.Object, "metadata", "namespace")

	// Extract source info
	path, _, _ := unstructured.NestedString(ks.Object, "spec", "path")
	sourceRef, _, _ := unstructured.NestedString(ks.Object, "spec", "sourceRef", "name")
	targetNamespace, _, _ := unstructured.NestedString(ks.Object, "spec", "targetNamespace")

	// Extract status
	conditions, _, _ := unstructured.NestedSlice(ks.Object, "status", "conditions")
	status, message := f.ExtractFluxStatus(conditions)

	// Get last applied revision
	lastAppliedRevision, _, _ := unstructured.NestedString(ks.Object, "status", "lastAppliedRevision")

	// Get timestamps
	creationTimestamp := ks.GetCreationTimestamp().Time
	updatedAt := extractUpdatedAtFromConditions(conditions)
	if updatedAt.IsZero() {
		updatedAt = creationTimestamp
	}

	return &adapter.Deployment{
		ID:          name,
		Name:        name,
		PackageID:   GeneratePackageID("git", fmt.Sprintf("%s/%s", sourceRef, path)),
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
func (f *Adapter) transformGitRepoToPackage(repo *unstructured.Unstructured) *adapter.DeploymentPackage {
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
		ID:          GeneratePackageID("git", url),
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
func (f *Adapter) transformHelmRepoToPackage(repo *unstructured.Unstructured) *adapter.DeploymentPackage {
	name, _, _ := unstructured.NestedString(repo.Object, "metadata", "name")
	url, _, _ := unstructured.NestedString(repo.Object, "spec", "url")
	repoType, _, _ := unstructured.NestedString(repo.Object, "spec", "type")

	if repoType == "" {
		repoType = "default"
	}

	creationTimestamp := repo.GetCreationTimestamp().Time

	return &adapter.DeploymentPackage{
		ID:          GeneratePackageID("helm", url),
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
func (f *Adapter) transformHelmReleaseToStatus(hr *unstructured.Unstructured) *adapter.DeploymentStatusDetail {
	name, _, _ := unstructured.NestedString(hr.Object, "metadata", "name")

	conditions, _, _ := unstructured.NestedSlice(hr.Object, "status", "conditions")
	status, message := f.ExtractFluxStatus(conditions)
	dmsConditions, updatedAt := f.parseConditions(conditions)

	if updatedAt.IsZero() {
		updatedAt = time.Now()
	}

	return &adapter.DeploymentStatusDetail{
		DeploymentID: name,
		Status:       status,
		Message:      message,
		Progress:     f.CalculateProgress(status),
		Conditions:   dmsConditions,
		UpdatedAt:    updatedAt,
		Extensions: map[string]interface{}{
			"flux.type": "helmrelease",
		},
	}
}

// transformKustomizationToStatus converts a Flux Kustomization to detailed status.
func (f *Adapter) transformKustomizationToStatus(ks *unstructured.Unstructured) *adapter.DeploymentStatusDetail {
	name, _, _ := unstructured.NestedString(ks.Object, "metadata", "name")

	conditions, _, _ := unstructured.NestedSlice(ks.Object, "status", "conditions")
	status, message := f.ExtractFluxStatus(conditions)
	dmsConditions, updatedAt := f.parseConditions(conditions)

	lastAppliedRevision, _, _ := unstructured.NestedString(ks.Object, "status", "lastAppliedRevision")

	if updatedAt.IsZero() {
		updatedAt = time.Now()
	}

	return &adapter.DeploymentStatusDetail{
		DeploymentID: name,
		Status:       status,
		Message:      message,
		Progress:     f.CalculateProgress(status),
		Conditions:   dmsConditions,
		UpdatedAt:    updatedAt,
		Extensions: map[string]interface{}{
			"flux.type":                "kustomization",
			"flux.lastAppliedRevision": lastAppliedRevision,
		},
	}
}

// parseConditions converts Flux conditions to DMS conditions and returns the latest update time.
// This is a shared helper to avoid code duplication between HelmRelease and Kustomization status extraction.
func (f *Adapter) parseConditions(conditions []interface{}) ([]adapter.DeploymentCondition, time.Time) {
	dmsConditions := make([]adapter.DeploymentCondition, 0, len(conditions))
	var latestUpdate time.Time

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
				if transitionTime.After(latestUpdate) {
					latestUpdate = transitionTime
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

	return dmsConditions, latestUpdate
}

// ExtractFluxStatus extracts status and message from Flux conditions.
func (f *Adapter) ExtractFluxStatus(conditions []interface{}) (adapter.DeploymentStatus, string) {
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

// extractUpdatedAtFromConditions extracts the last transition time from conditions.
func extractUpdatedAtFromConditions(conditions []interface{}) time.Time {
	if len(conditions) == 0 {
		return time.Time{}
	}
	lastCond, ok := conditions[len(conditions)-1].(map[string]interface{})
	if !ok {
		return time.Time{}
	}
	lastTime, ok := lastCond["lastTransitionTime"].(string)
	if !ok {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339, lastTime)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

// extractHelmReleaseHistory extracts history from a HelmRelease.
func (f *Adapter) extractHelmReleaseHistory(id string, hr *unstructured.Unstructured) *adapter.DeploymentHistory {
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
func (f *Adapter) extractKustomizationHistory(id string, ks *unstructured.Unstructured) *adapter.DeploymentHistory {
	lastAppliedRevision, _, _ := unstructured.NestedString(ks.Object, "status", "lastAppliedRevision")

	conditions, _, _ := unstructured.NestedSlice(ks.Object, "status", "conditions")
	status, _ := f.ExtractFluxStatus(conditions)

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

// CalculateProgress estimates deployment progress based on status.
func (f *Adapter) CalculateProgress(status adapter.DeploymentStatus) int {
	switch status {
	case adapter.DeploymentStatusDeployed:
		return progressDeployed
	case adapter.DeploymentStatusDeploying:
		return progressDeploying
	case adapter.DeploymentStatusPending:
		return progressPending
	case adapter.DeploymentStatusFailed:
		return progressFailed
	case adapter.DeploymentStatusRollingBack:
		return progressDeploying // Treat rollback as in-progress
	case adapter.DeploymentStatusDeleting:
		return progressDeploying // Treat deletion as in-progress
	default:
		return progressFailed
	}
}

// ApplyPagination applies limit and offset to deployment list.
func (f *Adapter) ApplyPagination(deployments []*adapter.Deployment, limit, offset int) []*adapter.Deployment {
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

// GeneratePackageID creates a unique package ID from type and URL.
func GeneratePackageID(pkgType, url string) string {
	id := fmt.Sprintf("%s-%s", pkgType, url)
	id = strings.ReplaceAll(id, "://", "-")
	id = strings.ReplaceAll(id, "/", "-")
	id = strings.ReplaceAll(id, ".", "-")
	return id
}

// BuildLabelSelector creates a Kubernetes label selector string from a map.
func BuildLabelSelector(labels map[string]string) string {
	selectors := make([]string, 0, len(labels))
	for k, v := range labels {
		selectors = append(selectors, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(selectors, ",")
}

// normalizeValueForJSON converts values to JSON-compatible types for k8s.io/apimachinery.
// The DeepCopyJSONValue function in k8s.io/apimachinery v0.35.0+ only accepts:
// bool, string, float64, map[string]interface{}, []interface{}, and nil.
// This function recursively converts int types to float64.
func normalizeValueForJSON(v interface{}) interface{} {
	if numVal, ok := tryConvertToFloat64(v); ok {
		return numVal
	}

	switch val := v.(type) {
	case map[string]interface{}:
		return normalizeMap(val)
	case []interface{}:
		return normalizeSlice(val)
	default:
		// bool, string, float64, nil are already JSON-compatible
		return v
	}
}

func tryConvertToFloat64(v interface{}) (float64, bool) {
	rv := reflect.ValueOf(v)
	kind := rv.Kind()

	if isIntKind(kind) {
		return float64(rv.Int()), true
	}
	if isUintKind(kind) {
		return float64(rv.Uint()), true
	}
	if isFloatKind(kind) {
		return rv.Float(), true
	}
	return 0, false
}

func isIntKind(kind reflect.Kind) bool {
	return kind >= reflect.Int && kind <= reflect.Int64
}

func isUintKind(kind reflect.Kind) bool {
	return kind >= reflect.Uint && kind <= reflect.Uintptr
}

func isFloatKind(kind reflect.Kind) bool {
	return kind >= reflect.Float32 && kind <= reflect.Float64
}

func normalizeMap(val map[string]interface{}) map[string]interface{} {
	normalized := make(map[string]interface{}, len(val))
	for k, mv := range val {
		normalized[k] = normalizeValueForJSON(mv)
	}
	return normalized
}

func normalizeSlice(val []interface{}) []interface{} {
	normalized := make([]interface{}, len(val))
	for i, elem := range val {
		normalized[i] = normalizeValueForJSON(elem)
	}
	return normalized
}
