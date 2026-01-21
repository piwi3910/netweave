package certmanager

import (
	"context"
	"fmt"
	"maps"
	"time"

	"github.com/piwi3910/netweave/internal/vault"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.uber.org/zap"
)

var tracer = otel.Tracer("certmanager")

// IssueCertificate issues a new certificate for a user.
func (s *Service) IssueCertificate(ctx context.Context, req *CertificateRequest) (*Certificate, error) {
	ctx, span := tracer.Start(ctx, "IssueCertificate")
	defer span.End()

	if req == nil {
		span.SetStatus(codes.Error, "request cannot be nil")
		return nil, fmt.Errorf("request cannot be nil")
	}

	span.SetAttributes(
		attribute.String("user_id", req.UserID),
		attribute.String("tenant_id", req.TenantID),
		attribute.String("common_name", req.CommonName),
		attribute.String("ttl", req.TTL),
	)
	if req.UserID == "" {
		span.SetStatus(codes.Error, "user_id is required")
		return nil, fmt.Errorf("user_id is required")
	}
	if req.TenantID == "" {
		span.SetStatus(codes.Error, "tenant_id is required")
		return nil, fmt.Errorf("tenant_id is required")
	}
	if req.CommonName == "" {
		span.SetStatus(codes.Error, "common_name is required")
		return nil, fmt.Errorf("common_name is required")
	}

	s.logger.Info("Issuing certificate",
		zap.String("user_id", req.UserID),
		zap.String("tenant_id", req.TenantID),
		zap.String("common_name", req.CommonName))

	// Check if context is canceled
	if ctxErr := ctx.Err(); ctxErr != nil {
		span.SetStatus(codes.Error, "context canceled")
		return nil, fmt.Errorf("context canceled: %w", ctxErr)
	}

	// Set default TTL if not specified
	ttl := req.TTL
	if ttl == "" {
		ttl = DefaultCertificateTTL
	}

	// Issue certificate from Vault
	vaultReq := &vault.CertificateRequest{
		CommonName: req.CommonName,
		AltNames:   req.AltNames,
		IPSANs:     req.IPSANs,
		TTL:        ttl,
		Format:     "pem",
	}

	vaultCert, err := s.vaultClient.IssueCertificate(ctx, s.config.VaultRole, vaultReq)
	if err != nil {
		certificateIssuances.WithLabelValues("failure").Inc()
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to issue certificate from vault")
		return nil, fmt.Errorf("failed to issue certificate from vault: %w", err)
	}
	certificateIssuances.WithLabelValues("success").Inc()
	span.AddEvent("certificate_issued_from_vault")
	span.SetAttributes(attribute.String("serial_number", vaultCert.SerialNumber))

	// Create certificate record with normalized TTL
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
		TTL:            ttl, // Use normalized TTL (with default applied)
		Status:         CertStatusActive,
	}

	// Store certificate
	s.mu.Lock()
	s.certificates[cert.SerialNumber] = cert
	s.mu.Unlock()

	// Update Keycloak user attributes
	// Note: If this fails, we log a warning but don't fail the operation because:
	// 1. The certificate is already issued and valid for mTLS authentication
	// 2. User attributes are for informational purposes only
	// 3. Attributes can be updated later via a dedicated endpoint
	// 4. Certificate should not be invalid just because metadata update failed
	if kcErr := s.updateKeycloakUser(ctx, req.UserID, cert); kcErr != nil {
		keycloakUpdateFailures.Inc()
		s.logger.Warn("Failed to update Keycloak user attributes",
			zap.String("user_id", req.UserID),
			zap.Error(kcErr))
	}

	s.logger.Info("Certificate issued successfully",
		zap.String("serial", cert.SerialNumber),
		zap.String("common_name", cert.CommonName),
		zap.Time("expires_at", cert.ExpiresAt))

	// Track certificate lifetime
	lifetime := cert.ExpiresAt.Sub(cert.IssuedAt).Seconds()
	certificateLifetime.Observe(lifetime)

	// Update status metrics
	s.updateCertificateMetrics()

	return cert, nil
}

// GetCertificate retrieves a certificate by serial number.
// TODO(#276): Use context for Redis storage operations and timeout control.
func (s *Service) GetCertificate(ctx context.Context, serialNumber string) (*Certificate, error) {
	// Check context before acquiring lock
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context canceled: %w", err)
	}

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
// TODO(#276): Use context for Redis storage operations and timeout control.
func (s *Service) ListCertificates(ctx context.Context, userID, tenantID string) ([]*Certificate, error) {
	// Check context before acquiring lock
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context canceled: %w", err)
	}

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
	ctx, span := tracer.Start(ctx, "RevokeCertificate")
	defer span.End()

	span.SetAttributes(attribute.String("serial_number", serialNumber))

	if serialNumber == "" {
		span.SetStatus(codes.Error, "serial_number is required")
		return fmt.Errorf("serial_number is required")
	}

	s.logger.Info("Revoking certificate", zap.String("serial", serialNumber))

	// Revoke in Vault
	if err := s.vaultClient.RevokeCertificateBySerial(ctx, serialNumber); err != nil {
		certificateRevocations.WithLabelValues("failure").Inc()
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to revoke certificate in vault")
		return fmt.Errorf("failed to revoke certificate in vault: %w", err)
	}
	certificateRevocations.WithLabelValues("success").Inc()
	span.AddEvent("certificate_revoked_in_vault")
	span.SetStatus(codes.Ok, "certificate revoked successfully")

	// Update certificate status
	s.mu.Lock()
	if cert, exists := s.certificates[serialNumber]; exists {
		cert.Status = CertStatusRevoked
		now := time.Now()
		cert.RevokedAt = &now
	}
	s.mu.Unlock()

	s.logger.Info("Certificate revoked successfully", zap.String("serial", serialNumber))

	// Update status metrics
	s.updateCertificateMetrics()

	return nil
}

// GetMonitoringReport generates a monitoring report with certificate statistics.
// Note: Statistics are a point-in-time snapshot; statuses may change during collection.
// TODO(#276): Use context for Redis storage operations and timeout control.
func (s *Service) GetMonitoringReport(ctx context.Context) (*MonitoringReport, error) {
	// Check context before acquiring lock
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context canceled: %w", err)
	}

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
		return fmt.Errorf("failed to get user %s for certificate attribute update: %w", userID, err)
	}

	// Update attributes
	if user.Attributes == nil {
		user.Attributes = make(map[string][]string)
	}
	maps.Copy(user.Attributes, attributes)

	// Update user in Keycloak
	if updateErr := s.keycloakClient.UpdateUser(ctx, user); updateErr != nil {
		return fmt.Errorf("failed to update user %s with certificate attributes: %w", userID, updateErr)
	}

	return nil
}
