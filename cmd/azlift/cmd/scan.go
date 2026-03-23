package cmd

import "github.com/spf13/cobra"

func newScanCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Build Azure resource inventory and cross-RG dependency graph",
		Long: `Query the Azure Resource Graph API to inventory all resources in a subscription,
analyse cross-resource-group dependencies, and recommend module/state boundaries
before any export is performed.`,
		RunE: notImplemented,
	}

	cmd.Flags().String("output-dir", ".", "Directory to write scan results")

	return cmd
}
