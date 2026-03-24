package bootstrap

import (
	"context"
	"fmt"
)

// PlatformConfig holds the parameters needed to provision a Git platform
// (GitHub or Azure DevOps) via the az-bootstrap PowerShell module.
type PlatformConfig struct {
	// Platform is "github" or "ado".
	Platform string
	// Org is the GitHub organisation or ADO organisation name.
	Org string
	// RepoName is the repository to create.
	RepoName string
	// TemplateRepoUrl is the GitHub template to clone (optional).
	// Defaults to kewalaka/terraform-azure-starter-template when empty.
	TemplateRepoUrl string
	// Environments is the list of deployment tiers (prod, staging, dev).
	Environments []string
	// Location is the Azure region for Managed Identity resources.
	Location string
	// StateStorage is the provisioned state backend config (names are passed to az-bootstrap).
	StateStorage StateStorageConfig
}

// ProvisionPlatform delegates to the az-bootstrap PowerShell module to create
// the repository, configure CI/CD environments, and provision Managed Identities
// with OIDC federated credentials.
//
// For GitHub it calls:
//   - Invoke-AzBootstrap for the first environment
//   - Add-AzBootstrapEnvironment for each subsequent environment
func ProvisionPlatform(
	ctx context.Context,
	runner Runner,
	cfg PlatformConfig,
	logLine func(string),
) error {
	switch cfg.Platform {
	case "github":
		return provisionGitHub(ctx, runner, cfg, logLine)
	case "ado":
		return fmt.Errorf("ado platform is not yet supported by the az-bootstrap module; use github")
	default:
		return fmt.Errorf("unsupported platform %q: must be github or ado", cfg.Platform)
	}
}

// provisionGitHub calls Invoke-AzBootstrap for the first environment then
// Add-AzBootstrapEnvironment for each additional environment.
func provisionGitHub(ctx context.Context, runner Runner, cfg PlatformConfig, logLine func(string)) error {
	if len(cfg.Environments) == 0 {
		return fmt.Errorf("at least one environment is required")
	}

	firstEnv := cfg.Environments[0]

	// Build Invoke-AzBootstrap call for the initial environment.
	args := []string{
		"Invoke-AzBootstrap",
		"-TargetRepoName", cfg.RepoName,
		"-GitHubOwner", cfg.Org,
		"-Location", cfg.Location,
		"-InitialEnvironmentName", firstEnv,
		"-ResourceGroupName", cfg.StateStorage.ResourceGroupName,
		"-PlanManagedIdentityName", MIName(cfg.RepoName, firstEnv, "plan"),
		"-ApplyManagedIdentityName", MIName(cfg.RepoName, firstEnv, "apply"),
		"-TerraformStateStorageAccountName", cfg.StateStorage.StorageAccountName,
		"-Confirm:$false", // SupportsShouldProcess — works on all module versions
	}
	if cfg.TemplateRepoUrl != "" {
		args = append(args, "-TemplateRepoUrl", cfg.TemplateRepoUrl)
	}

	if err := runner.Run(ctx, args, logLine); err != nil {
		return fmt.Errorf("Invoke-AzBootstrap: %w", err)
	}

	// Add subsequent environments.
	for _, env := range cfg.Environments[1:] {
		addArgs := []string{
			"Add-AzBootstrapEnvironment",
			"-EnvironmentName", env,
			"-ResourceGroupName", cfg.StateStorage.ResourceGroupName,
			"-Location", cfg.Location,
			"-PlanManagedIdentityName", MIName(cfg.RepoName, env, "plan"),
			"-ApplyManagedIdentityName", MIName(cfg.RepoName, env, "apply"),
			"-GitHubOwner", cfg.Org,
			"-GitHubRepo", cfg.RepoName,
			"-TerraformStateStorageAccountName", cfg.StateStorage.StorageAccountName,
		}
		if err := runner.Run(ctx, addArgs, logLine); err != nil {
			return fmt.Errorf("Add-AzBootstrapEnvironment (%s): %w", env, err)
		}
	}

	return nil
}
