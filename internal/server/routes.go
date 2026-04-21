package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/auth"
	"github.com/piwi3910/netweave/internal/backend"
	"github.com/piwi3910/netweave/internal/models"
	"github.com/piwi3910/netweave/internal/security/urlredact"
	"github.com/piwi3910/netweave/internal/storage"
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

	if s.tenantHandler != nil {
		v1.Use(TenantMiddleware())
	}

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

// Health check handlers

// handleHealth returns the health status of the server.
// This endpoint is used by load balancers and monitoring systems.
func (s *Server) handleHealth(c *gin.Context) {
	health := s.healthCheck.CheckHealth(c.Request.Context())

	statusCode := http.StatusOK
	if health.Status == "unhealthy" {
		statusCode = http.StatusServiceUnavailable
	}

	c.JSON(statusCode, health)
}

// handleReadiness returns the readiness status of the server.
// This endpoint checks if the server is ready to accept traffic.
func (s *Server) handleReadiness(c *gin.Context) {
	readiness := s.healthCheck.CheckReadiness(c.Request.Context())

	statusCode := http.StatusOK
	if !readiness.Ready {
		statusCode = http.StatusServiceUnavailable
	}

	c.JSON(statusCode, readiness)
}

// handleMetrics serves Prometheus metrics.
func (s *Server) handleMetrics(c *gin.Context) {
	handler := promhttp.Handler()
	handler.ServeHTTP(c.Writer, c.Request)
}

// API information handlers

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

// Subscription handlers

// handleListSubscriptions lists all subscriptions.
// GET /o2ims/v1/subscriptions.
func (s *Server) handleListSubscriptions(c *gin.Context) {
	ctx := c.Request.Context()

	// Extract tenant ID from authenticated context for tenant isolation
	tenantID := auth.TenantIDFromContext(ctx)

	s.logger.Info("listing subscriptions",
		zap.String("tenant_id", tenantID))

	// Get subscriptions from storage with tenant isolation
	var subs []*storage.Subscription
	var err error

	if tenantID != "" && !auth.IsPlatformAdminFromContext(ctx) {
		// Regular tenant user: only see their own subscriptions
		subs, err = s.store.ListByTenant(ctx, tenantID)
	} else {
		// Platform admin or no auth: see all subscriptions
		subs, err = s.store.List(ctx)
	}

	if err != nil {
		s.logger.Error("failed to list subscriptions", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to retrieve subscriptions",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	// Convert to adapter subscriptions for response
	result := make([]*adapter.Subscription, 0, len(subs))
	for _, sub := range subs {
		result = append(result, &adapter.Subscription{
			SubscriptionID:         sub.ID,
			Callback:               sub.Callback,
			ConsumerSubscriptionID: sub.ConsumerSubscriptionID,
			Filter: &adapter.SubscriptionFilter{
				ResourcePoolID: sub.Filter.ResourcePoolID,
				ResourceTypeID: sub.Filter.ResourceTypeID,
				ResourceID:     sub.Filter.ResourceID,
			},
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"subscriptions": result,
		"total":         len(result),
	})
}

// handleCreateSubscription creates a new subscription.
// POST /o2ims/v1/subscriptions.
func (s *Server) handleCreateSubscription(c *gin.Context) {
	ctx := c.Request.Context()

	// Extract tenant ID from authenticated context
	tenantID := auth.TenantIDFromContext(ctx)

	s.logger.Info("creating subscription",
		zap.String("tenant_id", tenantID))

	var req adapter.Subscription
	if err := c.ShouldBindJSON(&req); err != nil {
		s.writeClientError(c, http.StatusBadRequest, "BadRequest",
			"invalid request body",
			"failed to parse subscription request body",
			zap.Error(err),
		)
		return
	}

	// Validate callback URL early for fast failure (SSRF protection).
	// Validator errors can include URL details — keep them out of the
	// client response and log them with the request_id for operators.
	if err := s.ValidateCallback(ctx, &req); err != nil {
		s.writeClientError(c, http.StatusBadRequest, "BadRequest",
			"invalid callback URL",
			"callback URL validation failed",
			zap.Error(err),
		)
		return
	}

	// Check tenant quota before creating subscription
	if tenantID != "" && s.AuthStore != nil {
		if err := s.AuthStore.IncrementUsage(ctx, tenantID, "subscriptions"); err != nil {
			if errors.Is(err, auth.ErrQuotaExceeded) {
				s.logger.Warn("subscription quota exceeded",
					zap.String("tenant_id", tenantID))
				c.JSON(http.StatusTooManyRequests, gin.H{
					"error":   "QuotaExceeded",
					"message": "Subscription quota exceeded for tenant",
					"code":    http.StatusTooManyRequests,
				})
				return
			}
			s.logger.Error("failed to check subscription quota",
				zap.String("tenant_id", tenantID),
				zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "InternalError",
				"message": "Failed to check subscription quota",
				"code":    http.StatusInternalServerError,
			})
			return
		}
	}

	// Generate subscription ID
	req.SubscriptionID = "sub-" + uuid.New().String()

	// Create subscription via adapter

	adp := s.resolveAdapter(c)
	if adp == nil {
		return
	}

	created, err := adp.CreateSubscription(ctx, &req)
	if err != nil {
		// Audit log the failure
		if s.auditLogger != nil {
			user := auth.UserFromContext(ctx)
			s.auditLogger.LogSubscriptionOperation(
				ctx,
				auth.AuditEventSubscriptionCreated,
				req.SubscriptionID,
				req.Callback,
				user,
				map[string]string{
					"error":     err.Error(),
					"tenant_id": tenantID,
				},
			)
		}

		// Rollback quota increment on failure
		if tenantID != "" && s.AuthStore != nil {
			if decErr := s.AuthStore.DecrementUsage(ctx, tenantID, "subscriptions"); decErr != nil {
				s.logger.Error("failed to rollback subscription quota",
					zap.String("tenant_id", tenantID),
					zap.Error(decErr))
			}
		}

		// Check for conflict error (subscription already exists)
		if errors.Is(err, adapter.ErrSubscriptionExists) {
			c.JSON(http.StatusConflict, gin.H{
				"error":   "Conflict",
				"message": "Subscription already exists",
				"code":    http.StatusConflict,
			})
			return
		}

		s.logger.Error("failed to create subscription", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to create subscription",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	// Update subscription with tenant ID (adapter already stored it in Redis)
	storageSub := &storage.Subscription{
		ID:                     created.SubscriptionID,
		Callback:               created.Callback,
		ConsumerSubscriptionID: created.ConsumerSubscriptionID,
		TenantID:               tenantID,
	}
	if created.Filter != nil {
		storageSub.Filter = storage.SubscriptionFilter{
			ResourcePoolID: created.Filter.ResourcePoolID,
			ResourceTypeID: created.Filter.ResourceTypeID,
			ResourceID:     created.Filter.ResourceID,
		}
	}

	if err := s.store.Update(ctx, storageSub); err != nil {
		s.logger.Error("failed to update subscription with tenant ID", zap.Error(err))
		// Attempt to clean up adapter subscription (best effort)
		_ = adp.DeleteSubscription(ctx, created.SubscriptionID)
		// Rollback quota increment
		if tenantID != "" && s.AuthStore != nil {
			if decErr := s.AuthStore.DecrementUsage(ctx, tenantID, "subscriptions"); decErr != nil {
				s.logger.Error("failed to rollback subscription quota",
					zap.String("tenant_id", tenantID),
					zap.Error(decErr))
			}
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to store subscription",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	s.logger.Info("subscription created",
		zap.String("subscription_id", created.SubscriptionID),
		zap.String("callback", urlredact.Redact(created.Callback)))

	// Audit log the successful creation
	if s.auditLogger != nil {
		user := auth.UserFromContext(ctx)
		s.auditLogger.LogSubscriptionOperation(
			ctx,
			auth.AuditEventSubscriptionCreated,
			created.SubscriptionID,
			created.Callback,
			user,
			map[string]string{
				"consumer_subscription_id": created.ConsumerSubscriptionID,
				"tenant_id":                tenantID,
			},
		)
	}

	c.JSON(http.StatusCreated, created)
}

// handleGetSubscription retrieves a specific subscription.
// GET /o2ims/v1/subscriptions/:subscriptionId.
func (s *Server) handleGetSubscription(c *gin.Context) {
	ctx := c.Request.Context()
	subscriptionID := c.Param("subscriptionId")

	// Extract tenant ID from authenticated context for tenant isolation
	tenantID := auth.TenantIDFromContext(ctx)

	s.logger.Info("getting subscription",
		zap.String("subscription_id", subscriptionID),
		zap.String("tenant_id", tenantID))

	// Get subscription from storage. Both "not found" and "forbidden
	// (different tenant)" must return the same shape so the caller cannot
	// use response variance as a cross-tenant ID oracle.
	sub, err := s.store.Get(ctx, subscriptionID)
	if err != nil {
		if errors.Is(err, storage.ErrSubscriptionNotFound) {
			s.writeNotFoundOrForbidden(c, "subscription",
				"subscription lookup returned not-found",
				zap.String("subscription_id", subscriptionID),
				zap.String("tenant_id", tenantID),
			)
			return
		}

		s.writeClientError(c, http.StatusInternalServerError, "InternalError",
			"failed to retrieve subscription",
			"failed to get subscription from storage",
			zap.String("subscription_id", subscriptionID),
			zap.Error(err),
		)
		return
	}

	// Tenant isolation: verify subscription belongs to tenant (unless platform admin).
	if tenantID != "" && !auth.IsPlatformAdminFromContext(ctx) && sub.TenantID != tenantID {
		s.writeNotFoundOrForbidden(c, "subscription",
			"tenant attempting to access subscription from different tenant",
			zap.String("tenant_id", tenantID),
			zap.String("subscription_tenant_id", sub.TenantID),
			zap.String("subscription_id", subscriptionID),
		)
		return
	}

	// Convert to adapter subscription for response
	result := &adapter.Subscription{
		SubscriptionID:         sub.ID,
		Callback:               sub.Callback,
		ConsumerSubscriptionID: sub.ConsumerSubscriptionID,
		Filter: &adapter.SubscriptionFilter{
			ResourcePoolID: sub.Filter.ResourcePoolID,
			ResourceTypeID: sub.Filter.ResourceTypeID,
			ResourceID:     sub.Filter.ResourceID,
		},
	}

	c.JSON(http.StatusOK, result)
}

// handleUpdateSubscription updates an existing subscription.
// PUT /o2ims/v1/subscriptions/:subscriptionId.
// This endpoint allows updating both the callback URL and/or subscription filters.
// When filter is null, it removes all filters; empty filter object {} also removes filters.
func (s *Server) handleUpdateSubscription(c *gin.Context) {
	ctx := c.Request.Context()
	subscriptionID := c.Param("subscriptionId")

	// Extract tenant ID from authenticated context for tenant isolation
	tenantID := auth.TenantIDFromContext(ctx)

	s.logger.Info("updating subscription",
		zap.String("subscription_id", subscriptionID),
		zap.String("tenant_id", tenantID))

	// Tenant isolation: verify subscription belongs to tenant before update.
	// Any "wrong tenant" or "missing" case returns the same not-found shape
	// so the caller cannot enumerate sibling tenants' IDs.
	if s.store != nil {
		sub, err := s.store.Get(ctx, subscriptionID)
		if err != nil {
			if errors.Is(err, storage.ErrSubscriptionNotFound) {
				s.writeNotFoundOrForbidden(c, "subscription",
					"subscription lookup returned not-found during update",
					zap.String("subscription_id", subscriptionID),
					zap.String("tenant_id", tenantID),
				)
				return
			}
			s.writeClientError(c, http.StatusInternalServerError, "InternalError",
				"failed to verify subscription ownership",
				"failed to get subscription for tenant check",
				zap.String("subscription_id", subscriptionID),
				zap.Error(err),
			)
			return
		}

		if tenantID != "" && !auth.IsPlatformAdminFromContext(ctx) && sub.TenantID != tenantID {
			s.writeNotFoundOrForbidden(c, "subscription",
				"tenant attempting to update subscription from different tenant",
				zap.String("tenant_id", tenantID),
				zap.String("subscription_tenant_id", sub.TenantID),
				zap.String("subscription_id", subscriptionID),
			)
			return
		}
	}

	var req adapter.Subscription
	if err := c.ShouldBindJSON(&req); err != nil {
		s.writeClientError(c, http.StatusBadRequest, "BadRequest",
			"invalid request body",
			"failed to parse subscription update body",
			zap.Error(err),
		)
		return
	}

	if err := s.ValidateCallback(ctx, &req); err != nil {
		s.writeClientError(c, http.StatusBadRequest, "BadRequest",
			"invalid callback URL",
			"callback URL validation failed during update",
			zap.Error(err),
		)
		return
	}

	// Update subscription via adapter.

	adp := s.resolveAdapter(c)
	if adp == nil {
		return
	}

	updated, err := adp.UpdateSubscription(c.Request.Context(), subscriptionID, &req)
	if err != nil {
		if errors.Is(err, adapter.ErrSubscriptionNotFound) {
			s.writeNotFoundOrForbidden(c, "subscription",
				"adapter reported subscription not found during update",
				zap.String("subscription_id", subscriptionID),
			)
			return
		}

		s.writeClientError(c, http.StatusInternalServerError, "InternalError",
			"failed to update subscription",
			"adapter failed to update subscription",
			zap.String("subscription_id", subscriptionID),
			zap.Error(err),
		)
		return
	}

	s.logger.Info("subscription updated",
		zap.String("subscription_id", subscriptionID),
		zap.String("callback", urlredact.Redact(updated.Callback)))

	// Log audit event for subscription update
	s.logAuditEvent(
		c.Request.Context(),
		c,
		auth.AuditEventResourceModified,
		"subscription",
		subscriptionID,
		"subscription_updated",
		map[string]string{
			"callback": updated.Callback,
		},
	)

	c.JSON(http.StatusOK, updated)
}

// handleDeleteSubscription deletes a subscription.
// DELETE /o2ims/v1/subscriptions/:subscriptionId.
func (s *Server) handleDeleteSubscription(c *gin.Context) {
	ctx := c.Request.Context()
	subscriptionID := c.Param("subscriptionId")

	// Extract tenant ID from authenticated context
	tenantID := auth.TenantIDFromContext(ctx)

	s.logger.Info("deleting subscription",
		zap.String("subscription_id", subscriptionID),
		zap.String("tenant_id", tenantID))

	// Get subscription to extract tenant ID for quota tracking and tenant isolation check
	var storedTenantID string
	if s.store != nil {
		sub, err := s.store.Get(ctx, subscriptionID)
		if err == nil {
			storedTenantID = sub.TenantID

			// Tenant isolation: identical shape for "not found" and "wrong tenant".
			if tenantID != "" && !auth.IsPlatformAdminFromContext(ctx) && sub.TenantID != tenantID {
				s.writeNotFoundOrForbidden(c, "subscription",
					"tenant attempting to delete subscription from different tenant",
					zap.String("tenant_id", tenantID),
					zap.String("subscription_tenant_id", sub.TenantID),
					zap.String("subscription_id", subscriptionID),
				)
				return
			}
		} else if errors.Is(err, storage.ErrSubscriptionNotFound) {
			s.writeNotFoundOrForbidden(c, "subscription",
				"subscription lookup returned not-found during delete",
				zap.String("subscription_id", subscriptionID),
			)
			return
		}
	}

	// Delete from adapter

	adp := s.resolveAdapter(c)
	if adp == nil {
		return
	}

	if err := adp.DeleteSubscription(ctx, subscriptionID); err != nil {
		// Audit log the failure
		if s.auditLogger != nil {
			user := auth.UserFromContext(ctx)
			s.auditLogger.LogSubscriptionOperation(
				ctx,
				auth.AuditEventSubscriptionDeleted,
				subscriptionID,
				"",
				user,
				map[string]string{
					"error":     err.Error(),
					"tenant_id": storedTenantID,
				},
			)
		}

		s.writeClientError(c, http.StatusInternalServerError, "InternalError",
			"failed to delete subscription",
			"adapter failed to delete subscription",
			zap.String("subscription_id", subscriptionID),
			zap.Error(err),
		)
		return
	}

	// Delete from storage.
	if err := s.store.Delete(ctx, subscriptionID); err != nil {
		if s.auditLogger != nil {
			user := auth.UserFromContext(ctx)
			s.auditLogger.LogSubscriptionOperation(
				ctx,
				auth.AuditEventSubscriptionDeleted,
				subscriptionID,
				"",
				user,
				map[string]string{
					"error":     err.Error(),
					"tenant_id": storedTenantID,
				},
			)
		}

		if errors.Is(err, storage.ErrSubscriptionNotFound) {
			s.writeNotFoundOrForbidden(c, "subscription",
				"storage reported subscription not found during delete",
				zap.String("subscription_id", subscriptionID),
			)
			return
		}

		s.writeClientError(c, http.StatusInternalServerError, "InternalError",
			"failed to delete subscription",
			"storage failed to delete subscription",
			zap.String("subscription_id", subscriptionID),
			zap.Error(err),
		)
		return
	}

	// Decrement tenant quota after successful deletion
	if storedTenantID != "" && s.AuthStore != nil {
		if err := s.AuthStore.DecrementUsage(ctx, storedTenantID, "subscriptions"); err != nil {
			s.logger.Error("failed to decrement subscription quota",
				zap.String("tenant_id", storedTenantID),
				zap.Error(err))
			// Don't fail the delete operation if quota decrement fails
		}
	}

	s.logger.Info("subscription deleted", zap.String("subscription_id", subscriptionID))

	// Audit log the successful deletion
	if s.auditLogger != nil {
		user := auth.UserFromContext(ctx)
		s.auditLogger.LogSubscriptionOperation(
			ctx,
			auth.AuditEventSubscriptionDeleted,
			subscriptionID,
			"",
			user,
			map[string]string{
				"tenant_id": storedTenantID,
			},
		)
	}

	c.Status(http.StatusNoContent)
}

// parseFilterFromRequest parses filter parameters from the request context.
// It detects the API version and uses AdvancedFilter parsing for v2+ endpoints.
// Returns an adapter.Filter with tenant context applied.
//
// SECURITY: platform admins receive an empty tenant filter so they can see
// every tenant's resources. Non-admin authenticated callers are pinned to
// their own tenant. Unauthenticated callers (auth disabled) get no tenant
// filter — they rely on other controls (e.g., network isolation).
func (s *Server) parseFilterFromRequest(c *gin.Context) (*adapter.Filter, error) {
	// Detect API version from request path.
	path := c.Request.URL.Path
	isV2OrHigher := strings.Contains(path, "/v2/") || strings.Contains(path, "/v3/")

	// Extract tenant ID if present (v3+ with multi-tenancy). Platform admins
	// bypass the tenant filter so they see resources across tenants.
	ctx := c.Request.Context()
	var tenantID string
	if !auth.IsPlatformAdminFromContext(ctx) {
		tenantID = auth.TenantIDFromContext(ctx)
	}

	if isV2OrHigher {
		// Parse advanced filter for v2+ endpoints.
		advFilter, err := models.ParseAdvancedFilter(c.Request.URL.Query())
		if err != nil {
			return nil, fmt.Errorf("invalid filter parameters: %w", err)
		}

		// Create adapter filter with advanced filtering support.
		return &adapter.Filter{
			TenantID:       tenantID,
			Limit:          advFilter.Limit,
			Offset:         advFilter.Offset,
			AdvancedFilter: advFilter,
		}, nil
	}

	// For v1, create basic filter (no advanced features).
	return &adapter.Filter{
		TenantID: tenantID,
		Limit:    100, // Default limit for v1.
	}, nil
}

// Resource Pool handlers

// handleListResourcePools lists all resource pools.
// GET /o2ims/v1/resourcePools, /v2/resourcePools, /v3/resourcePools.
func (s *Server) handleListResourcePools(c *gin.Context) {
	s.logger.Info("listing resource pools")

	// Parse filter from request (supports v1 basic and v2+ advanced filtering).
	filter, err := s.parseFilterFromRequest(c)
	if err != nil {
		s.logger.Error("failed to parse filter", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "InvalidParameter",
			"message": err.Error(),
			"code":    http.StatusBadRequest,
		})
		return
	}

	// List resource pools via adapter.

	adp := s.resolveAdapter(c)
	if adp == nil {
		return
	}

	pools, err := adp.ListResourcePools(c.Request.Context(), filter)
	if err != nil {
		s.logger.Error("failed to list resource pools", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to retrieve resource pools",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"resourcePools": pools,
		"total":         len(pools),
	})
}

// handleGetResourcePool retrieves a specific resource pool.
// GET /o2ims/v1/resourcePools/:resourcePoolId.
func (s *Server) handleGetResourcePool(c *gin.Context) {
	resourcePoolID := c.Param("resourcePoolId")
	s.logger.Info("getting resource pool", zap.String("resource_pool_id", resourcePoolID))

	// Get resource pool via adapter

	adp := s.resolveAdapter(c)
	if adp == nil {
		return
	}

	pool, err := adp.GetResourcePool(c.Request.Context(), resourcePoolID)
	if err != nil {
		s.writeNotFoundOrForbidden(c, "resource pool",
			"adapter failed to fetch resource pool",
			zap.String("resource_pool_id", resourcePoolID),
			zap.Error(err),
		)
		return
	}

	// Tenant isolation: verify pool belongs to tenant (unless platform admin).
	// SECURITY: fail-closed via auth.AuthorizeTenantResource — a pool with
	// empty TenantID is inaccessible to non-platform-admin callers (issue #470).
	ctx := c.Request.Context()
	if !auth.AuthorizeTenantResource(ctx, pool.TenantID) {
		s.writeNotFoundOrForbidden(c, "resource pool",
			"tenant attempting to access resource pool from different tenant",
			zap.String("tenant_id", auth.TenantIDFromContext(ctx)),
			zap.String("pool_tenant_id", pool.TenantID),
			zap.String("resource_pool_id", resourcePoolID),
		)
		return
	}

	c.JSON(http.StatusOK, pool)
}

// handleListResourcesInPool lists resources in a specific pool.
// GET /o2ims/v1/resourcePools/:resourcePoolId/resources.
func (s *Server) handleListResourcesInPool(c *gin.Context) {
	resourcePoolID := c.Param("resourcePoolId")
	s.logger.Info("listing resources in pool", zap.String("resource_pool_id", resourcePoolID))

	// Create filter for this resource pool
	filter := &adapter.Filter{
		ResourcePoolID: resourcePoolID,
	}

	// Apply tenant filter for multi-tenancy isolation
	ctx := c.Request.Context()
	tenantID := auth.TenantIDFromContext(ctx)
	if tenantID != "" && !auth.IsPlatformAdminFromContext(ctx) {
		filter.TenantID = tenantID
	}

	// List resources via adapter

	adp := s.resolveAdapter(c)
	if adp == nil {
		return
	}

	resources, err := adp.ListResources(ctx, filter)
	if err != nil {
		s.logger.Error("failed to list resources in pool", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to retrieve resources",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"resources": resources,
		"total":     len(resources),
	})
}

// Validation constants for resource pool fields.
const (
	// MaxResourcePoolNameLength is the maximum allowed length for resource pool names.
	MaxResourcePoolNameLength = 255

	// MaxResourcePoolIDLength is the maximum allowed length for resource pool IDs.
	MaxResourcePoolIDLength = 255

	// MaxResourcePoolDescriptionLength is the maximum allowed length for resource pool descriptions.
	MaxResourcePoolDescriptionLength = 1000
)

// Validation constants for resource extension fields.
const (
	// MaxExtensionKeys is the maximum number of extension keys allowed.
	MaxExtensionKeys = 100

	// MaxExtensionKeyLength is the maximum length for an extension key.
	MaxExtensionKeyLength = 256

	// MaxExtensionValueSize is the maximum size for a single extension value when JSON-encoded.
	MaxExtensionValueSize = 4096

	// MaxExtensionsTotalSize is the maximum total size for all extensions combined (50KB).
	MaxExtensionsTotalSize = 50000
)

// SanitizeResourcePoolID sanitizes a string for use in resource pool IDs.
// Removes special characters that could cause security issues (path traversal, injection).
// Spaces and slashes are replaced with hyphens, all other special characters are dropped.
func SanitizeResourcePoolID(name string) string {
	var result strings.Builder
	for _, ch := range name {
		if isAlphanumericOrAllowed(ch) {
			result.WriteRune(ch)
		} else if ch == ' ' || ch == '/' {
			result.WriteRune('-') // Only replace spaces and slashes with hyphens
		}
		// All other special characters are simply dropped for security
	}

	return strings.ToLower(result.String())
}

// isAlphanumericOrAllowed checks if a rune is alphanumeric, hyphen, or underscore.
func isAlphanumericOrAllowed(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9') || ch == '-' || ch == '_'
}

// SanitizeForLogging removes CRLF characters to prevent log injection attacks.
// This prevents attackers from injecting fake log entries via user-controlled input.
func SanitizeForLogging(s string) string {
	// Remove CR, LF, and other control characters
	sanitized := strings.NewReplacer(
		"\r", "",
		"\n", "",
		"\t", " ",
	).Replace(s)

	// Remove any remaining control characters (ASCII 0-31 except space)
	var result strings.Builder
	for _, ch := range sanitized {
		if ch >= 32 || ch == ' ' {
			result.WriteRune(ch)
		}
	}

	return result.String()
}

// isValidIDCharacter checks if a character is valid for resource pool IDs.
func isValidIDCharacter(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9') || ch == '-' || ch == '_'
}

// validateResourcePoolID validates the resource pool ID format.
func validateResourcePoolID(id string) error {
	if len(id) > MaxResourcePoolIDLength {
		return fmt.Errorf("resourcePoolId must not exceed %d characters", MaxResourcePoolIDLength)
	}

	for _, ch := range id {
		if !isValidIDCharacter(ch) {
			return errors.New("resourcePoolId must contain only alphanumeric characters, hyphens, and underscores")
		}
	}

	return nil
}

// ValidateResourcePoolFields validates required fields in a ResourcePool.
func ValidateResourcePoolFields(pool *adapter.ResourcePool) error {
	var validationErrors []string

	// Validate Name is required
	if pool.Name == "" {
		validationErrors = append(validationErrors, "name is required")
	}

	// Validate Name length
	if len(pool.Name) > MaxResourcePoolNameLength {
		validationErrors = append(validationErrors,
			fmt.Sprintf("name must not exceed %d characters", MaxResourcePoolNameLength))
	}

	// Validate ResourcePoolID if provided
	if pool.ResourcePoolID != "" {
		if err := validateResourcePoolID(pool.ResourcePoolID); err != nil {
			validationErrors = append(validationErrors, err.Error())
		}
	}

	// Validate Description length if provided
	if len(pool.Description) > MaxResourcePoolDescriptionLength {
		validationErrors = append(validationErrors,
			fmt.Sprintf("description must not exceed %d characters", MaxResourcePoolDescriptionLength))
	}

	// Return all validation errors together
	if len(validationErrors) > 0 {
		return errors.New(strings.Join(validationErrors, "; "))
	}

	return nil
}

// handleCreateResourcePool creates a new resource pool.
// POST /o2ims/v1/resourcePools.
func (s *Server) handleCreateResourcePool(c *gin.Context) {
	s.logger.Info("creating resource pool")

	var req adapter.ResourcePool
	if err := c.ShouldBindJSON(&req); err != nil {
		s.writeClientError(c, http.StatusBadRequest, "BadRequest",
			"invalid request body",
			"failed to parse request body",
			zap.Error(err),
		)
		return
	}

	// Validate resource pool fields
	if err := ValidateResourcePoolFields(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": err.Error(),
			"code":    http.StatusBadRequest,
		})
		return
	}

	// Check tenant quota before creating resource pool
	ctx := c.Request.Context()
	tenantID := auth.TenantIDFromContext(ctx)
	if tenantID != "" && s.AuthStore != nil {
		if err := s.AuthStore.IncrementUsage(ctx, tenantID, "resource_pools"); err != nil {
			if errors.Is(err, auth.ErrQuotaExceeded) {
				s.logger.Warn("resource pool quota exceeded",
					zap.String("tenant_id", tenantID))
				c.JSON(http.StatusTooManyRequests, gin.H{
					"error":   "QuotaExceeded",
					"message": "Resource pool quota exceeded for tenant",
					"code":    http.StatusTooManyRequests,
				})
				return
			}
			s.logger.Error("failed to check resource pool quota",
				zap.String("tenant_id", tenantID),
				zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "InternalError",
				"message": "Failed to check resource pool quota",
				"code":    http.StatusInternalServerError,
			})
			return
		}
	}

	// Set tenant ID on the resource pool for isolation
	if tenantID != "" {
		req.TenantID = tenantID
	}

	// Generate resource pool ID if not provided (sanitized with UUID for uniqueness)
	// Format: pool-{sanitized-name}-{uuid}
	// Example: "GPU Pool (Production)" → "pool-gpu-pool-production-a1b2c3d4-e5f6-7890-abcd-1234567890ab"
	if req.ResourcePoolID == "" {
		sanitizedName := SanitizeResourcePoolID(req.Name)
		// Clean up consecutive hyphens that can occur from sanitization
		sanitizedName = strings.ReplaceAll(sanitizedName, "--", "-")
		sanitizedName = strings.Trim(sanitizedName, "-")

		// Ensure total length doesn't exceed 255 chars (per O2-IMS spec)
		// UUID is 36 chars, "pool-" is 5 chars, we need 2 chars for separating hyphens = 43 chars reserved
		// This leaves 212 chars for the sanitized name
		maxNameLength := 212
		if len(sanitizedName) > maxNameLength {
			sanitizedName = sanitizedName[:maxNameLength]
		}

		req.ResourcePoolID = "pool-" + sanitizedName + "-" + uuid.New().String()
	}

	// Create resource pool via adapter

	adp := s.resolveAdapter(c)
	if adp == nil {
		return
	}

	created, err := adp.CreateResourcePool(c.Request.Context(), &req)
	if err != nil {
		// Rollback quota increment on creation failure
		if tenantID != "" && s.AuthStore != nil {
			if rollbackErr := s.AuthStore.DecrementUsage(ctx, tenantID, "resource_pools"); rollbackErr != nil {
				s.logger.Error("failed to rollback resource pool quota after creation failure",
					zap.String("tenant_id", tenantID),
					zap.Error(rollbackErr))
			}
		}

		// Audit log the failure
		if s.auditLogger != nil {
			user := auth.UserFromContext(c.Request.Context())
			s.auditLogger.LogResourceOperation(
				c.Request.Context(),
				auth.AuditEventResourcePoolCreated,
				"resourcepool",
				req.ResourcePoolID,
				user,
				false,
				map[string]string{
					"name":  req.Name,
					"error": err.Error(),
				},
			)
		}

		// Check for duplicate resource pool using sentinel error
		if errors.Is(err, adapter.ErrResourcePoolExists) {
			c.JSON(http.StatusConflict, gin.H{
				"error":   "Conflict",
				"message": "Resource pool with ID " + SanitizeForLogging(req.ResourcePoolID) + " already exists",
				"code":    http.StatusConflict,
			})
			return
		}

		s.logger.Error("failed to create resource pool", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to create resource pool",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	s.logger.Info("resource pool created",
		zap.String("resource_pool_id", created.ResourcePoolID),
		zap.String("name", SanitizeForLogging(created.Name)))

	// Audit log the successful creation
	if s.auditLogger != nil {
		user := auth.UserFromContext(c.Request.Context())
		s.auditLogger.LogResourceOperation(
			c.Request.Context(),
			auth.AuditEventResourcePoolCreated,
			"resourcepool",
			created.ResourcePoolID,
			user,
			true,
			map[string]string{
				"name": created.Name,
			},
		)
	}

	// Set Location header for REST compliance
	c.Header("Location", "/o2ims/v1/resourcePools/"+created.ResourcePoolID)
	c.JSON(http.StatusCreated, created)
}

// handleUpdateResourcePool updates an existing resource pool.
// PUT /o2ims/v1/resourcePools/:resourcePoolId.
func (s *Server) handleUpdateResourcePool(c *gin.Context) {
	resourcePoolID := c.Param("resourcePoolId")
	s.logger.Info("updating resource pool", zap.String("resource_pool_id", resourcePoolID))

	var req adapter.ResourcePool
	if err := c.ShouldBindJSON(&req); err != nil {
		s.writeClientError(c, http.StatusBadRequest, "BadRequest",
			"invalid request body",
			"failed to parse request body",
			zap.Error(err),
		)
		return
	}

	// Validate field constraints
	if err := ValidateResourcePoolFields(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": err.Error(),
			"code":    http.StatusBadRequest,
		})
		return
	}

	// Update resource pool via adapter.

	adp := s.resolveAdapter(c)
	if adp == nil {
		return
	}

	ctx := c.Request.Context()
	// SECURITY: verify tenant ownership before update. Without this check a
	// non-platform-admin caller with resourcePools:update could modify any
	// tenant's pool. Fail-closed via auth.AuthorizeTenantResource so that
	// pools with empty TenantID cannot be reached by non-admin callers
	// (see issue #470 / issue #482).
	//
	// The check is gated on the presence of an authenticated user: when auth
	// middleware is not configured (local dev / tests), tenant enforcement
	// is deferred to deployment-level controls.
	if user := auth.UserFromContext(ctx); user != nil && !user.IsPlatformAdmin {
		existing, getErr := adp.GetResourcePool(ctx, resourcePoolID)
		if getErr != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "NotFound",
				"message": "Resource pool not found: " + resourcePoolID,
				"code":    http.StatusNotFound,
			})
			return
		}
		if !auth.AuthorizeTenantResource(ctx, existing.TenantID) {
			s.logger.Warn("tenant attempting to update resource pool from different tenant",
				zap.String("tenant_id", user.TenantID),
				zap.String("pool_tenant_id", existing.TenantID),
				zap.String("resource_pool_id", resourcePoolID))
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "NotFound",
				"message": "Resource pool not found: " + resourcePoolID,
				"code":    http.StatusNotFound,
			})
			return
		}
		// Preserve the existing tenant stamp; clients cannot move a pool
		// between tenants via update.
		req.TenantID = existing.TenantID
	}

	updated, err := adp.UpdateResourcePool(c.Request.Context(), resourcePoolID, &req)
	if err != nil {
		// Audit log the failure
		if s.auditLogger != nil {
			user := auth.UserFromContext(c.Request.Context())
			s.auditLogger.LogResourceOperation(
				c.Request.Context(),
				auth.AuditEventResourcePoolModified,
				"resourcepool",
				resourcePoolID,
				user,
				false,
				map[string]string{
					"name":  req.Name,
					"error": err.Error(),
				},
			)
		}

		if errors.Is(err, adapter.ErrResourcePoolNotFound) {
			s.writeNotFoundOrForbidden(c, "resource pool",
				"adapter reported resource pool not found during update",
				zap.String("resource_pool_id", resourcePoolID),
			)
			return
		}

		s.writeClientError(c, http.StatusInternalServerError, "InternalError",
			"failed to update resource pool",
			"adapter failed to update resource pool",
			zap.String("resource_pool_id", resourcePoolID),
			zap.Error(err),
		)
		return
	}

	s.logger.Info("resource pool updated",
		zap.String("resource_pool_id", updated.ResourcePoolID),
		zap.String("name", SanitizeForLogging(updated.Name)))

	// Audit log the successful update
	if s.auditLogger != nil {
		user := auth.UserFromContext(c.Request.Context())
		s.auditLogger.LogResourceOperation(
			c.Request.Context(),
			auth.AuditEventResourcePoolModified,
			"resourcepool",
			updated.ResourcePoolID,
			user,
			true,
			map[string]string{
				"name": updated.Name,
			},
		)
	}

	c.JSON(http.StatusOK, updated)
}

// handleDeleteResourcePool deletes a resource pool.
// DELETE /o2ims/v1/resourcePools/:resourcePoolId.
func (s *Server) handleDeleteResourcePool(c *gin.Context) {
	resourcePoolID := c.Param("resourcePoolId")
	ctx := c.Request.Context()
	tenantID := auth.TenantIDFromContext(ctx)

	// Verify tenant ownership before deletion.

	adp := s.resolveAdapter(c)
	if adp == nil {
		return
	}

	// SECURITY: fail-closed tenant ownership check. A pool with empty
	// TenantID is inaccessible to non-platform-admin callers. This prevents
	// the bypass described in issue #470 (C7) where adapters or legacy
	// data without a tenant stamp could be deleted from any tenant context.
	//
	// Gated on the presence of an authenticated user so that non-auth
	// deployments (e.g., dev/test) continue to work — production must
	// always configure auth middleware upstream.
	if user := auth.UserFromContext(ctx); user != nil && !user.IsPlatformAdmin {
		pool, err := adp.GetResourcePool(ctx, resourcePoolID)
		if err != nil {
			s.writeNotFoundOrForbidden(c, "resource pool",
				"resource pool lookup failed during delete authorization",
				zap.String("resource_pool_id", resourcePoolID),
				zap.String("tenant_id", tenantID),
				zap.Error(err),
			)
			return
		}
		if !auth.AuthorizeTenantResource(ctx, pool.TenantID) {
			s.writeNotFoundOrForbidden(c, "resource pool",
				"tenant attempting to delete resource pool from different tenant",
				zap.String("resource_pool_id", resourcePoolID),
				zap.String("tenant_id", tenantID),
				zap.String("pool_tenant_id", pool.TenantID),
			)
			return
		}
	}

	if err := adp.DeleteResourcePool(ctx, resourcePoolID); err != nil {
		// Audit log the failure
		if s.auditLogger != nil {
			user := auth.UserFromContext(ctx)
			s.auditLogger.LogResourceOperation(
				ctx,
				auth.AuditEventResourcePoolDeleted,
				"resourcepool",
				resourcePoolID,
				user,
				false,
				map[string]string{
					"error": err.Error(),
				},
			)
		}

		if errors.Is(err, adapter.ErrResourcePoolNotFound) {
			s.writeNotFoundOrForbidden(c, "resource pool",
				"adapter reported resource pool not found during delete",
				zap.String("resource_pool_id", resourcePoolID),
			)
			return
		}
		s.writeClientError(c, http.StatusInternalServerError, "InternalError",
			"failed to delete resource pool",
			"adapter failed to delete resource pool",
			zap.String("resource_pool_id", resourcePoolID),
			zap.Error(err),
		)
		return
	}

	// Audit log the successful deletion
	if s.auditLogger != nil {
		user := auth.UserFromContext(ctx)
		s.auditLogger.LogResourceOperation(
			ctx,
			auth.AuditEventResourcePoolDeleted,
			"resourcepool",
			resourcePoolID,
			user,
			true,
			nil,
		)
	}

	// Decrement tenant usage after successful deletion
	if tenantID != "" && s.AuthStore != nil {
		if err := s.AuthStore.DecrementUsage(ctx, tenantID, "resource_pools"); err != nil {
			s.logger.Error("failed to decrement resource pool usage",
				zap.String("tenant_id", tenantID),
				zap.Error(err))
		}
	}

	c.Status(http.StatusNoContent)
}

// Resource handlers

// isAlphanumericOrHyphen checks if a character is alphanumeric or hyphen.
func isAlphanumericOrHyphen(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-'
}

// validateURNNamespaceID validates the URN namespace identifier (nid).
func validateURNNamespaceID(nid string) error {
	if len(nid) < 2 || len(nid) > 32 {
		return errors.New("URN namespace identifier must be 2-32 characters")
	}

	for i, ch := range nid {
		if !isAlphanumericOrHyphen(ch) {
			return errors.New("URN namespace identifier must contain only alphanumeric characters and hyphens")
		}
		if i == 0 && ch == '-' {
			return errors.New("URN namespace identifier must start with alphanumeric character")
		}
	}

	return nil
}

// validateURN validates URN format according to RFC 8141.
// URN format: urn:<nid>:<nss> where:
// - nid (Namespace Identifier): 2-32 alphanumeric characters, case-insensitive.
// - nss (Namespace Specific String): at least 1 character.
func validateURN(urn string) error {
	if !strings.HasPrefix(urn, "urn:") {
		return errors.New("globalAssetId must start with 'urn:'")
	}

	parts := strings.SplitN(urn, ":", 3)
	if len(parts) < 3 {
		return errors.New("globalAssetId must be in URN format: urn:<nid>:<nss> (e.g., urn:o-ran:resource:node-001)")
	}

	if err := validateURNNamespaceID(parts[1]); err != nil {
		return err
	}

	if len(parts[2]) == 0 {
		return errors.New("URN namespace specific string must not be empty")
	}

	return nil
}

// validateExtensions validates resource extensions for size and content.
func validateExtensions(extensions map[string]interface{}) error {
	if len(extensions) > MaxExtensionKeys {
		return fmt.Errorf("extensions map must not exceed %d keys", MaxExtensionKeys)
	}

	totalSize := 0
	for key, value := range extensions {
		if len(key) > MaxExtensionKeyLength {
			return fmt.Errorf("extension keys must not exceed %d characters", MaxExtensionKeyLength)
		}
		// Check JSON-marshaled size to prevent large payloads
		valueJSON, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("extension value for key %q must be JSON-serializable", key)
		}
		if len(valueJSON) > MaxExtensionValueSize {
			return fmt.Errorf("extension values must not exceed %d bytes when JSON-encoded", MaxExtensionValueSize)
		}

		// Track total extensions payload size
		totalSize += len(valueJSON)
		if totalSize > MaxExtensionsTotalSize {
			return fmt.Errorf("total extensions payload must not exceed %d bytes (50KB)", MaxExtensionsTotalSize)
		}
	}

	return nil
}

// validateResourceFields validates resource field constraints.
func validateResourceFields(resource *adapter.Resource) error {
	// Validate GlobalAssetID format (URN) if provided
	if resource.GlobalAssetID != "" {
		if err := validateURN(resource.GlobalAssetID); err != nil {
			return err
		}
		if len(resource.GlobalAssetID) > 256 {
			return errors.New("globalAssetId must not exceed 256 characters")
		}
	}

	// Validate Description length
	if len(resource.Description) > 1000 {
		return errors.New("description must not exceed 1000 characters")
	}

	// Validate Extensions
	if resource.Extensions != nil {
		if err := validateExtensions(resource.Extensions); err != nil {
			return err
		}
	}

	return nil
}

// handleListResources lists all resources.
// GET /o2ims/v1/resources.
func (s *Server) handleListResources(c *gin.Context) {
	s.logger.Info("listing resources")

	// Parse filter from request (supports v1 basic and v2+ advanced filtering).
	filter, err := s.parseFilterFromRequest(c)
	if err != nil {
		s.logger.Error("failed to parse filter", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "InvalidParameter",
			"message": err.Error(),
			"code":    http.StatusBadRequest,
		})
		return
	}

	// List resources via adapter.

	adp := s.resolveAdapter(c)
	if adp == nil {
		return
	}

	resources, err := adp.ListResources(c.Request.Context(), filter)
	if err != nil {
		s.logger.Error("failed to list resources", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to retrieve resources",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"resources": resources,
		"total":     len(resources),
	})
}

// handleGetResource retrieves a specific resource.
// GET /o2ims/v1/resources/:resourceId.
func (s *Server) handleGetResource(c *gin.Context) {
	resourceID := c.Param("resourceId")
	s.logger.Info("getting resource", zap.String("resource_id", resourceID))

	// Get resource via adapter

	adp := s.resolveAdapter(c)
	if adp == nil {
		return
	}

	resource, err := adp.GetResource(c.Request.Context(), resourceID)
	if err != nil {
		if errors.Is(err, adapter.ErrResourceNotFound) {
			s.writeNotFoundOrForbidden(c, "resource",
				"adapter reported resource not found",
				zap.String("resource_id", resourceID),
			)
			return
		}

		s.writeClientError(c, http.StatusInternalServerError, "InternalError",
			"failed to retrieve resource",
			"adapter failed to fetch resource",
			zap.String("resource_id", resourceID),
			zap.Error(err),
		)
		return
	}

	// Tenant isolation: verify resource belongs to tenant (unless platform admin).
	// SECURITY: fail-closed via auth.AuthorizeTenantResource — a resource with
	// empty TenantID is inaccessible to non-platform-admin callers (issue #470).
	ctx := c.Request.Context()
	if !auth.AuthorizeTenantResource(ctx, resource.TenantID) {
		s.writeNotFoundOrForbidden(c, "resource",
			"tenant attempting to access resource from different tenant",
			zap.String("tenant_id", auth.TenantIDFromContext(ctx)),
			zap.String("resource_tenant_id", resource.TenantID),
			zap.String("resource_id", resourceID),
		)
		return
	}

	c.JSON(http.StatusOK, resource)
}

// validateCreateRequest validates required fields and constraints for resource creation.
func validateCreateRequest(req *adapter.Resource) error {
	if req.ResourceTypeID == "" {
		return adapter.ErrResourceTypeRequired
	}

	if req.ResourcePoolID == "" {
		return adapter.ErrResourcePoolRequired
	}

	return validateResourceFields(req)
}

// handleCreateResource creates a new resource.
// POST /o2ims/v1/resources.
func (s *Server) handleCreateResource(c *gin.Context) {
	s.logger.Info("creating resource")

	var req adapter.Resource
	if err := c.ShouldBindJSON(&req); err != nil {
		s.writeClientError(c, http.StatusBadRequest, "BadRequest",
			"invalid request body",
			"failed to parse request body",
			zap.Error(err),
		)
		return
	}

	// Validate required fields and constraints
	if err := validateCreateRequest(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": err.Error(),
			"code":    http.StatusBadRequest,
		})
		return
	}

	// Check tenant quota before creating resource
	ctx := c.Request.Context()
	tenantID := auth.TenantIDFromContext(ctx)
	if tenantID != "" && s.AuthStore != nil {
		if err := s.AuthStore.IncrementUsage(ctx, tenantID, "resources"); err != nil {
			if errors.Is(err, auth.ErrQuotaExceeded) {
				s.logger.Warn("resource quota exceeded",
					zap.String("tenant_id", tenantID))
				c.JSON(http.StatusTooManyRequests, gin.H{
					"error":   "QuotaExceeded",
					"message": "Resource quota exceeded for tenant",
					"code":    http.StatusTooManyRequests,
				})
				return
			}
			s.logger.Error("failed to check resource quota",
				zap.String("tenant_id", tenantID),
				zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "InternalError",
				"message": "Failed to check resource quota",
				"code":    http.StatusInternalServerError,
			})
			return
		}
	}

	// Set tenant ID on the resource for isolation
	if tenantID != "" {
		req.TenantID = tenantID
	}

	// Generate resource ID if not provided (using plain UUID for simplicity)
	if req.ResourceID == "" {
		req.ResourceID = uuid.New().String()
	} else {
		// Validate client-provided resource ID is a valid UUID
		// This prevents path traversal attacks (e.g., "../../../etc/passwd")
		if _, err := uuid.Parse(req.ResourceID); err != nil {
			s.logger.Warn("invalid resource ID format",
				zap.String("resource_id", SanitizeForLogging(req.ResourceID)))
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "BadRequest",
				"message": "resourceId must be a valid UUID",
				"code":    http.StatusBadRequest,
			})
			return
		}
	}

	// Create resource via adapter

	adp := s.resolveAdapter(c)
	if adp == nil {
		return
	}

	created, err := adp.CreateResource(c.Request.Context(), &req)
	if err != nil {
		// Rollback quota increment on creation failure
		if tenantID != "" && s.AuthStore != nil {
			if rollbackErr := s.AuthStore.DecrementUsage(ctx, tenantID, "resources"); rollbackErr != nil {
				s.logger.Error("failed to rollback resource quota after creation failure",
					zap.String("tenant_id", tenantID),
					zap.Error(rollbackErr))
			}
		}

		// Audit log the failure
		if s.auditLogger != nil {
			user := auth.UserFromContext(c.Request.Context())
			s.auditLogger.LogResourceOperation(
				c.Request.Context(),
				auth.AuditEventResourceCreated,
				req.ResourceTypeID,
				req.ResourceID,
				user,
				false,
				map[string]string{
					"error": err.Error(),
				},
			)
		}

		// Check if error indicates duplicate resource
		if errors.Is(err, adapter.ErrResourceExists) {
			c.JSON(http.StatusConflict, gin.H{
				"error":   "Conflict",
				"message": "Resource with ID " + SanitizeForLogging(req.ResourceID) + " already exists",
				"code":    http.StatusConflict,
			})
			return
		}

		s.logger.Error("failed to create resource", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to create resource",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	s.logger.Info("resource created",
		zap.String("resource_id", created.ResourceID),
		zap.String("resource_type_id", SanitizeForLogging(created.ResourceTypeID)))

	// Audit log the resource creation
	if s.auditLogger != nil {
		user := auth.UserFromContext(c.Request.Context())
		s.auditLogger.LogResourceOperation(
			c.Request.Context(),
			auth.AuditEventResourceCreated,
			created.ResourceTypeID,
			created.ResourceID,
			user,
			true,
			map[string]string{
				"resource_pool_id": created.ResourcePoolID,
			},
		)
	}

	// Set Location header for REST compliance
	c.Header("Location", "/o2ims/v1/resources/"+created.ResourceID)
	c.JSON(http.StatusCreated, created)
}

// handleUpdateResource updates an existing resource.
// PUT /o2ims/v1/resources/:resourceId.
func (s *Server) handleUpdateResource(c *gin.Context) {
	resourceID := c.Param("resourceId")
	s.logger.Info("updating resource", zap.String("resource_id", resourceID))

	var req adapter.Resource
	if err := c.ShouldBindJSON(&req); err != nil {
		s.writeClientError(c, http.StatusBadRequest, "BadRequest",
			"invalid request body",
			"failed to parse request body",
			zap.Error(err),
		)
		return
	}

	// Get existing resource
	existing, err := s.getExistingResource(c, resourceID)
	if err != nil || existing == nil {
		return // Response already sent
	}

	// SECURITY: verify tenant ownership before update. Without this check a
	// non-platform-admin caller with resources:update could modify any
	// tenant's resource. Fail-closed via auth.AuthorizeTenantResource so that
	// resources with empty TenantID cannot be reached by non-admin callers
	// (see issue #470 / issue #482).
	//
	// Gated on the presence of an authenticated user so that non-auth
	// deployments (dev/test) continue to work.
	ctx := c.Request.Context()
	if user := auth.UserFromContext(ctx); user != nil && !user.IsPlatformAdmin {
		if !auth.AuthorizeTenantResource(ctx, existing.TenantID) {
			s.logger.Warn("tenant attempting to update resource from different tenant",
				zap.String("tenant_id", user.TenantID),
				zap.String("resource_tenant_id", existing.TenantID),
				zap.String("resource_id", resourceID))
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "NotFound",
				"message": "Resource not found: " + resourceID,
				"code":    http.StatusNotFound,
			})
			return
		}
		// Preserve existing tenant stamp; clients cannot move a resource
		// between tenants via update.
		req.TenantID = existing.TenantID
	}

	// Validate request
	if err := s.validateUpdateRequest(c, &req, existing); err != nil {
		return // Response already sent
	}

	// Apply update
	s.applyResourceUpdate(c, resourceID, &req, existing)
}

func (s *Server) handleDeleteResource(c *gin.Context) {
	resourceID := c.Param("resourceId")
	ctx := c.Request.Context()
	tenantID := auth.TenantIDFromContext(ctx)

	// Get resource info before deletion for audit logging and tenant ownership check.

	adp := s.resolveAdapter(c)
	if adp == nil {
		return
	}

	var resourceTypeID string
	existing, err := adp.GetResource(ctx, resourceID)
	if err == nil && existing != nil {
		resourceTypeID = existing.ResourceTypeID
	}

	// Verify tenant ownership before deletion. Identical shape is returned
	// for "not found" vs "wrong tenant" to prevent enumeration.
	if user := auth.UserFromContext(ctx); user != nil && !user.IsPlatformAdmin {
		if err != nil {
			s.writeNotFoundOrForbidden(c, "resource",
				"resource lookup failed during delete authorization",
				zap.String("resource_id", resourceID),
				zap.String("tenant_id", tenantID),
				zap.Error(err),
			)
			return
		}
		if !auth.AuthorizeTenantResource(ctx, existing.TenantID) {
			s.writeNotFoundOrForbidden(c, "resource",
				"tenant attempting to delete resource from different tenant",
				zap.String("resource_id", resourceID),
				zap.String("tenant_id", tenantID),
				zap.String("resource_tenant_id", existing.TenantID),
			)
			return
		}
	}

	if err := adp.DeleteResource(ctx, resourceID); err != nil {
		if s.auditLogger != nil {
			user := auth.UserFromContext(ctx)
			s.auditLogger.LogResourceOperation(
				ctx,
				auth.AuditEventResourceDeleted,
				resourceTypeID,
				resourceID,
				user,
				false,
				map[string]string{
					"error": err.Error(),
				},
			)
		}

		if errors.Is(err, adapter.ErrResourceNotFound) {
			s.writeNotFoundOrForbidden(c, "resource",
				"adapter reported resource not found during delete",
				zap.String("resource_id", resourceID),
			)
			return
		}
		s.writeClientError(c, http.StatusInternalServerError, "InternalError",
			"failed to delete resource",
			"adapter failed to delete resource",
			zap.String("resource_id", resourceID),
			zap.Error(err),
		)
		return
	}

	// Audit log the successful deletion
	if s.auditLogger != nil {
		user := auth.UserFromContext(ctx)
		s.auditLogger.LogResourceOperation(
			ctx,
			auth.AuditEventResourceDeleted,
			resourceTypeID,
			resourceID,
			user,
			true,
			nil,
		)
	}

	// Decrement tenant usage after successful deletion
	if tenantID != "" && s.AuthStore != nil {
		if err := s.AuthStore.DecrementUsage(ctx, tenantID, "resources"); err != nil {
			s.logger.Error("failed to decrement resource usage",
				zap.String("tenant_id", tenantID),
				zap.Error(err))
		}
	}

	c.Status(http.StatusNoContent)
}

// getExistingResource retrieves an existing resource and handles errors.
func (s *Server) getExistingResource(c *gin.Context, resourceID string) (*adapter.Resource, error) {
	adp := s.resolveAdapter(c)
	if adp == nil {
		return nil, errors.New("no adapter available")
	}

	existing, err := adp.GetResource(c.Request.Context(), resourceID)
	if err != nil {
		if errors.Is(err, adapter.ErrResourceNotFound) {
			s.writeNotFoundOrForbidden(c, "resource",
				"adapter reported resource not found",
				zap.String("resource_id", resourceID),
			)
			return nil, fmt.Errorf("failed to get resource %s: %w", resourceID, err)
		}

		s.writeClientError(c, http.StatusInternalServerError, "InternalError",
			"failed to retrieve resource",
			"adapter failed to fetch resource",
			zap.String("resource_id", resourceID),
			zap.Error(err),
		)
		return nil, fmt.Errorf("failed to get resource %s: %w", resourceID, err)
	}
	return existing, nil
}

// validateUpdateRequest validates update request and immutable fields.
func (s *Server) validateUpdateRequest(c *gin.Context, req, existing *adapter.Resource) error {
	// Validate field constraints
	if err := validateResourceFields(req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": err.Error(),
			"code":    http.StatusBadRequest,
		})
		return err
	}

	// Check immutable fields
	if err := checkImmutableFields(req, existing); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "BadRequest",
			"message": err.Error(),
			"code":    http.StatusBadRequest,
		})
		return err
	}

	return nil
}

// checkImmutableFields validates that immutable fields haven't changed.
func checkImmutableFields(req, existing *adapter.Resource) error {
	if req.ResourceTypeID != "" && req.ResourceTypeID != existing.ResourceTypeID {
		return errors.New("resourceTypeId is immutable and cannot be changed")
	}
	if req.ResourcePoolID != "" && req.ResourcePoolID != existing.ResourcePoolID {
		return errors.New("resourcePoolId is immutable and cannot be changed")
	}
	return nil
}

// applyResourceUpdate performs the update operation.
func (s *Server) applyResourceUpdate(c *gin.Context, resourceID string, req, existing *adapter.Resource) {
	// Preserve immutable fields
	req.ResourceID = resourceID
	if req.ResourceTypeID == "" {
		req.ResourceTypeID = existing.ResourceTypeID
	}
	if req.ResourcePoolID == "" {
		req.ResourcePoolID = existing.ResourcePoolID
	}

	// Update via adapter

	adp := s.resolveAdapter(c)
	if adp == nil {
		return
	}

	updated, err := adp.UpdateResource(c.Request.Context(), resourceID, req)
	if err != nil {
		// Audit log the failure
		if s.auditLogger != nil {
			user := auth.UserFromContext(c.Request.Context())
			s.auditLogger.LogResourceOperation(
				c.Request.Context(),
				auth.AuditEventResourceModified,
				req.ResourceTypeID,
				resourceID,
				user,
				false,
				map[string]string{
					"error": err.Error(),
				},
			)
		}

		s.logger.Error("failed to update resource", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to update resource",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	s.logger.Info("resource updated",
		zap.String("resource_id", updated.ResourceID),
		zap.String("resource_type_id", SanitizeForLogging(updated.ResourceTypeID)))

	// Audit log the successful update
	if s.auditLogger != nil {
		user := auth.UserFromContext(c.Request.Context())
		s.auditLogger.LogResourceOperation(
			c.Request.Context(),
			auth.AuditEventResourceModified,
			updated.ResourceTypeID,
			updated.ResourceID,
			user,
			true,
			map[string]string{
				"resource_pool_id": updated.ResourcePoolID,
			},
		)
	}

	c.JSON(http.StatusOK, updated)
}

// Resource Type handlers

// handleListResourceTypes lists all resource types.
// GET /o2ims/v1/resourceTypes.
func (s *Server) handleListResourceTypes(c *gin.Context) {
	s.logger.Info("listing resource types")

	// Parse filter from request (supports v1 basic and v2+ advanced filtering).
	filter, err := s.parseFilterFromRequest(c)
	if err != nil {
		s.logger.Error("failed to parse filter", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "InvalidParameter",
			"message": err.Error(),
			"code":    http.StatusBadRequest,
		})
		return
	}

	// List resource types via adapter.

	adp := s.resolveAdapter(c)
	if adp == nil {
		return
	}

	types, err := adp.ListResourceTypes(c.Request.Context(), filter)
	if err != nil {
		s.logger.Error("failed to list resource types", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to retrieve resource types",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"resourceTypes": types,
		"total":         len(types),
	})
}

// handleGetResourceType retrieves a specific resource type.
// GET /o2ims/v1/resourceTypes/:resourceTypeId.
func (s *Server) handleGetResourceType(c *gin.Context) {
	resourceTypeID := c.Param("resourceTypeId")
	s.logger.Info("getting resource type", zap.String("resource_type_id", resourceTypeID))

	// Get resource type via adapter

	adp := s.resolveAdapter(c)
	if adp == nil {
		return
	}

	resType, err := adp.GetResourceType(c.Request.Context(), resourceTypeID)
	if err != nil {
		s.logger.Error("failed to get resource type", zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "NotFound",
			"message": "Resource type not found: " + resourceTypeID,
			"code":    http.StatusNotFound,
		})
		return
	}

	c.JSON(http.StatusOK, resType)
}

// Deployment Manager handlers

// handleListDeploymentManagers lists all deployment managers.
// GET /o2ims/v1/deploymentManagers.
func (s *Server) handleListDeploymentManagers(c *gin.Context) {
	s.logger.Info("listing deployment managers")

	adp := s.resolveAdapter(c)
	if adp == nil {
		return
	}

	dms, err := adp.ListDeploymentManagers(c.Request.Context(), nil)
	if err != nil {
		s.logger.Error("failed to list deployment managers", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to retrieve deployment managers",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"deploymentManagers": dms,
		"total":              len(dms),
	})
}

// handleGetDeploymentManager retrieves a specific deployment manager.
// GET /o2ims/v1/deploymentManagers/:deploymentManagerId.
func (s *Server) handleGetDeploymentManager(c *gin.Context) {
	deploymentManagerID := c.Param("deploymentManagerId")
	s.logger.Info("getting deployment manager", zap.String("deployment_manager_id", deploymentManagerID))

	// Get deployment manager via adapter

	adp := s.resolveAdapter(c)
	if adp == nil {
		return
	}

	dm, err := adp.GetDeploymentManager(c.Request.Context(), deploymentManagerID)
	if err != nil {
		s.logger.Error("failed to get deployment manager", zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "NotFound",
			"message": "Deployment manager not found: " + deploymentManagerID,
			"code":    http.StatusNotFound,
		})
		return
	}

	c.JSON(http.StatusOK, dm)
}

// O-Cloud Infrastructure handlers

// handleGetOCloudInfrastructure retrieves O-Cloud infrastructure information.
// GET /o2ims/v1/oCloudInfrastructure.
func (s *Server) handleGetOCloudInfrastructure(c *gin.Context) {
	s.logger.Info("getting O-Cloud infrastructure information")

	// List all registered deployment managers instead of looking up a hardcoded ID.

	adp := s.resolveAdapter(c)
	if adp == nil {
		return
	}

	dms, err := adp.ListDeploymentManagers(c.Request.Context(), nil)
	if err != nil {
		s.logger.Error("failed to list deployment managers for O-Cloud info", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "Failed to retrieve O-Cloud information",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	if len(dms) == 0 {
		s.logger.Error("no deployment managers registered")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "InternalError",
			"message": "No deployment managers available",
			"code":    http.StatusInternalServerError,
		})
		return
	}

	// Use the first registered deployment manager for O-Cloud metadata.
	dm := dms[0]

	c.JSON(http.StatusOK, gin.H{
		"oCloudId":    dm.OCloudID,
		"name":        dm.Name,
		"description": dm.Description,
		"serviceUri":  dm.ServiceURI,
	})
}

// Tenant quota handlers (v3)
// These remain as placeholders until quota management is fully implemented

// handleGetTenantQuotas retrieves tenant quotas.
// GET /o2ims/v3/tenants/:tenantId/quotas.
func (s *Server) handleGetTenantQuotas(c *gin.Context) {
	tenantID := c.Param("tenantId")

	// Defense in depth: re-enforce tenant ownership at the handler
	// level so a future routing mistake cannot bypass the middleware
	// (issue #469 / C6). Returns 404 to avoid leaking tenant existence.
	if !s.enforceTenantOwnershipInline(c, tenantID) {
		return
	}

	s.logger.Info("getting tenant quotas", zap.String("tenant_id", tenantID))

	c.JSON(http.StatusOK, gin.H{
		"tenantId": tenantID,
		"quotas": gin.H{
			"maxSubscriptions":  100,
			"maxResourcePools":  50,
			"maxResources":      1000,
			"usedSubscriptions": 10,
			"usedResourcePools": 5,
			"usedResources":     100,
		},
	})
}

// handleUpdateTenantQuotas updates tenant quotas.
// PUT /o2ims/v3/tenants/:tenantId/quotas.
func (s *Server) handleUpdateTenantQuotas(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := c.Param("tenantId")

	// Defense in depth: re-enforce tenant ownership at the handler
	// level so a future routing mistake cannot bypass the middleware
	// (issue #469 / C6). Returns 404 to avoid leaking tenant existence.
	if !s.enforceTenantOwnershipInline(c, tenantID) {
		return
	}

	s.logger.Info("updating tenant quotas", zap.String("tenant_id", tenantID))

	var req struct {
		MaxSubscriptions int `json:"maxSubscriptions,omitempty"`
		MaxResourcePools int `json:"maxResourcePools,omitempty"`
		MaxResources     int `json:"maxResources,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		s.writeClientError(c, http.StatusBadRequest, "BadRequest",
			"invalid request body",
			"failed to parse request body",
			zap.Error(err),
		)
		return
	}

	// Audit log the quota updates
	if s.auditLogger != nil {
		user := auth.UserFromContext(ctx)
		if req.MaxSubscriptions > 0 {
			s.auditLogger.LogQuotaUpdate(ctx, tenantID, user, "maxSubscriptions", 0, req.MaxSubscriptions)
		}
		if req.MaxResourcePools > 0 {
			s.auditLogger.LogQuotaUpdate(ctx, tenantID, user, "maxResourcePools", 0, req.MaxResourcePools)
		}
		if req.MaxResources > 0 {
			s.auditLogger.LogQuotaUpdate(ctx, tenantID, user, "maxResources", 0, req.MaxResources)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"tenantId": tenantID,
		"quotas": gin.H{
			"maxSubscriptions": req.MaxSubscriptions,
			"maxResourcePools": req.MaxResourcePools,
			"maxResources":     req.MaxResources,
		},
		"updatedAt": "2024-01-01T00:00:00Z",
	})
}

// ValidateCallback validates a subscription callback URL at registration time.
// It performs early validation to provide fast failure before calling the
// adapter. Includes SSRF protection to prevent callbacks to localhost and
// private IP ranges.
//
// Defense-in-depth at delivery time: the DNS-rebinding TOCTOU gap between
// this check and the actual webhook delivery is closed by the SSRF-safe
// DialContext installed on the shared HTTP client — see
// internal/webhook.NewSSRFSafeDialContext. That dialer re-resolves the
// hostname on every delivery attempt, rejects the connect if any resolved
// IP falls into the banned set (loopback, private, link-local, cloud
// metadata), and pins the TCP connect to the allow-listed IP so the OS
// does not re-resolve between the check and the handshake.
func (s *Server) ValidateCallback(ctx context.Context, sub *adapter.Subscription) error {
	if sub == nil {
		return fmt.Errorf("subscription cannot be nil")
	}

	if sub.Callback == "" {
		return fmt.Errorf("callback URL is required")
	}

	// Parse URL to validate format
	parsedURL, err := url.Parse(sub.Callback)
	if err != nil {
		return fmt.Errorf("invalid callback URL format: %w", err)
	}

	// Validate scheme
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("callback URL must use http or https scheme")
	}

	// Validate host
	if parsedURL.Host == "" {
		return fmt.Errorf("callback URL must have a valid host")
	}

	// SSRF Protection: Block localhost and private IP ranges
	// Skip SSRF protection if disabled in config (for testing only)
	if !s.config.Security.DisableSSRFProtection {
		if err := ValidateCallbackHost(ctx, parsedURL.Hostname()); err != nil {
			return err
		}
	}

	return nil
}

// ValidateCallbackHost validates that the callback host is not localhost or a private IP address.
// This prevents SSRF (Server-Side Request Forgery) attacks.
func ValidateCallbackHost(ctx context.Context, hostname string) error {
	// Block localhost variations
	if hostname == "localhost" || hostname == "127.0.0.1" || hostname == "::1" {
		return fmt.Errorf("callback URL cannot be localhost")
	}

	// Attempt to resolve hostname to IP
	// If DNS lookup fails, we allow it - the actual webhook delivery will fail naturally
	// This prevents blocking valid hostnames that are temporarily unresolvable
	resolver := &net.Resolver{}
	ips, _ := resolver.LookupIPAddr(ctx, hostname)
	if len(ips) == 0 {
		// No IPs resolved (possibly due to DNS failure), allow it
		return nil
	}

	// Check if any resolved IP is in a private range
	for _, ipAddr := range ips {
		if IsPrivateIP(ipAddr.IP) {
			return fmt.Errorf("callback URL cannot be a private IP address")
		}
	}

	return nil
}

// Pre-computed private IP ranges for SSRF protection.
// These are initialized on first use via sync.Once to avoid runtime parsing overhead.
var (
	privateIPv4Nets []*net.IPNet
	privateIPv6Nets []*net.IPNet
	privateIPOnce   sync.Once
)

// logAuditEvent logs an audit event with tenant context if auth store is configured.
func (s *Server) logAuditEvent(
	ctx context.Context,
	c *gin.Context,
	eventType auth.AuditEventType,
	resourceType, resourceID, action string,
	details map[string]string,
) {
	if s.AuthStore == nil {
		return // Auth store not configured, skip audit logging
	}

	// Extract tenant ID from authenticated context
	tenantID := auth.TenantIDFromContext(ctx)

	// Extract user information from context if available
	var userID, subject string
	if user, exists := c.Get("user"); exists {
		if authUser, ok := user.(*auth.AuthenticatedUser); ok {
			userID = authUser.UserID
			subject = authUser.Subject
		}
	}

	event := &auth.AuditEvent{
		ID:           uuid.New().String(),
		Type:         eventType,
		TenantID:     tenantID,
		UserID:       userID,
		Subject:      subject,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Action:       action,
		Details:      details,
		ClientIP:     c.ClientIP(),
		UserAgent:    c.Request.UserAgent(),
	}

	if err := s.AuthStore.LogEvent(ctx, event); err != nil {
		s.logger.Warn("failed to log audit event",
			zap.String("event_type", string(eventType)),
			zap.String("resource_type", resourceType),
			zap.String("resource_id", resourceID),
			zap.Error(err))
	}
}

// initPrivateIPRanges initializes the private IP range networks.
// This is called lazily on first use via sync.Once.
func initPrivateIPRanges() {
	// Parse private IPv4 ranges (RFC 1918 + link-local)
	privateIPv4CIDRs := []string{
		"10.0.0.0/8",     // Private class A
		"172.16.0.0/12",  // Private class B
		"192.168.0.0/16", // Private class C
		"169.254.0.0/16", // Link-local
	}

	for _, cidr := range privateIPv4CIDRs {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			// This should never happen with hardcoded CIDRs
			panic(fmt.Sprintf("invalid IPv4 CIDR in privateIPv4CIDRs: %s: %v", cidr, err))
		}
		privateIPv4Nets = append(privateIPv4Nets, network)
	}

	// Parse private IPv6 ranges
	privateIPv6CIDRs := []string{
		"fc00::/7",  // IPv6 unique local addresses (ULA)
		"fe80::/10", // IPv6 link-local
	}

	for _, cidr := range privateIPv6CIDRs {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			// This should never happen with hardcoded CIDRs
			panic(fmt.Sprintf("invalid IPv6 CIDR in privateIPv6CIDRs: %s: %v", cidr, err))
		}
		privateIPv6Nets = append(privateIPv6Nets, network)
	}
}

// IsPrivateIP checks if an IP address is in a private or reserved range.
func IsPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() {
		return true
	}

	if isPrivateIPv4(ip) {
		return true
	}

	return isPrivateIPv6(ip)
}

// isPrivateIPv4 checks if an IPv4 address is in a private range (RFC 1918).
func isPrivateIPv4(ip net.IP) bool {
	privateIPOnce.Do(initPrivateIPRanges)
	for _, network := range privateIPv4Nets {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// isPrivateIPv6 checks if an IPv6 address is in a private range.
func isPrivateIPv6(ip net.IP) bool {
	privateIPOnce.Do(initPrivateIPRanges)
	// Only check IPv6 addresses
	if ip.To4() != nil {
		return false
	}

	for _, network := range privateIPv6Nets {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}
