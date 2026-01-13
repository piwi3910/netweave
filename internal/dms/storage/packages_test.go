package storage

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/piwi3910/netweave/internal/dms/adapter"
)

// TestNewMemoryPackageStore tests store creation.
func TestNewMemoryPackageStore(t *testing.T) {
	store := NewMemoryPackageStore()
	require.NotNil(t, store)
	assert.NotNil(t, store.packages)
	assert.NotNil(t, store.content)
	assert.NotNil(t, store.byName)
}

// TestMemoryPackageStore_Create tests package creation.
func TestMemoryPackageStore_Create(t *testing.T) {
	tests := []struct {
		name        string
		pkg         *adapter.DeploymentPackage
		wantErr     bool
		errContains string
		errIs       error
	}{
		{
			name: "valid package",
			pkg: &adapter.DeploymentPackage{
				ID:          "pkg-1",
				Name:        "test-package",
				Version:     "1.0.0",
				PackageType: "helm-chart",
			},
			wantErr: false,
		},
		{
			name: "valid package with v prefix",
			pkg: &adapter.DeploymentPackage{
				ID:          "pkg-2",
				Name:        "test-package",
				Version:     "v2.0.0",
				PackageType: "helm-chart",
			},
			wantErr: false,
		},
		{
			name: "valid package with prerelease",
			pkg: &adapter.DeploymentPackage{
				ID:          "pkg-3",
				Name:        "test-package",
				Version:     "1.0.0-beta.1",
				PackageType: "helm-chart",
			},
			wantErr: false,
		},
		{
			name:        "nil package",
			pkg:         nil,
			wantErr:     true,
			errContains: "cannot be nil",
		},
		{
			name: "missing ID",
			pkg: &adapter.DeploymentPackage{
				Name:    "test-package",
				Version: "1.0.0",
			},
			wantErr:     true,
			errContains: "ID cannot be empty",
		},
		{
			name: "missing name",
			pkg: &adapter.DeploymentPackage{
				ID:      "pkg-4",
				Version: "1.0.0",
			},
			wantErr:     true,
			errContains: "name cannot be empty",
		},
		{
			name: "missing version",
			pkg: &adapter.DeploymentPackage{
				ID:   "pkg-5",
				Name: "test-package",
			},
			wantErr:     true,
			errContains: "version cannot be empty",
		},
		{
			name: "invalid version",
			pkg: &adapter.DeploymentPackage{
				ID:      "pkg-6",
				Name:    "test-package",
				Version: "invalid",
			},
			wantErr: true,
			errIs:   ErrInvalidPackageVersion,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewMemoryPackageStore()
			err := store.Create(context.Background(), tt.pkg)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				if tt.errIs != nil {
					assert.ErrorIs(t, err, tt.errIs)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestMemoryPackageStore_Create_Duplicates tests duplicate handling.
func TestMemoryPackageStore_Create_Duplicates(t *testing.T) {
	store := NewMemoryPackageStore()

	pkg := &adapter.DeploymentPackage{
		ID:          "pkg-1",
		Name:        "test-package",
		Version:     "1.0.0",
		PackageType: "helm-chart",
	}

	// First create should succeed
	err := store.Create(context.Background(), pkg)
	require.NoError(t, err)

	// Duplicate ID should fail
	t.Run("duplicate ID", func(t *testing.T) {
		err := store.Create(context.Background(), pkg)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrPackageExists)
	})

	// Same name+version but different ID should fail
	t.Run("duplicate name+version", func(t *testing.T) {
		dupPkg := &adapter.DeploymentPackage{
			ID:          "pkg-2",
			Name:        "test-package",
			Version:     "1.0.0",
			PackageType: "helm-chart",
		}
		err := store.Create(context.Background(), dupPkg)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrVersionExists)
	})

	// Same name but different version should succeed
	t.Run("same name different version", func(t *testing.T) {
		newVersionPkg := &adapter.DeploymentPackage{
			ID:          "pkg-3",
			Name:        "test-package",
			Version:     "2.0.0",
			PackageType: "helm-chart",
		}
		err := store.Create(context.Background(), newVersionPkg)
		require.NoError(t, err)
	})
}

// TestMemoryPackageStore_Get tests package retrieval.
func TestMemoryPackageStore_Get(t *testing.T) {
	store := NewMemoryPackageStore()

	pkg := &adapter.DeploymentPackage{
		ID:          "pkg-1",
		Name:        "test-package",
		Version:     "1.0.0",
		PackageType: "helm-chart",
		Description: "Test description",
		Extensions: map[string]interface{}{
			"key": "value",
		},
	}
	err := store.Create(context.Background(), pkg)
	require.NoError(t, err)

	t.Run("get existing", func(t *testing.T) {
		result, err := store.Get(context.Background(), "pkg-1")
		require.NoError(t, err)
		assert.Equal(t, pkg.ID, result.ID)
		assert.Equal(t, pkg.Name, result.Name)
		assert.Equal(t, pkg.Version, result.Version)
		assert.Equal(t, pkg.Description, result.Description)
		assert.Equal(t, "value", result.Extensions["key"])
	})

	t.Run("get nonexistent", func(t *testing.T) {
		_, err := store.Get(context.Background(), "nonexistent")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrPackageNotFound)
	})
}

// TestMemoryPackageStore_GetByNameVersion tests retrieval by name and version.
func TestMemoryPackageStore_GetByNameVersion(t *testing.T) {
	store := NewMemoryPackageStore()

	pkg1 := &adapter.DeploymentPackage{
		ID:          "pkg-1",
		Name:        "test-package",
		Version:     "1.0.0",
		PackageType: "helm-chart",
	}
	pkg2 := &adapter.DeploymentPackage{
		ID:          "pkg-2",
		Name:        "test-package",
		Version:     "2.0.0",
		PackageType: "helm-chart",
	}
	_ = store.Create(context.Background(), pkg1)
	_ = store.Create(context.Background(), pkg2)

	t.Run("get existing version", func(t *testing.T) {
		result, err := store.GetByNameVersion(context.Background(), "test-package", "1.0.0")
		require.NoError(t, err)
		assert.Equal(t, "pkg-1", result.ID)
	})

	t.Run("get different version", func(t *testing.T) {
		result, err := store.GetByNameVersion(context.Background(), "test-package", "2.0.0")
		require.NoError(t, err)
		assert.Equal(t, "pkg-2", result.ID)
	})

	t.Run("nonexistent name", func(t *testing.T) {
		_, err := store.GetByNameVersion(context.Background(), "nonexistent", "1.0.0")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrPackageNotFound)
	})

	t.Run("nonexistent version", func(t *testing.T) {
		_, err := store.GetByNameVersion(context.Background(), "test-package", "9.9.9")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrPackageNotFound)
	})
}

// TestMemoryPackageStore_List tests package listing.
func TestMemoryPackageStore_List(t *testing.T) {
	store := NewMemoryPackageStore()

	// Create test packages
	packages := []*adapter.DeploymentPackage{
		{
			ID: "helm-1", Name: "nginx", Version: "1.0.0", PackageType: "helm-chart",
			UploadedAt: time.Now().Add(-2 * time.Hour),
		},
		{
			ID: "helm-2", Name: "nginx", Version: "2.0.0", PackageType: "helm-chart",
			UploadedAt: time.Now().Add(-1 * time.Hour),
		},
		{ID: "helm-3", Name: "redis", Version: "1.0.0", PackageType: "helm-chart", UploadedAt: time.Now()},
		{ID: "git-1", Name: "my-app", Version: "1.0.0", PackageType: "git-repo", UploadedAt: time.Now()},
	}
	for _, pkg := range packages {
		_ = store.Create(context.Background(), pkg)
	}

	t.Run("list all", func(t *testing.T) {
		results, err := store.List(context.Background(), nil)
		require.NoError(t, err)
		assert.Len(t, results, 4)
	})

	t.Run("filter by name", func(t *testing.T) {
		results, err := store.List(context.Background(), &PackageFilter{Name: "nginx"})
		require.NoError(t, err)
		assert.Len(t, results, 2)
	})

	t.Run("filter by package type", func(t *testing.T) {
		results, err := store.List(context.Background(), &PackageFilter{PackageType: "git-repo"})
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, "my-app", results[0].Name)
	})

	t.Run("latest only", func(t *testing.T) {
		results, err := store.List(context.Background(), &PackageFilter{LatestOnly: true})
		require.NoError(t, err)
		// Should return latest version of each: nginx 2.0.0, redis 1.0.0, my-app 1.0.0
		assert.Len(t, results, 3)
	})

	t.Run("latest only filtered by name", func(t *testing.T) {
		results, err := store.List(context.Background(), &PackageFilter{
			Name:       "nginx",
			LatestOnly: true,
		})
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, "2.0.0", results[0].Version)
	})

	t.Run("pagination limit", func(t *testing.T) {
		results, err := store.List(context.Background(), &PackageFilter{Limit: 2})
		require.NoError(t, err)
		assert.Len(t, results, 2)
	})

	t.Run("pagination offset", func(t *testing.T) {
		results, err := store.List(context.Background(), &PackageFilter{Offset: 2, Limit: 10})
		require.NoError(t, err)
		assert.Len(t, results, 2)
	})
}

// TestMemoryPackageStore_ListVersions tests version listing.
func TestMemoryPackageStore_ListVersions(t *testing.T) {
	store := NewMemoryPackageStore()

	// Create multiple versions
	_ = store.Create(context.Background(), &adapter.DeploymentPackage{
		ID: "pkg-1", Name: "test-package", Version: "1.0.0", PackageType: "helm-chart",
	})
	_ = store.Create(context.Background(), &adapter.DeploymentPackage{
		ID: "pkg-2", Name: "test-package", Version: "2.0.0", PackageType: "helm-chart",
	})
	_ = store.Create(context.Background(), &adapter.DeploymentPackage{
		ID: "pkg-3", Name: "other-package", Version: "1.0.0", PackageType: "helm-chart",
	})

	t.Run("list versions", func(t *testing.T) {
		results, err := store.ListVersions(context.Background(), "test-package")
		require.NoError(t, err)
		assert.Len(t, results, 2)
	})

	t.Run("nonexistent package", func(t *testing.T) {
		results, err := store.ListVersions(context.Background(), "nonexistent")
		require.NoError(t, err)
		assert.Empty(t, results)
	})
}

// TestMemoryPackageStore_Update tests package updates.
func TestMemoryPackageStore_Update(t *testing.T) {
	store := NewMemoryPackageStore()

	pkg := &adapter.DeploymentPackage{
		ID:          "pkg-1",
		Name:        "test-package",
		Version:     "1.0.0",
		PackageType: "helm-chart",
		Description: "Original description",
	}
	_ = store.Create(context.Background(), pkg)

	t.Run("update existing", func(t *testing.T) {
		updatedPkg := &adapter.DeploymentPackage{
			ID:          "pkg-1",
			Name:        "test-package",
			Version:     "1.0.0",
			PackageType: "helm-chart",
			Description: "Updated description",
		}
		err := store.Update(context.Background(), updatedPkg)
		require.NoError(t, err)

		result, _ := store.Get(context.Background(), "pkg-1")
		assert.Equal(t, "Updated description", result.Description)
	})

	t.Run("update nonexistent", func(t *testing.T) {
		updatedPkg := &adapter.DeploymentPackage{
			ID:   "nonexistent",
			Name: "test-package",
		}
		err := store.Update(context.Background(), updatedPkg)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrPackageNotFound)
	})

	t.Run("update nil package", func(t *testing.T) {
		err := store.Update(context.Background(), nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})
}

// TestMemoryPackageStore_Delete tests package deletion.
func TestMemoryPackageStore_Delete(t *testing.T) {
	store := NewMemoryPackageStore()

	pkg := &adapter.DeploymentPackage{
		ID:          "pkg-1",
		Name:        "test-package",
		Version:     "1.0.0",
		PackageType: "helm-chart",
	}
	_ = store.Create(context.Background(), pkg)
	_ = store.SaveContent(context.Background(), "pkg-1", []byte("test content"))

	t.Run("delete existing", func(t *testing.T) {
		err := store.Delete(context.Background(), "pkg-1")
		require.NoError(t, err)

		// Verify package is deleted
		_, err = store.Get(context.Background(), "pkg-1")
		assert.ErrorIs(t, err, ErrPackageNotFound)

		// Verify content is also deleted
		_, err = store.GetContent(context.Background(), "pkg-1")
		assert.Error(t, err)

		// Verify name index is cleaned up
		versions, _ := store.ListVersions(context.Background(), "test-package")
		assert.Empty(t, versions)
	})

	t.Run("delete nonexistent", func(t *testing.T) {
		err := store.Delete(context.Background(), "nonexistent")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrPackageNotFound)
	})
}

// TestMemoryPackageStore_Content tests content operations.
func TestMemoryPackageStore_Content(t *testing.T) {
	store := NewMemoryPackageStore()

	pkg := &adapter.DeploymentPackage{
		ID:          "pkg-1",
		Name:        "test-package",
		Version:     "1.0.0",
		PackageType: "helm-chart",
	}
	_ = store.Create(context.Background(), pkg)

	t.Run("save and get content", func(t *testing.T) {
		content := []byte("test content data")
		err := store.SaveContent(context.Background(), "pkg-1", content)
		require.NoError(t, err)

		result, err := store.GetContent(context.Background(), "pkg-1")
		require.NoError(t, err)
		assert.Equal(t, content, result)
	})

	t.Run("save content for nonexistent package", func(t *testing.T) {
		err := store.SaveContent(context.Background(), "nonexistent", []byte("data"))
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrPackageNotFound)
	})

	t.Run("get content for nonexistent package", func(t *testing.T) {
		_, err := store.GetContent(context.Background(), "nonexistent")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrPackageNotFound)
	})

	t.Run("get content when not uploaded", func(t *testing.T) {
		pkg2 := &adapter.DeploymentPackage{
			ID:          "pkg-2",
			Name:        "no-content",
			Version:     "1.0.0",
			PackageType: "helm-chart",
		}
		_ = store.Create(context.Background(), pkg2)

		_, err := store.GetContent(context.Background(), "pkg-2")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "content not found")
	})

	t.Run("delete content", func(t *testing.T) {
		err := store.DeleteContent(context.Background(), "pkg-1")
		require.NoError(t, err)

		_, err = store.GetContent(context.Background(), "pkg-1")
		require.Error(t, err)
	})

	t.Run("save content exceeds max size", func(t *testing.T) {
		// Create a package for this test
		pkg3 := &adapter.DeploymentPackage{
			ID:          "pkg-3",
			Name:        "large-content",
			Version:     "1.0.0",
			PackageType: "helm-chart",
		}
		_ = store.Create(context.Background(), pkg3)

		// Create content that exceeds MaxContentSize
		// Use MaxContentSize + 1 to just exceed the limit
		largeContent := make([]byte, MaxContentSize+1)
		err := store.SaveContent(context.Background(), "pkg-3", largeContent)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrContentTooLarge)
	})

	t.Run("save content at max size succeeds", func(t *testing.T) {
		// Create a package for this test
		pkg4 := &adapter.DeploymentPackage{
			ID:          "pkg-4",
			Name:        "max-content",
			Version:     "1.0.0",
			PackageType: "helm-chart",
		}
		_ = store.Create(context.Background(), pkg4)

		// Create content exactly at MaxContentSize (should succeed)
		maxContent := make([]byte, MaxContentSize)
		err := store.SaveContent(context.Background(), "pkg-4", maxContent)
		require.NoError(t, err)

		// Verify we can retrieve it
		result, err := store.GetContent(context.Background(), "pkg-4")
		require.NoError(t, err)
		assert.Len(t, result, MaxContentSize)
	})
}

// TestMemoryPackageStore_Ping tests health check.
func TestMemoryPackageStore_Ping(t *testing.T) {
	store := NewMemoryPackageStore()
	err := store.Ping(context.Background())
	require.NoError(t, err)
}

// TestMemoryPackageStore_Close tests store closure.
func TestMemoryPackageStore_Close(t *testing.T) {
	store := NewMemoryPackageStore()

	// Add some data
	_ = store.Create(context.Background(), &adapter.DeploymentPackage{
		ID: "pkg-1", Name: "test", Version: "1.0.0", PackageType: "helm-chart",
	})
	_ = store.SaveContent(context.Background(), "pkg-1", []byte("data"))

	err := store.Close()
	require.NoError(t, err)

	// Verify data is cleared
	results, _ := store.List(context.Background(), nil)
	assert.Empty(t, results)
}

// TestMemoryPackageStore_ContextCancellation tests context handling.
func TestMemoryPackageStore_ContextCancellation(t *testing.T) {
	store := NewMemoryPackageStore()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name string
		fn   func() error
	}{
		{
			name: "Create",
			fn: func() error {
				return store.Create(ctx, &adapter.DeploymentPackage{
					ID: "pkg", Name: "test", Version: "1.0.0", PackageType: "helm",
				})
			},
		},
		{
			name: "Get",
			fn: func() error {
				_, err := store.Get(ctx, "pkg")
				return err
			},
		},
		{
			name: "GetByNameVersion",
			fn: func() error {
				_, err := store.GetByNameVersion(ctx, "test", "1.0.0")
				return err
			},
		},
		{
			name: "List",
			fn: func() error {
				_, err := store.List(ctx, nil)
				return err
			},
		},
		{
			name: "ListVersions",
			fn: func() error {
				_, err := store.ListVersions(ctx, "test")
				return err
			},
		},
		{
			name: "Update",
			fn: func() error {
				return store.Update(ctx, &adapter.DeploymentPackage{ID: "pkg"})
			},
		},
		{
			name: "Delete",
			fn: func() error {
				return store.Delete(ctx, "pkg")
			},
		},
		{
			name: "SaveContent",
			fn: func() error {
				return store.SaveContent(ctx, "pkg", []byte("data"))
			},
		},
		{
			name: "GetContent",
			fn: func() error {
				_, err := store.GetContent(ctx, "pkg")
				return err
			},
		},
		{
			name: "DeleteContent",
			fn: func() error {
				return store.DeleteContent(ctx, "pkg")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			require.Error(t, err)
			assert.ErrorIs(t, err, context.Canceled)
		})
	}
}

// TestValidateSemVer tests version validation.
func TestValidateSemVer(t *testing.T) {
	tests := []struct {
		version string
		valid   bool
	}{
		{"1.0.0", true},
		{"v1.0.0", true},
		{"1.2.3", true},
		{"1.0", true},
		{"1", true},
		{"1.0.0-alpha", true},
		{"1.0.0-alpha.1", true},
		{"1.0.0-beta.2", true},
		{"1.0.0+build", true},
		{"1.0.0-rc.1+build.123", true},
		{"v2.0.0-beta", true},
		{"invalid", false},
		{"a.b.c", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			err := validateSemVer(tt.version)
			if tt.valid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

// TestCopyPackage tests package copying.
func TestCopyPackage(t *testing.T) {
	t.Run("nil package", func(t *testing.T) {
		result := copyPackage(nil)
		assert.Nil(t, result)
	})

	t.Run("package with extensions", func(t *testing.T) {
		pkg := &adapter.DeploymentPackage{
			ID:      "pkg-1",
			Name:    "test",
			Version: "1.0.0",
			Extensions: map[string]interface{}{
				"key": "value",
			},
		}
		result := copyPackage(pkg)
		require.NotNil(t, result)
		assert.Equal(t, pkg.ID, result.ID)
		assert.NotSame(t, &pkg.Extensions, &result.Extensions)
		assert.Equal(t, pkg.Extensions["key"], result.Extensions["key"])
	})
}

// TestApplyPackagePagination tests pagination logic.
func TestApplyPackagePagination(t *testing.T) {
	packages := []*adapter.DeploymentPackage{
		{ID: "1"}, {ID: "2"}, {ID: "3"}, {ID: "4"}, {ID: "5"},
	}

	tests := []struct {
		name      string
		limit     int
		offset    int
		wantCount int
		wantFirst string
	}{
		{"no pagination", 0, 0, 5, "1"},
		{"limit only", 2, 0, 2, "1"},
		{"offset only", 10, 2, 3, "3"},
		{"limit and offset", 2, 1, 2, "2"},
		{"offset beyond length", 10, 10, 0, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := applyPackagePagination(packages, tt.limit, tt.offset)
			assert.Len(t, result, tt.wantCount)
			if tt.wantCount > 0 {
				assert.Equal(t, tt.wantFirst, result[0].ID)
			}
		})
	}
}

// Benchmark tests.

func BenchmarkMemoryPackageStore_Create(b *testing.B) {
	store := NewMemoryPackageStore()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pkg := &adapter.DeploymentPackage{
			ID:          formatBenchID("pkg-%d", i),
			Name:        "bench-package",
			Version:     formatBenchID("%d.0.0", i),
			PackageType: "helm-chart",
		}
		_ = store.Create(ctx, pkg)
	}
}

func BenchmarkMemoryPackageStore_Get(b *testing.B) {
	store := NewMemoryPackageStore()
	ctx := context.Background()

	// Create test packages
	for i := 0; i < 1000; i++ {
		pkg := &adapter.DeploymentPackage{
			ID:          formatBenchID("pkg-%d", i),
			Name:        "bench-package",
			Version:     formatBenchID("%d.0.0", i),
			PackageType: "helm-chart",
		}
		_ = store.Create(ctx, pkg)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = store.Get(ctx, formatBenchID("pkg-%d", i%1000))
	}
}

func BenchmarkMemoryPackageStore_List(b *testing.B) {
	store := NewMemoryPackageStore()
	ctx := context.Background()

	// Create test packages
	for i := 0; i < 100; i++ {
		pkg := &adapter.DeploymentPackage{
			ID:          formatBenchID("pkg-%d", i),
			Name:        formatBenchID("package-%d", i%10),
			Version:     formatBenchID("%d.0.0", i%10),
			PackageType: "helm-chart",
		}
		_ = store.Create(ctx, pkg)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = store.List(ctx, nil)
	}
}

func formatBenchID(format string, i int) string {
	return formatBenchString(format, i)
}

func formatBenchString(format string, a ...interface{}) string {
	// Simple sprintf equivalent to avoid import in benchmark
	if len(a) == 0 {
		return format
	}
	// This is a simplified version for benchmarks
	result := format
	for _, v := range a {
		if intVal, ok := v.(int); ok {
			result = replaceFirst(result, "%d", intToString(intVal))
		}
	}
	return result
}

func replaceFirst(s, old, replacement string) string {
	for i := 0; i <= len(s)-len(old); i++ {
		if s[i:i+len(old)] == old {
			return s[:i] + replacement + s[i+len(old):]
		}
	}
	return s
}

func intToString(n int) string {
	if n == 0 {
		return "0"
	}
	result := ""
	for n > 0 {
		result = string(rune('0'+n%10)) + result
		n /= 10
	}
	return result
}
