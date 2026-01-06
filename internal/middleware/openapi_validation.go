// Package middleware provides HTTP middleware for the O2-IMS Gateway.
// It includes request/response validation using OpenAPI specifications.
package middleware

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/gorillamux"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// OpenAPISpecs embeds the OpenAPI specification files.
//
//go:embed specs/*.yaml
var OpenAPISpecs embed.FS

// ValidationConfig holds configuration for the OpenAPI validation middleware.
type ValidationConfig struct {
	// SpecPath is the path to the OpenAPI specification file.
	// If empty, the embedded spec will be used.
	SpecPath string

	// ValidateRequest enables request validation against the OpenAPI spec.
	ValidateRequest bool

	// ValidateResponse enables response validation against the OpenAPI spec.
	// This should typically only be enabled in development/testing.
	ValidateResponse bool

	// ExcludePaths is a list of path prefixes to exclude from validation.
	// Health check endpoints are automatically excluded.
	ExcludePaths []string

	// Logger is the logger for validation errors.
	Logger *zap.Logger
}

// DefaultValidationConfig returns the default validation configuration.
func DefaultValidationConfig() *ValidationConfig {
	return &ValidationConfig{
		ValidateRequest:  true,
		ValidateResponse: false,
		ExcludePaths: []string{
			"/health",
			"/healthz",
			"/ready",
			"/readyz",
			"/metrics",
		},
	}
}

// OpenAPIValidator provides OpenAPI-based request/response validation.
type OpenAPIValidator struct {
	config *ValidationConfig
	router routers.Router
	spec   *openapi3.T
	mu     sync.RWMutex
	logger *zap.Logger
}

// NewOpenAPIValidator creates a new OpenAPI validator with the given configuration.
func NewOpenAPIValidator(cfg *ValidationConfig) (*OpenAPIValidator, error) {
	if cfg == nil {
		cfg = DefaultValidationConfig()
	}

	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	validator := &OpenAPIValidator{
		config: cfg,
		logger: logger,
	}

	return validator, nil
}

// LoadSpec loads the OpenAPI specification from the given content.
func (v *OpenAPIValidator) LoadSpec(specContent []byte) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	loader := openapi3.NewLoader()
	spec, err := loader.LoadFromData(specContent)
	if err != nil {
		return fmt.Errorf("failed to parse OpenAPI spec: %w", err)
	}

	if err := spec.Validate(context.Background()); err != nil {
		return fmt.Errorf("invalid OpenAPI spec: %w", err)
	}

	router, err := gorillamux.NewRouter(spec)
	if err != nil {
		return fmt.Errorf("failed to create OpenAPI router: %w", err)
	}

	v.spec = spec
	v.router = router

	v.logger.Info("OpenAPI spec loaded successfully",
		zap.String("title", spec.Info.Title),
		zap.String("version", spec.Info.Version),
	)

	return nil
}

// LoadSpecFromFile loads the OpenAPI specification from a file path.
func (v *OpenAPIValidator) LoadSpecFromFile(path string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	loader := openapi3.NewLoader()
	spec, err := loader.LoadFromFile(path)
	if err != nil {
		return fmt.Errorf("failed to load OpenAPI spec from file: %w", err)
	}

	if err := spec.Validate(context.Background()); err != nil {
		return fmt.Errorf("invalid OpenAPI spec: %w", err)
	}

	router, err := gorillamux.NewRouter(spec)
	if err != nil {
		return fmt.Errorf("failed to create OpenAPI router: %w", err)
	}

	v.spec = spec
	v.router = router

	v.logger.Info("OpenAPI spec loaded from file",
		zap.String("path", path),
		zap.String("title", spec.Info.Title),
		zap.String("version", spec.Info.Version),
	)

	return nil
}

// Spec returns the loaded OpenAPI specification.
func (v *OpenAPIValidator) Spec() *openapi3.T {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.spec
}

// isExcludedPath checks if the given path should be excluded from validation.
func (v *OpenAPIValidator) isExcludedPath(path string) bool {
	for _, excluded := range v.config.ExcludePaths {
		if strings.HasPrefix(path, excluded) {
			return true
		}
	}
	return false
}

// Middleware returns a Gin middleware function for OpenAPI validation.
func (v *OpenAPIValidator) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		v.mu.RLock()
		router := v.router
		v.mu.RUnlock()

		if router == nil {
			v.logger.Warn("OpenAPI spec not loaded, skipping validation")
			c.Next()
			return
		}

		path := c.Request.URL.Path
		if v.isExcludedPath(path) {
			c.Next()
			return
		}

		if v.config.ValidateRequest {
			if err := v.validateRequest(c); err != nil {
				return
			}
		}

		if v.config.ValidateResponse {
			v.validateResponseWithCapture(c)
			return
		}

		c.Next()
	}
}

// validateRequest validates the incoming request against the OpenAPI spec.
func (v *OpenAPIValidator) validateRequest(c *gin.Context) error {
	v.mu.RLock()
	router := v.router
	v.mu.RUnlock()

	route, pathParams, err := router.FindRoute(c.Request)
	if err != nil {
		v.logger.Debug("route not found in OpenAPI spec",
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Error(err),
		)
		c.Next()
		return nil
	}

	requestValidationInput := &openapi3filter.RequestValidationInput{
		Request:    c.Request,
		PathParams: pathParams,
		Route:      route,
		Options: &openapi3filter.Options{
			MultiError:         true,
			AuthenticationFunc: openapi3filter.NoopAuthenticationFunc,
		},
	}

	if c.Request.Body != nil && c.Request.ContentLength > 0 {
		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			v.logger.Error("failed to read request body", zap.Error(err))
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error":   "InternalError",
				"message": "Failed to read request body",
				"code":    http.StatusInternalServerError,
			})
			return err
		}
		c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		requestValidationInput.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}

	if err := openapi3filter.ValidateRequest(c.Request.Context(), requestValidationInput); err != nil {
		v.logger.Info("request validation failed",
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Error(err),
		)

		errorMessage := formatValidationError(err)
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"error":   "ValidationError",
			"message": errorMessage,
			"code":    http.StatusBadRequest,
		})
		return err
	}

	c.Next()
	return nil
}

// responseRecorder captures the response for validation.
type responseRecorder struct {
	gin.ResponseWriter
	body       *bytes.Buffer
	statusCode int
}

// Write captures the response body.
func (r *responseRecorder) Write(b []byte) (int, error) {
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}

// WriteHeader captures the status code.
func (r *responseRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

// validateResponseWithCapture validates the response against the OpenAPI spec.
func (v *OpenAPIValidator) validateResponseWithCapture(c *gin.Context) {
	v.mu.RLock()
	router := v.router
	v.mu.RUnlock()

	recorder := &responseRecorder{
		ResponseWriter: c.Writer,
		body:           &bytes.Buffer{},
		statusCode:     http.StatusOK,
	}
	c.Writer = recorder

	c.Next()

	route, pathParams, err := router.FindRoute(c.Request)
	if err != nil {
		return
	}

	responseValidationInput := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{
			Request:    c.Request,
			PathParams: pathParams,
			Route:      route,
		},
		Status: recorder.statusCode,
		Header: c.Writer.Header(),
		Body:   io.NopCloser(bytes.NewBuffer(recorder.body.Bytes())),
		Options: &openapi3filter.Options{
			MultiError: true,
		},
	}

	if err := openapi3filter.ValidateResponse(c.Request.Context(), responseValidationInput); err != nil {
		v.logger.Warn("response validation failed",
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", recorder.statusCode),
			zap.Error(err),
		)
	}
}

// formatValidationError formats validation errors for the API response.
func formatValidationError(err error) string {
	if err == nil {
		return ""
	}

	errStr := err.Error()

	if strings.Contains(errStr, "request body has an error") {
		if strings.Contains(errStr, "doesn't match schema") {
			return "Request body validation failed: " + extractSchemaError(errStr)
		}
		return "Invalid request body format"
	}

	if strings.Contains(errStr, "parameter") {
		return "Invalid request parameters: " + errStr
	}

	return "Request validation failed: " + errStr
}

// extractSchemaError extracts a human-readable error from schema validation.
func extractSchemaError(errStr string) string {
	if strings.Contains(errStr, "property") {
		parts := strings.Split(errStr, "property")
		if len(parts) > 1 {
			propertyPart := strings.TrimSpace(parts[1])
			if idx := strings.Index(propertyPart, " "); idx > 0 {
				return "invalid property " + propertyPart[:idx]
			}
		}
	}

	if strings.Contains(errStr, "missing") {
		return "missing required field"
	}

	if strings.Contains(errStr, "type") {
		return "invalid field type"
	}

	return "schema validation failed"
}

// ValidationError represents an OpenAPI validation error.
type ValidationError struct {
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
	Value   string `json:"value,omitempty"`
}

// ValidationErrors is a collection of validation errors.
type ValidationErrors struct {
	Errors []ValidationError `json:"errors"`
}
