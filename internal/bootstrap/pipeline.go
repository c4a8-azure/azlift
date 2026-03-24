package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
)

// Options drives the full bootstrap pipeline.
type Options struct {
	// SubscriptionID is the target Azure subscription (used for state config naming).
	SubscriptionID string
	// TenantID is the Azure AD tenant (written to .azbootstrap.jsonc).
	TenantID string
	// RepoName is the Git repository to create.
	RepoName string
	// RepoOrg is the GitHub org or ADO organisation.
	RepoOrg string
	// Platform is "github" or "ado".
	Platform string
	// Environments is the list of deployment tiers.
	// Defaults to ["prod", "staging", "dev"].
	Environments []string
	// InputDir is the refined Terraform output to commit.
	InputDir string
	// Location is the Azure region for state storage. Defaults to westeurope.
	Location string
	// TemplateRepoUrl is passed to Invoke-AzBootstrap (optional).
	TemplateRepoUrl string
	// Runner is the az-bootstrap subprocess runner.
	// Defaults to AzBootstrapRunner if nil.
	Runner Runner
	// Log is an optional structured logger for progress output.
	Log *slog.Logger
}

// Result summarises what the bootstrap pipeline produced.
type Result struct {
	// StateStorage holds the derived state backend config (names used for az-bootstrap).
	StateStorage StateStorageConfig
	// CommitMessage is the message of the initial commit (empty if InputDir was empty).
	CommitMessage string
}

// Run executes the full bootstrap pipeline:
//
//  1. Derive state storage names (DeriveStateConfig).
//  2. Call az-bootstrap (Invoke-AzBootstrap + Add-AzBootstrapEnvironment) which
//     creates the state storage, Managed Identities, OIDC federation, and Git repo.
//  3. Commit refined Terraform into the repo (if InputDir is set).
func Run(ctx context.Context, opts Options) (Result, error) {
	var result Result
	log := opts.Log
	if log == nil {
		log = slog.Default()
	}

	runner := opts.Runner
	if runner == nil {
		runner = &AzBootstrapRunner{}
	}

	envs := opts.Environments
	if len(envs) == 0 {
		envs = []string{"prod", "staging", "dev"}
	}

	location := opts.Location
	if location == "" {
		location = "westeurope"
	}

	logLine := func(line string) { log.Info("[az-bootstrap] " + line) }

	// Derive deterministic state storage names.
	stateCfg := DeriveStateConfig(opts.SubscriptionID, opts.RepoName, location)
	if err := ValidateStateConfig(stateCfg); err != nil {
		return result, fmt.Errorf("invalid state config: %w", err)
	}
	result.StateStorage = stateCfg

	log.Info(fmt.Sprintf("bootstrap: provisioning via az-bootstrap (platform: %s, repo: %s/%s)",
		opts.Platform, opts.RepoOrg, opts.RepoName))
	log.Info(fmt.Sprintf("bootstrap: state storage will be %s/%s",
		stateCfg.ResourceGroupName, stateCfg.StorageAccountName))

	platCfg := PlatformConfig{
		Platform:        opts.Platform,
		Org:             opts.RepoOrg,
		RepoName:        opts.RepoName,
		TemplateRepoUrl: opts.TemplateRepoUrl,
		Environments:    envs,
		Location:        location,
		StateStorage:    stateCfg,
	}
	if err := ProvisionPlatform(ctx, runner, platCfg, logLine); err != nil {
		return result, err
	}
	log.Info("bootstrap: platform provisioned")

	// Commit Terraform into repo.
	if opts.InputDir != "" {
		log.Info("bootstrap: committing Terraform output to repository")
		absCfg := &AzBootstrapConfig{
			SchemaVersion:  "1",
			SubscriptionID: opts.SubscriptionID,
			TenantID:       opts.TenantID,
			StateStorage:   stateCfg,
		}
		commitResult, err := CommitToRepo(ctx, CommitConfig{
			RepoDir:           opts.InputDir,
			SubscriptionID:    opts.SubscriptionID,
			AzBootstrapConfig: absCfg,
		})
		if err != nil {
			return result, err
		}
		result.CommitMessage = commitResult.Message
		log.Info("bootstrap: initial commit created")
	}

	return result, nil
}
