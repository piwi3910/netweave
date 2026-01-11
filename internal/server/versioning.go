// Package server provides HTTP server configuration and middleware.
package server

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// APIVersion represents an API version configuration.
type APIVersion struct {
	// Version is the version string (e.g., "v1", "v2", "v3").
	Version string
	// Status indicates the version status (stable, deprecated, sunset).
	Status string
	// SunsetDate is when the version will be removed (for deprecated versions).
	SunsetDate *time.Time
	// DeprecationMessage provides information about migration.
	DeprecationMessage string
}

// VersionStatus constants for API version lifecycle.
const (
	VersionStatusStable     = "stable"
	VersionStatusDeprecated = "deprecated"
	VersionStatusSunset     = "sunset"
)

// VersionConfig holds configuration for all API versions.
type VersionConfig struct {
	Versions       map[string]*APIVersion
	DefaultVersion string
}

// NewVersionConfig creates a new version configuration with default settings.
func NewVersionConfig() *VersionConfig {
	return &VersionConfig{
		Versions: map[string]*APIVersion{
			"v1": {
				Version:            "v1",
				Status:             VersionStatusStable,
				SunsetDate:         nil,
				DeprecationMessage: "",
			},
			"v2": {
				Version:            "v2",
				Status:             VersionStatusStable,
				SunsetDate:         nil,
				DeprecationMessage: "",
			},
			"v3": {
				Version:            "v3",
				Status:             VersionStatusStable,
				SunsetDate:         nil,
				DeprecationMessage: "",
			},
		},
		DefaultVersion: "v1",
	}
}

// VersioningMiddleware adds API version headers and handles deprecation notices.
func VersioningMiddleware(config *VersionConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extract version from path
		version := extractVersionFromPath(c.Request.URL.Path)
		if version == "" {
			version = config.DefaultVersion
		}

		versionInfo, exists := config.Versions[version]
		if !exists {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "NotFound",
				"message": "API version not found: " + version,
				"code":    http.StatusNotFound,
			})
			c.Abort()
			return
		}

		// Set version headers
		c.Header("X-API-Version", version)
		c.Header("X-API-Version-Status", versionInfo.Status)

		// Handle deprecated versions
		if versionInfo.Status == VersionStatusDeprecated {
			c.Header("Deprecation", "true")
			if versionInfo.DeprecationMessage != "" {
				c.Header("X-Deprecation-Notice", versionInfo.DeprecationMessage)
			}
			if versionInfo.SunsetDate != nil {
				c.Header("Sunset", versionInfo.SunsetDate.Format(time.RFC1123))
			}
		}

		// Handle sunset versions (completely removed)
		if versionInfo.Status == VersionStatusSunset {
			c.JSON(http.StatusGone, gin.H{
				"error":   "Gone",
				"message": "API version " + version + " has been removed. Please upgrade to a newer version.",
				"code":    http.StatusGone,
			})
			c.Abort()
			return
		}

		// Store version in context for handlers to use
		c.Set("api_version", version)
		c.Set("api_version_info", versionInfo)

		c.Next()
	}
}

// extractVersionFromPath extracts the API version from the URL path.
func extractVersionFromPath(path string) string {
	parts := strings.Split(path, "/")
	for _, part := range parts {
		if strings.HasPrefix(part, "v") && len(part) >= 2 {
			// Check if it's a valid version format (v1, v2, v3, etc.)
			versionNum := part[1:]
			if len(versionNum) > 0 && isNumeric(versionNum) {
				return part
			}
		}
	}
	return ""
}

// isNumeric checks if a string contains only numeric characters.
func isNumeric(s string) bool {
	// Prevent potential DoS from extremely long strings
	if len(s) > 10 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// V2Features contains feature flags for v2 API enhancements.
type V2Features struct {
	// EnhancedFiltering enables advanced query parameter filtering.
	EnhancedFiltering bool
	// FieldSelection enables selecting specific fields in responses.
	FieldSelection bool
	// BatchOperations enables batch create/update/delete endpoints.
	BatchOperations bool
	// CursorPagination enables cursor-based pagination.
	CursorPagination bool
}

// V3Features contains feature flags for v3 API with multi-tenancy.
type V3Features struct {
	// V2Features includes all v2 features.
	V2Features
	// MultiTenancy enables tenant isolation and management.
	MultiTenancy bool
	// TenantQuotas enables per-tenant resource quotas.
	TenantQuotas bool
	// CrossTenantSharing enables resource sharing between tenants.
	CrossTenantSharing bool
	// AuditLogging enables detailed audit trail for tenant operations.
	AuditLogging bool
}

// GetV2Features returns the v2 feature configuration.
func GetV2Features() V2Features {
	return V2Features{
		EnhancedFiltering: true,
		FieldSelection:    true,
		BatchOperations:   true,
		CursorPagination:  true,
	}
}

// GetV3Features returns the v3 feature configuration.
func GetV3Features() V3Features {
	return V3Features{
		V2Features:         GetV2Features(),
		MultiTenancy:       true,
		TenantQuotas:       true,
		CrossTenantSharing: true,
		AuditLogging:       true,
	}
}

// RequireVersion creates middleware that requires a minimum API version.
func RequireVersion(minVersion string) gin.HandlerFunc {
	return func(c *gin.Context) {
		currentVersion := c.GetString("api_version")
		if currentVersion == "" {
			currentVersion = "v1"
		}

		if !isVersionAtLeast(currentVersion, minVersion) {
			c.JSON(http.StatusNotImplemented, gin.H{
				"error":   "NotImplemented",
				"message": "This feature requires API version " + minVersion + " or higher",
				"code":    http.StatusNotImplemented,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// isVersionAtLeast checks if currentVersion is at least minVersion.
func isVersionAtLeast(current, minimum string) bool {
	currentNum := extractVersionNumber(current)
	minNum := extractVersionNumber(minimum)
	return currentNum >= minNum
}

// extractVersionNumber extracts the numeric version from a version string.
func extractVersionNumber(version string) int {
	version = strings.TrimPrefix(version, "v")
	num := 0
	for _, c := range version {
		if c >= '0' && c <= '9' {
			num = num*10 + int(c-'0')
		}
	}
	return num
}

// TenantMiddleware extracts tenant information for v3 multi-tenancy support.
func TenantMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check if we're using v3 API
		version := c.GetString("api_version")
		if version != "v3" {
			c.Next()
			return
		}

		// Extract tenant from header or path
		tenantID := c.GetHeader("X-Tenant-ID")
		if tenantID == "" {
			tenantID = c.Query("tenantId")
		}

		if tenantID == "" {
			// Default tenant for backward compatibility
			tenantID = "default"
		}

		// Store tenant in context
		c.Set("tenant_id", tenantID)

		// Add tenant to response headers
		c.Header("X-Tenant-ID", tenantID)

		c.Next()
	}
}
