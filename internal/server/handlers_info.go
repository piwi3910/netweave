package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// handleRoot returns a minimal unauthenticated banner.
//
// Detailed version / build / feature information is not emitted here: the
// admin router is reachable without authentication and anything returned is
// accessible to anonymous scanners. Operators can obtain the full banner
// from the authenticated O2 /o2ims endpoint (see handleAPIInfoDetailed).
func (s *Server) handleRoot(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"service": "netweave",
		"api":     "o2ims",
		"status":  "ok",
	})
}

// handleAPIInfo returns a minimal unauthenticated banner for /o2ims.
// See handleRoot for rationale.
func (s *Server) handleAPIInfo(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"service": "netweave",
		"api":     "o2ims",
		"status":  "ok",
	})
}

// handleAPIInfoDetailed returns the full O2-IMS API descriptor. It is mounted
// on the authenticated O2 router so operators/SMO clients can discover
// supported resources without leaking metadata to anonymous callers.
func (s *Server) handleAPIInfoDetailed(c *gin.Context) {
	resources := []string{
		"subscriptions",
		"resourcePools",
		"resources",
		"resourceTypes",
		"deploymentManagers",
		"oCloudInfrastructure",
		"batch",
	}

	if s.tenantHandler != nil {
		resources = append(resources, "tenants")
	}

	features := []string{
		"O-RAN O2-IMS v1 compliant",
		"Batch operations for subscriptions, resourcePools, and resources",
		"Advanced filtering with comparison operators (eq, ne, gt, lt, gte, lte, in, nin, regex)",
		"Field selection to optimize API responses",
		"Cursor-based pagination",
	}
	if s.tenantHandler != nil {
		features = append(features, "Multi-tenancy support with tenant isolation and quotas")
	}

	c.JSON(http.StatusOK, gin.H{
		"api_version": "v1",
		"base_path":   "/o2ims-infrastructureInventory/v1",
		"resources":   resources,
		"features":    features,
	})
}
