package apicommands

import (
	"github.com/spf13/cobra"
)

func newDeploymentManagersCmd() *cobra.Command {
	parent := &cobra.Command{
		Use:     "deployment-managers",
		Aliases: []string{"dm"},
		Short:   "Manage deployment managers",
	}

	parent.AddCommand(newDMListCmd())

	return parent
}

func newDMListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List deployment managers",
		RunE: func(c *cobra.Command, _ []string) error {
			return fetchAndRenderList(c, o2imsBasePath+"/deploymentManagers", []tableColumn{
				{"ID", "deploymentManagerId"},
				{"NAME", "name"},
				{"DESCRIPTION", "description"},
			})
		},
	}
}
