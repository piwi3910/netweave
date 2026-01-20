package certmanager

import (
	"fmt"
	"time"
)

// Config holds the configuration for the certificate manager service.
type Config struct {
	// VaultAddress is the Vault server address.
	VaultAddress string

	// VaultToken is the Vault authentication token.
	// In Kubernetes, this should be obtained via Kubernetes auth method.
	VaultToken string

	// VaultPKIPath is the path to the PKI secrets engine (default: "pki_int").
	VaultPKIPath string

	// VaultRole is the PKI role to use for certificate issuance (default: "netweave-client").
	VaultRole string

	// KeycloakBaseURL is the Keycloak server base URL.
	KeycloakBaseURL string

	// KeycloakRealm is the Keycloak realm name.
	KeycloakRealm string

	// KeycloakClientID is the Keycloak client ID.
	KeycloakClientID string

	// KeycloakClientSecret is the Keycloak client secret.
	KeycloakClientSecret string

	// KeycloakAdminUsername is the Keycloak admin username.
	KeycloakAdminUsername string

	// KeycloakAdminPassword is the Keycloak admin password.
	KeycloakAdminPassword string

	// MonitorInterval is how often to scan for expiring certificates (default: 1 hour).
	MonitorInterval time.Duration

	// RenewalPolicy defines the renewal policy.
	RenewalPolicy *RenewalPolicy

	// EnableAutoRenewal enables automatic certificate renewal (default: true).
	EnableAutoRenewal bool

	// NotificationWebhookURL is the webhook URL for sending notifications (optional).
	NotificationWebhookURL string
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		VaultAddress:      "http://localhost:8200",
		VaultPKIPath:      "pki_int",
		VaultRole:         "netweave-client",
		KeycloakBaseURL:   "http://localhost:8090",
		KeycloakRealm:     "netweave",
		MonitorInterval:   time.Hour,
		RenewalPolicy:     DefaultRenewalPolicy(),
		EnableAutoRenewal: true,
	}
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	if c.VaultAddress == "" {
		return fmt.Errorf("VaultAddress is required")
	}
	if c.VaultPKIPath == "" {
		return fmt.Errorf("VaultPKIPath is required")
	}
	if c.VaultRole == "" {
		return fmt.Errorf("VaultRole is required")
	}
	if c.KeycloakBaseURL == "" {
		return fmt.Errorf("KeycloakBaseURL is required")
	}
	if c.KeycloakRealm == "" {
		return fmt.Errorf("KeycloakRealm is required")
	}
	if c.MonitorInterval <= 0 {
		return fmt.Errorf("MonitorInterval must be positive")
	}
	if c.RenewalPolicy == nil {
		return fmt.Errorf("RenewalPolicy cannot be nil")
	}
	if c.RenewalPolicy.RenewalWindow <= 0 {
		return fmt.Errorf("RenewalPolicy.RenewalWindow must be positive")
	}
	if c.RenewalPolicy.MaxRetries < 0 {
		return fmt.Errorf("RenewalPolicy.MaxRetries cannot be negative")
	}
	if c.RenewalPolicy.RetryInterval <= 0 {
		return fmt.Errorf("RenewalPolicy.RetryInterval must be positive")
	}

	return nil
}
