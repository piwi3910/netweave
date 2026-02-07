package apicommands

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/piwi3910/netweave/internal/cli/cmd"
)

func newHealthCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "health",
		Short: "Check gateway health status",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()

			client, err := newAPIClient(ctx, c)
			if err != nil {
				return err
			}
			defer client.Close()

			body, err := client.doGet(ctx, "/healthz")
			if err != nil {
				return fmt.Errorf("health check failed: %w", err)
			}

			var result map[string]interface{}
			if jsonErr := json.Unmarshal(body, &result); jsonErr != nil {
				return cmd.Printer.PrintResult(string(body), func() {
					cmd.Printer.Success("Gateway is healthy")
				})
			}

			return cmd.Printer.PrintResult(result, func() {
				status := jsonString(result, "status")
				if status == "" {
					status = "ok"
				}
				cmd.Printer.Infof("Status: %s", status)
				cmd.Printer.Success("Gateway is healthy")
			})
		},
	}
}
