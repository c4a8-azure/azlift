package bootstrap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// defaultTemplateRepoUrl is the az-bootstrap template used when none is specified.
const defaultTemplateRepoUrl = "kewalaka/terraform-azure-starter-template"

// ProvisionPlatform delegates to the az-bootstrap PowerShell module to create
// the repository, configure CI/CD environments, and provision Managed Identities
// with OIDC federated credentials.
//
// Returns the path of the cloned repository so callers can commit files into it.
func ProvisionPlatform(
	ctx context.Context,
	runner Runner,
	cfg PlatformConfig,
	logLine func(string),
) (string, error) {
	switch cfg.Platform {
	case "github":
		return provisionGitHub(ctx, runner, cfg, logLine)
	case "ado":
		return "", fmt.Errorf("ado platform is not yet supported by the az-bootstrap module; use github")
	default:
		return "", fmt.Errorf("unsupported platform %q: must be github or ado", cfg.Platform)
	}
}

// provisionGitHub calls Invoke-AzBootstrap for the first environment then
// Add-AzBootstrapEnvironment for each additional environment.
//
// The repo is cloned to /tmp/azlift-bootstrap/<repoName>. That path is returned
// so the caller can commit the generated Terraform into the correct location.
// Each Add-AzBootstrapEnvironment call runs with Set-Location pointing at the
// clone so that gh CLI picks up the correct git remote.
func provisionGitHub(ctx context.Context, runner Runner, cfg PlatformConfig, logLine func(string)) (string, error) {
	if len(cfg.Environments) == 0 {
		return "", fmt.Errorf("at least one environment is required")
	}

	firstEnv := cfg.Environments[0]

	templateURL := cfg.TemplateRepoUrl
	if templateURL == "" {
		templateURL = defaultTemplateRepoUrl
	}

	// Clone to a deterministic temp path so we know where to cd for Add calls.
	targetDir := filepath.Join(os.TempDir(), "azlift-bootstrap", cfg.RepoName)

	// Note: -GitHubOwner is intentionally omitted — the installed az-bootstrap
	// version translates it to `gh repo create --owner <org>` which is not a
	// valid gh CLI flag. The module falls back to the authenticated gh user
	// (from `gh auth status`) for owner resolution.
	args := []string{
		"Invoke-AzBootstrap",
		"-TemplateRepoUrl", templateURL,
		"-TargetRepoName", cfg.RepoName,
		"-TargetDirectory", targetDir,
		"-Location", cfg.Location,
		"-InitialEnvironmentName", firstEnv,
		"-ResourceGroupName", cfg.StateStorage.ResourceGroupName,
		"-PlanManagedIdentityName", MIName(cfg.RepoName, firstEnv, "plan"),
		"-ApplyManagedIdentityName", MIName(cfg.RepoName, firstEnv, "apply"),
		"-TerraformStateStorageAccountName", cfg.StateStorage.StorageAccountName,
		"-Confirm:$false", // SupportsShouldProcess
	}

	if err := runner.Run(ctx, args, logLine); err != nil {
		return "", fmt.Errorf("Invoke-AzBootstrap: %w", err)
	}

	// Add subsequent environments, each running from inside the cloned repo so
	// gh picks up the correct remote (not the caller's working directory).
	safeDir := strings.ReplaceAll(targetDir, "'", "''")
	for _, env := range cfg.Environments[1:] {
		addArgs := []string{
			"Set-Location '" + safeDir + "'; Add-AzBootstrapEnvironment",
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
			return "", fmt.Errorf("Add-AzBootstrapEnvironment (%s): %w", env, err)
		}
	}

	return targetDir, nil
}
