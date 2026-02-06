package service

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/keycloak"
)

const (
	keycloakLabel        = "app=keycloak"
	keycloakPort         = 8080
	keycloakReadyTimeout = 120 * time.Second
)

// KeycloakConnection holds an active connection to Keycloak via port-forward.
type KeycloakConnection struct {
	Store     *keycloak.Store
	Client    *keycloak.Client
	Connector *Connector
	StopChan  chan struct{}
}

// ConnectKeycloak establishes a port-forward to Keycloak and returns a Store.
func ConnectKeycloak(
	ctx context.Context,
	kubeconfig, namespace, realm,
	adminUser, adminPassword string,
) (*KeycloakConnection, error) {
	conn, err := NewConnector(kubeconfig, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to create k8s connector: %w", err)
	}

	if waitErr := conn.WaitForPod(ctx, keycloakLabel, keycloakReadyTimeout); waitErr != nil {
		conn.Close()
		return nil, fmt.Errorf("keycloak pod not ready: %w", waitErr)
	}

	fwd, err := conn.PortForward(ctx, keycloakLabel, keycloakPort)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to port-forward to keycloak: %w", err)
	}

	baseURL := fmt.Sprintf("http://localhost:%d", fwd.LocalPort)
	kcClient, err := keycloak.NewClient(&keycloak.Config{
		BaseURL:       baseURL,
		Realm:         realm,
		ClientID:      keycloak.AdminCLIClientID,
		AdminUsername:  adminUser,
		AdminPassword:  adminPassword,
		Timeout:       30 * time.Second,
	})
	if err != nil {
		close(fwd.StopChan)
		conn.Close()
		return nil, fmt.Errorf("failed to create keycloak client: %w", err)
	}

	store := keycloak.NewStore(kcClient, zap.NewNop())

	return &KeycloakConnection{
		Store:     store,
		Client:    kcClient,
		Connector: conn,
		StopChan:  fwd.StopChan,
	}, nil
}

// Close cleans up the Keycloak connection.
func (kc *KeycloakConnection) Close() {
	close(kc.StopChan)
	kc.Connector.Close()
}
