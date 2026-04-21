package server

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/auth"
	"github.com/piwi3910/netweave/internal/backend"
)

// withPermission wraps a handler with permission-based authorization.
// If auth middleware is not configured, the handler runs without authorization checks.
func (s *Server) withPermission(permission string, handler gin.HandlerFunc) gin.HandlerFunc {
	if s.authMw == nil {
		// Auth not configured, return handler without authorization
		return handler
	}

	// Chain permission middleware with handler
	return func(c *gin.Context) {
		// Apply permission middleware
		s.authMw.RequirePermission(permission)(c)

		// If middleware aborted (unauthorized), stop here
		if c.IsAborted() {
			return
		}

		// Execute handler
		handler(c)
	}
}

// withPlatformAdmin wraps a handler with permission-based authorization AND
// a platform-admin-only check. This is used for endpoints that are inherently
// cross-tenant (e.g., listing all tenants, creating a tenant) and therefore
// must never be reachable by non-admin callers even if they hold a matching
// permission string.
func (s *Server) withPlatformAdmin(permission string, handler gin.HandlerFunc) gin.HandlerFunc {
	if s.authMw == nil {
		// Auth not configured — match existing withPermission semantics and
		// skip the check. Production deployments are expected to always
		// configure auth middleware.
		return handler
	}

	return func(c *gin.Context) {
		s.authMw.RequirePermission(permission)(c)
		if c.IsAborted() {
			return
		}

		s.authMw.RequirePlatformAdmin()(c)
		if c.IsAborted() {
			return
		}

		handler(c)
	}
}

// withTenantAccess wraps a handler with both permission-based authorization and
// tenant ownership enforcement. The caller must (a) hold the required permission
// and (b) either be a platform admin or be acting on their own tenant (as
// identified by the URL path parameter named by tenantIDParam).
//
// This prevents privilege escalation where a role with tenants:* permissions
// (for example role-tenant-admin) can access an arbitrary tenant's resources by
// sending requests scoped to a tenant they do not own.
func (s *Server) withTenantAccess(
	permission, tenantIDParam string,
	handler gin.HandlerFunc,
) gin.HandlerFunc {
	if s.authMw == nil {
		// Auth not configured — still enforce tenant ownership at the handler level
		// by performing an inline check using the authenticated user context.
		// Since no auth middleware is configured, skip the check entirely to match
		// existing withPermission semantics for non-authenticated deployments.
		return handler
	}

	return func(c *gin.Context) {
		// Apply permission middleware first so that 401/403 on missing credentials
		// takes precedence over tenant ownership checks.
		s.authMw.RequirePermission(permission)(c)
		if c.IsAborted() {
			return
		}

		// Apply tenant access middleware: platform admins pass through; other
		// users must be acting on their own tenant.
		s.authMw.RequireTenantAccess(tenantIDParam)(c)
		if c.IsAborted() {
			return
		}

		handler(c)
	}
}

// enforceTenantOwnershipInline is a defense-in-depth guard used by
// quota handlers to re-enforce tenant ownership at the handler level.
// It is intentionally redundant with the RequireTenantAccess route
// middleware so that a misconfigured route registration cannot
// silently leak cross-tenant access (issue #469 / C6). Returns true
// when the caller is authorised and false when a response has
// already been written.
//
// The response uses 404 rather than 403 to avoid leaking the
// existence of the target tenant to a non-owning caller.
func (s *Server) enforceTenantOwnershipInline(c *gin.Context, tenantID string) bool {
	user := auth.UserFromContext(c.Request.Context())
	// If no authenticated user context exists, the surrounding auth
	// middleware is responsible for rejecting the request; in an
	// auth-disabled deployment (tests) there is no tenant to enforce
	// against, so fall through.
	if user == nil {
		return true
	}
	if user.IsPlatformAdmin {
		return true
	}
	if user.TenantID == tenantID {
		return true
	}

	s.logger.Warn("cross-tenant access denied at handler guard",
		zap.String("user_id", user.UserID),
		zap.String("user_tenant", user.TenantID),
		zap.String("target_tenant", tenantID),
		zap.String("path", c.Request.URL.Path),
		zap.String("request_id", c.GetString("request_id")),
	)

	c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
		"error":   "NotFound",
		"message": "Tenant not found",
		"code":    http.StatusNotFound,
	})
	return false
}

// resolveAdapter determines which adapter to use for the current request.
// Returns nil and writes an error response if no adapter can be resolved.
//
// Resolution order:
//  1. If the adapter registry is configured, resolve dynamically (tenant access or X-Backend-ID).
//  2. If no registry but a static adapter exists (legacy/testing), use that.
//  3. Otherwise, return nil (503 Service Unavailable).
func (s *Server) resolveAdapter(c *gin.Context) adapter.Adapter {
	ctx := c.Request.Context()

	// If no registry is configured, fall back to static adapter (legacy/testing mode)
	if s.adapterRegistry == nil || s.backendStore == nil {
		if s.adapter != nil {
			return s.adapter
		}
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":   "ServiceUnavailable",
			"message": "No backend adapters configured. Create backend instances via the admin API.",
		})
		return nil
	}

	// Platform admin can explicitly select a backend via header
	backendID := c.GetHeader("X-Backend-ID")
	if backendID != "" && auth.IsPlatformAdminFromContext(ctx) {
		adp, err := s.adapterRegistry.GetAdapter(backendID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "NotFound",
				"message": fmt.Sprintf("Backend %q not found in registry", backendID),
			})
			return nil
		}
		return adp
	}

	// Resolve adapter based on tenant's backend access
	tenantID := auth.TenantIDFromContext(ctx)
	if tenantID == "" {
		// Platform admins must specify X-Backend-ID
		if auth.IsPlatformAdminFromContext(ctx) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "BadRequest",
				"message": "Platform admins must specify X-Backend-ID header to select a backend",
			})
			return nil
		}
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "Unauthorized",
			"message": "No tenant context available",
		})
		return nil
	}

	// Platform admins without X-Backend-ID header
	if auth.IsPlatformAdminFromContext(ctx) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": "Platform admins must specify X-Backend-ID header to select a backend",
		})
		return nil
	}

	adp, err := s.adapterRegistry.GetAdapterForTenant(ctx, tenantID, s.backendStore)
	if err != nil {
		if errors.Is(err, backend.ErrNoBackendAccess) {
			c.JSON(http.StatusForbidden, gin.H{
				"error":   "Forbidden",
				"message": "No backend access configured for this tenant",
			})
			return nil
		}
		if errors.Is(err, backend.ErrAdapterNotFound) {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error":   "ServiceUnavailable",
				"message": "Assigned backend is not currently available",
			})
			return nil
		}
		s.logger.Warn("failed to resolve adapter for tenant",
			zap.String("tenant_id", tenantID),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalServerError",
			"message": "Failed to resolve backend adapter",
		})
		return nil
	}

	return adp
}

// setupRoutes configures all HTTP routes across the multi-port architecture.
// Routes are distributed across 4 routers:
//   - adminRouter: Health, metrics, docs, admin API, info endpoints
//   - o2Router:    O2-IMS, O2-DMS, O2-SMO (mTLS authenticated)
//   - tmfRouter:   TMForum APIs (OAuth2 authenticated)
//   - graphqlRouter: GraphQL API (OAuth2 authenticated)
func (s *Server) setupRoutes() {
	// === Admin Router (port 8080): health, metrics, docs, admin, info ===
	s.adminRouter.GET("/health", s.handleHealth)
	s.adminRouter.GET("/healthz", s.handleHealth)
	s.adminRouter.GET("/ready", s.handleReadiness)
	s.adminRouter.GET("/readyz", s.handleReadiness)

	if s.config.Observability.Metrics.Enabled {
		s.adminRouter.GET(s.config.Observability.Metrics.Path, s.handleMetrics)
	}

	// /o2ims serves API metadata. It was previously opened up via a
	// SkipPaths entry that did exact-match on "/o2ims", which would silently
	// allow future siblings like "/o2ims/<child>" if anyone switched to
	// prefix matching. Mark the exact (method, path) public at the handler
	// level instead so that sibling routes remain authenticated.
	s.adminRouter.GET("/o2ims", s.handleAPIInfo)
	if s.authMw != nil {
		if mw, ok := s.authMw.(*auth.Middleware); ok {
			mw.MarkPublicRoute(http.MethodGet, "/o2ims")
		}
	}
	s.adminRouter.GET("/", s.handleRoot)

	// Documentation endpoints on admin router
	s.SetupDocsRoutes()

	// === O2 Router (port 8443): O2-IMS v1 routes with mTLS ===
	versionConfig := NewVersionConfig()

	v1 := s.o2Router.Group("/o2ims-infrastructureInventory/v1")
	v1.Use(PluginGuard(s.pluginRegistry, "o2ims"))
	v1.Use(VersioningMiddleware(versionConfig))

	s.setupV1Routes(v1)

	// === TMF Router (port 8444): TMForum API routes ===
	s.setupTMForumRoutesEarly()

	// === GraphQL Router (port 8445): GraphQL endpoint ===
	s.setupGraphQLRoutes()
}

// setupV1Routes configures the O2-IMS API v1 endpoints.
func (s *Server) setupV1Routes(v1 *gin.RouterGroup) {
	// Infrastructure Inventory Subscription Management
	// Endpoint: /subscriptions
	subscriptions := v1.Group("/subscriptions")
	{
		subscriptions.GET("", s.withPermission("subscriptions:read", s.handleListSubscriptions))
		subscriptions.POST("", s.withPermission("subscriptions:create", s.handleCreateSubscription))
		subscriptions.GET("/:subscriptionId", s.withPermission("subscriptions:read", s.handleGetSubscription))
		subscriptions.PUT("/:subscriptionId", s.withPermission("subscriptions:create", s.handleUpdateSubscription))
		subscriptions.DELETE("/:subscriptionId", s.withPermission("subscriptions:delete", s.handleDeleteSubscription))
	}

	// Resource Pool Management
	// Endpoint: /resourcePools
	resourcePools := v1.Group("/resourcePools")
	{
		resourcePools.GET("", s.withPermission("resourcePools:read", s.handleListResourcePools))
		resourcePools.POST("", s.withPermission("resourcePools:create", s.handleCreateResourcePool))
		resourcePools.GET("/:resourcePoolId", s.withPermission("resourcePools:read", s.handleGetResourcePool))
		resourcePools.PUT("/:resourcePoolId", s.withPermission("resourcePools:update", s.handleUpdateResourcePool))
		resourcePools.DELETE("/:resourcePoolId", s.withPermission("resourcePools:delete", s.handleDeleteResourcePool))
		resourcePools.GET("/:resourcePoolId/resources", s.withPermission("resourcePools:read", s.handleListResourcesInPool))
	}

	// Resource Management
	// Endpoint: /resources
	resources := v1.Group("/resources")
	{
		resources.GET("", s.withPermission("resources:read", s.handleListResources))
		resources.POST("", s.withPermission("resources:create", s.handleCreateResource))
		resources.GET("/:resourceId", s.withPermission("resources:read", s.handleGetResource))
		resources.PUT("/:resourceId", s.withPermission("resources:update", s.handleUpdateResource))
		resources.DELETE("/:resourceId", s.withPermission("resources:delete", s.handleDeleteResource))
	}

	// Resource Type Management
	// Endpoint: /resourceTypes
	resourceTypes := v1.Group("/resourceTypes")
	{
		resourceTypes.GET("", s.withPermission("resourceTypes:read", s.handleListResourceTypes))
		resourceTypes.GET("/:resourceTypeId", s.withPermission("resourceTypes:read", s.handleGetResourceType))
	}

	// Deployment Manager Management
	// Endpoint: /deploymentManagers
	deploymentManagers := v1.Group("/deploymentManagers")
	{
		deploymentManagers.GET("",
			s.withPermission("deploymentManagers:read", s.handleListDeploymentManagers))
		deploymentManagers.GET("/:deploymentManagerId",
			s.withPermission("deploymentManagers:read", s.handleGetDeploymentManager))
	}

	// O-Cloud Infrastructure Information
	// Endpoint: /oCloudInfrastructure
	v1.GET("/oCloudInfrastructure", s.withPermission("deploymentManagers:read", s.handleGetOCloudInfrastructure))

	// Batch Operations
	// Endpoint: /batch/*
	batch := v1.Group("/batch")
	{
		// Batch subscription operations
		batch.POST("/subscriptions", s.withPermission("subscriptions:create", s.batchHandler.BatchCreateSubscriptions))
		batch.POST("/subscriptions/delete", s.withPermission("subscriptions:delete", s.batchHandler.BatchDeleteSubscriptions))
		batch.POST("/subscriptions/update", s.withPermission("subscriptions:create", s.batchHandler.BatchUpdateSubscriptions))

		// Batch resource pool operations
		batch.POST("/resourcePools", s.withPermission("resourcePools:create", s.batchHandler.BatchCreateResourcePools))
		batch.POST("/resourcePools/delete", s.withPermission("resourcePools:delete", s.batchHandler.BatchDeleteResourcePools))
		batch.POST("/resourcePools/update", s.withPermission("resourcePools:update", s.batchHandler.BatchUpdateResourcePools))

		// Batch resource operations
		batch.POST("/resources", s.withPermission("resources:create", s.batchHandler.BatchCreateResources))
		batch.POST("/resources/delete", s.withPermission("resources:delete", s.batchHandler.BatchDeleteResources))
		batch.POST("/resources/update", s.withPermission("resources:update", s.batchHandler.BatchUpdateResources))
	}

	// Tenant Management (only if multi-tenancy enabled).
	//
	// Security: cross-tenant access is enforced via withTenantAccess, which
	// requires the authenticated caller to either be a platform admin OR to
	// match the :tenantId path parameter. A tenant-admin user for tenant A
	// must NOT be able to read/update/delete tenant B.
	//
	// - ListTenants / CreateTenant are inherently cross-tenant and must be
	//   restricted to platform admins via RequirePlatformAdmin (tenants:read /
	//   tenants:create permissions alone are insufficient because tenant-scoped
	//   roles could otherwise enumerate or create arbitrary tenants).
	if s.tenantHandler != nil {
		tenants := v1.Group("/tenants")
		{
			tenants.GET("",
				s.withPlatformAdmin("tenants:read", s.tenantHandler.ListTenants))
			tenants.POST("",
				s.withPlatformAdmin("tenants:create", s.tenantHandler.CreateTenant))
			tenants.GET("/:tenantId",
				s.withTenantAccess("tenants:read", "tenantId", s.tenantHandler.GetTenant))
			tenants.PUT("/:tenantId",
				s.withTenantAccess("tenants:update", "tenantId", s.tenantHandler.UpdateTenant))
			tenants.DELETE("/:tenantId",
				s.withTenantAccess("tenants:delete", "tenantId", s.tenantHandler.DeleteTenant))
			tenants.GET("/:tenantId/quotas",
				s.withTenantAccess("tenants:read", "tenantId", s.handleGetTenantQuotas))
			tenants.PUT("/:tenantId/quotas",
				s.withTenantAccess("tenants:update", "tenantId", s.handleUpdateTenantQuotas))
		}
	}

	// API version endpoint — served from the authenticated O2 router, so the
	// detailed descriptor is scoped to callers that have established mTLS.
	v1.GET("", s.handleAPIInfoDetailed)
}
