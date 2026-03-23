package terragrunt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateRoot_CreatesFile(t *testing.T) {
	tmp := t.TempDir()
	cfg := DefaultRootConfig()

	if err := GenerateRoot(tmp, cfg); err != nil {
		t.Fatalf("GenerateRoot: %v", err)
	}

	dest := filepath.Join(tmp, "terragrunt.hcl")
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("terragrunt.hcl not created: %v", err)
	}
}

func TestGenerateRoot_ContainsBackendConfig(t *testing.T) {
	tmp := t.TempDir()
	cfg := RootConfig{
		AzurermVersion:        "4.0",
		BackendResourceGroup:  "rg-mystate",
		BackendStorageAccount: "stmystate",
		BackendContainer:      "tfstate",
	}

	if err := GenerateRoot(tmp, cfg); err != nil {
		t.Fatalf("GenerateRoot: %v", err)
	}

	content, _ := os.ReadFile(filepath.Join(tmp, "terragrunt.hcl"))
	out := string(content)

	for _, want := range []string{
		"remote_state",
		"azurerm",
		"rg-mystate",
		"stmystate",
		"tfstate",
		"path_relative_to_include",
		"terraform.tfstate",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in root terragrunt.hcl\nfull:\n%s", want, out)
		}
	}
}

func TestGenerateRoot_ContainsProviderGenerate(t *testing.T) {
	tmp := t.TempDir()
	cfg := DefaultRootConfig()

	if err := GenerateRoot(tmp, cfg); err != nil {
		t.Fatalf("GenerateRoot: %v", err)
	}

	content, _ := os.ReadFile(filepath.Join(tmp, "terragrunt.hcl"))
	out := string(content)

	for _, want := range []string{
		`generate "provider"`,
		"hashicorp/azurerm",
		"~> 4.0",
		`features {}`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in provider generate block\nfull:\n%s", want, out)
		}
	}
}

func TestGenerateRoot_UsesEnvHcl(t *testing.T) {
	tmp := t.TempDir()
	if err := GenerateRoot(tmp, DefaultRootConfig()); err != nil {
		t.Fatalf("GenerateRoot: %v", err)
	}

	content, _ := os.ReadFile(filepath.Join(tmp, "terragrunt.hcl"))
	if !strings.Contains(string(content), "env.hcl") {
		t.Error("root terragrunt.hcl should read env.hcl via find_in_parent_folders")
	}
}
