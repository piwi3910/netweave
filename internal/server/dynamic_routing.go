// Package server provides HTTP server infrastructure for the O2-IMS Gateway.
// This file implements per-request dynamic routing for DMS, SMO, TMForum, and GraphQL APIs.
// It provides middleware and resolver methods that resolve the correct adapter/registry
// for each request based on tenant context, matching the pattern used by O2-IMS resolveAdapter.
package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/auth"
	dmsregistry "github.com/piwi3910/netweave/internal/dms/registry"
	"github.com/piwi3910/netweave/internal/httpx"
	"github.com/piwi3910/netweave/internal/smo"
)

// resolveDMSRegistry determines which DMS registry to use for the current request.
// Resolution order:
//  1. If a DMS registry override is stored in the gin context (from middleware), use it.
//  2. Fall back to the server's static DMS registry.
//  3. Return nil if no DMS registry is available.
func (s *Server) resolveDMSRegistry(c *gin.Context) *dmsregistry.Registry {
	// Check for context-override (set by middleware for per-tenant routing).
	if reg, exists := c.Get(ctxKeyDMSRegistry); exists {
		if r, ok := reg.(*dmsregistry.Registry); ok && r != nil {
			return r
		}
	}

	// Fall back to the server's static DMS registry.
	return s.dmsRegistry
}

// resolveSMORegistry determines which SMO registry to use for the current request.
// Resolution order:
//  1. If an SMO registry override is stored in the gin context (from middleware), use it.
//  2. Fall back to the server's static SMO registry.
//  3. Return nil if no SMO registry is available.
func (s *Server) resolveSMORegistry(c *gin.Context) *smo.Registry {
	// Check for context-override (set by middleware for per-tenant routing).
	if reg, exists := c.Get(ctxKeySMORegistry); exists {
		if r, ok := reg.(*smo.Registry); ok && r != nil {
			return r
		}
	}

	// Fall back to the server's static SMO registry.
	return s.smoRegistry
}

// dmsAdapterMiddleware is a middleware that resolves the DMS registry and IMS adapter
// for the current request and stores them in gin context for DMS handlers to use.
func (s *Server) dmsAdapterMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Resolve and store the DMS registry for this request.
		reg := s.resolveDMSRegistry(c)
		if reg != nil {
			c.Set(ctxKeyDMSRegistry, reg)
		}

		// Also resolve the IMS adapter so DMS handlers can access O2-IMS resources.
		adp := s.resolveAdapterSilent(c)
		if adp != nil {
			c.Set(ctxKeyIMSAdapter, adp)
		}

		c.Next()
	}
}

// smoAdapterMiddleware is a middleware that resolves the SMO registry for the current request
// and stores it in gin context for SMO handlers to use.
func (s *Server) smoAdapterMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Resolve and store the SMO registry for this request.
		reg := s.resolveSMORegistry(c)
		if reg != nil {
			c.Set(ctxKeySMORegistry, reg)
		}

		c.Next()
	}
}

// tmfAdapterMiddleware is a middleware that resolves the IMS adapter and DMS registry
// for the current request and stores them in gin context for TMForum handlers.
func (s *Server) tmfAdapterMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Resolve the IMS adapter for TMForum resource operations.
		adp := s.resolveAdapterSilent(c)
		if adp != nil {
			c.Set(ctxKeyIMSAdapter, adp)
		}

		// Resolve the DMS registry for TMForum service/deployment operations.
		reg := s.resolveDMSRegistry(c)
		if reg != nil {
			c.Set(ctxKeyDMSRegistry, reg)
		}

		c.Next()
	}
}

// graphqlAdapterMiddleware is a middleware that resolves the IMS adapter for the current request
// and stores it in gin context for GraphQL resolvers to use.
func (s *Server) graphqlAdapterMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Resolve the IMS adapter for GraphQL queries.
		adp := s.resolveAdapterSilent(c)
		if adp != nil {
			c.Set(ctxKeyIMSAdapter, adp)
		}

		c.Next()
	}
}

// resolveAdapterSilent resolves the IMS adapter for the current request without writing
// error responses to the gin context. This is used by middleware that runs before handlers,
// where the handler itself should decide how to handle missing adapters.
// Returns nil if no adapter can be resolved.
func (s *Server) resolveAdapterSilent(c *gin.Context) adapter.Adapter {
	// If no registry is configured, return the static adapter.
	if s.adapterRegistry == nil || s.backendStore == nil {
		return s.adapter
	}

	ctx := c.Request.Context()

	// Platform admin with explicit backend selection.
	backendID := c.GetHeader("X-Backend-ID")
	if backendID != "" {
		adp, err := s.adapterRegistry.GetAdapter(backendID)
		if err != nil {
			s.logger.Debug("middleware: backend not found",
				zap.String("backend_id", backendID),
				zap.Error(err),
			)
			return s.adapter
		}
		return adp
	}

	// Resolve adapter based on tenant.
	tenantID := auth.TenantIDFromContext(ctx)
	if tenantID == "" {
		// No tenant context available - fall back to static adapter.
		return s.adapter
	}

	adp, err := s.adapterRegistry.GetAdapterForTenant(ctx, tenantID, s.backendStore)
	if err != nil {
		s.logger.Debug("middleware: failed to resolve adapter for tenant",
			zap.String("tenant_id", tenantID),
			zap.Error(err),
		)
		// Fall back to static adapter rather than failing silently.
		return s.adapter
	}

	return adp
}

// IMSAdapterFromContext extracts the resolved IMS adapter from gin context.
// Returns nil if no adapter was stored. Used by handlers that support dynamic routing.
func IMSAdapterFromContext(c *gin.Context) adapter.Adapter {
	if adp, exists := c.Get(ctxKeyIMSAdapter); exists {
		if a, ok := adp.(adapter.Adapter); ok {
			return a
		}
	}
	return nil
}

// DMSRegistryFromContext extracts the resolved DMS registry from gin context.
// Returns nil if no registry was stored. Used by DMS and TMForum handlers.
func DMSRegistryFromContext(c *gin.Context) *dmsregistry.Registry {
	if reg, exists := c.Get(ctxKeyDMSRegistry); exists {
		if r, ok := reg.(*dmsregistry.Registry); ok {
			return r
		}
	}
	return nil
}

// SMORegistryFromContext extracts the resolved SMO registry from gin context.
// Returns nil if no registry was stored. Used by SMO handlers.
func SMORegistryFromContext(c *gin.Context) *smo.Registry {
	if reg, exists := c.Get(ctxKeySMORegistry); exists {
		if r, ok := reg.(*smo.Registry); ok {
			return r
		}
	}
	return nil
}

// handleAdapterUnavailable sends a 503 response when no adapter could be resolved.
// Used by handlers that require an adapter and cannot proceed without one.
func handleAdapterUnavailable(c *gin.Context) {
	httpx.WriteError(c, http.StatusServiceUnavailable, "ServiceUnavailable", "No backend adapter available for this request. Ensure backend instances are configured.")
}
