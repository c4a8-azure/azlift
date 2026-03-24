package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"
)

// BackendConfig holds the values for generating backend.tf.
type BackendConfig struct {
	// ResourceGroupName is the RG containing the state storage account.
	ResourceGroupName string
	// StorageAccountName is the globally-unique storage account name.
	StorageAccountName string
	// ContainerName is the blob container (default: "tfstate").
	ContainerName string
	// Key is the blob path for the state file (default: "<RepoName>.tfstate").
	Key string
	// Placeholder, when true, writes FILL_IN values for cross-tenant deployments.
	Placeholder bool
}

// WriteBackend writes backend.tf into repoDir.
//
// Same-tenant (Placeholder=false): real storage account values.
// Cross-tenant (Placeholder=true): FILL_IN placeholders with a comment directing
// the operator to apply bootstrap/ first.
func WriteBackend(cfg BackendConfig, repoDir string) error {
	container := cfg.ContainerName
	if container == "" {
		container = "tfstate"
	}

	var content string
	if cfg.Placeholder {
		content = fmt.Sprintf(`# Run the bootstrap/ Terraform module first, then replace FILL_IN values and run:
#   terraform init -reconfigure
terraform {
  backend "azurerm" {
    resource_group_name  = "FILL_IN"
    storage_account_name = "FILL_IN"
    container_name       = %q
    key                  = %q
    use_oidc             = true
  }
}
`, container, cfg.Key)
	} else {
		content = fmt.Sprintf(`terraform {
  backend "azurerm" {
    resource_group_name  = %q
    storage_account_name = %q
    container_name       = %q
    key                  = %q
    use_oidc             = true
  }
}
`,
			cfg.ResourceGroupName,
			cfg.StorageAccountName,
			container,
			cfg.Key,
		)
	}

	dest := filepath.Join(repoDir, "backend.tf")
	if err := os.WriteFile(dest, []byte(content), 0o644); err != nil { //nolint:gosec
		return fmt.Errorf("writing backend.tf: %w", err)
	}
	return nil
}
