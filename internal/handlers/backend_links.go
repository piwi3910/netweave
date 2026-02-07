package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/piwi3910/netweave/internal/backend"
	"github.com/piwi3910/netweave/internal/o2ims/models"
	"go.uber.org/zap"
)

// ListDMSLinks handles GET /admin/backends/:id/dms-links.
func (h *BackendHandler) ListDMSLinks(c *gin.Context) {
	imsID := c.Param("id")
	if imsID == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "IMS backend ID is required",
			Code:    http.StatusBadRequest,
		})
		return
	}

	links, err := h.store.ListDMSLinksByIMS(c.Request.Context(), imsID)
	if err != nil {
		h.logger.Error("failed to list DMS links", zap.String("ims_id", imsID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to list DMS links",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"links": backendsToResponses(links),
		"total": len(links),
	})
}

// CreateDMSLink handles POST /admin/backends/:id/dms-links.
func (h *BackendHandler) CreateDMSLink(c *gin.Context) {
	imsID := c.Param("id")
	if imsID == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "IMS backend ID is required",
			Code:    http.StatusBadRequest,
		})
		return
	}

	var req CreateDMSLinkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "Invalid request body",
			Code:    http.StatusBadRequest,
		})
		return
	}

	if err := h.store.CreateBackendLink(c.Request.Context(), imsID, req.DMSID); err != nil {
		h.handleCreateLinkError(c, imsID, req.DMSID, err)
		return
	}

	h.logger.Info("DMS link created",
		zap.String("ims_id", imsID),
		zap.String("dms_id", req.DMSID),
	)

	c.JSON(http.StatusCreated, gin.H{
		"imsId": imsID,
		"dmsId": req.DMSID,
	})
}

func (h *BackendHandler) handleCreateLinkError(c *gin.Context, imsID, dmsID string, err error) {
	if errors.Is(err, backend.ErrBackendLinkExists) {
		c.JSON(http.StatusConflict, models.ErrorResponse{
			Error:   "Conflict",
			Message: "Link already exists",
			Code:    http.StatusConflict,
		})
		return
	}

	h.logger.Error("failed to create DMS link",
		zap.String("ims_id", imsID),
		zap.String("dms_id", dmsID),
		zap.Error(err),
	)
	c.JSON(http.StatusInternalServerError, models.ErrorResponse{
		Error:   "InternalError",
		Message: "Failed to create DMS link",
		Code:    http.StatusInternalServerError,
	})
}

// DeleteDMSLink handles DELETE /admin/backends/:id/dms-links/:dmsId.
func (h *BackendHandler) DeleteDMSLink(c *gin.Context) {
	imsID := c.Param("id")
	dmsID := c.Param("dmsId")

	if imsID == "" || dmsID == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "IMS and DMS backend IDs are required",
			Code:    http.StatusBadRequest,
		})
		return
	}

	if err := h.store.DeleteBackendLink(c.Request.Context(), imsID, dmsID); err != nil {
		if errors.Is(err, backend.ErrBackendLinkNotFound) {
			c.JSON(http.StatusNotFound, models.ErrorResponse{
				Error:   "NotFound",
				Message: "Link not found",
				Code:    http.StatusNotFound,
			})
			return
		}

		h.logger.Error("failed to delete DMS link",
			zap.String("ims_id", imsID),
			zap.String("dms_id", dmsID),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Error:   "InternalError",
			Message: "Failed to delete DMS link",
			Code:    http.StatusInternalServerError,
		})
		return
	}

	h.logger.Info("DMS link deleted",
		zap.String("ims_id", imsID),
		zap.String("dms_id", dmsID),
	)
	c.Status(http.StatusNoContent)
}
