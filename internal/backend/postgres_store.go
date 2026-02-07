package backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/piwi3910/netweave/internal/database/dbsqlc"
)

// pgUniqueViolation is the PostgreSQL error code for unique constraint violations.
const pgUniqueViolation = "23505"

// PostgresStore implements Store using PostgreSQL via sqlc-generated queries.
type PostgresStore struct {
	pool    *pgxpool.Pool
	queries *dbsqlc.Queries
}

// NewPostgresStore creates a new PostgresStore backed by the given connection pool.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{
		pool:    pool,
		queries: dbsqlc.New(pool),
	}
}

// CreateBackend creates a new backend instance.
func (s *PostgresStore) CreateBackend(ctx context.Context, instance *Instance) error {
	params := dbsqlc.CreateBackendParams{
		ID:              instance.ID,
		Name:            instance.Name,
		Category:        instance.Category,
		AdapterType:     instance.AdapterType,
		Description:     instance.Description,
		Config:          instance.Config,
		Credentials:     instance.Credentials,
		Status:          instance.Status,
		StatusMessage:   instance.StatusMessage,
		LastHealthCheck: timestamptzFromTime(instance.LastHealthCheck),
		CreatedAt:       instance.CreatedAt,
		UpdatedAt:       instance.UpdatedAt,
	}

	if err := s.queries.CreateBackend(ctx, params); err != nil {
		if isUniqueViolation(err) {
			return ErrBackendExists
		}
		return fmt.Errorf("failed to create backend: %w", err)
	}
	return nil
}

// GetBackend retrieves a backend instance by ID.
func (s *PostgresStore) GetBackend(ctx context.Context, id string) (*Instance, error) {
	row, err := s.queries.GetBackend(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrBackendNotFound
		}
		return nil, fmt.Errorf("failed to get backend: %w", err)
	}
	return dbBackendToModel(&row), nil
}

// UpdateBackend updates an existing backend instance.
func (s *PostgresStore) UpdateBackend(ctx context.Context, instance *Instance) error {
	exists, err := s.queries.BackendExists(ctx, instance.ID)
	if err != nil {
		return fmt.Errorf("failed to check backend existence: %w", err)
	}
	if !exists {
		return ErrBackendNotFound
	}

	params := dbsqlc.UpdateBackendParams{
		ID:              instance.ID,
		Name:            instance.Name,
		Description:     instance.Description,
		Config:          instance.Config,
		Credentials:     instance.Credentials,
		Status:          instance.Status,
		StatusMessage:   instance.StatusMessage,
		LastHealthCheck: timestamptzFromTime(instance.LastHealthCheck),
		UpdatedAt:       instance.UpdatedAt,
	}

	if err := s.queries.UpdateBackend(ctx, params); err != nil {
		return fmt.Errorf("failed to update backend: %w", err)
	}
	return nil
}

// DeleteBackend deletes a backend instance by ID.
func (s *PostgresStore) DeleteBackend(ctx context.Context, id string) error {
	rows, err := s.queries.DeleteBackend(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to delete backend: %w", err)
	}
	if rows == 0 {
		return ErrBackendNotFound
	}
	return nil
}

// ListBackends retrieves all backend instances.
func (s *PostgresStore) ListBackends(ctx context.Context) ([]*Instance, error) {
	rows, err := s.queries.ListBackends(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list backends: %w", err)
	}
	return dbBackendsToModels(rows), nil
}

// ListBackendsByCategory retrieves backend instances filtered by category.
func (s *PostgresStore) ListBackendsByCategory(ctx context.Context, category string) ([]*Instance, error) {
	rows, err := s.queries.ListBackendsByCategory(ctx, category)
	if err != nil {
		return nil, fmt.Errorf("failed to list backends by category: %w", err)
	}
	return dbBackendsToModels(rows), nil
}

// UpdateBackendStatus updates the status fields of a backend instance.
func (s *PostgresStore) UpdateBackendStatus(ctx context.Context, id, status, message string) error {
	now := time.Now().UTC()
	params := dbsqlc.UpdateBackendStatusParams{
		ID:            id,
		Status:        status,
		StatusMessage: message,
		LastHealthCheck: pgtype.Timestamptz{
			Time:  now,
			Valid: true,
		},
		UpdatedAt: now,
	}

	if err := s.queries.UpdateBackendStatus(ctx, params); err != nil {
		return fmt.Errorf("failed to update backend status: %w", err)
	}
	return nil
}

// CreateBackendLink creates a link between an IMS and DMS backend.
func (s *PostgresStore) CreateBackendLink(ctx context.Context, imsID, dmsID string) error {
	params := dbsqlc.CreateBackendLinkParams{
		ImsID:     imsID,
		DmsID:     dmsID,
		CreatedAt: time.Now().UTC(),
	}

	if err := s.queries.CreateBackendLink(ctx, params); err != nil {
		if isUniqueViolation(err) {
			return ErrBackendLinkExists
		}
		return fmt.Errorf("failed to create backend link: %w", err)
	}
	return nil
}

// DeleteBackendLink removes a link between an IMS and DMS backend.
func (s *PostgresStore) DeleteBackendLink(ctx context.Context, imsID, dmsID string) error {
	params := dbsqlc.DeleteBackendLinkParams{
		ImsID: imsID,
		DmsID: dmsID,
	}

	rows, err := s.queries.DeleteBackendLink(ctx, params)
	if err != nil {
		return fmt.Errorf("failed to delete backend link: %w", err)
	}
	if rows == 0 {
		return ErrBackendLinkNotFound
	}
	return nil
}

// ListDMSLinksByIMS retrieves all DMS backends linked to an IMS backend.
func (s *PostgresStore) ListDMSLinksByIMS(ctx context.Context, imsID string) ([]*Instance, error) {
	rows, err := s.queries.ListDMSLinksByIMS(ctx, imsID)
	if err != nil {
		return nil, fmt.Errorf("failed to list DMS links: %w", err)
	}
	return dbBackendsToModels(rows), nil
}

// ListIMSLinksByDMS retrieves all IMS backends linked to a DMS backend.
func (s *PostgresStore) ListIMSLinksByDMS(ctx context.Context, dmsID string) ([]*Instance, error) {
	rows, err := s.queries.ListIMSLinksByDMS(ctx, dmsID)
	if err != nil {
		return nil, fmt.Errorf("failed to list IMS links: %w", err)
	}
	return dbBackendsToModels(rows), nil
}

// CreateAccess creates a new backend access record.
func (s *PostgresStore) CreateBackendAccess(ctx context.Context, access *Access) error {
	permJSON, err := json.Marshal(access.Permissions)
	if err != nil {
		return fmt.Errorf("failed to marshal permissions: %w", err)
	}

	constraintsJSON, err := json.Marshal(access.Constraints)
	if err != nil {
		return fmt.Errorf("failed to marshal constraints: %w", err)
	}

	params := dbsqlc.CreateBackendAccessParams{
		ID:          access.ID,
		TenantID:    access.TenantID,
		BackendID:   access.BackendID,
		Permissions: permJSON,
		Constraints: constraintsJSON,
		GrantedAt:   access.GrantedAt,
		GrantedBy:   access.GrantedBy,
	}

	if err := s.queries.CreateBackendAccess(ctx, params); err != nil {
		if isUniqueViolation(err) {
			return ErrAccessExists
		}
		return fmt.Errorf("failed to create backend access: %w", err)
	}
	return nil
}

// GetAccess retrieves a backend access record by ID.
func (s *PostgresStore) GetBackendAccess(ctx context.Context, id string) (*Access, error) {
	row, err := s.queries.GetBackendAccess(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAccessNotFound
		}
		return nil, fmt.Errorf("failed to get backend access: %w", err)
	}
	return dbAccessToModel(&row)
}

// UpdateAccess updates an existing backend access record.
func (s *PostgresStore) UpdateBackendAccess(ctx context.Context, access *Access) error {
	permJSON, err := json.Marshal(access.Permissions)
	if err != nil {
		return fmt.Errorf("failed to marshal permissions: %w", err)
	}

	constraintsJSON, err := json.Marshal(access.Constraints)
	if err != nil {
		return fmt.Errorf("failed to marshal constraints: %w", err)
	}

	params := dbsqlc.UpdateBackendAccessParams{
		ID:          access.ID,
		Permissions: permJSON,
		Constraints: constraintsJSON,
	}

	if err := s.queries.UpdateBackendAccess(ctx, params); err != nil {
		return fmt.Errorf("failed to update backend access: %w", err)
	}
	return nil
}

// DeleteAccess deletes a backend access record by ID.
func (s *PostgresStore) DeleteBackendAccess(ctx context.Context, id string) error {
	rows, err := s.queries.DeleteBackendAccess(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to delete backend access: %w", err)
	}
	if rows == 0 {
		return ErrAccessNotFound
	}
	return nil
}

// ListAccessByTenant retrieves all access records for a tenant.
func (s *PostgresStore) ListBackendAccessByTenant(ctx context.Context, tenantID string) ([]*Access, error) {
	rows, err := s.queries.ListBackendAccessByTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to list backend access by tenant: %w", err)
	}
	return dbAccessListToModels(rows)
}

// ListAccessByBackend retrieves all access records for a backend.
func (s *PostgresStore) ListBackendAccessByBackend(ctx context.Context, backendID string) ([]*Access, error) {
	rows, err := s.queries.ListBackendAccessByBackend(ctx, backendID)
	if err != nil {
		return nil, fmt.Errorf("failed to list backend access by backend: %w", err)
	}
	return dbAccessListToModels(rows)
}

// Close closes the connection pool.
func (s *PostgresStore) Close() error {
	s.pool.Close()
	return nil
}

// Ping checks if the database is reachable.
func (s *PostgresStore) Ping(ctx context.Context) error {
	if err := s.pool.Ping(ctx); err != nil {
		return fmt.Errorf("postgres ping failed: %w", err)
	}
	return nil
}

// isUniqueViolation checks if the error is a PostgreSQL unique constraint violation.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == pgUniqueViolation
	}
	return false
}

// timestamptzFromTime converts a time.Time to pgtype.Timestamptz.
func timestamptzFromTime(t time.Time) pgtype.Timestamptz {
	if t.IsZero() {
		return pgtype.Timestamptz{Valid: false}
	}
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// timeFromTimestamptz converts a pgtype.Timestamptz to time.Time.
func timeFromTimestamptz(ts pgtype.Timestamptz) time.Time {
	if !ts.Valid {
		return time.Time{}
	}
	return ts.Time
}

// dbBackendToModel converts a dbsqlc.BackendInstance to a backend.Instance.
func dbBackendToModel(row *dbsqlc.BackendInstance) *Instance {
	return &Instance{
		ID:              row.ID,
		Name:            row.Name,
		Category:        row.Category,
		AdapterType:     row.AdapterType,
		Description:     row.Description,
		Config:          row.Config,
		Credentials:     row.Credentials,
		Status:          row.Status,
		StatusMessage:   row.StatusMessage,
		LastHealthCheck: timeFromTimestamptz(row.LastHealthCheck),
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}
}

// dbBackendsToModels converts a slice of dbsqlc.BackendInstance to backend.Instance models.
func dbBackendsToModels(rows []dbsqlc.BackendInstance) []*Instance {
	result := make([]*Instance, 0, len(rows))
	for i := range rows {
		result = append(result, dbBackendToModel(&rows[i]))
	}
	return result
}

// dbAccessToModel converts a dbsqlc.BackendAccess to a backend.Access.
func dbAccessToModel(row *dbsqlc.BackendAccess) (*Access, error) {
	var perms []string
	if err := json.Unmarshal(row.Permissions, &perms); err != nil {
		return nil, fmt.Errorf("failed to unmarshal permissions: %w", err)
	}

	var constraints map[string]interface{}
	if err := json.Unmarshal(row.Constraints, &constraints); err != nil {
		return nil, fmt.Errorf("failed to unmarshal constraints: %w", err)
	}

	return &Access{
		ID:          row.ID,
		TenantID:    row.TenantID,
		BackendID:   row.BackendID,
		Permissions: perms,
		Constraints: constraints,
		GrantedAt:   row.GrantedAt,
		GrantedBy:   row.GrantedBy,
	}, nil
}

// dbAccessListToModels converts a slice of dbsqlc.BackendAccess to backend.Access models.
func dbAccessListToModels(rows []dbsqlc.BackendAccess) ([]*Access, error) {
	result := make([]*Access, 0, len(rows))
	for i := range rows {
		access, err := dbAccessToModel(&rows[i])
		if err != nil {
			return nil, err
		}
		result = append(result, access)
	}
	return result, nil
}
