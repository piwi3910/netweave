package osm

import (
	"context"
	"fmt"
	"time"
)

const (
	// OSM operational status values
	osmStatusRunning = "running"
)

// DeploymentPackage represents a deployment package (NSD or VNFD) in OSM.
type DeploymentPackage struct {
	ID          string                 `json:"_id,omitempty"`
	Name        string                 `json:"id"` // Package identifier (not _id)
	Version     string                 `json:"version,omitempty"`
	PackageType string                 `json:"package_type"` // "nsd" or "vnfd"
	Descriptor  map[string]interface{} `json:"descriptor,omitempty"`
	UploadedAt  time.Time              `json:"_admin.created,omitempty"`
}

// DeploymentRequest represents a request to deploy an NS (Network Service).
type DeploymentRequest struct {
	// NSName is the name for the NS instance
	NSName string `json:"nsName"`

	// NSDId is the ID of the NSD (Network Service Descriptor)
	NSDId string `json:"nsdId"`

	// VIMAccountID is the ID of the VIM account to deploy to
	VIMAccountID string `json:"vimAccountId"`

	// NSDescription provides context about this deployment
	NSDescription string `json:"nsDescription,omitempty"`

	// AdditionalParams contains deployment-specific parameters
	AdditionalParams map[string]interface{} `json:"additionalParamsForNs,omitempty"`

	// VNF-specific overrides
	VNF []VNFParams `json:"vnf,omitempty"`
}

// VNFParams contains VNF-specific deployment parameters.
type VNFParams struct {
	// MemberVnfIndex identifies which VNF in the NS
	MemberVnfIndex string `json:"member-vnf-index"`

	// VIMAccountID can override the NS-level VIM account
	VIMAccountID string `json:"vimAccountId,omitempty"`

	// AdditionalParams contains VNF-specific parameters
	AdditionalParams map[string]interface{} `json:"additionalParamsForVnf,omitempty"`
}

// Deployment represents a deployed NS instance in OSM.
type Deployment struct {
	// ID is the NS instance identifier (_id)
	ID string `json:"_id"`

	// Name is the NS instance name
	Name string `json:"name"`

	// NSDId is the ID of the NSD used
	NSDId string `json:"nsd-id"`

	// OperationalStatus indicates the current operational state
	OperationalStatus string `json:"operational-status"`

	// ConfigStatus indicates the configuration state
	ConfigStatus string `json:"config-status"`

	// DetailedStatus provides human-readable status information
	DetailedStatus string `json:"detailed-status,omitempty"`

	// ConstituentVNFRIds lists the VNF instance IDs
	ConstituentVNFRIds []string `json:"constituent-vnfr-ref,omitempty"`

	// VIMAccount is the VIM account used for deployment
	VIMAccount string `json:"vim-account-id,omitempty"`

	// CreateTime is when the NS instance was created
	CreateTime string `json:"create-time,omitempty"`

	// ModifyTime is when the NS instance was last modified
	ModifyTime string `json:"modify-time,omitempty"`

	// AdminStatus contains additional administrative information
	AdminStatus map[string]interface{} `json:"_admin,omitempty"`
}

// DeploymentStatus provides detailed status information for a deployment.
type DeploymentStatus struct {
	DeploymentID      string                 `json:"deploymentId"`
	Status            string                 `json:"status"`
	OperationalStatus string                 `json:"operationalStatus"`
	ConfigStatus      string                 `json:"configStatus"`
	DetailedStatus    string                 `json:"detailedStatus,omitempty"`
	VNFStatuses       []VNFStatus            `json:"vnfStatuses,omitempty"`
	UpdatedAt         time.Time              `json:"updatedAt"`
	Extensions        map[string]interface{} `json:"extensions,omitempty"`
}

// VNFStatus represents the status of a VNF within an NS.
type VNFStatus struct {
	VNFId             string `json:"vnfId"`
	MemberVnfIndex    string `json:"memberVnfIndex"`
	OperationalStatus string `json:"operationalStatus"`
	DetailedStatus    string `json:"detailedStatus,omitempty"`
}

// NSScaleRequest represents a request to scale an NS.
type NSScaleRequest struct {
	// ScaleType specifies the type of scaling operation
	ScaleType string `json:"scaleType"` // "SCALE_VNF"

	// ScaleVnfData contains VNF scaling parameters
	ScaleVnfData ScaleVnfData `json:"scaleVnfData,omitempty"`

	// ScaleTime specifies when to perform the scale operation
	ScaleTime string `json:"scaleTime,omitempty"`
}

// ScaleVnfData contains parameters for scaling a VNF.
type ScaleVnfData struct {
	// ScaleVnfType specifies scale direction ("SCALE_OUT" or "SCALE_IN")
	ScaleVnfType string `json:"scaleVnfType"`

	// ScaleByStepData contains step-based scaling parameters
	ScaleByStepData ScaleByStepData `json:"scaleByStepData,omitempty"`
}

// ScaleByStepData contains step-based scaling parameters.
type ScaleByStepData struct {
	// ScalingGroupDescriptor identifies the scaling group
	ScalingGroupDescriptor string `json:"scaling-group-descriptor"`

	// MemberVnfIndex identifies which VNF to scale
	MemberVnfIndex string `json:"member-vnf-index"`
}

// NSHealRequest represents a request to heal an NS (Day-2 operation).
type NSHealRequest struct {
	// VNFInstanceID identifies which VNF to heal
	VNFInstanceID string `json:"vnfInstanceId"`

	// Cause describes why healing is needed
	Cause string `json:"cause,omitempty"`

	// AdditionalParams contains healing-specific parameters
	AdditionalParams map[string]interface{} `json:"additionalParams,omitempty"`
}

// ==== DMS Backend Mode: Package Management ====

// OnboardNSD uploads and onboards an NSD (Network Service Descriptor) to OSM.
// This is a DMS backend operation for package management.
func (p *Plugin) OnboardNSD(ctx context.Context, nsdContent []byte) (string, error) {
	// OSM expects NSDs as tar.gz packages
	// The package should contain:
	//   - NS descriptor YAML
	//   - checksums.txt
	//   - metadata (optional)

	// TODO: Implement multipart/form-data upload
	// POST /osm/nsd/v1/ns_descriptors_content
	// Content-Type: application/gzip

	// For now, return an error indicating this needs implementation
	return "", fmt.Errorf("NSD onboarding not yet implemented")
}

// OnboardVNFD uploads and onboards a VNFD (VNF Descriptor) to OSM.
func (p *Plugin) OnboardVNFD(ctx context.Context, vnfdContent []byte) (string, error) {
	// Similar to NSD onboarding
	// POST /osm/vnfpkgm/v1/vnf_packages_content

	return "", fmt.Errorf("VNFD onboarding not yet implemented")
}

// GetNSD retrieves an NSD by ID.
func (p *Plugin) GetNSD(ctx context.Context, nsdID string) (*DeploymentPackage, error) {
	if nsdID == "" {
		return nil, fmt.Errorf("nsd id is required")
	}

	var pkg DeploymentPackage
	if err := p.client.get(ctx, fmt.Sprintf("/osm/nsd/v1/ns_descriptors/%s", nsdID), &pkg); err != nil {
		return nil, fmt.Errorf("failed to get NSD: %w", err)
	}

	pkg.PackageType = "nsd"
	return &pkg, nil
}

// ListNSDs retrieves all NSDs from OSM.
func (p *Plugin) ListNSDs(ctx context.Context) ([]*DeploymentPackage, error) {
	var pkgs []*DeploymentPackage
	if err := p.client.get(ctx, "/osm/nsd/v1/ns_descriptors", &pkgs); err != nil {
		return nil, fmt.Errorf("failed to list NSDs: %w", err)
	}

	for _, pkg := range pkgs {
		pkg.PackageType = "nsd"
	}

	return pkgs, nil
}

// DeleteNSD deletes an NSD from OSM.
func (p *Plugin) DeleteNSD(ctx context.Context, nsdID string) error {
	if nsdID == "" {
		return fmt.Errorf("nsd id is required")
	}

	if err := p.client.delete(ctx, fmt.Sprintf("/osm/nsd/v1/ns_descriptors/%s", nsdID)); err != nil {
		return fmt.Errorf("failed to delete NSD: %w", err)
	}

	return nil
}

// ==== DMS Backend Mode: NS Lifecycle Management ====

// InstantiateNS creates and instantiates a new NS instance.
// This is the primary DMS deployment operation.
func (p *Plugin) InstantiateNS(ctx context.Context, req *DeploymentRequest) (string, error) {
	if req == nil {
		return "", fmt.Errorf("deployment request cannot be nil")
	}

	// Validate required fields
	if req.NSName == "" {
		return "", fmt.Errorf("ns name is required")
	}
	if req.NSDId == "" {
		return "", fmt.Errorf("nsd id is required")
	}
	if req.VIMAccountID == "" {
		return "", fmt.Errorf("vim account id is required")
	}

	// Create NS instance via OSM NBI
	var result Deployment
	if err := p.client.post(ctx, "/osm/nslcm/v1/ns_instances_content", req, &result); err != nil {
		return "", fmt.Errorf("failed to instantiate NS: %w", err)
	}

	return result.ID, nil
}

// GetNSInstance retrieves detailed information about an NS instance.
func (p *Plugin) GetNSInstance(ctx context.Context, nsInstanceID string) (*Deployment, error) {
	if nsInstanceID == "" {
		return nil, fmt.Errorf("ns instance id is required")
	}

	var deployment Deployment
	if err := p.client.get(ctx, fmt.Sprintf("/osm/nslcm/v1/ns_instances/%s", nsInstanceID), &deployment); err != nil {
		return nil, fmt.Errorf("failed to get NS instance: %w", err)
	}

	return &deployment, nil
}

// ListNSInstances retrieves all NS instances from OSM.
func (p *Plugin) ListNSInstances(ctx context.Context) ([]*Deployment, error) {
	var deployments []*Deployment
	if err := p.client.get(ctx, "/osm/nslcm/v1/ns_instances", &deployments); err != nil {
		return nil, fmt.Errorf("failed to list NS instances: %w", err)
	}

	return deployments, nil
}

// TerminateNS terminates (deletes) an NS instance.
func (p *Plugin) TerminateNS(ctx context.Context, nsInstanceID string) error {
	if nsInstanceID == "" {
		return fmt.Errorf("ns instance id is required")
	}

	// OSM terminate requires a POST to the terminate action
	terminateReq := map[string]interface{}{
		"terminateTime": time.Now().UTC().Format(time.RFC3339),
	}

	if err := p.client.post(ctx, fmt.Sprintf("/osm/nslcm/v1/ns_instances/%s/terminate", nsInstanceID), terminateReq, nil); err != nil {
		return fmt.Errorf("failed to terminate NS: %w", err)
	}

	return nil
}

// GetNSStatus retrieves the current status of an NS instance.
// This polls OSM for the latest operational and configuration status.
func (p *Plugin) GetNSStatus(ctx context.Context, nsInstanceID string) (*DeploymentStatus, error) {
	deployment, err := p.GetNSInstance(ctx, nsInstanceID)
	if err != nil {
		return nil, err
	}

	// Parse modify time
	var modifyTime time.Time
	if deployment.ModifyTime != "" {
		modifyTime, _ = time.Parse(time.RFC3339, deployment.ModifyTime)
	}

	status := &DeploymentStatus{
		DeploymentID:      deployment.ID,
		Status:            p.mapOSMStatus(deployment.OperationalStatus),
		OperationalStatus: deployment.OperationalStatus,
		ConfigStatus:      deployment.ConfigStatus,
		DetailedStatus:    deployment.DetailedStatus,
		UpdatedAt:         modifyTime,
		Extensions: map[string]interface{}{
			"osm.nsdId":        deployment.NSDId,
			"osm.vimAccountId": deployment.VIMAccount,
			"osm.vnfCount":     len(deployment.ConstituentVNFRIds),
			"osm.createTime":   deployment.CreateTime,
		},
	}

	// Query VNF statuses if available
	if len(deployment.ConstituentVNFRIds) > 0 {
		status.VNFStatuses = p.getVNFStatuses(ctx, deployment.ConstituentVNFRIds)
	}

	return status, nil
}

// getVNFStatuses retrieves status information for multiple VNFs.
func (p *Plugin) getVNFStatuses(ctx context.Context, vnfIDs []string) []VNFStatus {
	statuses := make([]VNFStatus, 0, len(vnfIDs))

	for _, vnfID := range vnfIDs {
		var vnfr struct {
			ID                string `json:"_id"`
			MemberVnfIndex    string `json:"member-vnf-index-ref"`
			OperationalStatus string `json:"operational-status"`
			DetailedStatus    string `json:"detailed-status"`
		}

		if err := p.client.get(ctx, fmt.Sprintf("/osm/nslcm/v1/vnf_instances/%s", vnfID), &vnfr); err != nil {
			// Log error but continue with other VNFs
			continue
		}

		statuses = append(statuses, VNFStatus{
			VNFId:             vnfr.ID,
			MemberVnfIndex:    vnfr.MemberVnfIndex,
			OperationalStatus: vnfr.OperationalStatus,
			DetailedStatus:    vnfr.DetailedStatus,
		})
	}

	return statuses
}

// ==== Day-2 Operations ====

// ScaleNS scales an NS by adding or removing VNF instances.
// This is a Day-2 operation for horizontal scaling.
func (p *Plugin) ScaleNS(ctx context.Context, nsInstanceID string, scaleReq *NSScaleRequest) error {
	if nsInstanceID == "" {
		return fmt.Errorf("ns instance id is required")
	}
	if scaleReq == nil {
		return fmt.Errorf("scale request cannot be nil")
	}

	// Validate scale request
	if scaleReq.ScaleType != "SCALE_VNF" {
		return fmt.Errorf("unsupported scale type: %s", scaleReq.ScaleType)
	}

	// Execute scale operation via OSM NBI
	if err := p.client.post(ctx, fmt.Sprintf("/osm/nslcm/v1/ns_instances/%s/scale", nsInstanceID), scaleReq, nil); err != nil {
		return fmt.Errorf("failed to scale NS: %w", err)
	}

	return nil
}

// HealNS heals a failed VNF within an NS.
// This is a Day-2 operation for fault recovery.
func (p *Plugin) HealNS(ctx context.Context, nsInstanceID string, healReq *NSHealRequest) error {
	if nsInstanceID == "" {
		return fmt.Errorf("ns instance id is required")
	}
	if healReq == nil {
		return fmt.Errorf("heal request cannot be nil")
	}

	// Validate heal request
	if healReq.VNFInstanceID == "" {
		return fmt.Errorf("vnf instance id is required")
	}

	// Execute heal operation via OSM NBI
	if err := p.client.post(ctx, fmt.Sprintf("/osm/nslcm/v1/ns_instances/%s/heal", nsInstanceID), healReq, nil); err != nil {
		return fmt.Errorf("failed to heal NS: %w", err)
	}

	return nil
}

// mapOSMStatus maps OSM operational status to standardized deployment status.
func (p *Plugin) mapOSMStatus(osmStatus string) string {
	switch osmStatus {
	case "init", "building":
		return "BUILDING"
	case osmStatusRunning:
		return "ACTIVE"
	case "scaling":
		return "SCALING"
	case "healing":
		return "HEALING"
	case "terminating":
		return "DELETING"
	case "terminated":
		return "DELETED"
	case "failed", "error":
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// WaitForNSReady polls an NS instance until it reaches a stable state.
// This is useful for synchronous deployment operations.
func (p *Plugin) WaitForNSReady(ctx context.Context, nsInstanceID string, timeout time.Duration) error {
	if nsInstanceID == "" {
		return fmt.Errorf("ns instance id is required")
	}

	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(p.config.LCMPollingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled while waiting for NS ready: %w", ctx.Err())
		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for NS to become ready")
			}

			deployment, err := p.GetNSInstance(ctx, nsInstanceID)
			if err != nil {
				return fmt.Errorf("failed to query NS status: %w", err)
			}

			// Check if NS is in a stable state
			switch deployment.OperationalStatus {
			case osmStatusRunning:
				return nil // Success
			case "failed", "error", "terminated":
				return fmt.Errorf("NS entered failed state: %s", deployment.DetailedStatus)
			default:
				// Still in transition, continue polling
			}
		}
	}
}
