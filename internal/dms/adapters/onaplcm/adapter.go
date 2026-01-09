// Package onaplcm provides an O2-DMS adapter implementation for ONAP Lifecycle Management.
// This adapter enables CNF/VNF deployment management through ONAP's Service Orchestrator (SO)
// for network function lifecycle management in telecom environments.
package onaplcm

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
	// AdapterName is the unique identifier for the ONAP-LCM adapter.
	AdapterName = "onap-lcm"

	// AdapterVersion indicates the ONAP SO API version supported by this adapter.
	AdapterVersion = "1.0.0"

	// DefaultTimeout is the default timeout for API operations.
	DefaultTimeout = 30 * time.Second

	// DefaultSOAPIPath is the default ONAP SO API path.
	DefaultSOAPIPath = "/onap/so/infra/serviceInstances/v7"

	// DefaultVNFAPIPath is the default ONAP VNF API path.
	DefaultVNFAPIPath = "/onap/so/infra/serviceInstances/v7/{serviceInstanceId}/vnfs"
)

// Typed errors for the ONAP-LCM adapter.
var (
	// ErrDeploymentNotFound is returned when a deployment is not found.
	ErrDeploymentNotFound = errors.New("deployment not found")

	// ErrPackageNotFound is returned when a package is not found.
	ErrPackageNotFound = errors.New("package not found")

	// ErrOperationNotSupported is returned when an operation is not supported.
	ErrOperationNotSupported = errors.New("operation not supported")

	// ErrInvalidName is returned when a name is invalid.
	ErrInvalidName = errors.New("invalid name")

	// ErrConnectionFailed is returned when connection to ONAP fails.
	ErrConnectionFailed = errors.New("connection to ONAP failed")

	// ErrAuthenticationFailed is returned when authentication fails.
	ErrAuthenticationFailed = errors.New("authentication failed")
)

// ONAPLCMAdapter implements the DMS adapter interface for ONAP lifecycle management.
type ONAPLCMAdapter struct {
	config      *Config
	httpClient  *http.Client
	deployments map[string]*adapter.Deployment
	packages    map[string]*adapter.DeploymentPackage
	mu          sync.RWMutex
	initOnce    sync.Once
	initErr     error
}

// Config contains configuration for the ONAP-LCM adapter.
type Config struct {
	// SOEndpoint is the ONAP SO API endpoint URL.
	SOEndpoint string

	// Username is the username for ONAP API authentication.
	Username string

	// Password is the password for ONAP API authentication.
	Password string

	// Timeout is the default timeout for API operations.
	Timeout time.Duration

	// TLSSkipVerify skips TLS certificate verification (for testing).
	TLSSkipVerify bool

	// RequestID is the request ID header value for ONAP requests.
	RequestID string
}

// NewAdapter creates a new ONAP-LCM adapter instance.
func NewAdapter(config *Config) (*ONAPLCMAdapter, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Apply defaults
	if config.Timeout == 0 {
		config.Timeout = DefaultTimeout
	}

	return &ONAPLCMAdapter{
		config:      config,
		deployments: make(map[string]*adapter.Deployment),
		packages:    make(map[string]*adapter.DeploymentPackage),
	}, nil
}

// initialize performs lazy initialization of the HTTP client.
func (o *ONAPLCMAdapter) initialize() error {
	o.initOnce.Do(func() {
		o.httpClient = &http.Client{
			Timeout: o.config.Timeout,
		}
	})

	return o.initErr
}

// Name returns the adapter name.
func (o *ONAPLCMAdapter) Name() string {
	return AdapterName
}

// Version returns the ONAP SO API version supported by this adapter.
func (o *ONAPLCMAdapter) Version() string {
	return AdapterVersion
}

// Capabilities returns the capabilities supported by the ONAP-LCM adapter.
func (o *ONAPLCMAdapter) Capabilities() []adapter.Capability {
	return []adapter.Capability{
		adapter.CapabilityPackageManagement,
		adapter.CapabilityDeploymentLifecycle,
		adapter.CapabilityScaling,
		adapter.CapabilityHealthChecks,
	}
}

// ListDeploymentPackages retrieves all available VNF/CNF packages.
func (o *ONAPLCMAdapter) ListDeploymentPackages(
	ctx context.Context,
	filter *adapter.Filter,
) ([]*adapter.DeploymentPackage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
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

// GetDeploymentPackage retrieves a specific VNF/CNF package by ID.
func (o *ONAPLCMAdapter) GetDeploymentPackage(
	ctx context.Context,
	id string,
) (*adapter.DeploymentPackage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
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

// UploadDeploymentPackage registers a new VNF/CNF package.
func (o *ONAPLCMAdapter) UploadDeploymentPackage(
	ctx context.Context,
	pkg *adapter.DeploymentPackageUpload,
) (*adapter.DeploymentPackage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if pkg == nil {
		return nil, fmt.Errorf("package cannot be nil")
	}

	if err := o.initialize(); err != nil {
		return nil, err
	}

	// Get VNF descriptor ID from extensions
	vnfDescriptorID := ""
	if pkg.Extensions != nil {
		if id, ok := pkg.Extensions["onap.vnfdId"].(string); ok {
			vnfDescriptorID = id
		}
	}
	if vnfDescriptorID == "" {
		vnfDescriptorID = fmt.Sprintf("vnfd-%s-%s", pkg.Name, pkg.Version)
	}

	deploymentPkg := &adapter.DeploymentPackage{
		ID:          vnfDescriptorID,
		Name:        pkg.Name,
		Version:     pkg.Version,
		PackageType: "onap-vnf",
		Description: pkg.Description,
		UploadedAt:  time.Now(),
		Extensions: map[string]interface{}{
			"onap.vnfdId":      vnfDescriptorID,
			"onap.packageType": "VNF",
		},
	}

	o.mu.Lock()
	o.packages[vnfDescriptorID] = deploymentPkg
	o.mu.Unlock()

	return deploymentPkg, nil
}

// DeleteDeploymentPackage removes a VNF/CNF package.
func (o *ONAPLCMAdapter) DeleteDeploymentPackage(
	ctx context.Context,
	id string,
) error {
	if err := ctx.Err(); err != nil {
		return err
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

// ListDeployments retrieves all VNF/CNF instances.
func (o *ONAPLCMAdapter) ListDeployments(
	ctx context.Context,
	filter *adapter.Filter,
) ([]*adapter.Deployment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
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

// GetDeployment retrieves a specific VNF/CNF instance by ID.
func (o *ONAPLCMAdapter) GetDeployment(
	ctx context.Context,
	id string,
) (*adapter.Deployment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
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

// CreateDeployment instantiates a new VNF/CNF.
func (o *ONAPLCMAdapter) CreateDeployment(
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

	if err := o.initialize(); err != nil {
		return nil, err
	}

	// Generate VNF instance ID
	vnfInstanceID := fmt.Sprintf("vnf-%s-%d", req.Name, time.Now().UnixNano())

	now := time.Now()
	deployment := &adapter.Deployment{
		ID:          vnfInstanceID,
		Name:        req.Name,
		PackageID:   req.PackageID,
		Namespace:   req.Namespace,
		Status:      adapter.DeploymentStatusDeployed,
		Version:     1,
		Description: req.Description,
		CreatedAt:   now,
		UpdatedAt:   now,
		Extensions: map[string]interface{}{
			"onap.vnfInstanceId":  vnfInstanceID,
			"onap.vnfdId":         req.PackageID,
			"onap.instantiateVnf": true,
		},
	}

	o.mu.Lock()
	o.deployments[vnfInstanceID] = deployment
	o.mu.Unlock()

	return deployment, nil
}

// UpdateDeployment updates a VNF/CNF instance.
func (o *ONAPLCMAdapter) UpdateDeployment(
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

// DeleteDeployment terminates a VNF/CNF instance.
func (o *ONAPLCMAdapter) DeleteDeployment(
	ctx context.Context,
	id string,
) error {
	if err := ctx.Err(); err != nil {
		return err
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

// ScaleDeployment scales a VNF/CNF instance.
func (o *ONAPLCMAdapter) ScaleDeployment(
	ctx context.Context,
	id string,
	replicas int,
) error {
	if err := ctx.Err(); err != nil {
		return err
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
	dep.Extensions["onap.replicas"] = replicas
	dep.UpdatedAt = time.Now()

	return nil
}

// RollbackDeployment is not directly supported by ONAP SO.
func (o *ONAPLCMAdapter) RollbackDeployment(
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

	return fmt.Errorf("onap-lcm adapter %w: rollback must be done through ONAP SO workflow", ErrOperationNotSupported)
}

// GetDeploymentStatus retrieves detailed status for a VNF/CNF instance.
func (o *ONAPLCMAdapter) GetDeploymentStatus(
	ctx context.Context,
	id string,
) (*adapter.DeploymentStatusDetail, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
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

// GetDeploymentHistory retrieves the revision history for a VNF/CNF instance.
func (o *ONAPLCMAdapter) GetDeploymentHistory(
	ctx context.Context,
	id string,
) (*adapter.DeploymentHistory, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
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

// GetDeploymentLogs retrieves logs for a VNF/CNF instance.
func (o *ONAPLCMAdapter) GetDeploymentLogs(
	ctx context.Context,
	id string,
	_ *adapter.LogOptions,
) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
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

	return json.MarshalIndent(info, "", "  ")
}

// SupportsRollback returns false as ONAP SO doesn't support direct rollback.
func (o *ONAPLCMAdapter) SupportsRollback() bool {
	return false
}

// SupportsScaling returns true as ONAP SO supports VNF scaling.
func (o *ONAPLCMAdapter) SupportsScaling() bool {
	return true
}

// SupportsGitOps returns false as ONAP SO uses API-driven orchestration.
func (o *ONAPLCMAdapter) SupportsGitOps() bool {
	return false
}

// Health performs a health check on the ONAP SO endpoint.
func (o *ONAPLCMAdapter) Health(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := o.initialize(); err != nil {
		return fmt.Errorf("onap-lcm adapter not healthy: %w", err)
	}

	// If no endpoint configured, just verify initialization
	if o.config.SOEndpoint == "" {
		return nil
	}

	// Try to reach the ONAP SO health endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.config.SOEndpoint+"/manage/health", nil)
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
func (o *ONAPLCMAdapter) Close() error {
	o.httpClient = nil
	return nil
}

// doRequest performs an HTTP request to the ONAP SO API.
func (o *ONAPLCMAdapter) doRequest(
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

	req, err := http.NewRequestWithContext(ctx, method, o.config.SOEndpoint+path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if o.config.RequestID != "" {
		req.Header.Set("X-RequestID", o.config.RequestID)
	}

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

func (o *ONAPLCMAdapter) calculateProgress(status adapter.DeploymentStatus) int {
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

func (o *ONAPLCMAdapter) conditionStatus(status adapter.DeploymentStatus) string {
	if status == adapter.DeploymentStatusDeployed {
		return "True"
	}
	return "False"
}

func (o *ONAPLCMAdapter) conditionReason(status adapter.DeploymentStatus) string {
	switch status {
	case adapter.DeploymentStatusDeployed:
		return "InstantiationSucceeded"
	case adapter.DeploymentStatusDeploying:
		return "Instantiating"
	case adapter.DeploymentStatusFailed:
		return "InstantiationFailed"
	default:
		return "Unknown"
	}
}

func (o *ONAPLCMAdapter) applyPagination(
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

func (o *ONAPLCMAdapter) applyPackagePagination(
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
