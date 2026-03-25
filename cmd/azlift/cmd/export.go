package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/c4a8-azure/azlift/internal/export"
	"github.com/c4a8-azure/azlift/internal/logger"
)

func newExportCmd() *cobra.Command {
	var outputDir string
	var excludeTypes []string
	var useAzureAD bool

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export Azure resources via aztfexport",
		Long: `Wrap aztfexport per logical boundary (typically per resource group) with retry
logic for API throttling, exclusion lists for resources that should not be in IaC,
and mapping of unsupported resources to data sources.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resourceGroup, _ := cmd.Flags().GetString("resource-group")
			subscriptionID, _ := cmd.Root().PersistentFlags().GetString("subscription")
			if subscriptionID == "" {
				return fmt.Errorf("--subscription is required")
			}

			log := Log.WithStage(logger.StageExport)
			ctx := cmd.Context()

			// Build exclusion list.
			exclusions := export.NewExclusionList(excludeTypes)
			log.Info("exclusion list ready", "default_count", len(export.DefaultExcludedTypes), "extra", len(excludeTypes))

			// Prepare output directory.
			rgDir, err := export.PrepareOutputDir(outputDir, resourceGroup)
			if err != nil {
				return err
			}
			log.Info("output directory prepared", "path", rgDir)

			// Compose aztfexport args for resource-group mode.
			args := []string{
				"resource-group",
				"--subscription-id", subscriptionID,
				"--output-dir", rgDir,
				"--non-interactive",
				resourceGroup,
			}

			runExport := func(extraEnv []string) ([]string, error) {
				baseRunner := &export.AztfexportRunner{ExtraEnv: extraEnv}
				runner := export.NewRetryRunner(baseRunner)
				var lines []string
				err := runner.Run(ctx, args, func(line string) {
					log.Debug(line)
					lines = append(lines, line)
				})
				return lines, err
			}

			// If --use-azuread is set, inject Azure AD env vars from the start.
			var initialEnv []string
			if useAzureAD {
				initialEnv = export.AzureADEnv
				log.Info("Azure AD storage auth enabled via --use-azuread")
			}

			log.Info("running aztfexport", "resource_group", resourceGroup, "output_dir", rgDir)
			outputLines, runErr := runExport(initialEnv)

			// Auto-detect Azure AD storage auth errors and retry transparently.
			if runErr != nil && !useAzureAD && export.IsAzureADAuthError(runErr.Error(), joinLines(outputLines)) {
				log.Warn("aztfexport failed: storage account requires Azure AD auth — retrying with ARM_USE_AZUREAD=true and ARM_STORAGE_USE_AZUREAD=true")
				log.Warn("tip: pass --use-azuread to skip this auto-detection on future runs")
				outputLines, runErr = runExport(export.AzureADEnv)
			}

			if runErr != nil {
				log.Warn("aztfexport failed — full output:")
				for _, l := range outputLines {
					log.Warn(l)
				}
				return fmt.Errorf("aztfexport failed: %w", runErr)
			}
			log.Info("aztfexport complete")

			// Write manifest.
			manifest := &export.Manifest{
				SchemaVersion:  "1",
				SubscriptionID: subscriptionID,
				ResourceGroup:  resourceGroup,
				OutputDir:      rgDir,
			}
			// Log excluded types for user visibility.
			for _, t := range export.DefaultExcludedTypes {
				if exclusions.IsExcluded(t) {
					manifest.ExcludedResources = append(manifest.ExcludedResources, t)
				}
			}

			manifestPath, err := export.WriteManifest(manifest, rgDir)
			if err != nil {
				return err
			}
			log.Info("manifest written", "path", manifestPath)

			return nil
		},
	}

	cmd.Flags().String("resource-group", "", "Resource group to export (required)")
	cmd.Flags().StringVar(&outputDir, "output-dir", "./raw", "Base directory for raw aztfexport output")
	cmd.Flags().StringSliceVar(&excludeTypes, "exclude-types", nil, "Additional resource types to exclude (comma-separated)")
	cmd.Flags().BoolVar(&useAzureAD, "use-azuread", false, "Force Azure AD auth for the storage backend (sets ARM_USE_AZUREAD and ARM_STORAGE_USE_AZUREAD)")
	_ = cmd.MarkFlagRequired("resource-group")

	return cmd
}

func joinLines(lines []string) string {
	result := ""
	for _, l := range lines {
		result += l + "\n"
	}
	return result
}
