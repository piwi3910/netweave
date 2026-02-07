package apicommands

import (
	"github.com/spf13/cobra"
)

func newResourcePoolsCmd() *cobra.Command {
	parent := &cobra.Command{
		Use:     "resource-pools",
		Aliases: []string{"rp"},
		Short:   "Manage resource pools",
	}

	parent.AddCommand(newRPListCmd())

	return parent
}

func newRPListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List resource pools",
		RunE: func(c *cobra.Command, _ []string) error {
			return fetchAndRenderList(c, o2imsBasePath+"/resourcePools", []tableColumn{
				{"ID", "resourcePoolId"},
				{"NAME", "name"},
				{"DESCRIPTION", "description"},
				{"LOCATION", "location"},
			})
		},
	}
}
