// Package certmanager provides automated certificate lifecycle management using Vault PKI.
// It handles certificate issuance, renewal, revocation, and expiry monitoring for mTLS authentication.
package certmanager

import (
	"time"
)

const (
	// DefaultCertificateTTL is the default certificate lifetime when not specified.
	DefaultCertificateTTL = "8760h" // 1 year
)

// Certificate represents a managed certificate in the system.
type Certificate struct {
	// SerialNumber is the unique certificate serial number from Vault.
	SerialNumber string `json:"serial_number"`

	// CommonName is the CN (Common Name) from the certificate subject.
	CommonName string `json:"common_name"`

	// UserID is the Keycloak user ID associated with this certificate.
	UserID string `json:"user_id"`

	// TenantID is the tenant this certificate belongs to.
	TenantID string `json:"tenant_id"`

	// CertificatePEM is the PEM-encoded certificate.
	CertificatePEM string `json:"certificate_pem"`

	// PrivateKeyPEM is the PEM-encoded private key (only returned on issuance).
	PrivateKeyPEM string `json:"private_key_pem,omitempty"`

	// IssuingCA is the issuing CA certificate in PEM format.
	IssuingCA string `json:"issuing_ca"`

	// CAChain is the full certificate chain.
	CAChain []string `json:"ca_chain"`

	// IssuedAt is when the certificate was issued.
	IssuedAt time.Time `json:"issued_at"`

	// ExpiresAt is when the certificate expires.
	ExpiresAt time.Time `json:"expires_at"`

	// TTL is the original time-to-live duration for the certificate.
	TTL string `json:"ttl,omitempty"`

	// RevokedAt is when the certificate was revoked (if applicable).
	RevokedAt *time.Time `json:"revoked_at,omitempty"`

	// Status is the current certificate status.
	Status CertificateStatus `json:"status"`

	// RenewalAttempts tracks how many times renewal has been attempted.
	RenewalAttempts int `json:"renewal_attempts"`

	// LastRenewalAttempt is the timestamp of the last renewal attempt.
	LastRenewalAttempt *time.Time `json:"last_renewal_attempt,omitempty"`
}

// CertificateStatus represents the status of a certificate.
type CertificateStatus string

const (
	// CertStatusActive indicates the certificate is valid and active.
	CertStatusActive CertificateStatus = "active"

	// CertStatusExpiringSoon indicates the certificate will expire within the renewal window.
	CertStatusExpiringSoon CertificateStatus = "expiring_soon"

	// CertStatusExpired indicates the certificate has expired.
	CertStatusExpired CertificateStatus = "expired"

	// CertStatusRevoked indicates the certificate has been revoked.
	CertStatusRevoked CertificateStatus = "revoked"

	// CertStatusRenewalPending indicates automatic renewal is scheduled.
	CertStatusRenewalPending CertificateStatus = "renewal_pending"

	// CertStatusRenewalFailed indicates automatic renewal failed.
	CertStatusRenewalFailed CertificateStatus = "renewal_failed"
)

// CertificateRequest represents a request to issue a new certificate.
type CertificateRequest struct {
	// UserID is the Keycloak user ID for whom the certificate is being issued.
	UserID string `json:"user_id"`

	// TenantID is the tenant ID for the certificate.
	TenantID string `json:"tenant_id"`

	// CommonName is the CN for the certificate (typically user@tenant).
	CommonName string `json:"common_name"`

	// AltNames are additional DNS names for the certificate.
	AltNames []string `json:"alt_names,omitempty"`

	// IPSANs are IP Subject Alternative Names.
	IPSANs []string `json:"ip_sans,omitempty"`

	// TTL is the requested certificate lifetime (e.g., "8760h" for 1 year).
	TTL string `json:"ttl,omitempty"`
}

// RenewalPolicy defines when and how certificates should be renewed.
type RenewalPolicy struct {
	// RenewalWindow is how long before expiry to trigger renewal (default: 30 days).
	RenewalWindow time.Duration

	// MaxRetries is the maximum number of renewal attempts (default: 3).
	MaxRetries int

	// RetryInterval is the time between renewal retries (default: 1 hour).
	RetryInterval time.Duration

	// TODO(#298): Add notification support for renewal events
	// - NotifyAdmins bool   // Send notifications to administrators
	// - NotifyUser bool     // Send notifications to certificate owner
	// - NotificationConfig  // Email/webhook configuration
}

// DefaultRenewalPolicy returns the default renewal policy.
func DefaultRenewalPolicy() *RenewalPolicy {
	return &RenewalPolicy{
		RenewalWindow: 30 * 24 * time.Hour, // 30 days
		MaxRetries:    3,
		RetryInterval: time.Hour,
	}
}

// MonitoringReport contains certificate monitoring statistics.
type MonitoringReport struct {
	// TotalCertificates is the total number of certificates being managed.
	TotalCertificates int `json:"total_certificates"`

	// ActiveCertificates is the number of active certificates.
	ActiveCertificates int `json:"active_certificates"`

	// ExpiringSoon is the number of certificates expiring within the renewal window.
	ExpiringSoon int `json:"expiring_soon"`

	// Expired is the number of expired certificates.
	Expired int `json:"expired"`

	// Revoked is the number of revoked certificates.
	Revoked int `json:"revoked"`

	// RenewalsPending is the number of certificates pending renewal.
	RenewalsPending int `json:"renewals_pending"`

	// RenewalsFailed is the number of failed renewal attempts.
	RenewalsFailed int `json:"renewals_failed"`

	// GeneratedAt is when this report was generated.
	GeneratedAt time.Time `json:"generated_at"`
}
