package middleware_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ExampleNewOpenAPIValidator demonstrates creating a new OpenAPI validator.
func ExampleNewOpenAPIValidator() {
	// Create with default configuration
	validator, err := NewOpenAPIValidator(nil)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Validator created: %v\n", validator != nil)

	// Create with custom configuration
	cfg := &ValidationConfig{
		ValidateRequest:  true,
		ValidateResponse: false,           // Only enable in development
		MaxBodySize:      2 * 1024 * 1024, // 2MB
		ExcludePaths:     []string{"/health", "/metrics"},
	}
	validator, err = NewOpenAPIValidator(cfg)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Custom validator created: %v\n", validator != nil)
	// Output:
	// Validator created: true
	// Custom validator created: true
}

// ExampleOpenAPIValidator_LoadSpec demonstrates loading an OpenAPI spec from bytes.
func ExampleOpenAPIValidator_LoadSpec() {
	validator, _ := NewOpenAPIValidator(nil)

	// Load OpenAPI spec from bytes (typically embedded or read from file)
	specContent := []byte(`
openapi: 3.0.3
info:
  title: Example API
  version: 1.0.0
paths:
  /items:
    get:
      responses:
        '200':
          description: OK
`)
	err := validator.LoadSpec(specContent)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Spec loaded: %s\n", validator.Spec().Info.Title)
	// Output:
	// Spec loaded: Example API
}

// ExampleDefaultValidationConfig demonstrates the default configuration values.
func ExampleDefaultValidationConfig() {
	cfg := DefaultValidationConfig()
	fmt.Printf("ValidateRequest: %v\n", cfg.ValidateRequest)
	fmt.Printf("ValidateResponse: %v\n", cfg.ValidateResponse)
	fmt.Printf("MaxBodySize: %d bytes\n", cfg.MaxBodySize)
	// Output:
	// ValidateRequest: true
	// ValidateResponse: false
	// MaxBodySize: 1048576 bytes
}

// testOpenAPISpec is a minimal OpenAPI spec for testing.
const testOpenAPISpec = `
openapi: 3.0.3
info:
  title: Test API
  version: 1.0.0
servers:
  - url: /o2ims/v1
paths:
  /subscriptions:
    get:
      operationId: listSubscriptions
      responses:
        '200':
          description: OK
          content:
            application/json:
              schema:
                type: object
    post:
      operationId: createSubscription
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/SubscriptionCreateRequest'
      responses:
        '201':
          description: Created
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Subscription'
        '400':
          description: Bad Request
  /subscriptions/{subscriptionId}:
    get:
      operationId: getSubscription
      parameters:
        - name: subscriptionId
          in: path
          required: true
          schema:
            type: string
            minLength: 1
      responses:
        '200':
          description: OK
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Subscription'
        '404':
          description: Not Found
components:
  schemas:
    Subscription:
      type: object
      required:
        - subscriptionId
        - callback
      properties:
        subscriptionId:
          type: string
        callback:
          type: string
          format: uri
    SubscriptionCreateRequest:
      type: object
      required:
        - callback
      properties:
        callback:
          type: string
          format: uri
          minLength: 1
        consumerSubscriptionId:
          type: string
`

func setupTestRouter(t *testing.T, cfg *ValidationConfig) *gin.Engine {
	t.Helper()

	gin.SetMode(gin.TestMode)
	router := gin.New()

	if cfg == nil {
		cfg = DefaultValidationConfig()
	}

	logger := zap.NewNop()
	cfg.Logger = logger

	validator, err := NewOpenAPIValidator(cfg)
	require.NoError(t, err)

	err = validator.LoadSpec([]byte(testOpenAPISpec))
	require.NoError(t, err)

	router.Use(validator.Middleware())

	return router
}

func TestNewOpenAPIValidator(t *testing.T) {
	t.Run("creates validator with default config", func(t *testing.T) {
		validator, err := NewOpenAPIValidator(nil)
		require.NoError(t, err)
		assert.NotNil(t, validator)
	})

	t.Run("creates validator with custom config", func(t *testing.T) {
		cfg := &ValidationConfig{
			ValidateRequest:  true,
			ValidateResponse: false,
			ExcludePaths:     []string{"/health"},
		}

		validator, err := NewOpenAPIValidator(cfg)
		require.NoError(t, err)
		assert.NotNil(t, validator)
		assert.True(t, validator.config.ValidateRequest)
		assert.False(t, validator.config.ValidateResponse)
	})
}

func TestOpenAPIValidator_LoadSpec(t *testing.T) {
	t.Run("loads valid spec from content", func(t *testing.T) {
		validator, err := NewOpenAPIValidator(nil)
		require.NoError(t, err)

		err = validator.LoadSpec([]byte(testOpenAPISpec))
		require.NoError(t, err)
		assert.NotNil(t, validator.Spec())
		assert.Equal(t, "Test API", validator.Spec().Info.Title)
	})

	t.Run("fails on invalid spec", func(t *testing.T) {
		validator, err := NewOpenAPIValidator(nil)
		require.NoError(t, err)

		err = validator.LoadSpec([]byte("invalid yaml content"))
		require.Error(t, err)
	})

	t.Run("fails on empty spec", func(t *testing.T) {
		validator, err := NewOpenAPIValidator(nil)
		require.NoError(t, err)

		err = validator.LoadSpec([]byte(""))
		require.Error(t, err)
	})
}

func TestOpenAPIValidator_Middleware(t *testing.T) {
	t.Run("validates valid GET request", func(t *testing.T) {
		router := setupTestRouter(t, nil)

		router.GET("/o2ims/v1/subscriptions", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"subscriptions": []interface{}{}})
		})

		req := httptest.NewRequest(http.MethodGet, "/o2ims/v1/subscriptions", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("validates valid POST request with body", func(t *testing.T) {
		router := setupTestRouter(t, nil)

		router.POST("/o2ims/v1/subscriptions", func(c *gin.Context) {
			c.JSON(http.StatusCreated, gin.H{
				"subscriptionId": "test-123",
				"callback":       "https://example.com/callback",
			})
		})

		body := map[string]interface{}{
			"callback": "https://example.com/callback",
		}
		bodyBytes, err := json.Marshal(body)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/o2ims/v1/subscriptions", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
	})

	t.Run("rejects POST request with missing required field", func(t *testing.T) {
		router := setupTestRouter(t, nil)

		router.POST("/o2ims/v1/subscriptions", func(c *gin.Context) {
			c.JSON(http.StatusCreated, gin.H{"subscriptionId": "test-123"})
		})

		body := map[string]interface{}{
			"consumerSubscriptionId": "test",
		}
		bodyBytes, err := json.Marshal(body)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/o2ims/v1/subscriptions", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]interface{}
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "ValidationError", response["error"])
	})

	t.Run("rejects POST request with invalid JSON", func(t *testing.T) {
		router := setupTestRouter(t, nil)

		router.POST("/o2ims/v1/subscriptions", func(c *gin.Context) {
			c.JSON(http.StatusCreated, gin.H{"subscriptionId": "test-123"})
		})

		req := httptest.NewRequest(http.MethodPost, "/o2ims/v1/subscriptions", bytes.NewReader([]byte("invalid json")))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("allows request to excluded paths", func(t *testing.T) {
		cfg := DefaultValidationConfig()
		cfg.ExcludePaths = []string{"/health", "/metrics"}
		router := setupTestRouter(t, cfg)

		router.GET("/health", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "healthy"})
		})

		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("allows request to paths not in spec", func(t *testing.T) {
		router := setupTestRouter(t, nil)

		router.GET("/unknown/path", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"message": "ok"})
		})

		req := httptest.NewRequest(http.MethodGet, "/unknown/path", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("validates path parameters", func(t *testing.T) {
		router := setupTestRouter(t, nil)

		router.GET("/o2ims/v1/subscriptions/:subscriptionId", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"subscriptionId": c.Param("subscriptionId"),
				"callback":       "https://example.com/callback",
			})
		})

		req := httptest.NewRequest(http.MethodGet, "/o2ims/v1/subscriptions/test-123", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestOpenAPIValidator_DisabledValidation(t *testing.T) {
	t.Run("skips validation when disabled", func(t *testing.T) {
		cfg := &ValidationConfig{
			ValidateRequest:  false,
			ValidateResponse: false,
		}
		router := setupTestRouter(t, cfg)

		router.POST("/o2ims/v1/subscriptions", func(c *gin.Context) {
			c.JSON(http.StatusCreated, gin.H{"subscriptionId": "test-123"})
		})

		body := map[string]interface{}{}
		bodyBytes, err := json.Marshal(body)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/o2ims/v1/subscriptions", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
	})
}

func TestOpenAPIValidator_NoSpec(t *testing.T) {
	t.Run("skips validation when spec not loaded", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		router := gin.New()

		validator, err := NewOpenAPIValidator(nil)
		require.NoError(t, err)

		router.Use(validator.Middleware())
		router.GET("/test", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"message": "ok"})
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestDefaultValidationConfig(t *testing.T) {
	cfg := DefaultValidationConfig()

	assert.True(t, cfg.ValidateRequest)
	assert.False(t, cfg.ValidateResponse)
	assert.Equal(t, DefaultMaxBodySize, cfg.MaxBodySize)
	assert.Contains(t, cfg.ExcludePaths, "/health")
	assert.Contains(t, cfg.ExcludePaths, "/metrics")
}

func TestFormatValidationError(t *testing.T) {
	tests := []struct {
		name     string
		errStr   string
		expected string
	}{
		{
			name:     "nil error",
			errStr:   "",
			expected: "",
		},
		{
			name:     "body error with schema",
			errStr:   "request body has an error: doesn't match schema",
			expected: "Request body validation failed: schema validation failed",
		},
		{
			name:     "body error without schema",
			errStr:   "request body has an error",
			expected: "Invalid request body format",
		},
		{
			name:     "parameter error",
			errStr:   "parameter 'id' is invalid",
			expected: "Invalid request parameters: parameter 'id' is invalid",
		},
		{
			name:     "generic error",
			errStr:   "something went wrong",
			expected: "Request validation failed: something went wrong",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			if tt.errStr != "" {
				err = &mockError{msg: tt.errStr}
			}
			result := formatValidationError(err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

type mockError struct {
	msg string
}

func (e *mockError) Error() string {
	return e.msg
}

func TestOpenAPIValidator_IsExcludedPath(t *testing.T) {
	cfg := &ValidationConfig{
		ExcludePaths: []string{"/health", "/metrics", "/api/v1/internal"},
	}
	validator, err := NewOpenAPIValidator(cfg)
	require.NoError(t, err)

	tests := []struct {
		path     string
		excluded bool
	}{
		{"/health", true},
		{"/healthz", false},
		{"/health/live", true},
		{"/metrics", true},
		{"/metrics/prometheus", true},
		{"/api/v1/internal", true},
		{"/api/v1/internal/status", true},
		{"/api/v1/public", false},
		{"/o2ims/v1/subscriptions", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := validator.isExcludedPath(tt.path)
			assert.Equal(t, tt.excluded, result)
		})
	}
}

func TestOpenAPIValidator_ConcurrentAccess(t *testing.T) {
	router := setupTestRouter(t, nil)

	router.GET("/o2ims/v1/subscriptions", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"subscriptions": []interface{}{}})
	})

	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				req := httptest.NewRequest(http.MethodGet, "/o2ims/v1/subscriptions", nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				assert.Equal(t, http.StatusOK, w.Code)
			}
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestOpenAPIValidator_MaxBodySize(t *testing.T) {
	t.Run("rejects request exceeding max body size", func(t *testing.T) {
		cfg := &ValidationConfig{
			ValidateRequest: true,
			MaxBodySize:     100, // 100 bytes limit
		}
		router := setupTestRouter(t, cfg)

		router.POST("/o2ims/v1/subscriptions", func(c *gin.Context) {
			c.JSON(http.StatusCreated, gin.H{"subscriptionId": "test-123"})
		})

		// Create a body larger than 100 bytes
		largeBody := make([]byte, 200)
		for i := range largeBody {
			largeBody[i] = 'a'
		}

		req := httptest.NewRequest(http.MethodPost, "/o2ims/v1/subscriptions", bytes.NewReader(largeBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "RequestEntityTooLarge", response["error"])
	})

	t.Run("accepts request within max body size", func(t *testing.T) {
		cfg := &ValidationConfig{
			ValidateRequest: true,
			MaxBodySize:     1024, // 1KB limit
		}
		router := setupTestRouter(t, cfg)

		router.POST("/o2ims/v1/subscriptions", func(c *gin.Context) {
			c.JSON(http.StatusCreated, gin.H{
				"subscriptionId": "test-123",
				"callback":       "https://example.com/callback",
			})
		})

		body := map[string]interface{}{
			"callback": "https://example.com/callback",
		}
		bodyBytes, err := json.Marshal(body)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/o2ims/v1/subscriptions", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
	})

	t.Run("uses default max body size when not configured", func(t *testing.T) {
		cfg := DefaultValidationConfig()
		assert.Equal(t, DefaultMaxBodySize, cfg.MaxBodySize)
	})
}

func TestOpenAPIValidator_LoadSpecFromFile(t *testing.T) {
	t.Run("fails on non-existent file", func(t *testing.T) {
		validator, err := NewOpenAPIValidator(nil)
		require.NoError(t, err)

		err = validator.LoadSpecFromFile("/non/existent/path.yaml")
		require.Error(t, err)
	})

	t.Run("loads valid spec from file", func(t *testing.T) {
		// Create a temp file with the test spec
		tmpFile, err := os.CreateTemp("", "openapi-*.yaml")
		require.NoError(t, err)
		defer func() { _ = os.Remove(tmpFile.Name()) }()

		_, err = tmpFile.WriteString(testOpenAPISpec)
		require.NoError(t, err)
		err = tmpFile.Close()
		require.NoError(t, err)

		validator, err := NewOpenAPIValidator(nil)
		require.NoError(t, err)

		err = validator.LoadSpecFromFile(tmpFile.Name())
		require.NoError(t, err)
		assert.NotNil(t, validator.Spec())
		assert.Equal(t, "Test API", validator.Spec().Info.Title)
	})
}

func TestOpenAPIValidator_CorruptedSpec(t *testing.T) {
	t.Run("fails on corrupted YAML spec", func(t *testing.T) {
		validator, err := NewOpenAPIValidator(nil)
		require.NoError(t, err)

		corruptedSpec := []byte(`
openapi: 3.0.3
info:
  title: Test API
  version: 1.0.0
paths:
  /test:
    get:
      [invalid yaml structure
`)
		err = validator.LoadSpec(corruptedSpec)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse OpenAPI spec")
	})

	t.Run("fails on semantically invalid spec", func(t *testing.T) {
		validator, err := NewOpenAPIValidator(nil)
		require.NoError(t, err)

		// Missing required info section
		invalidSpec := []byte(`
openapi: 3.0.3
paths:
  /test:
    get:
      responses:
        '200':
          description: OK
`)
		err = validator.LoadSpec(invalidSpec)
		require.Error(t, err)
	})
}

func TestOpenAPIValidator_ResponseValidation(t *testing.T) {
	t.Run("validates response when enabled", func(t *testing.T) {
		cfg := &ValidationConfig{
			ValidateRequest:  true,
			ValidateResponse: true,
		}
		router := setupTestRouter(t, cfg)

		// Handler returns a valid response
		router.GET("/o2ims/v1/subscriptions/:subscriptionId", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"subscriptionId": c.Param("subscriptionId"),
				"callback":       "https://example.com/callback",
			})
		})

		req := httptest.NewRequest(http.MethodGet, "/o2ims/v1/subscriptions/test-123", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// Response validation doesn't block the response, just logs warnings
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("logs warning for invalid response schema", func(t *testing.T) {
		cfg := &ValidationConfig{
			ValidateRequest:  true,
			ValidateResponse: true,
		}
		router := setupTestRouter(t, cfg)

		// Handler returns an invalid response (missing required callback field)
		router.GET("/o2ims/v1/subscriptions/:subscriptionId", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"subscriptionId": c.Param("subscriptionId"),
				// Missing required "callback" field
			})
		})

		req := httptest.NewRequest(http.MethodGet, "/o2ims/v1/subscriptions/test-123", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// Response validation doesn't block - it just logs warnings
		// The response is still sent to the client
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("skips response validation when disabled", func(t *testing.T) {
		cfg := &ValidationConfig{
			ValidateRequest:  true,
			ValidateResponse: false, // Disabled
		}
		router := setupTestRouter(t, cfg)

		router.GET("/o2ims/v1/subscriptions/:subscriptionId", func(c *gin.Context) {
			// Return invalid response - should not trigger any validation
			c.JSON(http.StatusOK, gin.H{"invalid": "data"})
		})

		req := httptest.NewRequest(http.MethodGet, "/o2ims/v1/subscriptions/test-123", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// TestResponseRecorder_Write tests the Write method of responseRecorder.

// TestResponseRecorder_Write tests the Write method of responseRecorder.
func TestResponseRecorder_Write(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
	}{
		{
			name:  "write simple data",
			input: []byte("test data"),
		},
		{
			name:  "write empty data",
			input: []byte(""),
		},
		{
			name:  "write JSON data",
			input: []byte(`{"key":"value"}`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			recorder := &responseRecorder{
				ResponseWriter: c.Writer,
				body:           &bytes.Buffer{},
				statusCode:     http.StatusOK,
			}

			n, err := recorder.Write(tt.input)

			require.NoError(t, err)
			assert.Equal(t, len(tt.input), n)
			assert.Equal(t, string(tt.input), recorder.body.String())
		})
	}
}

// TestResponseRecorder_WriteHeader tests the WriteHeader method of responseRecorder.
func TestResponseRecorder_WriteHeader(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{
			name:       "write 200 OK",
			statusCode: http.StatusOK,
		},
		{
			name:       "write 201 Created",
			statusCode: http.StatusCreated,
		},
		{
			name:       "write 400 Bad Request",
			statusCode: http.StatusBadRequest,
		},
		{
			name:       "write 404 Not Found",
			statusCode: http.StatusNotFound,
		},
		{
			name:       "write 500 Internal Server Error",
			statusCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			recorder := &responseRecorder{
				ResponseWriter: c.Writer,
				body:           &bytes.Buffer{},
				statusCode:     http.StatusOK,
			}

			recorder.WriteHeader(tt.statusCode)

			assert.Equal(t, tt.statusCode, recorder.statusCode)
		})
	}
}
