package certlifecycle

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// KeycloakUserClient defines the Keycloak operations needed for certificate sync.
type KeycloakUserClient interface {
	SetUserAttribute(ctx context.Context, userID, attributeName, attributeValue string) error
}

// KeycloakSyncer synchronizes certificate metadata to Keycloak user attributes.
// Sync failures are logged as warnings and never block certificate operations.
type KeycloakSyncer struct {
	client KeycloakUserClient
	logger *zap.Logger
}

// NewKeycloakSyncer creates a new KeycloakSyncer.
func NewKeycloakSyncer(client KeycloakUserClient, logger *zap.Logger) *KeycloakSyncer {
	return &KeycloakSyncer{
		client: client,
		logger: logger,
	}
}

// SyncIssuance sets certificate attributes on the Keycloak user.
// It stores the serial number, common name, issued_at, expires_at, and status.
func (s *KeycloakSyncer) SyncIssuance(ctx context.Context, meta *CertificateMetadata) {
	if meta.UserID == "" {
		return
	}

	attrs := map[string]string{
		"cert_serial":     meta.SerialNumber,
		"cert_cn":         meta.CommonName,
		"cert_issued_at":  meta.IssuedAt.Format(time.RFC3339),
		"cert_expires_at": meta.ExpiresAt.Format(time.RFC3339),
		"cert_status":     string(meta.Status),
	}

	for name, value := range attrs {
		if err := s.client.SetUserAttribute(ctx, meta.UserID, name, value); err != nil {
			s.logger.Warn("failed to sync cert attribute to Keycloak",
				zap.String("user_id", meta.UserID),
				zap.String("attribute", name),
				zap.Error(err))
			return
		}
	}

	s.logger.Debug("synced certificate issuance to Keycloak",
		zap.String("user_id", meta.UserID),
		zap.String("serial", meta.SerialNumber))
}

// SyncRevocation updates the cert_status attribute to "revoked" on the Keycloak user.
func (s *KeycloakSyncer) SyncRevocation(ctx context.Context, userID string) {
	if userID == "" {
		return
	}

	if err := s.client.SetUserAttribute(ctx, userID, "cert_status", string(StatusRevoked)); err != nil {
		s.logger.Warn("failed to sync cert revocation to Keycloak",
			zap.String("user_id", userID),
			zap.Error(err))
		return
	}

	s.logger.Debug("synced certificate revocation to Keycloak",
		zap.String("user_id", userID))
}

// SyncRenewal updates the certificate attributes on the Keycloak user after renewal.
func (s *KeycloakSyncer) SyncRenewal(ctx context.Context, meta *CertificateMetadata) {
	if meta.UserID == "" {
		return
	}

	attrs := map[string]string{
		"cert_serial":     meta.SerialNumber,
		"cert_cn":         meta.CommonName,
		"cert_expires_at": meta.ExpiresAt.Format(time.RFC3339),
		"cert_status":     string(meta.Status),
		"cert_renewed_at": time.Now().Format(time.RFC3339),
	}

	for name, value := range attrs {
		if err := s.client.SetUserAttribute(ctx, meta.UserID, name, value); err != nil {
			s.logger.Warn("failed to sync cert renewal to Keycloak",
				zap.String("user_id", meta.UserID),
				zap.String("attribute", name),
				zap.Error(err))
			return
		}
	}

	s.logger.Debug("synced certificate renewal to Keycloak",
		zap.String("user_id", meta.UserID),
		zap.String("serial", meta.SerialNumber))
}

// SyncExpiringSoon updates the cert_status attribute when a cert is expiring soon.
func (s *KeycloakSyncer) SyncExpiringSoon(ctx context.Context, meta *CertificateMetadata) {
	if meta.UserID == "" {
		return
	}

	if err := s.client.SetUserAttribute(
		ctx, meta.UserID, "cert_status", string(StatusExpiringSoon),
	); err != nil {
		s.logger.Warn("failed to sync expiring_soon status to Keycloak",
			zap.String("user_id", meta.UserID),
			zap.Error(err))
		return
	}

	s.logger.Debug("synced expiring_soon status to Keycloak",
		zap.String("user_id", meta.UserID),
		zap.String("serial", meta.SerialNumber))
}
