package adapter_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStartSpan tests creating a span with adapter name and operation.
func TestStartSpan(t *testing.T) {
	ctx := context.Background()

	newCtx, span := adapter.StartSpan(ctx, "kubernetes", "ListResources")
	defer span.End()

	assert.NotNil(t, newCtx)
	assert.NotNil(t, span)
	// The returned context should be different from the input (it has the span).
	assert.NotEqual(t, ctx, newCtx)
}

// TestRecordError tests recording an error on a span.
func TestRecordError(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "non-nil error",
			err:  errors.New("connection failed"),
		},
		{
			name: "nil error does nothing",
			err:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, span := adapter.StartSpan(context.Background(), "test", "op")
			defer span.End()

			// Should not panic.
			adapter.RecordError(span, tt.err)
		})
	}
}

// TestRecordSuccess tests recording success on a span.
func TestRecordSuccess(t *testing.T) {
	tests := []struct {
		name  string
		count int
	}{
		{
			name:  "zero results",
			count: 0,
		},
		{
			name:  "multiple results",
			count: 42,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, span := adapter.StartSpan(context.Background(), "test", "op")
			defer span.End()

			// Should not panic.
			adapter.RecordSuccess(span, tt.count)
		})
	}
}

// TestAddAttributes tests adding various attribute types to a span.
func TestAddAttributes(t *testing.T) {
	tests := []struct {
		name  string
		attrs map[string]interface{}
	}{
		{
			name: "string attribute",
			attrs: map[string]interface{}{
				"resource.type": "node",
			},
		},
		{
			name: "int attribute",
			attrs: map[string]interface{}{
				"count": 5,
			},
		},
		{
			name: "int64 attribute",
			attrs: map[string]interface{}{
				"bytes": int64(1024),
			},
		},
		{
			name: "bool attribute",
			attrs: map[string]interface{}{
				"enabled": true,
			},
		},
		{
			name: "float64 attribute",
			attrs: map[string]interface{}{
				"ratio": 0.95,
			},
		},
		{
			name: "mixed attributes",
			attrs: map[string]interface{}{
				"name":    "test",
				"count":   10,
				"enabled": true,
				"ratio":   3.14,
				"size":    int64(2048),
			},
		},
		{
			name: "unsupported type ignored",
			attrs: map[string]interface{}{
				"data": []byte{1, 2, 3}, // byte slice not handled
			},
		},
		{
			name:  "empty attributes",
			attrs: map[string]interface{}{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, span := adapter.StartSpan(context.Background(), "test", "op")
			defer span.End()

			// Should not panic.
			adapter.AddAttributes(span, tt.attrs)
		})
	}
}

// TestRecordCacheOperation tests recording cache hit/miss on a span.
func TestRecordCacheOperation(t *testing.T) {
	tests := []struct {
		name string
		hit  bool
	}{
		{
			name: "cache hit",
			hit:  true,
		},
		{
			name: "cache miss",
			hit:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, span := adapter.StartSpan(context.Background(), "test", "op")
			defer span.End()

			// Should not panic.
			adapter.RecordCacheOperation(span, tt.hit)
		})
	}
}

// TestRecordBackendCall tests recording backend API call attributes.
func TestRecordBackendCall(t *testing.T) {
	tests := []struct {
		name       string
		endpoint   string
		method     string
		statusCode int
	}{
		{
			name:       "successful GET",
			endpoint:   "/api/v1/nodes",
			method:     "GET",
			statusCode: 200,
		},
		{
			name:       "not found",
			endpoint:   "/api/v1/pods/missing",
			method:     "GET",
			statusCode: 404,
		},
		{
			name:       "server error",
			endpoint:   "/api/v1/services",
			method:     "POST",
			statusCode: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, span := adapter.StartSpan(context.Background(), "test", "op")
			defer span.End()

			// Should not panic.
			adapter.RecordBackendCall(span, tt.endpoint, tt.method, tt.statusCode)
		})
	}
}

// TestRecordResourceOperation tests recording resource operation attributes.
func TestRecordResourceOperation(t *testing.T) {
	tests := []struct {
		name          string
		resourceType  string
		operationType string
		resourceID    string
	}{
		{
			name:          "get node",
			resourceType:  "node",
			operationType: "get",
			resourceID:    "node-123",
		},
		{
			name:          "list pods",
			resourceType:  "pod",
			operationType: "list",
			resourceID:    "",
		},
		{
			name:          "delete resource",
			resourceType:  "ec2-instance",
			operationType: "delete",
			resourceID:    "i-abc123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, span := adapter.StartSpan(context.Background(), "test", "op")
			defer span.End()

			// Should not panic.
			adapter.RecordResourceOperation(span, tt.resourceType, tt.operationType, tt.resourceID)
		})
	}
}

// TestObserveOperationWithTracing tests combined metrics and tracing.
func TestObserveOperationWithTracing(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "success with tracing",
			err:  nil,
		},
		{
			name: "error with tracing",
			err:  errors.New("operation failed"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter.Metrics.OperationTotal.Reset()
			adapter.Metrics.OperationDuration.Reset()

			_, span := adapter.StartSpan(context.Background(), "kubernetes", "TracedOp")
			start := time.Now().Add(-50 * time.Millisecond)

			// Should not panic.
			adapter.ObserveOperationWithTracing("kubernetes", "TracedOp", span, start, tt.err)
		})
	}

	t.Run("nil span does not panic", func(t *testing.T) {
		adapter.Metrics.OperationTotal.Reset()
		adapter.Metrics.OperationDuration.Reset()

		start := time.Now().Add(-10 * time.Millisecond)
		adapter.ObserveOperationWithTracing("kubernetes", "NilSpanOp", nil, start, nil)
	})
}

// TestStartOperation tests the convenience function that combines span and metrics.
func TestStartOperation(t *testing.T) {
	t.Run("success path", func(t *testing.T) {
		adapter.Metrics.OperationTotal.Reset()
		adapter.Metrics.OperationDuration.Reset()

		ctx := context.Background()
		newCtx, end := adapter.StartOperation(ctx, "kubernetes", "ListResources")

		assert.NotNil(t, newCtx)
		assert.NotNil(t, end)

		// Call end function with nil error (success).
		end(nil)
	})

	t.Run("error path", func(t *testing.T) {
		adapter.Metrics.OperationTotal.Reset()
		adapter.Metrics.OperationDuration.Reset()

		ctx := context.Background()
		_, end := adapter.StartOperation(ctx, "kubernetes", "GetResource")

		// Call end function with error.
		end(errors.New("not found"))
	})
}

// TestApplyAdvancedPagination tests cursor-based pagination.
func TestApplyAdvancedPagination(t *testing.T) {
	items := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}

	t.Run("nil pagination returns all items", func(t *testing.T) {
		result, nextCursor, err := adapter.ApplyAdvancedPagination(items, nil)
		require.NoError(t, err)
		assert.Equal(t, items, result)
		assert.Empty(t, nextCursor)
	})

	t.Run("first page with limit", func(t *testing.T) {
		pagination := &models.CursorPagination{
			Cursor: "",
			Limit:  3,
		}
		result, nextCursor, err := adapter.ApplyAdvancedPagination(items, pagination)
		require.NoError(t, err)
		assert.Equal(t, []string{"a", "b", "c"}, result)
		assert.NotEmpty(t, nextCursor, "should have next cursor for more results")
	})

	t.Run("second page using cursor from first page", func(t *testing.T) {
		// First page
		pagination := &models.CursorPagination{
			Cursor: "",
			Limit:  3,
		}
		_, nextCursor, err := adapter.ApplyAdvancedPagination(items, pagination)
		require.NoError(t, err)
		require.NotEmpty(t, nextCursor)

		// Second page
		pagination2 := &models.CursorPagination{
			Cursor: nextCursor,
			Limit:  3,
		}
		result2, nextCursor2, err := adapter.ApplyAdvancedPagination(items, pagination2)
		require.NoError(t, err)
		assert.Equal(t, []string{"d", "e", "f"}, result2)
		assert.NotEmpty(t, nextCursor2)
	})

	t.Run("last page has no next cursor", func(t *testing.T) {
		pagination := &models.CursorPagination{
			Cursor: "",
			Limit:  10,
		}
		result, nextCursor, err := adapter.ApplyAdvancedPagination(items, pagination)
		require.NoError(t, err)
		assert.Equal(t, items, result)
		assert.Empty(t, nextCursor, "no next cursor when all items fit in one page")
	})

	t.Run("invalid cursor returns error", func(t *testing.T) {
		pagination := &models.CursorPagination{
			Cursor: "not-valid-base64!@#",
			Limit:  5,
		}
		_, _, err := adapter.ApplyAdvancedPagination(items, pagination)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decode cursor")
	})

	t.Run("limit larger than remaining items", func(t *testing.T) {
		pagination := &models.CursorPagination{
			Cursor: "",
			Limit:  100,
		}
		result, nextCursor, err := adapter.ApplyAdvancedPagination(items, pagination)
		require.NoError(t, err)
		assert.Equal(t, items, result)
		assert.Empty(t, nextCursor)
	})

	t.Run("empty items", func(t *testing.T) {
		pagination := &models.CursorPagination{
			Cursor: "",
			Limit:  5,
		}
		result, nextCursor, err := adapter.ApplyAdvancedPagination([]string{}, pagination)
		require.NoError(t, err)
		assert.Equal(t, []string{}, result)
		assert.Empty(t, nextCursor)
	})
}

// TestApplyPagination_NegativeValues tests pagination with negative limit and offset.
func TestApplyPagination_NegativeValues(t *testing.T) {
	items := []string{"a", "b", "c", "d", "e"}

	tests := []struct {
		name   string
		limit  int
		offset int
		want   []string
	}{
		{
			name:   "negative offset treated as 0",
			limit:  3,
			offset: -5,
			want:   []string{"a", "b", "c"},
		},
		{
			name:   "negative limit treated as 0 (no limit)",
			limit:  -1,
			offset: 0,
			want:   items,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adapter.ApplyPagination(items, tt.limit, tt.offset)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestMatchesFilter_AdvancedFilter_MissingField tests advanced filter when field doesn't exist.
func TestMatchesFilter_AdvancedFilter_MissingField(t *testing.T) {
	filter := &adapter.Filter{
		AdvancedFilter: &models.AdvancedFilter{
			Conditions: []models.FilterCondition{
				{
					Field:    "nonexistent.field",
					Operator: models.OpEquals,
					Value:    "value",
				},
			},
		},
	}

	result := adapter.MatchesFilter(filter, "pool-1", "type-1", "us-east-1", nil)
	assert.False(t, result, "should not match when field doesn't exist")
}

// TestMatchesFilter_AdvancedFilter_Labels tests advanced filtering with label fields.
func TestMatchesFilter_AdvancedFilter_Labels(t *testing.T) {
	filter := &adapter.Filter{
		AdvancedFilter: &models.AdvancedFilter{
			Conditions: []models.FilterCondition{
				{
					Field:    "labels.env",
					Operator: models.OpEquals,
					Value:    "production",
				},
			},
		},
	}

	labels := map[string]string{"env": "production"}
	result := adapter.MatchesFilter(filter, "", "", "", labels)
	assert.True(t, result)
}

// TestMatchesFilter_AdvancedFilter_ConditionFails tests advanced filtering with non-matching condition.
func TestMatchesFilter_AdvancedFilter_ConditionFails(t *testing.T) {
	filter := &adapter.Filter{
		AdvancedFilter: &models.AdvancedFilter{
			Conditions: []models.FilterCondition{
				{
					Field:    "resourcePoolId",
					Operator: models.OpEquals,
					Value:    "pool-1",
				},
				{
					Field:    "location",
					Operator: models.OpEquals,
					Value:    "us-west-2",
				},
			},
		},
	}

	// First condition matches but second doesn't
	result := adapter.MatchesFilter(filter, "pool-1", "", "us-east-1", nil)
	assert.False(t, result)
}
