package events_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/piwi3910/netweave/internal/database"
	"github.com/piwi3910/netweave/internal/events"
)

const (
	eventsPgTestImage    = "postgres:16-alpine"
	eventsPgTestDB       = "netweave_events_test"
	eventsPgTestUser     = "testuser"
	eventsPgTestPassword = "testpass"
)

// setupEventsPgContainer starts a PostgreSQL container with migrations for events tests.
func setupEventsPgContainer(t *testing.T) *database.DB {
	t.Helper()

	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        eventsPgTestImage,
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     eventsPgTestUser,
			"POSTGRES_PASSWORD": eventsPgTestPassword,
			"POSTGRES_DB":       eventsPgTestDB,
		},
		WaitingFor: wait.ForAll(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
			wait.ForListeningPort("5432/tcp"),
		).WithDeadline(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "failed to start PostgreSQL container")

	host, err := container.Host(ctx)
	require.NoError(t, err)

	mappedPort, err := container.MappedPort(ctx, "5432/tcp")
	require.NoError(t, err)

	t.Cleanup(func() {
		if termErr := container.Terminate(ctx); termErr != nil {
			t.Logf("failed to terminate PostgreSQL container: %v", termErr)
		}
	})

	cfg := &database.PostgresConfig{
		Host:           host,
		Port:           mappedPort.Int(),
		Database:       eventsPgTestDB,
		User:           eventsPgTestUser,
		PasswordEnvVar: "TEST_PG_PASSWORD",
		SSLMode:        "disable",
		MaxConns:       5,
		MinConns:       1,
	}

	db, err := database.New(ctx, cfg, eventsPgTestPassword)
	require.NoError(t, err)

	err = database.Migrate(ctx, db.Pool)
	require.NoError(t, err)

	t.Cleanup(func() {
		db.Close()
	})

	return db
}

func newTestTracker(t *testing.T, db *database.DB) *events.PostgresDeliveryTracker {
	t.Helper()
	return events.NewPostgresDeliveryTracker(db)
}

func TestPostgresDeliveryTracker_Track(t *testing.T) {
	db := setupEventsPgContainer(t)

	tests := []struct {
		name     string
		delivery *events.NotificationDelivery
		wantErr  bool
	}{
		{
			name: "valid delivery",
			delivery: &events.NotificationDelivery{
				ID:             "delivery-1",
				EventID:        "event-1",
				SubscriptionID: "sub-1",
				CallbackURL:    "https://example.com/webhook",
				Status:         events.DeliveryStatusPending,
				Attempts:       0,
				MaxAttempts:    3,
				CreatedAt:      time.Now().UTC(),
			},
			wantErr: false,
		},
		{
			name: "delivery with all fields",
			delivery: &events.NotificationDelivery{
				ID:             "delivery-2",
				EventID:        "event-2",
				SubscriptionID: "sub-2",
				CallbackURL:    "https://example.com/webhook",
				Status:         events.DeliveryStatusDelivered,
				Attempts:       1,
				MaxAttempts:    3,
				LastAttemptAt:  time.Now().UTC(),
				HTTPStatusCode: 200,
				ResponseTime:   42,
				CreatedAt:      time.Now().UTC(),
				CompletedAt:    time.Now().UTC(),
			},
			wantErr: false,
		},
		{
			name:    "nil delivery",
			wantErr: true,
		},
		{
			name:     "empty ID",
			delivery: &events.NotificationDelivery{ID: ""},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracker := newTestTracker(t, db)
			ctx := context.Background()

			err := tracker.Track(ctx, tt.delivery)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestPostgresDeliveryTracker_TrackUpsert(t *testing.T) {
	db := setupEventsPgContainer(t)
	tracker := newTestTracker(t, db)
	ctx := context.Background()

	// Create initial delivery.
	delivery := &events.NotificationDelivery{
		ID:             "delivery-upsert-1",
		EventID:        "event-upsert",
		SubscriptionID: "sub-upsert",
		CallbackURL:    "https://example.com/webhook",
		Status:         events.DeliveryStatusPending,
		Attempts:       0,
		MaxAttempts:    3,
	}
	require.NoError(t, tracker.Track(ctx, delivery))

	// Update with retry status.
	delivery.Status = events.DeliveryStatusRetrying
	delivery.Attempts = 1
	delivery.LastAttemptAt = time.Now().UTC()
	delivery.LastError = "connection refused"
	delivery.HTTPStatusCode = 503
	require.NoError(t, tracker.Track(ctx, delivery))

	// Verify update.
	got, err := tracker.Get(ctx, "delivery-upsert-1")
	require.NoError(t, err)
	assert.Equal(t, events.DeliveryStatusRetrying, got.Status)
	assert.Equal(t, 1, got.Attempts)
	assert.Equal(t, "connection refused", got.LastError)
	assert.Equal(t, 503, got.HTTPStatusCode)
}

func TestPostgresDeliveryTracker_Get(t *testing.T) {
	db := setupEventsPgContainer(t)
	tracker := newTestTracker(t, db)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Microsecond)

	delivery := &events.NotificationDelivery{
		ID:             "delivery-get-1",
		EventID:        "event-get",
		SubscriptionID: "sub-get",
		CallbackURL:    "https://example.com/webhook",
		Status:         events.DeliveryStatusDelivered,
		Attempts:       1,
		MaxAttempts:    3,
		LastAttemptAt:  now,
		HTTPStatusCode: 200,
		ResponseTime:   25,
		CompletedAt:    now,
	}
	require.NoError(t, tracker.Track(ctx, delivery))

	got, err := tracker.Get(ctx, "delivery-get-1")
	require.NoError(t, err)
	assert.Equal(t, delivery.ID, got.ID)
	assert.Equal(t, delivery.EventID, got.EventID)
	assert.Equal(t, delivery.SubscriptionID, got.SubscriptionID)
	assert.Equal(t, delivery.CallbackURL, got.CallbackURL)
	assert.Equal(t, delivery.Status, got.Status)
	assert.Equal(t, delivery.Attempts, got.Attempts)
	assert.Equal(t, delivery.MaxAttempts, got.MaxAttempts)
	assert.Equal(t, delivery.HTTPStatusCode, got.HTTPStatusCode)
	assert.Equal(t, delivery.ResponseTime, got.ResponseTime)

	// Not found.
	_, err = tracker.Get(ctx, "delivery-nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// Empty ID.
	_, err = tracker.Get(ctx, "")
	require.Error(t, err)
}

func TestPostgresDeliveryTracker_ListByEvent(t *testing.T) {
	db := setupEventsPgContainer(t)
	tracker := newTestTracker(t, db)
	ctx := context.Background()

	// Track deliveries for different events.
	require.NoError(t, tracker.Track(ctx, &events.NotificationDelivery{
		ID: "d-event-1", EventID: "event-alpha", SubscriptionID: "sub-1",
		CallbackURL: "https://example.com/w", Status: events.DeliveryStatusPending,
	}))
	require.NoError(t, tracker.Track(ctx, &events.NotificationDelivery{
		ID: "d-event-2", EventID: "event-alpha", SubscriptionID: "sub-2",
		CallbackURL: "https://example.com/w", Status: events.DeliveryStatusPending,
	}))
	require.NoError(t, tracker.Track(ctx, &events.NotificationDelivery{
		ID: "d-event-3", EventID: "event-beta", SubscriptionID: "sub-1",
		CallbackURL: "https://example.com/w", Status: events.DeliveryStatusPending,
	}))

	deliveries, err := tracker.ListByEvent(ctx, "event-alpha")
	require.NoError(t, err)
	assert.Len(t, deliveries, 2)

	deliveries, err = tracker.ListByEvent(ctx, "event-beta")
	require.NoError(t, err)
	assert.Len(t, deliveries, 1)

	deliveries, err = tracker.ListByEvent(ctx, "event-nonexistent")
	require.NoError(t, err)
	assert.Empty(t, deliveries)

	_, err = tracker.ListByEvent(ctx, "")
	require.Error(t, err)
}

func TestPostgresDeliveryTracker_ListBySubscription(t *testing.T) {
	db := setupEventsPgContainer(t)
	tracker := newTestTracker(t, db)
	ctx := context.Background()

	require.NoError(t, tracker.Track(ctx, &events.NotificationDelivery{
		ID: "d-sub-1", EventID: "e-1", SubscriptionID: "sub-tracked",
		CallbackURL: "https://example.com/w", Status: events.DeliveryStatusPending,
	}))
	require.NoError(t, tracker.Track(ctx, &events.NotificationDelivery{
		ID: "d-sub-2", EventID: "e-2", SubscriptionID: "sub-tracked",
		CallbackURL: "https://example.com/w", Status: events.DeliveryStatusDelivered,
	}))
	require.NoError(t, tracker.Track(ctx, &events.NotificationDelivery{
		ID: "d-sub-3", EventID: "e-3", SubscriptionID: "sub-other",
		CallbackURL: "https://example.com/w", Status: events.DeliveryStatusPending,
	}))

	deliveries, err := tracker.ListBySubscription(ctx, "sub-tracked")
	require.NoError(t, err)
	assert.Len(t, deliveries, 2)

	_, err = tracker.ListBySubscription(ctx, "")
	require.Error(t, err)
}

func TestPostgresDeliveryTracker_ListFailed(t *testing.T) {
	db := setupEventsPgContainer(t)
	tracker := newTestTracker(t, db)
	ctx := context.Background()

	require.NoError(t, tracker.Track(ctx, &events.NotificationDelivery{
		ID: "d-fail-1", EventID: "e-1", SubscriptionID: "sub-1",
		CallbackURL: "https://example.com/w", Status: events.DeliveryStatusFailed,
		LastError: "timeout", Attempts: 3, MaxAttempts: 3,
	}))
	require.NoError(t, tracker.Track(ctx, &events.NotificationDelivery{
		ID: "d-fail-2", EventID: "e-2", SubscriptionID: "sub-1",
		CallbackURL: "https://example.com/w", Status: events.DeliveryStatusDelivered,
	}))
	require.NoError(t, tracker.Track(ctx, &events.NotificationDelivery{
		ID: "d-fail-3", EventID: "e-3", SubscriptionID: "sub-1",
		CallbackURL: "https://example.com/w", Status: events.DeliveryStatusFailed,
		LastError: "server error", Attempts: 3, MaxAttempts: 3,
	}))

	failed, err := tracker.ListFailed(ctx)
	require.NoError(t, err)
	assert.Len(t, failed, 2)

	for _, d := range failed {
		assert.Equal(t, events.DeliveryStatusFailed, d.Status)
	}
}
