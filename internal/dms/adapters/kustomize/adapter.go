// Package kustomize provides an O2-DMS adapter implementation using Kustomize.
// This adapter enables CNF/VNF deployment management through Kustomize overlays
// applied to Kubernetes clusters using kubectl or the Kubernetes client.
package kustomize

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
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
	// AdapterName is the unique identifier for the Kustomize adapter.
	AdapterName = "kustomize"

	// AdapterVersion indicates the Kustomize version supported by this adapter.
	AdapterVersion = "5.0.0"

	// DefaultNamespace is the default namespace for Kustomize deployments.
	DefaultNamespace = "default"

	// DefaultTimeout is the default timeout for operations.
	DefaultTimeout = 10 * time.Minute

	// DefaultInterval is the default reconciliation interval.
	DefaultInterval = 5 * time.Minute
)

// Typed errors for the Kustomize adapter.
var (
	// ErrDeploymentNotFound is returned when a deployment is not found.
	ErrDeploymentNotFound = errors.New("deployment not found")

	// ErrPackageNotFound is returned when a package is not found.
	ErrPackageNotFound = errors.New("package not found")

	// ErrOperationNotSupported is returned when an operation is not supported.
	ErrOperationNotSupported = errors.New("operation not supported")

	// ErrInvalidName is returned when a name is invalid.
	ErrInvalidName = errors.New("invalid name")

	// ErrInvalidPath is returned when a path is invalid.
	ErrInvalidPath = errors.New("invalid path")
)

// GVR definitions for Kustomize ConfigMaps (used to track deployments).
var (
	configMapGVR = schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "configmaps",
	}
)

// Adapter implements the DMS adapter interface for Kustomize deployments.
type Adapter struct {
	Config        *Config           // Exported for testing
	DynamicClient dynamic.Interface // Exported for testing
	InitOnce      sync.Once         // Exported for testing
	initErr       error
}

// Config contains configuration for the Kustomize adapter.
type Config struct {
	// Kubeconfig is the path to the Kubernetes config file.
	Kubeconfig string

	// Namespace is the default Kubernetes namespace for deployments.
	Namespace string

	// Timeout is the default timeout for operations.
	Timeout time.Duration

	// BaseURL is the base URL for git repositories containing kustomize bases.
	BaseURL string

	// Prune enables pruning of resources not in the kustomization.
	Prune bool

	// Force enables force apply of resources.
	Force bool
}

// NewAdapter creates a new Kustomize adapter instance.
func NewAdapter(config *Config) (*Adapter, error) {
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

	return &Adapter{
		Config: config,
	}, nil
}

// initialize performs lazy initialization of the Kubernetes client.
func (k *Adapter) initialize() error {
	k.InitOnce.Do(func() {
		var cfg *rest.Config
		var err error

		if k.Config.Kubeconfig != "" {
			cfg, err = clientcmd.BuildConfigFromFlags("", k.Config.Kubeconfig)
		} else {
			cfg, err = rest.InClusterConfig()
		}
		if err != nil {
			k.initErr = fmt.Errorf("failed to create Kubernetes config: %w", err)
			return
		}

		k.DynamicClient, err = dynamic.NewForConfig(cfg)
		if err != nil {
			k.initErr = fmt.Errorf("failed to create dynamic client: %w", err)
			return
		}
	})

	return k.initErr
}

// Name returns the adapter name.
func (k *Adapter) Name() string {
	return AdapterName
}

// Version returns the Kustomize version supported by this adapter.
func (k *Adapter) Version() string {
	return AdapterVersion
}

// Capabilities returns the capabilities supported by the Kustomize adapter.
func (k *Adapter) Capabilities() []adapter.Capability {
	return []adapter.Capability{
		adapter.CapabilityDeploymentLifecycle,
		adapter.CapabilityHealthChecks,
	}
}

// ListDeploymentPackages retrieves all Kustomize bases/overlays.
func (k *Adapter) ListDeploymentPackages(
	ctx context.Context,
	_ *adapter.Filter,
) ([]*adapter.DeploymentPackage, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	if err := k.initialize(); err != nil {
		return nil, err
	}

	// Kustomize packages are Git repositories or local directories
	// Return configured base packages
	packages := []*adapter.DeploymentPackage{}

	if k.Config.BaseURL != "" {
		pkg := &adapter.DeploymentPackage{
			ID:          GeneratePackageID(k.Config.BaseURL),
			Name:        "kustomize-base",
			Version:     "latest",
			PackageType: "kustomize",
			Description: "Kustomize base configuration",
			UploadedAt:  time.Now(),
			Extensions: map[string]interface{}{
				"kustomize.url": k.Config.BaseURL,
			},
		}
		packages = append(packages, pkg)
	}

	return packages, nil
}

// GetDeploymentPackage retrieves a specific Kustomize package by ID.
func (k *Adapter) GetDeploymentPackage(
	ctx context.Context,
	id string,
) (*adapter.DeploymentPackage, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	if err := k.initialize(); err != nil {
		return nil, err
	}

	// Check if the ID matches our configured base
	if k.Config.BaseURL != "" && id == GeneratePackageID(k.Config.BaseURL) {
		return &adapter.DeploymentPackage{
			ID:          id,
			Name:        "kustomize-base",
			Version:     "latest",
			PackageType: "kustomize",
			Description: "Kustomize base configuration",
			UploadedAt:  time.Now(),
			Extensions: map[string]interface{}{
				"kustomize.url": k.Config.BaseURL,
			},
		}, nil
	}

	return nil, fmt.Errorf("package %s: %w", id, ErrPackageNotFound)
}

// UploadDeploymentPackage registers a new Kustomize package reference.
func (k *Adapter) UploadDeploymentPackage(
	ctx context.Context,
	pkg *adapter.DeploymentPackageUpload,
) (*adapter.DeploymentPackage, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	if pkg == nil {
		return nil, fmt.Errorf("package cannot be nil")
	}

	url, ok := pkg.Extensions["kustomize.url"].(string)
	if !ok || url == "" {
		return nil, fmt.Errorf("kustomize.url extension is required")
	}

	return &adapter.DeploymentPackage{
		ID:          GeneratePackageID(url),
		Name:        pkg.Name,
		Version:     pkg.Version,
		PackageType: "kustomize",
		Description: pkg.Description,
		UploadedAt:  time.Now(),
		Extensions: map[string]interface{}{
			"kustomize.url": url,
		},
	}, nil
}

// DeleteDeploymentPackage is not supported for Kustomize.
func (k *Adapter) DeleteDeploymentPackage(
	ctx context.Context,
	_ string,
) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	return fmt.Errorf("kustomize adapter %w: package deletion requires Git repository access", ErrOperationNotSupported)
}

// ListDeployments retrieves all Kustomize deployments.
func (k *Adapter) ListDeployments(
	ctx context.Context,
	filter *adapter.Filter,
) ([]*adapter.Deployment, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	if err := k.initialize(); err != nil {
		return nil, err
	}

	cms, err := k.listConfigMaps(ctx, filter)
	if err != nil {
		return nil, err
	}

	deployments := k.filterAndTransformConfigMaps(cms.Items, filter)

	if filter != nil {
		deployments = k.ApplyPagination(deployments, filter.Limit, filter.Offset)
	}

	return deployments, nil
}

func (k *Adapter) listConfigMaps(ctx context.Context, filter *adapter.Filter) (*unstructured.UnstructuredList, error) {
	namespace := k.Config.Namespace
	if filter != nil && filter.Namespace != "" {
		namespace = filter.Namespace
	}

	cms, err := k.DynamicClient.Resource(configMapGVR).Namespace(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/managed-by=kustomize-adapter",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments: %w", err)
	}
	return cms, nil
}

func (k *Adapter) filterAndTransformConfigMaps(
	items []unstructured.Unstructured, filter *adapter.Filter,
) []*adapter.Deployment {
	deployments := make([]*adapter.Deployment, 0, len(items))
	for i := range items {
		deployment := k.transformConfigMapToDeployment(&items[i])

		if filter != nil && filter.Status != "" && deployment.Status != filter.Status {
			continue
		}

		deployments = append(deployments, deployment)
	}
	return deployments
}

// GetDeployment retrieves a specific Kustomize deployment by ID.
func (k *Adapter) GetDeployment(
	ctx context.Context,
	id string,
) (*adapter.Deployment, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	if err := k.initialize(); err != nil {
		return nil, err
	}

	cm, err := k.DynamicClient.Resource(configMapGVR).Namespace(k.Config.Namespace).Get(
		ctx,
		fmt.Sprintf("kustomize-%s", id),
		metav1.GetOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("deployment %s: %w", id, ErrDeploymentNotFound)
	}

	return k.transformConfigMapToDeployment(cm), nil
}

// CreateDeployment creates a new Kustomize deployment.
func (k *Adapter) CreateDeployment(
	ctx context.Context,
	req *adapter.DeploymentRequest,
) (*adapter.Deployment, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	if err := k.validateCreateRequest(req); err != nil {
		return nil, err
	}

	if err := k.initialize(); err != nil {
		return nil, err
	}

	path, err := k.extractAndValidatePath(req.Extensions)
	if err != nil {
		return nil, err
	}

	namespace := k.getNamespaceOrDefault(req.Namespace)
	cm := k.buildConfigMapForDeployment(req, namespace, path)

	created, err := k.DynamicClient.Resource(configMapGVR).Namespace(namespace).Create(
		ctx,
		cm,
		metav1.CreateOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create deployment: %w", err)
	}

	return k.transformConfigMapToDeployment(created), nil
}

func (k *Adapter) validateCreateRequest(req *adapter.DeploymentRequest) error {
	if req == nil {
		return fmt.Errorf("deployment request cannot be nil")
	}
	if req.Name == "" {
		return fmt.Errorf("name cannot be empty: %w", ErrInvalidName)
	}
	return ValidateName(req.Name)
}

func (k *Adapter) extractAndValidatePath(extensions map[string]interface{}) (string, error) {
	path := "./"
	if extensions != nil {
		if p, ok := extensions["kustomize.path"].(string); ok {
			path = p
		}
	}
	if err := ValidatePath(path); err != nil {
		return "", err
	}
	return path, nil
}

func (k *Adapter) getNamespaceOrDefault(namespace string) string {
	if namespace == "" {
		return k.Config.Namespace
	}
	return namespace
}

func (k *Adapter) buildConfigMapForDeployment(
	req *adapter.DeploymentRequest, namespace, path string,
) *unstructured.Unstructured {
	now := time.Now()
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      fmt.Sprintf("kustomize-%s", req.Name),
				"namespace": namespace,
				"labels": map[string]interface{}{
					"app.kubernetes.io/managed-by": "kustomize-adapter",
					"app.kubernetes.io/name":       req.Name,
				},
			},
			"data": map[string]interface{}{
				"name":        req.Name,
				"packageId":   req.PackageID,
				"path":        path,
				"status":      string(adapter.DeploymentStatusDeployed),
				"version":     "1",
				"createdAt":   now.Format(time.RFC3339),
				"updatedAt":   now.Format(time.RFC3339),
				"description": req.Description,
			},
		},
	}
}

// UpdateDeployment updates an existing Kustomize deployment.
func (k *Adapter) UpdateDeployment(
	ctx context.Context,
	id string,
	update *adapter.DeploymentUpdate,
) (*adapter.Deployment, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	if update == nil {
		return nil, fmt.Errorf("update cannot be nil")
	}

	if err := k.initialize(); err != nil {
		return nil, err
	}

	cm, err := k.getConfigMapForUpdate(ctx, id)
	if err != nil {
		return nil, err
	}

	if err := k.applyPathUpdate(cm, update.Extensions); err != nil {
		return nil, err
	}

	if err := k.applyMetadataUpdates(cm, update); err != nil {
		return nil, err
	}

	updated, err := k.DynamicClient.Resource(configMapGVR).Namespace(k.Config.Namespace).Update(
		ctx,
		cm,
		metav1.UpdateOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to update deployment: %w", err)
	}

	return k.transformConfigMapToDeployment(updated), nil
}

func (k *Adapter) getConfigMapForUpdate(ctx context.Context, id string) (*unstructured.Unstructured, error) {
	cm, err := k.DynamicClient.Resource(configMapGVR).Namespace(k.Config.Namespace).Get(
		ctx,
		fmt.Sprintf("kustomize-%s", id),
		metav1.GetOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("deployment %s: %w", id, ErrDeploymentNotFound)
	}
	return cm, nil
}

func (k *Adapter) applyPathUpdate(cm *unstructured.Unstructured, extensions map[string]interface{}) error {
	if extensions == nil {
		return nil
	}

	path, ok := extensions["kustomize.path"].(string)
	if !ok {
		return nil
	}

	if err := ValidatePath(path); err != nil {
		return err
	}

	data, _, _ := unstructured.NestedStringMap(cm.Object, "data")
	data["path"] = path
	if err := unstructured.SetNestedStringMap(cm.Object, data, "data"); err != nil {
		return fmt.Errorf("failed to update path: %w", err)
	}
	return nil
}

func (k *Adapter) applyMetadataUpdates(cm *unstructured.Unstructured, update *adapter.DeploymentUpdate) error {
	data, _, _ := unstructured.NestedStringMap(cm.Object, "data")

	version := k.parseVersion(data["version"])
	data["version"] = strconv.Itoa(version + 1)
	data["updatedAt"] = time.Now().Format(time.RFC3339)

	if update.Description != "" {
		data["description"] = update.Description
	}

	if err := unstructured.SetNestedStringMap(cm.Object, data, "data"); err != nil {
		return fmt.Errorf("failed to update data: %w", err)
	}
	return nil
}

func (k *Adapter) parseVersion(versionStr string) int {
	if versionStr == "" {
		return 1
	}
	if parsed, err := strconv.Atoi(versionStr); err == nil {
		return parsed
	}
	return 1
}

// DeleteDeployment deletes a Kustomize deployment.
func (k *Adapter) DeleteDeployment(
	ctx context.Context,
	id string,
) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	if err := k.initialize(); err != nil {
		return err
	}

	err := k.DynamicClient.Resource(configMapGVR).Namespace(k.Config.Namespace).Delete(
		ctx,
		fmt.Sprintf("kustomize-%s", id),
		metav1.DeleteOptions{},
	)
	if err != nil {
		return fmt.Errorf("deployment %s: %w", id, ErrDeploymentNotFound)
	}

	return nil
}

// ScaleDeployment is not directly supported by Kustomize.
// Scaling must be done through Kustomize patches.
func (k *Adapter) ScaleDeployment(
	ctx context.Context,
	_ string,
	replicas int,
) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	if replicas < 0 {
		return fmt.Errorf("replicas must be non-negative")
	}

	return fmt.Errorf("kustomize adapter %w: scaling must be done through kustomize patches", ErrOperationNotSupported)
}

// RollbackDeployment is not directly supported by Kustomize.
// Rollback must be done through Git or source control.
func (k *Adapter) RollbackDeployment(
	ctx context.Context,
	_ string,
	revision int,
) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	if revision < 0 {
		return fmt.Errorf("revision must be non-negative")
	}

	return fmt.Errorf("kustomize adapter %w: rollback must be done through source control", ErrOperationNotSupported)
}

// GetDeploymentStatus retrieves detailed status for a deployment.
func (k *Adapter) GetDeploymentStatus(
	ctx context.Context,
	id string,
) (*adapter.DeploymentStatusDetail, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	if err := k.initialize(); err != nil {
		return nil, err
	}

	deployment, err := k.GetDeployment(ctx, id)
	if err != nil {
		return nil, err
	}

	return &adapter.DeploymentStatusDetail{
		DeploymentID: deployment.ID,
		Status:       deployment.Status,
		Message:      deployment.Description,
		Progress:     k.CalculateProgress(deployment.Status),
		UpdatedAt:    deployment.UpdatedAt,
		Conditions: []adapter.DeploymentCondition{
			{
				Type:               "Ready",
				Status:             k.conditionStatus(deployment.Status),
				Reason:             k.conditionReason(deployment.Status),
				Message:            deployment.Description,
				LastTransitionTime: deployment.UpdatedAt,
			},
		},
		Extensions: deployment.Extensions,
	}, nil
}

// GetDeploymentHistory retrieves the revision history for a deployment.
func (k *Adapter) GetDeploymentHistory(
	ctx context.Context,
	id string,
) (*adapter.DeploymentHistory, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	if err := k.initialize(); err != nil {
		return nil, err
	}

	deployment, err := k.GetDeployment(ctx, id)
	if err != nil {
		return nil, err
	}

	// Kustomize doesn't maintain history in the same way
	// Return current version as the only revision
	return &adapter.DeploymentHistory{
		DeploymentID: id,
		Revisions: []adapter.DeploymentRevision{
			{
				Revision:    deployment.Version,
				Version:     "latest",
				DeployedAt:  deployment.UpdatedAt,
				Status:      deployment.Status,
				Description: deployment.Description,
			},
		},
	}, nil
}

// GetDeploymentLogs retrieves logs for a deployment.
func (k *Adapter) GetDeploymentLogs(
	ctx context.Context,
	id string,
	_ *adapter.LogOptions,
) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	if err := k.initialize(); err != nil {
		return nil, err
	}

	deployment, err := k.GetDeployment(ctx, id)
	if err != nil {
		return nil, err
	}

	// Return deployment information as JSON
	info := map[string]interface{}{
		"deploymentId": deployment.ID,
		"name":         deployment.Name,
		"status":       deployment.Status,
		"version":      deployment.Version,
		"updatedAt":    deployment.UpdatedAt,
	}

	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal deployment logs: %w", err)
	}
	return data, nil
}

// SupportsRollback returns false as Kustomize doesn't support direct rollback.
func (k *Adapter) SupportsRollback() bool {
	return false
}

// SupportsScaling returns false as Kustomize doesn't support direct scaling.
func (k *Adapter) SupportsScaling() bool {
	return false
}

// SupportsGitOps returns true as Kustomize is typically used with GitOps.
func (k *Adapter) SupportsGitOps() bool {
	return true
}

// Health performs a health check on the Kubernetes cluster.
func (k *Adapter) Health(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	if err := k.initialize(); err != nil {
		return fmt.Errorf("kustomize adapter not healthy: %w", err)
	}

	// Try to list namespaces to verify connectivity
	_, err := k.DynamicClient.Resource(schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "namespaces",
	}).List(ctx, metav1.ListOptions{Limit: 1})

	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}

	return nil
}

// Close cleanly shuts down the adapter.
func (k *Adapter) Close() error {
	k.DynamicClient = nil
	return nil
}

// Helper functions

func (k *Adapter) transformConfigMapToDeployment(
	cm *unstructured.Unstructured,
) *adapter.Deployment {
	data, _, _ := unstructured.NestedStringMap(cm.Object, "data")

	name := data["name"]
	if name == "" {
		name = cm.GetName()
	}

	status := adapter.DeploymentStatus(data["status"])
	if status == "" {
		status = adapter.DeploymentStatusDeployed
	}

	version := 1
	if v, ok := data["version"]; ok && v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			version = parsed
		}
		// If parsing fails, continue with default version 1
	}

	createdAt := cm.GetCreationTimestamp().Time
	if t, err := time.Parse(time.RFC3339, data["createdAt"]); err == nil {
		createdAt = t
	}

	updatedAt := createdAt
	if t, err := time.Parse(time.RFC3339, data["updatedAt"]); err == nil {
		updatedAt = t
	}

	return &adapter.Deployment{
		ID:          name,
		Name:        name,
		PackageID:   data["packageId"],
		Namespace:   cm.GetNamespace(),
		Status:      status,
		Version:     version,
		Description: data["description"],
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
		Extensions: map[string]interface{}{
			"kustomize.path": data["path"],
		},
	}
}

// ApplyPagination applies pagination to a list of deployments.
func (k *Adapter) ApplyPagination(
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

// CalculateProgress calculates deployment progress percentage based on status.
func (k *Adapter) CalculateProgress(status adapter.DeploymentStatus) int {
	switch status {
	case adapter.DeploymentStatusDeployed:
		return 100
	case adapter.DeploymentStatusDeploying:
		return 50
	case adapter.DeploymentStatusPending:
		return 25
	case adapter.DeploymentStatusRollingBack:
		return 30
	case adapter.DeploymentStatusDeleting:
		return 10
	case adapter.DeploymentStatusFailed:
		return 0
	default:
		return 0
	}
}

func (k *Adapter) conditionStatus(status adapter.DeploymentStatus) string {
	if status == adapter.DeploymentStatusDeployed {
		return "True"
	}
	return "False"
}

func (k *Adapter) conditionReason(status adapter.DeploymentStatus) string {
	switch status {
	case adapter.DeploymentStatusDeployed:
		return "ReconciliationSucceeded"
	case adapter.DeploymentStatusDeploying:
		return "Progressing"
	case adapter.DeploymentStatusPending:
		return "Pending"
	case adapter.DeploymentStatusRollingBack:
		return "RollingBack"
	case adapter.DeploymentStatusDeleting:
		return "Deleting"
	case adapter.DeploymentStatusFailed:
		return "ReconciliationFailed"
	default:
		return "Unknown"
	}
}

// GeneratePackageID generates a unique package ID from repository URL and path.
func GeneratePackageID(url string) string {
	// Sanitize URL to create a valid ID
	id := strings.ReplaceAll(url, "://", "-")
	id = strings.ReplaceAll(id, "/", "-")
	id = strings.ReplaceAll(id, ".", "-")
	return "kustomize-" + id
}

// ValidateName validates the deployment name.
func ValidateName(name string) error {
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

// ValidatePath validates the kustomize path.
func ValidatePath(path string) error {
	if path == "" {
		return nil
	}

	// Prevent path traversal attacks
	if strings.Contains(path, "..") {
		return fmt.Errorf("path cannot contain '..': %w", ErrInvalidPath)
	}

	// Prevent absolute paths
	if strings.HasPrefix(path, "/") {
		return fmt.Errorf("absolute paths not allowed: %w", ErrInvalidPath)
	}

	return nil
}
