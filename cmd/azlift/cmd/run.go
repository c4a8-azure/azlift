package cmd

import "github.com/spf13/cobra"

func newRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the full azlift pipeline end-to-end",
		Long: `Orchestrate all four pipeline stages in sequence:
  1. scan       — build resource inventory and dependency graph
  2. export     — export via aztfexport
  3. refine     — transform HCL into structured Terraform or Terragrunt
  4. bootstrap  — provision CI/CD plumbing (skippable with --no-bootstrap)`,
		RunE: notImplemented,
	}

	cmd.Flags().String("resource-group", "", "Resource group to process (required)")
	cmd.Flags().String("repo-name", "", "Name of the Git repository to create (required with bootstrap)")
	cmd.Flags().String("mode", "modules", "Output mode: modules or terragrunt")
	cmd.Flags().String("platform", "github", "CI/CD platform: github or ado")
	cmd.Flags().Bool("enrich", false, "Run AI enrichment pass")
	cmd.Flags().Bool("no-bootstrap", false, "Skip bootstrap; run terraform plan against refined output instead")
	cmd.Flags().Bool("dry-run", false, "Print planned actions without executing any external tools")
	_ = cmd.MarkFlagRequired("resource-group")

	return cmd
}
