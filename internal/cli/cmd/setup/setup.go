// Package setup provides commands for deploying and configuring the Netweave stack.
package setup

import (
	"github.com/spf13/cobra"
)

// NewSetupCmd creates the parent "setup" command.
func NewSetupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Deploy and configure the Netweave stack",
		Long: `Setup commands automate deployment of Vault, Helm charts, Keycloak
bootstrapping, and certificate issuance. Each step is idempotent and can
be re-run safely.

Use "setup all" to perform a complete deployment from scratch, or run
individual steps to configure specific components.`,
	}

	cmd.AddCommand(newVaultCmd())
	cmd.AddCommand(newHelmCmd())
	cmd.AddCommand(newKeycloakCmd())
	cmd.AddCommand(newCertsCmd())
	cmd.AddCommand(newAllCmd())
	cmd.AddCommand(newTeardownCmd())

	return cmd
}
