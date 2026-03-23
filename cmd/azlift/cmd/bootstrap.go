package cmd

import "github.com/spf13/cobra"

func newBootstrapCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bootstrap",
		Short: "Provision state storage, Managed Identities, and Git CI/CD pipeline",
		Long: `Wrap az-bootstrap to initialise the Git repository and CI/CD plumbing around
the generated Terraform. Provisions state storage, Managed Identities with OIDC
federated credentials (no stored secrets), and configures the GitHub or Azure
DevOps repository with environment-gated pipelines.`,
		RunE: notImplemented,
	}

	cmd.Flags().String("input-dir", "./refined", "Directory containing refined Terraform output")
	cmd.Flags().String("repo-name", "", "Name of the Git repository to create (required)")
	cmd.Flags().StringSlice("environments", []string{"dev", "staging", "prod"}, "Environments to create (comma-separated)")
	cmd.Flags().String("platform", "github", "CI/CD platform: github or ado")
	_ = cmd.MarkFlagRequired("repo-name")

	return cmd
}
