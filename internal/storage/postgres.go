package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/piwi3910/netweave/internal/database"
	"github.com/piwi3910/netweave/internal/database/dbsqlc"
)

// pgUniqueViolation is the PostgreSQL error code for unique constraint violations.
const pgUniqueViolation = "23505"

// PostgresStore implements the Store interface using PostgreSQL as the backend.
// It uses sqlc-generated queries for type-safe database access.
//
// Example:
//
//	db, _ := database.New(ctx, cfg, password)
//	store := NewPostgresStore(db, true)
//	defer store.Close()
//
//	sub := &Subscription{ID: "sub-123", Callback: "https://smo.example.com/notify"}
//	err := store.Create(ctx, sub)
type PostgresStore struct {
	db                     *database.DB
	queries                *dbsqlc.Queries
	allowInsecureCallbacks bool
}

// NewPostgresStore creates a new PostgresStore backed by the given database connection.
// The allowInsecureCallbacks parameter controls whether HTTP callbacks are permitted.
func NewPostgresStore(db *database.DB, allowInsecureCallbacks bool) *PostgresStore {
	return &PostgresStore{
		db:                     db,
		queries:                dbsqlc.New(db.Pool),
		allowInsecureCallbacks: allowInsecureCallbacks,
	}
}

// Create creates a new subscription in PostgreSQL.
// Returns ErrSubscriptionExists if a subscription with the same ID already exists.
// Returns ErrInvalidCallback if the callback URL is invalid.
// Returns ErrInvalidID if the subscription ID is empty.
func (s *PostgresStore) Create(ctx context.Context, sub *Subscription) error {
	if sub.ID == "" {
		return ErrInvalidID
	}
	if err := ValidateCallbackURL(sub.Callback, s.allowInsecureCallbacks); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidCallback, err.Error())
	}

	now := time.Now().UTC()
	sub.CreatedAt = now
	sub.UpdatedAt = now

	err := s.queries.CreateSubscription(ctx, dbsqlc.CreateSubscriptionParams{
		ID:                     sub.ID,
		TenantID:               sub.TenantID,
		Callback:               sub.Callback,
		ConsumerSubscriptionID: sub.ConsumerSubscriptionID,
		FilterResourcePoolID:   sub.Filter.ResourcePoolID,
		FilterResourceTypeID:   sub.Filter.ResourceTypeID,
		FilterResourceID:       sub.Filter.ResourceID,
		CreatedAt:              sub.CreatedAt,
		UpdatedAt:              sub.UpdatedAt,
	})
	if err != nil {
		if isPgUniqueViolation(err) {
			return ErrSubscriptionExists
		}
		return fmt.Errorf("failed to create subscription: %w", err)
	}

	return nil
}

// Get retrieves a subscription by ID.
// Returns ErrSubscriptionNotFound if the subscription does not exist.
func (s *PostgresStore) Get(ctx context.Context, id string) (*Subscription, error) {
	if id == "" {
		return nil, ErrInvalidID
	}

	row, err := s.queries.GetSubscription(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSubscriptionNotFound
		}
		return nil, fmt.Errorf("failed to get subscription: %w", err)
	}

	return dbsqlcSubscriptionToModel(&row), nil
}

// Update updates an existing subscription.
// Returns ErrSubscriptionNotFound if the subscription does not exist.
// Returns ErrInvalidCallback if the callback URL is invalid.
func (s *PostgresStore) Update(ctx context.Context, sub *Subscription) error {
	if sub.ID == "" {
		return ErrInvalidID
	}
	if err := ValidateCallbackURL(sub.Callback, s.allowInsecureCallbacks); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidCallback, err.Error())
	}

	exists, err := s.queries.SubscriptionExists(ctx, sub.ID)
	if err != nil {
		return fmt.Errorf("failed to check subscription existence: %w", err)
	}
	if !exists {
		return ErrSubscriptionNotFound
	}

	sub.UpdatedAt = time.Now().UTC()

	err = s.queries.UpdateSubscription(ctx, dbsqlc.UpdateSubscriptionParams{
		ID:                     sub.ID,
		Callback:               sub.Callback,
		ConsumerSubscriptionID: sub.ConsumerSubscriptionID,
		FilterResourcePoolID:   sub.Filter.ResourcePoolID,
		FilterResourceTypeID:   sub.Filter.ResourceTypeID,
		FilterResourceID:       sub.Filter.ResourceID,
		UpdatedAt:              sub.UpdatedAt,
	})
	if err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	return nil
}

// Delete deletes a subscription by ID.
// Returns ErrSubscriptionNotFound if the subscription does not exist.
func (s *PostgresStore) Delete(ctx context.Context, id string) error {
	if id == "" {
		return ErrInvalidID
	}

	rowsAffected, err := s.queries.DeleteSubscription(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to delete subscription: %w", err)
	}
	if rowsAffected == 0 {
		return ErrSubscriptionNotFound
	}

	return nil
}

// List retrieves all subscriptions.
// Returns an empty slice if no subscriptions exist.
func (s *PostgresStore) List(ctx context.Context) ([]*Subscription, error) {
	rows, err := s.queries.ListSubscriptions(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list subscriptions: %w", err)
	}

	return dbsqlcSubscriptionsToModels(rows), nil
}

// ListByResourcePool retrieves subscriptions filtered by resource pool ID.
// Returns an empty slice if no matching subscriptions exist.
func (s *PostgresStore) ListByResourcePool(ctx context.Context, resourcePoolID string) ([]*Subscription, error) {
	if resourcePoolID == "" {
		return []*Subscription{}, nil
	}

	rows, err := s.queries.ListSubscriptionsByResourcePool(ctx, resourcePoolID)
	if err != nil {
		return nil, fmt.Errorf("failed to list subscriptions by resource pool: %w", err)
	}

	return dbsqlcSubscriptionsToModels(rows), nil
}

// ListByResourceType retrieves subscriptions filtered by resource type ID.
// Returns an empty slice if no matching subscriptions exist.
func (s *PostgresStore) ListByResourceType(ctx context.Context, resourceTypeID string) ([]*Subscription, error) {
	if resourceTypeID == "" {
		return []*Subscription{}, nil
	}

	rows, err := s.queries.ListSubscriptionsByResourceType(ctx, resourceTypeID)
	if err != nil {
		return nil, fmt.Errorf("failed to list subscriptions by resource type: %w", err)
	}

	return dbsqlcSubscriptionsToModels(rows), nil
}

// ListByTenant retrieves subscriptions filtered by tenant ID.
// Returns an empty slice if no matching subscriptions exist.
func (s *PostgresStore) ListByTenant(ctx context.Context, tenantID string) ([]*Subscription, error) {
	if tenantID == "" {
		return []*Subscription{}, nil
	}

	rows, err := s.queries.ListSubscriptionsByTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to list subscriptions by tenant: %w", err)
	}

	return dbsqlcSubscriptionsToModels(rows), nil
}

// Close closes the PostgreSQL connection pool and releases resources.
func (s *PostgresStore) Close() error {
	s.db.Close()
	return nil
}

// Ping checks if PostgreSQL is available.
// Returns ErrStorageUnavailable if the database cannot be reached.
func (s *PostgresStore) Ping(ctx context.Context) error {
	if err := s.db.Ping(ctx); err != nil {
		return fmt.Errorf("%w: %s", ErrStorageUnavailable, err.Error())
	}
	return nil
}

// dbsqlcSubscriptionToModel converts a sqlc Subscription to a storage Subscription.
func dbsqlcSubscriptionToModel(row *dbsqlc.Subscription) *Subscription {
	return &Subscription{
		ID:                     row.ID,
		TenantID:               row.TenantID,
		Callback:               row.Callback,
		ConsumerSubscriptionID: row.ConsumerSubscriptionID,
		Filter: SubscriptionFilter{
			ResourcePoolID: row.FilterResourcePoolID,
			ResourceTypeID: row.FilterResourceTypeID,
			ResourceID:     row.FilterResourceID,
		},
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

// dbsqlcSubscriptionsToModels converts a slice of sqlc Subscriptions to storage Subscriptions.
func dbsqlcSubscriptionsToModels(rows []dbsqlc.Subscription) []*Subscription {
	subs := make([]*Subscription, 0, len(rows))
	for i := range rows {
		subs = append(subs, dbsqlcSubscriptionToModel(&rows[i]))
	}
	return subs
}

// isPgUniqueViolation checks if the error is a PostgreSQL unique constraint violation.
func isPgUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == pgUniqueViolation
	}
	return false
}
