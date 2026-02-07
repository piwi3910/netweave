package events

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/piwi3910/netweave/internal/database"
	"github.com/piwi3910/netweave/internal/database/dbsqlc"
)

// PostgresDeliveryTracker implements the DeliveryTracker interface using PostgreSQL.
// It uses sqlc-generated queries for type-safe database access.
//
// Example:
//
//	db, _ := database.New(ctx, cfg, password)
//	tracker := NewPostgresDeliveryTracker(db)
type PostgresDeliveryTracker struct {
	db      *database.DB
	queries *dbsqlc.Queries
}

// NewPostgresDeliveryTracker creates a new PostgresDeliveryTracker backed by the given database.
func NewPostgresDeliveryTracker(db *database.DB) *PostgresDeliveryTracker {
	return &PostgresDeliveryTracker{
		db:      db,
		queries: dbsqlc.New(db.Pool),
	}
}

// Track records a delivery attempt.
// Uses UPSERT to allow tracking updates for the same delivery ID.
func (t *PostgresDeliveryTracker) Track(ctx context.Context, delivery *NotificationDelivery) error {
	if delivery == nil {
		return errors.New("delivery cannot be nil")
	}
	if delivery.ID == "" {
		return errors.New("delivery ID cannot be empty")
	}

	if delivery.CreatedAt.IsZero() {
		delivery.CreatedAt = time.Now().UTC()
	}

	err := t.queries.TrackDelivery(ctx, dbsqlc.TrackDeliveryParams{
		ID:             delivery.ID,
		EventID:        delivery.EventID,
		SubscriptionID: delivery.SubscriptionID,
		CallbackUrl:    delivery.CallbackURL,
		Status:         string(delivery.Status),
		Attempts:       safeIntToInt32(delivery.Attempts),
		MaxAttempts:    safeIntToInt32(delivery.MaxAttempts),
		LastAttemptAt:  timeToTimestamptz(delivery.LastAttemptAt),
		NextAttemptAt:  timeToTimestamptz(delivery.NextAttemptAt),
		LastError:      delivery.LastError,
		HttpStatusCode: safeIntToInt32(delivery.HTTPStatusCode),
		ResponseTime:   delivery.ResponseTime,
		CreatedAt:      delivery.CreatedAt,
		CompletedAt:    timeToTimestamptz(delivery.CompletedAt),
	})
	if err != nil {
		return fmt.Errorf("failed to track delivery: %w", err)
	}

	return nil
}

// Get retrieves delivery information by ID.
func (t *PostgresDeliveryTracker) Get(ctx context.Context, deliveryID string) (*NotificationDelivery, error) {
	if deliveryID == "" {
		return nil, errors.New("delivery ID cannot be empty")
	}

	row, err := t.queries.GetDelivery(ctx, deliveryID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("delivery not found: %w", err)
		}
		return nil, fmt.Errorf("failed to get delivery: %w", err)
	}

	return dbsqlcDeliveryToModel(&row), nil
}

// ListByEvent retrieves all deliveries for a specific event.
func (t *PostgresDeliveryTracker) ListByEvent(ctx context.Context, eventID string) ([]*NotificationDelivery, error) {
	if eventID == "" {
		return nil, errors.New("event ID cannot be empty")
	}

	rows, err := t.queries.ListDeliveriesByEvent(ctx, eventID)
	if err != nil {
		return nil, fmt.Errorf("failed to list deliveries by event: %w", err)
	}

	return dbsqlcDeliveriesToModels(rows), nil
}

// ListBySubscription retrieves all deliveries for a specific subscription.
func (t *PostgresDeliveryTracker) ListBySubscription(
	ctx context.Context,
	subscriptionID string,
) ([]*NotificationDelivery, error) {
	if subscriptionID == "" {
		return nil, errors.New("subscription ID cannot be empty")
	}

	rows, err := t.queries.ListDeliveriesBySubscription(ctx, subscriptionID)
	if err != nil {
		return nil, fmt.Errorf("failed to list deliveries by subscription: %w", err)
	}

	return dbsqlcDeliveriesToModels(rows), nil
}

// ListFailed retrieves all failed deliveries that need attention.
func (t *PostgresDeliveryTracker) ListFailed(ctx context.Context) ([]*NotificationDelivery, error) {
	rows, err := t.queries.ListFailedDeliveries(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list failed deliveries: %w", err)
	}

	return dbsqlcDeliveriesToModels(rows), nil
}

// --- Conversion helpers ---

// dbsqlcDeliveryToModel converts a sqlc NotificationDelivery to an events NotificationDelivery.
func dbsqlcDeliveryToModel(row *dbsqlc.NotificationDelivery) *NotificationDelivery {
	d := &NotificationDelivery{
		ID:             row.ID,
		EventID:        row.EventID,
		SubscriptionID: row.SubscriptionID,
		CallbackURL:    row.CallbackUrl,
		Status:         DeliveryStatus(row.Status),
		Attempts:       int(row.Attempts),
		MaxAttempts:    int(row.MaxAttempts),
		LastError:      row.LastError,
		HTTPStatusCode: int(row.HttpStatusCode),
		ResponseTime:   row.ResponseTime,
		CreatedAt:      row.CreatedAt,
	}
	if row.LastAttemptAt.Valid {
		d.LastAttemptAt = row.LastAttemptAt.Time
	}
	if row.NextAttemptAt.Valid {
		d.NextAttemptAt = row.NextAttemptAt.Time
	}
	if row.CompletedAt.Valid {
		d.CompletedAt = row.CompletedAt.Time
	}
	return d
}

// dbsqlcDeliveriesToModels converts a slice of sqlc NotificationDeliveries to events models.
func dbsqlcDeliveriesToModels(rows []dbsqlc.NotificationDelivery) []*NotificationDelivery {
	deliveries := make([]*NotificationDelivery, 0, len(rows))
	for i := range rows {
		deliveries = append(deliveries, dbsqlcDeliveryToModel(&rows[i]))
	}
	return deliveries
}

// safeIntToInt32 safely converts int to int32 with clamping at math.MaxInt32.
func safeIntToInt32(v int) int32 {
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	if v < math.MinInt32 {
		return math.MinInt32
	}
	return int32(v)
}

// timeToTimestamptz converts a time.Time to pgtype.Timestamptz.
// Zero time results in an invalid (NULL) timestamp.
func timeToTimestamptz(t time.Time) pgtype.Timestamptz {
	if t.IsZero() {
		return pgtype.Timestamptz{Valid: false}
	}
	return pgtype.Timestamptz{Time: t, Valid: true}
}
