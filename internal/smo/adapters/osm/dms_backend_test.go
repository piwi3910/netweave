package osm_test

import (
	"testing"
	"time"

	"github.com/piwi3910/netweave/internal/smo/adapters/osm"
)

// TestInstantiateNS tests NS instantiation.
func TestInstantiateNS(t *testing.T) {
	tests := []struct {
		name    string
		req     *osm.DeploymentRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid deployment request",
			req: &osm.DeploymentRequest{
				NSName:        "test-ns-1",
				NSDId:         "nsd-123",
				VIMAccountID:  "vim-456",
				NSDescription: "Test network service",
				AdditionalParams: map[string]interface{}{
					"param1": "value1",
				},
			},
			wantErr: false,
		},
		{
			name:    "nil deployment request",
			req:     nil,
			wantErr: true,
			errMsg:  "deployment request cannot be nil",
		},
		{
			name: "missing NS name",
			req: &osm.DeploymentRequest{
				NSDId:        "nsd-123",
				VIMAccountID: "vim-456",
			},
			wantErr: true,
			errMsg:  "ns name is required",
		},
		{
			name: "missing NSD ID",
			req: &osm.DeploymentRequest{
				NSName:       "test-ns",
				VIMAccountID: "vim-456",
			},
			wantErr: true,
			errMsg:  "nsd id is required",
		},
		{
			name: "missing VIM account ID",
			req: &osm.DeploymentRequest{
				NSName: "test-ns",
				NSDId:  "nsd-123",
			},
			wantErr: true,
			errMsg:  "vim account id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: This test validates request validation logic only
			// Full integration testing would require a mock OSM server

			err := osm.ValidateDeploymentRequest(tt.req)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected validation error but got none")
					return
				}
				if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("Error = %v, want %v", err.Error(), tt.errMsg)
				}
			} else if err != nil {
				t.Errorf("Unexpected validation error: %v", err)
			}
		})
	}
}

// TestMapOSMStatusComprehensive tests all OSM status mappings.
func TestMapOSMStatusComprehensive(t *testing.T) {
	plugin := &osm.Plugin{}

	tests := []struct {
		osmStatus  string
		wantStatus string
	}{
		// Building states
		{"init", "BUILDING"},
		{"building", "BUILDING"},

		// Active states
		{"running", "ACTIVE"},

		// Operational states
		{"scaling", "SCALING"},
		{"healing", "HEALING"},

		// Deletion states
		{"terminating", "DELETING"},
		{"terminated", "DELETED"},

		// Error states
		{"failed", "ERROR"},
		{"error", "ERROR"},

		// Unknown states
		{"", "UNKNOWN"},
		{"invalid-state", "UNKNOWN"},
		{"pending", "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.osmStatus, func(t *testing.T) {
			result := plugin.MapOSMStatus(tt.osmStatus)
			if result != tt.wantStatus {
				t.Errorf("MapOSMStatus(%q) = %q, want %q", tt.osmStatus, result, tt.wantStatus)
			}
		})
	}
}

// TestNSScaleRequest tests NS scale request validation.
func TestNSScaleRequest(t *testing.T) {
	tests := []struct {
		name        string
		nsID        string
		scaleReq    *osm.NSScaleRequest
		wantErr     bool
		errContains string
	}{
		{
			name: "valid scale out request",
			nsID: "ns-123",
			scaleReq: &osm.NSScaleRequest{
				ScaleType: "SCALE_VNF",
				ScaleVnfData: osm.ScaleVnfData{
					ScaleVnfType: "SCALE_OUT",
					ScaleByStepData: osm.ScaleByStepData{
						ScalingGroupDescriptor: "default",
						MemberVnfIndex:         "1",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid scale in request",
			nsID: "ns-123",
			scaleReq: &osm.NSScaleRequest{
				ScaleType: "SCALE_VNF",
				ScaleVnfData: osm.ScaleVnfData{
					ScaleVnfType: "SCALE_IN",
					ScaleByStepData: osm.ScaleByStepData{
						ScalingGroupDescriptor: "default",
						MemberVnfIndex:         "1",
					},
				},
			},
			wantErr: false,
		},
		{
			name:        "empty NS ID",
			nsID:        "",
			scaleReq:    &osm.NSScaleRequest{ScaleType: "SCALE_VNF"},
			wantErr:     true,
			errContains: "ns instance id is required",
		},
		{
			name:        "nil scale request",
			nsID:        "ns-123",
			scaleReq:    nil,
			wantErr:     true,
			errContains: "scale request cannot be nil",
		},
		{
			name: "unsupported scale type",
			nsID: "ns-123",
			scaleReq: &osm.NSScaleRequest{
				ScaleType: "INVALID_TYPE",
			},
			wantErr:     true,
			errContains: "unsupported scale type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := osm.ValidateScaleRequest(tt.nsID, tt.scaleReq)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected validation error but got none")
					return
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("Error = %v, want to contain %v", err.Error(), tt.errContains)
				}
			} else if err != nil {
				t.Errorf("Unexpected validation error: %v", err)
			}
		})
	}
}

// TestNSHealRequest tests NS heal request validation.
func TestNSHealRequest(t *testing.T) {
	tests := []struct {
		name        string
		nsID        string
		healReq     *osm.NSHealRequest
		wantErr     bool
		errContains string
	}{
		{
			name: "valid heal request",
			nsID: "ns-123",
			healReq: &osm.NSHealRequest{
				VNFInstanceID: "vnf-456",
				Cause:         "VNF failure detected",
			},
			wantErr: false,
		},
		{
			name:        "empty NS ID",
			nsID:        "",
			healReq:     &osm.NSHealRequest{VNFInstanceID: "vnf-456"},
			wantErr:     true,
			errContains: "ns instance id is required",
		},
		{
			name:        "nil heal request",
			nsID:        "ns-123",
			healReq:     nil,
			wantErr:     true,
			errContains: "heal request cannot be nil",
		},
		{
			name: "missing VNF instance ID",
			nsID: "ns-123",
			healReq: &osm.NSHealRequest{
				Cause: "Need to heal something",
			},
			wantErr:     true,
			errContains: "vnf instance id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := osm.ValidateHealRequest(tt.nsID, tt.healReq)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected validation error but got none")
					return
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("Error = %v, want to contain %v", err.Error(), tt.errContains)
				}
			} else if err != nil {
				t.Errorf("Unexpected validation error: %v", err)
			}
		})
	}
}

// TestVNFStatus tests VNF status structure.
func TestVNFStatus(t *testing.T) {
	status := osm.VNFStatus{
		VNFId:             "vnf-123",
		MemberVnfIndex:    "1",
		OperationalStatus: "running",
		DetailedStatus:    "VNF is healthy",
	}

	if status.VNFId != "vnf-123" {
		t.Errorf("VNFId = %v, want %v", status.VNFId, "vnf-123")
	}
	if status.MemberVnfIndex != "1" {
		t.Errorf("MemberVnfIndex = %v, want %v", status.MemberVnfIndex, "1")
	}
	if status.OperationalStatus != "running" {
		t.Errorf("OperationalStatus = %v, want %v", status.OperationalStatus, "running")
	}
	if status.DetailedStatus != "VNF is healthy" {
		t.Errorf("DetailedStatus = %v, want %v", status.DetailedStatus, "VNF is healthy")
	}
}

// TestDeploymentStatus tests deployment status structure.
func TestDeploymentStatus(t *testing.T) {
	now := time.Now()
	status := &osm.DeploymentStatus{
		DeploymentID: "ns-123",
		Status:       "ACTIVE",
		UpdatedAt:    now,
		VNFStatuses: []osm.VNFStatus{
			{
				VNFId:             "vnf-1",
				MemberVnfIndex:    "1",
				OperationalStatus: "running",
			},
		},
		Extensions: map[string]interface{}{
			"osm.nsdId": "nsd-456",
		},
	}

	if status.DeploymentID != "ns-123" {
		t.Errorf("DeploymentID = %v, want %v", status.DeploymentID, "ns-123")
	}
	if status.Status != "ACTIVE" {
		t.Errorf("Status = %v, want %v", status.Status, "ACTIVE")
	}
	if len(status.VNFStatuses) != 1 {
		t.Errorf("VNFStatuses length = %v, want %v", len(status.VNFStatuses), 1)
	}
	if status.Extensions["osm.nsdId"] != "nsd-456" {
		t.Errorf("Extensions[osm.nsdId] = %v, want %v", status.Extensions["osm.nsdId"], "nsd-456")
	}
	if !status.UpdatedAt.Equal(now) {
		t.Errorf("UpdatedAt = %v, want %v", status.UpdatedAt, now)
	}
}

// Validation helper functions are now exported from the osm package
