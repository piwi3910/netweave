package adapter_test

import (
	"errors"
	"testing"
	"time"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestObserveOperation(t *testing.T) {
	tests := []struct {
		name          string
		adapterName   string
		operation     string
		duration      time.Duration
		err           error
		expectedCount float64
	}{
		{
			name:          "successful operation",
			adapterName:   "kubernetes",
			operation:     "ListResources",
			duration:      100 * time.Millisecond,
			err:           nil,
			expectedCount: 1,
		},
		{
			name:          "failed operation",
			adapterName:   "kubernetes",
			operation:     "GetResource",
			duration:      50 * time.Millisecond,
			err:           errors.New("resource not found"),
			expectedCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset metrics
			adapter.Metrics.OperationTotal.Reset()
			adapter.Metrics.OperationDuration.Reset()

			start := time.Now().Add(-tt.duration)
			adapter.ObserveOperation(tt.adapterName, tt.operation, start, tt.err)

			// Verify counter incremented
			status := "success"
			if tt.err != nil {
				status = "error"
			}

			count := testutil.ToFloat64(adapter.Metrics.OperationTotal.WithLabelValues(
				tt.adapterName, tt.operation, status,
			))
			assert.Equal(t, tt.expectedCount, count)

			// Note: Cannot easily test histogram sample count with testutil.ToFloat64
			// as WithLabelValues() returns an Observer, not a Collector.
			// The histogram is still being recorded by ObserveOperation().
		})
	}
}

func TestObserveHealthCheck(t *testing.T) {
	tests := []struct {
		name           string
		adapterName    string
		duration       time.Duration
		err            error
		expectedStatus float64
	}{
		{
			name:           "healthy adapter",
			adapterName:    "kubernetes",
			duration:       10 * time.Millisecond,
			err:            nil,
			expectedStatus: 1.0,
		},
		{
			name:           "unhealthy adapter",
			adapterName:    "aws",
			duration:       5 * time.Millisecond,
			err:            errors.New("connection failed"),
			expectedStatus: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset metrics
			adapter.Metrics.HealthCheckStatus.Reset()
			adapter.Metrics.HealthCheckDuration.Reset()

			start := time.Now().Add(-tt.duration)
			adapter.ObserveHealthCheck(tt.adapterName, start, tt.err)

			// Verify status gauge
			status := testutil.ToFloat64(adapter.Metrics.HealthCheckStatus.WithLabelValues(
				tt.adapterName,
			))
			assert.Equal(t, tt.expectedStatus, status)

			// Note: Cannot easily test histogram sample count with testutil.ToFloat64
			// as WithLabelValues() returns an Observer, not a Collector.
			// The histogram is still being recorded by ObserveHealthCheck().
		})
	}
}

func TestUpdateSubscriptionCount(t *testing.T) {
	tests := []struct {
		name        string
		adapterName string
		count       int
	}{
		{
			name:        "zero subscriptions",
			adapterName: "kubernetes",
			count:       0,
		},
		{
			name:        "multiple subscriptions",
			adapterName: "aws",
			count:       10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset metrics
			adapter.Metrics.SubscriptionCount.Reset()

			adapter.UpdateSubscriptionCount(tt.adapterName, tt.count)

			// Verify gauge value
			value := testutil.ToFloat64(adapter.Metrics.SubscriptionCount.WithLabelValues(
				tt.adapterName,
			))
			assert.Equal(t, float64(tt.count), value)
		})
	}
}

func TestRecordCacheHit(t *testing.T) {
	// Reset metrics
	adapter.Metrics.CacheHits.Reset()

	adapter.RecordCacheHit("kubernetes", "ListResources")

	count := testutil.ToFloat64(adapter.Metrics.CacheHits.WithLabelValues(
		"kubernetes", "ListResources",
	))
	assert.Equal(t, 1.0, count)
}

func TestRecordCacheMiss(t *testing.T) {
	// Reset metrics
	adapter.Metrics.CacheMisses.Reset()

	adapter.RecordCacheMiss("kubernetes", "GetResource")

	count := testutil.ToFloat64(adapter.Metrics.CacheMisses.WithLabelValues(
		"kubernetes", "GetResource",
	))
	assert.Equal(t, 1.0, count)
}

func TestUpdateResourceCount(t *testing.T) {
	tests := []struct {
		name         string
		adapterName  string
		resourceType string
		count        int
	}{
		{
			name:         "nodes",
			adapterName:  "kubernetes",
			resourceType: "node",
			count:        5,
		},
		{
			name:         "instances",
			adapterName:  "aws",
			resourceType: "ec2-instance",
			count:        20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset metrics
			adapter.Metrics.ResourcesTotal.Reset()

			adapter.UpdateResourceCount(tt.adapterName, tt.resourceType, tt.count)

			value := testutil.ToFloat64(adapter.Metrics.ResourcesTotal.WithLabelValues(
				tt.adapterName, tt.resourceType,
			))
			assert.Equal(t, float64(tt.count), value)
		})
	}
}

func TestUpdateResourcePoolCount(t *testing.T) {
	tests := []struct {
		name        string
		adapterName string
		count       int
	}{
		{
			name:        "kubernetes pools",
			adapterName: "kubernetes",
			count:       3,
		},
		{
			name:        "aws pools",
			adapterName: "aws",
			count:       10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset metrics
			adapter.Metrics.ResourcePoolsTotal.Reset()

			adapter.UpdateResourcePoolCount(tt.adapterName, tt.count)

			value := testutil.ToFloat64(adapter.Metrics.ResourcePoolsTotal.WithLabelValues(
				tt.adapterName,
			))
			assert.Equal(t, float64(tt.count), value)
		})
	}
}

func TestObserveBackendRequest(t *testing.T) {
	tests := []struct {
		name           string
		adapterName    string
		endpoint       string
		method         string
		statusCode     int
		err            error
		expectedStatus string
		expectError    bool
	}{
		{
			name:           "successful GET request",
			adapterName:    "kubernetes",
			endpoint:       "/api/v1/nodes",
			method:         "GET",
			statusCode:     200,
			err:            nil,
			expectedStatus: "success",
			expectError:    false,
		},
		{
			name:           "client error",
			adapterName:    "kubernetes",
			endpoint:       "/api/v1/pods",
			method:         "GET",
			statusCode:     404,
			err:            nil,
			expectedStatus: "error",
			expectError:    true,
		},
		{
			name:           "server error",
			adapterName:    "aws",
			endpoint:       "/ec2/instances",
			method:         "LIST",
			statusCode:     503,
			err:            nil,
			expectedStatus: "error",
			expectError:    true,
		},
		{
			name:           "network error",
			adapterName:    "kubernetes",
			endpoint:       "/api/v1/services",
			method:         "GET",
			statusCode:     0,
			err:            errors.New("connection refused"),
			expectedStatus: "error",
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset metrics
			adapter.Metrics.BackendRequestsTotal.Reset()
			adapter.Metrics.BackendLatency.Reset()
			adapter.Metrics.BackendErrors.Reset()

			start := time.Now().Add(-10 * time.Millisecond)
			adapter.ObserveBackendRequest(tt.adapterName, tt.endpoint, tt.method, start, tt.statusCode, tt.err)

			// Verify request counter
			count := testutil.ToFloat64(adapter.Metrics.BackendRequestsTotal.WithLabelValues(
				tt.adapterName, tt.endpoint, tt.method, tt.expectedStatus,
			))
			assert.Equal(t, 1.0, count)

			// Note: Cannot easily test histogram sample count with testutil.ToFloat64
			// as WithLabelValues() returns an Observer, not a Collector.
			// The histogram is still being recorded by ObserveBackendRequest().

			// Verify error counter if expected
			if tt.expectError {
				// Collect all error types for this adapter/endpoint/method
				errorMetric := adapter.Metrics.BackendErrors.MustCurryWith(prometheus.Labels{
					"adapter":  tt.adapterName,
					"endpoint": tt.endpoint,
					"method":   tt.method,
				})
				errorCount := testutil.CollectAndCount(errorMetric)
				assert.Greater(t, errorCount, 0, "expected at least one error type to be recorded")
			}
		})
	}
}

func TestMetricsLabels(t *testing.T) {
	// Test that all metrics have the expected labels

	t.Run("OperationTotal labels", func(t *testing.T) {
		adapter.Metrics.OperationTotal.Reset()
		adapter.Metrics.OperationTotal.WithLabelValues("kubernetes", "ListResources", "success").Inc()

		count := testutil.ToFloat64(adapter.Metrics.OperationTotal.WithLabelValues(
			"kubernetes", "ListResources", "success",
		))
		assert.Equal(t, 1.0, count)
	})

	t.Run("CacheHits labels", func(t *testing.T) {
		adapter.Metrics.CacheHits.Reset()
		adapter.Metrics.CacheHits.WithLabelValues("kubernetes", "GetResource").Inc()

		count := testutil.ToFloat64(adapter.Metrics.CacheHits.WithLabelValues(
			"kubernetes", "GetResource",
		))
		assert.Equal(t, 1.0, count)
	})

	t.Run("ResourcesTotal labels", func(t *testing.T) {
		adapter.Metrics.ResourcesTotal.Reset()
		adapter.Metrics.ResourcesTotal.WithLabelValues("kubernetes", "node").Set(10)

		value := testutil.ToFloat64(adapter.Metrics.ResourcesTotal.WithLabelValues(
			"kubernetes", "node",
		))
		assert.Equal(t, 10.0, value)
	})

	t.Run("BackendRequestsTotal labels", func(t *testing.T) {
		adapter.Metrics.BackendRequestsTotal.Reset()
		adapter.Metrics.BackendRequestsTotal.WithLabelValues(
			"kubernetes", "/api/v1/nodes", "GET", "success",
		).Inc()

		count := testutil.ToFloat64(adapter.Metrics.BackendRequestsTotal.WithLabelValues(
			"kubernetes", "/api/v1/nodes", "GET", "success",
		))
		assert.Equal(t, 1.0, count)
	})
}

func TestMetricsInitialization(t *testing.T) {
	// Verify all metrics are properly initialized

	require.NotNil(t, adapter.Metrics.OperationDuration)
	require.NotNil(t, adapter.Metrics.OperationTotal)
	require.NotNil(t, adapter.Metrics.OperationErrors)
	require.NotNil(t, adapter.Metrics.HealthCheckDuration)
	require.NotNil(t, adapter.Metrics.HealthCheckStatus)
	require.NotNil(t, adapter.Metrics.SubscriptionCount)
	require.NotNil(t, adapter.Metrics.CacheHits)
	require.NotNil(t, adapter.Metrics.CacheMisses)
	require.NotNil(t, adapter.Metrics.ResourcesTotal)
	require.NotNil(t, adapter.Metrics.ResourcePoolsTotal)
	require.NotNil(t, adapter.Metrics.BackendRequestsTotal)
	require.NotNil(t, adapter.Metrics.BackendLatency)
	require.NotNil(t, adapter.Metrics.BackendErrors)
}

func BenchmarkObserveOperation(b *testing.B) {
	start := time.Now()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		adapter.ObserveOperation("kubernetes", "ListResources", start, nil)
	}
}

func BenchmarkRecordCacheHit(b *testing.B) {
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		adapter.RecordCacheHit("kubernetes", "ListResources")
	}
}

func BenchmarkObserveBackendRequest(b *testing.B) {
	start := time.Now()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		adapter.ObserveBackendRequest("kubernetes", "/api/v1/nodes", "GET", start, 200, nil)
	}
}
