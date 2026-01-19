package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/keycloak"
)

// KeycloakBackup represents a backup of Keycloak state.
type KeycloakBackup struct {
	Timestamp time.Time         `json:"timestamp"`
	Realm     string            `json:"realm"`
	Users     []keycloak.User   `json:"users"`
	Roles     []keycloak.Role   `json:"roles"`
	Metadata  map[string]string `json:"metadata"`
}

// Exporter exports Keycloak state for backup/rollback.
type Exporter struct {
	keycloakClient *keycloak.Client
	logger         *zap.Logger
}

// NewExporter creates a new Exporter instance.
func NewExporter(
	keycloakClient *keycloak.Client,
	logger *zap.Logger,
) *Exporter {
	return &Exporter{
		keycloakClient: keycloakClient,
		logger:         logger,
	}
}

// Export exports the current Keycloak state to a file.
func (e *Exporter) Export(ctx context.Context, outputFile string) error {
	e.logger.Info("Exporting Keycloak state...")

	backup := &KeycloakBackup{
		Timestamp: time.Now(),
		Metadata:  make(map[string]string),
	}

	// Export users
	e.logger.Info("Exporting users...")
	users, err := e.keycloakClient.ListUsers(ctx, 0)
	if err != nil {
		return fmt.Errorf("failed to list users: %w", err)
	}
	backup.Users = users
	e.logger.Info("Exported users", zap.Int("count", len(users)))

	// Export roles
	e.logger.Info("Exporting roles...")
	roles, err := e.keycloakClient.ListRealmRoles(ctx)
	if err != nil {
		return fmt.Errorf("failed to list roles: %w", err)
	}
	backup.Roles = roles
	e.logger.Info("Exported roles", zap.Int("count", len(roles)))

	// Write backup to file
	e.logger.Info("Writing backup to file", zap.String("file", outputFile))
	data, err := json.MarshalIndent(backup, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal backup: %w", err)
	}

	if err := os.WriteFile(outputFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write backup file: %w", err)
	}

	e.logger.Info("Backup complete",
		zap.String("file", outputFile),
		zap.Int("users", len(users)),
		zap.Int("roles", len(roles)),
	)

	return nil
}
