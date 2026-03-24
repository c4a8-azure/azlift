package bootstrap

import (
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
	// ClientID is the Azure AD application (client) ID.
	ClientID string
	// ResourceID is the full Azure resource ID.
	ResourceID string
}

// MIName derives the MI resource name for an environment + role combination.
// Pattern: mi-<repo-slug>-<env>-<role>
func MIName(repoName, environment, role string) string {
	slug := sanitiseRepoName(repoName)
	return fmt.Sprintf("mi-%s-%s-%s", slug, environment, role)
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

// AzBootstrapConfig is written to .azbootstrap.jsonc in the output repo to
// record the bootstrap parameters for reference and future automation.
type AzBootstrapConfig struct {
	// SchemaVersion identifies the file format.
	SchemaVersion string `json:"schemaVersion"`
	// SubscriptionID is the target Azure subscription.
	SubscriptionID string `json:"subscriptionId"`
	// TenantID is the Azure AD tenant.
	TenantID string `json:"tenantId"`
	// StateStorage holds the provisioned state backend details.
	StateStorage StateStorageConfig `json:"stateStorage"`
}
