package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/c4a8-azure/azlift/internal/refine"
	"github.com/c4a8-azure/azlift/internal/terragrunt"
)

func newRefineCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "refine",
		Short: "Transform raw aztfexport HCL into structured Terraform or Terragrunt",
		Long: `Parse and restructure raw aztfexport output through a multi-step pipeline:
variable extraction, semantic naming analysis, resource grouping into logical files,
and backend/provider generation. Optionally produces a Terragrunt layered structure
or runs an AI enrichment pass.`,
		RunE: runRefine,
	}

	cmd.Flags().String("input-dir", "./raw", "Directory containing raw aztfexport output")
	cmd.Flags().String("output-dir", "./refined", "Directory to write refined Terraform output")
	cmd.Flags().String("mode", "modules", "Output mode: modules or terragrunt")
	cmd.Flags().String("resource-group", "", "Resource group name (used for backend state key)")
	cmd.Flags().Bool("enrich", false, "Run AI enrichment pass after deterministic transformation")
	cmd.Flags().Bool("skip-lint", false, "Skip the tflint pass")
	cmd.Flags().Bool("skip-docs", false, "Skip terraform-docs generation")

	return cmd
}

func runRefine(cmd *cobra.Command, _ []string) error {
	inputDir, _ := cmd.Flags().GetString("input-dir")
	outputDir, _ := cmd.Flags().GetString("output-dir")
	mode, _ := cmd.Flags().GetString("mode")
	rg, _ := cmd.Flags().GetString("resource-group")
	skipLint, _ := cmd.Flags().GetBool("skip-lint")
	skipDocs, _ := cmd.Flags().GetBool("skip-docs")

	log := Log.WithStage("REFINE")
	log.Info(fmt.Sprintf("refining %s → %s (mode: %s)", inputDir, outputDir, mode))

	// Run the core modules-mode pipeline regardless of output mode.
	// For Terragrunt mode the grouped files become module sources.
	result, err := refine.Run(cmd.Context(), refine.Options{
		InputDir:      inputDir,
		OutputDir:     outputDir,
		ResourceGroup: rg,
		SkipLint:      skipLint || mode == "terragrunt", // tflint not meaningful on TG layout
		SkipDocs:      skipDocs,
	})
	if err != nil {
		return err
	}

	log.Info(fmt.Sprintf("wrote %d files to %s", len(result.Files), outputDir))

	if result.Lint.Skipped {
		log.Info("lint skipped")
	} else {
		log.Info(fmt.Sprintf("lint: %d issue(s)", result.Lint.Issues))
	}

	if result.Docs.Skipped {
		log.Info("docs skipped")
	} else {
		log.Info("docs: README.md generated")
	}

	if mode == "terragrunt" {
		log.Info("generating Terragrunt layout")
		tgOpts := terragrunt.DefaultOptions(outputDir)
		if err := terragrunt.Run(result.Files, tgOpts); err != nil {
			return fmt.Errorf("terragrunt layout: %w", err)
		}
		log.Info("Terragrunt layout written")
	}

	return nil
}
