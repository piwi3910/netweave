// Package helpers provides common test utilities for integration tests.
// This includes testcontainers setup, mock servers, and test fixtures.
//
//go:build integration
// +build integration

package helpers

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// RedisContainer represents a test Redis container.
type RedisContainer struct {
	Container testcontainers.Container
	Host      string
	Port      string
}

// SetupRedisContainer starts a Redis container for testing.
// It waits for Redis to be ready before returning.
func SetupRedisContainer(ctx context.Context, t *testing.T) *RedisContainer {
	t.Helper()

	req := testcontainers.ContainerRequest{
		Image:        "redis:7.4-alpine",
		ExposedPorts: []string{"6379/tcp"},
		WaitingFor: wait.ForAll(
			wait.ForLog("Ready to accept connections"),
			wait.ForListeningPort("6379/tcp"),
		).WithDeadline(30 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start Redis container: %v", err)
	}

	// Get connection details
	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("failed to get Redis host: %v", err)
	}

	mappedPort, err := container.MappedPort(ctx, "6379")
	if err != nil {
		t.Fatalf("failed to get Redis port: %v", err)
	}

	return &RedisContainer{
		Container: container,
		Host:      host,
		Port:      mappedPort.Port(),
	}
}

// Addr returns the Redis connection address.
func (r *RedisContainer) Addr() string {
	return fmt.Sprintf("%s:%s", r.Host, r.Port)
}

// Cleanup terminates the Redis container.
func (r *RedisContainer) Cleanup(ctx context.Context) error {
	if r.Container != nil {
		if err := r.Container.Terminate(ctx); err != nil {
			return fmt.Errorf("failed to terminate Redis container: %w", err)
		}
	}
	return nil
}

// MinioContainer represents a test MinIO container (S3-compatible storage).
type MinioContainer struct {
	Container testcontainers.Container
	Host      string
	Port      string
	AccessKey string
	SecretKey string
}

// SetupMinioContainer starts a MinIO container for testing object storage.
func SetupMinioContainer(ctx context.Context, t *testing.T) *MinioContainer {
	t.Helper()

	accessKey := "minioadmin"
	secretKey := "minioadmin"

	req := testcontainers.ContainerRequest{
		Image:        "minio/minio:latest",
		ExposedPorts: []string{"9000/tcp"},
		Env: map[string]string{
			"MINIO_ROOT_USER":     accessKey,
			"MINIO_ROOT_PASSWORD": secretKey,
		},
		Cmd: []string{"server", "/data"},
		WaitingFor: wait.ForAll(
			wait.ForHTTP("/minio/health/live").WithPort("9000/tcp"),
			wait.ForListeningPort("9000/tcp"),
		).WithDeadline(30 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start MinIO container: %v", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("failed to get MinIO host: %v", err)
	}

	mappedPort, err := container.MappedPort(ctx, "9000")
	if err != nil {
		t.Fatalf("failed to get MinIO port: %v", err)
	}

	return &MinioContainer{
		Container: container,
		Host:      host,
		Port:      mappedPort.Port(),
		AccessKey: accessKey,
		SecretKey: secretKey,
	}
}

// Endpoint returns the MinIO endpoint URL.
func (m *MinioContainer) Endpoint() string {
	return fmt.Sprintf("http://%s:%s", m.Host, m.Port)
}

// Cleanup terminates the MinIO container.
func (m *MinioContainer) Cleanup(ctx context.Context) error {
	if m.Container != nil {
		if err := m.Container.Terminate(ctx); err != nil {
			return fmt.Errorf("failed to terminate MinIO container: %w", err)
		}
	}
	return nil
}

// TestEnvironment encapsulates all test infrastructure.
type TestEnvironment struct {
	Redis *RedisContainer
	Minio *MinioContainer
	ctx   context.Context
	t     *testing.T
}

// SetupTestEnvironment creates a complete test environment with all required containers.
func SetupTestEnvironment(t *testing.T) *TestEnvironment {
	t.Helper()

	ctx := context.Background()

	// Start Redis
	redis := SetupRedisContainer(ctx, t)

	// Start MinIO (optional, only if needed for specific tests)
	// Uncomment when needed:
	// minio := SetupMinioContainer(ctx, t)

	env := &TestEnvironment{
		Redis: redis,
		// Minio: minio,
		ctx: ctx,
		t:   t,
	}

	// Register cleanup
	t.Cleanup(func() {
		env.Cleanup()
	})

	return env
}

// Cleanup cleans up all test containers.
func (e *TestEnvironment) Cleanup() {
	if e.Redis != nil {
		if err := e.Redis.Cleanup(e.ctx); err != nil {
			e.t.Logf("failed to cleanup Redis: %v", err)
		}
	}

	if e.Minio != nil {
		if err := e.Minio.Cleanup(e.ctx); err != nil {
			e.t.Logf("failed to cleanup MinIO: %v", err)
		}
	}
}

// Context returns the test context.
func (e *TestEnvironment) Context() context.Context {
	return e.ctx
}
