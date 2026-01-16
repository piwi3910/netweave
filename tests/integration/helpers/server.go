//go:build integration
// +build integration

// Package helpers provides common test utilities for integration tests.
package helpers

import (
	"net/http/httptest"
	"testing"

	"github.com/piwi3910/netweave/internal/adapter"
	"github.com/piwi3910/netweave/internal/config"
	"github.com/piwi3910/netweave/internal/observability"
	"github.com/piwi3910/netweave/internal/server"
	"github.com/piwi3910/netweave/internal/storage"
)

// TestServer wraps an HTTP test server for integration testing.
type TestServer struct {
	Server       *httptest.Server
	Config       *config.Config
	InternalSrv  *server.Server // Exposed for advanced test setup (e.g., DMS initialization)
}

// NewTestServer creates a new test server with the given adapter and storage.
// It sets up a complete server environment with test configuration and logger.
func NewTestServer(t *testing.T, adp adapter.Adapter, store storage.Store) *TestServer {
	t.Helper()

	// Create test configuration
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host:    "localhost",
			Port:    8080,
			GinMode: "test",
		},
		Observability: config.ObservabilityConfig{
			Logging: config.LoggingConfig{
				// Use warn level in tests to suppress expected ERROR logs
				// from test cases that intentionally trigger error conditions
				Level:  "warn",
				Format: "json",
			},
			Metrics: config.MetricsConfig{
				Enabled:   false, // Disable metrics in tests
				Namespace: "netweave",
				Subsystem: "gateway",
			},
		},
		Security: config.SecurityConfig{
			EnableCORS:             false,
			RateLimitEnabled:       false,
			DisableSSRFProtection:  true, // Allow localhost callbacks in tests
			AllowInsecureCallbacks: true, // Allow HTTP callbacks in tests
		},
		Validation: config.ValidationConfig{
			Enabled:          true,
			ValidateResponse: false,
			MaxBodySize:      1048576,
		},
	}

	// Create logger using environment-based initialization
	obsLogger, err := observability.InitLogger("test")
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	// Create server (use the embedded zap.Logger)
	srv := server.New(cfg, obsLogger.Logger, adp, store, nil)

	// Create test HTTP server
	ts := httptest.NewServer(srv.Router())

	// Register cleanup
	t.Cleanup(func() {
		ts.Close()
	})

	return &TestServer{
		Server:      ts,
		Config:      cfg,
		InternalSrv: srv,
	}
}

// URL returns the base URL of the test server.
func (ts *TestServer) URL() string {
	return ts.Server.URL
}

// O2IMSURL returns the O2-IMS API base URL.
func (ts *TestServer) O2IMSURL() string {
	return ts.Server.URL + "/o2ims-infrastructureInventory/v1"
}

// Close closes the test server.
func (ts *TestServer) Close() {
	ts.Server.Close()
}
