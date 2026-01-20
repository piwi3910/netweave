package certmanager

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/piwi3910/netweave/internal/keycloak"
	"github.com/piwi3910/netweave/internal/vault"
	"go.uber.org/zap"
)

// Service manages certificate lifecycle operations.
type Service struct {
	config         *Config
	vaultClient    *vault.Client
	keycloakClient *keycloak.Client
	logger         *zap.Logger

	// Certificate storage (in production, use persistent storage)
	mu           sync.RWMutex
	certificates map[string]*Certificate // keyed by serial number

	// Background worker control
	stopCh chan struct{}
	doneCh chan struct{}
}

// NewService creates a new certificate manager service.
func NewService(config *Config, logger *zap.Logger) (*Service, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	if logger == nil {
		logger = zap.NewNop()
	}

	// Initialize Vault client
	vaultCfg := &vault.Config{
		Address: config.VaultAddress,
		Token:   config.VaultToken,
		PKIPath: config.VaultPKIPath,
	}
	vaultClient, err := vault.NewClient(vaultCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create Vault client: %w", err)
	}

	// Initialize Keycloak client
	keycloakCfg := &keycloak.Config{
		BaseURL:       config.KeycloakBaseURL,
		Realm:         config.KeycloakRealm,
		ClientID:      config.KeycloakClientID,
		ClientSecret:  config.KeycloakClientSecret,
		AdminUsername: config.KeycloakAdminUsername,
		AdminPassword: config.KeycloakAdminPassword,
	}
	keycloakClient, err := keycloak.NewClient(keycloakCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create Keycloak client: %w", err)
	}

	return &Service{
		config:         config,
		vaultClient:    vaultClient,
		keycloakClient: keycloakClient,
		logger:         logger,
		certificates:   make(map[string]*Certificate),
		stopCh:         make(chan struct{}),
		doneCh:         make(chan struct{}),
	}, nil
}

// Start starts the background monitoring and renewal service.
func (s *Service) Start(ctx context.Context) error {
	s.logger.Info("Starting certificate manager service",
		zap.Duration("monitor_interval", s.config.MonitorInterval),
		zap.Bool("auto_renewal", s.config.EnableAutoRenewal))

	go s.monitorLoop(ctx)

	return nil
}

// Stop gracefully stops the certificate manager service.
func (s *Service) Stop() error {
	s.logger.Info("Stopping certificate manager service")
	close(s.stopCh)
	<-s.doneCh
	s.logger.Info("Certificate manager service stopped")
	return nil
}

// monitorLoop periodically scans for expiring certificates and triggers renewals.
func (s *Service) monitorLoop(ctx context.Context) {
	defer close(s.doneCh)

	ticker := time.NewTicker(s.config.MonitorInterval)
	defer ticker.Stop()

	// Run immediately on start
	s.scanAndRenew(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.scanAndRenew(ctx)
		}
	}
}

// scanAndRenew scans all certificates and renews those expiring soon.
func (s *Service) scanAndRenew(ctx context.Context) {
	s.logger.Debug("Starting certificate scan")

	s.mu.RLock()
	certs := make([]*Certificate, 0, len(s.certificates))
	for _, cert := range s.certificates {
		certs = append(certs, cert)
	}
	s.mu.RUnlock()

	now := time.Now()
	renewalWindow := now.Add(s.config.RenewalPolicy.RenewalWindow)

	for _, cert := range certs {
		// Skip if already revoked or expired
		if cert.Status == CertStatusRevoked || cert.Status == CertStatusExpired {
			continue
		}

		// Check if certificate is expiring soon
		if cert.ExpiresAt.Before(renewalWindow) && cert.Status != CertStatusRenewalPending {
			s.logger.Info("Certificate expiring soon",
				zap.String("serial", cert.SerialNumber),
				zap.String("common_name", cert.CommonName),
				zap.Time("expires_at", cert.ExpiresAt))

			// Update status
			s.mu.Lock()
			cert.Status = CertStatusExpiringSoon
			s.mu.Unlock()

			// Trigger renewal if enabled
			if s.config.EnableAutoRenewal {
				go s.renewCertificate(ctx, cert)
			}
		}

		// Check if certificate has expired
		if cert.ExpiresAt.Before(now) && cert.Status != CertStatusExpired {
			s.logger.Warn("Certificate expired",
				zap.String("serial", cert.SerialNumber),
				zap.String("common_name", cert.CommonName),
				zap.Time("expired_at", cert.ExpiresAt))

			s.mu.Lock()
			cert.Status = CertStatusExpired
			s.mu.Unlock()
		}
	}

	s.logger.Debug("Certificate scan complete", zap.Int("total_certificates", len(certs)))
}

// renewCertificate attempts to renew a certificate.
func (s *Service) renewCertificate(ctx context.Context, cert *Certificate) {
	s.mu.Lock()
	cert.Status = CertStatusRenewalPending
	cert.RenewalAttempts++
	now := time.Now()
	cert.LastRenewalAttempt = &now
	s.mu.Unlock()

	s.logger.Info("Attempting certificate renewal",
		zap.String("serial", cert.SerialNumber),
		zap.String("common_name", cert.CommonName),
		zap.Int("attempt", cert.RenewalAttempts))

	// Issue new certificate
	req := &CertificateRequest{
		UserID:     cert.UserID,
		TenantID:   cert.TenantID,
		CommonName: cert.CommonName,
		TTL:        "8760h", // 1 year
	}

	newCert, err := s.IssueCertificate(ctx, req)
	if err != nil {
		s.logger.Error("Certificate renewal failed",
			zap.String("serial", cert.SerialNumber),
			zap.Error(err))

		s.mu.Lock()
		if cert.RenewalAttempts >= s.config.RenewalPolicy.MaxRetries {
			cert.Status = CertStatusRenewalFailed
		} else {
			// Schedule retry
			cert.Status = CertStatusExpiringSoon
		}
		s.mu.Unlock()

		return
	}

	// Revoke old certificate
	if err := s.RevokeCertificate(ctx, cert.SerialNumber); err != nil {
		s.logger.Warn("Failed to revoke old certificate after renewal",
			zap.String("serial", cert.SerialNumber),
			zap.Error(err))
	}

	s.logger.Info("Certificate renewed successfully",
		zap.String("old_serial", cert.SerialNumber),
		zap.String("new_serial", newCert.SerialNumber),
		zap.String("common_name", newCert.CommonName))

	// TODO: Send notification to user and admins
}

// Health returns the health status of the service.
func (s *Service) Health(ctx context.Context) error {
	// Check Vault connectivity
	if err := s.vaultClient.Ping(ctx); err != nil {
		return fmt.Errorf("vault health check failed: %w", err)
	}

	// Check Keycloak connectivity
	if err := s.keycloakClient.Ping(ctx); err != nil {
		return fmt.Errorf("keycloak health check failed: %w", err)
	}

	return nil
}
