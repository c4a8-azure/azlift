package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
)

// Options drives the full bootstrap pipeline.
type Options struct {
	// SubscriptionID is the target Azure subscription.
	SubscriptionID string
	// TenantID is the Azure AD tenant (empty = resolved by az-bootstrap).
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
	// Runner is the az-bootstrap subprocess runner.
	// Defaults to AzBootstrapRunner if nil.
	Runner Runner
	// Log is an optional structured logger for progress output.
	Log *slog.Logger
}

// Result summarises what the bootstrap pipeline produced.
type Result struct {
	// StateStorage is the provisioned state backend config.
	StateStorage StateStorageConfig
	// Identities maps "<env>/<role>" → ManagedIdentity.
	Identities map[string]ManagedIdentity
	// CommitMessage is the message of the initial commit.
	CommitMessage string
}

// Run executes the full bootstrap pipeline:
//
//  1. Provision state storage (RG, Storage Account, container).
//  2. Provision Managed Identities with OIDC federated credentials.
//  3. Provision Git platform (repo, environments, CI/CD variables).
//  4. Commit refined Terraform into the new repo.
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

	logLine := func(line string) { log.Info("[az-bootstrap] " + line) }

	// 1. State storage.
	log.Info("bootstrap: provisioning state storage")
	stateCfg := DeriveStateConfig(opts.SubscriptionID, opts.RepoName, opts.Location)
	if err := ValidateStateConfig(stateCfg); err != nil {
		return result, fmt.Errorf("invalid state config: %w", err)
	}
	if err := ProvisionStateStorage(ctx, runner, stateCfg, logLine); err != nil {
		return result, err
	}
	result.StateStorage = stateCfg
	log.Info(fmt.Sprintf("bootstrap: state storage ready — %s/%s", stateCfg.ResourceGroupName, stateCfg.StorageAccountName))

	// 2. Managed Identities.
	log.Info("bootstrap: provisioning Managed Identities")
	oidc := OIDCConfig{
		Platform: opts.Platform,
		RepoOrg:  opts.RepoOrg,
		RepoName: opts.RepoName,
	}
	idResult, err := ProvisionIdentities(
		ctx, runner,
		opts.SubscriptionID,
		stateCfg.ResourceGroupName, // co-locate MIs with state storage RG
		opts.RepoName,
		envs,
		oidc,
		logLine,
	)
	if err != nil {
		return result, err
	}
	result.Identities = idResult.Identities
	log.Info(fmt.Sprintf("bootstrap: %d Managed Identities provisioned", len(idResult.Identities)))

	// Build the client ID map for CI/CD variable injection.
	clientIDs := make(map[string]string, len(idResult.Identities))
	for key, mi := range idResult.Identities {
		clientIDs[key] = mi.ClientID
	}

	// 3. Git platform.
	log.Info(fmt.Sprintf("bootstrap: provisioning %s platform", opts.Platform))
	platCfg := PlatformConfig{
		Platform:       opts.Platform,
		Org:            opts.RepoOrg,
		RepoName:       opts.RepoName,
		Environments:   envs,
		Identities:     clientIDs,
		StateStorage:   stateCfg,
		SubscriptionID: opts.SubscriptionID,
		TenantID:       opts.TenantID,
	}
	if err := ProvisionPlatform(ctx, runner, platCfg, logLine); err != nil {
		return result, err
	}
	log.Info("bootstrap: platform provisioned")

	// 4. Commit Terraform into repo.
	if opts.InputDir != "" {
		log.Info("bootstrap: committing Terraform output to repository")
		absCfg := &AzBootstrapConfig{
			SchemaVersion:  "1",
			SubscriptionID: opts.SubscriptionID,
			TenantID:       opts.TenantID,
			StateStorage:   stateCfg,
			Identities:     clientIDs,
		}
		commitResult, err := CommitToRepo(ctx, CommitConfig{
			RepoDir:           opts.InputDir, // treat InputDir as the cloned repo
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
