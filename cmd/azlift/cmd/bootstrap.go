package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/c4a8-azure/azlift/internal/bootstrap"
)

func newBootstrapCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bootstrap",
		Short: "Provision state storage, Managed Identities, and Git CI/CD pipeline",
		Long: `Initialise the Git repository and CI/CD plumbing around the generated Terraform.

Provisions state storage, Managed Identities with OIDC federated credentials
(no stored secrets), and configures the GitHub repository with environment-gated
pipelines. A bootstrap/ Terraform module is always generated in the output repo.

For cross-tenant deployments (--target-tenant differs from source), Azure
resources are not provisioned automatically — apply the bootstrap/ module manually
in the target tenant instead.

Example (same-tenant):
  azlift bootstrap \
    --subscription <id> \
    --repo-name infra-prod \
    --org my-org \
    --input-dir ./refined

Example (cross-tenant):
  azlift bootstrap \
    --subscription <source-id> \
    --target-subscription <target-id> \
    --target-tenant <target-tenant-id> \
    --repo-name infra-prod \
    --org my-org \
    --input-dir ./refined`,
		RunE: runBootstrap,
	}

	cmd.Flags().String("input-dir", "./refined", "Directory containing refined Terraform output to commit")
	cmd.Flags().String("state-dir", "", "Directory containing terraform.tfstate from aztfexport (defaults to --input-dir)")
	cmd.Flags().String("repo-name", "", "Name of the Git repository to create (required)")
	cmd.Flags().String("org", "", "GitHub organisation (required)")
	cmd.Flags().StringSlice("environments", []string{"prod", "dev"}, "Deployment environments (comma-separated)")
	cmd.Flags().String("location", "westeurope", "Azure region for state storage")
	cmd.Flags().String("tenant-id", "", "Source Azure AD tenant ID (auto-detected if empty)")
	cmd.Flags().String("target-subscription", "", "Target subscription for CI/CD resources (defaults to --subscription)")
	cmd.Flags().String("target-tenant", "", "Target Azure AD tenant (if different → cross-tenant mode)")
	cmd.Flags().StringSlice("resource-groups", nil, "Resource groups being managed (for RBAC scope)")
	cmd.Flags().String("mi-resource-group", "", "RG for Managed Identities (defaults to first --resource-groups entry)")
	cmd.Flags().String("workflows-dir", "", "Custom GitHub Actions workflows directory (default: embedded)")

	_ = cmd.MarkFlagRequired("repo-name")
	_ = cmd.MarkFlagRequired("org")

	return cmd
}

func runBootstrap(cmd *cobra.Command, _ []string) error {
	inputDir, _ := cmd.Flags().GetString("input-dir")
	stateDir, _ := cmd.Flags().GetString("state-dir")
	repoName, _ := cmd.Flags().GetString("repo-name")
	org, _ := cmd.Flags().GetString("org")
	envs, _ := cmd.Flags().GetStringSlice("environments")
	location, _ := cmd.Flags().GetString("location")
	tenantID, _ := cmd.Flags().GetString("tenant-id")
	targetSub, _ := cmd.Flags().GetString("target-subscription")
	targetTenant, _ := cmd.Flags().GetString("target-tenant")
	resourceGroups, _ := cmd.Flags().GetStringSlice("resource-groups")
	miRG, _ := cmd.Flags().GetString("mi-resource-group")
	workflowsDir, _ := cmd.Flags().GetString("workflows-dir")

	sub, _ := cmd.Root().PersistentFlags().GetString("subscription")
	if sub == "" {
		return fmt.Errorf("--subscription is required for bootstrap")
	}

	log := Log.WithStage("BOOTSTRAP")
	log.Info(fmt.Sprintf("bootstrapping repo %s/%s", org, repoName))

	result, err := bootstrap.Run(cmd.Context(), bootstrap.Options{
		SubscriptionID:     sub,
		TargetSubscription: targetSub,
		TargetTenant:       targetTenant,
		TenantID:           tenantID,
		RepoName:           repoName,
		RepoOrg:            org,
		Environments:       envs,
		InputDir:           inputDir,
		TfStateDir:         stateDir,
		Location:           location,
		ResourceGroups:     resourceGroups,
		MIResourceGroup:    miRG,
		WorkflowsDir:       workflowsDir,
		Log:                Log.Slog(),
	})
	if err != nil {
		return err
	}

	log.Info(fmt.Sprintf("state storage: %s/%s",
		result.StateStorage.ResourceGroupName,
		result.StateStorage.StorageAccountName))
	if result.IsCrossTenant {
		log.Info("cross-tenant mode: apply the bootstrap/ Terraform module in the target tenant to activate CI/CD")
	}
	if result.CommitMessage != "" {
		log.Info("initial commit created")
	}
	if result.BackendPRURL != "" {
		log.Info(fmt.Sprintf("review and merge the PR to activate CI/CD: %s", result.BackendPRURL))
	}
	log.Info("bootstrap complete")
	return nil
}
