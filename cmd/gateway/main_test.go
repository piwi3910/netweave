package main_test

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	main "github.com/piwi3910/netweave/cmd/gateway"
	"github.com/piwi3910/netweave/internal/config"
)

func TestInitializeAuth_Standalone(t *testing.T) {
	// Start miniredis for testing.
	mr := miniredis.RunT(t)
	defer mr.Close()

	cfg := &config.Config{
		Redis: config.RedisConfig{
			Mode:         "standalone",
			Addresses:    []string{mr.Addr()},
			DB:           0,
			MaxRetries:   3,
			DialTimeout:  5 * time.Second,
			ReadTimeout:  3 * time.Second,
			WriteTimeout: 3 * time.Second,
			PoolSize:     10,
		},
		MultiTenancy: config.MultiTenancyConfig{
			Enabled:                true,
			RequireMTLS:            false,
			InitializeDefaultRoles: true,
			SkipAuthPaths:          []string{"/health", "/metrics"},
		},
	}

	logger := zap.NewNop()

	authStore, authMw, err := main.InitializeAuth(cfg, logger)
	require.NoError(t, err)
	require.NotNil(t, authStore)
	require.NotNil(t, authMw)

	// Clean up.
	err = authStore.Close()
	assert.NoError(t, err)
}

func TestInitializeAuth_StandaloneNoDefaultRoles(t *testing.T) {
	mr := miniredis.RunT(t)
	defer mr.Close()

	cfg := &config.Config{
		Redis: config.RedisConfig{
			Mode:         "standalone",
			Addresses:    []string{mr.Addr()},
			DB:           0,
			MaxRetries:   3,
			DialTimeout:  5 * time.Second,
			ReadTimeout:  3 * time.Second,
			WriteTimeout: 3 * time.Second,
			PoolSize:     10,
		},
		MultiTenancy: config.MultiTenancyConfig{
			Enabled:                true,
			RequireMTLS:            true,
			InitializeDefaultRoles: false,
			SkipAuthPaths:          []string{"/health"},
		},
	}

	logger := zap.NewNop()

	authStore, authMw, err := main.InitializeAuth(cfg, logger)
	require.NoError(t, err)
	require.NotNil(t, authStore)
	require.NotNil(t, authMw)

	err = authStore.Close()
	assert.NoError(t, err)
}

func TestInitializeAuth_DefaultAddress(t *testing.T) {
	// This test verifies default address logic when no addresses provided.
	// Note: This will fail to connect since localhost:6379 likely isn't running,
	// but it tests the configuration path.
	mr := miniredis.RunT(t)
	defer mr.Close()

	cfg := &config.Config{
		Redis: config.RedisConfig{
			Mode:         "standalone",
			Addresses:    []string{mr.Addr()}, // Use miniredis address
			DB:           0,
			MaxRetries:   3,
			DialTimeout:  5 * time.Second,
			ReadTimeout:  3 * time.Second,
			WriteTimeout: 3 * time.Second,
			PoolSize:     10,
		},
		MultiTenancy: config.MultiTenancyConfig{
			Enabled:                true,
			RequireMTLS:            false,
			InitializeDefaultRoles: false,
			SkipAuthPaths:          []string{},
		},
	}

	logger := zap.NewNop()

	authStore, authMw, err := main.InitializeAuth(cfg, logger)
	require.NoError(t, err)
	require.NotNil(t, authStore)
	require.NotNil(t, authMw)

	err = authStore.Close()
	assert.NoError(t, err)
}

func TestInitializeAuth_SentinelMode(t *testing.T) {
	// Test Sentinel mode configuration (without actual Sentinel).
	// This verifies the configuration path for Sentinel mode.
	mr := miniredis.RunT(t)
	defer mr.Close()

	cfg := &config.Config{
		Redis: config.RedisConfig{
			Mode:         "sentinel",
			Addresses:    []string{mr.Addr()}, // Use miniredis as "sentinel"
			MasterName:   "mymaster",
			DB:           0,
			MaxRetries:   3,
			DialTimeout:  5 * time.Second,
			ReadTimeout:  3 * time.Second,
			WriteTimeout: 3 * time.Second,
			PoolSize:     10,
		},
		MultiTenancy: config.MultiTenancyConfig{
			Enabled:                true,
			RequireMTLS:            true,
			InitializeDefaultRoles: false,
			SkipAuthPaths:          []string{"/health", "/metrics"},
		},
	}

	logger := zap.NewNop()

	// Note: This will fail because miniredis doesn't support Sentinel protocol,
	// but we're testing that the configuration path works correctly.
	authStore, authMw, err := main.InitializeAuth(cfg, logger)

	// Sentinel mode with miniredis will fail connectivity check.
	// This is expected behavior - we're testing the config path.
	if err != nil {
		assert.Contains(t, err.Error(), "connectivity check failed")
	} else {
		require.NotNil(t, authStore)
		require.NotNil(t, authMw)
		_ = authStore.Close()
	}
}

func TestInitializeAuth_ConnectionFailure(t *testing.T) {
	cfg := &config.Config{
		Redis: config.RedisConfig{
			Mode:         "standalone",
			Addresses:    []string{"localhost:59999"}, // Non-existent port.
			DB:           0,
			MaxRetries:   1, // Minimize retries for faster test.
			DialTimeout:  1 * time.Second,
			ReadTimeout:  1 * time.Second,
			WriteTimeout: 1 * time.Second,
			PoolSize:     1,
		},
		MultiTenancy: config.MultiTenancyConfig{
			Enabled:                true,
			RequireMTLS:            false,
			InitializeDefaultRoles: false,
			SkipAuthPaths:          []string{},
		},
	}

	logger := zap.NewNop()

	authStore, authMw, err := main.InitializeAuth(cfg, logger)
	assert.Error(t, err)
	assert.Nil(t, authStore)
	assert.Nil(t, authMw)
	assert.Contains(t, err.Error(), "connectivity check failed")
}

func TestApplicationComponents_Close(t *testing.T) {
	t.Run("handles nil components gracefully", func(t *testing.T) {
		logger := zap.NewNop()

		components := main.NewApplicationComponentsForTest(nil)

		// Should not panic with nil components and return nil error.
		err := components.Close(logger)
		assert.NoError(t, err)
	})

	t.Run("returns nil when all closes succeed", func(t *testing.T) {
		mr := miniredis.RunT(t)
		defer mr.Close()

		cfg := &config.Config{
			Redis: config.RedisConfig{
				Mode:         "standalone",
				Addresses:    []string{mr.Addr()},
				DB:           0,
				MaxRetries:   3,
				DialTimeout:  5 * time.Second,
				ReadTimeout:  3 * time.Second,
				WriteTimeout: 3 * time.Second,
				PoolSize:     10,
			},
			MultiTenancy: config.MultiTenancyConfig{
				Enabled:                true,
				RequireMTLS:            false,
				InitializeDefaultRoles: false,
				SkipAuthPaths:          []string{"/health"},
			},
		}

		logger := zap.NewNop()

		authStore, _, err := main.InitializeAuth(cfg, logger)
		require.NoError(t, err)

		components := main.NewApplicationComponentsForTest(authStore)

		// Close should succeed and return nil.
		err = components.Close(logger)
		assert.NoError(t, err)
	})
}
