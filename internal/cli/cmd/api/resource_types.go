package apicommands

import (
	"github.com/spf13/cobra"
)

func newResourceTypesCmd() *cobra.Command {
	parent := &cobra.Command{
		Use:     "resource-types",
		Aliases: []string{"rt"},
		Short:   "Manage resource types",
	}

	parent.AddCommand(newRTListCmd())

	return parent
}

func newRTListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List resource types",
		RunE: func(c *cobra.Command, _ []string) error {
			return fetchAndRenderList(c, o2imsBasePath+"/resourceTypes", []tableColumn{
				{"ID", "resourceTypeId"},
				{"NAME", "name"},
				{"VENDOR", "vendor"},
				{"MODEL", "model"},
				{"VERSION", "version"},
			})
		},
	}
}
