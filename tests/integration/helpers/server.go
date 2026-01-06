// Package helpers provides common test utilities for integration tests.
//
//go:build integration
// +build integration

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
	Server *httptest.Server
	Config *config.Config
}

// NewTestServer creates a new test server with the given adapter and storage.
// It sets up a complete server environment with test configuration and logger.
func NewTestServer(t *testing.T, adapter adapter.Adapter, store storage.Store) *TestServer {
	t.Helper()

	// Create test configuration
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host:    "localhost",
			Port:    8080,
			GinMode: "test",
		},
		Log: config.LogConfig{
			Level:  "info",
			Format: "json",
		},
	}

	// Create logger
	logger, err := observability.NewLogger(&cfg.Log)
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	// Create server
	srv := server.New(cfg, logger, adapter, store)

	// Create test HTTP server
	ts := httptest.NewServer(srv.Handler())

	// Register cleanup
	t.Cleanup(func() {
		ts.Close()
	})

	return &TestServer{
		Server: ts,
		Config: cfg,
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
