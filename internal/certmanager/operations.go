package certmanager

import (
	"context"
	"fmt"
	"time"

	"github.com/piwi3910/netweave/internal/vault"
	"go.uber.org/zap"
)

// IssueCertificate issues a new certificate for a user.
func (s *Service) IssueCertificate(ctx context.Context, req *CertificateRequest) (*Certificate, error) {
	if req == nil {
		return nil, fmt.Errorf("request cannot be nil")
	}
	if req.UserID == "" {
		return nil, fmt.Errorf("user_id is required")
	}
	if req.TenantID == "" {
		return nil, fmt.Errorf("tenant_id is required")
	}
	if req.CommonName == "" {
		return nil, fmt.Errorf("common_name is required")
	}

	s.logger.Info("Issuing certificate",
		zap.String("user_id", req.UserID),
		zap.String("tenant_id", req.TenantID),
		zap.String("common_name", req.CommonName))

	// Check if context is cancelled
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Issue certificate from Vault
	vaultReq := &vault.CertificateRequest{
		CommonName: req.CommonName,
		AltNames:   req.AltNames,
		IPSANs:     req.IPSANs,
		TTL:        req.TTL,
		Format:     "pem",
	}

	vaultCert, err := s.vaultClient.IssueCertificate(ctx, s.config.VaultRole, vaultReq)
	if err != nil {
		return nil, fmt.Errorf("failed to issue certificate from vault: %w", err)
	}

	// Create certificate record
	cert := &Certificate{
		SerialNumber:   vaultCert.SerialNumber,
		CommonName:     req.CommonName,
		UserID:         req.UserID,
		TenantID:       req.TenantID,
		CertificatePEM: vaultCert.Certificate,
		PrivateKeyPEM:  vaultCert.PrivateKey,
		IssuingCA:      vaultCert.IssuingCA,
		CAChain:        vaultCert.CAChain,
		IssuedAt:       time.Now(),
		ExpiresAt:      vaultCert.Expiration,
		TTL:            req.TTL,
		Status:         CertStatusActive,
	}

	// Store certificate
	s.mu.Lock()
	s.certificates[cert.SerialNumber] = cert
	s.mu.Unlock()

	// Update Keycloak user attributes
	if err := s.updateKeycloakUser(ctx, req.UserID, cert); err != nil {
		s.logger.Warn("Failed to update Keycloak user attributes",
			zap.String("user_id", req.UserID),
			zap.Error(err))
		// Don't fail the operation, certificate is still issued
	}

	s.logger.Info("Certificate issued successfully",
		zap.String("serial", cert.SerialNumber),
		zap.String("common_name", cert.CommonName),
		zap.Time("expires_at", cert.ExpiresAt))

	return cert, nil
}

// GetCertificate retrieves a certificate by serial number.
func (s *Service) GetCertificate(ctx context.Context, serialNumber string) (*Certificate, error) {
	if serialNumber == "" {
		return nil, fmt.Errorf("serial_number is required")
	}

	s.mu.RLock()
	cert, exists := s.certificates[serialNumber]
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("certificate not found: %s", serialNumber)
	}

	// Create a copy without private key
	result := *cert
	result.PrivateKeyPEM = "" // Never return private key after initial issuance

	return &result, nil
}

// ListCertificates lists all certificates, optionally filtered by user or tenant.
func (s *Service) ListCertificates(ctx context.Context, userID, tenantID string) ([]*Certificate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Certificate, 0, len(s.certificates))
	for _, cert := range s.certificates {
		// Apply filters
		if userID != "" && cert.UserID != userID {
			continue
		}
		if tenantID != "" && cert.TenantID != tenantID {
			continue
		}

		// Create a copy without private key
		certCopy := *cert
		certCopy.PrivateKeyPEM = ""
		result = append(result, &certCopy)
	}

	return result, nil
}

// RevokeCertificate revokes a certificate by serial number.
func (s *Service) RevokeCertificate(ctx context.Context, serialNumber string) error {
	if serialNumber == "" {
		return fmt.Errorf("serial_number is required")
	}

	s.logger.Info("Revoking certificate", zap.String("serial", serialNumber))

	// Revoke in Vault
	if err := s.vaultClient.RevokeCertificateBySerial(ctx, serialNumber); err != nil {
		return fmt.Errorf("failed to revoke certificate in vault: %w", err)
	}

	// Update certificate status
	s.mu.Lock()
	if cert, exists := s.certificates[serialNumber]; exists {
		cert.Status = CertStatusRevoked
		now := time.Now()
		cert.RevokedAt = &now
	}
	s.mu.Unlock()

	s.logger.Info("Certificate revoked successfully", zap.String("serial", serialNumber))

	return nil
}

// GetMonitoringReport generates a monitoring report with certificate statistics.
func (s *Service) GetMonitoringReport(ctx context.Context) (*MonitoringReport, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	report := &MonitoringReport{
		TotalCertificates: len(s.certificates),
		GeneratedAt:       time.Now(),
	}

	for _, cert := range s.certificates {
		// Count based on current status - trust the status field
		// (scanAndRenew updates status to ExpiringSoon when needed)
		switch cert.Status {
		case CertStatusActive:
			report.ActiveCertificates++
		case CertStatusExpiringSoon:
			report.ExpiringSoon++
		case CertStatusExpired:
			report.Expired++
		case CertStatusRevoked:
			report.Revoked++
		case CertStatusRenewalPending:
			report.RenewalsPending++
		case CertStatusRenewalFailed:
			report.RenewalsFailed++
		}
	}

	return report, nil
}

// updateKeycloakUser updates the user's certificate attributes in Keycloak.
func (s *Service) updateKeycloakUser(ctx context.Context, userID string, cert *Certificate) error {
	// Update user attributes with certificate subject and serial number
	attributes := map[string][]string{
		"certSubject":      {cert.CommonName},
		"certSerialNumber": {cert.SerialNumber},
		"certIssuedAt":     {cert.IssuedAt.Format(time.RFC3339)},
		"certExpiresAt":    {cert.ExpiresAt.Format(time.RFC3339)},
	}

	// Get existing user
	user, err := s.keycloakClient.GetUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Update attributes
	if user.Attributes == nil {
		user.Attributes = make(map[string][]string)
	}
	for k, v := range attributes {
		user.Attributes[k] = v
	}

	// Update user in Keycloak
	if err := s.keycloakClient.UpdateUser(ctx, user); err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	return nil
}
