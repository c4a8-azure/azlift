package terragrunt_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/c4a8-azure/azlift/internal/refine"
	"github.com/c4a8-azure/azlift/internal/terragrunt"
)

// writeTF writes content to dir/<name>.
func writeTF(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
}

// minimalRefinedModule returns a temporary directory with the minimal set of
// files that the refine stage would produce for a single-RG workload.
func minimalRefinedModule(t *testing.T) (dir string, files []*refine.ParsedFile) {
	t.Helper()
	dir = t.TempDir()

	writeTF(t, dir, "locals.tf", `locals {
  resource_group_name = "rg-myapp-prod"
  common_tags = {
    environment = ""
    owner       = ""
  }
}
`)
	writeTF(t, dir, "variables.tf", `variable "location" {
  description = "Azure region."
  type        = string
  default     = "westeurope"
}
`)
	writeTF(t, dir, "terraform.tf", `terraform {
  required_version = ">= 1.10"
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 4.0"
    }
  }
}
`)
	writeTF(t, dir, "resources.networking.tf", `resource "azurerm_virtual_network" "vnet" {
  name                = "vnet-prod"
  location            = var.location
  resource_group_name = local.resource_group_name
  tags                = merge(local.common_tags, {})
}
`)
	writeTF(t, dir, "backend.tf", `terraform {
  backend "azurerm" {
    resource_group_name  = "rg-tfstate"
    storage_account_name = "sttfstate"
    container_name       = "tfstate"
    key                  = "rg-myapp-prod/terraform.tfstate"
  }
}
`)
	writeTF(t, dir, "providers.tf", `terraform {
  required_providers {
    azurerm = {
      source = "hashicorp/azurerm"
    }
  }
}
provider "azurerm" {
  features {}
}
`)

	var err error
	files, err = refine.ParseDir(dir)
	if err != nil {
		t.Fatalf("ParseDir: %v", err)
	}
	return dir, files
}

func TestRun_NoDuplicateTFFilesInRoot(t *testing.T) {
	_, files := minimalRefinedModule(t)
	outDir := t.TempDir()

	if err := terragrunt.Run(files, terragrunt.Options{
		OutputDir:    outDir,
		Environments: []string{"prod"},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// No *.tf files should exist directly in outDir — they belong in module/.
	entries, err := filepath.Glob(filepath.Join(outDir, "*.tf"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(entries) > 0 {
		t.Errorf("found unexpected .tf files in output root: %v", entries)
	}
}

func TestRun_CreatesModuleDir(t *testing.T) {
	_, files := minimalRefinedModule(t)
	outDir := t.TempDir()

	opts := terragrunt.Options{
		OutputDir:           outDir,
		Environments:        []string{"prod", "dev"},
		SourceResourceGroup: "rg-myapp-prod",
	}
	if err := terragrunt.Run(files, opts); err != nil {
		t.Fatalf("Run: %v", err)
	}

	moduleDir := filepath.Join(outDir, "module")
	if _, err := os.Stat(moduleDir); err != nil {
		t.Errorf("module/ dir not created: %v", err)
	}
}

func TestRun_ModuleExcludesBackendAndProviders(t *testing.T) {
	_, files := minimalRefinedModule(t)
	outDir := t.TempDir()

	if err := terragrunt.Run(files, terragrunt.Options{
		OutputDir:    outDir,
		Environments: []string{"prod"},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	moduleDir := filepath.Join(outDir, "module")
	for _, skip := range []string{"backend.tf", "providers.tf"} {
		if _, err := os.Stat(filepath.Join(moduleDir, skip)); err == nil {
			t.Errorf("%s should NOT be copied into module/", skip)
		}
	}
}

func TestRun_ModuleVariablesHasEnvironment(t *testing.T) {
	_, files := minimalRefinedModule(t)
	outDir := t.TempDir()

	if err := terragrunt.Run(files, terragrunt.Options{
		OutputDir:    outDir,
		Environments: []string{"prod"},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(outDir, "module", "variables.tf"))
	if err != nil {
		t.Fatalf("reading variables.tf: %v", err)
	}
	content := string(raw)
	if !strings.Contains(content, `variable "environment"`) {
		t.Error("module/variables.tf should declare variable \"environment\"")
	}
	if !strings.Contains(content, `variable "resource_group_name"`) {
		t.Error("module/variables.tf should declare variable \"resource_group_name\"")
	}
}

func TestRun_ModuleLocalsRemovesRGName(t *testing.T) {
	_, files := minimalRefinedModule(t)
	outDir := t.TempDir()

	if err := terragrunt.Run(files, terragrunt.Options{
		OutputDir:    outDir,
		Environments: []string{"prod"},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(outDir, "module", "locals.tf"))
	if err != nil {
		t.Fatalf("reading locals.tf: %v", err)
	}
	content := string(raw)
	if strings.Contains(content, `resource_group_name = "rg-myapp-prod"`) {
		t.Error("module/locals.tf should not contain resource_group_name literal")
	}
}

func TestRun_ModuleLocalsWiresEnvironmentVar(t *testing.T) {
	_, files := minimalRefinedModule(t)
	outDir := t.TempDir()

	if err := terragrunt.Run(files, terragrunt.Options{
		OutputDir:    outDir,
		Environments: []string{"prod"},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(outDir, "module", "locals.tf"))
	if err != nil {
		t.Fatalf("reading locals.tf: %v", err)
	}
	if !strings.Contains(string(raw), `var.environment`) {
		t.Error("module/locals.tf common_tags should reference var.environment")
	}
}

func TestRun_ResourceFileUsesVarRG(t *testing.T) {
	_, files := minimalRefinedModule(t)
	outDir := t.TempDir()

	if err := terragrunt.Run(files, terragrunt.Options{
		OutputDir:    outDir,
		Environments: []string{"prod"},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(outDir, "module", "resources.networking.tf"))
	if err != nil {
		t.Fatalf("reading resources.networking.tf: %v", err)
	}
	content := string(raw)
	if strings.Contains(content, "local.resource_group_name") {
		t.Error("resource file should not reference local.resource_group_name")
	}
	if !strings.Contains(content, "var.resource_group_name") {
		t.Error("resource file should reference var.resource_group_name")
	}
}

func TestRun_CreatesRootHCL(t *testing.T) {
	_, files := minimalRefinedModule(t)
	outDir := t.TempDir()

	if err := terragrunt.Run(files, terragrunt.Options{
		OutputDir:    outDir,
		Environments: []string{"prod"},
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(outDir, "root.hcl"))
	if err != nil {
		t.Fatalf("root.hcl not created: %v", err)
	}
	content := string(raw)
	if !strings.Contains(content, "remote_state") {
		t.Error("root.hcl should contain remote_state block")
	}
	if !strings.Contains(content, `generate "provider"`) {
		t.Error("root.hcl should contain generate \"provider\" block")
	}
	if !strings.Contains(content, "westeurope") {
		t.Error("root.hcl should include the extracted location")
	}
}

func TestRun_CreatesEnvStacks(t *testing.T) {
	_, files := minimalRefinedModule(t)
	outDir := t.TempDir()

	if err := terragrunt.Run(files, terragrunt.Options{
		OutputDir:           outDir,
		Environments:        []string{"prod", "dev"},
		SourceResourceGroup: "rg-myapp-prod",
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	for _, env := range []string{"prod", "dev"} {
		path := filepath.Join(outDir, "envs", env, "terragrunt.hcl")
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("envs/%s/terragrunt.hcl not created: %v", env, err)
		}
		content := string(raw)
		if !strings.Contains(content, `find_in_parent_folders("root.hcl")`) {
			t.Errorf("envs/%s/terragrunt.hcl should include root.hcl", env)
		}
		if !strings.Contains(content, `"../../module"`) {
			t.Errorf("envs/%s/terragrunt.hcl should source ../../module", env)
		}
		if !strings.Contains(content, fmt.Sprintf(`environment = %q`, env)) {
			t.Errorf("envs/%s/terragrunt.hcl should set environment = %q", env, env)
		}
	}
}

func TestRun_DevEnvHasDerivedRGName(t *testing.T) {
	_, files := minimalRefinedModule(t)
	outDir := t.TempDir()

	if err := terragrunt.Run(files, terragrunt.Options{
		OutputDir:           outDir,
		Environments:        []string{"prod", "dev"},
		SourceResourceGroup: "rg-myapp-prod",
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(outDir, "envs", "dev", "terragrunt.hcl"))
	if err != nil {
		t.Fatalf("envs/dev/terragrunt.hcl not created: %v", err)
	}
	if !strings.Contains(string(raw), "rg-myapp-dev") {
		t.Error("dev env stack should reference rg-myapp-dev (derived from rg-myapp-prod)")
	}
}
