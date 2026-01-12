// Package adapter provides tracing instrumentation for adapter implementations.
package adapter

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	// TracerName is the name of the tracer for adapter operations.
	TracerName = "github.com/piwi3910/netweave/internal/adapter"
)

// Span wraps an OpenTelemetry span to provide a concrete type.
// This satisfies the ireturn linter while maintaining OpenTelemetry semantics.
type Span struct {
	trace.Span
}

// StartSpan starts a new span for an adapter operation.
// It returns a new context with the span and the span itself.
// The caller should defer span.End() to ensure the span is properly closed.
//
// Example usage:
//
//	ctx, span := adapter.StartSpan(ctx, "kubernetes", "ListResources")
//	defer span.End()
//
//	resources, err := a.listResourcesFromBackend(ctx)
//	if err != nil {
//	    adapter.RecordError(span, err)
//	    return nil, err
//	}
//	adapter.RecordSuccess(span, len(resources))
func StartSpan(ctx context.Context, adapterName, operation string) (context.Context, Span) {
	tracer := otel.Tracer(TracerName)
	ctx, span := tracer.Start(ctx, operation,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("adapter.name", adapterName),
			attribute.String("adapter.operation", operation),
		),
	)
	return ctx, Span{Span: span}
}

// RecordError records an error in the span and sets the span status to error.
//
// Example usage:
//
//	if err != nil {
//	    adapter.RecordError(span, err)
//	    return nil, err
//	}
func RecordError(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}

// RecordSuccess marks the span as successful and optionally records result metrics.
//
// Example usage:
//
//	adapter.RecordSuccess(span, len(resources))
func RecordSuccess(span trace.Span, count int) {
	span.SetStatus(codes.Ok, "operation completed successfully")
	span.SetAttributes(attribute.Int("result.count", count))
}

// AddAttributes adds custom attributes to the span.
//
// Example usage:
//
//	adapter.AddAttributes(span, map[string]interface{}{
//	    "resource.type": "node",
//	    "resource.pool": "default",
//	})
func AddAttributes(span trace.Span, attrs map[string]interface{}) {
	for key, value := range attrs {
		switch v := value.(type) {
		case string:
			span.SetAttributes(attribute.String(key, v))
		case int:
			span.SetAttributes(attribute.Int(key, v))
		case int64:
			span.SetAttributes(attribute.Int64(key, v))
		case bool:
			span.SetAttributes(attribute.Bool(key, v))
		case float64:
			span.SetAttributes(attribute.Float64(key, v))
		}
	}
}

// RecordCacheOperation records attributes for a cache operation.
//
// Example usage:
//
//	adapter.RecordCacheOperation(span, true)
func RecordCacheOperation(span trace.Span, hit bool) {
	span.SetAttributes(
		attribute.Bool("cache.hit", hit),
		attribute.Bool("cache.miss", !hit),
	)
}

// RecordBackendCall records attributes for a backend API call.
//
// Example usage:
//
//	adapter.RecordBackendCall(span, "/api/v1/nodes", "GET", 200)
func RecordBackendCall(span trace.Span, endpoint, method string, statusCode int) {
	span.SetAttributes(
		attribute.String("backend.endpoint", endpoint),
		attribute.String("backend.method", method),
		attribute.Int("backend.status_code", statusCode),
	)
}

// RecordResourceOperation records attributes for a resource operation.
//
// Example usage:
//
//	adapter.RecordResourceOperation(span, "node", "get", "node-123")
func RecordResourceOperation(span trace.Span, resourceType, operationType, resourceID string) {
	span.SetAttributes(
		attribute.String("resource.type", resourceType),
		attribute.String("resource.operation", operationType),
		attribute.String("resource.id", resourceID),
	)
}
