package setup

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/piwi3910/netweave/internal/auth"
	"github.com/piwi3910/netweave/internal/cli/cmd"
	"github.com/piwi3910/netweave/internal/cli/service"
	"github.com/piwi3910/netweave/internal/keycloak"
)

const (
	keycloakLabel              = "app.kubernetes.io/component=keycloak"
	keycloakPort               = 8080
	keycloakReadyTimeout       = 120 * time.Second
	defaultRealm               = "netweave"
	defaultClientID            = "netweave-gateway"
	defaultAdminUser           = "admin"
	defaultAdminPassword       = "admin"
	defaultTenantID            = "default"
	defaultTenantName          = "Default Tenant"
	defaultAdminUsername       = "netweave-admin"
	defaultAdminEmail          = "admin@netweave.local"
	defaultAdminCertCN         = "admin.netweave.local"
	defaultAdminPortalPassword = "admin"
)

func newKeycloakCmd() *cobra.Command {
	var (
		adminUser     string
		adminPassword string
		realm         string
	)

	kcCmd := &cobra.Command{
		Use:   "keycloak",
		Short: "Bootstrap Keycloak with user profile, tenant, roles, and admin user",
		Long: `Configures the Keycloak realm with required user profile attributes,
creates the default tenant, initializes system roles with permissions
(platform-admin, tenant-admin, etc.), and creates the initial admin user
with certificate binding.`,
		RunE: func(c *cobra.Command, _ []string) error {
			return runKeycloakSetup(c.Context(), adminUser, adminPassword, realm)
		},
	}

	kcCmd.Flags().StringVar(
		&adminUser, "admin-user", defaultAdminUser,
		"Keycloak admin username",
	)
	kcCmd.Flags().StringVar(
		&adminPassword, "admin-password", defaultAdminPassword,
		"Keycloak admin password",
	)
	kcCmd.Flags().StringVar(
		&realm, "realm", defaultRealm,
		"Keycloak realm name",
	)

	return kcCmd
}

func runKeycloakSetup(
	ctx context.Context,
	adminUser, adminPassword, realm string,
) error {
	steps := cmd.Printer.NewStepProgress(5)
	namespace := cmd.Global.Namespace

	// Step 1: Connect to Keycloak.
	steps.Stepf("Connecting to Keycloak...")

	conn, err := service.NewConnector(cmd.Global.Kubeconfig, namespace)
	if err != nil {
		return fmt.Errorf("failed to create k8s connector: %w", err)
	}
	defer conn.Close()

	if waitErr := conn.WaitForPod(ctx, keycloakLabel, keycloakReadyTimeout); waitErr != nil {
		return fmt.Errorf("keycloak pod not ready: %w", waitErr)
	}

	fwd, err := conn.PortForward(ctx, keycloakLabel, keycloakPort)
	if err != nil {
		return fmt.Errorf("failed to port-forward to keycloak: %w", err)
	}
	defer close(fwd.StopChan)

	baseURL := fmt.Sprintf("http://localhost:%d", fwd.LocalPort)
	steps.Donef("Connected to Keycloak at %s", baseURL)

	// Step 2: Declare user profile attributes.
	steps.Stepf("Declaring user profile attributes...")

	if err := declareUserProfileAttributes(ctx, baseURL, realm, adminUser, adminPassword); err != nil {
		return fmt.Errorf("failed to declare user profile attributes: %w", err)
	}

	steps.Donef("User profile attributes declared")

	// Step 3: Create default tenant and initialize roles.
	steps.Stepf("Initializing default tenant and roles...")

	kcClient, store, err := createKeycloakStore(baseURL, realm, adminUser, adminPassword)
	if err != nil {
		return err
	}

	if err := store.CreateTenant(ctx, &auth.Tenant{
		ID:          defaultTenantID,
		Name:        defaultTenantName,
		Description: "Default tenant for Netweave",
		Status:      auth.TenantStatusActive,
		Quota: auth.TenantQuota{
			MaxSubscriptions:     100,
			MaxResourcePools:     50,
			MaxDeployments:       100,
			MaxUsers:             50,
			MaxRequestsPerMinute: 1000,
		},
	}); err != nil {
		if !strings.Contains(err.Error(), "already exists") {
			return fmt.Errorf("failed to create default tenant: %w", err)
		}
		cmd.Printer.Verbosef("Default tenant already exists, skipping")
	}

	if err := initializeOrUpdateRoles(ctx, store); err != nil {
		return fmt.Errorf("failed to initialize roles: %w", err)
	}

	steps.Donef("Default tenant and roles initialized")

	// Step 4: Create admin user.
	steps.Stepf("Creating admin user...")

	if err := createAdminUser(ctx, kcClient, store); err != nil {
		return err
	}

	steps.Donef("Admin user created")

	// Step 5: Summary.
	steps.Stepf("Verifying configuration...")

	roles, err := store.ListRoles(ctx)
	if err != nil {
		return fmt.Errorf("failed to list roles: %w", err)
	}

	steps.Donef("Keycloak configured with %d roles", len(roles))

	cmd.Printer.Success("Keycloak setup complete")
	return nil
}

// keycloakHTTPClient returns an HTTP client configured for Keycloak API calls.
func keycloakHTTPClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}

// declareUserProfileAttributes configures the Keycloak realm's user profile
// to include custom attributes needed by Netweave (tenant_id, role_id, etc.).
func declareUserProfileAttributes(
	ctx context.Context,
	baseURL, realm, adminUser, adminPassword string,
) error {
	// Get admin token.
	token, err := getKeycloakAdminToken(ctx, baseURL, adminUser, adminPassword)
	if err != nil {
		return err
	}

	// Build user profile configuration with custom attributes.
	profileCfg := buildUserProfileConfig()

	body, err := json.Marshal(profileCfg)
	if err != nil {
		return fmt.Errorf("failed to marshal user profile config: %w", err)
	}

	profileURL := fmt.Sprintf(
		"%s/admin/realms/%s/users/profile", baseURL, realm,
	)

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPut, profileURL, strings.NewReader(string(body)),
	)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	client := keycloakHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= 400 {
		respBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return fmt.Errorf(
				"failed to update user profile (status %d)", resp.StatusCode,
			)
		}
		return fmt.Errorf(
			"failed to update user profile (status %d): %s",
			resp.StatusCode, string(respBody),
		)
	}

	return nil
}

func getKeycloakAdminToken(
	ctx context.Context,
	baseURL, username, password string,
) (string, error) {
	tokenURL := fmt.Sprintf(
		"%s/realms/master/protocol/openid-connect/token", baseURL,
	)

	formData := fmt.Sprintf(
		"grant_type=password&client_id=%s&username=%s&password=%s",
		keycloak.AdminCLIClientID, username, password,
	)

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, tokenURL, strings.NewReader(formData),
	)
	if err != nil {
		return "", fmt.Errorf("failed to create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := keycloakHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf(
			"failed to get admin token (status %d)", resp.StatusCode,
		)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode token response: %w", err)
	}

	token, ok := result["access_token"].(string)
	if !ok {
		return "", fmt.Errorf("access_token not found in response")
	}

	return token, nil
}

func buildUserProfileConfig() map[string]interface{} {
	customAttributes := []map[string]interface{}{
		userProfileAttribute("tenant_id", "Tenant ID"),
		userProfileAttribute("role_id", "Role ID"),
		userProfileAttribute("certificate_subject", "Certificate Subject"),
		userProfileAttribute("oauth_subject", "OAuth Subject"),
		userProfileAttribute("oauth_provider", "OAuth Provider"),
		userProfileAttribute("is_active", "Is Active"),
		userProfileAttribute("last_login_at", "Last Login At"),
		userProfileAttribute("created_at", "Created At"),
		userProfileAttribute("updated_at", "Updated At"),
	}

	// Start with required Keycloak built-in attributes.
	allAttributes := []map[string]interface{}{
		{
			"name":        "username",
			"displayName": "Username",
			"validations": map[string]interface{}{
				"length": map[string]interface{}{
					"min": 3, "max": 255,
				},
				"username-prohibited-characters": map[string]interface{}{},
				"up-username-not-idn-homograph":  map[string]interface{}{},
			},
			"permissions": map[string]interface{}{
				"view": []string{"admin", "user"},
				"edit": []string{"admin", "user"},
			},
		},
		{
			"name":        "email",
			"displayName": "Email",
			"validations": map[string]interface{}{
				"email":  map[string]interface{}{},
				"length": map[string]interface{}{"max": 255},
			},
			"required": map[string]interface{}{
				"roles": []string{"user"},
			},
			"permissions": map[string]interface{}{
				"view": []string{"admin", "user"},
				"edit": []string{"admin", "user"},
			},
		},
		{
			"name":        "firstName",
			"displayName": "First name",
			"validations": map[string]interface{}{
				"length":                            map[string]interface{}{"max": 255},
				"person-name-prohibited-characters": map[string]interface{}{},
			},
			"permissions": map[string]interface{}{
				"view": []string{"admin", "user"},
				"edit": []string{"admin", "user"},
			},
		},
		{
			"name":        "lastName",
			"displayName": "Last name",
			"validations": map[string]interface{}{
				"length":                            map[string]interface{}{"max": 255},
				"person-name-prohibited-characters": map[string]interface{}{},
			},
			"permissions": map[string]interface{}{
				"view": []string{"admin", "user"},
				"edit": []string{"admin", "user"},
			},
		},
	}

	allAttributes = append(allAttributes, customAttributes...)

	return map[string]interface{}{
		"attributes": allAttributes,
		"groups":     []map[string]interface{}{},
	}
}

func userProfileAttribute(name, display string) map[string]interface{} {
	return map[string]interface{}{
		"name":        name,
		"displayName": display,
		"permissions": map[string]interface{}{
			"view": []string{"admin"},
			"edit": []string{"admin"},
		},
	}
}

func createKeycloakStore(
	baseURL, realm, adminUser, adminPassword string,
) (*keycloak.Client, *keycloak.Store, error) {
	kcClient, err := keycloak.NewClient(&keycloak.Config{
		BaseURL:       baseURL,
		Realm:         realm,
		ClientID:      keycloak.AdminCLIClientID,
		AdminUsername: adminUser,
		AdminPassword: adminPassword,
		Timeout:       30 * time.Second,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create keycloak client: %w", err)
	}

	logger := zap.NewNop()
	store := keycloak.NewStore(kcClient, logger)

	return kcClient, store, nil
}

func createAdminUser(ctx context.Context, client *keycloak.Client, store *keycloak.Store) error {
	// Get the platform-admin role.
	role, err := store.GetRoleByName(ctx, auth.RolePlatformAdmin)
	if err != nil {
		return fmt.Errorf("failed to get platform-admin role: %w", err)
	}

	adminUser := &auth.TenantUser{
		ID:         uuid.New().String(),
		TenantID:   defaultTenantID,
		CommonName: defaultAdminCertCN,
		Email:      defaultAdminEmail,
		Subject:    fmt.Sprintf("CN=%s,O=Netweave", defaultAdminCertCN),
		RoleID:     role.ID,
		IsActive:   true,
	}

	if err := store.CreateUser(ctx, adminUser); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			cmd.Printer.Verbosef("Admin user already exists, skipping")
			return nil
		}
		return fmt.Errorf("failed to create admin user: %w", err)
	}

	// Set a default password so the admin can log into the portal immediately.
	if err := client.SetUserPassword(ctx, adminUser.ID, defaultAdminPortalPassword, false); err != nil {
		return fmt.Errorf("failed to set admin user password: %w", err)
	}

	cmd.Printer.Verbosef(
		"Admin user created: %s (cert CN: %s, password: %s)",
		defaultAdminUsername, defaultAdminCertCN, defaultAdminPortalPassword,
	)

	return nil
}

// initializeOrUpdateRoles ensures all default roles exist with correct permissions.
// Unlike store.InitializeDefaultRoles, this also updates existing roles that
// may have been created without permissions by the Helm chart init jobs.
func initializeOrUpdateRoles(ctx context.Context, store *keycloak.Store) error {
	for _, role := range auth.GetDefaultRoles() {
		existing, err := store.GetRoleByName(ctx, role.Name)
		if err == nil {
			// Role exists — update with correct permissions and type.
			existing.Type = role.Type
			existing.Description = role.Description
			existing.Permissions = role.Permissions
			if updateErr := store.UpdateRole(ctx, existing); updateErr != nil {
				return fmt.Errorf("failed to update role %s: %w", role.Name, updateErr)
			}
			cmd.Printer.Verbosef("Role %q updated with %d permissions", role.Name, len(role.Permissions))
			continue
		}
		// Role doesn't exist — create it.
		if createErr := store.CreateRole(ctx, role); createErr != nil {
			return fmt.Errorf("failed to create role %s: %w", role.Name, createErr)
		}
		cmd.Printer.Verbosef("Role %q created with %d permissions", role.Name, len(role.Permissions))
	}
	return nil
}
