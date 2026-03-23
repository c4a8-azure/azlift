package cmd

import "github.com/spf13/cobra"

func newExportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export Azure resources via aztfexport",
		Long: `Wrap aztfexport per logical boundary (typically per resource group) with retry
logic for API throttling, exclusion lists for resources that should not be in IaC,
and mapping of unsupported resources to data sources.`,
		RunE: notImplemented,
	}

	cmd.Flags().String("resource-group", "", "Resource group to export (required)")
	cmd.Flags().String("output-dir", "./raw", "Directory to write raw aztfexport output")
	cmd.Flags().StringSlice("exclude", nil, "Resource IDs to exclude from export")
	_ = cmd.MarkFlagRequired("resource-group")

	return cmd
}
