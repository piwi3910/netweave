// Package mock provides a mock O2-DMS adapter with realistic deployment simulation.
// This adapter is designed for:
// - Local development and testing without real Helm/ArgoCD infrastructure
// - E2E testing in CI pipelines
// - API demonstrations and documentation
//
// The mock adapter stores all data in memory and simulates realistic deployment
// lifecycle behavior including status progression, health checks, and rollback.
package mock

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/piwi3910/netweave/internal/dms/adapter"
)

// Adapter is a mock implementation of the DMS adapter interface.
// It stores all data in memory and provides realistic deployment lifecycle simulation.
type Adapter struct {
	mu          sync.RWMutex
	packages    map[string]*adapter.DeploymentPackage
	deployments map[string]*adapter.Deployment
	history     map[string]*adapter.DeploymentHistory
}

// NewAdapter creates a new mock DMS adapter with sample data.
// Pass populateSampleData=true to pre-populate with realistic test packages.
func NewAdapter(populateSampleData bool) *Adapter {
	a := &Adapter{
		packages:    make(map[string]*adapter.DeploymentPackage),
		deployments: make(map[string]*adapter.Deployment),
		history:     make(map[string]*adapter.DeploymentHistory),
	}

	if populateSampleData {
		a.populateSampleData()
	}

	return a
}

// populateSampleData adds realistic sample deployment packages for testing.
func (a *Adapter) populateSampleData() {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()

	// Sample packages for various NF types
	packages := []*adapter.DeploymentPackage{
		{
			ID:          "pkg-cuup-001",
			Name:        "oran-cuup",
			Version:     "1.2.0",
			PackageType: "helm-chart",
			Description: "O-RAN CU-UP (Centralized Unit User Plane) network function",
			UploadedAt:  now.Add(-72 * time.Hour),
			Extensions: map[string]interface{}{
				"repository":  "https://charts.oran.org",
				"chart_name":  "oran-cuup",
				"app_version": "2.0.0",
				"category":    "ran",
			},
		},
		{
			ID:          "pkg-cucp-001",
			Name:        "oran-cucp",
			Version:     "1.1.5",
			PackageType: "helm-chart",
			Description: "O-RAN CU-CP (Centralized Unit Control Plane) network function",
			UploadedAt:  now.Add(-48 * time.Hour),
			Extensions: map[string]interface{}{
				"repository":  "https://charts.oran.org",
				"chart_name":  "oran-cucp",
				"app_version": "1.9.2",
				"category":    "ran",
			},
		},
		{
			ID:          "pkg-du-001",
			Name:        "oran-du",
			Version:     "2.0.1",
			PackageType: "helm-chart",
			Description: "O-RAN Distributed Unit network function",
			UploadedAt:  now.Add(-24 * time.Hour),
			Extensions: map[string]interface{}{
				"repository":  "https://charts.oran.org",
				"chart_name":  "oran-du",
				"app_version": "3.1.0",
				"category":    "ran",
			},
		},
		{
			ID:          "pkg-upf-001",
			Name:        "5g-upf",
			Version:     "1.5.2",
			PackageType: "helm-chart",
			Description: "5G User Plane Function",
			UploadedAt:  now.Add(-96 * time.Hour),
			Extensions: map[string]interface{}{
				"repository":  "https://charts.5gcore.org",
				"chart_name":  "upf",
				"app_version": "1.5.2",
				"category":    "core",
			},
		},
		{
			ID:          "pkg-smf-001",
			Name:        "5g-smf",
			Version:     "2.1.0",
			PackageType: "helm-chart",
			Description: "5G Session Management Function",
			UploadedAt:  now.Add(-120 * time.Hour),
			Extensions: map[string]interface{}{
				"repository":  "https://charts.5gcore.org",
				"chart_name":  "smf",
				"app_version": "2.1.0",
				"category":    "core",
			},
		},
	}

	for _, pkg := range packages {
		a.packages[pkg.ID] = pkg
	}
}

// DMSAdapterMetadata implementation

// Name returns the adapter name.
func (a *Adapter) Name() string {
	return "mock"
}

// Version returns the adapter version.
func (a *Adapter) Version() string {
	return "1.0.0"
}

// Capabilities returns the list of supported DMS capabilities.
func (a *Adapter) Capabilities() []adapter.Capability {
	return []adapter.Capability{
		adapter.CapabilityPackageManagement,
		adapter.CapabilityDeploymentLifecycle,
		adapter.CapabilityRollback,
		adapter.CapabilityScaling,
		adapter.CapabilityHealthChecks,
		adapter.CapabilityMetrics,
	}
}

// PackageManager implementation

// ListDeploymentPackages retrieves all deployment packages matching the filter.
func (a *Adapter) ListDeploymentPackages(_ context.Context, filter *adapter.Filter) ([]*adapter.DeploymentPackage, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var packages []*adapter.DeploymentPackage
	for _, pkg := range a.packages {
		packages = append(packages, pkg)
	}

	return a.applyPaginationPackages(packages, filter), nil
}

// GetDeploymentPackage retrieves a specific deployment package by ID.
func (a *Adapter) GetDeploymentPackage(_ context.Context, id string) (*adapter.DeploymentPackage, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	pkg, ok := a.packages[id]
	if !ok {
		return nil, adapter.ErrPackageNotFound
	}

	return pkg, nil
}

// UploadDeploymentPackage uploads a new deployment package.
func (a *Adapter) UploadDeploymentPackage(_ context.Context, upload *adapter.DeploymentPackageUpload) (*adapter.DeploymentPackage, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	pkg := &adapter.DeploymentPackage{
		ID:          fmt.Sprintf("pkg-%s", uuid.New().String()[:8]),
		Name:        upload.Name,
		Version:     upload.Version,
		PackageType: upload.PackageType,
		Description: upload.Description,
		UploadedAt:  time.Now(),
		Extensions:  upload.Extensions,
	}

	a.packages[pkg.ID] = pkg
	return pkg, nil
}

// DeleteDeploymentPackage deletes a deployment package by ID.
func (a *Adapter) DeleteDeploymentPackage(_ context.Context, id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, ok := a.packages[id]; !ok {
		return adapter.ErrPackageNotFound
	}

	// Check if package is in use
	for _, deployment := range a.deployments {
		if deployment.PackageID == id {
			return fmt.Errorf("cannot delete package in use by deployment: %s", deployment.ID)
		}
	}

	delete(a.packages, id)
	return nil
}

// DeploymentLifecycleManager implementation

// ListDeployments retrieves all deployments matching the filter.
func (a *Adapter) ListDeployments(_ context.Context, filter *adapter.Filter) ([]*adapter.Deployment, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var deployments []*adapter.Deployment
	for _, dep := range a.deployments {
		if a.matchesFilter(dep, filter) {
			deployments = append(deployments, dep)
		}
	}

	return a.applyPaginationDeployments(deployments, filter), nil
}

// GetDeployment retrieves a specific deployment by ID.
func (a *Adapter) GetDeployment(_ context.Context, id string) (*adapter.Deployment, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	dep, ok := a.deployments[id]
	if !ok {
		return nil, adapter.ErrDeploymentNotFound
	}

	return dep, nil
}

// CreateDeployment creates a new deployment.
func (a *Adapter) CreateDeployment(_ context.Context, req *adapter.DeploymentRequest) (*adapter.Deployment, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Verify package exists
	if _, ok := a.packages[req.PackageID]; !ok {
		return nil, adapter.ErrPackageNotFound
	}

	now := time.Now()
	deployment := &adapter.Deployment{
		ID:          fmt.Sprintf("dep-%s", uuid.New().String()[:8]),
		Name:        req.Name,
		PackageID:   req.PackageID,
		Namespace:   req.Namespace,
		Status:      adapter.DeploymentStatusPending,
		Version:     1,
		Description: req.Description,
		CreatedAt:   now,
		UpdatedAt:   now,
		Extensions:  req.Extensions,
	}

	a.deployments[deployment.ID] = deployment

	// Simulate async deployment progression
	go a.simulateDeployment(deployment.ID)

	return deployment, nil
}

// UpdateDeployment updates an existing deployment.
func (a *Adapter) UpdateDeployment(_ context.Context, id string, update *adapter.DeploymentUpdate) (*adapter.Deployment, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	dep, ok := a.deployments[id]
	if !ok {
		return nil, adapter.ErrDeploymentNotFound
	}

	// Update deployment
	dep.Version++
	dep.UpdatedAt = time.Now()
	dep.Description = update.Description
	if update.Extensions != nil {
		dep.Extensions = update.Extensions
	}

	// Simulate async update
	dep.Status = adapter.DeploymentStatusDeploying
	go a.simulateDeployment(id)

	return dep, nil
}

// DeleteDeployment deletes a deployment by ID.
func (a *Adapter) DeleteDeployment(_ context.Context, id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	dep, ok := a.deployments[id]
	if !ok {
		return adapter.ErrDeploymentNotFound
	}

	// Simulate async deletion
	dep.Status = adapter.DeploymentStatusDeleting
	go a.simulateDelete(id)

	return nil
}

// DeploymentOperations implementation

// GetDeploymentStatus retrieves detailed status for a deployment.
func (a *Adapter) GetDeploymentStatus(_ context.Context, id string) (*adapter.DeploymentStatusDetail, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	dep, ok := a.deployments[id]
	if !ok {
		return nil, adapter.ErrDeploymentNotFound
	}

	progress := 100
	switch dep.Status {
	case adapter.DeploymentStatusDeploying:
		progress = 50
	case adapter.DeploymentStatusPending:
		progress = 0
	case adapter.DeploymentStatusDeployed:
		progress = 100
	case adapter.DeploymentStatusFailed:
		progress = 100
	case adapter.DeploymentStatusRollingBack:
		progress = 75
	case adapter.DeploymentStatusDeleting:
		progress = 50
	}

	status := &adapter.DeploymentStatusDetail{
		DeploymentID: dep.ID,
		Status:       dep.Status,
		Message:      fmt.Sprintf("Deployment is %s", dep.Status),
		Progress:     progress,
		UpdatedAt:    dep.UpdatedAt,
		Conditions: []adapter.DeploymentCondition{
			{
				Type: "Ready",
				Status: func() string {
					if dep.Status == adapter.DeploymentStatusDeployed {
						return "True"
					}
					return "False"
				}(),
				Reason:             "DeploymentComplete",
				Message:            "All pods are ready",
				LastTransitionTime: dep.UpdatedAt,
			},
		},
		Extensions: dep.Extensions,
	}

	return status, nil
}

// GetDeploymentLogs retrieves logs for a deployment.
func (a *Adapter) GetDeploymentLogs(_ context.Context, id string, opts *adapter.LogOptions) ([]byte, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if _, ok := a.deployments[id]; !ok {
		return nil, adapter.ErrDeploymentNotFound
	}

	// Return mock logs as bytes
	logs := fmt.Sprintf("[MOCK] Deployment logs for %s\nStatus: Running\nPods: 3/3 ready\n", id)
	return []byte(logs), nil
}

// RollbackDeployment rolls back a deployment to a previous version.
func (a *Adapter) RollbackDeployment(_ context.Context, id string, targetVersion int) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	dep, ok := a.deployments[id]
	if !ok {
		return adapter.ErrDeploymentNotFound
	}

	dep.Status = adapter.DeploymentStatusRollingBack
	dep.UpdatedAt = time.Now()

	// Simulate async rollback
	go a.simulateRollback(id, targetVersion)

	return nil
}

// ScaleDeployment scales a deployment to the specified replica count.
func (a *Adapter) ScaleDeployment(_ context.Context, id string, replicas int) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	dep, ok := a.deployments[id]
	if !ok {
		return adapter.ErrDeploymentNotFound
	}

	if dep.Extensions == nil {
		dep.Extensions = make(map[string]interface{})
	}
	dep.Extensions["replicas"] = replicas
	dep.UpdatedAt = time.Now()

	return nil
}

// GetDeploymentHistory retrieves the revision history for a deployment.
func (a *Adapter) GetDeploymentHistory(_ context.Context, id string) (*adapter.DeploymentHistory, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	dep, ok := a.deployments[id]
	if !ok {
		return nil, adapter.ErrDeploymentNotFound
	}

	history := &adapter.DeploymentHistory{
		DeploymentID: dep.ID,
		Revisions: []adapter.DeploymentRevision{
			{
				Revision:    dep.Version,
				Version:     fmt.Sprintf("v%d", dep.Version),
				DeployedAt:  dep.UpdatedAt,
				Status:      dep.Status,
				Description: fmt.Sprintf("Revision %d", dep.Version),
			},
		},
	}

	return history, nil
}

// DMSCapabilities implementation

// SupportsCapability checks if the adapter supports a specific capability.
func (a *Adapter) SupportsCapability(capability adapter.Capability) bool {
	for _, cap := range a.Capabilities() {
		if cap == capability {
			return true
		}
	}
	return false
}

// DMSAdapterLifecycle implementation

// Initialize performs any necessary initialization.
func (a *Adapter) Initialize(_ context.Context) error {
	// Mock adapter requires no initialization
	return nil
}

// Health checks the health of the adapter.
func (a *Adapter) Health(_ context.Context) error {
	// Mock adapter is always healthy
	return nil
}

// Shutdown performs cleanup when the adapter is shutting down.
func (a *Adapter) Shutdown(_ context.Context) error {
	// Mock adapter requires no cleanup
	return nil
}

// Helper methods

func (a *Adapter) matchesFilter(dep *adapter.Deployment, filter *adapter.Filter) bool {
	if filter == nil {
		return true
	}

	if filter.Namespace != "" && dep.Namespace != filter.Namespace {
		return false
	}

	if filter.Status != "" && dep.Status != filter.Status {
		return false
	}

	return true
}

func (a *Adapter) applyPaginationPackages(packages []*adapter.DeploymentPackage, filter *adapter.Filter) []*adapter.DeploymentPackage {
	if filter == nil {
		return packages
	}

	offset := filter.Offset
	limit := filter.Limit

	if limit == 0 {
		limit = len(packages)
	}

	if offset >= len(packages) {
		return []*adapter.DeploymentPackage{}
	}

	end := offset + limit
	if end > len(packages) {
		end = len(packages)
	}

	return packages[offset:end]
}

func (a *Adapter) applyPaginationDeployments(deployments []*adapter.Deployment, filter *adapter.Filter) []*adapter.Deployment {
	if filter == nil {
		return deployments
	}

	offset := filter.Offset
	limit := filter.Limit

	if limit == 0 {
		limit = len(deployments)
	}

	if offset >= len(deployments) {
		return []*adapter.Deployment{}
	}

	end := offset + limit
	if end > len(deployments) {
		end = len(deployments)
	}

	return deployments[offset:end]
}

// Simulation methods for realistic async behavior

func (a *Adapter) simulateDeployment(id string) {
	// Wait a bit, then mark as deployed
	time.Sleep(2 * time.Second)

	a.mu.Lock()
	defer a.mu.Unlock()

	if dep, ok := a.deployments[id]; ok {
		dep.Status = adapter.DeploymentStatusDeployed
		dep.UpdatedAt = time.Now()
	}
}

func (a *Adapter) simulateDelete(id string) {
	// Wait a bit, then remove
	time.Sleep(1 * time.Second)

	a.mu.Lock()
	defer a.mu.Unlock()

	delete(a.deployments, id)
}

func (a *Adapter) simulateRollback(id string, targetVersion int) {
	// Wait a bit, then mark as deployed
	time.Sleep(2 * time.Second)

	a.mu.Lock()
	defer a.mu.Unlock()

	if dep, ok := a.deployments[id]; ok {
		dep.Status = adapter.DeploymentStatusDeployed
		dep.Version = targetVersion
		dep.UpdatedAt = time.Now()
	}
}

// SupportsRollback indicates if the adapter supports rollback operations.
// Mock adapter returns true as it supports rollback simulation.
func (a *Adapter) SupportsRollback() bool {
	return true
}

// SupportsScaling indicates if the adapter supports scaling operations.
// Mock adapter returns true as it supports scaling simulation.
func (a *Adapter) SupportsScaling() bool {
	return true
}

// SupportsGitOps indicates if the adapter supports GitOps workflows.
// Mock adapter returns false as it doesn't support GitOps.
func (a *Adapter) SupportsGitOps() bool {
	return false
}

// Close cleanly shuts down the adapter and releases resources.
// For the mock adapter, this is a no-op since there are no external connections.
func (a *Adapter) Close() error {
	// Mock adapter has no resources to clean up
	return nil
}
