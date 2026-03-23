package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/c4a8-azure/azlift/internal/scan"
)

func newScanCmd() *cobra.Command {
	var outputDir string

	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Build Azure resource inventory and cross-RG dependency graph",
		Long: `Query the Azure Resource Graph API to inventory all resources in a subscription,
analyse cross-resource-group dependencies, and recommend module/state boundaries
before any export is performed.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			subscriptionID, _ := cmd.Flags().GetString("subscription")
			if subscriptionID == "" {
				subscriptionID, _ = cmd.Root().PersistentFlags().GetString("subscription")
			}
			if subscriptionID == "" {
				return fmt.Errorf("--subscription is required")
			}

			log := Log.WithStage(scan.StageLabel)
			ctx := cmd.Context()

			log.Info("building Resource Graph client")
			client, err := scan.NewClient()
			if err != nil {
				log.Error("authentication failed", "err", err)
				return fmt.Errorf("authentication failed: %w", err)
			}

			log.Info("querying resource inventory", "subscription", subscriptionID)
			groups, err := scan.Inventory(ctx, client, subscriptionID)
			if err != nil {
				return fmt.Errorf("inventory query: %w", err)
			}
			log.Info("inventory complete", "resource_groups", len(groups))

			log.Info("analysing cross-RG dependencies")
			graph := scan.AnalyseDependencies(groups)
			log.Info("dependency analysis complete", "edges", len(graph.Edges))

			// Render output
			format, _ := cmd.Root().PersistentFlags().GetString("output")
			if format == "json" {
				if err := scan.PrintJSON(os.Stdout, groups); err != nil {
					return fmt.Errorf("writing JSON: %w", err)
				}
			} else {
				scan.PrintTable(os.Stdout, groups)
			}

			result := scan.BuildResult(subscriptionID, groups, graph)
			scan.PrintRecommendations(os.Stdout, result)

			path, err := scan.SaveResult(result, outputDir)
			if err != nil {
				return fmt.Errorf("saving scan result: %w", err)
			}
			log.Info("scan result saved", "path", path)

			return nil
		},
	}

	cmd.Flags().StringVar(&outputDir, "output-dir", ".", "Directory to write scan-result.json")

	return cmd
}
