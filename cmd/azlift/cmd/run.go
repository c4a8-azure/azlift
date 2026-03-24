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
		Long: `Orchestrate all four pipeline stages in sequence:
  1. scan       — build resource inventory and dependency graph
  2. export     — export via aztfexport
  3. refine     — transform HCL into structured Terraform or Terragrunt
  4. bootstrap  — provision CI/CD plumbing (skippable with --no-bootstrap)

Use --no-bootstrap to generate code and run terraform plan without provisioning
any Azure resources or Git repositories.

Use --dry-run to print the planned actions without executing any external tools.

Example:
  azlift run \
    --subscription <id> \
    --resource-group rg-myapp-prod \
    --repo-name infra-prod \
    --org my-org \
    --platform github`,
		RunE: runPipeline,
	}

	cmd.Flags().String("resource-group", "", "Resource group to process (required)")
	cmd.Flags().String("repo-name", "", "Name of the Git repository to create")
	cmd.Flags().String("org", "", "GitHub organisation or ADO organisation")
	cmd.Flags().String("mode", "modules", "Output mode: modules or terragrunt")
	cmd.Flags().String("platform", "github", "CI/CD platform: github or ado")
	cmd.Flags().StringSlice("environments", []string{"prod", "staging", "dev"}, "Deployment environments (comma-separated)")
	cmd.Flags().String("location", "westeurope", "Azure region for state storage")
	cmd.Flags().String("tenant-id", "", "Azure AD tenant ID (auto-detected if empty)")
	cmd.Flags().String("work-dir", ".azlift", "Base directory for all pipeline outputs")
	cmd.Flags().Bool("enrich", false, "Run AI enrichment pass (lifecycle, security, tags, descriptions)")
	cmd.Flags().Bool("fix-security", false, "Auto-remediate safe security anti-patterns")
	cmd.Flags().Bool("no-bootstrap", false, "Skip bootstrap; run terraform plan against refined output instead")
	cmd.Flags().Bool("dry-run", false, "Print planned actions without executing any external tools")
	cmd.Flags().Bool("skip-lint", false, "Skip the tflint pass")
	cmd.Flags().Bool("skip-docs", false, "Skip terraform-docs generation")

	_ = cmd.MarkFlagRequired("resource-group")

	return cmd
}

func runPipeline(cmd *cobra.Command, _ []string) error {
	resourceGroup, _ := cmd.Flags().GetString("resource-group")
	repoName, _ := cmd.Flags().GetString("repo-name")
	org, _ := cmd.Flags().GetString("org")
	mode, _ := cmd.Flags().GetString("mode")
	platform, _ := cmd.Flags().GetString("platform")
	envs, _ := cmd.Flags().GetStringSlice("environments")
	location, _ := cmd.Flags().GetString("location")
	tenantID, _ := cmd.Flags().GetString("tenant-id")
	workDir, _ := cmd.Flags().GetString("work-dir")
	doEnrich, _ := cmd.Flags().GetBool("enrich")
	fixSecurity, _ := cmd.Flags().GetBool("fix-security")
	noBootstrap, _ := cmd.Flags().GetBool("no-bootstrap")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	skipLint, _ := cmd.Flags().GetBool("skip-lint")
	skipDocs, _ := cmd.Flags().GetBool("skip-docs")

	sub, _ := cmd.Root().PersistentFlags().GetString("subscription")
	if sub == "" {
		return fmt.Errorf("--subscription is required")
	}

	if !noBootstrap && repoName == "" {
		return fmt.Errorf("--repo-name is required for bootstrap; use --no-bootstrap to skip")
	}
	if !noBootstrap && org == "" {
		return fmt.Errorf("--org is required for bootstrap; use --no-bootstrap to skip")
	}

	if fixSecurity {
		doEnrich = true
	}

	log := Log.WithStage("RUN")
	log.Info(fmt.Sprintf("pipeline: sub=%s rg=%s mode=%s", sub, resourceGroup, mode))

	if dryRun {
		log.Info("dry-run mode — no external tools will be called")
	}

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if doEnrich {
		if apiKey == "" {
			log.Info("ANTHROPIC_API_KEY not set — AI descriptions will be skipped")
		} else {
			log.Info("ANTHROPIC_API_KEY detected — AI enrichment enabled")
		}
	}

	result, err := run.Run(cmd.Context(), run.Options{
		SubscriptionID: sub,
		ResourceGroup:  resourceGroup,
		RepoName:       repoName,
		RepoOrg:        org,
		Platform:       platform,
		Environments:   envs,
		Location:       location,
		TenantID:       tenantID,
		WorkDir:        workDir,
		Mode:           mode,
		Enrich:         doEnrich,
		FixSecurity:    fixSecurity,
		NoBootstrap:    noBootstrap,
		DryRun:         dryRun,
		SkipLint:       skipLint,
		SkipDocs:       skipDocs,
		Log:            Log.Slog(),
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
	log.Info(fmt.Sprintf("refine: %d file(s) written to %s", len(result.RefineResult.Files), result.RefinedDir))

	if result.BootstrapResult != nil {
		br := result.BootstrapResult
		log.Info(fmt.Sprintf("bootstrap: state storage %s/%s",
			br.StateStorage.ResourceGroupName,
			br.StateStorage.StorageAccountName))
		log.Info(fmt.Sprintf("bootstrap: %d Managed Identity(ies) provisioned", len(br.Identities)))
		if br.CommitMessage != "" {
			log.Info("bootstrap: initial commit created")
		}
	}

	if result.TerraformPlanOutput != "" {
		log.Info("terraform plan output:")
		log.Info(result.TerraformPlanOutput)
	}

	log.Info("pipeline complete")
	return nil
}
