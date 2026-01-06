// Package handlers provides HTTP handlers for O2-DMS API endpoints.
// It implements the O-RAN O2-DMS interface for deployment lifecycle management.
package handlers

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/piwi3910/netweave/internal/dms/adapter"
	"github.com/piwi3910/netweave/internal/dms/models"
	"github.com/piwi3910/netweave/internal/dms/registry"
	"github.com/piwi3910/netweave/internal/dms/storage"
	"go.uber.org/zap"
)

// Pagination constants.
const (
	// MaxPaginationLimit is the maximum number of items that can be returned in a list request.
	MaxPaginationLimit = 1000

	// DefaultPaginationLimit is the default number of items returned if not specified.
	DefaultPaginationLimit = 100
)

// Handler provides HTTP handlers for O2-DMS API endpoints.
type Handler struct {
	registry *registry.Registry
	store    storage.Store
	logger   *zap.Logger
}

// NewHandler creates a new DMS handler.
func NewHandler(reg *registry.Registry, store storage.Store, logger *zap.Logger) *Handler {
	return &Handler{
		registry: reg,
		store:    store,
		logger:   logger,
	}
}

// getAdapter returns the appropriate DMS adapter for the request.
// If no adapter name is specified, uses the default adapter.
func (h *Handler) getAdapter(c *gin.Context) (adapter.DMSAdapter, error) {
	adapterName := c.Query("adapter")
	if adapterName != "" {
		adp := h.registry.Get(adapterName)
		if adp == nil {
			return nil, errors.New("adapter not found: " + adapterName)
		}
		return adp, nil
	}

	adp := h.registry.GetDefault()
	if adp == nil {
		return nil, errors.New("no default DMS adapter configured")
	}
	return adp, nil
}

// errorResponse sends a standardized error response.
func (h *Handler) errorResponse(c *gin.Context, code int, errType, message string) {
	c.JSON(code, models.APIError{
		Error:   errType,
		Message: message,
		Code:    code,
	})
}

// validateCallbackURL validates a webhook callback URL for security.
// It ensures:
// - The URL is properly formatted
// - HTTPS is used (required for production)
// - The host is not an internal/private IP address
func validateCallbackURL(callbackURL string) error {
	parsed, err := url.Parse(callbackURL)
	if err != nil {
		return errors.New("invalid URL format")
	}

	// Enforce HTTPS for webhook callbacks.
	if parsed.Scheme != "https" {
		return errors.New("callback URL must use HTTPS")
	}

	// Extract host without port.
	host := parsed.Hostname()
	if host == "" {
		return errors.New("callback URL must have a valid host")
	}

	// Block localhost and loopback addresses.
	if host == "localhost" || strings.HasPrefix(host, "127.") || host == "::1" {
		return errors.New("callback URL cannot point to localhost")
	}

	// Resolve the hostname and check if it's a private IP.
	ips, err := net.LookupIP(host)
	if err == nil {
		for _, ip := range ips {
			if isPrivateIP(ip) {
				return errors.New("callback URL cannot point to private IP addresses")
			}
		}
	}

	return nil
}

// isPrivateIP checks if an IP address is in a private range.
func isPrivateIP(ip net.IP) bool {
	// Check for loopback.
	if ip.IsLoopback() {
		return true
	}

	// Check for link-local addresses.
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}

	// Private IPv4 ranges.
	privateBlocks := []string{
		"10.0.0.0/8",     // Class A private
		"172.16.0.0/12",  // Class B private
		"192.168.0.0/16", // Class C private
		"169.254.0.0/16", // Link-local
	}

	for _, block := range privateBlocks {
		_, cidr, err := net.ParseCIDR(block)
		if err != nil {
			continue
		}
		if cidr.Contains(ip) {
			return true
		}
	}

	return false
}

// redactURL redacts sensitive parts of a URL for logging.
func redactURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "[invalid-url]"
	}

	// Remove query parameters (may contain secrets).
	parsed.RawQuery = ""
	parsed.Fragment = ""

	// Remove user info if present.
	parsed.User = nil

	return parsed.String()
}

// validatePaginationLimit validates and normalizes the pagination limit.
// Returns the validated limit value.
func validatePaginationLimit(limit int) int {
	if limit <= 0 {
		return DefaultPaginationLimit
	}
	if limit > MaxPaginationLimit {
		return MaxPaginationLimit
	}
	return limit
}

// NF Deployment Handlers

// ListNFDeployments lists all NF deployments.
// GET /o2dms/v1/nfDeployments
func (h *Handler) ListNFDeployments(c *gin.Context) {
	h.logger.Info("listing NF deployments")

	adp, err := h.getAdapter(c)
	if err != nil {
		h.errorResponse(c, http.StatusServiceUnavailable, "ServiceUnavailable", err.Error())
		return
	}

	var filter models.ListFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		h.errorResponse(c, http.StatusBadRequest, "BadRequest", "Invalid filter parameters: "+err.Error())
		return
	}

	// Build adapter filter with validated pagination.
	adapterFilter := &adapter.Filter{
		Namespace: filter.Namespace,
		Limit:     validatePaginationLimit(filter.Limit),
		Offset:    filter.Offset,
	}
	if filter.Status != "" {
		adapterFilter.Status = adapter.DeploymentStatus(filter.Status)
	}

	deployments, err := adp.ListDeployments(c.Request.Context(), adapterFilter)
	if err != nil {
		h.logger.Error("failed to list NF deployments", zap.Error(err))
		h.errorResponse(c, http.StatusInternalServerError, "InternalError", "Failed to list NF deployments")
		return
	}

	// Convert to NF deployments.
	nfDeployments := make([]*models.NFDeployment, 0, len(deployments))
	for _, d := range deployments {
		nfDeployments = append(nfDeployments, convertToNFDeployment(d))
	}

	c.JSON(http.StatusOK, models.NFDeploymentListResponse{
		NFDeployments: nfDeployments,
		Total:         len(nfDeployments),
	})
}

// GetNFDeployment retrieves a specific NF deployment.
// GET /o2dms/v1/nfDeployments/:nfDeploymentId
func (h *Handler) GetNFDeployment(c *gin.Context) {
	nfDeploymentID := c.Param("nfDeploymentId")
	h.logger.Info("getting NF deployment", zap.String("nf_deployment_id", nfDeploymentID))

	adp, err := h.getAdapter(c)
	if err != nil {
		h.errorResponse(c, http.StatusServiceUnavailable, "ServiceUnavailable", err.Error())
		return
	}

	deployment, err := adp.GetDeployment(c.Request.Context(), nfDeploymentID)
	if err != nil {
		h.logger.Error("failed to get NF deployment", zap.String("id", nfDeploymentID), zap.Error(err))
		h.errorResponse(c, http.StatusNotFound, "NotFound", "NF deployment not found")
		return
	}

	c.JSON(http.StatusOK, convertToNFDeployment(deployment))
}

// CreateNFDeployment creates a new NF deployment.
// POST /o2dms/v1/nfDeployments
func (h *Handler) CreateNFDeployment(c *gin.Context) {
	h.logger.Info("creating NF deployment")

	adp, err := h.getAdapter(c)
	if err != nil {
		h.errorResponse(c, http.StatusServiceUnavailable, "ServiceUnavailable", err.Error())
		return
	}

	var req models.CreateNFDeploymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.errorResponse(c, http.StatusBadRequest, "BadRequest", "Invalid request body: "+err.Error())
		return
	}

	// Create deployment request.
	deployReq := &adapter.DeploymentRequest{
		Name:        req.Name,
		PackageID:   req.NFDeploymentDescriptorID,
		Namespace:   req.Namespace,
		Values:      req.ParameterValues,
		Description: req.Description,
		Extensions:  req.Extensions,
	}

	deployment, err := adp.CreateDeployment(c.Request.Context(), deployReq)
	if err != nil {
		h.logger.Error("failed to create NF deployment", zap.Error(err))
		h.errorResponse(c, http.StatusInternalServerError, "InternalError", "Failed to create NF deployment")
		return
	}

	h.logger.Info("NF deployment created",
		zap.String("nf_deployment_id", deployment.ID),
		zap.String("name", deployment.Name))

	c.JSON(http.StatusCreated, convertToNFDeployment(deployment))
}

// UpdateNFDeployment updates an existing NF deployment.
// PUT /o2dms/v1/nfDeployments/:nfDeploymentId
func (h *Handler) UpdateNFDeployment(c *gin.Context) {
	nfDeploymentID := c.Param("nfDeploymentId")
	h.logger.Info("updating NF deployment", zap.String("nf_deployment_id", nfDeploymentID))

	adp, err := h.getAdapter(c)
	if err != nil {
		h.errorResponse(c, http.StatusServiceUnavailable, "ServiceUnavailable", err.Error())
		return
	}

	var req models.UpdateNFDeploymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.errorResponse(c, http.StatusBadRequest, "BadRequest", "Invalid request body: "+err.Error())
		return
	}

	update := &adapter.DeploymentUpdate{
		Values:      req.ParameterValues,
		Description: req.Description,
		Extensions:  req.Extensions,
	}

	deployment, err := adp.UpdateDeployment(c.Request.Context(), nfDeploymentID, update)
	if err != nil {
		h.logger.Error("failed to update NF deployment", zap.String("id", nfDeploymentID), zap.Error(err))
		h.errorResponse(c, http.StatusInternalServerError, "InternalError", "Failed to update NF deployment")
		return
	}

	h.logger.Info("NF deployment updated", zap.String("nf_deployment_id", nfDeploymentID))

	c.JSON(http.StatusOK, convertToNFDeployment(deployment))
}

// DeleteNFDeployment deletes an NF deployment.
// DELETE /o2dms/v1/nfDeployments/:nfDeploymentId
func (h *Handler) DeleteNFDeployment(c *gin.Context) {
	nfDeploymentID := c.Param("nfDeploymentId")
	h.logger.Info("deleting NF deployment", zap.String("nf_deployment_id", nfDeploymentID))

	adp, err := h.getAdapter(c)
	if err != nil {
		h.errorResponse(c, http.StatusServiceUnavailable, "ServiceUnavailable", err.Error())
		return
	}

	if err := adp.DeleteDeployment(c.Request.Context(), nfDeploymentID); err != nil {
		h.logger.Error("failed to delete NF deployment", zap.String("id", nfDeploymentID), zap.Error(err))
		h.errorResponse(c, http.StatusInternalServerError, "InternalError", "Failed to delete NF deployment")
		return
	}

	h.logger.Info("NF deployment deleted", zap.String("nf_deployment_id", nfDeploymentID))
	c.Status(http.StatusNoContent)
}

// Lifecycle Operations

// ScaleNFDeployment scales an NF deployment.
// POST /o2dms/v1/nfDeployments/:nfDeploymentId/scale
func (h *Handler) ScaleNFDeployment(c *gin.Context) {
	nfDeploymentID := c.Param("nfDeploymentId")
	h.logger.Info("scaling NF deployment", zap.String("nf_deployment_id", nfDeploymentID))

	adp, err := h.getAdapter(c)
	if err != nil {
		h.errorResponse(c, http.StatusServiceUnavailable, "ServiceUnavailable", err.Error())
		return
	}

	if !adp.SupportsScaling() {
		h.errorResponse(c, http.StatusNotImplemented, "NotImplemented", "Scaling not supported by this adapter")
		return
	}

	var req models.ScaleNFDeploymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.errorResponse(c, http.StatusBadRequest, "BadRequest", "Invalid request body: "+err.Error())
		return
	}

	if err := adp.ScaleDeployment(c.Request.Context(), nfDeploymentID, req.Replicas); err != nil {
		h.logger.Error("failed to scale NF deployment", zap.String("id", nfDeploymentID), zap.Error(err))
		h.errorResponse(c, http.StatusInternalServerError, "InternalError", "Failed to scale NF deployment")
		return
	}

	h.logger.Info("NF deployment scaled",
		zap.String("nf_deployment_id", nfDeploymentID),
		zap.Int("replicas", req.Replicas))

	c.JSON(http.StatusAccepted, gin.H{
		"message":        "Scale operation initiated",
		"nfDeploymentId": nfDeploymentID,
		"targetReplicas": req.Replicas,
	})
}

// RollbackNFDeployment rolls back an NF deployment.
// POST /o2dms/v1/nfDeployments/:nfDeploymentId/rollback
func (h *Handler) RollbackNFDeployment(c *gin.Context) {
	nfDeploymentID := c.Param("nfDeploymentId")
	h.logger.Info("rolling back NF deployment", zap.String("nf_deployment_id", nfDeploymentID))

	adp, err := h.getAdapter(c)
	if err != nil {
		h.errorResponse(c, http.StatusServiceUnavailable, "ServiceUnavailable", err.Error())
		return
	}

	if !adp.SupportsRollback() {
		h.errorResponse(c, http.StatusNotImplemented, "NotImplemented", "Rollback not supported by this adapter")
		return
	}

	var req models.RollbackNFDeploymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.errorResponse(c, http.StatusBadRequest, "BadRequest", "Invalid request body: "+err.Error())
		return
	}

	// Default to previous revision if not specified.
	targetRevision := 0
	if req.TargetRevision != nil {
		targetRevision = *req.TargetRevision
	}

	if err := adp.RollbackDeployment(c.Request.Context(), nfDeploymentID, targetRevision); err != nil {
		h.logger.Error("failed to rollback NF deployment", zap.String("id", nfDeploymentID), zap.Error(err))
		h.errorResponse(c, http.StatusInternalServerError, "InternalError", "Failed to rollback NF deployment")
		return
	}

	h.logger.Info("NF deployment rollback initiated",
		zap.String("nf_deployment_id", nfDeploymentID),
		zap.Int("target_revision", targetRevision))

	c.JSON(http.StatusAccepted, gin.H{
		"message":        "Rollback operation initiated",
		"nfDeploymentId": nfDeploymentID,
		"targetRevision": targetRevision,
	})
}

// GetNFDeploymentStatus retrieves the status of an NF deployment.
// GET /o2dms/v1/nfDeployments/:nfDeploymentId/status
func (h *Handler) GetNFDeploymentStatus(c *gin.Context) {
	nfDeploymentID := c.Param("nfDeploymentId")
	h.logger.Info("getting NF deployment status", zap.String("nf_deployment_id", nfDeploymentID))

	adp, err := h.getAdapter(c)
	if err != nil {
		h.errorResponse(c, http.StatusServiceUnavailable, "ServiceUnavailable", err.Error())
		return
	}

	status, err := adp.GetDeploymentStatus(c.Request.Context(), nfDeploymentID)
	if err != nil {
		h.logger.Error("failed to get NF deployment status", zap.String("id", nfDeploymentID), zap.Error(err))
		h.errorResponse(c, http.StatusNotFound, "NotFound", "NF deployment not found")
		return
	}

	c.JSON(http.StatusOK, convertToStatusResponse(nfDeploymentID, status))
}

// GetNFDeploymentHistory retrieves the history of an NF deployment.
// GET /o2dms/v1/nfDeployments/:nfDeploymentId/history
func (h *Handler) GetNFDeploymentHistory(c *gin.Context) {
	nfDeploymentID := c.Param("nfDeploymentId")
	h.logger.Info("getting NF deployment history", zap.String("nf_deployment_id", nfDeploymentID))

	adp, err := h.getAdapter(c)
	if err != nil {
		h.errorResponse(c, http.StatusServiceUnavailable, "ServiceUnavailable", err.Error())
		return
	}

	history, err := adp.GetDeploymentHistory(c.Request.Context(), nfDeploymentID)
	if err != nil {
		h.logger.Error("failed to get NF deployment history", zap.String("id", nfDeploymentID), zap.Error(err))
		h.errorResponse(c, http.StatusNotFound, "NotFound", "NF deployment not found")
		return
	}

	c.JSON(http.StatusOK, convertToHistoryResponse(history))
}

// NF Deployment Descriptor Handlers

// ListNFDeploymentDescriptors lists all NF deployment descriptors.
// GET /o2dms/v1/nfDeploymentDescriptors
func (h *Handler) ListNFDeploymentDescriptors(c *gin.Context) {
	h.logger.Info("listing NF deployment descriptors")

	adp, err := h.getAdapter(c)
	if err != nil {
		h.errorResponse(c, http.StatusServiceUnavailable, "ServiceUnavailable", err.Error())
		return
	}

	var filter models.ListFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		h.errorResponse(c, http.StatusBadRequest, "BadRequest", "Invalid filter parameters: "+err.Error())
		return
	}

	// Build adapter filter with validated pagination.
	adapterFilter := &adapter.Filter{
		Limit:  validatePaginationLimit(filter.Limit),
		Offset: filter.Offset,
	}

	packages, err := adp.ListDeploymentPackages(c.Request.Context(), adapterFilter)
	if err != nil {
		h.logger.Error("failed to list NF deployment descriptors", zap.Error(err))
		h.errorResponse(c, http.StatusInternalServerError, "InternalError", "Failed to list NF deployment descriptors")
		return
	}

	descriptors := make([]*models.NFDeploymentDescriptor, 0, len(packages))
	for _, pkg := range packages {
		descriptors = append(descriptors, convertToNFDeploymentDescriptor(pkg))
	}

	c.JSON(http.StatusOK, models.NFDeploymentDescriptorListResponse{
		NFDeploymentDescriptors: descriptors,
		Total:                   len(descriptors),
	})
}

// GetNFDeploymentDescriptor retrieves a specific NF deployment descriptor.
// GET /o2dms/v1/nfDeploymentDescriptors/:nfDeploymentDescriptorId
func (h *Handler) GetNFDeploymentDescriptor(c *gin.Context) {
	descriptorID := c.Param("nfDeploymentDescriptorId")
	h.logger.Info("getting NF deployment descriptor", zap.String("descriptor_id", descriptorID))

	adp, err := h.getAdapter(c)
	if err != nil {
		h.errorResponse(c, http.StatusServiceUnavailable, "ServiceUnavailable", err.Error())
		return
	}

	pkg, err := adp.GetDeploymentPackage(c.Request.Context(), descriptorID)
	if err != nil {
		h.logger.Error("failed to get NF deployment descriptor", zap.String("id", descriptorID), zap.Error(err))
		h.errorResponse(c, http.StatusNotFound, "NotFound", "NF deployment descriptor not found")
		return
	}

	c.JSON(http.StatusOK, convertToNFDeploymentDescriptor(pkg))
}

// CreateNFDeploymentDescriptor creates a new NF deployment descriptor.
// POST /o2dms/v1/nfDeploymentDescriptors
func (h *Handler) CreateNFDeploymentDescriptor(c *gin.Context) {
	h.logger.Info("creating NF deployment descriptor")

	adp, err := h.getAdapter(c)
	if err != nil {
		h.errorResponse(c, http.StatusServiceUnavailable, "ServiceUnavailable", err.Error())
		return
	}

	var req models.CreateNFDeploymentDescriptorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.errorResponse(c, http.StatusBadRequest, "BadRequest", "Invalid request body: "+err.Error())
		return
	}

	pkgUpload := &adapter.DeploymentPackageUpload{
		Name:        req.ArtifactName,
		Version:     req.ArtifactVersion,
		PackageType: req.ArtifactType,
		Description: req.Description,
		Repository:  req.ArtifactRepository,
		Extensions:  req.Extensions,
	}

	pkg, err := adp.UploadDeploymentPackage(c.Request.Context(), pkgUpload)
	if err != nil {
		h.logger.Error("failed to create NF deployment descriptor", zap.Error(err))
		h.errorResponse(c, http.StatusInternalServerError, "InternalError", "Failed to create NF deployment descriptor")
		return
	}

	h.logger.Info("NF deployment descriptor created",
		zap.String("descriptor_id", pkg.ID),
		zap.String("name", pkg.Name))

	c.JSON(http.StatusCreated, convertToNFDeploymentDescriptor(pkg))
}

// DeleteNFDeploymentDescriptor deletes an NF deployment descriptor.
// DELETE /o2dms/v1/nfDeploymentDescriptors/:nfDeploymentDescriptorId
func (h *Handler) DeleteNFDeploymentDescriptor(c *gin.Context) {
	descriptorID := c.Param("nfDeploymentDescriptorId")
	h.logger.Info("deleting NF deployment descriptor", zap.String("descriptor_id", descriptorID))

	adp, err := h.getAdapter(c)
	if err != nil {
		h.errorResponse(c, http.StatusServiceUnavailable, "ServiceUnavailable", err.Error())
		return
	}

	if err := adp.DeleteDeploymentPackage(c.Request.Context(), descriptorID); err != nil {
		h.logger.Error("failed to delete NF deployment descriptor", zap.String("id", descriptorID), zap.Error(err))
		h.errorResponse(c, http.StatusInternalServerError, "InternalError", "Failed to delete NF deployment descriptor")
		return
	}

	h.logger.Info("NF deployment descriptor deleted", zap.String("descriptor_id", descriptorID))
	c.Status(http.StatusNoContent)
}

// DMS Subscription Handlers

// ListDMSSubscriptions lists all DMS subscriptions.
// GET /o2dms/v1/subscriptions
func (h *Handler) ListDMSSubscriptions(c *gin.Context) {
	h.logger.Info("listing DMS subscriptions")

	if h.store == nil {
		h.errorResponse(c, http.StatusServiceUnavailable, "ServiceUnavailable", "Subscription storage not configured")
		return
	}

	subs, err := h.store.List(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to list DMS subscriptions", zap.Error(err))
		h.errorResponse(c, http.StatusInternalServerError, "InternalError", "Failed to list subscriptions")
		return
	}

	c.JSON(http.StatusOK, models.DMSSubscriptionListResponse{
		Subscriptions: subs,
		Total:         len(subs),
	})
}

// GetDMSSubscription retrieves a specific DMS subscription.
// GET /o2dms/v1/subscriptions/:subscriptionId
func (h *Handler) GetDMSSubscription(c *gin.Context) {
	subscriptionID := c.Param("subscriptionId")
	h.logger.Info("getting DMS subscription", zap.String("subscription_id", subscriptionID))

	if h.store == nil {
		h.errorResponse(c, http.StatusServiceUnavailable, "ServiceUnavailable", "Subscription storage not configured")
		return
	}

	sub, err := h.store.Get(c.Request.Context(), subscriptionID)
	if err != nil {
		if errors.Is(err, storage.ErrSubscriptionNotFound) {
			h.errorResponse(c, http.StatusNotFound, "NotFound", "Subscription not found")
			return
		}
		h.logger.Error("failed to get DMS subscription", zap.Error(err))
		h.errorResponse(c, http.StatusInternalServerError, "InternalError", "Failed to get subscription")
		return
	}

	c.JSON(http.StatusOK, sub)
}

// CreateDMSSubscription creates a new DMS subscription.
// POST /o2dms/v1/subscriptions
func (h *Handler) CreateDMSSubscription(c *gin.Context) {
	h.logger.Info("creating DMS subscription")

	if h.store == nil {
		h.errorResponse(c, http.StatusServiceUnavailable, "ServiceUnavailable", "Subscription storage not configured")
		return
	}

	var req models.CreateDMSSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.errorResponse(c, http.StatusBadRequest, "BadRequest", "Invalid request body")
		return
	}

	// Validate callback URL for security.
	if err := validateCallbackURL(req.Callback); err != nil {
		h.logger.Warn("invalid callback URL",
			zap.String("callback", redactURL(req.Callback)),
			zap.Error(err))
		h.errorResponse(c, http.StatusBadRequest, "BadRequest", "Invalid callback URL: "+err.Error())
		return
	}

	sub := &models.DMSSubscription{
		SubscriptionID:         uuid.New().String(),
		Callback:               req.Callback,
		ConsumerSubscriptionID: req.ConsumerSubscriptionID,
		Filter:                 req.Filter,
		CreatedAt:              time.Now(),
		UpdatedAt:              time.Now(),
		Extensions:             req.Extensions,
	}

	if err := h.store.Create(c.Request.Context(), sub); err != nil {
		h.logger.Error("failed to create DMS subscription", zap.Error(err))
		h.errorResponse(c, http.StatusInternalServerError, "InternalError", "Failed to create subscription")
		return
	}

	h.logger.Info("DMS subscription created",
		zap.String("subscription_id", sub.SubscriptionID),
		zap.String("callback", redactURL(sub.Callback)))

	c.JSON(http.StatusCreated, sub)
}

// DeleteDMSSubscription deletes a DMS subscription.
// DELETE /o2dms/v1/subscriptions/:subscriptionId
func (h *Handler) DeleteDMSSubscription(c *gin.Context) {
	subscriptionID := c.Param("subscriptionId")
	h.logger.Info("deleting DMS subscription", zap.String("subscription_id", subscriptionID))

	if h.store == nil {
		h.errorResponse(c, http.StatusServiceUnavailable, "ServiceUnavailable", "Subscription storage not configured")
		return
	}

	if err := h.store.Delete(c.Request.Context(), subscriptionID); err != nil {
		if errors.Is(err, storage.ErrSubscriptionNotFound) {
			h.errorResponse(c, http.StatusNotFound, "NotFound", "Subscription not found")
			return
		}
		h.logger.Error("failed to delete DMS subscription", zap.Error(err))
		h.errorResponse(c, http.StatusInternalServerError, "InternalError", "Failed to delete subscription")
		return
	}

	h.logger.Info("DMS subscription deleted", zap.String("subscription_id", subscriptionID))
	c.Status(http.StatusNoContent)
}

// API Info Handlers

// GetDeploymentLifecycleInfo returns O2-DMS deployment lifecycle API information.
// GET /o2dms/v1/deploymentLifecycle
func (h *Handler) GetDeploymentLifecycleInfo(c *gin.Context) {
	h.logger.Info("getting deployment lifecycle info")

	adapters := h.registry.ListMetadata()

	adapterInfo := make([]gin.H, 0, len(adapters))
	for _, meta := range adapters {
		adapterInfo = append(adapterInfo, gin.H{
			"name":         meta.Name,
			"type":         meta.Type,
			"version":      meta.Version,
			"capabilities": meta.Capabilities,
			"healthy":      meta.Healthy,
			"default":      meta.Default,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"apiVersion":  "v1",
		"basePath":    "/o2dms/v1",
		"description": "O2-DMS Deployment Lifecycle Management API",
		"adapters":    adapterInfo,
		"endpoints": []string{
			"/nfDeployments",
			"/nfDeploymentDescriptors",
			"/subscriptions",
		},
	})
}

// Health returns the health status of the DMS subsystem.
func (h *Handler) Health(ctx context.Context) error {
	adp := h.registry.GetDefault()
	if adp == nil {
		return errors.New("no DMS adapter available")
	}
	return adp.Health(ctx)
}

// Conversion helpers

func convertToNFDeployment(d *adapter.Deployment) *models.NFDeployment {
	if d == nil {
		return nil
	}

	return &models.NFDeployment{
		NFDeploymentID:           d.ID,
		Name:                     d.Name,
		Description:              d.Description,
		NFDeploymentDescriptorID: d.PackageID,
		Status:                   convertDeploymentStatus(d.Status),
		Namespace:                d.Namespace,
		Version:                  d.Version,
		CreatedAt:                d.CreatedAt,
		UpdatedAt:                d.UpdatedAt,
		Extensions:               d.Extensions,
	}
}

func convertDeploymentStatus(s adapter.DeploymentStatus) models.NFDeploymentStatus {
	switch s {
	case adapter.DeploymentStatusPending:
		return models.NFDeploymentStatusPending
	case adapter.DeploymentStatusDeploying:
		return models.NFDeploymentStatusInstantiating
	case adapter.DeploymentStatusDeployed:
		return models.NFDeploymentStatusDeployed
	case adapter.DeploymentStatusFailed:
		return models.NFDeploymentStatusFailed
	case adapter.DeploymentStatusRollingBack:
		return models.NFDeploymentStatusUpdating
	case adapter.DeploymentStatusDeleting:
		return models.NFDeploymentStatusTerminating
	default:
		return models.NFDeploymentStatus(s)
	}
}

func convertToNFDeploymentDescriptor(pkg *adapter.DeploymentPackage) *models.NFDeploymentDescriptor {
	if pkg == nil {
		return nil
	}

	return &models.NFDeploymentDescriptor{
		NFDeploymentDescriptorID: pkg.ID,
		Name:                     pkg.Name,
		Description:              pkg.Description,
		ArtifactName:             pkg.Name,
		ArtifactVersion:          pkg.Version,
		ArtifactType:             pkg.PackageType,
		CreatedAt:                pkg.UploadedAt,
		UpdatedAt:                pkg.UploadedAt,
		Extensions:               pkg.Extensions,
	}
}

func convertToStatusResponse(id string, status *adapter.DeploymentStatusDetail) *models.DeploymentStatusResponse {
	if status == nil {
		return nil
	}

	conditions := make([]models.DeploymentCondition, 0, len(status.Conditions))
	for _, c := range status.Conditions {
		conditions = append(conditions, models.DeploymentCondition{
			Type:               c.Type,
			Status:             c.Status,
			Reason:             c.Reason,
			Message:            c.Message,
			LastTransitionTime: c.LastTransitionTime.Format(time.RFC3339),
		})
	}

	return &models.DeploymentStatusResponse{
		NFDeploymentID: id,
		Status:         convertDeploymentStatus(status.Status),
		StatusMessage:  status.Message,
		Progress:       status.Progress,
		Conditions:     conditions,
		UpdatedAt:      status.UpdatedAt.Format(time.RFC3339),
	}
}

func convertToHistoryResponse(history *adapter.DeploymentHistory) *models.DeploymentHistoryResponse {
	if history == nil {
		return nil
	}

	revisions := make([]models.DeploymentRevision, 0, len(history.Revisions))
	for _, r := range history.Revisions {
		revisions = append(revisions, models.DeploymentRevision{
			Revision:    r.Revision,
			Status:      convertDeploymentStatus(r.Status),
			Description: r.Description,
			DeployedAt:  r.DeployedAt.Format(time.RFC3339),
		})
	}

	return &models.DeploymentHistoryResponse{
		NFDeploymentID: history.DeploymentID,
		Revisions:      revisions,
	}
}
