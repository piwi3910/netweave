package observability_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/piwi3910/netweave/internal/observability"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHealthChecker(t *testing.T) {
	hc := observability.NewHealthChecker("v1.0.0")
	require.NotNil(t, hc)
	assert.Equal(t, "v1.0.0", hc.Version)
	assert.Equal(t, 5*time.Second, hc.Timeout)
	assert.NotNil(t, hc.HealthChecks)
	assert.NotNil(t, hc.ReadinessChecks)
}

func TestRegisterHealthCheck(t *testing.T) {
	hc := observability.NewHealthChecker("v1.0.0")

	checkFunc := func(_ context.Context) error {
		return nil
	}

	hc.RegisterHealthCheck("test-component", checkFunc)

	// Verify check was registered
	assert.Len(t, hc.HealthChecks, 1)
	assert.Contains(t, hc.HealthChecks, "test-component")
}

func TestRegisterReadinessCheck(t *testing.T) {
	hc := observability.NewHealthChecker("v1.0.0")

	checkFunc := func(_ context.Context) error {
		return nil
	}

	hc.RegisterReadinessCheck("test-component", checkFunc)

	// Verify check was registered
	assert.Len(t, hc.ReadinessChecks, 1)
	assert.Contains(t, hc.ReadinessChecks, "test-component")
}

func TestSetTimeout(t *testing.T) {
	hc := observability.NewHealthChecker("v1.0.0")
	assert.Equal(t, 5*time.Second, hc.Timeout)

	hc.SetTimeout(10 * time.Second)
	assert.Equal(t, 10*time.Second, hc.Timeout)
}

func TestCheckHealthAllHealthy(t *testing.T) {
	hc := observability.NewHealthChecker("v1.0.0")

	// Register healthy checks
	hc.RegisterHealthCheck("component1", func(_ context.Context) error {
		return nil
	})
	hc.RegisterHealthCheck("component2", func(_ context.Context) error {
		return nil
	})

	ctx := context.Background()
	response := hc.CheckHealth(ctx)

	require.NotNil(t, response)
	assert.Equal(t, observability.StatusHealthy, response.Status)
	assert.Equal(t, "v1.0.0", response.Version)
	assert.Len(t, response.Components, 2)

	for _, comp := range response.Components {
		assert.Equal(t, observability.StatusHealthy, comp.Status)
		assert.Empty(t, comp.Error)
	}
}

func TestCheckHealthWithUnhealthyComponent(t *testing.T) {
	hc := observability.NewHealthChecker("v1.0.0")

	// Register healthy and unhealthy checks
	hc.RegisterHealthCheck("healthy-component", func(_ context.Context) error {
		return nil
	})
	hc.RegisterHealthCheck("unhealthy-component", func(_ context.Context) error {
		return errors.New("component is down")
	})

	ctx := context.Background()
	response := hc.CheckHealth(ctx)

	require.NotNil(t, response)
	assert.Equal(t, observability.StatusUnhealthy, response.Status)

	healthyComp := response.Components["healthy-component"]
	assert.Equal(t, observability.StatusHealthy, healthyComp.Status)

	unhealthyComp := response.Components["unhealthy-component"]
	assert.Equal(t, observability.StatusUnhealthy, unhealthyComp.Status)
	assert.Contains(t, unhealthyComp.Error, "component is down")
}

func TestCheckHealthTimeout(t *testing.T) {
	hc := observability.NewHealthChecker("v1.0.0")
	hc.SetTimeout(100 * time.Millisecond)

	// Register a check that takes too long
	hc.RegisterHealthCheck("slow-component", func(ctx context.Context) error {
		select {
		case <-time.After(1 * time.Second):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})

	ctx := context.Background()
	response := hc.CheckHealth(ctx)

	require.NotNil(t, response)
	assert.Equal(t, observability.StatusUnhealthy, response.Status)

	slowComp := response.Components["slow-component"]
	assert.Equal(t, observability.StatusUnhealthy, slowComp.Status)
	assert.Equal(t, "check timed out", slowComp.Error)
}

func TestCheckReadinessAllReady(t *testing.T) {
	hc := observability.NewHealthChecker("v1.0.0")

	// Register ready checks
	hc.RegisterReadinessCheck("redis", func(_ context.Context) error {
		return nil
	})
	hc.RegisterReadinessCheck("kubernetes", func(_ context.Context) error {
		return nil
	})

	ctx := context.Background()
	response := hc.CheckReadiness(ctx)

	require.NotNil(t, response)
	assert.True(t, response.Ready)
	assert.Len(t, response.Components, 2)

	for _, comp := range response.Components {
		assert.Equal(t, observability.StatusHealthy, comp.Status)
	}
}

func TestCheckReadinessWithNotReadyComponent(t *testing.T) {
	hc := observability.NewHealthChecker("v1.0.0")

	hc.RegisterReadinessCheck("redis", func(_ context.Context) error {
		return nil
	})
	hc.RegisterReadinessCheck("kubernetes", func(_ context.Context) error {
		return errors.New("k8s not reachable")
	})

	ctx := context.Background()
	response := hc.CheckReadiness(ctx)

	require.NotNil(t, response)
	assert.False(t, response.Ready)

	k8sComp := response.Components["kubernetes"]
	assert.Equal(t, observability.StatusUnhealthy, k8sComp.Status)
	assert.Contains(t, k8sComp.Error, "k8s not reachable")
}

func TestExecuteChecksEmpty(t *testing.T) {
	hc := observability.NewHealthChecker("v1.0.0")
	ctx := context.Background()

	checks := make(map[string]observability.HealthCheck)
	components := hc.ExecuteChecks(ctx, checks)

	assert.NotNil(t, components)
	assert.Len(t, components, 0)
}

func TestExecuteChecksConcurrent(t *testing.T) {
	hc := observability.NewHealthChecker("v1.0.0")
	ctx := context.Background()

	checks := map[string]observability.HealthCheck{
		"check1": func(_ context.Context) error {
			time.Sleep(50 * time.Millisecond)
			return nil
		},
		"check2": func(_ context.Context) error {
			time.Sleep(50 * time.Millisecond)
			return nil
		},
		"check3": func(_ context.Context) error {
			time.Sleep(50 * time.Millisecond)
			return nil
		},
	}

	start := time.Now()
	components := hc.ExecuteChecks(ctx, checks)
	duration := time.Since(start)

	// Should complete in parallel (~50ms), not sequential (~150ms)
	assert.Less(t, duration, 100*time.Millisecond)
	assert.Len(t, components, 3)

	for _, comp := range components {
		assert.Equal(t, observability.StatusHealthy, comp.Status)
	}
}

func TestHealthHandler(t *testing.T) {
	hc := observability.NewHealthChecker("v1.0.0")
	hc.RegisterHealthCheck("test", func(_ context.Context) error {
		return nil
	})

	handler := hc.HealthHandler()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response observability.HealthResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, observability.StatusHealthy, response.Status)
	assert.Equal(t, "v1.0.0", response.Version)
}

func TestHealthHandlerUnhealthy(t *testing.T) {
	hc := observability.NewHealthChecker("v1.0.0")
	hc.RegisterHealthCheck("test", func(_ context.Context) error {
		return errors.New("component failed")
	})

	handler := hc.HealthHandler()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var response observability.HealthResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, observability.StatusUnhealthy, response.Status)
}

func TestReadinessHandler(t *testing.T) {
	hc := observability.NewHealthChecker("v1.0.0")
	hc.RegisterReadinessCheck("test", func(_ context.Context) error {
		return nil
	})

	handler := hc.ReadinessHandler()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response observability.ReadinessResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.True(t, response.Ready)
}

func TestReadinessHandlerNotReady(t *testing.T) {
	hc := observability.NewHealthChecker("v1.0.0")
	hc.RegisterReadinessCheck("test", func(_ context.Context) error {
		return errors.New("not ready")
	})

	handler := hc.ReadinessHandler()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var response observability.ReadinessResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.False(t, response.Ready)
}

func TestLivenessHandler(t *testing.T) {
	handler := observability.LivenessHandler()
	req := httptest.NewRequest(http.MethodGet, "/live", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	alive, ok := response["alive"].(bool)
	require.True(t, ok)
	assert.True(t, alive)

	_, hasTimestamp := response["timestamp"]
	assert.True(t, hasTimestamp)
}

func TestRedisHealthCheck(t *testing.T) {
	// Success case
	pingFunc := func(_ context.Context) error {
		return nil
	}
	check := observability.RedisHealthCheck(pingFunc)
	err := check(context.Background())
	assert.NoError(t, err)

	// Error case
	pingFuncErr := func(_ context.Context) error {
		return errors.New("redis connection failed")
	}
	checkErr := observability.RedisHealthCheck(pingFuncErr)
	err = checkErr(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "redis connection failed")

	// Nil function case
	checkNil := observability.RedisHealthCheck(nil)
	err = checkNil(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "redis ping function not provided")
}

func TestKubernetesHealthCheck(t *testing.T) {
	// Success case
	pingFunc := func(_ context.Context) error {
		return nil
	}
	check := observability.KubernetesHealthCheck(pingFunc)
	err := check(context.Background())
	assert.NoError(t, err)

	// Error case
	pingFuncErr := func(_ context.Context) error {
		return errors.New("k8s api unreachable")
	}
	checkErr := observability.KubernetesHealthCheck(pingFuncErr)
	err = checkErr(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "k8s api unreachable")

	// Nil function case
	checkNil := observability.KubernetesHealthCheck(nil)
	err = checkNil(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "kubernetes ping function not provided")
}

func TestAdapterHealthCheck(t *testing.T) {
	// Success case
	checkFunc := func(_ context.Context) error {
		return nil
	}
	check := observability.AdapterHealthCheck("k8s-adapter", checkFunc)
	err := check(context.Background())
	assert.NoError(t, err)

	// Error case
	checkFuncErr := func(_ context.Context) error {
		return errors.New("adapter error")
	}
	checkErr := observability.AdapterHealthCheck("mock-adapter", checkFuncErr)
	err = checkErr(context.Background())
	assert.Error(t, err)

	// Nil function case
	checkNil := observability.AdapterHealthCheck("test-adapter", nil)
	err = checkNil(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "adapter test-adapter check function not provided")
}

func TestGenericHealthCheck(t *testing.T) {
	checkFunc := func(_ context.Context) error {
		return nil
	}
	check := observability.GenericHealthCheck(checkFunc)
	err := check(context.Background())
	assert.NoError(t, err)

	checkFuncErr := func(_ context.Context) error {
		return errors.New("generic error")
	}
	checkErr := observability.GenericHealthCheck(checkFuncErr)
	err = checkErr(context.Background())
	assert.Error(t, err)
}

func TestHealthStatusConstants(t *testing.T) {
	assert.Equal(t, observability.HealthStatus("healthy"), observability.StatusHealthy)
	assert.Equal(t, observability.HealthStatus("unhealthy"), observability.StatusUnhealthy)
	assert.Equal(t, observability.HealthStatus("degraded"), observability.StatusDegraded)
}

func TestComponentHealthStructure(t *testing.T) {
	comp := observability.ComponentHealth{
		Status:  observability.StatusHealthy,
		Message: "Component is healthy",
		Latency: "10ms",
	}

	assert.Equal(t, observability.StatusHealthy, comp.Status)
	assert.Equal(t, "Component is healthy", comp.Message)
	assert.Equal(t, "10ms", comp.Latency)
	assert.Empty(t, comp.Error)
}

func TestHealthResponseStructure(t *testing.T) {
	now := time.Now()
	response := observability.HealthResponse{
		Status:     observability.StatusHealthy,
		Timestamp:  now,
		Version:    "v1.0.0",
		Components: make(map[string]observability.ComponentHealth),
	}

	response.Components["test"] = observability.ComponentHealth{
		Status: observability.StatusHealthy,
	}

	assert.Equal(t, observability.StatusHealthy, response.Status)
	assert.Equal(t, now, response.Timestamp)
	assert.Equal(t, "v1.0.0", response.Version)
	assert.Len(t, response.Components, 1)
}

func TestReadinessResponseStructure(t *testing.T) {
	now := time.Now()
	response := observability.ReadinessResponse{
		Ready:      true,
		Timestamp:  now,
		Components: make(map[string]observability.ComponentHealth),
	}

	response.Components["test"] = observability.ComponentHealth{
		Status: observability.StatusHealthy,
	}

	assert.True(t, response.Ready)
	assert.Equal(t, now, response.Timestamp)
	assert.Len(t, response.Components, 1)
}

// Benchmark tests for performance validation.
func BenchmarkHealthCheckExecution(b *testing.B) {
	hc := observability.NewHealthChecker("v1.0.0")
	hc.RegisterHealthCheck("test", func(_ context.Context) error {
		return nil
	})

	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = hc.CheckHealth(ctx)
	}
}

func BenchmarkReadinessCheckExecution(b *testing.B) {
	hc := observability.NewHealthChecker("v1.0.0")
	hc.RegisterReadinessCheck("test", func(_ context.Context) error {
		return nil
	})

	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = hc.CheckReadiness(ctx)
	}
}

func BenchmarkHealthHandlerExecution(b *testing.B) {
	hc := observability.NewHealthChecker("v1.0.0")
	hc.RegisterHealthCheck("test", func(_ context.Context) error {
		return nil
	})

	handler := hc.HealthHandler()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		handler(w, req)
	}
}
