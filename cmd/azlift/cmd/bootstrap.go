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
		Long: `Wrap az-bootstrap to initialise the Git repository and CI/CD plumbing around
the generated Terraform. Provisions state storage, Managed Identities with OIDC
federated credentials (no stored secrets), and configures the GitHub or Azure
DevOps repository with environment-gated pipelines.

Example:
  azlift bootstrap \
    --subscription <id> \
    --repo-name infra-prod \
    --org my-org \
    --platform github \
    --input-dir ./refined`,
		RunE: runBootstrap,
	}

	cmd.Flags().String("input-dir", "./refined", "Directory containing refined Terraform output to commit")
	cmd.Flags().String("repo-name", "", "Name of the Git repository to create (required)")
	cmd.Flags().String("org", "", "GitHub organisation or ADO organisation (required)")
	cmd.Flags().StringSlice("environments", []string{"prod", "staging", "dev"}, "Deployment environments (comma-separated)")
	cmd.Flags().String("platform", "github", "CI/CD platform: github or ado")
	cmd.Flags().String("location", "westeurope", "Azure region for state storage")
	cmd.Flags().String("tenant-id", "", "Azure AD tenant ID (auto-detected if empty)")

	_ = cmd.MarkFlagRequired("repo-name")
	_ = cmd.MarkFlagRequired("org")

	return cmd
}

func runBootstrap(cmd *cobra.Command, _ []string) error {
	inputDir, _ := cmd.Flags().GetString("input-dir")
	repoName, _ := cmd.Flags().GetString("repo-name")
	org, _ := cmd.Flags().GetString("org")
	envs, _ := cmd.Flags().GetStringSlice("environments")
	platform, _ := cmd.Flags().GetString("platform")
	location, _ := cmd.Flags().GetString("location")
	tenantID, _ := cmd.Flags().GetString("tenant-id")

	sub, _ := cmd.Root().PersistentFlags().GetString("subscription")
	if sub == "" {
		return fmt.Errorf("--subscription is required for bootstrap")
	}

	log := Log.WithStage("BOOTSTRAP")
	log.Info(fmt.Sprintf("bootstrapping repo %s/%s on %s", org, repoName, platform))

	result, err := bootstrap.Run(cmd.Context(), bootstrap.Options{
		SubscriptionID: sub,
		TenantID:       tenantID,
		RepoName:       repoName,
		RepoOrg:        org,
		Platform:       platform,
		Environments:   envs,
		InputDir:       inputDir,
		Location:       location,
		Log:            Log.Slog(),
	})
	if err != nil {
		return err
	}

	log.Info(fmt.Sprintf("state storage: %s/%s",
		result.StateStorage.ResourceGroupName,
		result.StateStorage.StorageAccountName))
	log.Info(fmt.Sprintf("identities: %d MI(s) provisioned", len(result.Identities)))
	if result.CommitMessage != "" {
		log.Info("initial commit created")
	}
	log.Info("bootstrap complete")
	return nil
}
