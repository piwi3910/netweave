package server

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/piwi3910/netweave/internal/auth"
	"github.com/piwi3910/netweave/internal/handlers"
	"github.com/piwi3910/netweave/internal/o2ims/models"
	"go.uber.org/zap"
)

// AuthHandlers contains all handlers for authentication and authorization.
type AuthHandlers struct {
	Tenant *handlers.TenantHandler
	User   *handlers.UserHandler
	Role   *handlers.RoleHandler
	Audit  *handlers.AuditHandler
}

// SetupAuthRoutes configures all authentication and multi-tenancy routes.
// All admin routes are consolidated under /admin with three sub-groups:
//   - /admin/platform/* — platform-admin routes (tenant management, platform audit)
//   - /admin/tenant/*   — tenant-scoped routes (current tenant, users, roles, audit)
//   - /admin/infrastructure/* — backend admin routes (registered via SetupBackendAdmin)
//
// These routes are only enabled when multi-tenancy is configured.
func (s *Server) SetupAuthRoutes(authStore auth.Store, authMw *auth.Middleware) {
	// Store the full auth store for tenant lookups in wrapWithTenantContext.
	s.fullAuthStore = authStore

	// Enable per-tenant rate limiting (must come after auth middleware is set up).
	s.setupTenantRateLimiter()

	// Create handlers.
	tenantHandler := handlers.NewTenantHandler(authStore, s.logger)
	userHandler := handlers.NewUserHandler(authStore, s.logger)
	roleHandler := handlers.NewRoleHandler(authStore, s.logger)
	auditHandler := handlers.NewAuditHandler(authStore, s.logger)

	// All admin routes live under /admin on the admin router.
	admin := s.adminRouter.Group("/admin")

	// --- Platform Admin Routes (/admin/platform/*) ---
	// These require platform-admin role.
	platform := admin.Group("/platform")
	platform.Use(authMw.AuthenticationMiddleware())
	platform.Use(authMw.RequirePlatformAdmin())
	{
		// Tenant Management.
		tenants := platform.Group("/tenants")
		{
			tenants.GET("", tenantHandler.ListTenants)
			tenants.POST("", tenantHandler.CreateTenant)
			tenants.GET("/:tenantId", tenantHandler.GetTenant)
			tenants.PUT("/:tenantId", tenantHandler.UpdateTenant)
			tenants.DELETE("/:tenantId", tenantHandler.DeleteTenant)

			// Admin can manage users in any tenant.
			tenants.GET("/:tenantId/users", s.wrapWithTenantContext(userHandler.ListUsers))
			tenants.POST("/:tenantId/users", s.wrapWithTenantContext(userHandler.CreateUser))
		}

		// Platform-level audit logs.
		platform.GET("/audit/events", auditHandler.ListAuditEvents)

		// Frontend plugin management.
		if s.pluginRegistry != nil {
			pluginAdapter := newPluginRegistryAdapter(s.pluginRegistry)
			pluginHandler := handlers.NewPluginHandler(pluginAdapter, s.logger)

			plugins := platform.Group("/plugins")
			{
				plugins.GET("", pluginHandler.ListPlugins)
				plugins.GET("/:name", pluginHandler.GetPlugin)
				plugins.PUT("/:name", pluginHandler.UpdatePlugin)
			}
		}
	}

	// --- Tenant Routes (/admin/tenant/*) ---
	// These require tenant-level authentication.
	tenant := admin.Group("/tenant")
	tenant.Use(authMw.AuthenticationMiddleware())
	{
		// Current tenant info.
		tenant.GET("", tenantHandler.GetCurrentTenant)

		// Current user info.
		tenant.GET("/me", userHandler.GetCurrentUser)

		// User management within tenant.
		users := tenant.Group("/users")
		users.Use(authMw.RequirePermission(string(auth.PermissionUserRead)))
		{
			users.GET("", userHandler.ListUsers)
			users.POST("", authMw.RequirePermission(string(auth.PermissionUserCreate)), userHandler.CreateUser)
			users.GET("/:userId", userHandler.GetUser)
			users.PUT("/:userId", authMw.RequirePermission(string(auth.PermissionUserUpdate)), userHandler.UpdateUser)
			users.DELETE("/:userId", authMw.RequirePermission(string(auth.PermissionUserDelete)), userHandler.DeleteUser)
		}

		// Role routes (read-only for authenticated users).
		roles := tenant.Group("/roles")
		roles.Use(authMw.RequirePermission(string(auth.PermissionRoleRead)))
		{
			roles.GET("", roleHandler.ListRoles)
			roles.GET("/:roleId", roleHandler.GetRole)
		}

		// Permissions endpoint.
		tenant.GET("/permissions", roleHandler.ListPermissions)

		// Tenant-level audit logs.
		audit := tenant.Group("/audit")
		audit.Use(authMw.RequirePermission(string(auth.PermissionAuditRead)))
		{
			audit.GET("/events", auditHandler.ListAuditEvents)
			audit.GET("/events/type/:eventType", auditHandler.ListAuditEventsByType)
			audit.GET("/events/user/:userId", auditHandler.ListAuditEventsByUser)
		}
	}
}

// wrapWithTenantContext wraps a handler to inject tenant context from path parameter.
// It overrides both the authenticated user's TenantID and the Tenant object in context
// so that downstream handlers (e.g. CreateUser) check quota against the target tenant.
func (s *Server) wrapWithTenantContext(handler gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := c.Param("tenantId")
		if tenantID != "" {
			// Update context with tenant ID for handlers that use context.
			ctx := auth.ContextWithRequestID(c.Request.Context(), c.GetString("request_id"))
			// Retrieve the authenticated user from context (should always exist behind auth middleware).
			user := auth.UserFromContext(c.Request.Context())
			if user == nil {
				s.logger.Error("no authenticated user in context for tenant override")
				c.JSON(http.StatusInternalServerError, models.ErrorResponse{
					Error:   "InternalError",
					Message: "Authentication context missing",
					Code:    http.StatusInternalServerError,
				})
				c.Abort()
				return
			}

			// Override tenant ID for admin operations.
			adminUser := &auth.AuthenticatedUser{
				UserID:          user.UserID,
				TenantID:        tenantID,
				Subject:         user.Subject,
				CommonName:      user.CommonName,
				Role:            user.Role,
				IsPlatformAdmin: user.IsPlatformAdmin,
			}
			ctx = auth.ContextWithUser(ctx, adminUser)

			// Load the target tenant from the store so handlers get the correct
			// tenant object (with proper quota/usage) instead of the admin's tenant.
			if s.fullAuthStore != nil {
				targetTenant, err := s.fullAuthStore.GetTenant(ctx, tenantID)
				if err != nil {
					s.handleTenantLookupError(c, tenantID, err)
					return
				}
				ctx = auth.ContextWithTenant(ctx, targetTenant)
			}

			c.Request = c.Request.WithContext(ctx)
		}
		handler(c)
	}
}

// handleTenantLookupError responds with the appropriate error when a tenant lookup fails.
func (s *Server) handleTenantLookupError(c *gin.Context, tenantID string, err error) {
	if errors.Is(err, auth.ErrTenantNotFound) {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "NotFound",
			Message: "Tenant not found",
			Code:    http.StatusNotFound,
		})
		return
	}
	s.logger.Error("failed to load target tenant",
		zap.String("tenant_id", tenantID),
		zap.Error(err),
	)
	c.JSON(http.StatusInternalServerError, models.ErrorResponse{
		Error:   "InternalError",
		Message: "Failed to load target tenant",
		Code:    http.StatusInternalServerError,
	})
}
