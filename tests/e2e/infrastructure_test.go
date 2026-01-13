// Package e2e provides end-to-end tests for the O2-IMS Gateway.
// These tests verify complete user workflows against a real Kubernetes cluster.
//
//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// doHTTPGet performs an HTTP GET request and unmarshals the JSON response.
func doHTTPGet(t *testing.T, fw *TestFramework, url string, result any) int {
	t.Helper()
	req, err := http.NewRequestWithContext(fw.Context, http.MethodGet, url, nil)
	require.NoError(t, err)

	resp, err := fw.APIClient.Do(req)
	require.NoError(t, err)
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Logf("Failed to close response body: %v", err)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	if result != nil {
		err = json.Unmarshal(body, result)
		require.NoError(t, err)
	}

	return resp.StatusCode
}

// getFirstPoolID retrieves the first available resource pool ID.
func getFirstPoolID(t *testing.T, fw *TestFramework) string {
	t.Helper()
	var pools []map[string]any
	doHTTPGet(t, fw, fw.GatewayURL+APIPathResourcePools, &pools)

	if len(pools) == 0 {
		t.Skip("No resource pools available")
	}

	poolID, ok := pools[0]["resourcePoolId"].(string)
	require.True(t, ok, "resourcePoolId is not a string")
	require.NotEmpty(t, poolID, "resourcePoolId is empty")
	return poolID
}

// getFirstResourceID retrieves the first available resource ID from a pool.
func getFirstResourceID(t *testing.T, fw *TestFramework, poolID string) string {
	t.Helper()
	var resources []map[string]any
	url := fw.GatewayURL + fmt.Sprintf(APIPathResourcesInPool, poolID)
	doHTTPGet(t, fw, url, &resources)

	if len(resources) == 0 {
		t.Skip("No resources available for testing")
	}

	resourceID, ok := resources[0]["resourceId"].(string)
	require.True(t, ok, "resourceId is not a string")
	require.NotEmpty(t, resourceID, "resourceId is empty")
	return resourceID
}

// TestInfrastructureDiscovery tests the complete infrastructure discovery workflow.
// This verifies that an SMO can discover resource pools and resources.
func TestInfrastructureDiscovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	fw, err := NewTestFramework(DefaultOptions())
	require.NoError(t, err, "Failed to create test framework")
	defer fw.Cleanup()

	t.Run("list resource pools", func(t *testing.T) {
		var pools []map[string]any
		statusCode := doHTTPGet(t, fw, fw.GatewayURL+APIPathResourcePools, &pools)

		assert.Equal(t, http.StatusOK, statusCode, "Expected 200 OK")
		assert.NotEmpty(t, pools, "Expected at least one resource pool")

		if len(pools) > 0 {
			pool := pools[0]
			assert.Contains(t, pool, "resourcePoolId")
			assert.Contains(t, pool, "name")
			assert.Contains(t, pool, "oCloudId")
		}

		fw.Logger.Info("Successfully listed resource pools", zap.Int("count", len(pools)))
	})

	t.Run("get specific resource pool", func(t *testing.T) {
		poolID := getFirstPoolID(t, fw)

		url := fw.GatewayURL + fmt.Sprintf(APIPathResourcePoolByID, poolID)
		var pool map[string]any
		statusCode := doHTTPGet(t, fw, url, &pool)
		assert.Equal(t, http.StatusOK, statusCode)
		assert.Equal(t, poolID, pool["resourcePoolId"])
		assert.Contains(t, pool, "name")
		assert.Contains(t, pool, "description")

		fw.Logger.Info("Successfully retrieved resource pool", zap.String("poolId", poolID), zap.Any("name", pool["name"]))
	})

	t.Run("list resources in pool", func(t *testing.T) {
		poolID := getFirstPoolID(t, fw)

		url := fw.GatewayURL + fmt.Sprintf(APIPathResourcesInPool, poolID)
		var resources []map[string]any
		statusCode := doHTTPGet(t, fw, url, &resources)
		assert.Equal(t, http.StatusOK, statusCode)
		fw.Logger.Info("Listed resources in pool", zap.String("poolId", poolID), zap.Int("count", len(resources)))

		if len(resources) > 0 {
			resource := resources[0]
			assert.Contains(t, resource, "resourceId")
			assert.Contains(t, resource, "resourceType")
			assert.Contains(t, resource, "resourcePoolId")
		}
	})

	t.Run("get specific resource", func(t *testing.T) {
		poolID := getFirstPoolID(t, fw)
		resourceID := getFirstResourceID(t, fw, poolID)

		url := fw.GatewayURL + fmt.Sprintf(APIPathResourceByID, poolID, resourceID)
		var resource map[string]any
		statusCode := doHTTPGet(t, fw, url, &resource)
		assert.Equal(t, http.StatusOK, statusCode)
		assert.Equal(t, resourceID, resource["resourceId"])
		assert.Equal(t, poolID, resource["resourcePoolId"])
		assert.Contains(t, resource, "resourceType")

		fw.Logger.Info("Successfully retrieved resource", zap.String("poolId", poolID), zap.String("resourceId", resourceID), zap.Any("resourceType", resource["resourceType"]))
	})

	t.Run("filter resources by type", func(t *testing.T) {
		poolID := getFirstPoolID(t, fw)

		url := fw.GatewayURL + fmt.Sprintf(APIPathResourcesInPool, poolID) + "?filter=resourceType==Node"
		var resources []map[string]any
		statusCode := doHTTPGet(t, fw, url, &resources)
		assert.Equal(t, http.StatusOK, statusCode)
		for _, resource := range resources {
			assert.Equal(t, "Node", resource["resourceType"])
		}

		fw.Logger.Info("Successfully filtered resources by type", zap.String("poolId", poolID), zap.String("filter", "resourceType==Node"), zap.Int("count", len(resources)))
	})

	t.Run("pagination support", func(t *testing.T) {
		poolID := getFirstPoolID(t, fw)

		url := fw.GatewayURL + fmt.Sprintf(APIPathResourcesInPool, poolID) + "?limit=5"
		var resources []map[string]any
		statusCode := doHTTPGet(t, fw, url, &resources)
		assert.Equal(t, http.StatusOK, statusCode)
		assert.LessOrEqual(t, len(resources), 5)

		fw.Logger.Info("Successfully tested pagination", zap.String("poolId", poolID), zap.Int("limit", 5), zap.Int("returned", len(resources)))
	})
}

// TestErrorHandling tests error responses from the API.
func TestErrorHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E test in short mode")
	}

	fw, err := NewTestFramework(DefaultOptions())
	require.NoError(t, err)
	defer fw.Cleanup()

	t.Run("get non-existent pool", func(t *testing.T) {
		url := fw.GatewayURL + fmt.Sprintf(APIPathResourcePoolByID, "non-existent-pool-id")
		var errorResp map[string]any
		statusCode := doHTTPGet(t, fw, url, &errorResp)
		assert.Equal(t, http.StatusNotFound, statusCode)
		assert.Contains(t, errorResp, "error")

		fw.Logger.Info("Successfully handled non-existent resource pool")
	})

	t.Run("invalid filter syntax", func(t *testing.T) {
		url := fw.GatewayURL + APIPathResourcePools + "?filter=invalid syntax here"
		statusCode := doHTTPGet(t, fw, url, nil)
		assert.True(t, statusCode >= 400, "Expected error status code")
		fw.Logger.Info("Successfully handled invalid filter syntax", zap.Int("statusCode", statusCode))
	})
}
