package cmd

import (
	"context"
	"fmt"

	"github.com/piwi3910/netweave/internal/cli/service"
)

// Default Keycloak connection parameters used by CLI commands that manage
// users, roles, and tenants. These mirror the bootstrap values produced by
// the platform setup flow.
const (
	DefaultKeycloakRealm         = "netweave"
	DefaultKeycloakAdminUser     = "admin"
	DefaultKeycloakAdminPassword = "admin"
)

// ConnectKeycloakDefaults establishes a Keycloak connection using the default
// realm and bootstrap admin credentials. It uses the global kubeconfig and
// namespace flags registered on the root command.
//
// Callers must invoke Close on the returned connection when finished.
func ConnectKeycloakDefaults(ctx context.Context) (*service.KeycloakConnection, error) {
	kc, err := service.ConnectKeycloak(
		ctx,
		Global.Kubeconfig,
		Global.Namespace,
		DefaultKeycloakRealm,
		DefaultKeycloakAdminUser,
		DefaultKeycloakAdminPassword,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to keycloak: %w", err)
	}
	return kc, nil
}
