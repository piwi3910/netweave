package certlifecycle

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/piwi3910/netweave/internal/database"
	"github.com/piwi3910/netweave/internal/database/dbsqlc"
)

// pgUniqueViolation is the PostgreSQL error code for unique constraint violations.
const pgUniqueViolation = "23505"

// PostgresStore implements the Store interface using PostgreSQL as the backend.
// It uses sqlc-generated queries for type-safe database access.
type PostgresStore struct {
	db      *database.DB
	queries *dbsqlc.Queries
}

// NewPostgresStore creates a new PostgresStore backed by the given database connection.
func NewPostgresStore(db *database.DB) *PostgresStore {
	return &PostgresStore{
		db:      db,
		queries: dbsqlc.New(db.Pool),
	}
}

// Close closes the underlying database connection.
func (s *PostgresStore) Close() error {
	s.db.Close()
	return nil
}

// Ping checks PostgreSQL connectivity.
func (s *PostgresStore) Ping(ctx context.Context) error {
	return s.db.Ping(ctx)
}

// Create stores metadata for a newly issued certificate.
func (s *PostgresStore) Create(ctx context.Context, meta *CertificateMetadata) error {
	if meta.SerialNumber == "" {
		return ErrInvalidSerial
	}

	now := time.Now().UTC()
	meta.CreatedAt = now
	meta.UpdatedAt = now

	err := s.queries.CreateCertificateMetadata(ctx, dbsqlc.CreateCertificateMetadataParams{
		SerialNumber: meta.SerialNumber,
		UserID:       meta.UserID,
		TenantID:     meta.TenantID,
		CommonName:   meta.CommonName,
		RoleName:     meta.RoleName,
		Status:       string(meta.Status),
		IssuedAt:     meta.IssuedAt,
		ExpiresAt:    meta.ExpiresAt,
		RenewedFrom:  meta.RenewedFrom,
		RenewedTo:    meta.RenewedTo,
		RenewalCount: safeIntToInt32(meta.RenewalCount),
		LastError:    meta.LastError,
		RetryCount:   safeIntToInt32(meta.RetryCount),
		NextRetryAt:  timeToTimestamptz(meta.NextRetryAt),
		CreatedAt:    meta.CreatedAt,
		UpdatedAt:    meta.UpdatedAt,
	})
	if err != nil {
		if isPgUniqueViolation(err) {
			return ErrCertificateExists
		}
		return fmt.Errorf("failed to create certificate metadata: %w", err)
	}

	return nil
}

// Get retrieves certificate metadata by serial number.
func (s *PostgresStore) Get(ctx context.Context, serialNumber string) (*CertificateMetadata, error) {
	if serialNumber == "" {
		return nil, ErrInvalidSerial
	}

	row, err := s.queries.GetCertificateMetadata(ctx, serialNumber)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCertificateNotFound
		}
		return nil, fmt.Errorf("failed to get certificate metadata: %w", err)
	}

	return dbsqlcCertToModel(&row), nil
}

// UpdateStatus updates the lifecycle status of a certificate.
func (s *PostgresStore) UpdateStatus(ctx context.Context, serialNumber string, status CertificateStatus) error {
	if serialNumber == "" {
		return ErrInvalidSerial
	}

	return s.queries.UpdateCertificateStatus(ctx, dbsqlc.UpdateCertificateStatusParams{
		SerialNumber: serialNumber,
		Status:       string(status),
	})
}

// MarkRevoked marks a certificate as revoked.
func (s *PostgresStore) MarkRevoked(ctx context.Context, serialNumber string) error {
	if serialNumber == "" {
		return ErrInvalidSerial
	}

	return s.queries.UpdateCertificateRevoked(ctx, serialNumber)
}

// MarkRenewed marks a certificate as renewed, linking to the new serial.
func (s *PostgresStore) MarkRenewed(ctx context.Context, serialNumber, newSerial string) error {
	if serialNumber == "" {
		return ErrInvalidSerial
	}

	return s.queries.UpdateCertificateRenewed(ctx, dbsqlc.UpdateCertificateRenewedParams{
		SerialNumber: serialNumber,
		RenewedTo:    newSerial,
	})
}

// MarkRenewalFailed records a renewal failure with error details and next retry time.
func (s *PostgresStore) MarkRenewalFailed(ctx context.Context, serialNumber, errMsg string, nextRetry time.Time) error {
	if serialNumber == "" {
		return ErrInvalidSerial
	}

	return s.queries.UpdateCertificateRenewalFailed(ctx, dbsqlc.UpdateCertificateRenewalFailedParams{
		SerialNumber: serialNumber,
		LastError:    errMsg,
		NextRetryAt:  timeToTimestamptz(nextRetry),
	})
}

// ListByTenant returns all certificates belonging to a tenant.
func (s *PostgresStore) ListByTenant(ctx context.Context, tenantID string) ([]*CertificateMetadata, error) {
	rows, err := s.queries.ListCertificatesByTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to list certificates by tenant: %w", err)
	}

	return dbsqlcCertsToModels(rows), nil
}

// ListByUser returns all certificates belonging to a user.
func (s *PostgresStore) ListByUser(ctx context.Context, userID string) ([]*CertificateMetadata, error) {
	rows, err := s.queries.ListCertificatesByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list certificates by user: %w", err)
	}

	return dbsqlcCertsToModels(rows), nil
}

// ListByStatus returns all certificates with the given status.
func (s *PostgresStore) ListByStatus(ctx context.Context, status CertificateStatus) ([]*CertificateMetadata, error) {
	rows, err := s.queries.ListCertificatesByStatus(ctx, string(status))
	if err != nil {
		return nil, fmt.Errorf("failed to list certificates by status: %w", err)
	}

	return dbsqlcCertsToModels(rows), nil
}

// ListExpiring returns active certificates that expire before the given time.
func (s *PostgresStore) ListExpiring(ctx context.Context, before time.Time) ([]*CertificateMetadata, error) {
	rows, err := s.queries.ListExpiringCertificates(ctx, before)
	if err != nil {
		return nil, fmt.Errorf("failed to list expiring certificates: %w", err)
	}

	return dbsqlcCertsToModels(rows), nil
}

// ListRenewalFailed returns failed certificates eligible for retry.
func (s *PostgresStore) ListRenewalFailed(ctx context.Context, now time.Time) ([]*CertificateMetadata, error) {
	rows, err := s.queries.ListRenewalFailedCertificates(ctx, timeToTimestamptz(now))
	if err != nil {
		return nil, fmt.Errorf("failed to list renewal-failed certificates: %w", err)
	}

	return dbsqlcCertsToModels(rows), nil
}

// CountByStatus returns certificate counts grouped by status.
func (s *PostgresStore) CountByStatus(ctx context.Context) (map[CertificateStatus]int64, error) {
	rows, err := s.queries.CountCertificatesByStatus(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to count certificates by status: %w", err)
	}

	result := make(map[CertificateStatus]int64, len(rows))
	for _, row := range rows {
		result[CertificateStatus(row.Status)] = row.Count
	}

	return result, nil
}

// Delete removes certificate metadata by serial number.
func (s *PostgresStore) Delete(ctx context.Context, serialNumber string) error {
	if serialNumber == "" {
		return ErrInvalidSerial
	}

	rowsAffected, err := s.queries.DeleteCertificateMetadata(ctx, serialNumber)
	if err != nil {
		return fmt.Errorf("failed to delete certificate metadata: %w", err)
	}
	if rowsAffected == 0 {
		return ErrCertificateNotFound
	}

	return nil
}

// --- Conversion helpers ---

// dbsqlcCertToModel converts a sqlc CertificateMetadatum to a CertificateMetadata.
func dbsqlcCertToModel(row *dbsqlc.CertificateMetadatum) *CertificateMetadata {
	meta := &CertificateMetadata{
		SerialNumber: row.SerialNumber,
		UserID:       row.UserID,
		TenantID:     row.TenantID,
		CommonName:   row.CommonName,
		RoleName:     row.RoleName,
		Status:       CertificateStatus(row.Status),
		IssuedAt:     row.IssuedAt,
		ExpiresAt:    row.ExpiresAt,
		RenewedFrom:  row.RenewedFrom,
		RenewedTo:    row.RenewedTo,
		RenewalCount: int(row.RenewalCount),
		LastError:    row.LastError,
		RetryCount:   int(row.RetryCount),
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}

	if row.RevokedAt.Valid {
		meta.RevokedAt = row.RevokedAt.Time
	}
	if row.RenewedAt.Valid {
		meta.RenewedAt = row.RenewedAt.Time
	}
	if row.NextRetryAt.Valid {
		meta.NextRetryAt = row.NextRetryAt.Time
	}

	return meta
}

// dbsqlcCertsToModels converts a slice of sqlc CertificateMetadatum to CertificateMetadata pointers.
func dbsqlcCertsToModels(rows []dbsqlc.CertificateMetadatum) []*CertificateMetadata {
	result := make([]*CertificateMetadata, 0, len(rows))
	for i := range rows {
		result = append(result, dbsqlcCertToModel(&rows[i]))
	}
	return result
}

// timeToTimestamptz converts a time.Time to pgtype.Timestamptz.
// Zero time results in an invalid (NULL) timestamp.
func timeToTimestamptz(t time.Time) pgtype.Timestamptz {
	if t.IsZero() {
		return pgtype.Timestamptz{Valid: false}
	}
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// safeIntToInt32 converts int to int32 with clamping to prevent overflow.
func safeIntToInt32(v int) int32 {
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	if v < math.MinInt32 {
		return math.MinInt32
	}
	return int32(v)
}

// isPgUniqueViolation checks if the error is a PostgreSQL unique constraint violation.
func isPgUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == pgUniqueViolation
	}
	return false
}
