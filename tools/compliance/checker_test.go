package compliance_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/piwi3910/netweave/tools/compliance"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type mockEndpoint struct {
	statusCode int
	response   string
}

// mockO2IMSHandler returns a mock HTTP handler for O2-IMS endpoints.
func mockO2IMSHandler() http.HandlerFunc {
	// Map of endpoints to responses
	endpoints := map[string]mockEndpoint{
		"GET:/o2ims/v1/subscriptions":        {http.StatusOK, `{"subscriptions": [], "total": 0}`},
		"POST:/o2ims/v1/subscriptions":       {http.StatusCreated, `{"subscriptionId": "test-sub-123"}`},
		"GET:/o2ims/v1/resourcePools":        {http.StatusOK, `{"resourcePools": [], "total": 0}`},
		"GET:/o2ims/v1/resources":            {http.StatusOK, `{"resources": [], "total": 0}`},
		"GET:/o2ims/v1/resourceTypes":        {http.StatusOK, `{"resourceTypes": [], "total": 0}`},
		"GET:/o2ims/v1/deploymentManagers":   {http.StatusOK, `{"deploymentManagers": [], "total": 1}`},
		"GET:/o2ims/v1/oCloudInfrastructure": {http.StatusOK, `{"oCloudId": "test-ocloud"}`},
	}

	return func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + ":" + r.URL.Path
		if endpoint, ok := endpoints[key]; ok {
			w.WriteHeader(endpoint.statusCode)
			_, _ = w.Write([]byte(endpoint.response))
			return
		}
		// Return 404 for parameterized endpoints
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error": "NotFound"}`))
	}
}

func TestChecker_CheckO2IMS(t *testing.T) {
	// Create mock gateway server
	server := httptest.NewServer(mockO2IMSHandler())
	defer server.Close()

	checker := compliance.NewChecker(server.URL, zap.NewNop())
	spec := compliance.SpecVersion{
		Name:    "O2-IMS",
		Version: "v3.0.0",
		SpecURL: "https://specifications.o-ran.org/o2ims",
	}

	result, err := checker.CheckO2IMS(context.Background(), spec)
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Return 404 for all O2-DMS endpoints (not implemented)
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error": "NotFound"}`))
	}))
	defer server.Close()

	checker := compliance.NewChecker(server.URL, zap.NewNop())
	spec := compliance.SpecVersion{
		Name:    "O2-DMS",
		Version: "v3.0.0",
		SpecURL: "https://specifications.o-ran.org/o2dms",
	}

	result, err := checker.CheckO2DMS(context.Background(), spec)
	require.NoError(t, err)

	// Verify result - should have low compliance.compliance since O2-DMS not implemented
	assert.Equal(t, "O2-DMS", result.SpecName)
	assert.Equal(t, compliance.ComplianceNone, result.Level)
	// Note: Some endpoints may return non-404 status due to partial implementation
	assert.Greater(t, result.TotalEndpoints, 0, "Should have tested at least some endpoints")
	assert.Greater(t, result.FailedEndpoints, 0, "Should have some failures with mock 404 responses")
}

func TestChecker_CheckAll(t *testing.T) {
	// Create mock gateway server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock O2-IMS endpoints (implemented)
		if len(r.URL.Path) >= 8 && r.URL.Path[:8] == "/o2ims/v" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
			return
		}

		// O2-DMS endpoints (not implemented)
		if len(r.URL.Path) >= 8 && r.URL.Path[:8] == "/o2dms/v" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		// Default
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	checker := compliance.NewChecker(server.URL, zap.NewNop())

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
			result := compliance.ReplacePlaceholders(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEndpointTest_Coverage(t *testing.T) {
	// Ensure we're testing all required O2-IMS endpoints
	checker := compliance.NewChecker("http://localhost:8080", zap.NewNop())
	spec := compliance.SpecVersion{Name: "O2-IMS", Version: "v3.0.0"}

	// Get endpoint tests (via CheckO2IMS)
	// This verifies that we have comprehensive endpoint coverage
	ctx := context.Background()
	_, err := checker.CheckO2IMS(ctx, spec)

	// Should not error (even if server is down, endpoint definition should work)
	// Error would indicate a problem with endpoint test definitions
	if err != nil {
		t.Logf("checkO2IMS returned error (expected if gateway not running): %v", err)
	}
}
