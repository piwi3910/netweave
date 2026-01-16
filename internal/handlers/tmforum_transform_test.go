package handlers

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	imsadapter "github.com/piwi3910/netweave/internal/adapter"
	dmsadapter "github.com/piwi3910/netweave/internal/dms/adapter"
	"github.com/piwi3910/netweave/internal/models"
)

// ========================================
// TMF639 Resource Pool Transformations
// ========================================

func TestTransformResourcePoolToTMF639Resource(t *testing.T) {
	tests := []struct {
		name     string
		pool     *imsadapter.ResourcePool
		baseURL  string
		expected *models.TMF639Resource
	}{
		{
			name: "basic resource pool",
			pool: &imsadapter.ResourcePool{
				ResourcePoolID:   "pool-1",
				Name:             "Test Pool",
				Description:      "Test Description",
				GlobalLocationID: "us-east-1",
				Extensions: map[string]interface{}{
					"datacenter": "DC1",
				},
			},
			baseURL: "http://localhost:8080",
			expected: &models.TMF639Resource{
				ID:               "pool-1",
				Href:             "http://localhost:8080/tmf-api/resourceInventoryManagement/v4/resource/pool-1",
				Name:             "Test Pool",
				Description:      "Test Description",
				Category:         "resourcePool",
				ResourceStatus:   "available",
				OperationalState: "enable",
				AtType:           "ResourcePool",
			},
		},
		{
			name: "resource pool with TMF extensions",
			pool: &imsadapter.ResourcePool{
				ResourcePoolID: "pool-2",
				Name:           "GPU Pool",
				Extensions: map[string]interface{}{
					"tmf.category":         "gpu-pool",
					"tmf.resourceStatus":   "reserved",
					"tmf.operationalState": "disable",
					"gpu_count":            "32",
				},
			},
			baseURL: "http://localhost:8080",
			expected: &models.TMF639Resource{
				ID:               "pool-2",
				Name:             "GPU Pool",
				Category:         "gpu-pool",
				ResourceStatus:   "reserved",
				OperationalState: "disable",
				AtType:           "ResourcePool",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TransformResourcePoolToTMF639Resource(tt.pool, tt.baseURL)

			assert.Equal(t, tt.expected.ID, result.ID)
			assert.Equal(t, tt.expected.Name, result.Name)
			assert.Equal(t, tt.expected.Description, result.Description)
			assert.Equal(t, tt.expected.Category, result.Category)
			assert.Equal(t, tt.expected.ResourceStatus, result.ResourceStatus)
			assert.Equal(t, tt.expected.OperationalState, result.OperationalState)
			assert.Equal(t, tt.expected.AtType, result.AtType)
			assert.Contains(t, result.Href, tt.expected.ID)
		})
	}
}

func TestTransformTMF639ResourceToResourcePool(t *testing.T) {
	tests := []struct {
		name     string
		tmf      *models.TMF639Resource
		expected *imsadapter.ResourcePool
	}{
		{
			name: "basic TMF639 to resource pool",
			tmf: &models.TMF639Resource{
				ID:          "pool-1",
				Name:        "Test Pool",
				Description: "Test Description",
				Category:    "resourcePool",
				Place: []models.PlaceRef{
					{ID: "us-east-1", Role: "location"},
				},
				ResourceCharacteristic: []models.Characteristic{
					{Name: "datacenter", Value: "DC1"},
				},
			},
			expected: &imsadapter.ResourcePool{
				ResourcePoolID:   "pool-1",
				Name:             "Test Pool",
				Description:      "Test Description",
				GlobalLocationID: "us-east-1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TransformTMF639ResourceToResourcePool(tt.tmf)

			assert.Equal(t, tt.expected.ResourcePoolID, result.ResourcePoolID)
			assert.Equal(t, tt.expected.Name, result.Name)
			assert.Equal(t, tt.expected.Description, result.Description)
			assert.Equal(t, tt.expected.GlobalLocationID, result.GlobalLocationID)
			assert.NotNil(t, result.Extensions)
		})
	}
}

// ========================================
// TMF639 Resource Transformations
// ========================================

func TestTransformResourceToTMF639Resource(t *testing.T) {
	tests := []struct {
		name     string
		resource *imsadapter.Resource
		baseURL  string
		expected *models.TMF639Resource
	}{
		{
			name: "basic resource",
			resource: &imsadapter.Resource{
				ResourceID:     "res-1",
				ResourceTypeID: "rt-cpu-001",
				Description:    "CPU Resource",
				Extensions: map[string]interface{}{
					"name":     "cpu-node-1",
					"location": "rack-1",
					"status":   "active",
				},
			},
			baseURL: "http://localhost:8080",
			expected: &models.TMF639Resource{
				ID:               "res-1",
				Name:             "cpu-node-1",
				Description:      "CPU Resource",
				Category:         "rt-cpu-001",
				ResourceStatus:   "available",
				OperationalState: "enable",
				AtType:           "Resource",
			},
		},
		{
			name: "resource with TMF overrides",
			resource: &imsadapter.Resource{
				ResourceID:     "res-2",
				ResourceTypeID: "rt-gpu-001",
				Extensions: map[string]interface{}{
					"tmf.category":         "gpu-high-end",
					"tmf.resourceStatus":   "standby",
					"tmf.operationalState": "disable",
				},
			},
			baseURL: "http://localhost:8080",
			expected: &models.TMF639Resource{
				ID:               "res-2",
				Category:         "gpu-high-end",
				ResourceStatus:   "standby",
				OperationalState: "disable",
				AtType:           "Resource",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TransformResourceToTMF639Resource(tt.resource, tt.baseURL)

			assert.Equal(t, tt.expected.ID, result.ID)
			assert.Equal(t, tt.expected.Category, result.Category)
			assert.Equal(t, tt.expected.ResourceStatus, result.ResourceStatus)
			assert.Equal(t, tt.expected.OperationalState, result.OperationalState)
			assert.Equal(t, tt.expected.AtType, result.AtType)
		})
	}
}

func TestTransformTMF639ResourceToResource(t *testing.T) {
	tests := []struct {
		name     string
		tmf      *models.TMF639Resource
		expected *imsadapter.Resource
	}{
		{
			name: "basic TMF639 to resource",
			tmf: &models.TMF639Resource{
				ID:          "res-1",
				Description: "Test Resource",
				Category:    "rt-cpu-001",
				ResourceCharacteristic: []models.Characteristic{
					{Name: "hostname", Value: "node-1"},
					{Name: "status", Value: "active"},
				},
			},
			expected: &imsadapter.Resource{
				ResourceID:     "res-1",
				Description:    "Test Resource",
				ResourceTypeID: "rt-cpu-001",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TransformTMF639ResourceToResource(tt.tmf)

			assert.Equal(t, tt.expected.ResourceID, result.ResourceID)
			assert.Equal(t, tt.expected.Description, result.Description)
			assert.Equal(t, tt.expected.ResourceTypeID, result.ResourceTypeID)
			assert.NotNil(t, result.Extensions)
		})
	}
}

// ========================================
// TMF638 Service/Deployment Transformations
// ========================================

func TestTransformDeploymentToTMF638Service(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name       string
		deployment *dmsadapter.Deployment
		baseURL    string
		expected   *models.TMF638Service
	}{
		{
			name: "basic deployment",
			deployment: &dmsadapter.Deployment{
				ID:          "dep-1",
				Name:        "Test Service",
				Description: "Test Description",
				PackageID:   "pkg-1",
				Status:      dmsadapter.DeploymentStatusDeployed,
				Namespace:   "default",
				CreatedAt:   now,
			},
			baseURL: "http://localhost:8080",
			expected: &models.TMF638Service{
				ID:          "dep-1",
				Name:        "Test Service",
				Description: "Test Description",
				State:       tmfServiceStateActive,
				ServiceType: "deployment",
			},
		},
		{
			name: "deployment with different statuses",
			deployment: &dmsadapter.Deployment{
				ID:     "dep-2",
				Name:   "Pending Service",
				Status: dmsadapter.DeploymentStatusPending,
			},
			baseURL: "http://localhost:8080",
			expected: &models.TMF638Service{
				ID:    "dep-2",
				Name:  "Pending Service",
				State: tmfServiceStateFeasibility,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TransformDeploymentToTMF638Service(tt.deployment, tt.baseURL)

			assert.Equal(t, tt.expected.ID, result.ID)
			assert.Equal(t, tt.expected.Name, result.Name)
			assert.Equal(t, tt.expected.Description, result.Description)
			assert.Equal(t, tt.expected.State, result.State)
			assert.Contains(t, result.Href, tt.expected.ID)
		})
	}
}

func TestTransformTMF638ServiceToDeployment(t *testing.T) {
	tests := []struct {
		name     string
		tmf      *models.TMF638ServiceCreate
		expected *dmsadapter.DeploymentRequest
	}{
		{
			name: "basic service to deployment",
			tmf: &models.TMF638ServiceCreate{
				Name:        "Test Service",
				Description: "Test Description",
				ServiceSpecification: &models.ServiceSpecificationRef{
					ID: "pkg-1",
				},
				Place: []models.PlaceRef{
					{ID: "default", Role: "namespace"},
				},
				ServiceCharacteristic: []models.Characteristic{
					{Name: "replicas", Value: 3},
				},
			},
			expected: &dmsadapter.DeploymentRequest{
				Name:        "Test Service",
				Description: "Test Description",
				PackageID:   "pkg-1",
				Namespace:   "default",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TransformTMF638ServiceToDeployment(tt.tmf)

			assert.Equal(t, tt.expected.Name, result.Name)
			assert.Equal(t, tt.expected.Description, result.Description)
			assert.Equal(t, tt.expected.PackageID, result.PackageID)
			assert.Equal(t, tt.expected.Namespace, result.Namespace)
			assert.NotNil(t, result.Values)
		})
	}
}

// ========================================
// Status Mapping Functions
// ========================================

func TestMapDeploymentStatusToServiceState(t *testing.T) {
	tests := []struct {
		name     string
		status   dmsadapter.DeploymentStatus
		expected string
	}{
		{"pending", dmsadapter.DeploymentStatusPending, tmfServiceStateFeasibility},
		{"deploying", dmsadapter.DeploymentStatusDeploying, tmfServiceStateDesigned},
		{"deployed", dmsadapter.DeploymentStatusDeployed, tmfServiceStateActive},
		{"failed", dmsadapter.DeploymentStatusFailed, tmfServiceStateTerminated},
		{"rolling back", dmsadapter.DeploymentStatusRollingBack, tmfServiceStateDesigned},
		{"deleting", dmsadapter.DeploymentStatusDeleting, tmfServiceStateTerminated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapDeploymentStatusToServiceState(tt.status)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMapServiceStateToDeploymentStatus(t *testing.T) {
	tests := []struct {
		name     string
		state    string
		expected dmsadapter.DeploymentStatus
	}{
		{"feasibility", tmfServiceStateFeasibility, dmsadapter.DeploymentStatusPending},
		{"designed", tmfServiceStateDesigned, dmsadapter.DeploymentStatusDeploying},
		{"reserved", "reserved", dmsadapter.DeploymentStatusDeploying},
		{"active", tmfServiceStateActive, dmsadapter.DeploymentStatusDeployed},
		{"inactive", tmfServiceStateInactive, dmsadapter.DeploymentStatusFailed},
		{"terminated", tmfServiceStateTerminated, dmsadapter.DeploymentStatusFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapServiceStateToDeploymentStatus(tt.state)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ========================================
// Helper Functions
// ========================================

func TestExtractTMFFieldsFromExtensions(t *testing.T) {
	tests := []struct {
		name                 string
		extensions           map[string]interface{}
		expectedTMFFields    map[string]string
		expectedCharsCount   int
		expectedCharContains string
	}{
		{
			name: "extract TMF fields and characteristics",
			extensions: map[string]interface{}{
				"tmf.category":         "test-category",
				"tmf.resourceStatus":   "available",
				"tmf.operationalState": "enable",
				"datacenter":           "DC1",
				"rack":                 "R01",
			},
			expectedTMFFields: map[string]string{
				"tmf.category":         "test-category",
				"tmf.resourceStatus":   "available",
				"tmf.operationalState": "enable",
			},
			expectedCharsCount:   2, // datacenter and rack
			expectedCharContains: "datacenter",
		},
		{
			name: "only regular characteristics",
			extensions: map[string]interface{}{
				"hostname": "node-1",
				"status":   "active",
			},
			expectedTMFFields:  map[string]string{},
			expectedCharsCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capturedFields := make(map[string]string)
			chars := extractTMFFieldsFromExtensions(tt.extensions, func(key, value string) {
				capturedFields[key] = value
			})

			// Check TMF fields were captured
			for key, expectedValue := range tt.expectedTMFFields {
				assert.Equal(t, expectedValue, capturedFields[key])
			}

			// Check characteristics
			assert.Len(t, chars, tt.expectedCharsCount)
			if tt.expectedCharContains != "" {
				found := false
				for _, char := range chars {
					if char.Name == tt.expectedCharContains {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected characteristic %s not found", tt.expectedCharContains)
			}
		})
	}
}

func TestSetDefaultTMFStatuses(t *testing.T) {
	tests := []struct {
		name     string
		input    *models.TMF639Resource
		expected *models.TMF639Resource
	}{
		{
			name:  "set defaults when empty",
			input: &models.TMF639Resource{},
			expected: &models.TMF639Resource{
				ResourceStatus:   "available",
				OperationalState: "enable",
			},
		},
		{
			name: "preserve existing values",
			input: &models.TMF639Resource{
				ResourceStatus:   "reserved",
				OperationalState: "disable",
			},
			expected: &models.TMF639Resource{
				ResourceStatus:   "reserved",
				OperationalState: "disable",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setDefaultTMFStatuses(tt.input)
			assert.Equal(t, tt.expected.ResourceStatus, tt.input.ResourceStatus)
			assert.Equal(t, tt.expected.OperationalState, tt.input.OperationalState)
		})
	}
}

func TestExtractPlaceFromLocation(t *testing.T) {
	tests := []struct {
		name     string
		location string
		expected []models.PlaceRef
	}{
		{
			name:     "valid location",
			location: "us-east-1",
			expected: []models.PlaceRef{
				{ID: "us-east-1", Name: "us-east-1", Role: "location"},
			},
		},
		{
			name:     "empty location",
			location: "",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractPlaceFromLocation(tt.location)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Len(t, result, len(tt.expected))
				assert.Equal(t, tt.expected[0].ID, result[0].ID)
			}
		})
	}
}

func TestApplyTMF638ServiceUpdate(t *testing.T) {
	now := time.Now()
	dep := &dmsadapter.Deployment{
		ID:        "dep-1",
		Name:      "Original Name",
		Status:    dmsadapter.DeploymentStatusDeployed,
		CreatedAt: now,
	}

	newName := "Updated Name"
	newDesc := "Updated Description"
	newState := tmfServiceStateDesigned
	newType := "managed-service"

	update := &models.TMF638ServiceUpdate{
		Name:        &newName,
		Description: &newDesc,
		State:       &newState,
		ServiceType: &newType,
		ServiceCharacteristic: &[]models.Characteristic{
			{Name: "replicas", Value: 5},
		},
	}

	applyTMF638ServiceUpdate(dep, update)

	assert.Equal(t, "Updated Name", dep.Name)
	assert.Equal(t, "Updated Description", dep.Description)
	assert.Equal(t, dmsadapter.DeploymentStatusDeploying, dep.Status)
	assert.NotNil(t, dep.Extensions)
	assert.Equal(t, "managed-service", dep.Extensions["serviceType"])
	assert.Equal(t, 5, dep.Extensions["replicas"])
	assert.True(t, dep.UpdatedAt.After(now))
}
