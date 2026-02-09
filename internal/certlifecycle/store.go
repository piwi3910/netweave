// Package certlifecycle provides certificate lifecycle automation on top of Vault PKI.
// It includes metadata storage, expiry monitoring, automatic renewal, webhook
// notifications, Prometheus metrics, and optional Keycloak attribute sync.
package certlifecycle

import (
	"context"
	"errors"
	"time"
)

// CertificateStatus represents the lifecycle status of a certificate.
type CertificateStatus string

const (
	// StatusActive indicates a currently valid certificate.
	StatusActive CertificateStatus = "active"

	// StatusExpiringSoon indicates a certificate that will expire within the renewal window.
	StatusExpiringSoon CertificateStatus = "expiring_soon"

	// StatusExpired indicates a certificate that has passed its expiration time.
	StatusExpired CertificateStatus = "expired"

	// StatusRevoked indicates a certificate that has been explicitly revoked.
	StatusRevoked CertificateStatus = "revoked"

	// StatusRenewed indicates a certificate that has been replaced by a newer one.
	StatusRenewed CertificateStatus = "renewed"

	// StatusRenewalFailed indicates a certificate whose automatic renewal attempt failed.
	StatusRenewalFailed CertificateStatus = "renewal_failed"
)

// Sentinel errors for certificate store operations.
var (
	ErrCertificateNotFound = errors.New("certificate not found")
	ErrCertificateExists   = errors.New("certificate already exists")
	ErrInvalidSerial       = errors.New("serial number is required")
)

// CertificateMetadata holds metadata about a certificate's lifecycle.
type CertificateMetadata struct {
	SerialNumber string
	UserID       string
	TenantID     string
	CommonName   string
	RoleName     string
	Status       CertificateStatus
	IssuedAt     time.Time
	ExpiresAt    time.Time
	RevokedAt    time.Time
	RenewedAt    time.Time
	RenewedFrom  string
	RenewedTo    string
	RenewalCount int
	LastError    string
	RetryCount   int
	NextRetryAt  time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// CertificateWriter defines write operations for certificate metadata.
type CertificateWriter interface {
	// Create stores metadata for a newly issued certificate.
	// Returns ErrCertificateExists if a record with the same serial number exists.
	Create(ctx context.Context, meta *CertificateMetadata) error

	// UpdateStatus updates the lifecycle status of a certificate.
	UpdateStatus(ctx context.Context, serialNumber string, status CertificateStatus) error

	// MarkRevoked marks a certificate as revoked with the current timestamp.
	MarkRevoked(ctx context.Context, serialNumber string) error

	// MarkRenewed marks a certificate as renewed, linking it to the new serial number.
	MarkRenewed(ctx context.Context, serialNumber, newSerial string) error

	// MarkRenewalFailed records a renewal failure with error details and next retry time.
	MarkRenewalFailed(ctx context.Context, serialNumber, errMsg string, nextRetry time.Time) error

	// Delete removes certificate metadata by serial number.
	// Returns ErrCertificateNotFound if no matching record exists.
	Delete(ctx context.Context, serialNumber string) error
}

// CertificateReader defines read operations for certificate metadata.
type CertificateReader interface {
	// Get retrieves certificate metadata by serial number.
	// Returns ErrCertificateNotFound if no matching record exists.
	Get(ctx context.Context, serialNumber string) (*CertificateMetadata, error)

	// ListByTenant returns all certificates belonging to a tenant.
	ListByTenant(ctx context.Context, tenantID string) ([]*CertificateMetadata, error)

	// ListByUser returns all certificates belonging to a user.
	ListByUser(ctx context.Context, userID string) ([]*CertificateMetadata, error)

	// ListByStatus returns all certificates with the given status.
	ListByStatus(ctx context.Context, status CertificateStatus) ([]*CertificateMetadata, error)

	// ListExpiring returns active certificates that expire before the given time.
	ListExpiring(ctx context.Context, before time.Time) ([]*CertificateMetadata, error)

	// ListRenewalFailed returns failed certificates eligible for retry (next_retry_at <= now).
	ListRenewalFailed(ctx context.Context, now time.Time) ([]*CertificateMetadata, error)

	// CountByStatus returns certificate counts grouped by status.
	CountByStatus(ctx context.Context) (map[CertificateStatus]int64, error)
}

// Store defines the interface for certificate metadata persistence.
type Store interface {
	CertificateWriter
	CertificateReader

	// Close releases any resources held by the store.
	Close() error

	// Ping checks store connectivity.
	Ping(ctx context.Context) error
}
