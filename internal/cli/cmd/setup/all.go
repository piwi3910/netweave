package setup

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/piwi3910/netweave/internal/cli/cmd"
)

func newAllCmd() *cobra.Command {
	var (
		chartPath  string
		valuesFile string
	)

	allCmd := &cobra.Command{
		Use:   "all",
		Short: "Run complete setup: vault, helm, keycloak, certs",
		Long: `Orchestrates the full Netweave deployment sequence:
  1. Deploy and configure Vault (TLS, init, unseal, PKI)
  2. Issue certificates (server + client TLS)
  3. Install Helm chart (PostgreSQL, Redis, Keycloak, Gateway)
  4. Bootstrap Keycloak (profile attrs, tenant, roles, admin user)

Each step is idempotent and skips if already completed.`,
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()

			cmd.Printer.Infof("Starting full Netweave setup...")
			cmd.Printer.Separator()

			// Step 1: Vault setup.
			cmd.Printer.Infof("[Phase 1/4] Setting up Vault...")
			if err := runVaultSetup(nil, nil); err != nil {
				return fmt.Errorf("vault setup failed: %w", err)
			}
			cmd.Printer.Separator()

			// Step 2: Issue certificates.
			cmd.Printer.Infof("[Phase 2/4] Issuing certificates...")
			if err := runCertsSetup(ctx, serverCertCN, clientCertCN); err != nil {
				return fmt.Errorf("certificate setup failed: %w", err)
			}
			cmd.Printer.Separator()

			// Step 3: Helm install.
			cmd.Printer.Infof("[Phase 3/4] Installing Helm chart...")
			if err := runHelmSetup(ctx, chartPath, valuesFile, false); err != nil {
				return fmt.Errorf("helm setup failed: %w", err)
			}
			cmd.Printer.Separator()

			// Step 4: Keycloak bootstrap.
			cmd.Printer.Infof("[Phase 4/4] Bootstrapping Keycloak...")
			if err := runKeycloakSetup(
				ctx,
				defaultAdminUser,
				defaultAdminPassword,
				defaultRealm,
			); err != nil {
				return fmt.Errorf("keycloak setup failed: %w", err)
			}

			cmd.Printer.Separator()
			cmd.Printer.Success("Full Netweave setup complete!")
			return nil
		},
	}

	allCmd.Flags().StringVar(
		&chartPath, "chart-path", defaultChartPath,
		"Path to the Netweave Helm chart directory",
	)
	allCmd.Flags().StringVar(
		&valuesFile, "values-file", "",
		"Path to a values override file",
	)

	return allCmd
}
