// Package cmd defines the CLI command hierarchy for the netweave-cli tool.
package cmd

import (
	"github.com/spf13/cobra"

	"github.com/piwi3910/netweave/internal/cli/output"
)

// GlobalFlags holds flags shared across all commands.
type GlobalFlags struct {
	Namespace  string
	Kubeconfig string
	JSONOutput bool
	Verbose    bool
}

// Global holds the parsed global flags.
var Global GlobalFlags

// Printer is the shared output printer, initialized after flag parsing.
var Printer *output.Printer

// NewRootCmd creates the root cobra command for netweave-cli.
func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "netweave-cli",
		Short: "Netweave O2-IMS management CLI",
		Long: `netweave-cli is the command-line interface for deploying, managing, and
interacting with the Netweave O2-IMS gateway.

It automates cluster setup (Vault, Helm, Keycloak, certificates) and provides
full CRUD management for tenants, users, roles, and O2-IMS API resources.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRun: func(c *cobra.Command, _ []string) {
			Printer = output.NewPrinter(Global.JSONOutput, Global.Verbose)
			Printer.SetOutput(c.OutOrStdout())
			Printer.SetErrOutput(c.ErrOrStderr())
		},
	}

	// Global persistent flags
	pf := cmd.PersistentFlags()
	pf.StringVarP(&Global.Namespace, "namespace", "n", "netweave", "Kubernetes namespace")
	pf.StringVar(&Global.Kubeconfig, "kubeconfig", "",
		"Path to kubeconfig file (default: $KUBECONFIG or ~/.kube/config)")
	pf.BoolVar(&Global.JSONOutput, "json", false, "Output in JSON format")
	pf.BoolVarP(&Global.Verbose, "verbose", "v", false, "Enable verbose output")

	return cmd
}
