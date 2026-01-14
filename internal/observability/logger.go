package observability

import (
	"context"
	"fmt"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger is a wrapper around zap.Logger with additional convenience methods.
type Logger struct {
	*zap.Logger
}

// loggerContextKey is the context key for storing logger instances.
type loggerContextKey struct{}

var (
	// GlobalLogger is the default logger instance. Exported for testing.
	GlobalLogger *Logger
)

// InitLogger initializes the global logger with the specified environment
// Valid environments: development, test, staging, production.
func InitLogger(env string) (*Logger, error) {
	var config zap.Config

	switch env {
	case "development", "test":
		config = zap.NewDevelopmentConfig()
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	case "production", "staging":
		config = zap.NewProductionConfig()
		config.EncoderConfig.TimeKey = "timestamp"
		config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	default:
		return nil, fmt.Errorf("invalid environment: %s (must be development, test, staging, or production)", env)
	}

	// Set log level from environment variable if provided
	if logLevel := os.Getenv("LOG_LEVEL"); logLevel != "" {
		var level zapcore.Level
		if err := level.UnmarshalText([]byte(logLevel)); err != nil {
			return nil, fmt.Errorf("invalid log level: %w", err)
		}
		config.Level = zap.NewAtomicLevelAt(level)
	}

	// Build the logger
	zapLogger, err := config.Build(
		zap.AddCallerSkip(1), // Skip wrapper functions in stack trace
		zap.AddStacktrace(zapcore.ErrorLevel),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build logger: %w", err)
	}

	logger := &Logger{Logger: zapLogger}
	GlobalLogger = logger

	return logger, nil
}

// GetLogger returns the global logger instance
// Panics if InitLogger has not been called.
func GetLogger() *Logger {
	if GlobalLogger == nil {
		panic("logger not initialized - call InitLogger first")
	}
	return GlobalLogger
}

// WithContext creates a new logger with fields from contex.
func (l *Logger) WithContext(ctx context.Context) *Logger {
	// Extract request ID, trace ID, or other contextual information
	fields := ExtractContextFields(ctx)
	if len(fields) > 0 {
		return &Logger{Logger: l.With(fields...)}
	}
	return l
}

// WithFields creates a new logger with additional fields.
func (l *Logger) WithFields(fields ...zap.Field) *Logger {
	return &Logger{Logger: l.With(fields...)}
}

// WithError adds an error field to the logger.
func (l *Logger) WithError(err error) *Logger {
	return &Logger{Logger: l.With(zap.Error(err))}
}

// WithComponent adds a component field to the logger.
func (l *Logger) WithComponent(component string) *Logger {
	return &Logger{Logger: l.With(zap.String("component", component))}
}

// ContextWithLogger adds the logger to the contex.
func ContextWithLogger(ctx context.Context, logger *Logger) context.Context {
	return context.WithValue(ctx, loggerContextKey{}, logger)
}

// LoggerFromContext retrieves the logger from context
// Returns the global logger if not found in contex.
func LoggerFromContext(ctx context.Context) *Logger {
	if logger, ok := ctx.Value(loggerContextKey{}).(*Logger); ok {
		return logger
	}
	return GetLogger()
}

// extractContextFields extracts logging fields from context
// This can be extended to include request ID, trace ID, user ID, etc.
func ExtractContextFields(_ context.Context) []zap.Field {
	var fields []zap.Field

	// Example: Extract request ID if available
	// if requestID := ctx.Value("requestID"); requestID != nil {
	//     fields = append(fields, zap.String("requestID", requestID.(string)))
	// }

	// Example: Extract trace ID from OpenTelemetry context
	// if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
	//     fields = append(fields, zap.String("traceID", span.SpanContext().TraceID().String()))
	//     fields = append(fields, zap.String("spanID", span.SpanContext().SpanID().String()))
	// }

	return fields
}

// Sync flushes any buffered log entries.
// Should be called before application shutdown.
func (l *Logger) Sync() error {
	if err := l.Logger.Sync(); err != nil {
		return fmt.Errorf("failed to sync logger: %w", err)
	}
	return nil
}

// Helper methods for common logging patterns

// LogRequest logs an HTTP reques.
func (l *Logger) LogRequest(method, path string, statusCode int, duration float64) {
	l.Info("http request",
		zap.String("method", method),
		zap.String("path", path),
		zap.Int("status", statusCode),
		zap.Float64("duration_ms", duration),
	)
}

// LogAdapterOperation logs an adapter operation.
func (l *Logger) LogAdapterOperation(operation, adapterType string, resourceID string, err error) {
	if err != nil {
		l.Error("adapter operation failed",
			zap.String("operation", operation),
			zap.String("adapter", adapterType),
			zap.String("resourceID", resourceID),
			zap.Error(err),
		)
	} else {
		l.Info("adapter operation completed",
			zap.String("operation", operation),
			zap.String("adapter", adapterType),
			zap.String("resourceID", resourceID),
		)
	}
}

// LogSubscriptionEvent logs a subscription-related even.
func (l *Logger) LogSubscriptionEvent(eventType, subscriptionID string, details map[string]interface{}) {
	fields := []zap.Field{
		zap.String("event", eventType),
		zap.String("subscriptionID", subscriptionID),
	}

	// Add additional details as fields
	for key, value := range details {
		fields = append(fields, zap.Any(key, value))
	}

	l.Info("subscription event", fields...)
}

// LogRedisOperation logs a Redis operation.
func (l *Logger) LogRedisOperation(operation string, key string, err error) {
	if err != nil {
		l.Error("redis operation failed",
			zap.String("operation", operation),
			zap.String("key", key),
			zap.Error(err),
		)
	} else {
		l.Debug("redis operation completed",
			zap.String("operation", operation),
			zap.String("key", key),
		)
	}
}

// LogKubernetesOperation logs a Kubernetes API operation.
func (l *Logger) LogKubernetesOperation(operation, resource, namespace, name string, err error) {
	if err != nil {
		l.Error("kubernetes operation failed",
			zap.String("operation", operation),
			zap.String("resource", resource),
			zap.String("namespace", namespace),
			zap.String("name", name),
			zap.Error(err),
		)
	} else {
		l.Debug("kubernetes operation completed",
			zap.String("operation", operation),
			zap.String("resource", resource),
			zap.String("namespace", namespace),
			zap.String("name", name),
		)
	}
}
