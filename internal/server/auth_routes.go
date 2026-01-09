package server

import (
	"github.com/gin-gonic/gin"
	"github.com/piwi3910/netweave/internal/auth"
	"github.com/piwi3910/netweave/internal/handlers"
)

// AuthHandlers contains all handlers for authentication and authorization.
type AuthHandlers struct {
	Tenant *handlers.TenantHandler
	User   *handlers.UserHandler
	Role   *handlers.RoleHandler
	Audit  *handlers.AuditHandler
}

// SetupAuthRoutes configures all authentication and multi-tenancy routes.
// These routes are only enabled when multi-tenancy is configured.
// This is the main entry point for wiring up multi-tenancy routes.
func (s *Server) SetupAuthRoutes(authStore auth.Store, authMw *auth.Middleware) {
	// Create handlers.
	tenantHandler := handlers.NewTenantHandler(authStore, s.logger)
	userHandler := handlers.NewUserHandler(authStore, s.logger)
	roleHandler := handlers.NewRoleHandler(authStore, s.logger)
	auditHandler := handlers.NewAuditHandler(authStore, s.logger)

	// Platform Admin Routes (/admin/*)
	// These require platform-admin role.
	admin := s.router.Group("/admin")
	admin.Use(authMw.AuthenticationMiddleware())
	admin.Use(authMw.RequirePlatformAdmin())
	{
		// Tenant Management.
		tenants := admin.Group("/tenants")
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
		admin.GET("/audit/events", auditHandler.ListAuditEvents)
	}

	// Tenant Routes (/tenant/*)
	// These require tenant-level authentication.
	tenant := s.router.Group("/tenant")
	tenant.Use(authMw.AuthenticationMiddleware())
	{
		// Current tenant info.
		tenant.GET("", tenantHandler.GetCurrentTenant)

		// User management within tenant.
		users := tenant.Group("/users")
		users.Use(authMw.RequirePermission(auth.PermissionUserRead))
		{
			users.GET("", userHandler.ListUsers)
			users.POST("", authMw.RequirePermission(auth.PermissionUserCreate), userHandler.CreateUser)
			users.GET("/:userId", userHandler.GetUser)
			users.PUT("/:userId", authMw.RequirePermission(auth.PermissionUserUpdate), userHandler.UpdateUser)
			users.DELETE("/:userId", authMw.RequirePermission(auth.PermissionUserDelete), userHandler.DeleteUser)
		}

		// Tenant-level audit logs.
		audit := tenant.Group("/audit")
		audit.Use(authMw.RequirePermission(auth.PermissionAuditRead))
		{
			audit.GET("/events", auditHandler.ListAuditEvents)
			audit.GET("/events/type/:eventType", auditHandler.ListAuditEventsByType)
			audit.GET("/events/user/:userId", auditHandler.ListAuditEventsByUser)
		}
	}

	// Current User Routes (/user/*)
	// These require any authenticated user.
	user := s.router.Group("/user")
	user.Use(authMw.AuthenticationMiddleware())
	{
		user.GET("", userHandler.GetCurrentUser)
	}

	// Role Routes (/roles/*)
	// Read-only access to roles for all authenticated users.
	roles := s.router.Group("/roles")
	roles.Use(authMw.AuthenticationMiddleware())
	roles.Use(authMw.RequirePermission(auth.PermissionRoleRead))
	{
		roles.GET("", roleHandler.ListRoles)
		roles.GET("/:roleId", roleHandler.GetRole)
	}

	// Permissions endpoint.
	s.router.GET("/permissions", authMw.AuthenticationMiddleware(), roleHandler.ListPermissions)
}

// wrapWithTenantContext wraps a handler to inject tenant context from path parameter.
func (s *Server) wrapWithTenantContext(handler gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := c.Param("tenantId")
		if tenantID != "" {
			c.Set("tenant_id", tenantID)
			// Update context with tenant ID for handlers that use context.
			ctx := auth.ContextWithRequestID(c.Request.Context(), c.GetString("request_id"))
			// Create a minimal authenticated user for context.
			user := auth.UserFromContext(c.Request.Context())
			if user != nil {
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
				c.Request = c.Request.WithContext(ctx)
			}
		}
		handler(c)
	}
}

// setupAuthMiddleware applies authentication middleware to O2-IMS routes.
func (s *Server) setupAuthMiddleware(authMw *auth.Middleware) {
	// Apply authentication to all O2-IMS API routes.
	// The middleware is configured to skip health/metrics endpoints.
	s.router.Use(authMw.AuthenticationMiddleware())
}
