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
// These names are passed to Invoke-AzBootstrap / Add-AzBootstrapEnvironment.
func MIName(repoName, environment, role string) string {
	slug := sanitiseRepoName(repoName)
	return fmt.Sprintf("mi-%s-%s-%s", slug, environment, role)
}

// roleGUIDForRole maps the azlift role name to a well-known Azure RBAC role GUID.
func roleGUIDForRole(role string) string {
	switch strings.ToLower(role) {
	case "apply":
		return "b24988ac-6180-42a0-ab88-20f7382dd24c" // Contributor
	default:
		return "acdd72a7-3385-48ef-bd42-f606fba81ae7" // Reader
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
