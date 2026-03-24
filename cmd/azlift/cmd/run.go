package cmd

import "github.com/spf13/cobra"

func newRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the full azlift pipeline end-to-end",
		Long: `Orchestrate all pipeline stages in sequence:

  1. scan       — build resource inventory and dependency graph
  2. export     — export via aztfexport
  3. refine     — transform HCL into structured Terraform
  4. repo init  — git init, embed workflows, generate bootstrap/ module
  5. github     — create GitHub repo and push
  6. activate   — provision state storage, MIs, OIDC, RBAC (same-tenant)
                  or generate bootstrap/ module for manual apply (cross-tenant)

Cross-tenant mode is detected automatically when --target-tenant differs from
the source tenant. In cross-tenant mode stage 6 is skipped — apply the
bootstrap/ Terraform module in the target tenant to activate CI/CD.

Example (same-tenant):
  azlift run \
    --subscription <id> \
    --resource-group rg-myapp-prod \
    --repo-name infra-prod \
    --org my-org

Example (cross-tenant):
  azlift run \
    --subscription <source-id> \
    --target-subscription <target-id> \
    --target-tenant <target-tenant-id> \
    --resource-group rg-myapp-prod \
    --repo-name infra-prod \
    --org my-org`,
		RunE: notImplemented,
	}

	// Source
	cmd.Flags().String("resource-group", "", "Resource group to export (required)")
	cmd.Flags().StringSlice("resource-groups", nil, "All resource groups in scope (for RBAC; defaults to --resource-group)")

	// Target
	cmd.Flags().String("target-subscription", "", "Target subscription for CI/CD resources (defaults to --subscription)")
	cmd.Flags().String("target-tenant", "", "Target Azure AD tenant (if different → cross-tenant mode)")

	// Repository
	cmd.Flags().String("repo-name", "", "Name of the Git repository to create (required)")
	cmd.Flags().String("org", "", "GitHub organisation (required)")

	// Managed Identities
	cmd.Flags().String("mi-resource-group", "", "RG for Managed Identities (defaults to --resource-group)")

	// Output
	cmd.Flags().String("mode", "modules", "Output mode: modules or terragrunt")
	cmd.Flags().StringSlice("environments", []string{"prod", "staging", "dev"}, "Deployment environments (comma-separated)")
	cmd.Flags().String("location", "westeurope", "Azure region for state storage")
	cmd.Flags().String("tenant-id", "", "Source Azure AD tenant ID (auto-detected if empty)")
	cmd.Flags().String("work-dir", ".azlift", "Base directory for pipeline outputs")
	cmd.Flags().String("workflows-dir", "", "Custom GitHub Actions workflows directory (default: embedded)")

	// Enrichment
	cmd.Flags().Bool("enrich", false, "Run AI enrichment pass (lifecycle, security, tags, descriptions)")
	cmd.Flags().Bool("fix-security", false, "Auto-remediate safe security anti-patterns")

	// Misc
	cmd.Flags().Bool("dry-run", false, "Print planned actions without executing any external tools")
	cmd.Flags().Bool("skip-lint", false, "Skip the tflint pass")
	cmd.Flags().Bool("skip-docs", false, "Skip terraform-docs generation")

	_ = cmd.MarkFlagRequired("resource-group")

	return cmd
}
