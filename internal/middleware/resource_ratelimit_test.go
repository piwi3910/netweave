package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestExtractResourceType(t *testing.T) {
	tests := []struct {
		path     string
		expected ResourceType
	}{
		{"/o2ims/v1/deploymentManagers", ResourceTypeDeploymentManagers},
		{"/o2ims/v1/deploymentManagers/dm-123", ResourceTypeDeploymentManagers},
		{"/o2ims/v1/resourcePools", ResourceTypeResourcePools},
		{"/o2ims/v1/resourcePools/pool-123", ResourceTypeResourcePools},
		{"/o2ims/v1/resources", ResourceTypeResources},
		{"/o2ims/v1/resources/res-123", ResourceTypeResources},
		{"/o2ims/v1/resourceTypes", ResourceTypeResourceTypes},
		{"/o2ims/v1/resourceTypes/type-123", ResourceTypeResourceTypes},
		{"/o2ims/v1/subscriptions", ResourceTypeSubscriptions},
		{"/o2ims/v1/subscriptions/sub-123", ResourceTypeSubscriptions},
		{"/o2ims-dms/v1/deploymentManagers", ResourceTypeDeploymentManagers},
		{"/o2ims/v1/dm-123/resourcePools", ResourceTypeResourcePools},
		{"/o2ims/v1/dm-123/resources", ResourceTypeResources},
		{"/o2ims/v1/unknown-endpoint", ResourceTypeUnknown},
		{"/health", ResourceTypeUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := extractResourceType(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractOperation(t *testing.T) {
	tests := []struct {
		method   string
		path     string
		expected OperationType
	}{
		{http.MethodGet, "/o2ims/v1/deploymentManagers", OperationList},
		{http.MethodGet, "/o2ims/v1/deploymentManagers/dm-123", OperationRead},
		{http.MethodGet, "/o2ims/v1/resourcePools", OperationList},
		{http.MethodGet, "/o2ims/v1/resourcePools/pool-123", OperationRead},
		{http.MethodGet, "/o2ims/v1/subscriptions", OperationList},
		{http.MethodGet, "/o2ims/v1/subscriptions/sub-123", OperationRead},
		{http.MethodPost, "/o2ims/v1/subscriptions", OperationWrite},
		{http.MethodPut, "/o2ims/v1/subscriptions/sub-123", OperationWrite},
		{http.MethodPatch, "/o2ims/v1/resources/res-123", OperationWrite},
		{http.MethodDelete, "/o2ims/v1/subscriptions/sub-123", OperationDelete},
	}

	for _, tt := range tests {
		t.Run(tt.method+"_"+tt.path, func(t *testing.T) {
			result := extractOperation(tt.method, tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsCollectionPath(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"/o2ims/v1/deploymentManagers", true},
		{"/o2ims/v1/deploymentManagers/dm-123", false},
		{"/o2ims/v1/resourcePools", true},
		{"/o2ims/v1/resourcePools/pool-123", false},
		{"/o2ims/v1/resources", true},
		{"/o2ims/v1/resources/res-123", false},
		{"/o2ims/v1/subscriptions", true},
		{"/o2ims/v1/subscriptions/sub-123", false},
		{"/health", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := isCollectionPath(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDefaultResourceRateLimitConfig(t *testing.T) {
	config := DefaultResourceRateLimitConfig()

	assert.True(t, config.Enabled)

	// DeploymentManagers limits
	assert.Equal(t, 100, config.DeploymentManagers.ReadsPerMinute)
	assert.Equal(t, 10, config.DeploymentManagers.WritesPerMinute)
	assert.Equal(t, 100, config.DeploymentManagers.ListPageSizeMax)

	// ResourcePools limits
	assert.Equal(t, 500, config.ResourcePools.ReadsPerMinute)
	assert.Equal(t, 50, config.ResourcePools.WritesPerMinute)
	assert.Equal(t, 100, config.ResourcePools.ListPageSizeMax)

	// Resources limits
	assert.Equal(t, 1000, config.Resources.ReadsPerMinute)
	assert.Equal(t, 100, config.Resources.WritesPerMinute)
	assert.Equal(t, 100, config.Resources.ListPageSizeMax)

	// Subscriptions limits
	assert.Equal(t, 100, config.Subscriptions.CreatesPerHour)
	assert.Equal(t, 50, config.Subscriptions.MaxActive)
	assert.Equal(t, 200, config.Subscriptions.ReadsPerMinute)
}

func TestGetResourceTenantID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name          string
		setupContext  func(*gin.Context)
		expectedID    string
		shouldContain string
	}{
		{
			name: "tenant from context",
			setupContext: func(c *gin.Context) {
				c.Set("tenant_id", "tenant-123")
			},
			expectedID: "tenant-123",
		},
		{
			name: "tenant from header",
			setupContext: func(c *gin.Context) {
				c.Request.Header.Set("X-Tenant-ID", "tenant-456")
			},
			expectedID: "tenant-456",
		},
		{
			name: "fallback to client IP",
			setupContext: func(_ *gin.Context) {
				// No tenant set
			},
			shouldContain: ".", // IP address contains dots
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)

			tt.setupContext(c)

			result := getResourceTenantID(c)

			if tt.expectedID != "" {
				assert.Equal(t, tt.expectedID, result)
			}
			if tt.shouldContain != "" {
				assert.Contains(t, result, tt.shouldContain)
			}
		})
	}
}

func TestResourceRateLimiter_GetMaxPageSize(t *testing.T) {
	config := DefaultResourceRateLimitConfig()
	rl := &ResourceRateLimiter{config: config}

	tests := []struct {
		resourceType ResourceType
		expected     int
	}{
		{ResourceTypeDeploymentManagers, 100},
		{ResourceTypeResourcePools, 100},
		{ResourceTypeResources, 100},
		{ResourceTypeResourceTypes, 100},
		{ResourceTypeSubscriptions, 100},
		{ResourceTypeUnknown, 100},
	}

	for _, tt := range tests {
		t.Run(string(tt.resourceType), func(t *testing.T) {
			result := rl.getMaxPageSize(tt.resourceType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestResourceRateLimiter_CheckPageSize(t *testing.T) {
	gin.SetMode(gin.TestMode)

	config := DefaultResourceRateLimitConfig()
	rl := &ResourceRateLimiter{config: config}

	tests := []struct {
		name         string
		query        string
		resourceType ResourceType
		expected     bool
	}{
		{
			name:         "no limit param",
			query:        "",
			resourceType: ResourceTypeResources,
			expected:     true,
		},
		{
			name:         "limit within max",
			query:        "limit=50",
			resourceType: ResourceTypeResources,
			expected:     true,
		},
		{
			name:         "limit at max",
			query:        "limit=100",
			resourceType: ResourceTypeResources,
			expected:     true,
		},
		{
			name:         "limit exceeds max",
			query:        "limit=200",
			resourceType: ResourceTypeResources,
			expected:     false,
		},
		{
			name:         "invalid limit",
			query:        "limit=abc",
			resourceType: ResourceTypeResources,
			expected:     true, // Let other validation handle
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			url := "/o2ims/v1/resources"
			if tt.query != "" {
				url += "?" + tt.query
			}
			c.Request = httptest.NewRequest(http.MethodGet, url, nil)

			result := rl.checkPageSize(c, tt.resourceType)
			assert.Equal(t, tt.expected, result)

			if !tt.expected {
				assert.Equal(t, http.StatusBadRequest, w.Code)
			}
		})
	}
}
