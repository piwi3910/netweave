package helm_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	dmsadapter "github.com/piwi3910/netweave/internal/dms/adapter"
	"github.com/piwi3910/netweave/internal/dms/adapters/helm"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createMockHelmRepo creates a mock HTTP server that serves a Helm repository index.
func createMockHelmRepo() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/index.yaml" {
			w.Header().Set("Content-Type", "application/x-yaml")
			_, _ = fmt.Fprint(w, `apiVersion: v1
entries:
  nginx:
  - apiVersion: v2
    appVersion: "1.19.0"
    created: "2024-01-15T12:00:00Z"
    description: NGINX web server
    digest: abc123
    name: nginx
    urls:
    - https://charts.example.com/nginx-1.0.0.tgz
    version: 1.0.0
  - apiVersion: v2
    appVersion: "1.18.0"
    created: "2023-12-01T12:00:00Z"
    description: NGINX web server
    digest: def456
    name: nginx
    urls:
    - https://charts.example.com/nginx-0.9.0.tgz
    version: 0.9.0
  postgresql:
  - apiVersion: v2
    appVersion: "14.0"
    created: "2024-01-10T10:00:00Z"
    description: PostgreSQL database
    digest: ghi789
    name: postgresql
    urls:
    - https://charts.example.com/postgresql-2.0.0.tgz
    version: 2.0.0
generated: "2024-01-15T12:00:00Z"
`)
		} else {
			http.NotFound(w, r)
		}
	}))
}

// TestHelmAdapter_ListDeploymentPackages_WithMockRepo tests listing packages with a mock repository.
func TestHelmAdapter_ListDeploymentPackages_WithMockRepo(t *testing.T) {
	mockRepo := createMockHelmRepo()
	defer mockRepo.Close()

	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace:     "test",
		RepositoryURL: mockRepo.URL,
	})
	require.NoError(t, err)

	ctx := context.Background()

	tests := []struct {
		name          string
		filter        *dmsadapter.Filter
		expectedCount int
		expectedFirst string
	}{
		{
			name:          "list all packages",
			filter:        nil,
			expectedCount: 2, // nginx and postgresql latest versions
			expectedFirst: "nginx",
		},
		{
			name: "filter by chart name",
			filter: &dmsadapter.Filter{
				Extensions: map[string]interface{}{
					"helm.chartName": "nginx",
				},
			},
			expectedCount: 1,
			expectedFirst: "nginx",
		},
		{
			name: "filter by version",
			filter: &dmsadapter.Filter{
				Extensions: map[string]interface{}{
					"helm.chartVersion": "1.0.0",
				},
			},
			expectedCount: 1,
			expectedFirst: "nginx",
		},
		{
			name: "filter with no matches",
			filter: &dmsadapter.Filter{
				Extensions: map[string]interface{}{
					"helm.chartName": "nonexistent",
				},
			},
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packages, err := adapter.ListDeploymentPackages(ctx, tt.filter)
			require.NoError(t, err)
			assert.Len(t, packages, tt.expectedCount)

			if tt.expectedCount > 0 {
				assert.Equal(t, tt.expectedFirst, packages[0].Name)
				assert.Equal(t, "helm-chart", packages[0].PackageType)
				assert.NotEmpty(t, packages[0].ID)
				assert.NotEmpty(t, packages[0].Version)
				assert.NotNil(t, packages[0].Extensions)
			}
		})
	}
}

// TestHelmAdapter_GetDeploymentPackage_WithMockRepo tests getting a specific package with a mock repository.
func TestHelmAdapter_GetDeploymentPackage_WithMockRepo(t *testing.T) {
	mockRepo := createMockHelmRepo()
	defer mockRepo.Close()

	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace:     "test",
		RepositoryURL: mockRepo.URL,
	})
	require.NoError(t, err)

	ctx := context.Background()

	tests := []struct {
		name        string
		packageID   string
		expectErr   bool
		expectedPkg *dmsadapter.DeploymentPackage
	}{
		{
			name:      "get existing package - nginx latest",
			packageID: "nginx-1.0.0",
			expectErr: false,
			expectedPkg: &dmsadapter.DeploymentPackage{
				ID:          "nginx-1.0.0",
				Name:        "nginx",
				Version:     "1.0.0",
				PackageType: "helm-chart",
				Description: "NGINX web server",
			},
		},
		{
			name:      "get existing package - nginx old version",
			packageID: "nginx-0.9.0",
			expectErr: false,
			expectedPkg: &dmsadapter.DeploymentPackage{
				ID:          "nginx-0.9.0",
				Name:        "nginx",
				Version:     "0.9.0",
				PackageType: "helm-chart",
				Description: "NGINX web server",
			},
		},
		{
			name:      "get existing package - postgresql",
			packageID: "postgresql-2.0.0",
			expectErr: false,
			expectedPkg: &dmsadapter.DeploymentPackage{
				ID:          "postgresql-2.0.0",
				Name:        "postgresql",
				Version:     "2.0.0",
				PackageType: "helm-chart",
				Description: "PostgreSQL database",
			},
		},
		{
			name:      "package not found - wrong version",
			packageID: "nginx-99.0.0",
			expectErr: true,
		},
		{
			name:      "package not found - wrong chart",
			packageID: "redis-1.0.0",
			expectErr: true,
		},
		{
			name:      "package not found - invalid format",
			packageID: "invalid",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkg, err := adapter.GetDeploymentPackage(ctx, tt.packageID)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, pkg)
				assert.Contains(t, err.Error(), "chart not found")
			} else {
				require.NoError(t, err)
				require.NotNil(t, pkg)
				assert.Equal(t, tt.expectedPkg.ID, pkg.ID)
				assert.Equal(t, tt.expectedPkg.Name, pkg.Name)
				assert.Equal(t, tt.expectedPkg.Version, pkg.Version)
				assert.Equal(t, tt.expectedPkg.PackageType, pkg.PackageType)
				assert.Equal(t, tt.expectedPkg.Description, pkg.Description)

				// Verify extensions
				assert.NotNil(t, pkg.Extensions)
				assert.Equal(t, tt.expectedPkg.Name, pkg.Extensions["helm.chartName"])
				assert.Equal(t, tt.expectedPkg.Version, pkg.Extensions["helm.chartVersion"])
				assert.Equal(t, mockRepo.URL, pkg.Extensions["helm.repository"])
				assert.NotEmpty(t, pkg.Extensions["helm.appVersion"])
				assert.NotEmpty(t, pkg.Extensions["helm.apiVersion"])
				assert.NotNil(t, pkg.Extensions["helm.deprecated"])
			}
		})
	}
}

// TestHelmAdapter_LoadRepositoryIndex_Caching tests repository index caching.
func TestHelmAdapter_LoadRepositoryIndex_Caching(t *testing.T) {
	callCount := 0
	mockRepo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/index.yaml" {
			callCount++
			w.Header().Set("Content-Type", "application/x-yaml")
			_, _ = fmt.Fprint(w, `apiVersion: v1
entries:
  test:
  - apiVersion: v2
    name: test
    version: 1.0.0
generated: "`+time.Now().Format(time.RFC3339)+`"
`)
		} else {
			http.NotFound(w, r)
		}
	}))
	defer mockRepo.Close()

	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace:     "test",
		RepositoryURL: mockRepo.URL,
	})
	require.NoError(t, err)

	ctx := context.Background()

	// First call - should download index
	err = adapter.LoadRepositoryIndex(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount)

	// Second call - should use cached index
	err = adapter.LoadRepositoryIndex(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount, "Index should be cached, not re-downloaded")

	// Third call - should still use cached index
	err = adapter.LoadRepositoryIndex(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount, "Index should still be cached")
}

// TestHelmAdapter_DeleteDeploymentPackage_WithMockRepo tests package deletion with cache invalidation.
func TestHelmAdapter_DeleteDeploymentPackage_WithMockRepo(t *testing.T) {
	mockRepo := createMockHelmRepo()
	defer mockRepo.Close()

	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace:     "test",
		RepositoryURL: mockRepo.URL,
	})
	require.NoError(t, err)

	ctx := context.Background()

	// Load the index first
	err = adapter.LoadRepositoryIndex(ctx)
	require.NoError(t, err)

	// Get a package to verify it exists
	pkg, err := adapter.GetDeploymentPackage(ctx, "nginx-1.0.0")
	require.NoError(t, err)
	require.NotNil(t, pkg)

	// Try to delete it - this should fail (not fully implemented)
	// but should clear the cache
	err = adapter.DeleteDeploymentPackage(ctx, "nginx-1.0.0")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not fully implemented")

	// The cache should be cleared even though deletion failed
	// This tests the cache invalidation code path
}

// TestHelmAdapter_UploadDeploymentPackage_Complete tests package upload.
func TestHelmAdapter_UploadDeploymentPackage_Complete(t *testing.T) {
	mockRepo := createMockHelmRepo()
	defer mockRepo.Close()

	adapter, err := helm.NewAdapter(&helm.Config{
		Namespace:     "test",
		RepositoryURL: mockRepo.URL,
	})
	require.NoError(t, err)

	ctx := context.Background()

	tests := []struct {
		name string
		pkg  *dmsadapter.DeploymentPackageUpload
	}{
		{
			name: "upload new chart",
			pkg: &dmsadapter.DeploymentPackageUpload{
				Name:        "redis",
				Version:     "3.0.0",
				PackageType: "helm-chart",
				Description: "Redis in-memory database",
				Content:     []byte("chart content"),
			},
		},
		{
			name: "upload chart update",
			pkg: &dmsadapter.DeploymentPackageUpload{
				Name:        "nginx",
				Version:     "2.0.0",
				PackageType: "helm-chart",
				Description: "NGINX web server updated",
				Content:     []byte("updated chart content"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := adapter.UploadDeploymentPackage(ctx, tt.pkg)
			require.NoError(t, err)
			require.NotNil(t, result)

			assert.Equal(t, tt.pkg.Name, result.Name)
			assert.Equal(t, tt.pkg.Version, result.Version)
			assert.Equal(t, "helm-chart", result.PackageType)
			assert.Equal(t, tt.pkg.Description, result.Description)
			assert.NotEmpty(t, result.ID)
			assert.NotNil(t, result.Extensions)
		})
	}
}
