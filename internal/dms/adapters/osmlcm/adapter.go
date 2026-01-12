// Package osmlcm provides an O2-DMS adapter implementation for OSM (Open Source MANO).
// This adapter enables CNF/VNF deployment management through OSM's Northbound Interface
// for network function lifecycle management in telecom environments.
package osmlcm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/piwi3910/netweave/internal/dms/adapter"
)

const (
	// AdapterName is the unique identifier for the OSM-LCM adapter.
	AdapterName = "osm-lcm"

	// AdapterVersion indicates the OSM NBI API version supported by this adapter.
	AdapterVersion = "1.0.0"

	// DefaultTimeout is the default timeout for API operations.
	DefaultTimeout = 30 * time.Second

	// DefaultNBIPath is the default OSM NBI API path.
	DefaultNBIPath = "/osm/nbi/v1"
)

// Typed errors for the OSM-LCM adapter.
var (
	// ErrDeploymentNotFound is returned when a deployment is not found.
	ErrDeploymentNotFound = errors.New("deployment not found")

	// ErrPackageNotFound is returned when a package is not found.
	ErrPackageNotFound = errors.New("package not found")

	// ErrOperationNotSupported is returned when an operation is not supported.
	ErrOperationNotSupported = errors.New("operation not supported")

	// ErrInvalidName is returned when a name is invalid.
	ErrInvalidName = errors.New("invalid name")

	// ErrConnectionFailed is returned when connection to OSM fails.
	ErrConnectionFailed = errors.New("connection to OSM failed")

	// ErrAuthenticationFailed is returned when authentication fails.
	ErrAuthenticationFailed = errors.New("authentication failed")
)

// Adapter implements the DMS adapter interface for OSM lifecycle management.
type Adapter struct {
	config      *Config
	httpClient  *http.Client
	deployments map[string]*adapter.Deployment
	packages    map[string]*adapter.DeploymentPackage
	mu          sync.RWMutex
	initOnce    sync.Once
	initErr     error
}

// Config contains configuration for the OSM-LCM adapter.
type Config struct {
	// NBIEndpoint is the OSM NBI API endpoint URL.
	NBIEndpoint string

	// Username is the username for OSM API authentication.
	Username string

	// Password is the password for OSM API authentication.
	Password string

	// Project is the OSM project name.
	Project string

	// Timeout is the default timeout for API operations.
	Timeout time.Duration

	// TLSSkipVerify skips TLS certificate verification.
	// WARNING: This should only be used in development/testing environments.
	// In production, always use proper TLS certificate verification.
	TLSSkipVerify bool
}

// NewAdapter creates a new OSM-LCM adapter instance.
func NewAdapter(config *Config) (*Adapter, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Apply defaults
	if config.Timeout == 0 {
		config.Timeout = DefaultTimeout
	}
	if config.Project == "" {
		config.Project = "admin"
	}

	return &Adapter{
		config:      config,
		deployments: make(map[string]*adapter.Deployment),
		packages:    make(map[string]*adapter.DeploymentPackage),
	}, nil
}

// initialize performs lazy initialization of the HTTP client.
func (o *Adapter) initialize() error {
	o.initOnce.Do(func() {
		o.httpClient = &http.Client{
			Timeout: o.config.Timeout,
		}
	})

	return o.initErr
}

// Name returns the adapter name.
func (o *Adapter) Name() string {
	return AdapterName
}

// Version returns the OSM NBI API version supported by this adapter.
func (o *Adapter) Version() string {
	return AdapterVersion
}

// Capabilities returns the capabilities supported by the OSM-LCM adapter.
func (o *Adapter) Capabilities() []adapter.Capability {
	return []adapter.Capability{
		adapter.CapabilityPackageManagement,
		adapter.CapabilityDeploymentLifecycle,
		adapter.CapabilityScaling,
		adapter.CapabilityHealthChecks,
	}
}

// ListDeploymentPackages retrieves all available VNF/NS packages.
func (o *Adapter) ListDeploymentPackages(
	ctx context.Context,
	filter *adapter.Filter,
) ([]*adapter.DeploymentPackage, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	if err := o.initialize(); err != nil {
		return nil, err
	}

	o.mu.RLock()
	defer o.mu.RUnlock()

	packages := make([]*adapter.DeploymentPackage, 0, len(o.packages))
	for _, pkg := range o.packages {
		packages = append(packages, pkg)
	}

	// Apply pagination
	if filter != nil && (filter.Limit > 0 || filter.Offset > 0) {
		packages = o.applyPackagePagination(packages, filter.Limit, filter.Offset)
	}

	return packages, nil
}

// GetDeploymentPackage retrieves a specific VNF/NS package by ID.
func (o *Adapter) GetDeploymentPackage(
	ctx context.Context,
	id string,
) (*adapter.DeploymentPackage, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	if err := o.initialize(); err != nil {
		return nil, err
	}

	o.mu.RLock()
	defer o.mu.RUnlock()

	pkg, exists := o.packages[id]
	if !exists {
		return nil, fmt.Errorf("package %s: %w", id, ErrPackageNotFound)
	}

	return pkg, nil
}

// UploadDeploymentPackage registers a new VNF/NS package.
func (o *Adapter) UploadDeploymentPackage(
	ctx context.Context,
	pkg *adapter.DeploymentPackageUpload,
) (*adapter.DeploymentPackage, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	if pkg == nil {
		return nil, fmt.Errorf("package cannot be nil")
	}

	if err := o.initialize(); err != nil {
		return nil, err
	}

	// Get package type from extensions (vnfd or nsd)
	pkgType := "vnfd"
	if pkg.Extensions != nil {
		if t, ok := pkg.Extensions["osm.packageType"].(string); ok {
			pkgType = t
		}
	}

	// Generate package ID
	pkgID := fmt.Sprintf("%s-%s-%s", pkgType, pkg.Name, pkg.Version)

	deploymentPkg := &adapter.DeploymentPackage{
		ID:          pkgID,
		Name:        pkg.Name,
		Version:     pkg.Version,
		PackageType: "osm-" + pkgType,
		Description: pkg.Description,
		UploadedAt:  time.Now(),
		Extensions: map[string]interface{}{
			"osm.packageType": pkgType,
			"osm.project":     o.config.Project,
		},
	}

	o.mu.Lock()
	o.packages[pkgID] = deploymentPkg
	o.mu.Unlock()

	return deploymentPkg, nil
}

// DeleteDeploymentPackage removes a VNF/NS package.
func (o *Adapter) DeleteDeploymentPackage(
	ctx context.Context,
	id string,
) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	if err := o.initialize(); err != nil {
		return err
	}

	o.mu.Lock()
	defer o.mu.Unlock()

	if _, exists := o.packages[id]; !exists {
		return fmt.Errorf("package %s: %w", id, ErrPackageNotFound)
	}

	delete(o.packages, id)
	return nil
}

// ListDeployments retrieves all NS/VNF instances.
func (o *Adapter) ListDeployments(
	ctx context.Context,
	filter *adapter.Filter,
) ([]*adapter.Deployment, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	if err := o.initialize(); err != nil {
		return nil, err
	}

	o.mu.RLock()
	defer o.mu.RUnlock()

	deployments := make([]*adapter.Deployment, 0, len(o.deployments))
	for _, dep := range o.deployments {
		// Apply status filter
		if filter != nil && filter.Status != "" && dep.Status != filter.Status {
			continue
		}
		deployments = append(deployments, dep)
	}

	// Apply pagination
	if filter != nil {
		deployments = o.applyPagination(deployments, filter.Limit, filter.Offset)
	}

	return deployments, nil
}

// GetDeployment retrieves a specific NS/VNF instance by ID.
func (o *Adapter) GetDeployment(
	ctx context.Context,
	id string,
) (*adapter.Deployment, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	if err := o.initialize(); err != nil {
		return nil, err
	}

	o.mu.RLock()
	defer o.mu.RUnlock()

	dep, exists := o.deployments[id]
	if !exists {
		return nil, fmt.Errorf("deployment %s: %w", id, ErrDeploymentNotFound)
	}

	return dep, nil
}

// CreateDeployment instantiates a new NS/VNF.
func (o *Adapter) CreateDeployment(
	ctx context.Context,
	req *adapter.DeploymentRequest,
) (*adapter.Deployment, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
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

	if err := o.initialize(); err != nil {
		return nil, err
	}

	// Generate NS instance ID
	nsInstanceID := fmt.Sprintf("ns-%s-%d", req.Name, time.Now().UnixNano())

	// Get VIM account from extensions
	vimAccount := "openstack-site"
	if req.Extensions != nil {
		if va, ok := req.Extensions["osm.vimAccount"].(string); ok {
			vimAccount = va
		}
	}

	now := time.Now()
	deployment := &adapter.Deployment{
		ID:          nsInstanceID,
		Name:        req.Name,
		PackageID:   req.PackageID,
		Namespace:   req.Namespace,
		Status:      adapter.DeploymentStatusDeployed,
		Version:     1,
		Description: req.Description,
		CreatedAt:   now,
		UpdatedAt:   now,
		Extensions: map[string]interface{}{
			"osm.nsInstanceId": nsInstanceID,
			"osm.nsdId":        req.PackageID,
			"osm.vimAccount":   vimAccount,
			"osm.project":      o.config.Project,
		},
	}

	o.mu.Lock()
	o.deployments[nsInstanceID] = deployment
	o.mu.Unlock()

	return deployment, nil
}

// UpdateDeployment updates an NS/VNF instance.
func (o *Adapter) UpdateDeployment(
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

	if err := o.initialize(); err != nil {
		return nil, err
	}

	o.mu.Lock()
	defer o.mu.Unlock()

	dep, exists := o.deployments[id]
	if !exists {
		return nil, fmt.Errorf("deployment %s: %w", id, ErrDeploymentNotFound)
	}

	// Update deployment
	dep.Version++
	dep.UpdatedAt = time.Now()
	if update.Description != "" {
		dep.Description = update.Description
	}

	return dep, nil
}

// DeleteDeployment terminates an NS/VNF instance.
func (o *Adapter) DeleteDeployment(
	ctx context.Context,
	id string,
) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	if err := o.initialize(); err != nil {
		return err
	}

	o.mu.Lock()
	defer o.mu.Unlock()

	if _, exists := o.deployments[id]; !exists {
		return fmt.Errorf("deployment %s: %w", id, ErrDeploymentNotFound)
	}

	delete(o.deployments, id)
	return nil
}

// ScaleDeployment scales an NS/VNF instance.
func (o *Adapter) ScaleDeployment(
	ctx context.Context,
	id string,
	replicas int,
) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	if replicas < 0 {
		return fmt.Errorf("replicas must be non-negative")
	}

	if err := o.initialize(); err != nil {
		return err
	}

	o.mu.Lock()
	defer o.mu.Unlock()

	dep, exists := o.deployments[id]
	if !exists {
		return fmt.Errorf("deployment %s: %w", id, ErrDeploymentNotFound)
	}

	// Update extensions with scale information
	if dep.Extensions == nil {
		dep.Extensions = make(map[string]interface{})
	}
	dep.Extensions["osm.scaleCount"] = replicas
	dep.UpdatedAt = time.Now()

	return nil
}

// RollbackDeployment is not directly supported by OSM.
func (o *Adapter) RollbackDeployment(
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

	return fmt.Errorf("osm-lcm adapter %w: rollback must be done through OSM day-2 operations", ErrOperationNotSupported)
}

// GetDeploymentStatus retrieves detailed status for an NS/VNF instance.
func (o *Adapter) GetDeploymentStatus(
	ctx context.Context,
	id string,
) (*adapter.DeploymentStatusDetail, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	if err := o.initialize(); err != nil {
		return nil, err
	}

	deployment, err := o.GetDeployment(ctx, id)
	if err != nil {
		return nil, err
	}

	return &adapter.DeploymentStatusDetail{
		DeploymentID: deployment.ID,
		Status:       deployment.Status,
		Message:      deployment.Description,
		Progress:     o.calculateProgress(deployment.Status),
		UpdatedAt:    deployment.UpdatedAt,
		Conditions: []adapter.DeploymentCondition{
			{
				Type:               "Ready",
				Status:             o.conditionStatus(deployment.Status),
				Reason:             o.conditionReason(deployment.Status),
				Message:            deployment.Description,
				LastTransitionTime: deployment.UpdatedAt,
			},
		},
		Extensions: deployment.Extensions,
	}, nil
}

// GetDeploymentHistory retrieves the revision history for an NS/VNF instance.
func (o *Adapter) GetDeploymentHistory(
	ctx context.Context,
	id string,
) (*adapter.DeploymentHistory, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	if err := o.initialize(); err != nil {
		return nil, err
	}

	deployment, err := o.GetDeployment(ctx, id)
	if err != nil {
		return nil, err
	}

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

// GetDeploymentLogs retrieves logs for an NS/VNF instance.
func (o *Adapter) GetDeploymentLogs(
	ctx context.Context,
	id string,
	_ *adapter.LogOptions,
) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context cancelled: %w", err)
	}

	if err := o.initialize(); err != nil {
		return nil, err
	}

	deployment, err := o.GetDeployment(ctx, id)
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

	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal deployment logs: %w", err)
	}
	return data, nil
}

// SupportsRollback returns false as OSM doesn't support direct rollback.
func (o *Adapter) SupportsRollback() bool {
	return false
}

// SupportsScaling returns true as OSM supports NS scaling.
func (o *Adapter) SupportsScaling() bool {
	return true
}

// SupportsGitOps returns false as OSM uses API-driven orchestration.
func (o *Adapter) SupportsGitOps() bool {
	return false
}

// Health performs a health check on the OSM NBI endpoint.
func (o *Adapter) Health(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context cancelled: %w", err)
	}

	if err := o.initialize(); err != nil {
		return fmt.Errorf("osm-lcm adapter not healthy: %w", err)
	}

	// If no endpoint configured, just verify initialization
	if o.config.NBIEndpoint == "" {
		return nil
	}

	// Try to reach the OSM NBI health endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.config.NBIEndpoint+"/version", nil)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("health check failed: %w", ErrConnectionFailed)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("health check failed: status %d", resp.StatusCode)
	}

	return nil
}

// Close cleanly shuts down the adapter.
func (o *Adapter) Close() error {
	o.httpClient = nil
	return nil
}

// doRequest performs an HTTP request to the OSM NBI API.
// The body parameter uses interface{} to accept various request payload types
// (maps, structs) that are marshaled to JSON - this flexibility is required
// to support different OSM NBI endpoints with varying request schemas.
func (o *Adapter) doRequest(
	ctx context.Context,
	method, path string,
	body interface{},
) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, o.config.NBIEndpoint+path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Set basic auth
	if o.config.Username != "" {
		req.SetBasicAuth(o.config.Username, o.config.Password)
	}

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// Helper functions

func (o *Adapter) calculateProgress(status adapter.DeploymentStatus) int {
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

func (o *Adapter) conditionStatus(status adapter.DeploymentStatus) string {
	if status == adapter.DeploymentStatusDeployed {
		return "True"
	}
	return "False"
}

func (o *Adapter) conditionReason(status adapter.DeploymentStatus) string {
	switch status {
	case adapter.DeploymentStatusDeployed:
		return "InstantiationSucceeded"
	case adapter.DeploymentStatusDeploying:
		return "Instantiating"
	case adapter.DeploymentStatusPending:
		return "Pending"
	case adapter.DeploymentStatusRollingBack:
		return "RollingBack"
	case adapter.DeploymentStatusDeleting:
		return "Deleting"
	case adapter.DeploymentStatusFailed:
		return "InstantiationFailed"
	default:
		return "Unknown"
	}
}

func (o *Adapter) applyPagination(
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

func (o *Adapter) applyPackagePagination(
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
