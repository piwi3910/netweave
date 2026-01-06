package observability

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHealthChecker(t *testing.T) {
	hc := NewHealthChecker("v1.0.0")
	require.NotNil(t, hc)
	assert.Equal(t, "v1.0.0", hc.version)
	assert.Equal(t, 5*time.Second, hc.timeout)
	assert.NotNil(t, hc.healthChecks)
	assert.NotNil(t, hc.readinessChecks)
}

func TestRegisterHealthCheck(t *testing.T) {
	hc := NewHealthChecker("v1.0.0")

	checkFunc := func(ctx context.Context) error {
		return nil
	}

	hc.RegisterHealthCheck("test-component", checkFunc)

	// Verify check was registered
	assert.Len(t, hc.healthChecks, 1)
	assert.Contains(t, hc.healthChecks, "test-component")
}

func TestRegisterReadinessCheck(t *testing.T) {
	hc := NewHealthChecker("v1.0.0")

	checkFunc := func(ctx context.Context) error {
		return nil
	}

	hc.RegisterReadinessCheck("test-component", checkFunc)

	// Verify check was registered
	assert.Len(t, hc.readinessChecks, 1)
	assert.Contains(t, hc.readinessChecks, "test-component")
}

func TestSetTimeout(t *testing.T) {
	hc := NewHealthChecker("v1.0.0")
	assert.Equal(t, 5*time.Second, hc.timeout)

	hc.SetTimeout(10 * time.Second)
	assert.Equal(t, 10*time.Second, hc.timeout)
}

func TestCheckHealthAllHealthy(t *testing.T) {
	hc := NewHealthChecker("v1.0.0")

	// Register healthy checks
	hc.RegisterHealthCheck("component1", func(ctx context.Context) error {
		return nil
	})
	hc.RegisterHealthCheck("component2", func(ctx context.Context) error {
		return nil
	})

	ctx := context.Background()
	response := hc.CheckHealth(ctx)

	require.NotNil(t, response)
	assert.Equal(t, StatusHealthy, response.Status)
	assert.Equal(t, "v1.0.0", response.Version)
	assert.Len(t, response.Components, 2)

	for _, comp := range response.Components {
		assert.Equal(t, StatusHealthy, comp.Status)
		assert.Empty(t, comp.Error)
	}
}

func TestCheckHealthWithUnhealthyComponent(t *testing.T) {
	hc := NewHealthChecker("v1.0.0")

	// Register healthy and unhealthy checks
	hc.RegisterHealthCheck("healthy-component", func(ctx context.Context) error {
		return nil
	})
	hc.RegisterHealthCheck("unhealthy-component", func(ctx context.Context) error {
		return errors.New("component is down")
	})

	ctx := context.Background()
	response := hc.CheckHealth(ctx)

	require.NotNil(t, response)
	assert.Equal(t, StatusUnhealthy, response.Status)

	healthyComp := response.Components["healthy-component"]
	assert.Equal(t, StatusHealthy, healthyComp.Status)

	unhealthyComp := response.Components["unhealthy-component"]
	assert.Equal(t, StatusUnhealthy, unhealthyComp.Status)
	assert.Contains(t, unhealthyComp.Error, "component is down")
}

func TestCheckHealthTimeout(t *testing.T) {
	hc := NewHealthChecker("v1.0.0")
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
	assert.Equal(t, StatusUnhealthy, response.Status)

	slowComp := response.Components["slow-component"]
	assert.Equal(t, StatusUnhealthy, slowComp.Status)
	assert.Equal(t, "check timed out", slowComp.Error)
}

func TestCheckReadinessAllReady(t *testing.T) {
	hc := NewHealthChecker("v1.0.0")

	// Register ready checks
	hc.RegisterReadinessCheck("redis", func(ctx context.Context) error {
		return nil
	})
	hc.RegisterReadinessCheck("kubernetes", func(ctx context.Context) error {
		return nil
	})

	ctx := context.Background()
	response := hc.CheckReadiness(ctx)

	require.NotNil(t, response)
	assert.True(t, response.Ready)
	assert.Len(t, response.Components, 2)

	for _, comp := range response.Components {
		assert.Equal(t, StatusHealthy, comp.Status)
	}
}

func TestCheckReadinessWithNotReadyComponent(t *testing.T) {
	hc := NewHealthChecker("v1.0.0")

	hc.RegisterReadinessCheck("redis", func(ctx context.Context) error {
		return nil
	})
	hc.RegisterReadinessCheck("kubernetes", func(ctx context.Context) error {
		return errors.New("k8s not reachable")
	})

	ctx := context.Background()
	response := hc.CheckReadiness(ctx)

	require.NotNil(t, response)
	assert.False(t, response.Ready)

	k8sComp := response.Components["kubernetes"]
	assert.Equal(t, StatusUnhealthy, k8sComp.Status)
	assert.Contains(t, k8sComp.Error, "k8s not reachable")
}

func TestExecuteChecksEmpty(t *testing.T) {
	hc := NewHealthChecker("v1.0.0")
	ctx := context.Background()

	checks := make(map[string]HealthCheck)
	components := hc.executeChecks(ctx, checks)

	assert.NotNil(t, components)
	assert.Len(t, components, 0)
}

func TestExecuteChecksConcurrent(t *testing.T) {
	hc := NewHealthChecker("v1.0.0")
	ctx := context.Background()

	checks := map[string]HealthCheck{
		"check1": func(ctx context.Context) error {
			time.Sleep(50 * time.Millisecond)
			return nil
		},
		"check2": func(ctx context.Context) error {
			time.Sleep(50 * time.Millisecond)
			return nil
		},
		"check3": func(ctx context.Context) error {
			time.Sleep(50 * time.Millisecond)
			return nil
		},
	}

	start := time.Now()
	components := hc.executeChecks(ctx, checks)
	duration := time.Since(start)

	// Should complete in parallel (~50ms), not sequential (~150ms)
	assert.Less(t, duration, 100*time.Millisecond)
	assert.Len(t, components, 3)

	for _, comp := range components {
		assert.Equal(t, StatusHealthy, comp.Status)
	}
}

func TestHealthHandler(t *testing.T) {
	hc := NewHealthChecker("v1.0.0")
	hc.RegisterHealthCheck("test", func(ctx context.Context) error {
		return nil
	})

	handler := hc.HealthHandler()
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response HealthResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, StatusHealthy, response.Status)
	assert.Equal(t, "v1.0.0", response.Version)
}

func TestHealthHandlerUnhealthy(t *testing.T) {
	hc := NewHealthChecker("v1.0.0")
	hc.RegisterHealthCheck("test", func(ctx context.Context) error {
		return errors.New("component failed")
	})

	handler := hc.HealthHandler()
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var response HealthResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, StatusUnhealthy, response.Status)
}

func TestReadinessHandler(t *testing.T) {
	hc := NewHealthChecker("v1.0.0")
	hc.RegisterReadinessCheck("test", func(ctx context.Context) error {
		return nil
	})

	handler := hc.ReadinessHandler()
	req := httptest.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response ReadinessResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.True(t, response.Ready)
}

func TestReadinessHandlerNotReady(t *testing.T) {
	hc := NewHealthChecker("v1.0.0")
	hc.RegisterReadinessCheck("test", func(ctx context.Context) error {
		return errors.New("not ready")
	})

	handler := hc.ReadinessHandler()
	req := httptest.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var response ReadinessResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	assert.False(t, response.Ready)
}

func TestLivenessHandler(t *testing.T) {
	handler := LivenessHandler()
	req := httptest.NewRequest("GET", "/live", nil)
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
	pingFunc := func(ctx context.Context) error {
		return nil
	}
	check := RedisHealthCheck(pingFunc)
	err := check(context.Background())
	assert.NoError(t, err)

	// Error case
	pingFuncErr := func(ctx context.Context) error {
		return errors.New("redis connection failed")
	}
	checkErr := RedisHealthCheck(pingFuncErr)
	err = checkErr(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "redis connection failed")

	// Nil function case
	checkNil := RedisHealthCheck(nil)
	err = checkNil(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "redis ping function not provided")
}

func TestKubernetesHealthCheck(t *testing.T) {
	// Success case
	pingFunc := func(ctx context.Context) error {
		return nil
	}
	check := KubernetesHealthCheck(pingFunc)
	err := check(context.Background())
	assert.NoError(t, err)

	// Error case
	pingFuncErr := func(ctx context.Context) error {
		return errors.New("k8s api unreachable")
	}
	checkErr := KubernetesHealthCheck(pingFuncErr)
	err = checkErr(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "k8s api unreachable")

	// Nil function case
	checkNil := KubernetesHealthCheck(nil)
	err = checkNil(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "kubernetes ping function not provided")
}

func TestAdapterHealthCheck(t *testing.T) {
	// Success case
	checkFunc := func(ctx context.Context) error {
		return nil
	}
	check := AdapterHealthCheck("k8s-adapter", checkFunc)
	err := check(context.Background())
	assert.NoError(t, err)

	// Error case
	checkFuncErr := func(ctx context.Context) error {
		return errors.New("adapter error")
	}
	checkErr := AdapterHealthCheck("mock-adapter", checkFuncErr)
	err = checkErr(context.Background())
	assert.Error(t, err)

	// Nil function case
	checkNil := AdapterHealthCheck("test-adapter", nil)
	err = checkNil(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "adapter test-adapter check function not provided")
}

func TestGenericHealthCheck(t *testing.T) {
	checkFunc := func(ctx context.Context) error {
		return nil
	}
	check := GenericHealthCheck(checkFunc)
	err := check(context.Background())
	assert.NoError(t, err)

	checkFuncErr := func(ctx context.Context) error {
		return errors.New("generic error")
	}
	checkErr := GenericHealthCheck(checkFuncErr)
	err = checkErr(context.Background())
	assert.Error(t, err)
}

func TestHealthStatusConstants(t *testing.T) {
	assert.Equal(t, HealthStatus("healthy"), StatusHealthy)
	assert.Equal(t, HealthStatus("unhealthy"), StatusUnhealthy)
	assert.Equal(t, HealthStatus("degraded"), StatusDegraded)
}

func TestComponentHealthStructure(t *testing.T) {
	comp := ComponentHealth{
		Status:  StatusHealthy,
		Message: "Component is healthy",
		Latency: "10ms",
	}

	assert.Equal(t, StatusHealthy, comp.Status)
	assert.Equal(t, "Component is healthy", comp.Message)
	assert.Equal(t, "10ms", comp.Latency)
	assert.Empty(t, comp.Error)
}

func TestHealthResponseStructure(t *testing.T) {
	now := time.Now()
	response := HealthResponse{
		Status:     StatusHealthy,
		Timestamp:  now,
		Version:    "v1.0.0",
		Components: make(map[string]ComponentHealth),
	}

	response.Components["test"] = ComponentHealth{
		Status: StatusHealthy,
	}

	assert.Equal(t, StatusHealthy, response.Status)
	assert.Equal(t, now, response.Timestamp)
	assert.Equal(t, "v1.0.0", response.Version)
	assert.Len(t, response.Components, 1)
}

func TestReadinessResponseStructure(t *testing.T) {
	now := time.Now()
	response := ReadinessResponse{
		Ready:      true,
		Timestamp:  now,
		Components: make(map[string]ComponentHealth),
	}

	response.Components["test"] = ComponentHealth{
		Status: StatusHealthy,
	}

	assert.True(t, response.Ready)
	assert.Equal(t, now, response.Timestamp)
	assert.Len(t, response.Components, 1)
}

// Benchmark tests for performance validation
func BenchmarkHealthCheckExecution(b *testing.B) {
	hc := NewHealthChecker("v1.0.0")
	hc.RegisterHealthCheck("test", func(ctx context.Context) error {
		return nil
	})

	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = hc.CheckHealth(ctx)
	}
}

func BenchmarkReadinessCheckExecution(b *testing.B) {
	hc := NewHealthChecker("v1.0.0")
	hc.RegisterReadinessCheck("test", func(ctx context.Context) error {
		return nil
	})

	ctx := context.Background()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = hc.CheckReadiness(ctx)
	}
}

func BenchmarkHealthHandlerExecution(b *testing.B) {
	hc := NewHealthChecker("v1.0.0")
	hc.RegisterHealthCheck("test", func(ctx context.Context) error {
		return nil
	})

	handler := hc.HealthHandler()
	req := httptest.NewRequest("GET", "/health", nil)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		handler(w, req)
	}
}
