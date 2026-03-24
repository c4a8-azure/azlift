package bootstrap

import (
	"context"
	"fmt"
)

// PlatformConfig holds the parameters needed to provision a Git platform
// (GitHub or Azure DevOps).
type PlatformConfig struct {
	// Platform is "github" or "ado".
	Platform string
	// Org is the GitHub organisation or ADO organisation name.
	Org string
	// RepoName is the repository to create or use.
	RepoName string
	// Environments is the list of deployment tiers (prod, staging, dev).
	Environments []string
	// Identities maps "<env>/<role>" → MI client ID, set by ProvisionIdentities.
	Identities map[string]string
	// StateStorage is the provisioned state backend config.
	StateStorage StateStorageConfig
	// SubscriptionID is the target Azure subscription.
	SubscriptionID string
	// TenantID is the Azure AD tenant.
	TenantID string
}

// ProvisionPlatform delegates to az-bootstrap to create the repository,
// configure CI/CD environments, and set pipeline variables.
// It dispatches to the correct az-bootstrap subcommand based on Platform.
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
		return provisionADO(ctx, runner, cfg, logLine)
	default:
		return fmt.Errorf("unsupported platform %q: must be github or ado", cfg.Platform)
	}
}

// provisionGitHub calls az-bootstrap github with the full set of flags:
//   - repo creation
//   - environment protection rules (required reviewers on apply environments)
//   - repository variables: ARM_CLIENT_ID, AZURE_SUBSCRIPTION_ID, TF_STATE_*
func provisionGitHub(ctx context.Context, runner Runner, cfg PlatformConfig, logLine func(string)) error {
	args := []string{
		"github",
		"--org", cfg.Org,
		"--repo", cfg.RepoName,
		"--subscription-id", cfg.SubscriptionID,
		"--tenant-id", cfg.TenantID,
		"--state-resource-group", cfg.StateStorage.ResourceGroupName,
		"--state-storage-account", cfg.StateStorage.StorageAccountName,
		"--state-container", cfg.StateStorage.ContainerName,
	}

	for _, env := range cfg.Environments {
		args = append(args, "--environment", env)
		if id, ok := cfg.Identities[env+"/plan"]; ok {
			args = append(args, fmt.Sprintf("--plan-client-id-%s=%s", env, id))
		}
		if id, ok := cfg.Identities[env+"/apply"]; ok {
			args = append(args, fmt.Sprintf("--apply-client-id-%s=%s", env, id))
		}
	}

	if err := runner.Run(ctx, args, logLine); err != nil {
		return fmt.Errorf("provisioning GitHub platform: %w", err)
	}
	return nil
}

// provisionADO calls az-bootstrap ado with the full set of flags:
//   - project + repo creation
//   - pipeline import from YAML templates
//   - variable groups with MI client IDs and state backend
//   - environment approval gates on apply environments
func provisionADO(ctx context.Context, runner Runner, cfg PlatformConfig, logLine func(string)) error {
	args := []string{
		"ado",
		"--org", cfg.Org,
		"--repo", cfg.RepoName,
		"--subscription-id", cfg.SubscriptionID,
		"--tenant-id", cfg.TenantID,
		"--state-resource-group", cfg.StateStorage.ResourceGroupName,
		"--state-storage-account", cfg.StateStorage.StorageAccountName,
		"--state-container", cfg.StateStorage.ContainerName,
	}

	for _, env := range cfg.Environments {
		args = append(args, "--environment", env)
		if id, ok := cfg.Identities[env+"/plan"]; ok {
			args = append(args, fmt.Sprintf("--plan-client-id-%s=%s", env, id))
		}
		if id, ok := cfg.Identities[env+"/apply"]; ok {
			args = append(args, fmt.Sprintf("--apply-client-id-%s=%s", env, id))
		}
	}

	if err := runner.Run(ctx, args, logLine); err != nil {
		return fmt.Errorf("provisioning ADO platform: %w", err)
	}
	return nil
}
