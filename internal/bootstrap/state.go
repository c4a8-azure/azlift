package bootstrap

import (
	"fmt"
	"regexp"
	"strings"
)

// StateStorageConfig holds the Azure resource details for Terraform state storage.
type StateStorageConfig struct {
	// SubscriptionID is the Azure subscription where state resources live.
	SubscriptionID string
	// ResourceGroupName is the RG that contains the storage account.
	ResourceGroupName string
	// StorageAccountName is the globally-unique storage account name.
	StorageAccountName string
	// ContainerName is the blob container for state files (default: "tfstate").
	ContainerName string
	// Location is the Azure region (default: "westeurope").
	Location string
}

// saNameRe matches valid storage account names (3–24 lowercase alphanumeric).
var saNameRe = regexp.MustCompile(`^[a-z0-9]{3,24}$`)

// DeriveStateConfig builds a StateStorageConfig from a repo name and
// subscription ID, deriving deterministic resource names.
//
// Naming rules:
//   - RG:  rg-tfstate-<sanitised-repo>
//   - SA:  st + first 22 chars of sanitised repo (lowercase alphanum only, 3–24)
//   - Container: "tfstate"
func DeriveStateConfig(subscriptionID, repoName, location string) StateStorageConfig {
	if location == "" {
		location = "westeurope"
	}

	sanitised := sanitiseRepoName(repoName)

	rg := fmt.Sprintf("rg-tfstate-%s", sanitised)

	// Storage account: "st" + up to 22 alphanum chars from sanitised name.
	saBase := onlyAlphaNum(sanitised)
	if len(saBase) > 22 {
		saBase = saBase[:22]
	}
	sa := "st" + saBase
	// Ensure minimum length of 3.
	for len(sa) < 3 {
		sa += "x"
	}

	return StateStorageConfig{
		SubscriptionID:     subscriptionID,
		ResourceGroupName:  rg,
		StorageAccountName: sa,
		ContainerName:      "tfstate",
		Location:           location,
	}
}

// ValidateStateConfig returns an error if cfg contains obviously invalid values.
func ValidateStateConfig(cfg StateStorageConfig) error {
	if cfg.SubscriptionID == "" {
		return fmt.Errorf("subscription ID is required")
	}
	if cfg.ResourceGroupName == "" {
		return fmt.Errorf("resource group name is required")
	}
	if !saNameRe.MatchString(cfg.StorageAccountName) {
		return fmt.Errorf("storage account name %q is invalid: must be 3–24 lowercase alphanumeric characters", cfg.StorageAccountName)
	}
	return nil
}

// sanitiseRepoName converts a repo name to a lowercase hyphen-separated slug
// safe for use in Azure resource names.
func sanitiseRepoName(name string) string {
	name = strings.ToLower(name)
	var sb strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
		} else {
			sb.WriteByte('-')
		}
	}
	result := sb.String()
	for strings.Contains(result, "--") {
		result = strings.ReplaceAll(result, "--", "-")
	}
	return strings.Trim(result, "-")
}

// onlyAlphaNum strips all non-alphanumeric characters (storage account names
// cannot contain hyphens).
func onlyAlphaNum(s string) string {
	var sb strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}
