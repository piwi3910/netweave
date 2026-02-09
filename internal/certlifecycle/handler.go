package certlifecycle

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Handler provides HTTP endpoints for certificate lifecycle queries.
type Handler struct {
	store  Store
	logger *zap.Logger
}

// NewHandler creates a new certificate lifecycle HTTP handler.
func NewHandler(store Store, logger *zap.Logger) *Handler {
	return &Handler{
		store:  store,
		logger: logger,
	}
}

// RegisterRoutes registers certificate lifecycle routes on the given group.
func (h *Handler) RegisterRoutes(group *gin.RouterGroup) {
	certs := group.Group("/certificates")
	certs.GET("", h.listCertificates)
	certs.GET("/monitoring", h.monitoringStats)
	certs.GET("/:serial", h.getCertificate)
}

// listCertificates returns certificates with optional filters.
func (h *Handler) listCertificates(c *gin.Context) {
	ctx := c.Request.Context()

	tenantID := c.Query("tenant_id")
	userID := c.Query("user_id")
	status := c.Query("status")

	var certs []*CertificateMetadata
	var err error

	switch {
	case tenantID != "":
		certs, err = h.store.ListByTenant(ctx, tenantID)
	case userID != "":
		certs, err = h.store.ListByUser(ctx, userID)
	case status != "":
		certs, err = h.store.ListByStatus(ctx, CertificateStatus(status))
	default:
		// Without a filter, return active certs as a sensible default.
		certs, err = h.store.ListByStatus(ctx, StatusActive)
	}

	if err != nil {
		h.logger.Error("failed to list certificates",
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to list certificates",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"certificates": certs,
		"count":        len(certs),
	})
}

// monitoringStats returns aggregate certificate statistics.
func (h *Handler) monitoringStats(c *gin.Context) {
	ctx := c.Request.Context()

	counts, err := h.store.CountByStatus(ctx)
	if err != nil {
		h.logger.Error("failed to get monitoring stats",
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to get monitoring stats",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status_counts": counts,
	})
}

// getCertificate returns a single certificate by serial number.
func (h *Handler) getCertificate(c *gin.Context) {
	ctx := c.Request.Context()
	serial := c.Param("serial")

	meta, err := h.store.Get(ctx, serial)
	if err != nil {
		if errors.Is(err, ErrCertificateNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "certificate not found",
			})
			return
		}
		h.logger.Error("failed to get certificate",
			zap.String("serial", serial),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to get certificate",
		})
		return
	}

	c.JSON(http.StatusOK, meta)
}
