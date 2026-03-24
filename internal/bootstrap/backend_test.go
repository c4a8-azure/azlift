package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteBackend_SameTenant(t *testing.T) {
	dir := t.TempDir()
	cfg := BackendConfig{
		ResourceGroupName:  "rg-tfstate-infra-prod",
		StorageAccountName: "stinfraprod",
		ContainerName:      "tfstate",
		Key:                "infra-prod.tfstate",
		Placeholder:        false,
	}

	if err := WriteBackend(cfg, dir); err != nil {
		t.Fatalf("WriteBackend: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "backend.tf"))
	if err != nil {
		t.Fatalf("reading backend.tf: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "stinfraprod") {
		t.Error("backend.tf should contain storage account name")
	}
	if !strings.Contains(content, "rg-tfstate-infra-prod") {
		t.Error("backend.tf should contain resource group name")
	}
	if strings.Contains(content, "FILL_IN") {
		t.Error("same-tenant backend.tf should not contain FILL_IN")
	}
}

func TestWriteBackend_CrossTenant_Placeholder(t *testing.T) {
	dir := t.TempDir()
	cfg := BackendConfig{
		ContainerName: "tfstate",
		Key:           "infra-prod.tfstate",
		Placeholder:   true,
	}

	if err := WriteBackend(cfg, dir); err != nil {
		t.Fatalf("WriteBackend: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "backend.tf"))
	content := string(data)

	if !strings.Contains(content, "FILL_IN") {
		t.Error("cross-tenant backend.tf should contain FILL_IN placeholders")
	}
	if !strings.Contains(content, "bootstrap/") {
		t.Error("cross-tenant backend.tf should mention bootstrap/ module in comment")
	}
}

func TestWriteBackend_DefaultContainer(t *testing.T) {
	dir := t.TempDir()
	cfg := BackendConfig{
		ResourceGroupName:  "rg-tfstate-test",
		StorageAccountName: "sttest",
		Key:                "test.tfstate",
	}

	if err := WriteBackend(cfg, dir); err != nil {
		t.Fatalf("WriteBackend: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "backend.tf"))
	if !strings.Contains(string(data), `"tfstate"`) {
		t.Error("backend.tf should default container_name to tfstate")
	}
}
