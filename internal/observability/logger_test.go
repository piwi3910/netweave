package observability_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestInitLogger(t *testing.T) {
	tests := []struct {
		name    string
		env     string
		wantErr bool
	}{
		{
			name:    "development environment",
			env:     "development",
			wantErr: false,
		},
		{
			name:    "production environment",
			env:     "production",
			wantErr: false,
		},
		{
			name:    "staging environment",
			env:     "staging",
			wantErr: false,
		},
		{
			name:    "invalid environment",
			env:     "invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset global logger
			globalLogger = nil

			logger, err := InitLogger(tt.env)
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, logger)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, logger)
			assert.NotNil(t, logger.Logger)

			// Cleanup
			_ = logger.Sync()
		})
	}
}

func TestInitLoggerWithLogLevel(t *testing.T) {
	// Reset global logger
	globalLogger = nil

	// Set log level via environment variable
	_ = os.Setenv("LOG_LEVEL", "warn")
	defer func() { _ = os.Unsetenv("LOG_LEVEL") }()

	logger, err := InitLogger("production")
	require.NoError(t, err)
	require.NotNil(t, logger)

	// Cleanup
	_ = logger.Sync()
}

func TestInitLoggerInvalidLogLevel(t *testing.T) {
	// Reset global logger
	globalLogger = nil

	// Set invalid log level
	_ = os.Setenv("LOG_LEVEL", "invalid")
	defer func() { _ = os.Unsetenv("LOG_LEVEL") }()

	logger, err := InitLogger("production")
	require.Error(t, err)
	assert.Nil(t, logger)
	assert.Contains(t, err.Error(), "invalid log level")
}

func TestGetLogger(t *testing.T) {
	// Reset and initialize global logger
	globalLogger = nil
	logger, err := InitLogger("development")
	require.NoError(t, err)
	defer func() { _ = logger.Sync() }()

	// Get logger
	retrieved := GetLogger()
	require.NotNil(t, retrieved)
	assert.Equal(t, logger, retrieved)
}

func TestGetLoggerPanicsWhenNotInitialized(t *testing.T) {
	// Reset global logger
	globalLogger = nil

	assert.Panics(t, func() {
		GetLogger()
	})
}

func TestLoggerWithContext(t *testing.T) {
	globalLogger = nil
	logger, err := InitLogger("development")
	require.NoError(t, err)
	defer func() { _ = logger.Sync() }()

	ctx := context.Background()
	contextLogger := logger.WithContext(ctx)
	require.NotNil(t, contextLogger)
}

func TestLoggerWithFields(t *testing.T) {
	globalLogger = nil
	logger, err := InitLogger("development")
	require.NoError(t, err)
	defer func() { _ = logger.Sync() }()

	fieldsLogger := logger.WithFields(
		zap.String("key1", "value1"),
		zap.Int("key2", 42),
	)
	require.NotNil(t, fieldsLogger)
	assert.NotEqual(t, logger, fieldsLogger)
}

func TestLoggerWithError(t *testing.T) {
	globalLogger = nil
	logger, err := InitLogger("development")
	require.NoError(t, err)
	defer func() { _ = logger.Sync() }()

	testErr := assert.AnError
	errorLogger := logger.WithError(testErr)
	require.NotNil(t, errorLogger)
}

func TestLoggerWithComponent(t *testing.T) {
	globalLogger = nil
	logger, err := InitLogger("development")
	require.NoError(t, err)
	defer func() { _ = logger.Sync() }()

	componentLogger := logger.WithComponent("test-component")
	require.NotNil(t, componentLogger)
}

func TestContextWithLogger(t *testing.T) {
	globalLogger = nil
	logger, err := InitLogger("development")
	require.NoError(t, err)
	defer func() { _ = logger.Sync() }()

	ctx := context.Background()
	ctxWithLogger := ContextWithLogger(ctx, logger)
	require.NotNil(t, ctxWithLogger)

	// Verify we can retrieve the logger
	retrieved := LoggerFromContext(ctxWithLogger)
	require.NotNil(t, retrieved)
	assert.Equal(t, logger, retrieved)
}

func TestLoggerFromContextFallsBackToGlobal(t *testing.T) {
	globalLogger = nil
	logger, err := InitLogger("development")
	require.NoError(t, err)
	defer func() { _ = logger.Sync() }()

	// Context without logger
	ctx := context.Background()
	retrieved := LoggerFromContext(ctx)
	require.NotNil(t, retrieved)
	assert.Equal(t, logger, retrieved)
}

func TestLogRequest(t *testing.T) {
	globalLogger = nil
	logger, err := InitLogger("development")
	require.NoError(t, err)
	defer func() { _ = logger.Sync() }()

	// This should not panic
	logger.LogRequest("GET", "/api/v1/subscriptions", 200, 15.5)
}

func TestLogAdapterOperation(t *testing.T) {
	globalLogger = nil
	logger, err := InitLogger("development")
	require.NoError(t, err)
	defer func() { _ = logger.Sync() }()

	// Success case
	logger.LogAdapterOperation("GetResourcePool", "k8s", "pool-123", nil)

	// Error case
	logger.LogAdapterOperation("GetResourcePool", "k8s", "pool-456", assert.AnError)
}

func TestLogSubscriptionEvent(t *testing.T) {
	globalLogger = nil
	logger, err := InitLogger("development")
	require.NoError(t, err)
	defer func() { _ = logger.Sync() }()

	details := map[string]interface{}{
		"resourceID": "pool-123",
		"action":     "created",
	}

	logger.LogSubscriptionEvent("resource.created", "sub-123", details)
}

func TestLogRedisOperation(t *testing.T) {
	globalLogger = nil
	logger, err := InitLogger("development")
	require.NoError(t, err)
	defer func() { _ = logger.Sync() }()

	// Success case
	logger.LogRedisOperation("SET", "subscription:123", nil)

	// Error case
	logger.LogRedisOperation("GET", "subscription:456", assert.AnError)
}

func TestLogKubernetesOperation(t *testing.T) {
	globalLogger = nil
	logger, err := InitLogger("development")
	require.NoError(t, err)
	defer func() { _ = logger.Sync() }()

	// Success case
	logger.LogKubernetesOperation("Get", "Node", "default", "node-1", nil)

	// Error case
	logger.LogKubernetesOperation("Get", "Node", "default", "node-2", assert.AnError)
}

func TestLogLevels(t *testing.T) {
	globalLogger = nil
	logger, err := InitLogger("development")
	require.NoError(t, err)
	defer func() { _ = logger.Sync() }()

	// Test all log levels
	logger.Debug("debug message", zap.String("level", "debug"))
	logger.Info("info message", zap.String("level", "info"))
	logger.Warn("warn message", zap.String("level", "warn"))
	logger.Error("error message", zap.String("level", "error"))

	// Should not panic or fail
}

func TestLoggerConfigDevelopment(t *testing.T) {
	globalLogger = nil
	logger, err := InitLogger("development")
	require.NoError(t, err)
	defer func() { _ = logger.Sync() }()

	// Development logger should use console encoding
	assert.NotNil(t, logger)
}

func TestLoggerConfigProduction(t *testing.T) {
	globalLogger = nil
	logger, err := InitLogger("production")
	require.NoError(t, err)
	defer func() { _ = logger.Sync() }()

	// Production logger should use JSON encoding
	assert.NotNil(t, logger)
}

func TestExtractContextFields(t *testing.T) {
	ctx := context.Background()
	fields := extractContextFields(ctx)

	// Currently returns nil or empty array (both are valid), but function should not panic
	// A nil slice is valid and has length 0
	assert.IsType(t, []zap.Field{}, fields)
	assert.Len(t, fields, 0)
}

func TestLoggerSync(t *testing.T) {
	globalLogger = nil
	logger, err := InitLogger("development")
	require.NoError(t, err)

	// Sync should not fail (may return error for stdout/stderr, which is acceptable)
	_ = logger.Sync()
}

// Benchmark tests for performance validation.
func BenchmarkLoggerInfo(b *testing.B) {
	globalLogger = nil
	logger, err := InitLogger("production")
	require.NoError(b, err)
	defer func() { _ = logger.Sync() }()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Info("benchmark test",
			zap.String("key", "value"),
			zap.Int("iteration", i),
		)
	}
}

func BenchmarkLoggerWithFields(b *testing.B) {
	globalLogger = nil
	logger, err := InitLogger("production")
	require.NoError(b, err)
	defer func() { _ = logger.Sync() }()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.WithFields(
			zap.String("key1", "value1"),
			zap.String("key2", "value2"),
			zap.Int("iteration", i),
		)
	}
}

func BenchmarkLogRequest(b *testing.B) {
	globalLogger = nil
	logger, err := InitLogger("production")
	require.NoError(b, err)
	defer func() { _ = logger.Sync() }()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.LogRequest("GET", "/api/v1/test", 200, 10.5)
	}
}
