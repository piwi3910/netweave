// Package server provides HTTP server configuration and middleware.
package server

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/piwi3910/netweave/internal/httpx"
)

// APIVersion represents an API version configuration.
type APIVersion struct {
	// Version is the version string (e.g., "v1").
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
// Only v1 is registered; v2/v3 route groups do not exist in the gateway, so
// advertising them as stable would be misleading.
func NewVersionConfig() *VersionConfig {
	return &VersionConfig{
		Versions: map[string]*APIVersion{
			"v1": {
				Version:            "v1",
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
		version := ExtractVersionFromPath(c.Request.URL.Path)
		if version == "" {
			version = config.DefaultVersion
		}

		versionInfo, exists := config.Versions[version]
		if !exists {
			httpx.AbortWithError(c, http.StatusNotFound, "NotFound", "API version not found: "+version)
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
			httpx.AbortWithError(
				c,
				http.StatusGone,
				"Gone",
				"API version "+version+" has been removed. Please upgrade to a newer version.",
			)
			return
		}

		// Store version in context for handlers to use
		c.Set("api_version", version)
		c.Set("api_version_info", versionInfo)

		c.Next()
	}
}

// ExtractVersionFromPath extracts the API version from the URL path.
func ExtractVersionFromPath(path string) string {
	parts := strings.Split(path, "/")
	for _, part := range parts {
		if strings.HasPrefix(part, "v") && len(part) >= 2 {
			// Check if it's a valid version format (v1, v2, v3, etc.)
			versionNum := part[1:]
			if len(versionNum) > 0 && IsNumeric(versionNum) {
				return part
			}
		}
	}
	return ""
}

// IsNumeric checks if a string contains only numeric characters.
func IsNumeric(s string) bool {
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
