// Package handlers provides HTTP request handlers for the O2-IMS API.
package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/o2ims/models"
	"github.com/piwi3910/netweave/internal/observability"
	"github.com/piwi3910/netweave/internal/storage"
)

const (
	// MaxWorkers limits concurrent operations in batch requests.
	MaxWorkers = 10
	// MinBatchSize is the minimum number of items in a batch request.
	MinBatchSize = 1
	// MaxBatchSize is the maximum number of items in a batch request.
	MaxBatchSize = 100
)

// BatchHandler handles batch operation API endpoints.
type BatchHandler struct {
	adapter adapter.Adapter
	store   storage.Store
	logger  *zap.Logger
	metrics *observability.Metrics
}

// NewBatchHandler creates a new BatchHandler.
// It requires an adapter for backend operations, a store for subscription persistence,
// a logger for structured logging, and metrics for observability.
// If metrics is nil, the global metrics instance will be used.
func NewBatchHandler(
	adp adapter.Adapter,
	store storage.Store,
	logger *zap.Logger,
	metrics *observability.Metrics,
) *BatchHandler {
	if adp == nil {
		panic("adapter cannot be nil")
	}
	if store == nil {
		panic("store cannot be nil")
	}
	if logger == nil {
		panic("logger cannot be nil")
	}

	// Metrics are optional - use global if not provided
	if metrics == nil {
		// Try to get global metrics, but handle initialization gracefully
		if globalMetrics := observability.GetMetrics(); globalMetrics != nil {
			metrics = globalMetrics
		} else {
			// Initialize with default namespace for testing
			metrics = observability.InitMetrics("test")
		}
	}

	return &BatchHandler{
		adapter: adp,
		store:   store,
		logger:  logger,
		metrics: metrics,
	}
}

// BatchRequest represents a batch operation request.
type BatchRequest struct {
	// Operations is the list of operations to perform.
	Operations []BatchOperation `json:"operations" binding:"required,min=1,max=100"`
	// Atomic indicates whether all operations should succeed or fail together.
	Atomic bool `json:"atomic,omitempty"`
}

// BatchOperation represents a single operation in a batch request.
type BatchOperation struct {
	// Method is the HTTP method (POST, PUT, DELETE).
	Method string `json:"method" binding:"required,oneof=POST PUT DELETE"`
	// Path is the resource path (e.g., "/subscriptions", "/resourcePools").
	Path string `json:"path" binding:"required"`
	// Body is the request payload for POST/PUT operations.
	Body interface{} `json:"body,omitempty"`
}

// BatchResponse represents a batch operation response.
type BatchResponse struct {
	// Results contains the result of each operation.
	Results []BatchResult `json:"results"`
	// Success indicates whether all operations succeeded.
	Success bool `json:"success"`
	// SuccessCount is the number of successful operations.
	SuccessCount int `json:"successCount"`
	// FailureCount is the number of failed operations.
	FailureCount int `json:"failureCount"`
}

// BatchResult represents the result of a single batch operation.
type BatchResult struct {
	// Index is the operation index in the request.
	Index int `json:"index"`
	// Status is the HTTP status code.
	Status int `json:"status"`
	// Success indicates whether the operation succeeded.
	Success bool `json:"success"`
	// Data contains the response data for successful operations.
	Data interface{} `json:"data,omitempty"`
	// Error contains error details for failed operations.
	Error *models.ErrorResponse `json:"error,omitempty"`
}

// BatchSubscriptionCreate represents a batch subscription creation request.
type BatchSubscriptionCreate struct {
	// Subscriptions is the list of subscriptions to create.
	Subscriptions []models.Subscription `json:"subscriptions" binding:"required,min=1,max=100"`
	// Atomic indicates whether all operations should succeed or fail together.
	Atomic bool `json:"atomic,omitempty"`
}

// BatchSubscriptionDelete represents a batch subscription deletion request.
type BatchSubscriptionDelete struct {
	// SubscriptionIDs is the list of subscription IDs to delete.
	SubscriptionIDs []string `json:"subscriptionIds" binding:"required,min=1,max=100"`
	// Atomic indicates whether all operations should succeed or fail together.
	Atomic bool `json:"atomic,omitempty"`
}

// BatchResourcePoolCreate represents a batch resource pool creation request.
type BatchResourcePoolCreate struct {
	// ResourcePools is the list of resource pools to create.
	ResourcePools []models.ResourcePool `json:"resourcePools" binding:"required,min=1,max=100"`
	// Atomic indicates whether all operations should succeed or fail together.
	Atomic bool `json:"atomic,omitempty"`
}

// BatchResourcePoolDelete represents a batch resource pool deletion request.
type BatchResourcePoolDelete struct {
	// ResourcePoolIDs is the list of resource pool IDs to delete.
	ResourcePoolIDs []string `json:"resourcePoolIds" binding:"required,min=1,max=100"`
	// Atomic indicates whether all operations should succeed or fail together.
	Atomic bool `json:"atomic,omitempty"`
}

// BatchSubscriptionUpdate represents a batch subscription update request.
type BatchSubscriptionUpdate struct {
	// Updates is the list of subscription updates to perform.
	Updates []SubscriptionUpdateItem `json:"updates" binding:"required,min=1,max=100"`
	// Atomic indicates whether all operations should succeed or fail together.
	Atomic bool `json:"atomic,omitempty"`
}

// SubscriptionUpdateItem represents a single subscription update in a batch.
type SubscriptionUpdateItem struct {
	// SubscriptionID is the ID of the subscription to update.
	SubscriptionID string `json:"subscriptionId" binding:"required"`
	// Update contains the fields to update (partial updates allowed).
	Update models.Subscription `json:"update"`
}

// BatchResourcePoolUpdate represents a batch resource pool update request.
type BatchResourcePoolUpdate struct {
	// Updates is the list of resource pool updates to perform.
	Updates []ResourcePoolUpdateItem `json:"updates" binding:"required,min=1,max=100"`
	// Atomic indicates whether all operations should succeed or fail together.
	Atomic bool `json:"atomic,omitempty"`
}

// ResourcePoolUpdateItem represents a single resource pool update in a batch.
type ResourcePoolUpdateItem struct {
	// ResourcePoolID is the ID of the resource pool to update.
	ResourcePoolID string `json:"resourcePoolId" binding:"required"`
	// Update contains the fields to update (partial updates allowed).
	Update models.ResourcePool `json:"update"`
}

// batchOperationFunc defines a function that processes a single batch item.
// It returns the result and optionally a created ID for rollback purposes.
type batchOperationFunc func(ctx context.Context, idx int) (BatchResult, string)

// rollbackFunc defines a function that rolls back created items.
type rollbackFunc func(ctx context.Context, ids []string) int

// batchConfig holds configuration for a batch operation.
type batchConfig struct {
	operationName string
	atomic        bool
	itemCount     int
	useWorkerPool bool // true for creates, false for deletes
}

// executeBatch is the core generic batch processor that eliminates code duplication.
// It handles request validation, worker pool execution, atomic rollback, and metrics.
func (h *BatchHandler) executeBatch(
	c *gin.Context,
	config batchConfig,
	operation batchOperationFunc,
	rollback rollbackFunc,
) {
	startTime := time.Now()
	ctx := c.Request.Context()

	h.logger.Info(
		fmt.Sprintf("batch %s", config.operationName),
		zap.String("request_id", c.GetString("request_id")),
		zap.Int("item_count", config.itemCount),
		zap.Bool("atomic", config.atomic),
	)

	// Validate batch size
	if err := h.validateBatchSize(config.itemCount); err != nil {
		h.logger.Warn("invalid batch size", zap.Error(err))
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: err.Error(),
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Execute operations
	results, successCount, failureCount, createdIDs := h.executeOperations(
		ctx,
		config,
		operation,
	)

	// Handle atomic rollback if needed
	if h.shouldRollback(config.atomic, failureCount, createdIDs, rollback) {
		h.performRollback(ctx, rollback, createdIDs, results)
		successCount = 0
		failureCount = len(results)
	}

	// Send response
	h.sendBatchResponse(c, config, startTime, results, successCount, failureCount)
}

// validateBatchSize validates that the batch size is within limits.
func (h *BatchHandler) validateBatchSize(size int) error {
	if size < MinBatchSize || size > MaxBatchSize {
		return fmt.Errorf(
			"batch size must be between %d and %d, got %d",
			MinBatchSize,
			MaxBatchSize,
			size,
		)
	}
	return nil
}

// executeOperations runs batch operations using the appropriate execution strategy.
func (h *BatchHandler) executeOperations(
	ctx context.Context,
	config batchConfig,
	operation batchOperationFunc,
) ([]BatchResult, int, int, []string) {
	if config.useWorkerPool {
		return h.executeWithWorkerPool(ctx, config.itemCount, operation)
	}
	results, successCount, failureCount := h.executeSequentially(ctx, config.itemCount, operation)
	return results, successCount, failureCount, nil
}

// shouldRollback determines if rollback is needed.
func (h *BatchHandler) shouldRollback(
	atomic bool,
	failureCount int,
	createdIDs []string,
	rollback rollbackFunc,
) bool {
	return atomic && failureCount > 0 && len(createdIDs) > 0 && rollback != nil
}

// performRollback executes rollback and marks operations as rolled back.
func (h *BatchHandler) performRollback(
	ctx context.Context,
	rollback rollbackFunc,
	createdIDs []string,
	results []BatchResult,
) {
	rollbackFailures := rollback(ctx, createdIDs)
	if rollbackFailures > 0 {
		h.logger.Error("atomic rollback incomplete",
			zap.Int("failed_rollbacks", rollbackFailures),
			zap.Int("total_items", len(createdIDs)),
		)
	}

	// Mark all successful operations as rolled back
	for i := range results {
		if results[i].Success {
			results[i].Success = false
			results[i].Status = http.StatusConflict
			results[i].Data = nil
			results[i].Error = &models.ErrorResponse{
				Error:   "RolledBack",
				Message: "Operation rolled back due to atomic batch failure",
				Code:    http.StatusConflict,
			}
		}
	}
}

// sendBatchResponse builds and sends the batch response.
func (h *BatchHandler) sendBatchResponse(
	c *gin.Context,
	config batchConfig,
	startTime time.Time,
	results []BatchResult,
	successCount, failureCount int,
) {
	response := BatchResponse{
		Results:      results,
		Success:      failureCount == 0,
		SuccessCount: successCount,
		FailureCount: failureCount,
	}

	statusCode := h.determineStatusCode(successCount, failureCount)

	h.logger.Info(
		fmt.Sprintf("batch %s completed", config.operationName),
		zap.Int("success_count", successCount),
		zap.Int("failure_count", failureCount),
	)

	h.metrics.RecordBatchOperation(
		config.operationName,
		config.atomic,
		time.Since(startTime),
		successCount,
		failureCount,
	)

	c.JSON(statusCode, response)
}

// executeWithWorkerPool processes items concurrently using a worker pool.
// Used for create operations that may take longer.
func (h *BatchHandler) executeWithWorkerPool(
	ctx context.Context,
	count int,
	operation batchOperationFunc,
) ([]BatchResult, int, int, []string) {
	results := make([]BatchResult, count)
	// Use index-aligned storage to prevent race condition where order doesn't match results
	createdIDsAligned := make([]string, count)
	var successCount, failureCount int
	var mu sync.Mutex
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, MaxWorkers)

	for i := 0; i < count; i++ {
		wg.Add(1)
		semaphore <- struct{}{}
		go func(idx int) {
			defer wg.Done()
			defer func() { <-semaphore }()

			select {
			case <-ctx.Done():
				mu.Lock()
				results[idx] = BatchResult{
					Index:   idx,
					Status:  http.StatusRequestTimeout,
					Success: false,
					Error: &models.ErrorResponse{
						Error:   "RequestCanceled",
						Message: "Request canceled or timed out",
						Code:    http.StatusRequestTimeout,
					},
				}
				failureCount++
				mu.Unlock()
				return
			default:
			}

			result, createdID := operation(ctx, idx)
			result.Index = idx

			mu.Lock()
			results[idx] = result
			if result.Success {
				successCount++
				if createdID != "" {
					createdIDsAligned[idx] = createdID
				}
			} else {
				failureCount++
			}
			mu.Unlock()
		}(i)
	}

	wg.Wait()

	// Filter out empty strings to build final createdIDs list
	createdIDs := make([]string, 0, successCount)
	for _, id := range createdIDsAligned {
		if id != "" {
			createdIDs = append(createdIDs, id)
		}
	}

	return results, successCount, failureCount, createdIDs
}

// executeSequentially processes items one by one without concurrency.
// Used for delete operations that are typically fast.
func (h *BatchHandler) executeSequentially(
	ctx context.Context,
	count int,
	operation batchOperationFunc,
) ([]BatchResult, int, int) {
	results := make([]BatchResult, count)
	var successCount, failureCount int

	// Check for context cancellation before processing
	select {
	case <-ctx.Done():
		for i := 0; i < count; i++ {
			results[i] = BatchResult{
				Index:   i,
				Status:  http.StatusRequestTimeout,
				Success: false,
				Error: &models.ErrorResponse{
					Error:   "RequestCanceled",
					Message: "Request canceled or timed out",
					Code:    http.StatusRequestTimeout,
				},
			}
		}
		failureCount = count
		return results, successCount, failureCount
	default:
	}

	for i := 0; i < count; i++ {
		result, _ := operation(ctx, i)
		result.Index = i
		results[i] = result

		if result.Success {
			successCount++
		} else {
			failureCount++
		}
	}

	return results, successCount, failureCount
}

// determineStatusCode determines HTTP status based on success/failure counts.
// Returns:
//   - 400 Bad Request: All operations failed (total failure)
//   - 207 Multi-Status: Mixed success and failure (partial success)
//   - 200 OK: All operations succeeded (total success)
//
// Rationale: Using 400 for total failure allows clients to distinguish between
// "total failure" and "partial failure" without parsing the response body.
// This follows RFC 7231 semantics where 400 indicates the request couldn't
// be processed due to client error (invalid data in all items).
func (h *BatchHandler) determineStatusCode(successCount, failureCount int) int {
	if failureCount > 0 && successCount == 0 {
		// All operations failed - return 400 to indicate total failure
		return http.StatusBadRequest
	}
	if failureCount > 0 {
		// Mixed results - return 207 Multi-Status
		return http.StatusMultiStatus
	}
	// All operations succeeded
	return http.StatusOK
}

// handleBindError handles JSON binding errors.
func (h *BatchHandler) handleBindError(c *gin.Context, err error) {
	h.logger.Warn("invalid batch request body", zap.Error(err))
	c.JSON(http.StatusBadRequest, models.ErrorResponse{
		Error:   "BadRequest",
		Message: "Invalid request body: " + err.Error(),
		Code:    http.StatusBadRequest,
	})
}

// sendAtomicValidationFailure sends a failure response for atomic validation.
func (h *BatchHandler) sendAtomicValidationFailure(
	c *gin.Context,
	count int,
	message string,
) {
	results := make([]BatchResult, count)
	for i := range results {
		results[i] = BatchResult{
			Index:   i,
			Status:  http.StatusConflict,
			Success: false,
			Error: &models.ErrorResponse{
				Error:   "AtomicFailure",
				Message: "Atomic batch failed: " + message,
				Code:    http.StatusConflict,
			},
		}
	}
	c.JSON(http.StatusBadRequest, BatchResponse{
		Results:      results,
		Success:      false,
		SuccessCount: 0,
		FailureCount: count,
	})
}

// BatchCreateSubscriptions handles POST /o2ims/v1/batch/subscriptions.
// Creates multiple subscriptions in a single request.
func (h *BatchHandler) BatchCreateSubscriptions(c *gin.Context) {
	var req BatchSubscriptionCreate
	if err := c.ShouldBindJSON(&req); err != nil {
		h.handleBindError(c, err)
		return
	}

	config := batchConfig{
		operationName: "create_subscriptions",
		atomic:        req.Atomic,
		itemCount:     len(req.Subscriptions),
		useWorkerPool: true,
	}

	operation := func(ctx context.Context, idx int) (BatchResult, string) {
		return h.executeSubscriptionCreate(ctx, req.Subscriptions[idx])
	}

	h.executeBatch(c, config, operation, h.rollbackSubscriptions)
}

// executeSubscriptionCreate processes a single subscription creation.
func (h *BatchHandler) executeSubscriptionCreate(
	ctx context.Context,
	sub models.Subscription,
) (BatchResult, string) {
	result := h.createSingleSubscription(ctx, sub)
	var createdID string
	if result.Success {
		if sub, ok := result.Data.(*models.Subscription); ok {
			createdID = sub.SubscriptionID
		}
	}
	return result, createdID
}

// BatchDeleteSubscriptions handles POST /o2ims/v1/batch/subscriptions/delete.
// Deletes multiple subscriptions in a single request.
func (h *BatchHandler) BatchDeleteSubscriptions(c *gin.Context) {
	var req BatchSubscriptionDelete
	if err := c.ShouldBindJSON(&req); err != nil {
		h.handleBindError(c, err)
		return
	}

	// Pre-validation for atomic operations
	if req.Atomic && !h.validateSubscriptionsExist(c, req.SubscriptionIDs) {
		return
	}

	config := batchConfig{
		operationName: "delete_subscriptions",
		atomic:        req.Atomic,
		itemCount:     len(req.SubscriptionIDs),
		useWorkerPool: false,
	}

	operation := func(ctx context.Context, idx int) (BatchResult, string) {
		return h.deleteSubscription(ctx, req.SubscriptionIDs[idx])
	}

	h.executeBatch(c, config, operation, nil)
}

// BatchUpdateSubscriptions handles POST /o2ims/v1/batch/subscriptions/update.
// Updates multiple subscriptions in a single request.
func (h *BatchHandler) BatchUpdateSubscriptions(c *gin.Context) {
	var req BatchSubscriptionUpdate
	if err := c.ShouldBindJSON(&req); err != nil {
		h.handleBindError(c, err)
		return
	}

	// Pre-validation for atomic operations
	if req.Atomic && !h.validateSubscriptionUpdatesExist(c, req.Updates) {
		return
	}

	config := batchConfig{
		operationName: "update_subscriptions",
		atomic:        req.Atomic,
		itemCount:     len(req.Updates),
		useWorkerPool: true,
	}

	operation := func(ctx context.Context, idx int) (BatchResult, string) {
		return h.executeSubscriptionUpdate(ctx, req.Updates[idx])
	}

	h.executeBatch(c, config, operation, nil)
}

// executeSubscriptionUpdate processes a single subscription update.
func (h *BatchHandler) executeSubscriptionUpdate(
	ctx context.Context,
	item SubscriptionUpdateItem,
) (BatchResult, string) {
	result := h.updateSingleSubscription(ctx, item.SubscriptionID, item.Update)
	return result, ""
}

// updateSingleSubscription updates a single subscription.
func (h *BatchHandler) updateSingleSubscription(
	ctx context.Context,
	id string,
	update models.Subscription,
) BatchResult {
	// Get existing subscription
	existing, err := h.store.Get(ctx, id)
	if err != nil {
		return h.makeSubscriptionNotFoundResult(id, err)
	}

	// Apply updates
	h.applySubscriptionUpdates(existing, update)

	// Update in storage
	if err := h.store.Update(ctx, existing); err != nil {
		return h.makeSubscriptionUpdateFailedResult(id, err)
	}

	h.logger.Info("subscription updated",
		zap.String("subscriptionID", id))

	return BatchResult{
		Status:  http.StatusOK,
		Success: true,
		Data:    existing,
	}
}

// applySubscriptionUpdates applies update fields to existing subscription.
func (h *BatchHandler) applySubscriptionUpdates(
	existing *storage.Subscription,
	update models.Subscription,
) {
	if update.Callback != "" {
		existing.Callback = update.Callback
	}
	if update.ConsumerSubscriptionID != "" {
		existing.ConsumerSubscriptionID = update.ConsumerSubscriptionID
	}
	// Update filter fields if provided
	h.applySubscriptionFilterUpdates(&existing.Filter, update.Filter)
}

// applySubscriptionFilterUpdates applies filter updates.
func (h *BatchHandler) applySubscriptionFilterUpdates(
	existing *storage.SubscriptionFilter,
	update models.SubscriptionFilter,
) {
	if len(update.ResourcePoolID) > 0 {
		existing.ResourcePoolID = update.ResourcePoolID[0]
	}
	if len(update.ResourceTypeID) > 0 {
		existing.ResourceTypeID = update.ResourceTypeID[0]
	}
	if len(update.ResourceID) > 0 {
		existing.ResourceID = update.ResourceID[0]
	}
}

// makeSubscriptionNotFoundResult creates a not found error result.
func (h *BatchHandler) makeSubscriptionNotFoundResult(
	id string,
	err error,
) BatchResult {
	h.logger.Error("failed to get subscription for update",
		zap.String("subscriptionID", id),
		zap.Error(err))
	return BatchResult{
		Status:  http.StatusNotFound,
		Success: false,
		Error: &models.ErrorResponse{
			Error:   "NotFound",
			Message: fmt.Sprintf("subscription %s not found", id),
			Code:    http.StatusNotFound,
		},
	}
}

// makeSubscriptionUpdateFailedResult creates an update failed error result.
func (h *BatchHandler) makeSubscriptionUpdateFailedResult(
	id string,
	err error,
) BatchResult {
	h.logger.Error("failed to update subscription",
		zap.String("subscriptionID", id),
		zap.Error(err))
	return BatchResult{
		Status:  http.StatusInternalServerError,
		Success: false,
		Error: &models.ErrorResponse{
			Error:   "InternalError",
			Message: fmt.Sprintf("failed to update subscription: %v", err),
			Code:    http.StatusInternalServerError,
		},
	}
}

// validateSubscriptionUpdatesExist validates all subscriptions exist for atomic operations.
func (h *BatchHandler) validateSubscriptionUpdatesExist(
	c *gin.Context,
	updates []SubscriptionUpdateItem,
) bool {
	ctx := c.Request.Context()
	for _, item := range updates {
		if _, err := h.store.Get(ctx, item.SubscriptionID); err != nil {
			h.sendAtomicValidationFailure(c, len(updates), "some subscriptions not found")
			return false
		}
	}
	return true
}

// validateSubscriptionsExist validates all subscriptions exist for atomic operations.
func (h *BatchHandler) validateSubscriptionsExist(
	c *gin.Context,
	ids []string,
) bool {
	ctx := c.Request.Context()
	for _, id := range ids {
		if _, err := h.store.Get(ctx, id); err != nil {
			h.sendAtomicValidationFailure(c, len(ids), "some subscriptions not found")
			return false
		}
	}
	return true
}

// deleteSubscription deletes a single subscription.
func (h *BatchHandler) deleteSubscription(
	ctx context.Context,
	id string,
) (BatchResult, string) {
	err := h.store.Delete(ctx, id)
	if err != nil {
		return BatchResult{
			Status:  http.StatusNotFound,
			Success: false,
			Error: &models.ErrorResponse{
				Error:   "NotFound",
				Message: "Subscription not found: " + id,
				Code:    http.StatusNotFound,
			},
		}, ""
	}
	return BatchResult{
		Status:  http.StatusNoContent,
		Success: true,
	}, ""
}

// BatchCreateResourcePools handles POST /o2ims/v1/batch/resourcePools.
// Creates multiple resource pools in a single request.
func (h *BatchHandler) BatchCreateResourcePools(c *gin.Context) {
	var req BatchResourcePoolCreate
	if err := c.ShouldBindJSON(&req); err != nil {
		h.handleBindError(c, err)
		return
	}

	config := batchConfig{
		operationName: "create_resource_pools",
		atomic:        req.Atomic,
		itemCount:     len(req.ResourcePools),
		useWorkerPool: true,
	}

	operation := func(ctx context.Context, idx int) (BatchResult, string) {
		return h.executeResourcePoolCreate(ctx, req.ResourcePools[idx])
	}

	h.executeBatch(c, config, operation, h.rollbackResourcePools)
}

// executeResourcePoolCreate processes a single resource pool creation.
func (h *BatchHandler) executeResourcePoolCreate(
	ctx context.Context,
	pool models.ResourcePool,
) (BatchResult, string) {
	result := h.createSingleResourcePool(ctx, pool)
	var createdID string
	if result.Success {
		if pool, ok := result.Data.(*models.ResourcePool); ok {
			createdID = pool.ResourcePoolID
		}
	}
	return result, createdID
}

// BatchDeleteResourcePools handles POST /o2ims/v1/batch/resourcePools/delete.
// Deletes multiple resource pools in a single request.
func (h *BatchHandler) BatchDeleteResourcePools(c *gin.Context) {
	var req BatchResourcePoolDelete
	if err := c.ShouldBindJSON(&req); err != nil {
		h.handleBindError(c, err)
		return
	}

	// Pre-validation for atomic operations
	if req.Atomic && !h.validateResourcePoolsExist(c, req.ResourcePoolIDs) {
		return
	}

	config := batchConfig{
		operationName: "delete_resource_pools",
		atomic:        req.Atomic,
		itemCount:     len(req.ResourcePoolIDs),
		useWorkerPool: false,
	}

	operation := func(ctx context.Context, idx int) (BatchResult, string) {
		return h.deleteResourcePool(ctx, req.ResourcePoolIDs[idx])
	}

	h.executeBatch(c, config, operation, nil)
}

// BatchUpdateResourcePools handles POST /o2ims/v1/batch/resourcePools/update.
// Updates multiple resource pools in a single request.
func (h *BatchHandler) BatchUpdateResourcePools(c *gin.Context) {
	var req BatchResourcePoolUpdate
	if err := c.ShouldBindJSON(&req); err != nil {
		h.handleBindError(c, err)
		return
	}

	// Pre-validation for atomic operations
	if req.Atomic && !h.validateResourcePoolUpdatesExist(c, req.Updates) {
		return
	}

	config := batchConfig{
		operationName: "update_resource_pools",
		atomic:        req.Atomic,
		itemCount:     len(req.Updates),
		useWorkerPool: true,
	}

	operation := func(ctx context.Context, idx int) (BatchResult, string) {
		return h.executeResourcePoolUpdate(ctx, req.Updates[idx])
	}

	h.executeBatch(c, config, operation, nil)
}

// executeResourcePoolUpdate processes a single resource pool update.
func (h *BatchHandler) executeResourcePoolUpdate(
	ctx context.Context,
	item ResourcePoolUpdateItem,
) (BatchResult, string) {
	result := h.updateSingleResourcePool(ctx, item.ResourcePoolID, item.Update)
	return result, ""
}

// updateSingleResourcePool updates a single resource pool.
func (h *BatchHandler) updateSingleResourcePool(
	ctx context.Context,
	id string,
	update models.ResourcePool,
) BatchResult {
	// Get existing resource pool
	existing, err := h.adapter.GetResourcePool(ctx, id)
	if err != nil {
		h.logger.Error("failed to get resource pool for update",
			zap.String("resourcePoolID", id),
			zap.Error(err))
		return BatchResult{
			Status:  http.StatusNotFound,
			Success: false,
			Error: &models.ErrorResponse{
				Error:   "NotFound",
				Message: fmt.Sprintf("resource pool %s not found", id),
				Code:    http.StatusNotFound,
			},
		}
	}

	// Apply updates to existing resource pool
	if update.Name != "" {
		existing.Name = update.Name
	}
	if update.Description != "" {
		existing.Description = update.Description
	}
	if update.Location != "" {
		existing.Location = update.Location
	}
	if len(update.Extensions) > 0 {
		existing.Extensions = update.Extensions
	}

	// Update via adapter
	updatedPool, err := h.adapter.UpdateResourcePool(ctx, id, existing)
	if err != nil {
		h.logger.Error("failed to update resource pool",
			zap.String("resourcePoolID", id),
			zap.Error(err))
		return BatchResult{
			Status:  http.StatusInternalServerError,
			Success: false,
			Error: &models.ErrorResponse{
				Error:   "InternalError",
				Message: fmt.Sprintf("failed to update resource pool: %v", err),
				Code:    http.StatusInternalServerError,
			},
		}
	}

	h.logger.Info("resource pool updated",
		zap.String("resourcePoolID", id))

	return BatchResult{
		Status:  http.StatusOK,
		Success: true,
		Data:    updatedPool,
	}
}

// validateResourcePoolUpdatesExist validates all resource pools exist for atomic operations.
func (h *BatchHandler) validateResourcePoolUpdatesExist(
	c *gin.Context,
	updates []ResourcePoolUpdateItem,
) bool {
	ctx := c.Request.Context()
	for _, item := range updates {
		if _, err := h.adapter.GetResourcePool(ctx, item.ResourcePoolID); err != nil {
			h.sendAtomicValidationFailure(c, len(updates), "some resource pools not found")
			return false
		}
	}
	return true
}

// validateResourcePoolsExist validates all resource pools exist for atomic operations.
func (h *BatchHandler) validateResourcePoolsExist(
	c *gin.Context,
	ids []string,
) bool {
	ctx := c.Request.Context()
	for _, id := range ids {
		if _, err := h.adapter.GetResourcePool(ctx, id); err != nil {
			h.sendAtomicValidationFailure(c, len(ids), "some resource pools not found")
			return false
		}
	}
	return true
}

// deleteResourcePool deletes a single resource pool.
func (h *BatchHandler) deleteResourcePool(
	ctx context.Context,
	id string,
) (BatchResult, string) {
	err := h.adapter.DeleteResourcePool(ctx, id)
	if err != nil {
		return BatchResult{
			Status:  http.StatusNotFound,
			Success: false,
			Error: &models.ErrorResponse{
				Error:   "NotFound",
				Message: "Resource pool not found: " + id,
				Code:    http.StatusNotFound,
			},
		}, ""
	}
	return BatchResult{
		Status:  http.StatusNoContent,
		Success: true,
	}, ""
}

// createSingleSubscription creates a single subscription and returns the result.
func (h *BatchHandler) createSingleSubscription(
	ctx context.Context,
	sub models.Subscription,
) BatchResult {
	// Validate callback URL
	if sub.Callback == "" {
		return BatchResult{
			Status:  http.StatusBadRequest,
			Success: false,
			Error: &models.ErrorResponse{
				Error:   "BadRequest",
				Message: "Callback URL is required",
				Code:    http.StatusBadRequest,
			},
		}
	}

	if _, err := url.ParseRequestURI(sub.Callback); err != nil {
		return BatchResult{
			Status:  http.StatusBadRequest,
			Success: false,
			Error: &models.ErrorResponse{
				Error:   "BadRequest",
				Message: "Invalid callback URL: " + err.Error(),
				Code:    http.StatusBadRequest,
			},
		}
	}

	subscriptionID := uuid.New().String()

	storageSub := &storage.Subscription{
		ID:                     subscriptionID,
		Callback:               sub.Callback,
		ConsumerSubscriptionID: sub.ConsumerSubscriptionID,
	}

	if len(sub.Filter.ResourcePoolID) > 0 {
		storageSub.Filter.ResourcePoolID = sub.Filter.ResourcePoolID[0]
	}
	if len(sub.Filter.ResourceTypeID) > 0 {
		storageSub.Filter.ResourceTypeID = sub.Filter.ResourceTypeID[0]
	}
	if len(sub.Filter.ResourceID) > 0 {
		storageSub.Filter.ResourceID = sub.Filter.ResourceID[0]
	}

	if err := h.store.Create(ctx, storageSub); err != nil {
		h.logger.Error("failed to create subscription",
			zap.String("subscription_id", subscriptionID),
			zap.Error(err),
		)
		return BatchResult{
			Status:  http.StatusInternalServerError,
			Success: false,
			Error: &models.ErrorResponse{
				Error:   "InternalError",
				Message: "Failed to create subscription",
				Code:    http.StatusInternalServerError,
			},
		}
	}

	createdSub := &models.Subscription{
		SubscriptionID:         subscriptionID,
		Callback:               storageSub.Callback,
		ConsumerSubscriptionID: storageSub.ConsumerSubscriptionID,
		Filter:                 sub.Filter,
		CreatedAt:              storageSub.CreatedAt,
	}

	return BatchResult{
		Status:  http.StatusCreated,
		Success: true,
		Data:    createdSub,
	}
}

// createSingleResourcePool creates a single resource pool and returns the result.
func (h *BatchHandler) createSingleResourcePool(
	ctx context.Context,
	pool models.ResourcePool,
) BatchResult {
	adapterPool := &adapter.ResourcePool{
		ResourcePoolID:   pool.ResourcePoolID,
		Name:             pool.Name,
		Description:      pool.Description,
		Location:         pool.Location,
		OCloudID:         pool.OCloudID,
		GlobalLocationID: pool.GlobalAssetID,
		Extensions:       pool.Extensions,
	}

	createdPool, err := h.adapter.CreateResourcePool(ctx, adapterPool)
	if err != nil {
		h.logger.Error("failed to create resource pool",
			zap.String("resource_pool_id", pool.ResourcePoolID),
			zap.Error(err),
		)
		return BatchResult{
			Status:  http.StatusInternalServerError,
			Success: false,
			Error: &models.ErrorResponse{
				Error:   "InternalError",
				Message: "Failed to create resource pool",
				Code:    http.StatusInternalServerError,
			},
		}
	}

	resultPool := &models.ResourcePool{
		ResourcePoolID: createdPool.ResourcePoolID,
		Name:           createdPool.Name,
		Description:    createdPool.Description,
		Location:       createdPool.Location,
		OCloudID:       createdPool.OCloudID,
		GlobalAssetID:  createdPool.GlobalLocationID,
		Extensions:     createdPool.Extensions,
	}

	return BatchResult{
		Status:  http.StatusCreated,
		Success: true,
		Data:    resultPool,
	}
}

// rollbackSubscriptions deletes the given subscription IDs.
// Returns the number of failed rollback operations.
func (h *BatchHandler) rollbackSubscriptions(ctx context.Context, ids []string) int {
	var rollbackFailures int
	for _, id := range ids {
		if err := h.store.Delete(ctx, id); err != nil {
			rollbackFailures++
			h.logger.Error("failed to rollback subscription",
				zap.String("subscription_id", id),
				zap.Error(err),
			)
		}
	}
	if rollbackFailures > 0 {
		h.logger.Warn("partial rollback failure",
			zap.Int("total_rollbacks", len(ids)),
			zap.Int("failed_rollbacks", rollbackFailures),
		)
	}
	return rollbackFailures
}

// rollbackResourcePools deletes the given resource pool IDs.
// Returns the number of failed rollback operations.
func (h *BatchHandler) rollbackResourcePools(ctx context.Context, ids []string) int {
	var rollbackFailures int
	for _, id := range ids {
		if err := h.adapter.DeleteResourcePool(ctx, id); err != nil {
			rollbackFailures++
			h.logger.Error("failed to rollback resource pool",
				zap.String("resource_pool_id", id),
				zap.Error(err),
			)
		}
	}
	if rollbackFailures > 0 {
		h.logger.Warn("partial rollback failure",
			zap.Int("total_rollbacks", len(ids)),
			zap.Int("failed_rollbacks", rollbackFailures),
		)
	}
	return rollbackFailures
}
