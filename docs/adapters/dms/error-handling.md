# O2-DMS Error Handling

**Version:** 1.0
**Last Updated:** 2026-01-14

## Overview

This document describes the error handling implementation in the O2-DMS (Deployment Management Services) subsystem of netweave. It covers O-RAN specification compliance, error response formats, error categories, sentinel errors, validation logic, and best practices for error handling.

**Key Features:**
- ✅ O-RAN compliant error response format
- ✅ Comprehensive validation (DNS-1123, callback URLs, parameters)
- ✅ Sentinel errors for common error conditions
- ✅ Structured error logging with context
- ✅ Security-focused error messages (no sensitive data leakage)
- ✅ Adapter error wrapping with full context

---

## Table of Contents

1. [O-RAN Error Response Format](#o-ran-error-response-format)
2. [Error Categories](#error-categories)
3. [Sentinel Errors](#sentinel-errors)
4. [Validation Errors](#validation-errors)
5. [Error Handling Patterns](#error-handling-patterns)
6. [Adapter Error Wrapping](#adapter-error-wrapping)
7. [Security Considerations](#security-considerations)
8. [Error Examples by Operation](#error-examples-by-operation)
9. [Troubleshooting Guide](#troubleshooting-guide)
10. [Best Practices](#best-practices)

---

## O-RAN Error Response Format

### Specification Compliance

The O2-DMS API follows the O-RAN specification error response format defined in **O-RAN.WG6.O2DMS-INTERFACE v3.0.0**.

### Error Response Structure

```go
// APIError represents an O2-DMS API error response.
type APIError struct {
    // Error is the error type identifier (e.g., "NotFound", "BadRequest").
    Error string `json:"error"`

    // Message provides human-readable error details.
    Message string `json:"message"`

    // Code is the HTTP status code.
    Code int `json:"code"`

    // Details provides additional error context (optional).
    Details map[string]interface{} `json:"details,omitempty"`
}
```

### Example Error Response

```json
{
  "error": "NotFound",
  "message": "NF deployment with ID 'nginx-123' does not exist",
  "code": 404,
  "details": {
    "deploymentId": "nginx-123",
    "adapter": "helm"
  }
}
```

### Error Response Method

All DMS handlers use a standardized error response method:

```go
// errorResponse sends a standardized error response.
func (h *Handler) errorResponse(c *gin.Context, code int, errType, message string) {
    c.JSON(code, models.APIError{
        Error:   errType,
        Message: message,
        Code:    code,
    })
}
```

**Usage Pattern:**
```go
if err != nil {
    h.errorResponse(c, http.StatusNotFound, "NotFound", "NF deployment not found")
    return
}
```

---

## Error Categories

### 4xx Client Errors

Client errors indicate that the request was invalid or cannot be fulfilled due to client-side issues.

#### 400 Bad Request

**Error Type:** `BadRequest`

**Common Causes:**
- Invalid request body (malformed JSON)
- Missing required fields
- Invalid query parameters
- Validation failures (DNS-1123, URL format)

**Example:**
```json
{
  "error": "BadRequest",
  "message": "Invalid filter parameters: limit must be between 1 and 1000",
  "code": 400
}
```

**Handler Implementation:**
```go
var filter models.ListFilter
if err := c.ShouldBindQuery(&filter); err != nil {
    h.errorResponse(c, http.StatusBadRequest, "BadRequest",
        "Invalid filter parameters: "+err.Error())
    return
}
```

#### 404 Not Found

**Error Type:** `NotFound`

**Common Causes:**
- Resource does not exist
- Invalid deployment ID
- Package not found

**Example:**
```json
{
  "error": "NotFound",
  "message": "NF deployment not found",
  "code": 404
}
```

**Handler Implementation:**
```go
if errors.Is(err, adapter.ErrDeploymentNotFound) {
    h.errorResponse(c, http.StatusNotFound, "NotFound", "NF deployment not found")
    return
}
```

#### 409 Conflict

**Error Type:** `Conflict`

**Common Causes:**
- Resource already exists (duplicate deployment name)
- Deployment in incompatible state for operation
- Version conflicts

**Example:**
```json
{
  "error": "Conflict",
  "message": "Deployment 'nginx-prod' already exists in namespace 'default'",
  "code": 409
}
```

#### 422 Unprocessable Entity

**Error Type:** `ValidationError`

**Common Causes:**
- DNS-1123 naming validation failures
- Invalid Helm values
- Semantic validation failures

**Example:**
```json
{
  "error": "ValidationError",
  "message": "Deployment name 'Nginx_Prod' must match DNS-1123 format: lowercase alphanumeric with hyphens",
  "code": 422
}
```

### 5xx Server Errors

Server errors indicate internal failures or backend system issues.

#### 500 Internal Server Error

**Error Type:** `InternalError`

**Common Causes:**
- Unexpected exceptions
- Backend adapter failures
- Database errors
- Network failures

**Example:**
```json
{
  "error": "InternalError",
  "message": "Failed to create NF deployment",
  "code": 500
}
```

**Handler Implementation:**
```go
if err != nil {
    h.logger.Error("failed to create deployment", zap.Error(err))
    h.errorResponse(c, http.StatusInternalServerError, "InternalError",
        "Failed to create NF deployment")
    return
}
```

#### 503 Service Unavailable

**Error Type:** `ServiceUnavailable`

**Common Causes:**
- No DMS adapter configured
- Adapter not found
- Backend system unavailable
- Dependency failures

**Example:**
```json
{
  "error": "ServiceUnavailable",
  "message": "adapter not found: helm",
  "code": 503
}
```

**Handler Implementation:**
```go
adp, err := h.getAdapterFromQuery(c)
if err != nil {
    h.errorResponse(c, http.StatusServiceUnavailable, "ServiceUnavailable", err.Error())
    return
}
```

---

## Sentinel Errors

### DMS Adapter Sentinel Errors

Defined in `internal/dms/adapter/adapter.go`:

```go
var (
    // ErrDeploymentNotFound is returned when a deployment is not found.
    ErrDeploymentNotFound = errors.New("deployment not found")

    // ErrPackageNotFound is returned when a deployment package is not found.
    ErrPackageNotFound = errors.New("deployment package not found")

    // ErrOperationNotSupported is returned when an operation is not supported.
    ErrOperationNotSupported = errors.New("operation not supported")
)
```

### Usage Pattern

Handlers use `errors.Is()` to check for sentinel errors:

```go
if err != nil {
    if errors.Is(err, adapter.ErrDeploymentNotFound) {
        h.errorResponse(c, http.StatusNotFound, "NotFound", "NF deployment not found")
    } else if errors.Is(err, adapter.ErrOperationNotSupported) {
        h.errorResponse(c, http.StatusNotImplemented, "NotImplemented",
            "Operation not supported by adapter")
    } else {
        h.errorResponse(c, http.StatusInternalServerError, "InternalError",
            "Failed to process request")
    }
    return
}
```

### Why Sentinel Errors?

1. **Type-Safe Error Checking**: `errors.Is()` provides type-safe error matching
2. **Error Wrapping**: Works with `fmt.Errorf("%w", err)` for error context
3. **Decoupling**: Handlers don't need to know adapter implementation details
4. **Consistent Behavior**: Same error handling across all DMS adapters

---

## Validation Errors

### DNS-1123 Name Validation

**Purpose**: Ensure deployment and package names are valid Kubernetes resource names.

**Rules:**
- Lowercase alphanumeric characters and hyphens only
- Must start with alphanumeric
- Must end with alphanumeric
- Maximum 253 characters

**Implementation:**
```go
var dns1123Regex = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

func ValidateDNS1123Name(name string) error {
    if name == "" {
        return errors.New("name cannot be empty")
    }
    if len(name) > 253 {
        return errors.New("name cannot exceed 253 characters")
    }
    if !dns1123Regex.MatchString(name) {
        return errors.New("name must match DNS-1123 format: lowercase alphanumeric with hyphens")
    }
    return nil
}
```

**Example Error:**
```json
{
  "error": "BadRequest",
  "message": "Invalid deployment name 'MyApp_v1': name must match DNS-1123 format: lowercase alphanumeric with hyphens",
  "code": 400
}
```

### Callback URL Validation

**Purpose**: Prevent SSRF attacks and ensure callback URLs are valid HTTPS endpoints.

**Security Checks:**
- ✅ Valid URL format
- ✅ HTTPS required (production)
- ✅ Not a private IP address (10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16)
- ✅ Not localhost or cloud metadata endpoints
- ✅ DNS resolution successful

**Implementation:**
```go
func ValidateCallbackURL(callbackURL string) error {
    parsed, err := url.Parse(callbackURL)
    if err != nil {
        return errors.New("invalid URL format")
    }

    // Validate scheme (HTTPS required in production)
    if parsed.Scheme != "https" && os.Getenv("ENV") == "production" {
        return errors.New("callback URL must use HTTPS in production")
    }

    // Validate host is not private IP
    host := parsed.Hostname()
    if isPrivateIP(host) {
        return errors.New("callback URL cannot be a private IP address")
    }

    // Validate DNS resolution
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    if _, err := net.DefaultResolver.LookupHost(ctx, host); err != nil {
        return fmt.Errorf("callback URL host cannot be resolved: %w", err)
    }

    return nil
}
```

**Example Error:**
```json
{
  "error": "BadRequest",
  "message": "Invalid callback URL: callback URL must use HTTPS in production",
  "code": 400
}
```

### Query Parameter Validation

**Pagination Validation:**
```go
const (
    MaxPaginationLimit     = 1000
    DefaultPaginationLimit = 100
)

func validatePagination(offset, limit int) error {
    if offset < 0 {
        return errors.New("offset must be non-negative")
    }
    if limit < 1 || limit > MaxPaginationLimit {
        return fmt.Errorf("limit must be between 1 and %d", MaxPaginationLimit)
    }
    return nil
}
```

---

## Error Handling Patterns

### Pattern 1: Generic Delete Handler

**Purpose**: Reusable delete handler with consistent error handling.

```go
func (h *Handler) handleDelete(
    c *gin.Context,
    paramName string,
    logMsg string,
    deleteFn func(context.Context, string) error,
    notFoundErr error,
    notFoundMsg string,
    errorMsg string,
) {
    id := c.Param(paramName)
    h.logger.Info(logMsg, zap.String(paramName, id))

    if err := deleteFn(c.Request.Context(), id); err != nil {
        h.logger.Error(errorMsg, zap.String("id", id), zap.Error(err))
        if errors.Is(err, notFoundErr) {
            h.errorResponse(c, http.StatusNotFound, "NotFound", notFoundMsg)
        } else {
            h.errorResponse(c, http.StatusInternalServerError, "InternalError", errorMsg)
        }
        return
    }

    h.logger.Info(logMsg+" success", zap.String(paramName, id))
    c.Status(http.StatusNoContent)
}
```

**Usage:**
```go
func (h *Handler) DeleteNFDeployment(c *gin.Context) {
    adp, err := h.getAdapterFromQuery(c)
    if err != nil {
        h.errorResponse(c, http.StatusServiceUnavailable, "ServiceUnavailable", err.Error())
        return
    }

    h.handleDelete(
        c,
        "nfDeploymentId",
        "deleting NF deployment",
        adp.DeleteDeployment,
        adapter.ErrDeploymentNotFound,
        "NF deployment not found",
        "Failed to delete NF deployment",
    )
}
```

### Pattern 2: Adapter Retrieval with Error Handling

```go
func (h *Handler) getAdapterFromQuery(c *gin.Context) (adapter.DMSAdapter, error) {
    adapterName := c.Query("adapter")
    var adp adapter.DMSAdapter

    if adapterName != "" {
        h.registry.Mu.RLock()
        adp = h.registry.Plugins[adapterName]
        h.registry.Mu.RUnlock()

        if adp == nil {
            return nil, fmt.Errorf("adapter not found: %s", adapterName)
        }
        return adp, nil
    }

    // Use default adapter
    h.registry.Mu.RLock()
    if h.registry.DefaultPlugin != "" {
        adp = h.registry.Plugins[h.registry.DefaultPlugin]
    }
    h.registry.Mu.RUnlock()

    if adp == nil {
        return nil, fmt.Errorf("no default DMS adapter configured")
    }

    return adp, nil
}
```

**Usage:**
```go
adp, err := h.getAdapterFromQuery(c)
if err != nil {
    h.errorResponse(c, http.StatusServiceUnavailable, "ServiceUnavailable", err.Error())
    return
}
```

### Pattern 3: Structured Error Logging

```go
// ✅ GOOD: Structured logging with context
h.logger.Error("failed to create deployment",
    zap.String("deploymentId", req.ID),
    zap.String("name", req.Name),
    zap.String("adapter", adapterName),
    zap.Error(err),
)

// ❌ BAD: Unstructured logging
h.logger.Error(fmt.Sprintf("failed to create deployment %s: %v", req.Name, err))
```

---

## Adapter Error Wrapping

### Error Context Preservation

Adapters wrap errors with context using `fmt.Errorf("%w", err)`:

```go
// In Helm adapter
func (a *Adapter) CreateDeployment(ctx context.Context, req *DeploymentRequest) (*Deployment, error) {
    _, err := a.client.Install(release, chart, req.Values)
    if err != nil {
        // Wrap error with context
        return nil, fmt.Errorf("failed to install Helm release %s: %w", req.Name, err)
    }
    return deployment, nil
}
```

### Handler Error Unwrapping

Handlers use `errors.Is()` to check wrapped errors:

```go
deployment, err := adp.CreateDeployment(ctx, &req)
if err != nil {
    h.logger.Error("failed to create deployment", zap.Error(err))

    // Check for specific error types
    if errors.Is(err, adapter.ErrDeploymentNotFound) {
        h.errorResponse(c, http.StatusNotFound, "NotFound", "NF deployment not found")
    } else if errors.Is(err, adapter.ErrOperationNotSupported) {
        h.errorResponse(c, http.StatusNotImplemented, "NotImplemented",
            "Operation not supported by adapter")
    } else {
        // Generic error response
        h.errorResponse(c, http.StatusInternalServerError, "InternalError",
            "Failed to create NF deployment")
    }
    return
}
```

---

## Security Considerations

### 1. No Sensitive Data in Error Messages

**❌ BAD:**
```go
// Exposes internal details
h.errorResponse(c, http.StatusInternalServerError, "InternalError",
    fmt.Sprintf("Database connection failed: postgres://user:password@host:5432/db"))
```

**✅ GOOD:**
```go
// Generic message, full error in logs
h.logger.Error("database connection failed", zap.Error(err))
h.errorResponse(c, http.StatusInternalServerError, "InternalError",
    "Failed to connect to backend storage")
```

### 2. Sanitize User Input in Errors

**❌ BAD:**
```go
// Includes unsanitized user input
h.errorResponse(c, http.StatusBadRequest, "BadRequest",
    "Invalid deployment name: "+req.Name)
```

**✅ GOOD:**
```go
// Validates first, then includes in error
if err := ValidateDNS1123Name(req.Name); err != nil {
    h.errorResponse(c, http.StatusBadRequest, "BadRequest",
        "Invalid deployment name: "+err.Error())
    return
}
```

### 3. SSRF Protection in Callback URLs

**Implementation:**
```go
func ValidateCallbackURL(callbackURL string) error {
    parsed, err := url.Parse(callbackURL)
    if err != nil {
        return errors.New("invalid URL format")
    }

    host := parsed.Hostname()

    // Block private IP ranges
    if isPrivateIP(host) {
        return errors.New("callback URL cannot be a private IP address")
    }

    // Block cloud metadata endpoints
    if isCloudMetadata(host) {
        return errors.New("callback URL cannot be a cloud metadata endpoint")
    }

    return nil
}

func isPrivateIP(host string) bool {
    ip := net.ParseIP(host)
    if ip == nil {
        return false
    }

    // Check private ranges: 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16
    return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()
}
```

### 4. Rate Limiting Error Responses

**Example:**
```json
{
  "error": "TooManyRequests",
  "message": "Rate limit exceeded: 100 requests per minute",
  "code": 429,
  "details": {
    "retryAfter": 60
  }
}
```

---

## Error Examples by Operation

### Create NF Deployment

**Success:**
```bash
curl -X POST "http://localhost:8080/o2dms/v1/nfDeployments" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "nginx-prod",
    "descriptorId": "nginx-chart-1.0.0",
    "namespace": "production"
  }'
```

**Response:**
```json
{
  "nfDeploymentId": "nginx-prod-abc123",
  "name": "nginx-prod",
  "status": "INSTANTIATED"
}
```

**Error: Invalid Name (400)**
```bash
curl -X POST "http://localhost:8080/o2dms/v1/nfDeployments" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Nginx_Prod",
    "descriptorId": "nginx-chart-1.0.0"
  }'
```

**Response:**
```json
{
  "error": "BadRequest",
  "message": "Invalid deployment name: name must match DNS-1123 format: lowercase alphanumeric with hyphens",
  "code": 400
}
```

**Error: Missing Required Field (400)**
```bash
curl -X POST "http://localhost:8080/o2dms/v1/nfDeployments" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "nginx-prod"
  }'
```

**Response:**
```json
{
  "error": "BadRequest",
  "message": "Invalid request: descriptorId is required",
  "code": 400
}
```

**Error: Adapter Unavailable (503)**
```bash
curl -X POST "http://localhost:8080/o2dms/v1/nfDeployments?adapter=argocd" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "nginx-prod",
    "descriptorId": "nginx-chart-1.0.0"
  }'
```

**Response:**
```json
{
  "error": "ServiceUnavailable",
  "message": "adapter not found: argocd",
  "code": 503
}
```

### Get NF Deployment

**Error: Not Found (404)**
```bash
curl "http://localhost:8080/o2dms/v1/nfDeployments/nonexistent-id"
```

**Response:**
```json
{
  "error": "NotFound",
  "message": "NF deployment not found",
  "code": 404
}
```

### List NF Deployments

**Error: Invalid Pagination (400)**
```bash
curl "http://localhost:8080/o2dms/v1/nfDeployments?limit=2000"
```

**Response:**
```json
{
  "error": "BadRequest",
  "message": "Invalid filter parameters: limit must be between 1 and 1000",
  "code": 400
}
```

### Delete NF Deployment

**Success:**
```bash
curl -X DELETE "http://localhost:8080/o2dms/v1/nfDeployments/nginx-prod-abc123"
```

**Response:** `204 No Content`

**Error: Not Found (404)**
```bash
curl -X DELETE "http://localhost:8080/o2dms/v1/nfDeployments/nonexistent-id"
```

**Response:**
```json
{
  "error": "NotFound",
  "message": "NF deployment not found",
  "code": 404
}
```

---

## Troubleshooting Guide

### Common Error Scenarios

#### Error: "adapter not found"

**Symptom:**
```json
{
  "error": "ServiceUnavailable",
  "message": "adapter not found: helm",
  "code": 503
}
```

**Causes:**
1. Adapter not initialized in `cmd/gateway/main.go`
2. Adapter name mismatch in query parameter
3. Adapter registration failed during startup

**Solutions:**
1. Check gateway logs for adapter initialization errors
2. Verify adapter is registered: `GET /o2dms`
3. Check adapter name: `GET /o2dms/v1/deploymentLifecycle`

#### Error: "no default DMS adapter configured"

**Symptom:**
```json
{
  "error": "ServiceUnavailable",
  "message": "no default DMS adapter configured",
  "code": 503
}
```

**Causes:**
1. No adapters registered
2. Default adapter not set during registration

**Solutions:**
1. Verify `initializeDMS()` calls `Register()` with `isDefault=true`
2. Check DMS registry initialization in gateway logs

#### Error: "failed to install Helm release"

**Symptom:**
```json
{
  "error": "InternalError",
  "message": "Failed to create NF deployment",
  "code": 500
}
```

**Gateway Logs:**
```
ERROR failed to create deployment {"deploymentId": "nginx-prod", "error": "failed to install Helm release nginx-prod: chart not found"}
```

**Causes:**
1. Helm chart not available in repository
2. Invalid chart name or version
3. Repository authentication failure
4. Kubernetes RBAC permissions

**Solutions:**
1. Verify chart exists: `helm search repo <chart-name>`
2. Check Helm repository configuration
3. Verify gateway service account permissions
4. Review Helm adapter logs for detailed errors

---

## Best Practices

### 1. Always Use Structured Logging

**✅ GOOD:**
```go
h.logger.Error("failed to create deployment",
    zap.String("deploymentId", req.ID),
    zap.String("name", req.Name),
    zap.String("adapter", adapterName),
    zap.Error(err),
)
```

**❌ BAD:**
```go
log.Printf("Error creating deployment %s: %v", req.Name, err)
```

### 2. Validate Early, Return Immediately

**✅ GOOD:**
```go
if err := ValidateDNS1123Name(req.Name); err != nil {
    h.errorResponse(c, http.StatusBadRequest, "BadRequest", err.Error())
    return
}

if err := ValidateCallbackURL(req.Callback); err != nil {
    h.errorResponse(c, http.StatusBadRequest, "BadRequest", err.Error())
    return
}

// Proceed with business logic
```

**❌ BAD:**
```go
// No validation, fails later
deployment, err := adp.CreateDeployment(ctx, &req)
if err != nil {
    // Error could have been prevented
}
```

### 3. Use Sentinel Errors for Expected Conditions

**✅ GOOD:**
```go
if errors.Is(err, adapter.ErrDeploymentNotFound) {
    h.errorResponse(c, http.StatusNotFound, "NotFound", "NF deployment not found")
    return
}
```

**❌ BAD:**
```go
if strings.Contains(err.Error(), "not found") {
    // Fragile string matching
}
```

### 4. Wrap Errors with Context

**✅ GOOD:**
```go
if err := a.client.Install(release, chart, values); err != nil {
    return nil, fmt.Errorf("failed to install Helm release %s: %w", release, err)
}
```

**❌ BAD:**
```go
if err := a.client.Install(release, chart, values); err != nil {
    return nil, err  // Lost context
}
```

### 5. Return Generic Errors to Clients, Log Details

**✅ GOOD:**
```go
h.logger.Error("database query failed",
    zap.String("query", query),
    zap.Error(err),
)
h.errorResponse(c, http.StatusInternalServerError, "InternalError",
    "Failed to retrieve deployments")
```

**❌ BAD:**
```go
h.errorResponse(c, http.StatusInternalServerError, "InternalError",
    fmt.Sprintf("Database query failed: %v", err))  // Exposes internal details
```

### 6. Test Error Paths

**Example Test:**
```go
func TestCreateDeployment_ValidationError(t *testing.T) {
    tests := []struct {
        name        string
        request     DeploymentRequest
        wantCode    int
        wantError   string
    }{
        {
            name: "invalid name - uppercase",
            request: DeploymentRequest{
                Name:         "Nginx-Prod",
                DescriptorID: "nginx-1.0.0",
            },
            wantCode:  http.StatusBadRequest,
            wantError: "name must match DNS-1123 format",
        },
        {
            name: "missing descriptor",
            request: DeploymentRequest{
                Name: "nginx-prod",
            },
            wantCode:  http.StatusBadRequest,
            wantError: "descriptorId is required",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            w := httptest.NewRecorder()
            c, _ := gin.CreateTestContext(w)

            handler.CreateNFDeployment(c)

            assert.Equal(t, tt.wantCode, w.Code)

            var response models.APIError
            err := json.Unmarshal(w.Body.Bytes(), &response)
            require.NoError(t, err)
            assert.Contains(t, response.Message, tt.wantError)
        })
    }
}
```

---

## Testing Error Handling

### Unit Tests

**Test all error paths:**
```go
func TestHandler_ErrorResponse(t *testing.T) {
    tests := []struct {
        name     string
        code     int
        errType  string
        message  string
        want     models.APIError
    }{
        {
            name:    "not found error",
            code:    http.StatusNotFound,
            errType: "NotFound",
            message: "Resource not found",
            want: models.APIError{
                Error:   "NotFound",
                Message: "Resource not found",
                Code:    404,
            },
        },
        // Add more test cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            w := httptest.NewRecorder()
            c, _ := gin.CreateTestContext(w)

            handler := NewHandler(registry, store, logger)
            handler.errorResponse(c, tt.code, tt.errType, tt.message)

            var got models.APIError
            err := json.Unmarshal(w.Body.Bytes(), &got)
            require.NoError(t, err)
            assert.Equal(t, tt.want, got)
        })
    }
}
```

### Integration Tests

**Test with real adapters:**
```go
func TestCreateDeployment_Integration(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test")
    }

    // Setup: Real Helm adapter
    adapter := setupHelmAdapter(t)
    handler := NewHandler(registry, store, logger)

    // Test: Create deployment with invalid name
    req := DeploymentRequest{
        Name:         "Invalid_Name",
        DescriptorID: "nginx-1.0.0",
    }

    w := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(w)

    handler.CreateNFDeployment(c)

    assert.Equal(t, http.StatusBadRequest, w.Code)

    var response models.APIError
    err := json.Unmarshal(w.Body.Bytes(), &response)
    require.NoError(t, err)
    assert.Equal(t, "BadRequest", response.Error)
    assert.Contains(t, response.Message, "DNS-1123")
}
```

---

## Monitoring and Alerting

### Error Metrics

**Prometheus Metrics:**
```go
var (
    dmsErrorsTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "o2dms_errors_total",
            Help: "Total number of DMS API errors",
        },
        []string{"error_type", "endpoint", "adapter"},
    )

    dmsErrorLatency = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "o2dms_error_latency_seconds",
            Help:    "Time to process error responses",
            Buckets: prometheus.DefBuckets,
        },
        []string{"error_type"},
    )
)
```

**Instrumentation:**
```go
func (h *Handler) errorResponse(c *gin.Context, code int, errType, message string) {
    // Increment error counter
    dmsErrorsTotal.WithLabelValues(
        errType,
        c.Request.URL.Path,
        c.Query("adapter"),
    ).Inc()

    c.JSON(code, models.APIError{
        Error:   errType,
        Message: message,
        Code:    code,
    })
}
```

### Alert Rules

**Prometheus AlertManager:**
```yaml
groups:
  - name: o2dms_errors
    rules:
      - alert: HighDMSErrorRate
        expr: rate(o2dms_errors_total[5m]) > 10
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High DMS API error rate"
          description: "DMS API error rate is {{ $value }} errors/sec"

      - alert: DMSAdapterUnavailable
        expr: sum(rate(o2dms_errors_total{error_type="ServiceUnavailable"}[5m])) > 1
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "DMS adapter unavailable"
          description: "DMS adapter is returning ServiceUnavailable errors"
```

---

## References

### O-RAN Specifications

- **O2-DMS v3.0.0**: [Deployment Management Services](https://specifications.o-ran.org/specifications?specificationId=O-RAN.WG6.O2DMS-INTERFACE)

### Related Documentation

- [DMS Adapter Overview](README.md)
- [Package Management](package-management.md)
- [Lifecycle Operations](lifecycle-operations.md)
- [Helm Adapter](helm.md)

### Go Error Handling

- [Go Blog: Error Handling](https://go.dev/blog/error-handling-and-go)
- [Go Blog: Working with Errors in Go 1.13](https://go.dev/blog/go1.13-errors)

---

**For questions about error handling or to report issues, please open a GitHub issue.**
