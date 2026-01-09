// Package crossplane provides an O2-DMS adapter implementation using Crossplane.
// This adapter enables CNF/VNF deployment management through Crossplane
// Compositions and Claims for Kubernetes-native infrastructure provisioning.
package crossplane

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

const (
	// AdapterName is the unique identifier for the Crossplane adapter.
	AdapterName = "crossplane"

	// AdapterVersion indicates the Crossplane version supported by this adapter.
	AdapterVersion = "1.14.0"

	// DefaultNamespace is the default namespace for Crossplane claims.
	DefaultNamespace = "default"

	// DefaultTimeout is the default timeout for operations.
	DefaultTimeout = 10 * time.Minute

	// CrossplaneGroup is the Crossplane API group.
	CrossplaneGroup = "pkg.crossplane.io"

	// CrossplaneVersion is the Crossplane API version.
	CrossplaneVersion = "v1"

	// CompositeResourceClaimsKind is the kind for Composite Resource Claims.
	CompositeResourceClaimsKind = "composite-resource-claims"
)

// GVR definitions for Crossplane resources.
var (
	// compositionsGVR is the GVR for Crossplane Compositions.
	compositionsGVR = schema.GroupVersionResource{
		Group:    "apiextensions.crossplane.io",
		Version:  "v1",
		Resource: "compositions",
	}

	// configurationGVR is the GVR for Crossplane Configurations.
	configurationGVR = schema.GroupVersionResource{
		Group:    CrossplaneGroup,
		Version:  CrossplaneVersion,
		Resource: "configurations",
	}

	// providerGVR is the GVR for Crossplane Providers.
	providerGVR = schema.GroupVersionResource{
		Group:    CrossplaneGroup,
		Version:  CrossplaneVersion,
		Resource: "providers",
	}
)

// Typed errors for the Crossplane adapter.
var (
	// ErrDeploymentNotFound is returned when a deployment is not found.
	ErrDeploymentNotFound = errors.New("deployment not found")

	// ErrPackageNotFound is returned when a package is not found.
	ErrPackageNotFound = errors.New("package not found")

	// ErrOperationNotSupported is returned when an operation is not supported.
	ErrOperationNotSupported = errors.New("operation not supported")

	// ErrInvalidName is returned when a name is invalid.
	ErrInvalidName = errors.New("invalid name")

	// ErrMissingCompositionRef is returned when composition reference is missing.
	ErrMissingCompositionRef = errors.New("composition reference is required")
)

// CrossplaneAdapter implements the DMS adapter interface for Crossplane deployments.
type CrossplaneAdapter struct {
	config        *Config
	dynamicClient dynamic.Interface
	initOnce      sync.Once
	initErr       error
}

// Config contains configuration for the Crossplane adapter.
type Config struct {
	// Kubeconfig is the path to the Kubernetes config file.
	Kubeconfig string

	// Namespace is the default Kubernetes namespace for claims.
	Namespace string

	// Timeout is the default timeout for operations.
	Timeout time.Duration

	// DefaultCompositionRef is the default composition to use for deployments.
	DefaultCompositionRef string

	// ProviderConfig is the name of the default provider config.
	ProviderConfig string
}

// NewAdapter creates a new Crossplane adapter instance.
func NewAdapter(config *Config) (*CrossplaneAdapter, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Apply defaults
	if config.Namespace == "" {
		config.Namespace = DefaultNamespace
	}
	if config.Timeout == 0 {
		config.Timeout = DefaultTimeout
	}

	return &CrossplaneAdapter{
		config: config,
	}, nil
}

// initialize performs lazy initialization of the Kubernetes client.
func (c *CrossplaneAdapter) initialize() error {
	c.initOnce.Do(func() {
		var cfg *rest.Config
		var err error

		if c.config.Kubeconfig != "" {
			cfg, err = clientcmd.BuildConfigFromFlags("", c.config.Kubeconfig)
		} else {
			cfg, err = rest.InClusterConfig()
		}
		if err != nil {
			c.initErr = fmt.Errorf("failed to create Kubernetes config: %w", err)
			return
		}

		c.dynamicClient, err = dynamic.NewForConfig(cfg)
		if err != nil {
			c.initErr = fmt.Errorf("failed to create dynamic client: %w", err)
			return
		}
	})

	return c.initErr
}

// Name returns the adapter name.
func (c *CrossplaneAdapter) Name() string {
	return AdapterName
}

// Version returns the Crossplane version supported by this adapter.
func (c *CrossplaneAdapter) Version() string {
	return AdapterVersion
}

// Capabilities returns the capabilities supported by the Crossplane adapter.
func (c *CrossplaneAdapter) Capabilities() []adapter.Capability {
	return []adapter.Capability{
		adapter.CapabilityPackageManagement,
		adapter.CapabilityDeploymentLifecycle,
		adapter.CapabilityHealthChecks,
	}
}

// ListDeploymentPackages retrieves all Crossplane Compositions.
func (c *CrossplaneAdapter) ListDeploymentPackages(
	ctx context.Context,
	filter *adapter.Filter,
) ([]*adapter.DeploymentPackage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if err := c.initialize(); err != nil {
		return nil, err
	}

	compositions, err := c.dynamicClient.Resource(compositionsGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list compositions: %w", err)
	}

	packages := make([]*adapter.DeploymentPackage, 0, len(compositions.Items))
	for i := range compositions.Items {
		pkg := c.transformCompositionToPackage(&compositions.Items[i])
		packages = append(packages, pkg)
	}

	// Apply pagination
	if filter != nil && (filter.Limit > 0 || filter.Offset > 0) {
		packages = c.applyPackagePagination(packages, filter.Limit, filter.Offset)
	}

	return packages, nil
}

// GetDeploymentPackage retrieves a specific Crossplane Composition by ID.
func (c *CrossplaneAdapter) GetDeploymentPackage(
	ctx context.Context,
	id string,
) (*adapter.DeploymentPackage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if err := c.initialize(); err != nil {
		return nil, err
	}

	composition, err := c.dynamicClient.Resource(compositionsGVR).Get(ctx, id, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("package %s: %w", id, ErrPackageNotFound)
	}

	return c.transformCompositionToPackage(composition), nil
}

// UploadDeploymentPackage creates a new Crossplane Composition reference.
func (c *CrossplaneAdapter) UploadDeploymentPackage(
	ctx context.Context,
	pkg *adapter.DeploymentPackageUpload,
) (*adapter.DeploymentPackage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if pkg == nil {
		return nil, fmt.Errorf("package cannot be nil")
	}

	compositionRef, ok := pkg.Extensions["crossplane.compositionRef"].(string)
	if !ok || compositionRef == "" {
		return nil, fmt.Errorf("%w: crossplane.compositionRef extension is required", ErrMissingCompositionRef)
	}

	return &adapter.DeploymentPackage{
		ID:          compositionRef,
		Name:        pkg.Name,
		Version:     pkg.Version,
		PackageType: "crossplane-composition",
		Description: pkg.Description,
		UploadedAt:  time.Now(),
		Extensions: map[string]interface{}{
			"crossplane.compositionRef": compositionRef,
		},
	}, nil
}

// DeleteDeploymentPackage is not supported for Crossplane.
func (c *CrossplaneAdapter) DeleteDeploymentPackage(
	ctx context.Context,
	_ string,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	return fmt.Errorf("crossplane adapter %w: composition deletion must be done through GitOps", ErrOperationNotSupported)
}

// ListDeployments retrieves all Crossplane Claims.
func (c *CrossplaneAdapter) ListDeployments(
	ctx context.Context,
	filter *adapter.Filter,
) ([]*adapter.Deployment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if err := c.initialize(); err != nil {
		return nil, err
	}

	// List all Configurations as deployments
	configs, err := c.dynamicClient.Resource(configurationGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list configurations: %w", err)
	}

	deployments := make([]*adapter.Deployment, 0, len(configs.Items))
	for i := range configs.Items {
		deployment := c.transformConfigurationToDeployment(&configs.Items[i])

		// Apply status filter
		if filter != nil && filter.Status != "" && deployment.Status != filter.Status {
			continue
		}

		deployments = append(deployments, deployment)
	}

	// Apply pagination
	if filter != nil {
		deployments = c.applyPagination(deployments, filter.Limit, filter.Offset)
	}

	return deployments, nil
}

// GetDeployment retrieves a specific Crossplane Configuration by ID.
func (c *CrossplaneAdapter) GetDeployment(
	ctx context.Context,
	id string,
) (*adapter.Deployment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if err := c.initialize(); err != nil {
		return nil, err
	}

	config, err := c.dynamicClient.Resource(configurationGVR).Get(ctx, id, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("deployment %s: %w", id, ErrDeploymentNotFound)
	}

	return c.transformConfigurationToDeployment(config), nil
}

// CreateDeployment creates a new Crossplane Configuration.
func (c *CrossplaneAdapter) CreateDeployment(
	ctx context.Context,
	req *adapter.DeploymentRequest,
) (*adapter.Deployment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if req == nil {
		return nil, fmt.Errorf("deployment request cannot be nil")
	}
	if req.Name == "" {
		return nil, fmt.Errorf("name cannot be empty: %w", ErrInvalidName)
	}
	if err := validateName(req.Name); err != nil {
		return nil, err
	}

	if err := c.initialize(); err != nil {
		return nil, err
	}

	// Get package reference from extensions or request
	packageRef := req.PackageID
	if req.Extensions != nil {
		if ref, ok := req.Extensions["crossplane.package"].(string); ok && ref != "" {
			packageRef = ref
		}
	}
	if packageRef == "" {
		return nil, fmt.Errorf("package reference is required (packageId or crossplane.package extension)")
	}

	// Create Configuration resource
	config := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": fmt.Sprintf("%s/%s", CrossplaneGroup, CrossplaneVersion),
			"kind":       "Configuration",
			"metadata": map[string]interface{}{
				"name": req.Name,
				"labels": map[string]interface{}{
					"app.kubernetes.io/managed-by": "crossplane-adapter",
					"app.kubernetes.io/name":       req.Name,
				},
			},
			"spec": map[string]interface{}{
				"package": packageRef,
			},
		},
	}

	// Add revision activation policy if specified
	if req.Extensions != nil {
		if policy, ok := req.Extensions["crossplane.revisionActivationPolicy"].(string); ok {
			if err := unstructured.SetNestedField(config.Object, policy, "spec", "revisionActivationPolicy"); err != nil {
				return nil, fmt.Errorf("failed to set revision activation policy: %w", err)
			}
		}
	}

	created, err := c.dynamicClient.Resource(configurationGVR).Create(ctx, config, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create configuration: %w", err)
	}

	return c.transformConfigurationToDeployment(created), nil
}

// UpdateDeployment updates an existing Crossplane Configuration.
func (c *CrossplaneAdapter) UpdateDeployment(
	ctx context.Context,
	id string,
	update *adapter.DeploymentUpdate,
) (*adapter.Deployment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if update == nil {
		return nil, fmt.Errorf("update cannot be nil")
	}

	if err := c.initialize(); err != nil {
		return nil, err
	}

	// Get existing configuration
	config, err := c.dynamicClient.Resource(configurationGVR).Get(ctx, id, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("deployment %s: %w", id, ErrDeploymentNotFound)
	}

	// Update package reference if provided
	if update.Extensions != nil {
		if packageRef, ok := update.Extensions["crossplane.package"].(string); ok && packageRef != "" {
			if err := unstructured.SetNestedField(config.Object, packageRef, "spec", "package"); err != nil {
				return nil, fmt.Errorf("failed to update package reference: %w", err)
			}
		}

		if policy, ok := update.Extensions["crossplane.revisionActivationPolicy"].(string); ok {
			if err := unstructured.SetNestedField(config.Object, policy, "spec", "revisionActivationPolicy"); err != nil {
				return nil, fmt.Errorf("failed to update revision activation policy: %w", err)
			}
		}
	}

	updated, err := c.dynamicClient.Resource(configurationGVR).Update(ctx, config, metav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to update configuration: %w", err)
	}

	return c.transformConfigurationToDeployment(updated), nil
}

// DeleteDeployment deletes a Crossplane Configuration.
func (c *CrossplaneAdapter) DeleteDeployment(
	ctx context.Context,
	id string,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := c.initialize(); err != nil {
		return err
	}

	err := c.dynamicClient.Resource(configurationGVR).Delete(ctx, id, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("deployment %s: %w", id, ErrDeploymentNotFound)
	}

	return nil
}

// ScaleDeployment is not directly supported by Crossplane.
func (c *CrossplaneAdapter) ScaleDeployment(
	ctx context.Context,
	_ string,
	replicas int,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if replicas < 0 {
		return fmt.Errorf("replicas must be non-negative")
	}

	return fmt.Errorf("crossplane adapter %w: scaling must be done through composition updates", ErrOperationNotSupported)
}

// RollbackDeployment is not directly supported by Crossplane.
func (c *CrossplaneAdapter) RollbackDeployment(
	ctx context.Context,
	_ string,
	revision int,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if revision < 0 {
		return fmt.Errorf("revision must be non-negative")
	}

	return fmt.Errorf("crossplane adapter %w: rollback must be done through package version changes", ErrOperationNotSupported)
}

// GetDeploymentStatus retrieves detailed status for a deployment.
func (c *CrossplaneAdapter) GetDeploymentStatus(
	ctx context.Context,
	id string,
) (*adapter.DeploymentStatusDetail, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if err := c.initialize(); err != nil {
		return nil, err
	}

	deployment, err := c.GetDeployment(ctx, id)
	if err != nil {
		return nil, err
	}

	conditions := c.extractConditions(deployment.Extensions)

	return &adapter.DeploymentStatusDetail{
		DeploymentID: deployment.ID,
		Status:       deployment.Status,
		Message:      deployment.Description,
		Progress:     c.calculateProgress(deployment.Status),
		UpdatedAt:    deployment.UpdatedAt,
		Conditions:   conditions,
		Extensions:   deployment.Extensions,
	}, nil
}

// GetDeploymentHistory retrieves the revision history for a deployment.
func (c *CrossplaneAdapter) GetDeploymentHistory(
	ctx context.Context,
	id string,
) (*adapter.DeploymentHistory, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if err := c.initialize(); err != nil {
		return nil, err
	}

	deployment, err := c.GetDeployment(ctx, id)
	if err != nil {
		return nil, err
	}

	// Crossplane maintains revision through package revisions
	return &adapter.DeploymentHistory{
		DeploymentID: id,
		Revisions: []adapter.DeploymentRevision{
			{
				Revision:    deployment.Version,
				Version:     fmt.Sprintf("%d", deployment.Version),
				DeployedAt:  deployment.UpdatedAt,
				Status:      deployment.Status,
				Description: deployment.Description,
			},
		},
	}, nil
}

// GetDeploymentLogs retrieves logs for a deployment.
func (c *CrossplaneAdapter) GetDeploymentLogs(
	ctx context.Context,
	id string,
	_ *adapter.LogOptions,
) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if err := c.initialize(); err != nil {
		return nil, err
	}

	deployment, err := c.GetDeployment(ctx, id)
	if err != nil {
		return nil, err
	}

	// Return deployment status as JSON
	info := map[string]interface{}{
		"deploymentId": deployment.ID,
		"name":         deployment.Name,
		"status":       deployment.Status,
		"version":      deployment.Version,
		"updatedAt":    deployment.UpdatedAt,
		"extensions":   deployment.Extensions,
	}

	return json.MarshalIndent(info, "", "  ")
}

// SupportsRollback returns false as Crossplane doesn't support direct rollback.
func (c *CrossplaneAdapter) SupportsRollback() bool {
	return false
}

// SupportsScaling returns false as Crossplane doesn't support direct scaling.
func (c *CrossplaneAdapter) SupportsScaling() bool {
	return false
}

// SupportsGitOps returns true as Crossplane is typically used with GitOps.
func (c *CrossplaneAdapter) SupportsGitOps() bool {
	return true
}

// Health performs a health check on the Crossplane backend.
func (c *CrossplaneAdapter) Health(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := c.initialize(); err != nil {
		return fmt.Errorf("crossplane adapter not healthy: %w", err)
	}

	// Try to list providers to verify Crossplane is installed
	_, err := c.dynamicClient.Resource(providerGVR).List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}

	return nil
}

// Close cleanly shuts down the adapter.
func (c *CrossplaneAdapter) Close() error {
	c.dynamicClient = nil
	return nil
}

// Helper functions

func (c *CrossplaneAdapter) transformCompositionToPackage(
	composition *unstructured.Unstructured,
) *adapter.DeploymentPackage {
	name := composition.GetName()

	// Extract composite type reference
	compositeTypeRef, _, _ := unstructured.NestedString(composition.Object, "spec", "compositeTypeRef", "kind")
	apiVersion, _, _ := unstructured.NestedString(composition.Object, "spec", "compositeTypeRef", "apiVersion")

	return &adapter.DeploymentPackage{
		ID:          name,
		Name:        name,
		Version:     composition.GetResourceVersion(),
		PackageType: "crossplane-composition",
		Description: fmt.Sprintf("Crossplane Composition for %s", compositeTypeRef),
		UploadedAt:  composition.GetCreationTimestamp().Time,
		Extensions: map[string]interface{}{
			"crossplane.compositeTypeRef.kind":       compositeTypeRef,
			"crossplane.compositeTypeRef.apiVersion": apiVersion,
		},
	}
}

func (c *CrossplaneAdapter) transformConfigurationToDeployment(
	config *unstructured.Unstructured,
) *adapter.Deployment {
	name := config.GetName()

	// Extract package reference
	packageRef, _, _ := unstructured.NestedString(config.Object, "spec", "package")

	// Extract status conditions
	conditions, _, _ := unstructured.NestedSlice(config.Object, "status", "conditions")

	status := c.extractStatus(conditions)
	version := 1

	// Try to get current revision
	if rev, found, _ := unstructured.NestedInt64(config.Object, "status", "currentRevision"); found {
		version = int(rev)
	}

	return &adapter.Deployment{
		ID:          name,
		Name:        name,
		PackageID:   packageRef,
		Namespace:   config.GetNamespace(),
		Status:      status,
		Version:     version,
		Description: fmt.Sprintf("Crossplane Configuration: %s", packageRef),
		CreatedAt:   config.GetCreationTimestamp().Time,
		UpdatedAt:   config.GetCreationTimestamp().Time,
		Extensions: map[string]interface{}{
			"crossplane.package":    packageRef,
			"crossplane.conditions": conditions,
		},
	}
}

func (c *CrossplaneAdapter) extractStatus(conditions []interface{}) adapter.DeploymentStatus {
	for _, cond := range conditions {
		condMap, ok := cond.(map[string]interface{})
		if !ok {
			continue
		}

		condType, _ := condMap["type"].(string)
		condStatus, _ := condMap["status"].(string)

		if condType == "Healthy" && condStatus == "True" {
			return adapter.DeploymentStatusDeployed
		}
		if condType == "Installed" && condStatus == "True" {
			return adapter.DeploymentStatusDeployed
		}
		if condType == "Healthy" && condStatus == "False" {
			return adapter.DeploymentStatusFailed
		}
	}

	return adapter.DeploymentStatusDeploying
}

func (c *CrossplaneAdapter) extractConditions(extensions map[string]interface{}) []adapter.DeploymentCondition {
	var result []adapter.DeploymentCondition

	if conditions, ok := extensions["crossplane.conditions"].([]interface{}); ok {
		for _, cond := range conditions {
			condMap, ok := cond.(map[string]interface{})
			if !ok {
				continue
			}

			condType, _ := condMap["type"].(string)
			condStatus, _ := condMap["status"].(string)
			reason, _ := condMap["reason"].(string)
			message, _ := condMap["message"].(string)

			result = append(result, adapter.DeploymentCondition{
				Type:               condType,
				Status:             condStatus,
				Reason:             reason,
				Message:            message,
				LastTransitionTime: time.Now(),
			})
		}
	}

	if len(result) == 0 {
		result = append(result, adapter.DeploymentCondition{
			Type:               "Ready",
			Status:             "Unknown",
			Reason:             "Unknown",
			Message:            "No conditions available",
			LastTransitionTime: time.Now(),
		})
	}

	return result
}

func (c *CrossplaneAdapter) calculateProgress(status adapter.DeploymentStatus) int {
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

func (c *CrossplaneAdapter) applyPagination(
	deployments []*adapter.Deployment,
	limit, offset int,
) []*adapter.Deployment {
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

func (c *CrossplaneAdapter) applyPackagePagination(
	packages []*adapter.DeploymentPackage,
	limit, offset int,
) []*adapter.DeploymentPackage {
	if offset >= len(packages) {
		return []*adapter.DeploymentPackage{}
	}

	start := offset
	end := len(packages)

	if limit > 0 && start+limit < end {
		end = start + limit
	}

	return packages[start:end]
}

// validateName validates the deployment name.
func validateName(name string) error {
	if name == "" {
		return fmt.Errorf("name cannot be empty: %w", ErrInvalidName)
	}
	if len(name) > 63 {
		return fmt.Errorf("name too long (max 63 chars): %w", ErrInvalidName)
	}

	// DNS-1123 label validation
	pattern := regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	if !pattern.MatchString(name) {
		return fmt.Errorf("name must be DNS-1123 compliant: %w", ErrInvalidName)
	}

	return nil
}

// buildLabelSelector builds a label selector string from a map.
func buildLabelSelector(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}

	parts := make([]string, 0, len(labels))
	for k, v := range labels {
		parts = append(parts, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(parts, ",")
}
