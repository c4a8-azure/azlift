package cmd

import "github.com/spf13/cobra"

func newRefineCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "refine",
		Short: "Transform raw aztfexport HCL into structured Terraform or Terragrunt",
		Long: `Parse and restructure raw aztfexport output through a multi-step pipeline:
variable extraction, semantic naming analysis, resource grouping into logical files,
and backend/provider generation. Optionally produces a Terragrunt layered structure
or runs an AI enrichment pass.`,
		RunE: notImplemented,
	}

	cmd.Flags().String("input-dir", "./raw", "Directory containing raw aztfexport output")
	cmd.Flags().String("output-dir", "./refined", "Directory to write refined Terraform output")
	cmd.Flags().String("mode", "modules", "Output mode: modules or terragrunt")
	cmd.Flags().Bool("enrich", false, "Run AI enrichment pass after deterministic transformation")

	return cmd
}
