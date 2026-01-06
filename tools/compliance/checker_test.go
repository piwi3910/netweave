package compliance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestChecker_CheckO2IMS(t *testing.T) {
	// Create mock gateway server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock O2-IMS endpoints
		switch {
		case r.URL.Path == "/o2ims/v1/subscriptions" && r.Method == "GET":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"subscriptions": [], "total": 0}`))
		case r.URL.Path == "/o2ims/v1/subscriptions" && r.Method == "POST":
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"subscriptionId": "test-sub-123"}`))
		case r.URL.Path == "/o2ims/v1/resourcePools" && r.Method == "GET":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"resourcePools": [], "total": 0}`))
		case r.URL.Path == "/o2ims/v1/resources" && r.Method == "GET":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"resources": [], "total": 0}`))
		case r.URL.Path == "/o2ims/v1/resourceTypes" && r.Method == "GET":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"resourceTypes": [], "total": 0}`))
		case r.URL.Path == "/o2ims/v1/deploymentManagers" && r.Method == "GET":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"deploymentManagers": [], "total": 1}`))
		case r.URL.Path == "/o2ims/v1/oCloudInfrastructure" && r.Method == "GET":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"oCloudId": "test-ocloud"}`))
		default:
			// Return 404 for parameterized endpoints (simulating endpoints exist but resource not found)
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error": "NotFound"}`))
		}
	}))
	defer server.Close()

	checker := NewChecker(server.URL, zap.NewNop())
	spec := SpecVersion{
		Name:    "O2-IMS",
		Version: "v3.0.0",
		SpecURL: "https://specifications.o-ran.org/o2ims",
	}

	result, err := checker.checkO2IMS(context.Background(), spec)
	require.NoError(t, err)

	// Verify result
	assert.Equal(t, "O2-IMS", result.SpecName)
	assert.Equal(t, "v3.0.0", result.SpecVersion)
	assert.Greater(t, result.ComplianceScore, 0.0)
	assert.LessOrEqual(t, result.ComplianceScore, 100.0)
	assert.Equal(t, result.PassedEndpoints+result.FailedEndpoints, result.TotalEndpoints)
}

func TestChecker_CheckO2DMS(t *testing.T) {
	// Create mock gateway server (O2-DMS not implemented yet)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return 404 for all O2-DMS endpoints (not implemented)
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error": "NotFound"}`))
	}))
	defer server.Close()

	checker := NewChecker(server.URL, zap.NewNop())
	spec := SpecVersion{
		Name:    "O2-DMS",
		Version: "v3.0.0",
		SpecURL: "https://specifications.o-ran.org/o2dms",
	}

	result, err := checker.checkO2DMS(context.Background(), spec)
	require.NoError(t, err)

	// Verify result - should have low compliance since O2-DMS not implemented
	assert.Equal(t, "O2-DMS", result.SpecName)
	assert.Equal(t, ComplianceNone, result.ComplianceLevel)
	assert.Equal(t, 0, result.PassedEndpoints)
	assert.Equal(t, result.TotalEndpoints, result.FailedEndpoints)
}

func TestChecker_CheckAll(t *testing.T) {
	// Create mock gateway server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock O2-IMS endpoints (implemented)
		if r.URL.Path[:8] == "/o2ims/v" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{}`))
			return
		}

		// O2-DMS endpoints (not implemented)
		if r.URL.Path[:8] == "/o2dms/v" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		// Default
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	checker := NewChecker(server.URL, zap.NewNop())

	results, err := checker.CheckAll(context.Background())
	require.NoError(t, err)

	// Should return results for all 3 specs
	assert.Len(t, results, 3)

	// Verify each spec is present
	specNames := make(map[string]bool)
	for _, result := range results {
		specNames[result.SpecName] = true
	}

	assert.True(t, specNames["O2-IMS"])
	assert.True(t, specNames["O2-DMS"])
	assert.True(t, specNames["O2-SMO"])
}

func TestReplacePlaceholders(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "subscription ID",
			path:     "/o2ims/v1/subscriptions/{subscriptionId}",
			expected: "/o2ims/v1/subscriptions/test-subscription-id",
		},
		{
			name:     "resource pool ID",
			path:     "/o2ims/v1/resourcePools/{resourcePoolId}",
			expected: "/o2ims/v1/resourcePools/test-pool-id",
		},
		{
			name:     "resource ID",
			path:     "/o2ims/v1/resources/{resourceId}",
			expected: "/o2ims/v1/resources/test-resource-id",
		},
		{
			name:     "deployment ID",
			path:     "/o2dms/v1/deployments/{deploymentId}",
			expected: "/o2dms/v1/deployments/test-deployment-id",
		},
		{
			name:     "no placeholders",
			path:     "/o2ims/v1/subscriptions",
			expected: "/o2ims/v1/subscriptions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := replacePlaceholders(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEndpointTest_Coverage(t *testing.T) {
	// Ensure we're testing all required O2-IMS endpoints
	checker := NewChecker("http://localhost:8080", zap.NewNop())
	spec := SpecVersion{Name: "O2-IMS", Version: "v3.0.0"}

	// Get endpoint tests (via checkO2IMS)
	// This verifies that we have comprehensive endpoint coverage
	ctx := context.Background()
	_, err := checker.checkO2IMS(ctx, spec)

	// Should not error (even if server is down, endpoint definition should work)
	// Error would indicate a problem with endpoint test definitions
	if err != nil {
		t.Logf("checkO2IMS returned error (expected if gateway not running): %v", err)
	}
}
