package apicommands

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newResourcesCmd() *cobra.Command {
	parent := &cobra.Command{
		Use:   "resources",
		Short: "Manage resources",
	}

	parent.AddCommand(newResListCmd())

	return parent
}

func newResListCmd() *cobra.Command {
	var poolID string

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List resources",
		RunE: func(c *cobra.Command, _ []string) error {
			var path string
			if poolID != "" {
				path = fmt.Sprintf(
					"%s/resourcePools/%s/resources", o2imsBasePath, poolID,
				)
			} else {
				path = o2imsBasePath + "/resources"
			}

			return fetchAndRenderList(c, path, []tableColumn{
				{"ID", "resourceId"},
				{"TYPE", "resourceTypeId"},
				{"POOL ID", "resourcePoolId"},
				{"DESCRIPTION", "description"},
			})
		},
	}

	listCmd.Flags().StringVar(&poolID, "pool-id", "", "Filter by resource pool ID")
	return listCmd
}
