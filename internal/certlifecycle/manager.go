package certlifecycle

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ManagerConfig holds configuration for the lifecycle manager.
type ManagerConfig struct {
	Store         Store
	VaultClient   VaultPKI
	Logger        *zap.Logger
	ScanInterval  time.Duration
	RenewalWindow time.Duration
	MaxRetries    int
	RetryInterval time.Duration
	WebhookURL    string
	HMACSecret    string
}

// Manager orchestrates certificate lifecycle automation.
// It ties together the monitor, renewer, and notifier.
type Manager struct {
	store    Store
	monitor  *Monitor
	renewer  *Renewer
	notifier *Notifier
	logger   *zap.Logger
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// NewManager creates a new lifecycle manager.
func NewManager(cfg *ManagerConfig) (*Manager, error) {
	if cfg.Store == nil {
		return nil, fmt.Errorf("store is required")
	}
	if cfg.Logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	m := &Manager{
		store:  cfg.Store,
		logger: cfg.Logger,
		stopCh: make(chan struct{}),
	}

	// Initialize notifier if webhook URL is configured.
	if cfg.WebhookURL != "" {
		notifier, err := NewNotifier(&NotifierConfig{
			WebhookURL: cfg.WebhookURL,
			HMACSecret: cfg.HMACSecret,
			Logger:     cfg.Logger,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create notifier: %w", err)
		}
		m.notifier = notifier
	}

	// Initialize renewer if vault client is provided.
	if cfg.VaultClient != nil {
		m.renewer = NewRenewer(&RenewerConfig{
			MaxRetries:    cfg.MaxRetries,
			RetryInterval: cfg.RetryInterval,
			Store:         cfg.Store,
			VaultClient:   cfg.VaultClient,
			Notifier:      m.notifier,
			Logger:        cfg.Logger,
		})
	}

	// Initialize monitor.
	m.monitor = NewMonitor(&MonitorConfig{
		ScanInterval:  cfg.ScanInterval,
		RenewalWindow: cfg.RenewalWindow,
		Store:         cfg.Store,
		Logger:        cfg.Logger,
	})

	// Wire monitor callbacks.
	m.monitor.OnExpiringSoon = m.onExpiringSoon
	m.monitor.OnExpired = m.onExpired

	return m, nil
}

// Start begins the lifecycle manager and its background monitor.
func (m *Manager) Start(ctx context.Context) error {
	m.logger.Info("starting certificate lifecycle manager")

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		if err := m.monitor.Start(ctx); err != nil {
			m.logger.Error("monitor stopped with error", zap.Error(err))
		}
	}()

	// Process failed renewals on each monitor tick.
	m.wg.Add(1)
	go m.retryFailedRenewals(ctx)

	return nil
}

// Stop signals the manager and all workers to shut down.
func (m *Manager) Stop() error {
	m.logger.Info("stopping certificate lifecycle manager")
	close(m.stopCh)
	if err := m.monitor.Stop(); err != nil {
		m.logger.Error("monitor stop error", zap.Error(err))
	}
	m.wg.Wait()
	m.logger.Info("certificate lifecycle manager stopped")
	return nil
}

// RecordIssuance records a newly issued certificate in the store and metrics.
func (m *Manager) RecordIssuance(
	ctx context.Context,
	cert *CertificateMetadata,
) error {
	if err := m.store.Create(ctx, cert); err != nil {
		return fmt.Errorf("failed to record issuance: %w", err)
	}

	RecordIssuance(cert.TenantID, cert.RoleName, "success")

	if m.notifier != nil {
		event := NewCertEvent(EventCertIssued, cert)
		if err := m.notifier.NotifyWithRetry(ctx, event); err != nil {
			m.logger.Warn("failed to send issuance notification",
				zap.String("serial", cert.SerialNumber),
				zap.Error(err))
		}
	}

	return nil
}

// RecordRevocation records a certificate revocation in the store and metrics.
func (m *Manager) RecordRevocation(
	ctx context.Context, serialNumber string,
) error {
	meta, err := m.store.Get(ctx, serialNumber)
	if err != nil {
		return fmt.Errorf("failed to get certificate for revocation: %w", err)
	}

	if err := m.store.MarkRevoked(ctx, serialNumber); err != nil {
		return fmt.Errorf("failed to mark certificate as revoked: %w", err)
	}

	RecordRevocation(meta.TenantID)

	if m.notifier != nil {
		meta.Status = StatusRevoked
		event := NewCertEvent(EventCertRevoked, meta)
		if err := m.notifier.NotifyWithRetry(ctx, event); err != nil {
			m.logger.Warn("failed to send revocation notification",
				zap.String("serial", serialNumber),
				zap.Error(err))
		}
	}

	return nil
}

// onExpiringSoon handles the monitor's expiring_soon callback.
func (m *Manager) onExpiringSoon(ctx context.Context, meta *CertificateMetadata) {
	// Send notification.
	if m.notifier != nil {
		event := NewCertEvent(EventCertExpiringSoon, meta)
		if err := m.notifier.NotifyWithRetry(ctx, event); err != nil {
			m.logger.Warn("failed to send expiring_soon notification",
				zap.String("serial", meta.SerialNumber),
				zap.Error(err))
		}
	}

	// Attempt automatic renewal if renewer is available.
	if m.renewer != nil {
		if err := m.renewer.RenewCertificate(ctx, meta); err != nil {
			m.logger.Warn("automatic renewal failed for expiring cert",
				zap.String("serial", meta.SerialNumber),
				zap.Error(err))
		}
	}
}

// onExpired handles the monitor's expired callback.
func (m *Manager) onExpired(ctx context.Context, meta *CertificateMetadata) {
	if m.notifier != nil {
		event := NewCertEvent(EventCertExpired, meta)
		if err := m.notifier.NotifyWithRetry(ctx, event); err != nil {
			m.logger.Warn("failed to send expired notification",
				zap.String("serial", meta.SerialNumber),
				zap.Error(err))
		}
	}
}

// retryFailedRenewals periodically retries certificates that previously
// failed renewal and are eligible for retry.
func (m *Manager) retryFailedRenewals(ctx context.Context) {
	defer m.wg.Done()

	if m.renewer == nil {
		return
	}

	ticker := time.NewTicker(DefaultScanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.processFailedRenewals(ctx)
		}
	}
}

// processFailedRenewals finds and retries eligible failed renewals.
func (m *Manager) processFailedRenewals(ctx context.Context) {
	certs, err := m.store.ListRenewalFailed(ctx, time.Now())
	if err != nil {
		m.logger.Error("failed to list renewal-failed certificates",
			zap.Error(err))
		return
	}

	for _, cert := range certs {
		if err := m.renewer.RenewCertificate(ctx, cert); err != nil {
			m.logger.Warn("retry renewal failed",
				zap.String("serial", cert.SerialNumber),
				zap.Error(err))
		}
	}
}
