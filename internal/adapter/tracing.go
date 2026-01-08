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

// StartSpan starts a new span for an adapter operation.
func StartSpan(ctx context.Context, adapterName, operation string) (context.Context, trace.Span) {
	tracer := otel.Tracer(TracerName)
	ctx, span := tracer.Start(ctx, operation,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("adapter.name", adapterName),
			attribute.String("adapter.operation", operation),
		),
	)
	return ctx, span
}

// RecordError records an error in the span.
func RecordError(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}

// RecordSuccess marks the span as successful.
func RecordSuccess(span trace.Span, count int) {
	span.SetStatus(codes.Ok, "operation completed successfully")
	span.SetAttributes(attribute.Int("result.count", count))
}

// AddAttributes adds custom attributes to the span.
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

// RecordCacheOperation records cache operation attributes.
func RecordCacheOperation(span trace.Span, hit bool) {
	span.SetAttributes(
		attribute.Bool("cache.hit", hit),
		attribute.Bool("cache.miss", !hit),
	)
}

// RecordBackendCall records backend API call attributes.
func RecordBackendCall(span trace.Span, endpoint, method string, statusCode int) {
	span.SetAttributes(
		attribute.String("backend.endpoint", endpoint),
		attribute.String("backend.method", method),
		attribute.Int("backend.status_code", statusCode),
	)
}

// RecordResourceOperation records resource operation attributes.
func RecordResourceOperation(span trace.Span, resourceType, operationType, resourceID string) {
	span.SetAttributes(
		attribute.String("resource.type", resourceType),
		attribute.String("resource.operation", operationType),
		attribute.String("resource.id", resourceID),
	)
}
