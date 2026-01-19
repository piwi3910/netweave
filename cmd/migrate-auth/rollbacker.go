package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/keycloak"
)

// Rollbacker restores Keycloak state from a backup.
type Rollbacker struct {
	keycloakClient *keycloak.Client
	logger         *zap.Logger
}

// NewRollbacker creates a new Rollbacker instance.
func NewRollbacker(
	keycloakClient *keycloak.Client,
	logger *zap.Logger,
) *Rollbacker {
	return &Rollbacker{
		keycloakClient: keycloakClient,
		logger:         logger,
	}
}

// Rollback restores Keycloak state from a backup file.
func (r *Rollbacker) Rollback(ctx context.Context, inputFile string) error {
	r.logger.Warn("Starting rollback - this will delete existing data!")

	// Read backup file
	r.logger.Info("Reading backup file", zap.String("file", inputFile))
	data, err := os.ReadFile(filepath.Clean(inputFile))
	if err != nil {
		return fmt.Errorf("failed to read backup file: %w", err)
	}

	var backup KeycloakBackup
	if err := json.Unmarshal(data, &backup); err != nil {
		return fmt.Errorf("failed to unmarshal backup: %w", err)
	}

	r.logger.Info("Loaded backup",
		zap.Time("timestamp", backup.Timestamp),
		zap.Int("users", len(backup.Users)),
		zap.Int("roles", len(backup.Roles)),
	)

	// Delete existing users (except admin)
	r.logger.Info("Deleting existing users...")
	existingUsers, err := r.keycloakClient.ListUsers(ctx, 0)
	if err != nil {
		return fmt.Errorf("failed to list existing users: %w", err)
	}

	for _, user := range existingUsers {
		// Skip admin users
		if user.Username == "admin" {
			continue
		}

		if err := r.keycloakClient.DeleteUser(ctx, user.ID); err != nil {
			r.logger.Warn("Failed to delete user",
				zap.String("userID", user.ID),
				zap.String("username", user.Username),
				zap.Error(err),
			)
		} else {
			r.logger.Debug("Deleted user", zap.String("username", user.Username))
		}
	}

	// Delete existing custom roles (keep system roles)
	r.logger.Info("Deleting existing custom roles...")
	existingRoles, err := r.keycloakClient.ListRealmRoles(ctx)
	if err != nil {
		return fmt.Errorf("failed to list existing roles: %w", err)
	}

	systemRoles := map[string]bool{
		"offline_access":         true,
		"uma_authorization":      true,
		"default-roles-master":   true,
		"default-roles-netweave": true,
	}

	for _, role := range existingRoles {
		// Skip system roles
		if systemRoles[role.Name] {
			continue
		}

		if err := r.keycloakClient.DeleteRealmRole(ctx, role.Name); err != nil {
			r.logger.Warn("Failed to delete role",
				zap.String("roleName", role.Name),
				zap.Error(err),
			)
		} else {
			r.logger.Debug("Deleted role", zap.String("roleName", role.Name))
		}
	}

	// Restore roles from backup
	r.logger.Info("Restoring roles from backup...")
	for _, role := range backup.Roles {
		// Skip system roles
		if systemRoles[role.Name] {
			continue
		}

		if err := r.keycloakClient.CreateRealmRole(ctx, &role); err != nil {
			r.logger.Warn("Failed to restore role",
				zap.String("roleName", role.Name),
				zap.Error(err),
			)
		} else {
			r.logger.Debug("Restored role", zap.String("roleName", role.Name))
		}
	}

	// Restore users from backup
	r.logger.Info("Restoring users from backup...")
	for _, user := range backup.Users {
		// Skip admin users
		if user.Username == "admin" {
			continue
		}

		userID, err := r.keycloakClient.CreateUser(ctx, &user)
		if err != nil {
			r.logger.Warn("Failed to restore user",
				zap.String("username", user.Username),
				zap.Error(err),
			)
		} else {
			r.logger.Debug("Restored user",
				zap.String("username", user.Username),
				zap.String("userID", userID),
			)
		}
	}

	r.logger.Info("Rollback complete",
		zap.Int("restoredUsers", len(backup.Users)),
		zap.Int("restoredRoles", len(backup.Roles)),
	)

	return nil
}
