package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/c4a8-azure/azlift/internal/enrich"
	"github.com/c4a8-azure/azlift/internal/refine"
	"github.com/c4a8-azure/azlift/internal/terragrunt"
)

func newRefineCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "refine",
		Short: "Transform raw aztfexport HCL into structured Terraform or Terragrunt",
		Long: `Parse and restructure raw aztfexport output through a multi-step pipeline:
variable extraction, semantic naming analysis, resource grouping into logical files,
and backend/provider generation.

--enrich activates the post-refine quality pass:
  - lifecycle { prevent_destroy = true } injected on stateful resources
  - security anti-patterns detected (and optionally fixed with --fix-security)
  - tag policy normalised to merge(local.common_tags, {...})
  - AI descriptions added to variable/output blocks (requires ANTHROPIC_API_KEY)

--fix-security can be used alone (without --enrich) to apply safe security
auto-remediations to the output files.`,
		RunE: runRefine,
	}

	cmd.Flags().String("input-dir", "./raw", "Directory containing raw aztfexport output")
	cmd.Flags().String("output-dir", "./refined", "Directory to write refined Terraform output")
	cmd.Flags().String("mode", "modules", "Output mode: modules or terragrunt")
	cmd.Flags().String("resource-group", "", "Resource group name (used for backend state key)")
	cmd.Flags().String("terraform-version", "", "Minimum Terraform version constraint injected when the input omits required_version (default: \""+refine.DefaultMinTerraformVersion+"\")")
	cmd.Flags().Bool("enrich", false, "Run full enrichment pass (lifecycle, security, tags, AI descriptions)")
	cmd.Flags().Bool("fix-security", false, "Auto-remediate safe security anti-patterns in the output")
	cmd.Flags().Bool("skip-lint", false, "Skip the tflint pass")
	cmd.Flags().Bool("skip-docs", false, "Skip terraform-docs generation")

	return cmd
}

func runRefine(cmd *cobra.Command, _ []string) error {
	inputDir, _ := cmd.Flags().GetString("input-dir")
	outputDir, _ := cmd.Flags().GetString("output-dir")
	mode, _ := cmd.Flags().GetString("mode")
	rg, _ := cmd.Flags().GetString("resource-group")
	tfVersion, _ := cmd.Flags().GetString("terraform-version")
	doEnrich, _ := cmd.Flags().GetBool("enrich")
	fixSecurity, _ := cmd.Flags().GetBool("fix-security")
	skipLint, _ := cmd.Flags().GetBool("skip-lint")
	skipDocs, _ := cmd.Flags().GetBool("skip-docs")

	// --fix-security implies running the enrichment pass.
	if fixSecurity {
		doEnrich = true
	}

	log := Log.WithStage("REFINE")
	log.Info(fmt.Sprintf("refining %s → %s (mode: %s)", inputDir, outputDir, mode))

	// Run the core modules-mode pipeline.
	result, err := refine.Run(cmd.Context(), refine.Options{
		InputDir:            inputDir,
		OutputDir:           outputDir,
		ResourceGroup:       rg,
		MinTerraformVersion: tfVersion,
		SkipLint:            skipLint || mode == "terragrunt",
		SkipDocs:            skipDocs,
	})
	if err != nil {
		return err
	}

	log.Info(fmt.Sprintf("wrote %d files to %s", len(result.Files), outputDir))
	if result.StateCopied {
		log.Info("terraform.tfstate copied to output directory for bootstrap stage")
	}

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

	// Enrichment pass.
	if doEnrich {
		log.Info("starting enrichment pass")

		apiKey := os.Getenv("ANTHROPIC_API_KEY")
		if apiKey == "" {
			log.Info("ANTHROPIC_API_KEY not set — AI description generation will be skipped")
		} else {
			log.Info(fmt.Sprintf("ANTHROPIC_API_KEY detected — AI descriptions will use model %s", enrich.DefaultModel))
		}

		// Locate locals.tf from the refine output.
		var localsFile *refine.ParsedFile
		for _, pf := range result.Files {
			if len(pf.Path) >= 9 && pf.Path[len(pf.Path)-9:] == "locals.tf" {
				localsFile = pf
				break
			}
		}

		enrichResult, err := enrich.Run(cmd.Context(), result.Files, localsFile, enrich.Options{
			APIKey:      apiKey,
			FixSecurity: fixSecurity,
			Log:         Log.Slog(),
		})
		if err != nil {
			return fmt.Errorf("enrichment: %w", err)
		}

		if enrichResult.AnalysisFile != "" {
			log.Info(fmt.Sprintf("architecture analysis: %s (injected into README.md)", enrichResult.AnalysisFile))
		}

		// Re-write enriched files to disk.
		for _, pf := range result.Files {
			if err := refine.WriteFile(pf); err != nil {
				return fmt.Errorf("writing enriched %s: %w", pf.Path, err)
			}
		}
		if localsFile != nil {
			if err := refine.WriteFile(localsFile); err != nil {
				return fmt.Errorf("writing enriched locals.tf: %w", err)
			}
		}
		log.Info("enrichment pass complete")
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
