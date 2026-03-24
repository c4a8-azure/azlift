package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testModuleConfig() ModuleConfig {
	return ModuleConfig{
		RepoName:       "infra-prod",
		RepoOrg:        "my-org",
		Environments:   []string{"prod", "staging", "dev"},
		SubscriptionID: "sub-123",
		TenantID:       "tenant-456",
		Location:       "westeurope",
		ResourceGroups: []string{"rg-myapp-prod"},
		IsCrossTenant:  false,
		StateStorage:   DeriveStateConfig("sub-123", "infra-prod", "westeurope"),
	}
}

func TestGenerateBootstrapModule_CreatesFiles(t *testing.T) {
	dir := t.TempDir()
	if err := GenerateBootstrapModule(testModuleConfig(), dir); err != nil {
		t.Fatalf("GenerateBootstrapModule: %v", err)
	}

	for _, name := range []string{"terraform.tf", "variables.tf", "main.tf", "outputs.tf"} {
		path := filepath.Join(dir, "bootstrap", name)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected bootstrap/%s to exist: %v", name, err)
		}
	}
}

func TestGenerateBootstrapModule_MainTFHasStorageAccount(t *testing.T) {
	dir := t.TempDir()
	cfg := testModuleConfig()
	if err := GenerateBootstrapModule(cfg, dir); err != nil {
		t.Fatalf("GenerateBootstrapModule: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "bootstrap", "main.tf"))
	content := string(data)

	if !strings.Contains(content, "azurerm_storage_account") {
		t.Error("main.tf should contain azurerm_storage_account")
	}
	if !strings.Contains(content, "azurerm_user_assigned_identity") {
		t.Error("main.tf should contain azurerm_user_assigned_identity")
	}
	if !strings.Contains(content, "azurerm_federated_identity_credential") {
		t.Error("main.tf should contain azurerm_federated_identity_credential")
	}
	if !strings.Contains(content, "github_repository_environment") {
		t.Error("main.tf should contain github_repository_environment")
	}
}

func TestGenerateBootstrapModule_SameTenantRGScope(t *testing.T) {
	dir := t.TempDir()
	cfg := testModuleConfig()
	cfg.IsCrossTenant = false
	if err := GenerateBootstrapModule(cfg, dir); err != nil {
		t.Fatalf("GenerateBootstrapModule: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "bootstrap", "main.tf"))
	content := string(data)

	if !strings.Contains(content, "resourceGroups") {
		t.Error("same-tenant main.tf should scope RBAC to resource groups")
	}
}

func TestGenerateBootstrapModule_CrossTenantSubscriptionScope(t *testing.T) {
	dir := t.TempDir()
	cfg := testModuleConfig()
	cfg.IsCrossTenant = true
	if err := GenerateBootstrapModule(cfg, dir); err != nil {
		t.Fatalf("GenerateBootstrapModule: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "bootstrap", "main.tf"))
	content := string(data)

	// Cross-tenant uses subscription scope (no /resourceGroups/ in RBAC scope)
	if strings.Contains(content, `"/subscriptions/${var.subscription_id}/resourceGroups`) {
		t.Error("cross-tenant main.tf should not scope RBAC to resource groups")
	}
	if !strings.Contains(content, `"/subscriptions/${var.subscription_id}"`) {
		t.Error("cross-tenant main.tf should scope RBAC to subscription")
	}
}

func TestGenerateBootstrapModule_OutputsTFHasMIClientIDs(t *testing.T) {
	dir := t.TempDir()
	if err := GenerateBootstrapModule(testModuleConfig(), dir); err != nil {
		t.Fatalf("GenerateBootstrapModule: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "bootstrap", "outputs.tf"))
	if !strings.Contains(string(data), "client_id") {
		t.Error("outputs.tf should expose MI client IDs")
	}
}
