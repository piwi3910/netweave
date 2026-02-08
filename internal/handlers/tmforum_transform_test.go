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

func TestApplyTMF638ServiceUpdate_WithPlace(t *testing.T) {
	dep := &dmsadapter.Deployment{
		ID:     "dep-1",
		Name:   "Original",
		Status: dmsadapter.DeploymentStatusDeployed,
	}

	newPlaces := []models.PlaceRef{{ID: "new-namespace", Role: "namespace"}}
	update := &models.TMF638ServiceUpdate{
		Place: &newPlaces,
	}

	applyTMF638ServiceUpdate(dep, update)

	assert.Equal(t, "new-namespace", dep.Namespace)
}

func TestApplyTMF638ServiceUpdate_NilFields(t *testing.T) {
	dep := &dmsadapter.Deployment{
		ID:     "dep-1",
		Name:   "Original",
		Status: dmsadapter.DeploymentStatusDeployed,
	}

	update := &models.TMF638ServiceUpdate{}

	applyTMF638ServiceUpdate(dep, update)

	// Original values should be preserved
	assert.Equal(t, "Original", dep.Name)
	assert.Equal(t, dmsadapter.DeploymentStatusDeployed, dep.Status)
}

// ========================================
// TMF641 Transform Tests (0% coverage)
// ========================================

func TestBuildBaseURL(t *testing.T) {
	tests := []struct {
		name     string
		scheme   string
		host     string
		expected string
	}{
		{
			name:     "with scheme",
			scheme:   "https",
			host:     "api.example.com",
			expected: "https://api.example.com",
		},
		{
			name:     "empty scheme defaults to http",
			scheme:   "",
			host:     "localhost:8080",
			expected: "http://localhost:8080",
		},
		{
			name:     "http scheme",
			scheme:   "http",
			host:     "localhost",
			expected: "http://localhost",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildBaseURL(tt.scheme, tt.host)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestApplyTMF639ResourceUpdate(t *testing.T) {
	tests := []struct {
		name     string
		pool     *imsadapter.ResourcePool
		update   *models.TMF639ResourceUpdate
		expected *imsadapter.ResourcePool
	}{
		{
			name: "update name and description",
			pool: &imsadapter.ResourcePool{
				ResourcePoolID: "pool-1",
				Name:           "Original",
				Description:    "Original Desc",
				Extensions:     map[string]interface{}{},
			},
			update: func() *models.TMF639ResourceUpdate {
				name := "Updated Pool"
				desc := "Updated Desc"
				return &models.TMF639ResourceUpdate{
					Name:        &name,
					Description: &desc,
				}
			}(),
			expected: &imsadapter.ResourcePool{
				Name:        "Updated Pool",
				Description: "Updated Desc",
			},
		},
		{
			name: "update place",
			pool: &imsadapter.ResourcePool{
				ResourcePoolID:   "pool-1",
				GlobalLocationID: "old-location",
				Extensions:       map[string]interface{}{},
			},
			update: func() *models.TMF639ResourceUpdate {
				places := []models.PlaceRef{{ID: "new-location"}}
				return &models.TMF639ResourceUpdate{
					Place: &places,
				}
			}(),
			expected: &imsadapter.ResourcePool{
				GlobalLocationID: "new-location",
			},
		},
		{
			name: "update characteristics",
			pool: &imsadapter.ResourcePool{
				ResourcePoolID: "pool-1",
				Extensions:     map[string]interface{}{"existing": "value"},
			},
			update: func() *models.TMF639ResourceUpdate {
				chars := []models.Characteristic{
					{Name: "new-key", Value: "new-value"},
				}
				return &models.TMF639ResourceUpdate{
					ResourceCharacteristic: &chars,
				}
			}(),
			expected: &imsadapter.ResourcePool{
				Extensions: map[string]interface{}{
					"existing": "value",
					"new-key":  "new-value",
				},
			},
		},
		{
			name: "update status fields",
			pool: &imsadapter.ResourcePool{
				ResourcePoolID: "pool-1",
				Extensions:     map[string]interface{}{},
			},
			update: func() *models.TMF639ResourceUpdate {
				status := "reserved"
				opState := "disable"
				return &models.TMF639ResourceUpdate{
					ResourceStatus:   &status,
					OperationalState: &opState,
				}
			}(),
			expected: &imsadapter.ResourcePool{
				Extensions: map[string]interface{}{
					"tmf.resourceStatus":   "reserved",
					"tmf.operationalState": "disable",
				},
			},
		},
		{
			name: "nil update fields preserve originals",
			pool: &imsadapter.ResourcePool{
				ResourcePoolID: "pool-1",
				Name:           "Original",
				Extensions:     map[string]interface{}{},
			},
			update: &models.TMF639ResourceUpdate{},
			expected: &imsadapter.ResourcePool{
				Name: "Original",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applyTMF639ResourceUpdate(tt.pool, tt.update)

			if tt.expected.Name != "" {
				assert.Equal(t, tt.expected.Name, tt.pool.Name)
			}
			if tt.expected.Description != "" {
				assert.Equal(t, tt.expected.Description, tt.pool.Description)
			}
			if tt.expected.GlobalLocationID != "" {
				assert.Equal(t, tt.expected.GlobalLocationID, tt.pool.GlobalLocationID)
			}
			for k, v := range tt.expected.Extensions {
				assert.Equal(t, v, tt.pool.Extensions[k], "extension key %s", k)
			}
		})
	}
}

func TestTransformTMF641ServiceOrderToDeployment(t *testing.T) {
	tests := []struct {
		name     string
		order    *models.TMF641ServiceOrderCreate
		item     *models.ServiceOrderItemCreate
		expected *dmsadapter.DeploymentRequest
	}{
		{
			name: "full order with service details",
			order: &models.TMF641ServiceOrderCreate{
				ExternalID:  "ext-123",
				Priority:    "1",
				Category:    "initial",
				Description: "Order description",
			},
			item: &models.ServiceOrderItemCreate{
				Action: "add",
				Service: &models.TMF638ServiceCreate{
					Name: "Order Service",
					ServiceSpecification: &models.ServiceSpecificationRef{
						ID: "pkg-1",
					},
					Place: []models.PlaceRef{
						{ID: "production"},
					},
					ServiceCharacteristic: []models.Characteristic{
						{Name: "replicas", Value: 3},
					},
				},
			},
			expected: &dmsadapter.DeploymentRequest{
				Name:        "Order Service",
				Description: "Order description",
				PackageID:   "pkg-1",
				Namespace:   "production",
			},
		},
		{
			name: "order without service spec",
			order: &models.TMF641ServiceOrderCreate{
				ExternalID:  "ext-456",
				Description: "Simple order",
			},
			item: &models.ServiceOrderItemCreate{
				Action: "add",
				Service: &models.TMF638ServiceCreate{
					Name: "Simple Service",
				},
			},
			expected: &dmsadapter.DeploymentRequest{
				Name:        "Simple Service",
				Description: "Simple order",
			},
		},
		{
			name: "order with nil service",
			order: &models.TMF641ServiceOrderCreate{
				ExternalID:  "ext-789",
				Description: "No service details",
			},
			item: &models.ServiceOrderItemCreate{
				Action:  "add",
				Service: nil,
			},
			expected: &dmsadapter.DeploymentRequest{
				Name:        "order-ext-789",
				Description: "No service details",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TransformTMF641ServiceOrderToDeployment(tt.order, tt.item)

			assert.Equal(t, tt.expected.Name, result.Name)
			assert.Equal(t, tt.expected.Description, result.Description)
			assert.Equal(t, tt.expected.PackageID, result.PackageID)
			assert.Equal(t, tt.expected.Namespace, result.Namespace)
			assert.NotNil(t, result.Values)
			assert.NotNil(t, result.Extensions)
		})
	}
}

func TestTransformDeploymentToTMF641ServiceOrder(t *testing.T) {
	now := time.Now()
	later := now.Add(time.Hour)

	tests := []struct {
		name          string
		deployment    *dmsadapter.Deployment
		baseURL       string
		expectedState string
		hasCompletion bool
		hasExternalID bool
		hasPriority   bool
		hasCategory   bool
	}{
		{
			name: "completed deployment",
			deployment: &dmsadapter.Deployment{
				ID:        "dep-1",
				Name:      "Test Deployment",
				Status:    dmsadapter.DeploymentStatusDeployed,
				CreatedAt: now,
				UpdatedAt: later,
				Extensions: map[string]interface{}{
					"orderExternalId": "ext-1",
					"orderPriority":   "1",
					"orderCategory":   "initial",
				},
			},
			baseURL:       "http://localhost",
			expectedState: "completed",
			hasCompletion: true,
			hasExternalID: true,
			hasPriority:   true,
			hasCategory:   true,
		},
		{
			name: "pending deployment",
			deployment: &dmsadapter.Deployment{
				ID:         "dep-2",
				Name:       "Pending",
				Status:     dmsadapter.DeploymentStatusPending,
				CreatedAt:  now,
				Extensions: map[string]interface{}{},
			},
			baseURL:       "http://localhost",
			expectedState: "acknowledged",
			hasCompletion: false,
		},
		{
			name: "deploying deployment",
			deployment: &dmsadapter.Deployment{
				ID:         "dep-3",
				Status:     dmsadapter.DeploymentStatusDeploying,
				CreatedAt:  now,
				Extensions: map[string]interface{}{},
			},
			baseURL:       "http://localhost",
			expectedState: stateInProgress,
		},
		{
			name: "failed deployment",
			deployment: &dmsadapter.Deployment{
				ID:         "dep-4",
				Status:     dmsadapter.DeploymentStatusFailed,
				CreatedAt:  now,
				Extensions: map[string]interface{}{},
			},
			baseURL:       "http://localhost",
			expectedState: stateFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TransformDeploymentToTMF641ServiceOrder(tt.deployment, tt.baseURL)

			assert.Equal(t, tt.deployment.ID, result.ID)
			assert.Equal(t, tt.expectedState, result.State)
			assert.NotEmpty(t, result.Href)
			assert.NotNil(t, result.OrderDate)
			assert.Equal(t, "ServiceOrder", result.AtType)
			assert.Len(t, result.ServiceOrderItem, 1)

			if tt.hasCompletion {
				assert.NotNil(t, result.CompletionDate)
			} else {
				assert.Nil(t, result.CompletionDate)
			}
			if tt.hasExternalID {
				assert.Equal(t, "ext-1", result.ExternalID)
			}
			if tt.hasPriority {
				assert.Equal(t, "1", result.Priority)
			}
			if tt.hasCategory {
				assert.Equal(t, "initial", result.Category)
			}
		})
	}
}

func TestMapDeploymentStatusToOrderState(t *testing.T) {
	tests := []struct {
		name     string
		status   dmsadapter.DeploymentStatus
		expected string
	}{
		{"pending", dmsadapter.DeploymentStatusPending, "acknowledged"},
		{"deploying", dmsadapter.DeploymentStatusDeploying, stateInProgress},
		{"deployed", dmsadapter.DeploymentStatusDeployed, "completed"},
		{"failed", dmsadapter.DeploymentStatusFailed, stateFailed},
		{"rolling back", dmsadapter.DeploymentStatusRollingBack, stateInProgress},
		{"deleting", dmsadapter.DeploymentStatusDeleting, "cancelled"},
		{"unknown", dmsadapter.DeploymentStatus("unknown"), statePending},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapDeploymentStatusToOrderState(tt.status)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMapOrderStateToDeploymentStatus(t *testing.T) {
	tests := []struct {
		name     string
		state    string
		expected dmsadapter.DeploymentStatus
	}{
		{"acknowledged", "acknowledged", dmsadapter.DeploymentStatusPending},
		{"pending", statePending, dmsadapter.DeploymentStatusPending},
		{"inProgress", stateInProgress, dmsadapter.DeploymentStatusDeploying},
		{"held", "held", dmsadapter.DeploymentStatusDeploying},
		{"completed", "completed", dmsadapter.DeploymentStatusDeployed},
		{"failed", stateFailed, dmsadapter.DeploymentStatusFailed},
		{"rejected", "rejected", dmsadapter.DeploymentStatusFailed},
		{"cancelled", "cancelled", dmsadapter.DeploymentStatusDeleting},
		{"unknown", "unknown", dmsadapter.DeploymentStatusPending},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapOrderStateToDeploymentStatus(tt.state)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestApplyTMF641ServiceOrderUpdate(t *testing.T) {
	tests := []struct {
		name   string
		dep    *dmsadapter.Deployment
		update *models.TMF641ServiceOrderUpdate
		check  func(t *testing.T, dep *dmsadapter.Deployment)
	}{
		{
			name: "update description",
			dep: &dmsadapter.Deployment{
				ID:          "dep-1",
				Description: "Original",
				Extensions:  map[string]interface{}{},
			},
			update: func() *models.TMF641ServiceOrderUpdate {
				desc := "Updated"
				return &models.TMF641ServiceOrderUpdate{Description: &desc}
			}(),
			check: func(t *testing.T, dep *dmsadapter.Deployment) {
				assert.Equal(t, "Updated", dep.Description)
			},
		},
		{
			name: "update state",
			dep: &dmsadapter.Deployment{
				ID:         "dep-1",
				Status:     dmsadapter.DeploymentStatusPending,
				Extensions: map[string]interface{}{},
			},
			update: func() *models.TMF641ServiceOrderUpdate {
				state := "completed"
				return &models.TMF641ServiceOrderUpdate{State: &state}
			}(),
			check: func(t *testing.T, dep *dmsadapter.Deployment) {
				assert.Equal(t, dmsadapter.DeploymentStatusDeployed, dep.Status)
			},
		},
		{
			name: "update external ID",
			dep: &dmsadapter.Deployment{
				ID:         "dep-1",
				Extensions: map[string]interface{}{},
			},
			update: func() *models.TMF641ServiceOrderUpdate {
				extID := "new-ext-id"
				return &models.TMF641ServiceOrderUpdate{ExternalID: &extID}
			}(),
			check: func(t *testing.T, dep *dmsadapter.Deployment) {
				assert.Equal(t, "new-ext-id", dep.Extensions["orderExternalId"])
			},
		},
		{
			name: "update priority",
			dep: &dmsadapter.Deployment{
				ID:         "dep-1",
				Extensions: map[string]interface{}{},
			},
			update: func() *models.TMF641ServiceOrderUpdate {
				priority := "1"
				return &models.TMF641ServiceOrderUpdate{Priority: &priority}
			}(),
			check: func(t *testing.T, dep *dmsadapter.Deployment) {
				assert.Equal(t, "1", dep.Extensions["orderPriority"])
			},
		},
		{
			name: "update category",
			dep: &dmsadapter.Deployment{
				ID:         "dep-1",
				Extensions: map[string]interface{}{},
			},
			update: func() *models.TMF641ServiceOrderUpdate {
				cat := "change"
				return &models.TMF641ServiceOrderUpdate{Category: &cat}
			}(),
			check: func(t *testing.T, dep *dmsadapter.Deployment) {
				assert.Equal(t, "change", dep.Extensions["orderCategory"])
			},
		},
		{
			name: "update with nil extensions initializes map",
			dep: &dmsadapter.Deployment{
				ID: "dep-1",
			},
			update: func() *models.TMF641ServiceOrderUpdate {
				extID := "ext-init"
				return &models.TMF641ServiceOrderUpdate{ExternalID: &extID}
			}(),
			check: func(t *testing.T, dep *dmsadapter.Deployment) {
				assert.NotNil(t, dep.Extensions)
				assert.Equal(t, "ext-init", dep.Extensions["orderExternalId"])
			},
		},
		{
			name: "nil update fields preserve originals",
			dep: &dmsadapter.Deployment{
				ID:          "dep-1",
				Description: "Original",
				Status:      dmsadapter.DeploymentStatusPending,
				Extensions:  map[string]interface{}{},
			},
			update: &models.TMF641ServiceOrderUpdate{},
			check: func(t *testing.T, dep *dmsadapter.Deployment) {
				assert.Equal(t, "Original", dep.Description)
				assert.Equal(t, dmsadapter.DeploymentStatusPending, dep.Status)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applyTMF641ServiceOrderUpdate(tt.dep, tt.update)
			tt.check(t, tt.dep)
		})
	}
}

func TestGenerateDeploymentName(t *testing.T) {
	tests := []struct {
		name     string
		order    *models.TMF641ServiceOrderCreate
		item     *models.ServiceOrderItemCreate
		expected string
	}{
		{
			name:  "from service name",
			order: &models.TMF641ServiceOrderCreate{},
			item: &models.ServiceOrderItemCreate{
				Service: &models.TMF638ServiceCreate{
					Name: "My Service",
				},
			},
			expected: "My Service",
		},
		{
			name: "from external ID when no service name",
			order: &models.TMF641ServiceOrderCreate{
				ExternalID: "ext-123",
			},
			item: &models.ServiceOrderItemCreate{
				Service: &models.TMF638ServiceCreate{
					Name: "",
				},
			},
			expected: "order-ext-123",
		},
		{
			name: "from external ID when nil service",
			order: &models.TMF641ServiceOrderCreate{
				ExternalID: "ext-456",
			},
			item:     &models.ServiceOrderItemCreate{},
			expected: "order-ext-456",
		},
		{
			name:  "auto-generated when no name or external ID",
			order: &models.TMF641ServiceOrderCreate{},
			item:  &models.ServiceOrderItemCreate{},
			// The name will be "order-<unix timestamp>" so we just check prefix
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateDeploymentName(tt.order, tt.item)

			if tt.expected != "" {
				assert.Equal(t, tt.expected, result)
			} else {
				// Auto-generated name should start with "order-"
				assert.Contains(t, result, "order-")
			}
		})
	}
}

func TestExtractPlaceID(t *testing.T) {
	tests := []struct {
		name     string
		places   []models.PlaceRef
		expected string
	}{
		{
			name:     "with places",
			places:   []models.PlaceRef{{ID: "loc-1"}, {ID: "loc-2"}},
			expected: "loc-1",
		},
		{
			name:     "empty places",
			places:   []models.PlaceRef{},
			expected: "",
		},
		{
			name:     "nil places",
			places:   nil,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractPlaceID(tt.places)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEnsureExtensions(t *testing.T) {
	tests := []struct {
		name string
		dep  *dmsadapter.Deployment
	}{
		{
			name: "nil extensions",
			dep:  &dmsadapter.Deployment{},
		},
		{
			name: "existing extensions",
			dep: &dmsadapter.Deployment{
				Extensions: map[string]interface{}{"key": "value"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ensureExtensions(tt.dep)
			assert.NotNil(t, tt.dep.Extensions)
		})
	}
}

func TestUpdateServiceCharacteristics(t *testing.T) {
	tests := []struct {
		name  string
		dep   *dmsadapter.Deployment
		chars *[]models.Characteristic
		check func(t *testing.T, dep *dmsadapter.Deployment)
	}{
		{
			name: "nil chars does nothing",
			dep: &dmsadapter.Deployment{
				Extensions: map[string]interface{}{"existing": "value"},
			},
			chars: nil,
			check: func(t *testing.T, dep *dmsadapter.Deployment) {
				assert.Equal(t, "value", dep.Extensions["existing"])
			},
		},
		{
			name: "updates extensions from chars",
			dep: &dmsadapter.Deployment{
				Extensions: map[string]interface{}{},
			},
			chars: &[]models.Characteristic{
				{Name: "replicas", Value: 5},
				{Name: "memory", Value: "8Gi"},
			},
			check: func(t *testing.T, dep *dmsadapter.Deployment) {
				assert.Equal(t, 5, dep.Extensions["replicas"])
				assert.Equal(t, "8Gi", dep.Extensions["memory"])
			},
		},
		{
			name: "initializes nil extensions",
			dep:  &dmsadapter.Deployment{},
			chars: &[]models.Characteristic{
				{Name: "key", Value: "val"},
			},
			check: func(t *testing.T, dep *dmsadapter.Deployment) {
				assert.NotNil(t, dep.Extensions)
				assert.Equal(t, "val", dep.Extensions["key"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updateServiceCharacteristics(tt.dep, tt.chars)
			tt.check(t, tt.dep)
		})
	}
}

// ========================================
// TMF639 Transform with full features
// ========================================

func TestTransformTMF639ResourceToResourcePool_WithAllFields(t *testing.T) {
	tmf := &models.TMF639Resource{
		ID:               "pool-complete",
		Name:             "Complete Pool",
		Description:      "Full featured pool",
		Category:         "resourcePool",
		ResourceStatus:   "available",
		OperationalState: "enable",
		UsageState:       "active",
		Place: []models.PlaceRef{
			{ID: "us-west-2"},
		},
		ResourceCharacteristic: []models.Characteristic{
			{Name: "datacenter", Value: "DC1"},
			{Name: "rack", Value: "R5"},
		},
		ResourceSpecification: &models.ResourceSpecificationRef{
			ID:      "spec-1",
			Name:    "Pool Spec",
			Version: "1.0",
		},
	}

	result := TransformTMF639ResourceToResourcePool(tmf)

	assert.Equal(t, "pool-complete", result.ResourcePoolID)
	assert.Equal(t, "Complete Pool", result.Name)
	assert.Equal(t, "Full featured pool", result.Description)
	assert.Equal(t, "us-west-2", result.GlobalLocationID)
	assert.NotNil(t, result.Extensions)
	assert.Equal(t, "DC1", result.Extensions["datacenter"])
	assert.Equal(t, "R5", result.Extensions["rack"])
	assert.Equal(t, "resourcePool", result.Extensions["tmf.category"])
	assert.Equal(t, "available", result.Extensions["tmf.resourceStatus"])
	assert.Equal(t, "enable", result.Extensions["tmf.operationalState"])
	assert.Equal(t, "active", result.Extensions["tmf.usageState"])
	assert.NotNil(t, result.Extensions["tmf.resourceSpecification"])
}

func TestTransformTMF639ResourceToResource_WithAllFields(t *testing.T) {
	tmf := &models.TMF639Resource{
		ID:               "res-complete",
		Name:             "Complete Resource",
		Description:      "Full featured resource",
		Category:         "compute",
		ResourceStatus:   "standby",
		OperationalState: "disable",
		UsageState:       "busy",
		Place: []models.PlaceRef{
			{ID: "rack-7"},
		},
		ResourceCharacteristic: []models.Characteristic{
			{Name: "cpu_cores", Value: 64},
		},
		ResourceSpecification: &models.ResourceSpecificationRef{
			ID:      "spec-compute-1",
			Name:    "Compute Spec",
			Version: "2.0",
		},
	}

	result := TransformTMF639ResourceToResource(tmf)

	assert.Equal(t, "res-complete", result.ResourceID)
	assert.Equal(t, "Full featured resource", result.Description)
	assert.Equal(t, "compute", result.ResourceTypeID)
	assert.NotNil(t, result.Extensions)
	assert.Equal(t, "Complete Resource", result.Extensions["name"])
	assert.Equal(t, "rack-7", result.Extensions["location"])
	assert.Equal(t, 64, result.Extensions["cpu_cores"])
	assert.Equal(t, "compute", result.Extensions["tmf.category"])
	assert.Equal(t, "standby", result.Extensions["tmf.resourceStatus"])
	assert.Equal(t, "disable", result.Extensions["tmf.operationalState"])
	assert.Equal(t, "busy", result.Extensions["tmf.usageState"])
	assert.NotNil(t, result.Extensions["tmf.resourceSpecification"])
}

// ========================================
// TMF638 Service Transform with extensions
// ========================================

func TestTransformDeploymentToTMF638Service_WithExtensions(t *testing.T) {
	now := time.Now()
	dep := &dmsadapter.Deployment{
		ID:          "dep-ext",
		Name:        "Extended Service",
		Description: "Service with extensions",
		PackageID:   "pkg-1",
		Status:      dmsadapter.DeploymentStatusDeployed,
		Namespace:   "production",
		CreatedAt:   now,
		Extensions: map[string]interface{}{
			"serviceType": "CNF",
			"replicas":    3,
			"custom":      "value",
		},
	}

	result := TransformDeploymentToTMF638Service(dep, "http://localhost")

	assert.Equal(t, "dep-ext", result.ID)
	assert.Equal(t, "Extended Service", result.Name)
	assert.Equal(t, tmfServiceStateActive, result.State)
	assert.Equal(t, "CNF", result.ServiceType)
	assert.NotNil(t, result.ServiceSpecification)
	assert.Equal(t, "pkg-1", result.ServiceSpecification.ID)
	assert.Len(t, result.Place, 1)
	assert.Equal(t, "production", result.Place[0].ID)
	assert.NotNil(t, result.StartDate)
	assert.NotNil(t, result.ServiceDate)
	assert.Equal(t, "Service", result.AtType)
	// Characteristics should include custom extensions but not serviceType
	foundCustom := false
	for _, ch := range result.ServiceCharacteristic {
		if ch.Name == "custom" {
			foundCustom = true
		}
		assert.NotEqual(t, "serviceType", ch.Name, "serviceType should be extracted, not in characteristics")
	}
	assert.True(t, foundCustom, "custom extension should be in characteristics")
}

func TestTransformTMF638ServiceToDeployment_WithServiceType(t *testing.T) {
	tmf := &models.TMF638ServiceCreate{
		Name:        "Typed Service",
		Description: "Service with type",
		ServiceType: "CNF",
		ServiceSpecification: &models.ServiceSpecificationRef{
			ID: "pkg-typed",
		},
		Place: []models.PlaceRef{
			{ID: "ns-typed"},
		},
		ServiceCharacteristic: []models.Characteristic{
			{Name: "replicas", Value: 5},
		},
	}

	result := TransformTMF638ServiceToDeployment(tmf)

	assert.Equal(t, "Typed Service", result.Name)
	assert.Equal(t, "Service with type", result.Description)
	assert.Equal(t, "pkg-typed", result.PackageID)
	assert.Equal(t, "ns-typed", result.Namespace)
	assert.Equal(t, 5, result.Values["replicas"])
	assert.Equal(t, "CNF", result.Values["serviceType"])
}
