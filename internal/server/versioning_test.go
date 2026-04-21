package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/piwi3910/netweave/internal/server"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

func TestNewVersionConfig(t *testing.T) {
	config := server.NewVersionConfig()

	// Verify default version
	if config.DefaultVersion != "v1" {
		t.Errorf("DefaultVersion = %s, want v1", config.DefaultVersion)
	}

	// Only v1 should be registered; v2/v3 route groups do not exist.
	if _, exists := config.Versions["v1"]; !exists {
		t.Error("v1 not found in config")
	}
	if len(config.Versions) != 1 {
		t.Errorf("Versions count = %d, want 1", len(config.Versions))
	}

	// Verify v1 is stable
	if config.Versions["v1"].Status != server.VersionStatusStable {
		t.Errorf("v1 Status = %s, want %s", config.Versions["v1"].Status, server.VersionStatusStable)
	}
}

func TestVersioningMiddleware(t *testing.T) {
	config := server.NewVersionConfig()

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
			name:           "unregistered version returns 404",
			path:           "/o2ims-infrastructureInventory/v2/resources",
			expectedStatus: http.StatusNotFound,
			expectedHeader: "",
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
			router.Use(server.VersioningMiddleware(config))
			router.GET("/*path", func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, tt.path, nil)
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
	config := server.NewVersionConfig()

	// Mark v1 as deprecated
	sunsetDate := time.Now().AddDate(0, 6, 0) // 6 months from now
	config.Versions["v1"].Status = server.VersionStatusDeprecated
	config.Versions["v1"].SunsetDate = &sunsetDate
	config.Versions["v1"].DeprecationMessage = "Please migrate to v2"

	router := gin.New()
	router.Use(server.VersioningMiddleware(config))
	router.GET("/*path", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req, _ := http.NewRequestWithContext(
		context.Background(), http.MethodGet, "/o2ims-infrastructureInventory/v1/resources", nil,
	)
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
	config := server.NewVersionConfig()

	// Mark v1 as sunset (removed)
	config.Versions["v1"].Status = server.VersionStatusSunset

	router := gin.New()
	router.Use(server.VersioningMiddleware(config))
	router.GET("/*path", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req, _ := http.NewRequestWithContext(
		context.Background(), http.MethodGet, "/o2ims-infrastructureInventory/v1/resources", nil,
	)
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
		{
			name:     "malformed path with v but no number",
			path:     "/api/v/resources",
			expected: "",
		},
		{
			name:     "malformed path with v and non-numeric",
			path:     "/api/vabc/resources",
			expected: "",
		},
		{
			name:     "very long numeric version (DoS attempt)",
			path:     "/api/v12345678901/resources",
			expected: "",
		},
		{
			name:     "multiple version segments (takes first)",
			path:     "/api/v1/v2/resources",
			expected: "v1",
		},
		{
			name:     "version at end of path",
			path:     "/api/resources/v2",
			expected: "v2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := server.ExtractVersionFromPath(tt.path)
			if result != tt.expected {
				t.Errorf("server.ExtractVersionFromPath(%q) = %q, want %q", tt.path, result, tt.expected)
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
		{"12345678901", false}, // > 10 chars - DoS prevention
		{"9999999999", true},   // Exactly 10 chars - should pass
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := server.IsNumeric(tt.input)
			if result != tt.expected {
				t.Errorf("server.IsNumeric(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}
