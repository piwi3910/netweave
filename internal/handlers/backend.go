package handlers

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/piwi3910/netweave/internal/backend"
	"github.com/piwi3910/netweave/internal/encryption"
	"go.uber.org/zap"
)

// BackendHandler handles admin API endpoints for backend instance management.
type BackendHandler struct {
	store     backend.Store
	encryptor encryption.Encryptor
	registry  *backend.SchemaRegistry
	logger    *zap.Logger
}

// NewBackendHandler creates a new BackendHandler.
func NewBackendHandler(
	store backend.Store,
	encryptor encryption.Encryptor,
	registry *backend.SchemaRegistry,
	logger *zap.Logger,
) *BackendHandler {
	if store == nil {
		panic("backend store cannot be nil")
	}
	if encryptor == nil {
		panic("encryptor cannot be nil")
	}
	if registry == nil {
		panic("schema registry cannot be nil")
	}
	if logger == nil {
		panic("logger cannot be nil")
	}

	return &BackendHandler{
		store:     store,
		encryptor: encryptor,
		registry:  registry,
		logger:    logger,
	}
}

// RegisterRoutes registers all backend admin API routes on the given router group.
// Routes:
//
//	GET    /backends
//	POST   /backends
//	GET    /backends/:id
//	PUT    /backends/:id
//	DELETE /backends/:id
//	POST   /backends/:id/test
//	GET    /backends/:id/dms-links
//	POST   /backends/:id/dms-links
//	DELETE /backends/:id/dms-links/:dmsId
//	GET    /tenants/:tid/backend-access
//	POST   /tenants/:tid/backend-access
//	PUT    /tenants/:tid/backend-access/:aid
//	DELETE /tenants/:tid/backend-access/:aid
//	GET    /backend-types
//	GET    /backend-types/:type/schema
func (h *BackendHandler) RegisterRoutes(router *gin.RouterGroup) {
	router.GET("/backends", h.ListBackends)
	router.POST("/backends", h.CreateBackend)
	router.GET("/backends/:id", h.GetBackend)
	router.PUT("/backends/:id", h.UpdateBackend)
	router.DELETE("/backends/:id", h.DeleteBackend)
	router.POST("/backends/:id/test", h.TestBackend)

	router.GET("/backends/:id/dms-links", h.ListDMSLinks)
	router.POST("/backends/:id/dms-links", h.CreateDMSLink)
	router.DELETE("/backends/:id/dms-links/:dmsId", h.DeleteDMSLink)

	router.GET("/tenants/:tid/backend-access", h.ListBackendAccess)
	router.POST("/tenants/:tid/backend-access", h.CreateBackendAccess)
	router.PUT("/tenants/:tid/backend-access/:aid", h.UpdateBackendAccess)
	router.DELETE("/tenants/:tid/backend-access/:aid", h.DeleteBackendAccess)

	router.GET("/backend-types", h.ListBackendTypes)
	router.GET("/backend-types/:type/schema", h.GetBackendTypeSchema)
}

// CreateBackendRequest is the request body for creating a backend instance.
type CreateBackendRequest struct {
	Name        string            `json:"name" binding:"required"`
	Category    string            `json:"category" binding:"required"`
	AdapterType string            `json:"adapterType" binding:"required"`
	Description string            `json:"description,omitempty"`
	Config      map[string]string `json:"config,omitempty"`
	Credentials map[string]string `json:"credentials,omitempty"`
}

// UpdateBackendRequest is the request body for updating a backend instance.
type UpdateBackendRequest struct {
	Name        string            `json:"name,omitempty"`
	Description string            `json:"description,omitempty"`
	Config      map[string]string `json:"config,omitempty"`
	Credentials map[string]string `json:"credentials,omitempty"`
}

// CreateDMSLinkRequest is the request body for creating a DMS link.
type CreateDMSLinkRequest struct {
	DMSID string `json:"dmsId" binding:"required"`
}

// CreateBackendAccessRequest is the request body for creating backend access.
type CreateBackendAccessRequest struct {
	BackendID   string                 `json:"backendId" binding:"required"`
	Permissions []string               `json:"permissions" binding:"required"`
	Constraints map[string]interface{} `json:"constraints,omitempty"`
}

// UpdateBackendAccessRequest is the request body for updating backend access.
type UpdateBackendAccessRequest struct {
	Permissions []string               `json:"permissions" binding:"required"`
	Constraints map[string]interface{} `json:"constraints,omitempty"`
}

// BackendResponse is the API response for a backend instance.
// It never exposes raw credentials.
type BackendResponse struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Category        string    `json:"category"`
	AdapterType     string    `json:"adapterType"`
	Description     string    `json:"description,omitempty"`
	Status          string    `json:"status"`
	StatusMessage   string    `json:"statusMessage,omitempty"`
	HasCredentials  bool      `json:"hasCredentials"`
	LastHealthCheck time.Time `json:"lastHealthCheck,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

func backendToResponse(b *backend.Instance) *BackendResponse {
	return &BackendResponse{
		ID:              b.ID,
		Name:            b.Name,
		Category:        b.Category,
		AdapterType:     b.AdapterType,
		Description:     b.Description,
		Status:          b.Status,
		StatusMessage:   b.StatusMessage,
		HasCredentials:  len(b.Credentials) > 0,
		LastHealthCheck: b.LastHealthCheck,
		CreatedAt:       b.CreatedAt,
		UpdatedAt:       b.UpdatedAt,
	}
}

func backendsToResponses(backends []*backend.Instance) []*BackendResponse {
	result := make([]*BackendResponse, 0, len(backends))
	for _, b := range backends {
		result = append(result, backendToResponse(b))
	}
	return result
}
