package refine

import (
	"strings"
	"testing"
)

func TestGenerateBackend_ContainsRequiredFields(t *testing.T) {
	tmp := t.TempDir()
	cfg := DefaultBackendConfig("rg-prod-network")
	pf, err := GenerateBackend(tmp, cfg)
	if err != nil {
		t.Fatalf("GenerateBackend: %v", err)
	}

	out := string(pf.File.Bytes())
	for _, want := range []string{
		"backend",
		"azurerm",
		"rg-prod-network/terraform.tfstate",
		"resource_group_name",
		"storage_account_name",
		"container_name",
		"key",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("backend.tf missing %q\nfull output:\n%s", want, out)
		}
	}
}

func TestGenerateBackend_PlaceholdersWhenEmpty(t *testing.T) {
	tmp := t.TempDir()
	cfg := BackendConfig{StateKey: "mykey"}
	pf, err := GenerateBackend(tmp, cfg)
	if err != nil {
		t.Fatalf("GenerateBackend: %v", err)
	}

	out := string(pf.File.Bytes())
	if !strings.Contains(out, "<resource_group_name>") {
		t.Error("empty ResourceGroupName should produce placeholder")
	}
	if !strings.Contains(out, "<storage_account_name>") {
		t.Error("empty StorageAccountName should produce placeholder")
	}
}

func TestGenerateVersions_DefaultPins(t *testing.T) {
	tmp := t.TempDir()
	pf, err := GenerateVersions(tmp, "", nil)
	if err != nil {
		t.Fatalf("GenerateVersions: %v", err)
	}

	out := string(pf.File.Bytes())
	for _, want := range []string{
		"required_version",
		">= 1.5.0",
		"required_providers",
		"hashicorp/azurerm",
		"~> 4.0",
		"azure/azapi",
		"~> 2.0",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("versions.tf missing %q\nfull output:\n%s", want, out)
		}
	}
}

func TestGenerateVersions_CustomPin(t *testing.T) {
	tmp := t.TempDir()
	pins := []ProviderPin{{Source: "hashicorp/azurerm", Version: "~> 3.0"}}
	pf, err := GenerateVersions(tmp, ">= 1.6.0", pins)
	if err != nil {
		t.Fatalf("GenerateVersions: %v", err)
	}

	out := string(pf.File.Bytes())
	if !strings.Contains(out, "~> 3.0") {
		t.Errorf("expected custom pin ~> 3.0, output:\n%s", out)
	}
	if !strings.Contains(out, ">= 1.6.0") {
		t.Errorf("expected custom required_version, output:\n%s", out)
	}
}

func TestGenerateProvider_AzurermBlock(t *testing.T) {
	tmp := t.TempDir()
	pf, err := GenerateProvider(tmp)
	if err != nil {
		t.Fatalf("GenerateProvider: %v", err)
	}

	out := string(pf.File.Bytes())
	if !strings.Contains(out, "provider") || !strings.Contains(out, "azurerm") {
		t.Errorf("providers.tf missing provider azurerm block:\n%s", out)
	}
	if !strings.Contains(out, "features") {
		t.Errorf("providers.tf missing features block:\n%s", out)
	}
}

func TestDefaultBackendConfig_StateKey(t *testing.T) {
	cfg := DefaultBackendConfig("rg-network-prod")
	if cfg.StateKey != "rg-network-prod/terraform.tfstate" {
		t.Errorf("unexpected state key: %s", cfg.StateKey)
	}
}
