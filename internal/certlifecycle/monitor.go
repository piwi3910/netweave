package certlifecycle

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
)

const (
	// DefaultScanInterval is the default interval between certificate scan loops.
	DefaultScanInterval = 5 * time.Minute

	// DefaultRenewalWindow is the default window before expiry to trigger renewal.
	DefaultRenewalWindow = 720 * time.Hour // 30 days.
)

// MonitorConfig holds configuration for the certificate expiry monitor.
type MonitorConfig struct {
	// ScanInterval is how often the monitor scans for expiring/expired certs.
	ScanInterval time.Duration

	// RenewalWindow is how far before expiry to mark certificates as expiring_soon.
	RenewalWindow time.Duration

	// Store provides certificate metadata persistence.
	Store Store

	// Logger provides structured logging.
	Logger *zap.Logger
}

// Monitor watches for certificate expiry and status transitions.
// It runs periodic scans to detect certificates that are expiring soon or
// have already expired, updating their status and invoking callbacks.
type Monitor struct {
	scanInterval  time.Duration
	renewalWindow time.Duration
	store         Store
	logger        *zap.Logger
	stopCh        chan struct{}
	wg            sync.WaitGroup

	// OnExpiringSoon is called when a certificate transitions to expiring_soon.
	OnExpiringSoon func(ctx context.Context, meta *CertificateMetadata)

	// OnExpired is called when a certificate transitions to expired.
	OnExpired func(ctx context.Context, meta *CertificateMetadata)
}

// NewMonitor creates a new certificate expiry monitor.
func NewMonitor(cfg *MonitorConfig) *Monitor {
	scanInterval := cfg.ScanInterval
	if scanInterval == 0 {
		scanInterval = DefaultScanInterval
	}

	renewalWindow := cfg.RenewalWindow
	if renewalWindow == 0 {
		renewalWindow = DefaultRenewalWindow
	}

	return &Monitor{
		scanInterval:  scanInterval,
		renewalWindow: renewalWindow,
		store:         cfg.Store,
		logger:        cfg.Logger,
		stopCh:        make(chan struct{}),
	}
}

// Start begins the periodic certificate scan loop.
// It blocks until Stop is called or the context is canceled.
func (m *Monitor) Start(ctx context.Context) error {
	m.logger.Info("starting certificate monitor",
		zap.Duration("scan_interval", m.scanInterval),
		zap.Duration("renewal_window", m.renewalWindow))

	m.wg.Add(1)
	go m.scanLoop(ctx)

	// Wait for shutdown signal.
	select {
	case <-ctx.Done():
		return m.Stop()
	case <-m.stopCh:
		return nil
	}
}

// Stop signals the monitor to stop and waits for it to finish.
func (m *Monitor) Stop() error {
	m.logger.Info("stopping certificate monitor")
	close(m.stopCh)
	m.wg.Wait()
	m.logger.Info("certificate monitor stopped")
	return nil
}

// scanLoop runs the periodic scan.
func (m *Monitor) scanLoop(ctx context.Context) {
	defer m.wg.Done()

	// Run initial scan immediately.
	m.runScan(ctx)

	ticker := time.NewTicker(m.scanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.runScan(ctx)
		}
	}
}

// runScan performs a single scan cycle for expiring and expired certificates.
func (m *Monitor) runScan(ctx context.Context) {
	start := time.Now()
	m.logger.Debug("starting certificate scan")

	m.scanExpiringSoon(ctx)
	m.scanExpired(ctx)
	m.updateStatusGauge(ctx)

	duration := time.Since(start).Seconds()
	RecordMonitorLoop(duration)

	m.logger.Debug("certificate scan complete",
		zap.Float64("duration_seconds", duration))
}

// scanExpiringSoon finds active certificates within the renewal window and marks them.
func (m *Monitor) scanExpiringSoon(ctx context.Context) {
	threshold := time.Now().Add(m.renewalWindow)

	certs, err := m.store.ListExpiring(ctx, threshold)
	if err != nil {
		m.logger.Error("failed to list expiring certificates", zap.Error(err))
		return
	}

	for _, cert := range certs {
		if err := m.store.UpdateStatus(ctx, cert.SerialNumber, StatusExpiringSoon); err != nil {
			m.logger.Error("failed to update certificate status to expiring_soon",
				zap.String("serial", cert.SerialNumber),
				zap.Error(err))
			continue
		}

		m.logger.Info("certificate marked as expiring_soon",
			zap.String("serial", cert.SerialNumber),
			zap.Time("expires_at", cert.ExpiresAt))

		if m.OnExpiringSoon != nil {
			cert.Status = StatusExpiringSoon
			m.OnExpiringSoon(ctx, cert)
		}
	}
}

// scanExpired finds active or expiring_soon certificates past their expiry and marks them.
func (m *Monitor) scanExpired(ctx context.Context) {
	now := time.Now()

	// Check both active and expiring_soon for expired certs.
	for _, status := range []CertificateStatus{StatusActive, StatusExpiringSoon} {
		certs, err := m.store.ListByStatus(ctx, status)
		if err != nil {
			m.logger.Error("failed to list certificates by status",
				zap.String("status", string(status)),
				zap.Error(err))
			continue
		}

		for _, cert := range certs {
			if cert.ExpiresAt.After(now) {
				continue
			}

			if err := m.store.UpdateStatus(ctx, cert.SerialNumber, StatusExpired); err != nil {
				m.logger.Error("failed to update certificate status to expired",
					zap.String("serial", cert.SerialNumber),
					zap.Error(err))
				continue
			}

			m.logger.Info("certificate marked as expired",
				zap.String("serial", cert.SerialNumber),
				zap.Time("expires_at", cert.ExpiresAt))

			if m.OnExpired != nil {
				cert.Status = StatusExpired
				m.OnExpired(ctx, cert)
			}
		}
	}
}

// updateStatusGauge refreshes the by-status gauge from the store.
func (m *Monitor) updateStatusGauge(ctx context.Context) {
	counts, err := m.store.CountByStatus(ctx)
	if err != nil {
		m.logger.Error("failed to count certificates by status", zap.Error(err))
		return
	}
	RecordCertsByStatus(counts)
}
