package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestNewVersionConfig(t *testing.T) {
	config := NewVersionConfig()

	// Verify default version
	if config.DefaultVersion != "v1" {
		t.Errorf("DefaultVersion = %s, want v1", config.DefaultVersion)
	}

	// Verify all versions are defined
	versions := []string{"v1", "v2", "v3"}
	for _, v := range versions {
		if _, exists := config.Versions[v]; !exists {
			t.Errorf("Version %s not found in config", v)
		}
	}

	// Verify v1 is stable
	if config.Versions["v1"].Status != VersionStatusStable {
		t.Errorf("v1 Status = %s, want %s", config.Versions["v1"].Status, VersionStatusStable)
	}

	// Verify v2 is stable
	if config.Versions["v2"].Status != VersionStatusStable {
		t.Errorf("v2 Status = %s, want %s", config.Versions["v2"].Status, VersionStatusStable)
	}

	// Verify v3 is stable
	if config.Versions["v3"].Status != VersionStatusStable {
		t.Errorf("v3 Status = %s, want %s", config.Versions["v3"].Status, VersionStatusStable)
	}
}

func TestVersioningMiddleware(t *testing.T) {
	config := NewVersionConfig()

	tests := []struct {
		name           string
		path           string
		expectedStatus int
		expectedHeader string
	}{
		{
			name:           "v1 path sets version header",
			path:           "/o2ims-infrastructureInventory/v1/resources",
			expectedStatus: http.StatusOK,
			expectedHeader: "v1",
		},
		{
			name:           "v2 path sets version header",
			path:           "/o2ims-infrastructureInventory/v2/resources",
			expectedStatus: http.StatusOK,
			expectedHeader: "v2",
		},
		{
			name:           "v3 path sets version header",
			path:           "/o2ims-infrastructureInventory/v3/resources",
			expectedStatus: http.StatusOK,
			expectedHeader: "v3",
		},
		{
			name:           "non-existent version returns 404",
			path:           "/o2ims-infrastructureInventory/v99/resources",
			expectedStatus: http.StatusNotFound,
			expectedHeader: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(VersioningMiddleware(config))
			router.GET("/*path", func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			req, _ := http.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Status = %d, want %d", w.Code, tt.expectedStatus)
			}

			if tt.expectedHeader != "" {
				header := w.Header().Get("X-API-Version")
				if header != tt.expectedHeader {
					t.Errorf("X-API-Version = %s, want %s", header, tt.expectedHeader)
				}
			}
		})
	}
}

func TestVersioningMiddleware_Deprecation(t *testing.T) {
	config := NewVersionConfig()

	// Mark v1 as deprecated
	sunsetDate := time.Now().AddDate(0, 6, 0) // 6 months from now
	config.Versions["v1"].Status = VersionStatusDeprecated
	config.Versions["v1"].SunsetDate = &sunsetDate
	config.Versions["v1"].DeprecationMessage = "Please migrate to v2"

	router := gin.New()
	router.Use(VersioningMiddleware(config))
	router.GET("/*path", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req, _ := http.NewRequest(http.MethodGet, "/o2ims-infrastructureInventory/v1/resources", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should still return 200 but with deprecation headers
	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	deprecation := w.Header().Get("Deprecation")
	if deprecation != "true" {
		t.Errorf("Deprecation header = %s, want 'true'", deprecation)
	}

	notice := w.Header().Get("X-Deprecation-Notice")
	if notice != "Please migrate to v2" {
		t.Errorf("X-Deprecation-Notice = %s, want 'Please migrate to v2'", notice)
	}

	sunset := w.Header().Get("Sunset")
	if sunset == "" {
		t.Error("Expected Sunset header to be set")
	}
}

func TestVersioningMiddleware_Sunset(t *testing.T) {
	config := NewVersionConfig()

	// Mark v1 as sunset (removed)
	config.Versions["v1"].Status = VersionStatusSunset

	router := gin.New()
	router.Use(VersioningMiddleware(config))
	router.GET("/*path", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req, _ := http.NewRequest(http.MethodGet, "/o2ims-infrastructureInventory/v1/resources", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should return 410 Gone
	if w.Code != http.StatusGone {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusGone)
	}
}

func TestExtractVersionFromPath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "v1 version",
			path:     "/o2ims-infrastructureInventory/v1/resources",
			expected: "v1",
		},
		{
			name:     "v2 version",
			path:     "/o2ims-infrastructureInventory/v2/subscriptions",
			expected: "v2",
		},
		{
			name:     "v3 version",
			path:     "/o2ims-infrastructureInventory/v3/tenants",
			expected: "v3",
		},
		{
			name:     "v10 version",
			path:     "/api/v10/resources",
			expected: "v10",
		},
		{
			name:     "no version",
			path:     "/health",
			expected: "",
		},
		{
			name:     "invalid version format",
			path:     "/api/version1/resources",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractVersionFromPath(tt.path)
			if result != tt.expected {
				t.Errorf("extractVersionFromPath(%q) = %q, want %q", tt.path, result, tt.expected)
			}
		})
	}
}

func TestIsNumeric(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"123", true},
		{"0", true},
		{"1a", false},
		{"abc", false},
		{"", true}, // Empty string contains no non-numeric chars
		{"12.3", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := isNumeric(tt.input)
			if result != tt.expected {
				t.Errorf("isNumeric(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsVersionAtLeast(t *testing.T) {
	tests := []struct {
		current  string
		min      string
		expected bool
	}{
		{"v1", "v1", true},
		{"v2", "v1", true},
		{"v1", "v2", false},
		{"v3", "v2", true},
		{"v10", "v9", true},
		{"v1", "v10", false},
	}

	for _, tt := range tests {
		t.Run(tt.current+">="+tt.min, func(t *testing.T) {
			result := isVersionAtLeast(tt.current, tt.min)
			if result != tt.expected {
				t.Errorf("isVersionAtLeast(%q, %q) = %v, want %v", tt.current, tt.min, result, tt.expected)
			}
		})
	}
}

func TestExtractVersionNumber(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"v1", 1},
		{"v2", 2},
		{"v10", 10},
		{"1", 1},
		{"v123", 123},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := extractVersionNumber(tt.input)
			if result != tt.expected {
				t.Errorf("extractVersionNumber(%q) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestRequireVersion(t *testing.T) {
	router := gin.New()

	// Simulate version being set in context
	router.Use(func(c *gin.Context) {
		c.Set("api_version", "v1")
		c.Next()
	})
	router.Use(RequireVersion("v2"))
	router.GET("/feature", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req, _ := http.NewRequest(http.MethodGet, "/feature", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should return 501 Not Implemented
	if w.Code != http.StatusNotImplemented {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusNotImplemented)
	}
}

func TestRequireVersion_Satisfied(t *testing.T) {
	router := gin.New()

	// Simulate version being set in context
	router.Use(func(c *gin.Context) {
		c.Set("api_version", "v3")
		c.Next()
	})
	router.Use(RequireVersion("v2"))
	router.GET("/feature", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req, _ := http.NewRequest(http.MethodGet, "/feature", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should return 200 OK
	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestTenantMiddleware(t *testing.T) {
	tests := []struct {
		name             string
		version          string
		tenantHeader     string
		tenantQuery      string
		expectedTenantID string
	}{
		{
			name:             "v1 should not set tenant",
			version:          "v1",
			tenantHeader:     "tenant-123",
			expectedTenantID: "",
		},
		{
			name:             "v3 with header",
			version:          "v3",
			tenantHeader:     "tenant-123",
			expectedTenantID: "tenant-123",
		},
		{
			name:             "v3 with query param",
			version:          "v3",
			tenantQuery:      "tenant-456",
			expectedTenantID: "tenant-456",
		},
		{
			name:             "v3 without tenant uses default",
			version:          "v3",
			expectedTenantID: "default",
		},
		{
			name:             "v3 header takes precedence over query",
			version:          "v3",
			tenantHeader:     "tenant-header",
			tenantQuery:      "tenant-query",
			expectedTenantID: "tenant-header",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedTenant string

			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set("api_version", tt.version)
				c.Next()
			})
			router.Use(TenantMiddleware())
			router.GET("/test", func(c *gin.Context) {
				capturedTenant = c.GetString("tenant_id")
				c.Status(http.StatusOK)
			})

			path := "/test"
			if tt.tenantQuery != "" {
				path += "?tenantId=" + tt.tenantQuery
			}

			req, _ := http.NewRequest(http.MethodGet, path, nil)
			if tt.tenantHeader != "" {
				req.Header.Set("X-Tenant-ID", tt.tenantHeader)
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if capturedTenant != tt.expectedTenantID {
				t.Errorf("tenant_id = %q, want %q", capturedTenant, tt.expectedTenantID)
			}
		})
	}
}

func TestGetV2Features(t *testing.T) {
	features := GetV2Features()

	if !features.EnhancedFiltering {
		t.Error("V2 EnhancedFiltering should be true")
	}
	if !features.FieldSelection {
		t.Error("V2 FieldSelection should be true")
	}
	if !features.BatchOperations {
		t.Error("V2 BatchOperations should be true")
	}
	if !features.CursorPagination {
		t.Error("V2 CursorPagination should be true")
	}
}

func TestGetV3Features(t *testing.T) {
	features := GetV3Features()

	// Should include all V2 features
	if !features.EnhancedFiltering {
		t.Error("V3 EnhancedFiltering should be true")
	}
	if !features.FieldSelection {
		t.Error("V3 FieldSelection should be true")
	}
	if !features.BatchOperations {
		t.Error("V3 BatchOperations should be true")
	}
	if !features.CursorPagination {
		t.Error("V3 CursorPagination should be true")
	}

	// V3 specific features
	if !features.MultiTenancy {
		t.Error("V3 MultiTenancy should be true")
	}
	if !features.TenantQuotas {
		t.Error("V3 TenantQuotas should be true")
	}
	if !features.CrossTenantSharing {
		t.Error("V3 CrossTenantSharing should be true")
	}
	if !features.AuditLogging {
		t.Error("V3 AuditLogging should be true")
	}
}
