package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/piwi3910/netweave/internal/backend"
	"github.com/piwi3910/netweave/internal/o2ims/models"
	"go.uber.org/zap"
)

// ListBackends handles GET /admin/backends.
func (h *BackendHandler) ListBackends(c *gin.Context) {
	ctx := c.Request.Context()
	category := c.Query("category")

	var (
		backends []*backend.Instance
		err      error
	)

	if category != "" {
		backends, err = h.store.ListBackendsByCategory(ctx, category)
	} else {
		backends, err = h.store.ListBackends(ctx)
	}

	if err != nil {
		h.logger.Error("failed to list backends", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to list backends",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"backends": backendsToResponses(backends),
		"total":    len(backends),
	})
}

// CreateBackend handles POST /admin/backends.
func (h *BackendHandler) CreateBackend(c *gin.Context) {
	var req CreateBackendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "Invalid request body",
			Code:    http.StatusBadRequest,
		})
		return
	}

	if err := h.validateCreateBackendRequest(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: err.Error(),
			Code:    http.StatusBadRequest,
		})
		return
	}

	instance, err := h.buildBackendInstance(&req)
	if err != nil {
		h.logger.Error("failed to build backend instance", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to encrypt backend data",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	if err := h.store.CreateBackend(c.Request.Context(), instance); err != nil {
		h.handleCreateBackendError(c, err)
		return
	}

	h.logger.Info("backend created",
		zap.String("backend_id", instance.ID),
		zap.String("name", instance.Name),
	)

	h.refreshAdapterAsync(c.Request.Context(), instance.ID)

	c.JSON(http.StatusCreated, backendToResponse(instance))
}

func (h *BackendHandler) validateCreateBackendRequest(req *CreateBackendRequest) error {
	if req.Category != "ims" && req.Category != "dms" {
		return errors.New("category must be 'ims' or 'dms'")
	}

	if _, ok := h.registry.Get(req.AdapterType); !ok {
		return errors.New("unknown adapter type")
	}

	return nil
}

func (h *BackendHandler) buildBackendInstance(req *CreateBackendRequest) (*backend.Instance, error) {
	now := time.Now().UTC()

	encConfig, err := h.encryptMap(req.Config)
	if err != nil {
		return nil, err
	}

	encCreds, err := h.encryptMap(req.Credentials)
	if err != nil {
		return nil, err
	}

	return &backend.Instance{
		ID:          uuid.New().String(),
		Name:        req.Name,
		Category:    req.Category,
		AdapterType: req.AdapterType,
		Description: req.Description,
		Config:      encConfig,
		Credentials: encCreds,
		Status:      "unknown",
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func (h *BackendHandler) handleCreateBackendError(c *gin.Context, err error) {
	if errors.Is(err, backend.ErrBackendExists) {
		c.JSON(http.StatusConflict, models.ErrorResponse{
			Error:   "Conflict",
			Message: "Backend with this name already exists",
			Code:    http.StatusConflict,
		})
		return
	}

	h.logger.Error("failed to create backend", zap.Error(err))
	c.JSON(http.StatusInternalServerError, models.ErrorResponse{
		Error:   "InternalError",
		Message: "Failed to create backend",
		Code:    http.StatusInternalServerError,
	})
}

// GetBackend handles GET /admin/backends/:id.
func (h *BackendHandler) GetBackend(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "Backend ID is required",
			Code:    http.StatusBadRequest,
		})
		return
	}

	b, err := h.store.GetBackend(c.Request.Context(), id)
	if err != nil {
		h.handleGetBackendError(c, id, err)
		return
	}

	c.JSON(http.StatusOK, backendToResponse(b))
}

func (h *BackendHandler) handleGetBackendError(c *gin.Context, id string, err error) {
	if errors.Is(err, backend.ErrBackendNotFound) {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "NotFound",
			Message: "Backend not found",
			Code:    http.StatusNotFound,
		})
		return
	}

	h.logger.Error("failed to get backend", zap.String("id", id), zap.Error(err))
	c.JSON(http.StatusInternalServerError, models.ErrorResponse{
		Error:   "InternalError",
		Message: "Failed to retrieve backend",
		Code:    http.StatusInternalServerError,
	})
}

// UpdateBackend handles PUT /admin/backends/:id.
func (h *BackendHandler) UpdateBackend(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "Backend ID is required",
			Code:    http.StatusBadRequest,
		})
		return
	}

	var req UpdateBackendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "Invalid request body",
			Code:    http.StatusBadRequest,
		})
		return
	}

	existing, err := h.store.GetBackend(c.Request.Context(), id)
	if err != nil {
		h.handleGetBackendError(c, id, err)
		return
	}

	if err := h.applyBackendUpdates(existing, &req); err != nil {
		h.logger.Error("failed to encrypt backend data", zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to encrypt backend data",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	if err := h.store.UpdateBackend(c.Request.Context(), existing); err != nil {
		h.logger.Error("failed to update backend", zap.String("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to update backend",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	h.logger.Info("backend updated", zap.String("backend_id", id))

	h.refreshAdapterAsync(c.Request.Context(), id)

	c.JSON(http.StatusOK, backendToResponse(existing))
}

func (h *BackendHandler) applyBackendUpdates(existing *backend.Instance, req *UpdateBackendRequest) error {
	if req.Name != "" {
		existing.Name = req.Name
	}
	if req.Description != "" {
		existing.Description = req.Description
	}
	if req.Config != nil {
		encConfig, err := h.encryptMap(req.Config)
		if err != nil {
			return err
		}
		existing.Config = encConfig
	}
	if req.Credentials != nil {
		encCreds, err := h.encryptMap(req.Credentials)
		if err != nil {
			return err
		}
		existing.Credentials = encCreds
	}
	existing.UpdatedAt = time.Now().UTC()
	return nil
}

// DeleteBackend handles DELETE /admin/backends/:id.
func (h *BackendHandler) DeleteBackend(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "Backend ID is required",
			Code:    http.StatusBadRequest,
		})
		return
	}

	if err := h.store.DeleteBackend(c.Request.Context(), id); err != nil {
		if errors.Is(err, backend.ErrBackendNotFound) {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "NotFound",
				Message: "Backend not found",
				Code:    http.StatusNotFound,
			})
			return
		}

		h.logger.Error("failed to delete backend", zap.String("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to delete backend",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	h.removeAdapter(id)

	h.logger.Info("backend deleted", zap.String("backend_id", id))
	c.Status(http.StatusNoContent)
}

// TestBackend handles POST /admin/backends/:id/test.
func (h *BackendHandler) TestBackend(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "Backend ID is required",
			Code:    http.StatusBadRequest,
		})
		return
	}

	_, err := h.store.GetBackend(c.Request.Context(), id)
	if err != nil {
		h.handleGetBackendError(c, id, err)
		return
	}

	// Update status to indicate test was run.
	if err := h.store.UpdateBackendStatus(c.Request.Context(), id, "unknown", "connection test not yet implemented"); err != nil {
		h.logger.Error("failed to update backend status", zap.String("id", id), zap.Error(err))
	}

	c.JSON(http.StatusOK, gin.H{
		"backendId": id,
		"status":    "unknown",
		"message":   "Connection test not yet implemented",
	})
}

// refreshAdapterAsync refreshes the adapter for a backend in the registry.
// Errors are logged but do not fail the HTTP request.
func (h *BackendHandler) refreshAdapterAsync(ctx context.Context, backendID string) {
	if h.adapterRegistry == nil {
		return
	}
	if err := h.adapterRegistry.RefreshAdapter(ctx, h.store, backendID); err != nil {
		h.logger.Warn("failed to refresh adapter after backend change",
			zap.String("backend_id", backendID),
			zap.Error(err),
		)
	}
}

// removeAdapter removes an adapter from the registry.
func (h *BackendHandler) removeAdapter(backendID string) {
	if h.adapterRegistry == nil {
		return
	}
	h.adapterRegistry.RemoveAdapter(backendID)
}

// encryptMap marshals a map to JSON and encrypts it.
func (h *BackendHandler) encryptMap(data map[string]string) ([]byte, error) {
	if data == nil {
		return nil, nil
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	encrypted, err := h.encryptor.Encrypt(jsonData)
	if err != nil {
		return nil, err
	}

	return encrypted, nil
}
