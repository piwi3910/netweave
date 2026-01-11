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
func NewBatchHandler(adp adapter.Adapter, store storage.Store, logger *zap.Logger, metrics *observability.Metrics) *BatchHandler {
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
		metrics = observability.GetMetrics()
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

// BatchCreateSubscriptions handles POST /o2ims/v1/batch/subscriptions.
// Creates multiple subscriptions in a single request.
func (h *BatchHandler) BatchCreateSubscriptions(c *gin.Context) {
	startTime := time.Now()
	ctx := c.Request.Context()

	h.logger.Info("batch creating subscriptions",
		zap.String("request_id", c.GetString("request_id")),
	)

	var req BatchSubscriptionCreate
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("invalid batch request body", zap.Error(err))
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "Invalid request body: " + err.Error(),
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Validate batch size
	batchSize := len(req.Subscriptions)
	if batchSize < MinBatchSize || batchSize > MaxBatchSize {
		h.logger.Warn("invalid batch size",
			zap.Int("batch_size", batchSize),
			zap.Int("min", MinBatchSize),
			zap.Int("max", MaxBatchSize),
		)
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: fmt.Sprintf("Batch size must be between %d and %d, got %d", MinBatchSize, MaxBatchSize, batchSize),
			Code:    http.StatusBadRequest,
		})
		return
	}

	results := make([]BatchResult, len(req.Subscriptions))
	createdIDs := make([]string, 0, len(req.Subscriptions))
	var mu sync.Mutex
	var wg sync.WaitGroup
	var successCount, failureCount int

	// Worker pool to limit concurrency
	semaphore := make(chan struct{}, MaxWorkers)

	for i, sub := range req.Subscriptions {
		wg.Add(1)
		semaphore <- struct{}{} // Acquire worker slot
		go func(idx int, subscription models.Subscription) {
			defer wg.Done()
			defer func() { <-semaphore }() // Release worker slot

			// Check for context cancellation
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

			result := h.createSingleSubscription(ctx, subscription)
			result.Index = idx

			mu.Lock()
			results[idx] = result
			if result.Success {
				successCount++
				if createdSub, ok := result.Data.(*models.Subscription); ok {
					createdIDs = append(createdIDs, createdSub.SubscriptionID)
				}
			} else {
				failureCount++
			}
			mu.Unlock()
		}(i, sub)
	}

	wg.Wait()

	// If atomic and any failure, rollback all created subscriptions
	mu.Lock()
	if req.Atomic && failureCount > 0 {
		mu.Unlock()
		rollbackFailures := h.rollbackSubscriptions(ctx, createdIDs)
		if rollbackFailures > 0 {
			h.logger.Error("atomic rollback incomplete",
				zap.Int("failed_rollbacks", rollbackFailures),
				zap.Int("total_subscriptions", len(createdIDs)),
			)
		}
		mu.Lock()
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
		successCount = 0
		failureCount = len(results)
	}

	response := BatchResponse{
		Results:      results,
		Success:      failureCount == 0,
		SuccessCount: successCount,
		FailureCount: failureCount,
	}
	mu.Unlock()

	statusCode := http.StatusOK
	if response.FailureCount > 0 && response.SuccessCount == 0 {
		statusCode = http.StatusBadRequest
	} else if response.FailureCount > 0 {
		statusCode = http.StatusMultiStatus
	}

	h.logger.Info("batch subscriptions created",
		zap.Int("success_count", response.SuccessCount),
		zap.Int("failure_count", response.FailureCount),
	)

	// Record metrics
	h.metrics.RecordBatchOperation(
		"create_subscriptions",
		req.Atomic,
		time.Since(startTime),
		response.SuccessCount,
		response.FailureCount,
	)

	c.JSON(statusCode, response)
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
		return BatchResult{
			Status:  http.StatusInternalServerError,
			Success: false,
			Error: &models.ErrorResponse{
				Error:   "InternalError",
				Message: fmt.Sprintf("Failed to create subscription: %v", err),
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

// BatchDeleteSubscriptions handles POST /o2ims/v1/batch/subscriptions/delete.
// Deletes multiple subscriptions in a single request.
func (h *BatchHandler) BatchDeleteSubscriptions(c *gin.Context) {
	startTime := time.Now()
	ctx := c.Request.Context()

	h.logger.Info("batch deleting subscriptions",
		zap.String("request_id", c.GetString("request_id")),
	)

	var req BatchSubscriptionDelete
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("invalid batch request body", zap.Error(err))
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "Invalid request body: " + err.Error(),
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Validate batch size
	batchSize := len(req.SubscriptionIDs)
	if batchSize < MinBatchSize || batchSize > MaxBatchSize {
		h.logger.Warn("invalid batch size",
			zap.Int("batch_size", batchSize),
			zap.Int("min", MinBatchSize),
			zap.Int("max", MaxBatchSize),
		)
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: fmt.Sprintf("Batch size must be between %d and %d, got %d", MinBatchSize, MaxBatchSize, batchSize),
			Code:    http.StatusBadRequest,
		})
		return
	}

	results := make([]BatchResult, len(req.SubscriptionIDs))
	successCount := 0
	failureCount := 0

	// Check for context cancellation before processing
	select {
	case <-ctx.Done():
		c.JSON(http.StatusRequestTimeout, models.ErrorResponse{
			Error:   "RequestCanceled",
			Message: "Request canceled or timed out",
			Code:    http.StatusRequestTimeout,
		})
		return
	default:
	}

	// For atomic operations, we need to verify all exist first
	if req.Atomic {
		for i, id := range req.SubscriptionIDs {
			_, err := h.store.Get(ctx, id)
			if err != nil {
				results[i] = BatchResult{
					Index:   i,
					Status:  http.StatusNotFound,
					Success: false,
					Error: &models.ErrorResponse{
						Error:   "NotFound",
						Message: "Subscription not found: " + id,
						Code:    http.StatusNotFound,
					},
				}
				failureCount++
			}
		}

		if failureCount > 0 {
			// Mark all remaining (not yet failed) as failed due to atomic requirement
			for i := range results {
				if results[i].Error == nil {
					results[i] = BatchResult{
						Index:   i,
						Status:  http.StatusConflict,
						Success: false,
						Error: &models.ErrorResponse{
							Error:   "AtomicFailure",
							Message: "Atomic batch failed: some subscriptions not found",
							Code:    http.StatusConflict,
						},
					}
				}
			}

			// Recalculate counts - all operations have now failed
			failureCount = len(results)

			c.JSON(http.StatusBadRequest, BatchResponse{
				Results:      results,
				Success:      false,
				SuccessCount: 0,
				FailureCount: failureCount,
			})
			return
		}
	}

	// Perform deletions
	for i, id := range req.SubscriptionIDs {
		err := h.store.Delete(ctx, id)
		if err != nil {
			results[i] = BatchResult{
				Index:   i,
				Status:  http.StatusNotFound,
				Success: false,
				Error: &models.ErrorResponse{
					Error:   "NotFound",
					Message: "Subscription not found: " + id,
					Code:    http.StatusNotFound,
				},
			}
			failureCount++
		} else {
			results[i] = BatchResult{
				Index:   i,
				Status:  http.StatusNoContent,
				Success: true,
			}
			successCount++
		}
	}

	response := BatchResponse{
		Results:      results,
		Success:      failureCount == 0,
		SuccessCount: successCount,
		FailureCount: failureCount,
	}

	statusCode := http.StatusOK
	if failureCount > 0 && successCount == 0 {
		statusCode = http.StatusNotFound
	} else if failureCount > 0 {
		statusCode = http.StatusMultiStatus
	}

	h.logger.Info("batch subscriptions deleted",
		zap.Int("success_count", successCount),
		zap.Int("failure_count", failureCount),
	)

	// Record metrics
	h.metrics.RecordBatchOperation(
		"delete_subscriptions",
		req.Atomic,
		time.Since(startTime),
		successCount,
		failureCount,
	)

	c.JSON(statusCode, response)
}

// BatchCreateResourcePools handles POST /o2ims/v1/batch/resourcePools.
// Creates multiple resource pools in a single request.
func (h *BatchHandler) BatchCreateResourcePools(c *gin.Context) {
	startTime := time.Now()
	ctx := c.Request.Context()

	h.logger.Info("batch creating resource pools",
		zap.String("request_id", c.GetString("request_id")),
	)

	var req BatchResourcePoolCreate
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("invalid batch request body", zap.Error(err))
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "Invalid request body: " + err.Error(),
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Validate batch size
	batchSize := len(req.ResourcePools)
	if batchSize < MinBatchSize || batchSize > MaxBatchSize {
		h.logger.Warn("invalid batch size",
			zap.Int("batch_size", batchSize),
			zap.Int("min", MinBatchSize),
			zap.Int("max", MaxBatchSize),
		)
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: fmt.Sprintf("Batch size must be between %d and %d, got %d", MinBatchSize, MaxBatchSize, batchSize),
			Code:    http.StatusBadRequest,
		})
		return
	}

	results := make([]BatchResult, len(req.ResourcePools))
	createdIDs := make([]string, 0, len(req.ResourcePools))
	var mu sync.Mutex
	var wg sync.WaitGroup
	var successCount, failureCount int

	// Worker pool to limit concurrency
	semaphore := make(chan struct{}, MaxWorkers)

	for i, pool := range req.ResourcePools {
		wg.Add(1)
		semaphore <- struct{}{} // Acquire worker slot
		go func(idx int, resourcePool models.ResourcePool) {
			defer wg.Done()
			defer func() { <-semaphore }() // Release worker slot

			// Check for context cancellation
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

			result := h.createSingleResourcePool(ctx, resourcePool)
			result.Index = idx

			mu.Lock()
			results[idx] = result
			if result.Success {
				successCount++
				if createdPool, ok := result.Data.(*models.ResourcePool); ok {
					createdIDs = append(createdIDs, createdPool.ResourcePoolID)
				}
			} else {
				failureCount++
			}
			mu.Unlock()
		}(i, pool)
	}

	wg.Wait()

	// If atomic and any failure, rollback all created resource pools
	mu.Lock()
	if req.Atomic && failureCount > 0 {
		mu.Unlock()
		rollbackFailures := h.rollbackResourcePools(ctx, createdIDs)
		if rollbackFailures > 0 {
			h.logger.Error("atomic rollback incomplete",
				zap.Int("failed_rollbacks", rollbackFailures),
				zap.Int("total_resource_pools", len(createdIDs)),
			)
		}
		mu.Lock()
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
		successCount = 0
		failureCount = len(results)
	}

	response := BatchResponse{
		Results:      results,
		Success:      failureCount == 0,
		SuccessCount: successCount,
		FailureCount: failureCount,
	}
	mu.Unlock()

	statusCode := http.StatusOK
	if response.FailureCount > 0 && response.SuccessCount == 0 {
		statusCode = http.StatusBadRequest
	} else if response.FailureCount > 0 {
		statusCode = http.StatusMultiStatus
	}

	h.logger.Info("batch resource pools created",
		zap.Int("success_count", response.SuccessCount),
		zap.Int("failure_count", response.FailureCount),
	)

	// Record metrics
	h.metrics.RecordBatchOperation(
		"create_resource_pools",
		req.Atomic,
		time.Since(startTime),
		response.SuccessCount,
		response.FailureCount,
	)

	c.JSON(statusCode, response)
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
		return BatchResult{
			Status:  http.StatusInternalServerError,
			Success: false,
			Error: &models.ErrorResponse{
				Error:   "InternalError",
				Message: fmt.Sprintf("Failed to create resource pool: %v", err),
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

// BatchDeleteResourcePools handles POST /o2ims/v1/batch/resourcePools/delete.
// Deletes multiple resource pools in a single request.
func (h *BatchHandler) BatchDeleteResourcePools(c *gin.Context) {
	startTime := time.Now()
	ctx := c.Request.Context()

	h.logger.Info("batch deleting resource pools",
		zap.String("request_id", c.GetString("request_id")),
	)

	var req BatchResourcePoolDelete
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Warn("invalid batch request body", zap.Error(err))
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: "Invalid request body: " + err.Error(),
			Code:    http.StatusBadRequest,
		})
		return
	}

	// Validate batch size
	batchSize := len(req.ResourcePoolIDs)
	if batchSize < MinBatchSize || batchSize > MaxBatchSize {
		h.logger.Warn("invalid batch size",
			zap.Int("batch_size", batchSize),
			zap.Int("min", MinBatchSize),
			zap.Int("max", MaxBatchSize),
		)
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Error:   "BadRequest",
			Message: fmt.Sprintf("Batch size must be between %d and %d, got %d", MinBatchSize, MaxBatchSize, batchSize),
			Code:    http.StatusBadRequest,
		})
		return
	}

	results := make([]BatchResult, len(req.ResourcePoolIDs))
	successCount := 0
	failureCount := 0

	// Check for context cancellation before processing
	select {
	case <-ctx.Done():
		c.JSON(http.StatusRequestTimeout, models.ErrorResponse{
			Error:   "RequestCanceled",
			Message: "Request canceled or timed out",
			Code:    http.StatusRequestTimeout,
		})
		return
	default:
	}

	// For atomic operations, verify all exist first
	if req.Atomic {
		for i, id := range req.ResourcePoolIDs {
			_, err := h.adapter.GetResourcePool(ctx, id)
			if err != nil {
				results[i] = BatchResult{
					Index:   i,
					Status:  http.StatusNotFound,
					Success: false,
					Error: &models.ErrorResponse{
						Error:   "NotFound",
						Message: "Resource pool not found: " + id,
						Code:    http.StatusNotFound,
					},
				}
				failureCount++
			}
		}

		if failureCount > 0 {
			// Mark all remaining (not yet failed) as failed due to atomic requirement
			for i := range results {
				if results[i].Error == nil {
					results[i] = BatchResult{
						Index:   i,
						Status:  http.StatusConflict,
						Success: false,
						Error: &models.ErrorResponse{
							Error:   "AtomicFailure",
							Message: "Atomic batch failed: some resource pools not found",
							Code:    http.StatusConflict,
						},
					}
				}
			}

			// Recalculate counts - all operations have now failed
			failureCount = len(results)

			c.JSON(http.StatusBadRequest, BatchResponse{
				Results:      results,
				Success:      false,
				SuccessCount: 0,
				FailureCount: failureCount,
			})
			return
		}
	}

	// Perform deletions
	for i, id := range req.ResourcePoolIDs {
		err := h.adapter.DeleteResourcePool(ctx, id)
		if err != nil {
			results[i] = BatchResult{
				Index:   i,
				Status:  http.StatusNotFound,
				Success: false,
				Error: &models.ErrorResponse{
					Error:   "NotFound",
					Message: "Resource pool not found: " + id,
					Code:    http.StatusNotFound,
				},
			}
			failureCount++
		} else {
			results[i] = BatchResult{
				Index:   i,
				Status:  http.StatusNoContent,
				Success: true,
			}
			successCount++
		}
	}

	response := BatchResponse{
		Results:      results,
		Success:      failureCount == 0,
		SuccessCount: successCount,
		FailureCount: failureCount,
	}

	statusCode := http.StatusOK
	if failureCount > 0 && successCount == 0 {
		statusCode = http.StatusNotFound
	} else if failureCount > 0 {
		statusCode = http.StatusMultiStatus
	}

	h.logger.Info("batch resource pools deleted",
		zap.Int("success_count", successCount),
		zap.Int("failure_count", failureCount),
	)

	// Record metrics
	h.metrics.RecordBatchOperation(
		"delete_resource_pools",
		req.Atomic,
		time.Since(startTime),
		successCount,
		failureCount,
	)

	c.JSON(statusCode, response)
}
