package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/piwi3910/netweave/internal/o2ims/models"
)

// ListBackendTypes handles GET /admin/backend-types.
func (h *BackendHandler) ListBackendTypes(c *gin.Context) {
	category := c.Query("category")

	var schemas interface{}
	if category != "" {
		schemas = h.registry.ListByCategory(category)
	} else {
		schemas = h.registry.List()
	}

	c.JSON(http.StatusOK, gin.H{
		"types": schemas,
	})
}

// GetBackendTypeSchema handles GET /admin/backend-types/:type/schema.
func (h *BackendHandler) GetBackendTypeSchema(c *gin.Context) {
	adapterType := c.Param("type")
	if adapterType == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "Adapter type is required",
			Code:    http.StatusBadRequest,
		})
		return
	}

	schema, ok := h.registry.Get(adapterType)
	if !ok {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Error:   "NotFound",
			Message: "Adapter type not found",
			Code:    http.StatusNotFound,
		})
		return
	}

	c.JSON(http.StatusOK, schema)
}
