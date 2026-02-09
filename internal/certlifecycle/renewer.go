package certlifecycle

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/vault"
)

const (
	// DefaultMaxRenewalRetries is the default max attempts for certificate renewal.
	DefaultMaxRenewalRetries = 3

	// DefaultRetryInterval is the default wait between renewal retries.
	DefaultRetryInterval = 1 * time.Hour
)

// RenewerConfig holds configuration for the certificate renewal service.
type RenewerConfig struct {
	// MaxRetries is the maximum number of renewal attempts.
	MaxRetries int

	// RetryInterval is the base interval between retry attempts.
	RetryInterval time.Duration

	// Store provides certificate metadata persistence.
	Store Store

	// VaultClient provides certificate issuance and revocation.
	VaultClient VaultPKI

	// Notifier sends webhook notifications for renewal events (optional).
	Notifier *Notifier

	// Logger provides structured logging.
	Logger *zap.Logger
}

// VaultPKI defines the vault operations needed for certificate renewal.
type VaultPKI interface {
	IssueCertificate(
		ctx context.Context, roleName string, req *vault.CertificateRequest,
	) (*vault.Certificate, error)
	RevokeCertificateBySerial(ctx context.Context, serialNumber string) error
}

// Renewer handles automatic certificate renewal.
type Renewer struct {
	maxRetries    int
	retryInterval time.Duration
	store         Store
	vaultClient   VaultPKI
	notifier      *Notifier
	logger        *zap.Logger
}

// NewRenewer creates a new certificate renewal service.
func NewRenewer(cfg *RenewerConfig) *Renewer {
	maxRetries := cfg.MaxRetries
	if maxRetries == 0 {
		maxRetries = DefaultMaxRenewalRetries
	}

	retryInterval := cfg.RetryInterval
	if retryInterval == 0 {
		retryInterval = DefaultRetryInterval
	}

	return &Renewer{
		maxRetries:    maxRetries,
		retryInterval: retryInterval,
		store:         cfg.Store,
		vaultClient:   cfg.VaultClient,
		notifier:      cfg.Notifier,
		logger:        cfg.Logger,
	}
}

// RenewCertificate attempts to renew a certificate by issuing a new one,
// storing the metadata, marking the old cert as renewed, and revoking the old cert.
func (r *Renewer) RenewCertificate(
	ctx context.Context, meta *CertificateMetadata,
) error {
	r.logger.Info("attempting certificate renewal",
		zap.String("serial", meta.SerialNumber),
		zap.String("common_name", meta.CommonName),
		zap.String("role", meta.RoleName))

	// Issue new certificate via Vault.
	newCert, err := r.vaultClient.IssueCertificate(
		ctx, meta.RoleName, &vault.CertificateRequest{
			CommonName: meta.CommonName,
		},
	)
	if err != nil {
		return r.handleRenewalFailure(ctx, meta, err)
	}

	// Store new certificate metadata.
	newMeta := &CertificateMetadata{
		SerialNumber: newCert.SerialNumber,
		UserID:       meta.UserID,
		TenantID:     meta.TenantID,
		CommonName:   meta.CommonName,
		RoleName:     meta.RoleName,
		Status:       StatusActive,
		IssuedAt:     time.Now().UTC(),
		ExpiresAt:    newCert.Expiration,
		RenewedFrom:  meta.SerialNumber,
		RenewalCount: meta.RenewalCount + 1,
	}

	if err := r.store.Create(ctx, newMeta); err != nil {
		return fmt.Errorf(
			"failed to store renewed certificate metadata: %w", err,
		)
	}

	// Mark old certificate as renewed.
	if err := r.store.MarkRenewed(
		ctx, meta.SerialNumber, newCert.SerialNumber,
	); err != nil {
		r.logger.Error("failed to mark old certificate as renewed",
			zap.String("old_serial", meta.SerialNumber),
			zap.Error(err))
	}

	// Revoke old certificate in Vault.
	if err := r.vaultClient.RevokeCertificateBySerial(
		ctx, meta.SerialNumber,
	); err != nil {
		r.logger.Error("failed to revoke old certificate",
			zap.String("old_serial", meta.SerialNumber),
			zap.Error(err))
	}

	// Record metrics.
	RecordRenewal(meta.TenantID, "success")
	RecordRenewalAttempts(
		meta.TenantID, "success", meta.RetryCount+1,
	)
	lifetime := meta.ExpiresAt.Sub(meta.IssuedAt).Seconds()
	RecordLifetime(meta.TenantID, lifetime)

	// Send notification.
	r.notifyRenewal(ctx, newMeta)

	r.logger.Info("certificate renewed successfully",
		zap.String("old_serial", meta.SerialNumber),
		zap.String("new_serial", newCert.SerialNumber))

	return nil
}

// handleRenewalFailure records a renewal failure and schedules a retry.
func (r *Renewer) handleRenewalFailure(
	ctx context.Context,
	meta *CertificateMetadata,
	renewErr error,
) error {
	nextRetry := time.Now().Add(
		exponentialBackoff(r.retryInterval, meta.RetryCount+1),
	)

	r.logger.Error("certificate renewal failed",
		zap.String("serial", meta.SerialNumber),
		zap.Int("retry_count", meta.RetryCount),
		zap.Time("next_retry", nextRetry),
		zap.Error(renewErr))

	if meta.RetryCount+1 >= r.maxRetries {
		RecordRenewal(meta.TenantID, "max_retries_exceeded")
		RecordRenewalAttempts(
			meta.TenantID, "max_retries_exceeded", meta.RetryCount+1,
		)
		r.notifyRenewalFailed(ctx, meta, renewErr)
	} else {
		RecordRenewal(meta.TenantID, "failure")
	}

	if storeErr := r.store.MarkRenewalFailed(
		ctx, meta.SerialNumber, renewErr.Error(), nextRetry,
	); storeErr != nil {
		r.logger.Error("failed to record renewal failure",
			zap.String("serial", meta.SerialNumber),
			zap.Error(storeErr))
	}

	return fmt.Errorf("renewal failed for %s: %w", meta.SerialNumber, renewErr)
}

// notifyRenewal sends a renewal success notification if a notifier is configured.
func (r *Renewer) notifyRenewal(ctx context.Context, meta *CertificateMetadata) {
	if r.notifier == nil {
		return
	}
	event := NewCertEvent(EventCertRenewed, meta)
	if err := r.notifier.NotifyWithRetry(ctx, event); err != nil {
		r.logger.Warn("failed to send renewal notification",
			zap.String("serial", meta.SerialNumber),
			zap.Error(err))
	}
}

// notifyRenewalFailed sends a renewal failure notification.
func (r *Renewer) notifyRenewalFailed(
	ctx context.Context,
	meta *CertificateMetadata,
	renewErr error,
) {
	if r.notifier == nil {
		return
	}
	meta.LastError = renewErr.Error()
	event := NewCertEvent(EventCertRenewalFailed, meta)
	if err := r.notifier.NotifyWithRetry(ctx, event); err != nil {
		r.logger.Warn("failed to send renewal failure notification",
			zap.String("serial", meta.SerialNumber),
			zap.Error(err))
	}
}
