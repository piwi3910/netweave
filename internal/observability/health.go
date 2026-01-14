package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// HealthStatus represents the health status of a componen.
type HealthStatus string

const (
	// StatusHealthy indicates the component is healthy.
	StatusHealthy HealthStatus = "healthy"
	// StatusUnhealthy indicates the component is unhealthy.
	StatusUnhealthy HealthStatus = "unhealthy"
	// StatusDegraded indicates the component is degraded but functional.
	StatusDegraded HealthStatus = "degraded"
)

// HealthCheck represents a health check function.
type HealthCheck func(ctx context.Context) error

// ComponentHealth represents the health status of a single componen.
type ComponentHealth struct {
	Status  HealthStatus `json:"status"`
	Message string       `json:"message,omitempty"`
	Error   string       `json:"error,omitempty"`
	Latency string       `json:"latency,omitempty"`
}

// HealthResponse represents the overall health check response.
type HealthResponse struct {
	Status     HealthStatus               `json:"status"`
	Timestamp  time.Time                  `json:"timestamp"`
	Version    string                     `json:"version,omitempty"`
	Components map[string]ComponentHealth `json:"components"`
}

// ReadinessResponse represents the readiness check response.
type ReadinessResponse struct {
	Ready      bool                       `json:"ready"`
	Timestamp  time.Time                  `json:"timestamp"`
	Components map[string]ComponentHealth `json:"components"`
}

// HealthChecker manages health and readiness checks.
type HealthChecker struct {
	mu              sync.RWMutex
	HealthChecks    map[string]HealthCheck // Exported for testing
	ReadinessChecks map[string]HealthCheck // Exported for testing
	Version         string                 // Exported for testing
	Timeout         time.Duration          // Exported for testing
}

// NewHealthChecker creates a new health checker.
func NewHealthChecker(version string) *HealthChecker {
	return &HealthChecker{
		HealthChecks:    make(map[string]HealthCheck),
		ReadinessChecks: make(map[string]HealthCheck),
		Version:         version,
		Timeout:         5 * time.Second, // Default timeout
	}
}

// RegisterHealthCheck registers a health check for a componen.
func (hc *HealthChecker) RegisterHealthCheck(name string, check HealthCheck) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	hc.HealthChecks[name] = check
}

// RegisterReadinessCheck registers a readiness check for a componen.
func (hc *HealthChecker) RegisterReadinessCheck(name string, check HealthCheck) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	hc.ReadinessChecks[name] = check
}

// SetTimeout sets the timeout for health checks.
func (hc *HealthChecker) SetTimeout(timeout time.Duration) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	hc.Timeout = timeout
}

// CheckHealth performs all health checks and returns the health status.
func (hc *HealthChecker) CheckHealth(ctx context.Context) *HealthResponse {
	hc.mu.RLock()
	checks := make(map[string]HealthCheck, len(hc.HealthChecks))
	for name, check := range hc.HealthChecks {
		checks[name] = check
	}
	timeout := hc.Timeout
	hc.mu.RUnlock()

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	components := hc.ExecuteChecks(ctx, checks)

	// Determine overall status
	overallStatus := StatusHealthy
	for _, component := range components {
		if component.Status == StatusUnhealthy {
			overallStatus = StatusUnhealthy
			break
		}
		if component.Status == StatusDegraded && overallStatus == StatusHealthy {
			overallStatus = StatusDegraded
		}
	}

	return &HealthResponse{
		Status:     overallStatus,
		Timestamp:  time.Now(),
		Version:    hc.Version,
		Components: components,
	}
}

// CheckReadiness performs all readiness checks and returns the readiness status.
func (hc *HealthChecker) CheckReadiness(ctx context.Context) *ReadinessResponse {
	hc.mu.RLock()
	checks := make(map[string]HealthCheck, len(hc.ReadinessChecks))
	for name, check := range hc.ReadinessChecks {
		checks[name] = check
	}
	timeout := hc.Timeout
	hc.mu.RUnlock()

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	components := hc.ExecuteChecks(ctx, checks)

	// Determine overall readiness - all components must be healthy
	ready := true
	for _, component := range components {
		if component.Status != StatusHealthy {
			ready = false
			break
		}
	}

	return &ReadinessResponse{
		Ready:      ready,
		Timestamp:  time.Now(),
		Components: components,
	}
}

// ExecuteChecks executes a set of health checks concurrently.
// ExecuteChecks executes checks concurrently. Exported for testing.
func (hc *HealthChecker) ExecuteChecks(ctx context.Context, checks map[string]HealthCheck) map[string]ComponentHealth {
	components := make(map[string]ComponentHealth)
	if len(checks) == 0 {
		return components
	}

	var wg sync.WaitGroup
	resultChan := make(chan struct {
		name   string
		health ComponentHealth
	}, len(checks))

	for name, check := range checks {
		wg.Add(1)
		go func(name string, check HealthCheck) {
			defer wg.Done()

			start := time.Now()
			err := check(ctx)
			latency := time.Since(start)

			health := ComponentHealth{
				Status:  StatusHealthy,
				Latency: latency.String(),
			}

			if err != nil {
				if ctx.Err() != nil {
					health.Status = StatusUnhealthy
					health.Error = "check timed out"
				} else {
					health.Status = StatusUnhealthy
					health.Error = err.Error()
				}
			}

			resultChan <- struct {
				name   string
				health ComponentHealth
			}{name: name, health: health}
		}(name, check)
	}

	// Close channel when all checks complete
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results
	for result := range resultChan {
		components[result.name] = result.health
	}

	return components
}

// HealthHandler returns an HTTP handler for the health endpoin.
func (hc *HealthChecker) HealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		health := hc.CheckHealth(r.Context())

		statusCode := http.StatusOK
		if health.Status == StatusUnhealthy {
			statusCode = http.StatusServiceUnavailable
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)

		if err := json.NewEncoder(w).Encode(health); err != nil {
			GetLogger().WithError(err).Error("failed to encode health response")
		}
	}
}

// ReadinessHandler returns an HTTP handler for the readiness endpoin.
func (hc *HealthChecker) ReadinessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		readiness := hc.CheckReadiness(r.Context())

		statusCode := http.StatusOK
		if !readiness.Ready {
			statusCode = http.StatusServiceUnavailable
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)

		if err := json.NewEncoder(w).Encode(readiness); err != nil {
			GetLogger().WithError(err).Error("failed to encode readiness response")
		}
	}
}

// LivenessHandler returns an HTTP handler for the liveness endpoint
// Liveness is simpler - just checks if the process is alive.
func LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		response := map[string]interface{}{
			"alive":     true,
			"timestamp": time.Now(),
		}

		if err := json.NewEncoder(w).Encode(response); err != nil {
			GetLogger().WithError(err).Error("failed to encode liveness response")
		}
	}
}

// Common health check implementations

// RedisHealthCheck creates a health check for Redis.
func RedisHealthCheck(pingFunc func(ctx context.Context) error) HealthCheck {
	return func(ctx context.Context) error {
		if pingFunc == nil {
			return fmt.Errorf("redis ping function not provided")
		}
		return pingFunc(ctx)
	}
}

// KubernetesHealthCheck creates a health check for Kubernetes API.
func KubernetesHealthCheck(pingFunc func(ctx context.Context) error) HealthCheck {
	return func(ctx context.Context) error {
		if pingFunc == nil {
			return fmt.Errorf("kubernetes ping function not provided")
		}
		return pingFunc(ctx)
	}
}

// AdapterHealthCheck creates a health check for an adapter.
func AdapterHealthCheck(name string, checkFunc func(ctx context.Context) error) HealthCheck {
	return func(ctx context.Context) error {
		if checkFunc == nil {
			return fmt.Errorf("adapter %s check function not provided", name)
		}
		return checkFunc(ctx)
	}
}

// GenericHealthCheck creates a generic health check from a function.
func GenericHealthCheck(checkFunc func(ctx context.Context) error) HealthCheck {
	return checkFunc
}
