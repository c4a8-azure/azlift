package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
)

// Options drives the full bootstrap pipeline.
type Options struct {
	// SubscriptionID is the source Azure subscription (scan/export origin).
	SubscriptionID string
	// TargetSubscription is where CI/CD resources are provisioned.
	// Defaults to SubscriptionID (same-tenant mode).
	TargetSubscription string
	// TargetTenant is the target Azure AD tenant.
	// If non-empty and different from the source tenant → cross-tenant mode:
	// Azure resources are not provisioned; a bootstrap/ Terraform module is
	// generated instead for the user to apply manually.
	TargetTenant string
	// TenantID is the source Azure AD tenant.
	TenantID string
	// RepoName is the Git repository to create.
	RepoName string
	// RepoOrg is the GitHub organisation.
	RepoOrg string
	// Environments is the list of deployment tiers. Defaults to [prod, staging, dev].
	Environments []string
	// InputDir is the refined Terraform output to commit into the new repo.
	InputDir string
	// Location is the Azure region for state storage. Defaults to westeurope.
	Location string
	// ResourceGroups are the RGs whose resources are being managed.
	// Used for RBAC scope in same-tenant mode.
	ResourceGroups []string
	// MIResourceGroup is the RG for Managed Identities.
	// Defaults to the first entry in ResourceGroups.
	MIResourceGroup string
	// WorkflowsDir overrides the embedded GitHub Actions workflow templates.
	WorkflowsDir string
	// Log is an optional structured logger.
	Log *slog.Logger
}

// Result summarises what the bootstrap pipeline produced.
type Result struct {
	// StateStorage holds the derived state backend config.
	StateStorage StateStorageConfig
	// IsCrossTenant is true when the pipeline ran in cross-tenant mode.
	IsCrossTenant bool
	// CommitMessage is the message of the initial commit (empty if InputDir was empty).
	CommitMessage string
}

// Run executes the full bootstrap pipeline. Implementation is built up across
// issues #75–#83; this stub validates inputs and returns an informative error
// until the native provisioning is complete.
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

	envs := opts.Environments
	if len(envs) == 0 {
		envs = []string{"prod", "staging", "dev"}
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

	if isCrossTenant {
		log.Info("bootstrap: cross-tenant mode — bootstrap/ Terraform module will be generated")
	} else {
		log.Info("bootstrap: same-tenant mode — Azure resources will be provisioned natively")
	}

	return result, fmt.Errorf("bootstrap: native provisioning not yet implemented; tracking in #79–#83")
}
