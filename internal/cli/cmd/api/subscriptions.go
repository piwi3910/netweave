package apicommands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/piwi3910/netweave/internal/cli/cmd"
)

func newSubscriptionsCmd() *cobra.Command {
	parent := &cobra.Command{
		Use:     "subscriptions",
		Aliases: []string{"sub"},
		Short:   "Manage subscriptions",
	}

	parent.AddCommand(newSubListCmd())
	parent.AddCommand(newSubCreateCmd())
	parent.AddCommand(newSubDeleteCmd())

	return parent
}

func newSubListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List subscriptions",
		RunE: func(c *cobra.Command, _ []string) error {
			return fetchAndRenderList(c, o2imsBasePath+"/subscriptions", []tableColumn{
				{"ID", "subscriptionId"},
				{"CALLBACK", "callback"},
				{"FILTER", "filter"},
			})
		},
	}
}

func newSubCreateCmd() *cobra.Command {
	var (
		callback string
		filter   string
	)

	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a subscription",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()

			if callback == "" {
				return fmt.Errorf("--callback flag is required")
			}

			client, err := newAPIClient(ctx, c)
			if err != nil {
				return err
			}
			defer client.Close()

			payload := map[string]string{
				"callback": callback,
			}
			if filter != "" {
				payload["filter"] = filter
			}

			jsonBody, err := json.Marshal(payload)
			if err != nil {
				return fmt.Errorf("failed to marshal request: %w", err)
			}

			body, statusCode, err := client.doRequest(
				ctx, http.MethodPost,
				o2imsBasePath+"/subscriptions",
				bytes.NewReader(jsonBody),
			)
			if err != nil {
				return fmt.Errorf("failed to create subscription: %w", err)
			}

			if statusCode >= 400 {
				return fmt.Errorf(
					"failed to create subscription (status %d): %s",
					statusCode, string(body),
				)
			}

			var result map[string]interface{}
			if jsonErr := json.Unmarshal(body, &result); jsonErr != nil {
				return fmt.Errorf("failed to parse response: %w", jsonErr)
			}

			return cmd.Printer.PrintResult(result, func() {
				cmd.Printer.Infof("Subscription created: %s", jsonString(result, "subscriptionId"))
				cmd.Printer.Infof("Callback: %s", callback)
			})
		},
	}

	createCmd.Flags().StringVar(&callback, "callback", "", "Callback URL for notifications (required)")
	createCmd.Flags().StringVar(&filter, "filter", "", "Optional subscription filter")
	return createCmd
}

func newSubDeleteCmd() *cobra.Command {
	var subID string

	deleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a subscription",
		RunE: func(c *cobra.Command, _ []string) error {
			ctx := c.Context()

			if subID == "" {
				return fmt.Errorf("--id flag is required")
			}

			client, err := newAPIClient(ctx, c)
			if err != nil {
				return err
			}
			defer client.Close()

			path := fmt.Sprintf("%s/subscriptions/%s", o2imsBasePath, subID)
			_, statusCode, err := client.doRequest(
				ctx, http.MethodDelete, path, nil,
			)
			if err != nil {
				return fmt.Errorf("failed to delete subscription: %w", err)
			}

			if statusCode >= 400 {
				return fmt.Errorf(
					"failed to delete subscription (status %d)", statusCode,
				)
			}

			cmd.Printer.Infof("Subscription %s deleted", subID)
			return nil
		},
	}

	deleteCmd.Flags().StringVar(&subID, "id", "", "Subscription ID to delete (required)")
	return deleteCmd
}
