package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"

	"github.com/c4a8-azure/azlift/internal/gitrepo"
	"github.com/c4a8-azure/azlift/internal/workflows"
)

// Options drives the full bootstrap pipeline.
type Options struct {
	// SubscriptionID is the source Azure subscription (scan/export origin).
	SubscriptionID string
	// TargetSubscription is where CI/CD resources are provisioned.
	// Defaults to SubscriptionID (same-tenant mode).
	TargetSubscription string
	// TargetTenant is the target Azure AD tenant.
	// If non-empty and different from TenantID → cross-tenant mode:
	// Azure resources are not provisioned; a bootstrap/ Terraform module is
	// generated instead for the user to apply manually in the target tenant.
	TargetTenant string
	// TenantID is the source Azure AD tenant (empty = resolved from az CLI).
	TenantID string
	// RepoName is the Git repository to create.
	RepoName string
	// RepoOrg is the GitHub organisation.
	RepoOrg string
	// Environments is the list of deployment tiers. Defaults to [prod, staging, dev].
	Environments []string
	// InputDir is the refined Terraform output to commit into the new repo.
	InputDir string
	// TfStateDir is the directory containing terraform.tfstate from aztfexport.
	// Defaults to InputDir when empty. In a full pipeline run this is the raw
	// export directory (e.g. .azlift/raw/<rg>), not the refined directory.
	TfStateDir string
	// Location is the Azure region for state storage. Defaults to westeurope.
	Location string
	// ResourceGroups are the RGs whose resources are being managed.
	// Used as RBAC scope in same-tenant mode.
	ResourceGroups []string
	// MIResourceGroup is the RG for Managed Identities.
	// Defaults to the first entry in ResourceGroups, then falls back to the state RG.
	MIResourceGroup string
	// WorkflowsDir overrides the embedded GitHub Actions workflow templates.
	WorkflowsDir string
	// RepoDir is the local path where the git repo is initialised.
	// Defaults to <os.TempDir()>/azlift-repo/<RepoName>.
	RepoDir string
	// Log is an optional structured logger.
	Log *slog.Logger
}

// Result summarises what the bootstrap pipeline produced.
type Result struct {
	// StateStorage holds the derived state backend config.
	StateStorage StateStorageConfig
	// IsCrossTenant is true when the pipeline ran in cross-tenant mode.
	IsCrossTenant bool
	// RepoDir is the local path of the initialised git repository.
	RepoDir string
	// CommitMessage is the message of the initial commit.
	CommitMessage string
}

// Run executes the full bootstrap pipeline:
//
//  1. Validate inputs and derive names.
//  2. Initialise a local git repo (git init + copy InputDir + workflows + bootstrap module + backend.tf).
//  3. Create the GitHub repo and push.
//  4. Activate (same-tenant only): provision state storage, MIs, OIDC, RBAC, GitHub env vars, upload tfstate.
func Run(ctx context.Context, opts Options) (Result, error) {
	var result Result

	log := opts.Log
	if log == nil {
		log = slog.Default()
	}

	if opts.SubscriptionID == "" {
		return result, fmt.Errorf("SubscriptionID is required")
	}
	if opts.RepoName == "" {
		return result, fmt.Errorf("RepoName is required")
	}
	if opts.RepoOrg == "" {
		return result, fmt.Errorf("RepoOrg is required")
	}

	// Auto-detect tenant if not provided.
	tenantID := opts.TenantID
	if tenantID == "" {
		detected, err := detectTenantID(ctx)
		if err != nil {
			return result, fmt.Errorf("detecting tenant ID: %w", err)
		}
		tenantID = detected
	}
	// Propagate resolved tenant back into opts so downstream helpers see it.
	opts.TenantID = tenantID

	envs := opts.Environments
	if len(envs) == 0 {
		envs = []string{"prod", "dev"}
	}

	location := opts.Location
	if location == "" {
		location = "westeurope"
	}

	targetSub := opts.TargetSubscription
	if targetSub == "" {
		targetSub = opts.SubscriptionID
	}

	isCrossTenant := opts.TargetTenant != "" && opts.TargetTenant != opts.TenantID
	result.IsCrossTenant = isCrossTenant

	stateCfg := DeriveStateConfig(targetSub, opts.RepoName, location)
	if err := ValidateStateConfig(stateCfg); err != nil {
		return result, fmt.Errorf("invalid state config: %w", err)
	}
	result.StateStorage = stateCfg

	// MIs always go into the state RG alongside the storage account.
	// This keeps all azlift CI/CD infrastructure co-located and prevents
	// MIs from appearing in aztfexport output on re-runs.
	// The --mi-resource-group flag overrides for cases where a shared
	// identity RG already exists.
	miRG := opts.MIResourceGroup
	if miRG == "" {
		miRG = stateCfg.ResourceGroupName
	}

	// ── Stage 4: REPO INIT ────────────────────────────────────────────────────

	repoDir := opts.RepoDir
	if repoDir == "" {
		repoDir = filepath.Join(defaultTempDir(), "azlift-repo", opts.RepoName)
	}
	result.RepoDir = repoDir

	log.Info("bootstrap: initialising git repository", "path", repoDir)

	if err := initRepo(ctx, log, opts, repoDir, envs, location, isCrossTenant, stateCfg, targetSub, miRG); err != nil {
		return result, err
	}

	// ── Stage 5: GITHUB REPO ──────────────────────────────────────────────────

	log.Info("bootstrap: creating GitHub repository", "org", opts.RepoOrg, "repo", opts.RepoName)
	if err := CreateAndPushRepo(ctx, RepoConfig{
		Org:     opts.RepoOrg,
		Name:    opts.RepoName,
		RepoDir: repoDir,
		Private: true,
	}); err != nil {
		return result, fmt.Errorf("creating GitHub repo: %w", err)
	}
	log.Info("bootstrap: repository created and pushed")

	// ── Stage 6: ACTIVATE (same-tenant only) ──────────────────────────────────

	if isCrossTenant {
		log.Info("bootstrap: cross-tenant mode — skipping Azure provisioning")
		log.Info("bootstrap: apply the bootstrap/ Terraform module in the target tenant to activate CI/CD")
		return result, nil
	}

	log.Info("bootstrap: activating CI/CD (same-tenant)")

	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return result, fmt.Errorf("obtaining Azure credential: %w", err)
	}

	// 6a. State storage.
	log.Info("bootstrap: provisioning state storage", "rg", stateCfg.ResourceGroupName, "sa", stateCfg.StorageAccountName)
	if err := ProvisionStateStorage(ctx, cred, stateCfg); err != nil {
		return result, fmt.Errorf("provisioning state storage: %w", err)
	}

	// 6b. Managed Identities + OIDC + RBAC.
	rbacScopes := buildRBACScopes(targetSub, opts.ResourceGroups)
	stateStorageScope := fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Storage/storageAccounts/%s",
		targetSub, stateCfg.ResourceGroupName, stateCfg.StorageAccountName,
	)
	log.Info("bootstrap: provisioning Managed Identities", "environments", envs)
	idResult, err := ProvisionIdentities(ctx, cred, IdentityProvisionConfig{
		SubscriptionID:    targetSub,
		ResourceGroup:     miRG,
		Location:          location,
		RepoOrg:           opts.RepoOrg,
		RepoName:          opts.RepoName,
		Environments:      envs,
		RBACScopes:        rbacScopes,
		StateStorageScope: stateStorageScope,
	})
	if err != nil {
		return result, fmt.Errorf("provisioning identities: %w", err)
	}
	log.Info(fmt.Sprintf("bootstrap: %d MI(s) provisioned", len(idResult.Identities)))

	// 6c. Configure GitHub environments with MI client IDs.
	log.Info("bootstrap: configuring GitHub environments")
	ghEnvs := buildGHEnvironments(opts.RepoOrg, opts.RepoName, envs, targetSub,
		resolvedTenant(opts.TenantID, opts.TargetTenant), idResult)
	if err := ConfigureEnvironments(ctx, ghEnvs); err != nil {
		return result, fmt.Errorf("configuring GitHub environments: %w", err)
	}

	// 6d. Write real backend.tf and push.
	log.Info("bootstrap: writing backend.tf with state storage details")
	if err := WriteBackend(BackendConfig{
		ResourceGroupName:  stateCfg.ResourceGroupName,
		StorageAccountName: stateCfg.StorageAccountName,
		ContainerName:      stateCfg.ContainerName,
		Key:                opts.RepoName + ".tfstate",
	}, repoDir); err != nil {
		return result, fmt.Errorf("writing backend.tf: %w", err)
	}
	if err := gitrepo.Add(ctx, repoDir, "backend.tf"); err != nil {
		return result, err
	}
	if err := gitrepo.Commit(ctx, repoDir, "chore: configure azurerm backend"); err != nil {
		return result, err
	}
	if err := gitrepo.Push(ctx, repoDir, "origin", "main"); err != nil {
		return result, fmt.Errorf("pushing backend.tf: %w", err)
	}

	// 6e. Upload tfstate (same-tenant: state from aztfexport is valid).
	tfStateSearchDir := opts.TfStateDir
	if tfStateSearchDir == "" {
		tfStateSearchDir = opts.InputDir
	}
	if tfStateSearchDir == "" {
		return result, fmt.Errorf("TfStateDir and InputDir are both empty — cannot locate terraform.tfstate")
	}
	tfStatePath := tfstatePath(tfStateSearchDir)
	if !fileExists(tfStatePath) {
		return result, fmt.Errorf(
			"terraform.tfstate not found at %s — aztfexport must have failed or the state was not written to the expected location; "+
				"state storage container %s/%s was provisioned but is empty",
			tfStatePath, stateCfg.StorageAccountName, stateCfg.ContainerName,
		)
	}
	log.Info("bootstrap: uploading terraform.tfstate to remote state", "path", tfStatePath)
	if err := UploadTfState(ctx, cred, TfStateUploadConfig{
		SubscriptionID:     targetSub,
		ResourceGroupName:  stateCfg.ResourceGroupName,
		StorageAccountName: stateCfg.StorageAccountName,
		ContainerName:      stateCfg.ContainerName,
		BlobKey:            opts.RepoName + ".tfstate",
		LocalPath:          tfStatePath,
	}); err != nil {
		return result, fmt.Errorf("uploading tfstate: %w", err)
	}
	log.Info("bootstrap: tfstate uploaded", "blob", opts.RepoName+".tfstate")

	log.Info("bootstrap: complete")
	return result, nil
}

// initRepo sets up the local git repository with all generated files.
func initRepo(
	ctx context.Context,
	log *slog.Logger,
	opts Options,
	repoDir string,
	envs []string,
	location string,
	isCrossTenant bool,
	stateCfg StateStorageConfig,
	targetSub, miRG string,
) error {
	if err := prepareDir(repoDir); err != nil {
		return err
	}
	if err := gitrepo.Init(ctx, repoDir); err != nil {
		return fmt.Errorf("git init: %w", err)
	}
	if err := gitrepo.ConfigUser(ctx, repoDir, "azlift", "azlift@noreply"); err != nil {
		return fmt.Errorf("git config user: %w", err)
	}

	// Copy refined Terraform into repo root.
	if opts.InputDir != "" {
		log.Info("bootstrap: copying refined Terraform into repo")
		if err := copyDirContents(opts.InputDir, repoDir); err != nil {
			return fmt.Errorf("copying refined output: %w", err)
		}
	}

	// GitHub Actions workflows.
	log.Info("bootstrap: writing GitHub Actions workflows")
	if err := workflows.Write(repoDir, workflows.Config{
		Environments: envs,
		CustomDir:    opts.WorkflowsDir,
	}); err != nil {
		return fmt.Errorf("writing workflows: %w", err)
	}

	// Bootstrap Terraform module.
	log.Info("bootstrap: generating bootstrap/ Terraform module")
	modCfg := ModuleConfig{
		RepoName:       opts.RepoName,
		RepoOrg:        opts.RepoOrg,
		Environments:   envs,
		SubscriptionID: targetSub,
		TenantID:       resolvedTenant(opts.TenantID, opts.TargetTenant),
		Location:       location,
		ResourceGroups: opts.ResourceGroups,
		IsCrossTenant:  isCrossTenant,
		StateStorage:   stateCfg,
	}
	if err := GenerateBootstrapModule(modCfg, repoDir); err != nil {
		return fmt.Errorf("generating bootstrap module: %w", err)
	}

	// backend.tf: always write placeholders in the initial commit.
	// Same-tenant: overwritten with real values after state storage is provisioned.
	// Cross-tenant: placeholder stays; operator fills in after applying bootstrap/.
	if err := WriteBackend(BackendConfig{
		ContainerName: stateCfg.ContainerName,
		Key:           opts.RepoName + ".tfstate",
		Placeholder:   true,
	}, repoDir); err != nil {
		return fmt.Errorf("writing backend.tf: %w", err)
	}

	// Initial commit.
	if err := gitrepo.Add(ctx, repoDir, "."); err != nil {
		return fmt.Errorf("git add: %w", err)
	}
	msg := fmt.Sprintf("chore: initial Terraform export by azlift\n\nSource subscription: %s", opts.SubscriptionID)
	if err := gitrepo.Commit(ctx, repoDir, msg); err != nil {
		return fmt.Errorf("initial commit: %w", err)
	}
	return nil
}

// buildRBACScopes constructs full Azure scope strings for role assignments.
// Same-tenant: one scope per managed resource group, plus the state storage RG.
// Cross-tenant (or no RGs specified): subscription scope.
// Duplicates are deduplicated.
func buildRBACScopes(subscriptionID string, resourceGroups []string, extraRGs ...string) []string {
	seen := map[string]bool{}
	add := func(rg string) string {
		return fmt.Sprintf("/subscriptions/%s/resourceGroups/%s", subscriptionID, rg)
	}

	if len(resourceGroups) == 0 {
		return []string{fmt.Sprintf("/subscriptions/%s", subscriptionID)}
	}

	var scopes []string
	for _, rg := range append(resourceGroups, extraRGs...) {
		scope := add(rg)
		if !seen[scope] {
			seen[scope] = true
			scopes = append(scopes, scope)
		}
	}
	return scopes
}

// buildGHEnvironments assembles the GHEnvironmentConfig for all plan+apply environments.
func buildGHEnvironments(org, repo string, envs []string, sub, tenant string, ids ProvisionedIdentities) GHEnvironmentConfig {
	ghEnvs := make(map[string][]EnvVarConfig)
	for _, env := range envs {
		for _, role := range []string{"plan", "apply"} {
			ghEnvName := fmt.Sprintf("%s-iac-%s", env, role)
			clientID := ""
			if mi, ok := ids.Identities[fmt.Sprintf("%s/%s", env, role)]; ok {
				clientID = mi.ClientID
			}
			ghEnvs[ghEnvName] = []EnvVarConfig{
				{Name: "AZURE_CLIENT_ID", Value: clientID},
				{Name: "AZURE_TENANT_ID", Value: tenant},
				{Name: "AZURE_SUBSCRIPTION_ID", Value: sub},
			}
		}
	}
	return GHEnvironmentConfig{Org: org, Repo: repo, Environments: ghEnvs}
}

// resolvedTenant returns the effective tenant ID to use in configurations.
func resolvedTenant(sourceTenant, targetTenant string) string {
	if targetTenant != "" {
		return targetTenant
	}
	return sourceTenant
}

// detectTenantID calls `az account show` to discover the active tenant ID.
func detectTenantID(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "az", "account", "show", "--query", "tenantId", "-o", "tsv").Output()
	if err != nil {
		return "", fmt.Errorf("az account show: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// tfstatePath returns the expected path of the aztfexport state file.
func tfstatePath(inputDir string) string {
	return filepath.Join(inputDir, "terraform.tfstate")
}
