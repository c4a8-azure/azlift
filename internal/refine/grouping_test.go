package refine

import (
	"path/filepath"
	"strings"
	"testing"
)

const mixedHCL = `
terraform {
  required_version = ">= 1.5"
}

resource "azurerm_virtual_network" "vnet" {
  name = "vnet-prod-core-westeu"
}

resource "azurerm_subnet" "snet" {
  name = "snet-prod-app-westeu"
}

resource "azurerm_linux_virtual_machine" "vm" {
  name = "vm-prod-web-westeu-001"
}

resource "azurerm_storage_account" "sa" {
  name = "stprodcore001"
}

resource "azurerm_unknown_type" "unk" {
  name = "unknown-resource"
}
`

func TestGroupResources_SplitsIntoFiles(t *testing.T) {
	tmp := t.TempDir()
	writeTFFile(t, tmp, "main.tf", mixedHCL)
	files, _ := ParseDir(tmp)

	out := GroupResources(files, tmp)

	fileMap := map[string]*ParsedFile{}
	for _, pf := range out {
		fileMap[filepath.Base(pf.Path)] = pf
	}

	if _, ok := fileMap["resources.networking.tf"]; !ok {
		t.Error("expected resources.networking.tf")
	}
	if _, ok := fileMap["resources.compute.tf"]; !ok {
		t.Error("expected resources.compute.tf")
	}
	if _, ok := fileMap["resources.storage.tf"]; !ok {
		t.Error("expected resources.storage.tf")
	}
	// main.tf contains the catch-all unknown resource; terraform/provider blocks
	// are stripped here because scaffold writes them to terraform.tf/providers.tf.
	if _, ok := fileMap["main.tf"]; !ok {
		t.Error("expected main.tf (catch-all for unknown resource types)")
	}
}

func TestGroupResources_ResourceInExactlyOneFile(t *testing.T) {
	tmp := t.TempDir()
	writeTFFile(t, tmp, "main.tf", mixedHCL)
	files, _ := ParseDir(tmp)

	out := GroupResources(files, tmp)

	// Count occurrences of each resource label across all output files.
	counts := map[string]int{}
	for _, pf := range out {
		content := string(pf.File.Bytes())
		for _, label := range []string{
			"azurerm_virtual_network",
			"azurerm_subnet",
			"azurerm_linux_virtual_machine",
			"azurerm_storage_account",
			"azurerm_unknown_type",
		} {
			if strings.Contains(content, label) {
				counts[label]++
			}
		}
	}

	for label, count := range counts {
		if count != 1 {
			t.Errorf("%s appears in %d files, want exactly 1", label, count)
		}
	}
}

func TestGroupResources_UnknownTypeGoesToMain(t *testing.T) {
	tmp := t.TempDir()
	writeTFFile(t, tmp, "main.tf", mixedHCL)
	files, _ := ParseDir(tmp)

	out := GroupResources(files, tmp)

	for _, pf := range out {
		if filepath.Base(pf.Path) == "main.tf" {
			if !strings.Contains(string(pf.File.Bytes()), "azurerm_unknown_type") {
				t.Error("unknown resource type should be in main.tf")
			}
			return
		}
	}
	t.Error("main.tf not found in output")
}

func TestGroupResources_TerraformBlockStripped(t *testing.T) {
	tmp := t.TempDir()
	writeTFFile(t, tmp, "main.tf", mixedHCL)
	files, _ := ParseDir(tmp)

	out := GroupResources(files, tmp)

	// terraform {} and provider {} blocks must NOT appear in any grouped output
	// file — scaffold writes them to terraform.tf/providers.tf/backend.tf to avoid
	// "duplicate block" errors from Terraform.
	for _, pf := range out {
		content := string(pf.File.Bytes())
		if strings.Contains(content, "required_version") {
			t.Errorf("terraform block leaked into %s — should be stripped by GroupResources", filepath.Base(pf.Path))
		}
		if strings.Contains(content, `provider "azurerm"`) {
			t.Errorf("provider block leaked into %s — should be stripped by GroupResources", filepath.Base(pf.Path))
		}
	}
}

func TestGroupResources_EmptyInput(t *testing.T) {
	tmp := t.TempDir()
	out := GroupResources([]*ParsedFile{}, tmp)
	if len(out) != 0 {
		t.Errorf("expected empty output, got %d files", len(out))
	}
}

func TestTargetFile_KnownTypes(t *testing.T) {
	cases := []struct {
		resourceType string
		wantFile     string
	}{
		{"azurerm_virtual_network", "resources.networking.tf"},
		{"azurerm_subnet", "resources.networking.tf"},
		{"azurerm_linux_virtual_machine", "resources.compute.tf"},
		{"azurerm_storage_account", "resources.storage.tf"},
		{"azurerm_key_vault", "resources.keyvault.tf"},
		{"azurerm_role_assignment", "resources.iam.tf"},
		{"azurerm_log_analytics_workspace", "resources.monitoring.tf"},
		{"azurerm_kubernetes_cluster", "resources.appservice.tf"},
		{"azurerm_something_custom", "main.tf"},
	}
	for _, tc := range cases {
		got := targetFile(tc.resourceType)
		if got != tc.wantFile {
			t.Errorf("targetFile(%q) = %q, want %q", tc.resourceType, got, tc.wantFile)
		}
	}
}
