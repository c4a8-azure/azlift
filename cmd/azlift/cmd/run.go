package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/c4a8-azure/azlift/internal/run"
)

func newRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the full azlift pipeline end-to-end",
		Long: `Orchestrate all pipeline stages in sequence:

  1. scan       — build resource inventory and cross-RG dependency graph
  2. export     — export via aztfexport
  3. refine     — transform HCL into structured Terraform (or Terragrunt)
  4. repo init  — git init, embed CI/CD workflows, generate bootstrap/ module
  5. github     — create GitHub repository and push
  6. activate   — provision state storage, MIs, OIDC, RBAC, upload tfstate
                  (same-tenant; skipped for cross-tenant — apply bootstrap/ instead)

Cross-tenant mode is detected automatically when --target-tenant differs from
the source tenant.

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
		RunE: runPipeline,
	}

	// ── Source ────────────────────────────────────────────────────────────────
	cmd.Flags().String("resource-group", "", "Resource group to export (required)")
	cmd.Flags().StringSlice("resource-groups", nil, "All RGs in scope for RBAC (defaults to --resource-group)")

	// ── Target ────────────────────────────────────────────────────────────────
	cmd.Flags().String("target-subscription", "", "Target subscription for CI/CD resources (defaults to --subscription)")
	cmd.Flags().String("target-tenant", "", "Target Azure AD tenant (if different → cross-tenant mode)")
	cmd.Flags().String("tenant-id", "", "Source Azure AD tenant ID (auto-detected if empty)")

	// ── Repository ────────────────────────────────────────────────────────────
	cmd.Flags().String("repo-name", "", "Name of the Git repository to create")
	cmd.Flags().String("org", "", "GitHub organisation")
	cmd.Flags().String("mi-resource-group", "", "RG for Managed Identities (defaults to --resource-group)")

	// ── Output ────────────────────────────────────────────────────────────────
	cmd.Flags().String("mode", "modules", "Output mode: modules or terragrunt")
	cmd.Flags().StringSlice("environments", []string{"prod", "dev"}, "Deployment environments (comma-separated)")
	cmd.Flags().String("location", "westeurope", "Azure region for state storage")
	cmd.Flags().String("work-dir", ".azlift", "Base directory for all pipeline outputs")
	cmd.Flags().String("workflows-dir", "", "Custom GitHub Actions workflows directory (default: embedded)")

	// ── Enrichment ────────────────────────────────────────────────────────────
	cmd.Flags().Bool("enrich", false, "Run AI enrichment pass (lifecycle, security, tags, descriptions)")
	cmd.Flags().Bool("fix-security", false, "Auto-remediate safe security anti-patterns")

	// ── Misc ──────────────────────────────────────────────────────────────────
	cmd.Flags().Bool("dry-run", false, "Print planned actions without executing any external tools")
	cmd.Flags().Bool("skip-lint", false, "Skip the tflint pass")
	cmd.Flags().Bool("skip-docs", false, "Skip terraform-docs generation")

	_ = cmd.MarkFlagRequired("resource-group")

	return cmd
}

func runPipeline(cmd *cobra.Command, _ []string) error {
	resourceGroup, _ := cmd.Flags().GetString("resource-group")
	resourceGroups, _ := cmd.Flags().GetStringSlice("resource-groups")
	targetSub, _ := cmd.Flags().GetString("target-subscription")
	targetTenant, _ := cmd.Flags().GetString("target-tenant")
	tenantID, _ := cmd.Flags().GetString("tenant-id")
	repoName, _ := cmd.Flags().GetString("repo-name")
	org, _ := cmd.Flags().GetString("org")
	miRG, _ := cmd.Flags().GetString("mi-resource-group")
	mode, _ := cmd.Flags().GetString("mode")
	envs, _ := cmd.Flags().GetStringSlice("environments")
	location, _ := cmd.Flags().GetString("location")
	workDir, _ := cmd.Flags().GetString("work-dir")
	workflowsDir, _ := cmd.Flags().GetString("workflows-dir")
	doEnrich, _ := cmd.Flags().GetBool("enrich")
	fixSecurity, _ := cmd.Flags().GetBool("fix-security")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	skipLint, _ := cmd.Flags().GetBool("skip-lint")
	skipDocs, _ := cmd.Flags().GetBool("skip-docs")

	sub, _ := cmd.Root().PersistentFlags().GetString("subscription")
	if sub == "" {
		return fmt.Errorf("--subscription is required")
	}

	if fixSecurity {
		doEnrich = true
	}

	if doEnrich && os.Getenv("ANTHROPIC_API_KEY") == "" {
		Log.Info("ANTHROPIC_API_KEY not set — AI descriptions will be skipped")
	}

	log := Log.WithStage("RUN")
	log.Info(fmt.Sprintf("pipeline start: sub=%s rg=%s mode=%s", sub, resourceGroup, mode))

	result, err := run.Run(cmd.Context(), run.Options{
		SubscriptionID:     sub,
		ResourceGroup:      resourceGroup,
		ResourceGroups:     resourceGroups,
		TargetSubscription: targetSub,
		TargetTenant:       targetTenant,
		TenantID:           tenantID,
		RepoName:           repoName,
		RepoOrg:            org,
		MIResourceGroup:    miRG,
		Mode:               mode,
		Environments:       envs,
		Location:           location,
		WorkDir:            workDir,
		WorkflowsDir:       workflowsDir,
		Enrich:             doEnrich,
		FixSecurity:        fixSecurity,
		DryRun:             dryRun,
		SkipLint:           skipLint,
		SkipDocs:           skipDocs,
		Log:                Log.Slog(),
	})
	if err != nil {
		return err
	}

	if dryRun {
		return nil
	}

	if result.ScanResult != nil {
		log.Info(fmt.Sprintf("scan: %d resource group(s)", len(result.ScanResult.ResourceGroups)))
	}
	log.Info(fmt.Sprintf("refine: %d file(s) → %s", len(result.RefineResult.Files), result.RefinedDir))

	if result.BootstrapResult != nil {
		br := result.BootstrapResult
		log.Info(fmt.Sprintf("bootstrap: state storage %s/%s",
			br.StateStorage.ResourceGroupName, br.StateStorage.StorageAccountName))
		if br.IsCrossTenant {
			log.Info("bootstrap: apply bootstrap/ module in target tenant to activate CI/CD")
		}
	}

	log.Info("pipeline complete")
	return nil
}
