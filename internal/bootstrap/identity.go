package bootstrap

import (
	"context"
	"fmt"
	"strings"
)

// ManagedIdentity describes one provisioned Managed Identity.
type ManagedIdentity struct {
	// Name is the MI resource name.
	Name string
	// Environment is the deployment tier this MI belongs to (prod, staging, dev).
	Environment string
	// Role is either "plan" (Reader) or "apply" (Contributor).
	Role string
	// ClientID is populated after provisioning.
	ClientID string
	// ResourceID is the full Azure resource ID.
	ResourceID string
}

// OIDCConfig holds the federated credential details for one MI.
type OIDCConfig struct {
	// Platform is "github" or "ado".
	Platform string
	// RepoOrg is the GitHub org or ADO organisation.
	RepoOrg string
	// RepoName is the repository name.
	RepoName string
	// Environment is the GitHub/ADO environment name (e.g. "prod-iac-apply").
	Environment string
}

// IdentityProvisionResult holds all MIs created for a set of environments.
type IdentityProvisionResult struct {
	// Identities maps "<env>/<role>" → ManagedIdentity.
	Identities map[string]ManagedIdentity
}

// MIName derives the MI resource name for an environment + role combination.
// Pattern: mi-<repo-slug>-<env>-<role>
func MIName(repoName, environment, role string) string {
	slug := sanitiseRepoName(repoName)
	return fmt.Sprintf("mi-%s-%s-%s", slug, environment, role)
}

// ProvisionIdentities creates plan+apply MIs for each environment via
// az-bootstrap, configures OIDC federated credentials, and returns the
// provisioned identities. All operations are idempotent.
func ProvisionIdentities(
	ctx context.Context,
	runner Runner,
	subscriptionID string,
	resourceGroup string,
	repoName string,
	environments []string,
	oidc OIDCConfig,
	logLine func(string),
) (IdentityProvisionResult, error) {
	result := IdentityProvisionResult{
		Identities: make(map[string]ManagedIdentity),
	}

	for _, env := range environments {
		for _, role := range []string{"plan", "apply"} {
			miName := MIName(repoName, env, role)
			rbacRole := rbacRoleForRole(role)
			oidcEnv := fmt.Sprintf("%s-iac-%s", env, role)

			args := []string{
				"managed-identity",
				"--subscription-id", subscriptionID,
				"--resource-group", resourceGroup,
				"--name", miName,
				"--role", rbacRole,
				"--oidc-platform", oidc.Platform,
				"--oidc-org", oidc.RepoOrg,
				"--oidc-repo", oidc.RepoName,
				"--oidc-environment", oidcEnv,
			}

			if err := runner.Run(ctx, args, logLine); err != nil {
				return result, fmt.Errorf("provisioning MI %s: %w", miName, err)
			}

			key := fmt.Sprintf("%s/%s", env, role)
			result.Identities[key] = ManagedIdentity{
				Name:        miName,
				Environment: env,
				Role:        role,
				// ClientID is filled in by ReadIdentityOutputs after provisioning.
			}
		}
	}

	return result, nil
}

// rbacRoleForRole maps the azlift role name to the Azure RBAC role name.
func rbacRoleForRole(role string) string {
	switch strings.ToLower(role) {
	case "apply":
		return "Contributor"
	default:
		return "Reader"
	}
}

// AzBootstrapConfig is the `.azbootstrap.jsonc` contract — the output written
// by az-bootstrap that azlift reads to configure CI/CD variables.
type AzBootstrapConfig struct {
	// SchemaVersion identifies the file format.
	SchemaVersion string `json:"schemaVersion"`
	// SubscriptionID is the target Azure subscription.
	SubscriptionID string `json:"subscriptionId"`
	// TenantID is the Azure AD tenant.
	TenantID string `json:"tenantId"`
	// StateStorage holds the provisioned state backend details.
	StateStorage StateStorageConfig `json:"stateStorage"`
	// Identities maps "<env>/<role>" → client ID strings.
	Identities map[string]string `json:"identities"`
}
